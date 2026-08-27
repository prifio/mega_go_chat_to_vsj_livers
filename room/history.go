package room

import (
	"log"
	"sync/atomic"
)

type Message struct {
	Uname string
	Txt   string
}

type GetResponse struct {
	Msg          Message
	RequestedInd int
	ResultInd    int
	HistLen      int
}

type twoHistoryBufs struct {
	buf0  []Message
	buf1  []Message
	shift int
}

type historyAsync struct { // object for async reads from history. may delay from main loop local history
	bufs atomic.Pointer[twoHistoryBufs]
	len  atomic.Uint64
}

type history struct {
	bufLen int
	buf0   []Message
	buf1   []Message
	len    int
	shift  int
}

func (hs *history) addMessage(hsAsync *historyAsync, msg Message) {
	locInd := hs.len % hs.bufLen
	if locInd == 0 && hs.len >= 2*hs.bufLen { // need new buf
		buf2 := make([]Message, hs.bufLen)
		buf2[0] = msg
		hs.buf0 = hs.buf1
		hs.buf1 = buf2
		hs.shift += hs.bufLen
		newBufs := twoHistoryBufs{
			buf0:  hs.buf0,
			buf1:  hs.buf1,
			shift: hs.shift,
		}
		hsAsync.bufs.Store(&newBufs)
	} else if hs.len < hs.bufLen {
		hs.buf0[hs.len] = msg
	} else {
		hs.buf1[locInd] = msg
	}
	hs.len++
	hsAsync.len.Store(uint64(hs.len))
}

func (history *historyAsync) getShiftAndLen() (int, int) {
	shift := history.bufs.Load().shift
	histLen := max(int(history.len.Load()), shift)
	return shift, histLen
}

func (history *historyAsync) getMessage(i int) (GetResponse, bool) {
	hl := int(history.len.Load())
	if hl <= i {
		return GetResponse{}, false
	}
	bufs := history.bufs.Load()
	hl = max(hl, bufs.shift)
	var res GetResponse
	if i < bufs.shift {
		log.Printf("INFO: Asked for outdated message %v, have only %v\n", i, bufs.shift)
		res = GetResponse{
			Msg:          bufs.buf0[0],
			RequestedInd: i,
			ResultInd:    bufs.shift,
			HistLen:      hl,
		}
	} else {
		iLoc := i - bufs.shift
		bufLen := len(bufs.buf0)
		msg := Message{}
		if iLoc < bufLen {
			msg = bufs.buf0[iLoc]
		} else {
			msg = bufs.buf1[iLoc-bufLen]
		}
		res = GetResponse{
			Msg:          msg,
			RequestedInd: i,
			ResultInd:    i,
			HistLen:      hl,
		}
	}
	return res, true
}
