package history

import "sync/atomic"

type Message struct {
	uname string
	txt   string
}

type Manager struct {
	bufs    atomic.Pointer[twoHistBufs]
	histLen atomic.Uint64
	addReq  chan AddRequest
	subReq  chan Subscription
	subs    []Subscription
}

type Subscription struct {
	wake    chan struct{}
	isAlive *atomic.Bool
}

type twoHistBufs struct {
	buf0  []Message
	buf1  []Message
	shift int
}

type AddRequest struct {
	msg   Message
	reply chan bool
}

func NewHistManager(halfHistLen int) *Manager {
	buf0 := make([]Message, halfHistLen)
	buf1 := make([]Message, halfHistLen)
	bufs := twoHistBufs{
		buf0:  buf0,
		buf1:  buf1,
		shift: 0,
	}

	msgReq := make(chan AddRequest, 16)
	subReq := make(chan Subscription, 16)
	subs := make([]Subscription, 0, 1)
	hm := Manager{
		bufs:    atomic.Pointer[twoHistBufs]{},
		histLen: atomic.Uint64{}, // = 0
		addReq:  msgReq,
		subReq:  subReq,
		subs:    subs,
	}
	hm.bufs.Store(&bufs)
	return &hm
}
