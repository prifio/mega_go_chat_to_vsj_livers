package history

import (
	"fmt"
	"log"
	"sync/atomic"
)

type Message struct {
	Uname string
	Txt   string
}

type Manager struct {
	bufs    atomic.Pointer[twoHistBufs]
	histLen atomic.Uint64
	addReq  chan addRequest
	subReq  chan Subscription
}

type Subscription struct {
	Wake    chan struct{}
	isAlive *atomic.Bool
}

type twoHistBufs struct {
	buf0  []Message
	buf1  []Message
	shift int
}

type addRequest struct {
	msg   Message
	reply chan bool
}

func NewManager(halfHistLen int) *Manager {
	buf0 := make([]Message, halfHistLen)
	buf1 := make([]Message, halfHistLen)
	bufs := twoHistBufs{
		buf0:  buf0,
		buf1:  buf1,
		shift: 0,
	}

	hm := Manager{
		bufs:    atomic.Pointer[twoHistBufs]{},
		histLen: atomic.Uint64{}, // = 0
		addReq:  make(chan addRequest, 16),
		subReq:  make(chan Subscription, 16),
	}
	hm.bufs.Store(&bufs)
	return &hm
}

func (hm *Manager) Start() {
	startBufs := hm.bufs.Load()
	// local copies of some atomic hm vars
	buf0 := startBufs.buf0
	buf1 := startBufs.buf1
	bufLen := len(buf0)
	histLen := 0
	shift := 0
	subs := make([]Subscription, 0, 1)
	startBufs = nil // drop shared ownership

	processAddReq := func(req addRequest) bool {
		locInd := histLen % bufLen
		if locInd == 0 && histLen >= 2*bufLen { // need new buf
			buf2 := make([]Message, bufLen)
			buf2[0] = req.msg
			buf0 = buf1
			buf1 = buf2
			shift += bufLen
			newBufs := twoHistBufs{
				buf0:  buf0,
				buf1:  buf1,
				shift: shift,
			}
			hm.bufs.Store(&newBufs)
		} else if histLen < bufLen {
			buf0[histLen] = req.msg
		} else {
			buf1[locInd] = req.msg
		}
		histLen++
		hm.histLen.Store(uint64(histLen))
		req.reply <- true
		return true
	}

	isWaitAddReqs := true
LL:
	for isWaitAddReqs {
		req, ok := <-hm.addReq
		if !ok {
			fmt.Println("INFO: Closing history manager")
			isWaitAddReqs = false
			break LL
		}
		if ok := processAddReq(req); !ok {
			continue LL
		}
	EL:
		for range 10 { // up to 10 extra messages, if they are ready
			select {
			case req, ok := <-hm.addReq:
				{
					if !ok {
						isWaitAddReqs = false
						fmt.Println("INFO: Start closing history manager")
						break EL
					}
					processAddReq(req)
				}
			default:
				break EL
			}
		}
		// mb move from subscriptions+wake channels into cond variable+mutex?
	SL:
		for {
			select {
			case newSub := <-hm.subReq:
				subs = append(subs, newSub)
			default:
				break SL
			}
		}
		oldSubsCnt := len(subs)
		curSubsCnt := 0
		for i := range oldSubsCnt {
			curSub := subs[i]
			if curSub.isAlive.Load() {
				curSub.Wake <- struct{}{}
				subs[curSubsCnt] = curSub
				curSubsCnt++
			}
		}
		if curSubsCnt < oldSubsCnt {
			subs = subs[:curSubsCnt]
		}
	}
}

func (hm *Manager) GetHistLen() int {
	return int(hm.histLen.Load())
}

func (hm *Manager) GetMessage(i int) (lt Message, ok bool) {
	if hl := hm.GetHistLen(); hl <= i {
		return Message{}, false
	}
	bufs := hm.bufs.Load()
	if i < bufs.shift { // todo: return with flag that this is the oldest we can provide
		log.Printf("INFO: Asked for outdated message %v, have only %v\n", i, bufs.shift)
		firstAv := bufs.buf0[0]
		msg := Message{
			Uname: firstAv.Uname,
			Txt:   fmt.Sprintf("*dont have such old message, there is the oldest available* %v", firstAv.Txt),
		}
		return msg, true
	}
	iLoc := i - bufs.shift
	bufLen := len(bufs.buf0)
	if iLoc < bufLen {
		return bufs.buf0[iLoc], true
	}
	return bufs.buf1[iLoc-bufLen], true
}

func (hm *Manager) AddMessage(msg Message) bool {
	reply := make(chan bool, 1)
	hm.addReq <- addRequest{
		msg:   msg,
		reply: reply,
	}
	return <-reply
}

func (hm *Manager) Subscribe() Subscription {
	isAlive := atomic.Bool{}
	isAlive.Store(true)
	subscription := Subscription{
		Wake:    make(chan struct{}, 1),
		isAlive: &isAlive,
	}
	hm.subReq <- subscription
	return subscription
}

func (hm *Manager) UnSub(subscription Subscription) { // hm still may send wake signals
	subscription.isAlive.Store(false)
}
