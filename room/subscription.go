package room

import (
	"sync/atomic"
)

type subscriptionInner struct {
	rm     *Manager
	uname  string
	wake   chan struct{}
	done   chan error
	isDead atomic.Bool
}
type Subscription = *subscriptionInner

type HistoryNotify struct {
	Shift int
	Len   int
	Sub   Subscription
}

type SubscriptionWake int

const (
	SubscriptionNotify SubscriptionWake = iota
	SubscriptionClose
	SubscriptionKick
)

// we have a map by *subscriptionInner and an atomic inside, so it's better to forget about raw inner type to avoid unwanted copy

func sendWake[T any](c chan<- T, val T) bool {
	select {
	case c <- val:
		return true
	default:
		return false
	}
}

func (sub Subscription) stop() {
	sub.isDead.Store(true)
	close(sub.wake)
}

func (sub Subscription) GetRoomName() string {
	return sub.rm.Name
}

func (sub Subscription) TryGetShiftAndHistLen() (int, int, bool) {
	s, l := sub.rm.historyReader.getShiftAndLen()
	if sub.isDead.Load() {
		return 0, 0, false
	} else {
		return s, l, true
	}
}

func (sub Subscription) TryGetMessage(i int) (GetResponse, bool) {
	if sub.isDead.Load() { // not necessary for consistency, but may save some time
		return GetResponse{}, false
	}
	if res, ok := sub.rm.historyReader.getMessage(i); !ok || sub.isDead.Load() {
		return GetResponse{}, false
	} else {
		return res, true
	}
}

func (sub Subscription) AddMessage(msg Message) {
	if !sub.isDead.Load() { // not necessary for consistency, but may save some time
		reply := make(chan bool, 1)
		sub.rm.addMsgReq <- addMessageRequest{
			msg:   msg,
			reply: reply,
		}
		// _ = <-reply
	}
}

func (sub Subscription) Unsub() {
	if !sub.isDead.Load() { // not necessary for consistency, but may save some time
		sub.rm.unsubReq <- sub
	}
}

func (rm *Manager) Subscribe(uname string) (Subscription, error) {
	subscription := &subscriptionInner{
		rm:     rm,
		uname:  uname,
		wake:   make(chan struct{}, 1),
		done:   make(chan error, 1),
		isDead: atomic.Bool{}, // = false
	}
	rm.subReq <- subscription
	err := <-subscription.done
	return subscription, err
}

func (sub Subscription) Start(notify chan<- HistoryNotify, notifyClose chan<- Subscription) {
	defer func() { notifyClose <- sub }()
	s := 0
	l := 0
WL:
	for {
		ns, nl, ok := sub.TryGetShiftAndHistLen()
		if !ok {
			break WL
		}
		if nl > l {
			s = max(s, ns)
			l = nl
			notify <- HistoryNotify{
				Shift: s,
				Len:   l,
				Sub:   sub,
			}
		}
		_, ok = <-sub.wake
		if !ok {
			break WL
		}
	}
}
