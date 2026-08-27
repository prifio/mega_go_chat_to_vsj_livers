package room

import (
	"fmt"
	"log"
	"sync/atomic"
)

type Manager struct {
	Name          string
	historyReader historyAsync
	addMsgReq     chan addMessageRequest
	subReq        chan Subscription
	unsubReq      chan Subscription
	addUserReq    chan string
	removeUserReq chan string
}

type localState struct { // state of main room loop, which is single-thread
	history history
	subs    map[Subscription]bool
	users   map[string]bool
}

type addMessageRequest struct {
	msg   Message
	reply chan bool
}

func NewManager(name string, halfHistLen int) *Manager {
	buf0 := make([]Message, halfHistLen)
	buf1 := make([]Message, halfHistLen)
	bufs := twoHistoryBufs{
		buf0:  buf0,
		buf1:  buf1,
		shift: 0,
	}
	rm := Manager{
		Name: name,
		historyReader: historyAsync{
			bufs: atomic.Pointer[twoHistoryBufs]{},
			len:  atomic.Uint64{}, // = 0
		},
		addMsgReq:     make(chan addMessageRequest, 16),
		subReq:        make(chan Subscription, 1),
		unsubReq:      make(chan Subscription, 1),
		addUserReq:    make(chan string, 1),
		removeUserReq: make(chan string, 1),
	}
	rm.historyReader.bufs.Store(&bufs)
	return &rm
}

func (ls *localState) processAddMessage(ha *historyAsync, req addMessageRequest) error {
	uname := req.msg.Uname
	if _, ok := ls.users[uname]; !ok {
		req.reply <- false
		return fmt.Errorf("Cannot add message from invalid user %v", uname)
	}
	ls.history.addMessage(ha, req.msg)
	req.reply <- true
	return nil
}

func (ls *localState) processSubscribe(sub Subscription) {
	if _, ok := ls.subs[sub]; ok {
		log.Printf("ERR: double subscribe call for user %v\n", sub.uname)
		return
	}
	if _, ok := ls.users[sub.uname]; !ok {
		sub.done <- fmt.Errorf("Cannot subscribe invalid user %v", sub.uname)
		sub.isDead.Store(true)
		return
	}
	ls.subs[sub] = true
	sub.done <- nil
}

func (ls *localState) processUnsubscribe(sub Subscription) {
	if _, ok := ls.subs[sub]; !ok {
		return
	}
	delete(ls.subs, sub)
	sub.stop()
}

func (ls *localState) processAddUser(uname string) bool {
	if _, ok := ls.users[uname]; ok {
		log.Printf("WARN: Request to add already existed user %v\n", uname)
		return false
	}
	ls.users[uname] = true
	return true
}

func (ls *localState) processRemoveUser(uname string) bool {
	if _, ok := ls.users[uname]; !ok {
		log.Printf("WARN: Request to remove not existed user %v\n", uname)
		return false
	}
	delete(ls.users, uname)
	for sub := range ls.subs {
		if sub.uname == uname {
			delete(ls.subs, sub) // it's correct to iterate and delete keys in the same time. see https://go.dev/ref/spec#For_range
			sub.stop()
		}
	}
	return true
}

func (ls *localState) processManyMessages(rm *Manager, req addMessageRequest) bool {
	ls.processAddMessage(&rm.historyReader, req)
	for range 16 { // up to 16 extra messages, if they are ready
		select {
		case req, ok := <-rm.addMsgReq:
			{
				if !ok {
					return false
				}
				ls.processAddMessage(&rm.historyReader, req)
			}
		default:
			return true
		}
	}
	return true
}

func (ls *localState) notify() {
	for sub := range ls.subs {
		sendWake(sub.wake, struct{}{})
	}
}

func (ls *localState) unsabAll() {
	for sub := range ls.subs {
		sub.stop()
	}
	clear(ls.subs)
}

func (rm *Manager) Start() {
	startBufs := rm.historyReader.bufs.Load()
	ls := localState{
		history: history{
			bufLen: len(startBufs.buf0),
			buf0:   startBufs.buf0,
			buf1:   startBufs.buf1,
			len:    0,
			shift:  0,
		},
		subs:  map[Subscription]bool{},
		users: map[string]bool{},
	}
	startBufs = nil // drop shared ownership
	defer ls.unsabAll()
LL:
	for {
		select {
		case req, notClose := <-rm.addMsgReq:
			{
				if !notClose {
					fmt.Println("INFO: Closing history manager")
					break LL
				}
				notClose = ls.processManyMessages(rm, req)
				ls.notify()
				if !notClose {
					fmt.Println("INFO: Closing history manager")
					break LL
				}
			}
		case sub := <-rm.subReq:
			ls.processSubscribe(sub)
		case sub := <-rm.unsubReq:
			ls.processUnsubscribe(sub)
		case uname := <-rm.addUserReq:
			ls.processAddUser(uname)
		case uname := <-rm.removeUserReq:
			ls.processRemoveUser(uname)
		}
	}
}

func (rm *Manager) AddUser(uname string) {
	rm.addUserReq <- uname
}

func (rm *Manager) RemoveUser(uname string) {
	rm.removeUserReq <- uname
}

func (rm *Manager) Stop() {
	close(rm.addMsgReq)
}
