package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
)

type letter struct {
	uname string
	txt   string
}

type globalManager struct { // in future - multirooms and extra non-chat-history logic like auth
	hm *historyManager
}

type histMangAddReq struct {
	lt    letter
	reply chan bool
}

type historyManagerSubscription struct {
	wake    chan struct{}
	isAlive *atomic.Bool
}

type twoHistBufs struct {
	buf0  []letter
	buf1  []letter
	shift int
}

type historyManager struct {
	bufs    atomic.Pointer[twoHistBufs]
	histLen atomic.Uint64
	addReq  chan histMangAddReq
	subReq  chan historyManagerSubscription
	subs    []historyManagerSubscription
}

func newGlobalManager(hm *historyManager) globalManager {
	return globalManager{
		hm: hm,
	}
}

func (gm *globalManager) launch() {
	go gm.hm.launch()
	handler := func(w http.ResponseWriter, r *http.Request) {
		user, err := newUserManager(gm.hm, w, r)
		if err != nil {
			return
		}
		go user.launch()
	}
	http.HandleFunc("/ws", handler)
}

func newHistManager(halfHistLen int) *historyManager {
	buf0 := make([]letter, halfHistLen)
	buf1 := make([]letter, halfHistLen)
	bufs := twoHistBufs{
		buf0:  buf0,
		buf1:  buf1,
		shift: 0,
	}

	letReq := make(chan histMangAddReq, 16)
	subReq := make(chan historyManagerSubscription, 16)
	subs := make([]historyManagerSubscription, 0, 1)
	hm := historyManager{
		bufs:    atomic.Pointer[twoHistBufs]{},
		histLen: atomic.Uint64{}, // = 0
		addReq:  letReq,
		subReq:  subReq,
		subs:    subs,
	}
	hm.bufs.Store(&bufs)
	return &hm
}

func (hm *historyManager) launch() {
	startBufs := hm.bufs.Load()
	// local copies of some atomic hm vars
	buf0 := startBufs.buf0
	buf1 := startBufs.buf1
	bufLen := len(buf0)
	histLen := 0
	shift := 0
	startBufs = nil // drop shared ownership

	processAddReq := func(req histMangAddReq) bool {
		locInd := histLen % bufLen
		if locInd == 0 && histLen >= 2*bufLen { // need new buf
			buf2 := make([]letter, bufLen)
			buf2[0] = req.lt
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
			buf0[histLen] = req.lt
		} else {
			buf1[locInd] = req.lt
		}
		histLen++
		hm.histLen.Store(uint64(histLen))
		req.reply <- true
		return true
	}

	isChanAlive := true
LL:
	for isChanAlive {
		req, ok := <-hm.addReq
		if !ok {
			fmt.Println("INFO: Closing history manager")
			isChanAlive = false
			break LL
		}
		if ok := processAddReq(req); !ok {
			continue LL
		}
	IL:
		for range 10 { // up to 10 extra messages, if they are ready
			select {
			case req, ok := <-hm.addReq:
				{
					if !ok {
						isChanAlive = false
						fmt.Println("INFO: Start closing history manager")
						break IL
					}
					processAddReq(req)
				}
			default:
				break IL
			}
		}
		// mb move from subscriptions+wake channels into cond variable+mutex?
	SL:
		for {
			select {
			case newSub := <-hm.subReq:
				hm.subs = append(hm.subs, newSub)
			default:
				break SL
			}
		}
		oldSubsCnt := len(hm.subs)
		curSubsCnt := 0
		for i := range oldSubsCnt {
			curSub := hm.subs[i]
			if curSub.isAlive.Load() {
				curSub.wake <- struct{}{}
				hm.subs[curSubsCnt] = curSub
				curSubsCnt++
			}
		}
		if curSubsCnt < oldSubsCnt {
			hm.subs = hm.subs[:curSubsCnt]
		}
	}
}

func (hm *historyManager) getHistLen() int {
	return int(hm.histLen.Load())
}

func (hm *historyManager) getLetter(i int) (lt letter, ok bool) {
	if hl := hm.getHistLen(); hl <= i {
		return letter{}, false
	}
	bufs := hm.bufs.Load()
	if i < bufs.shift { // todo: return with flag that this is the oldest we can provide
		log.Printf("INFO: Asked for outdated message %v, have only %v\n", i, bufs.shift)
		firstAv := bufs.buf0[0]
		lt := letter{
			uname: firstAv.uname,
			txt:   fmt.Sprintf("*dont have such old message, there is the oldest available* %v", firstAv.txt),
		}
		return lt, true
	}
	iLoc := i - bufs.shift
	bufLen := len(bufs.buf0)
	if iLoc < bufLen {
		return bufs.buf0[iLoc], true
	}
	return bufs.buf1[iLoc-bufLen], true
}

func (hm *historyManager) addLetter(lt letter) bool {
	reply := make(chan bool, 1)
	hm.addReq <- histMangAddReq{
		lt:    lt,
		reply: reply,
	}
	return <-reply
}

func (hm *historyManager) addSubsc() historyManagerSubscription {
	wake := make(chan struct{}, 1)
	isAlive := atomic.Bool{}
	isAlive.Store(true)
	subscription := historyManagerSubscription{
		wake:    wake,
		isAlive: &isAlive,
	}
	hm.subReq <- subscription
	return subscription
}

func (hm *historyManager) unSub(subscription historyManagerSubscription) { // hm still may send wake signals
	subscription.isAlive.Store(false)
}

func (um *userManager) processMessage(txt string) (errmsg string, ok bool) {
	if len(txt) == 0 {
		return "Invalid request", false
	}
	switch txt[0] {
	case '0':
		{ // read
			ind, err := strconv.Atoi(txt[1:])
			if err != nil {
				return "Invalid request", false
			}
			lt, ok := um.hm.getLetter(ind)
			if !ok {
				return fmt.Sprintf("Request for non-existing message %v", ind), false
			}
			um.sendChan <- lt
			return "", true
		}
	case '1':
		{ // write
			lt := letter{
				uname: um.uname,
				txt:   txt[1:],
			}
			ok := um.hm.addLetter(lt)
			if !ok { // now it's always ok
				return "History overflow", false
			}
			return "", true
		}
	default:
		return "Invalid request", false
	}
}

func (um *userManager) launchListener() {
	go func() { // web socket listener
		defer close(um.sendChan)
	LL:
		for {
			txt, status, _ := um.read()
			if status == closeRead {
				log.Printf("INFO: Client %v disconnected\n", um.uname)
				break LL
			}
			if status == errRead {
				log.Printf("WARN: Client %v sent invalid message", um.uname)
				continue LL
			}
			errmsg, ok := um.processMessage(txt)
			if !ok {
				log.Printf("WARN: Client %v; %v", um.uname, errmsg)
			}
		}
	}()
}

func (um *userManager) launchWriter(sendedHistLen int) {
	histLen := sendedHistLen
	go func() { // web socket writer
		defer um.conn.Close()
		defer um.hm.unSub(um.historyManagerSubscription)
	SL:
		for {
			select {
			case lt, ok := <-um.sendChan:
				{
					if !ok { // connection closed from reader
						break SL
					}
					message := fmt.Sprintf("1%v: %v", lt.uname, lt.txt)
					err := um.send(message)

					if err != nil {
						log.Printf("WARN: Client %v; Cannot send message, close connection\n", um.uname)
						break SL
					}
				}
			case <-um.historyManagerSubscription.wake:
				{
					newHistLen := um.hm.getHistLen()
					if newHistLen > histLen {
						um.sendNotify(newHistLen)
						histLen = newHistLen
					}
				}
			}
		}
	}()
}

func (um *userManager) launch() {
	uname, stat, _ := um.read()
	if stat != oKRead {
		log.Println("WARN: Cannot login client")
		return
	}
	um.uname = uname
	histLen := um.hm.getHistLen()
	if histLen > 0 {
		err := um.sendNotify(histLen)
		if err != nil {
			log.Printf("WARN: Cannot init client %v\n", um.uname)
			return
		}
	}
	log.Printf("INFO: Client %v successfully logged in\n", um.uname)
	um.launchListener()
	um.launchWriter(histLen)
}

func main() {
	http.HandleFunc("/", homePage)
	halfHistLen := 1
	// halfHistLen := 4
	// halfHistLen := 1024
	hm := newHistManager(halfHistLen)
	gm := newGlobalManager(hm)
	gm.launch()
	fmt.Println("Init finished!")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
