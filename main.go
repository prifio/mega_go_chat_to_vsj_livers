package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

type letter struct {
	uname string
	txt   string
}

type globalManager struct { // in future - multirooms and extra non-chat-history logic like auth
	hm *histManager
}

type histMangAddReq struct {
	lt    letter
	reply chan bool
}

type histManagerSubsc struct {
	wake    chan int // actually, we don't really rely on the passed value
	isAlive *atomic.Bool
}

type twoHistBufs struct {
	buf0  []letter
	buf1  []letter
	shift int
}

type histManager struct {
	bufs    atomic.Pointer[twoHistBufs]
	histLen atomic.Uint64
	letReq  chan histMangAddReq
	subReq  chan histManagerSubsc
	subs    []histManagerSubsc
}

type userManager struct {
	hm             *histManager
	conn           *websocket.Conn
	uname          string
	sendChan       chan letter
	hmSubscription histManagerSubsc
}

func newGlobalManager(hm *histManager) globalManager {
	return globalManager{
		hm: hm,
	}
}

func (gm *globalManager) launch() {
	gm.hm.launch()
	handler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("WARN: Cannot upgrade connection")
			return
		}
		user := newUserManager(gm.hm, conn)
		user.launch()
	}
	http.HandleFunc("/ws", handler)
}

func newHistManager(halfHistLen int) *histManager {
	buf0 := make([]letter, halfHistLen)
	buf1 := make([]letter, halfHistLen)
	bufs := twoHistBufs{
		buf0:  buf0,
		buf1:  buf1,
		shift: 0,
	}

	letReq := make(chan histMangAddReq, 16)
	subReq := make(chan histManagerSubsc, 16)
	subs := make([]histManagerSubsc, 0, 1)
	hm := histManager{
		bufs:    atomic.Pointer[twoHistBufs]{},
		histLen: atomic.Uint64{}, // = 0
		letReq:  letReq,
		subReq:  subReq,
		subs:    subs,
	}
	hm.bufs.Store(&bufs)
	return &hm
}

func (hm *histManager) launch() {
	go func() {
		startBufs := hm.bufs.Load()
		// local copies of some atomic hm vars
		buf0 := startBufs.buf0
		buf1 := startBufs.buf1
		bufLen := len(buf0)
		histLen := 0
		shift := 0
		startBufs = nil // drop shared ownership

		processGetReq := func(req histMangAddReq) bool {
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
			req, ok := <-hm.letReq
			if !ok {
				fmt.Println("INFO: Closing history manager")
				isChanAlive = false
				break LL
			}
			if ok := processGetReq(req); !ok {
				continue LL
			}
		IL:
			for range 10 { // up to 10 extra messages, if they are ready
				select {
				case req, ok := <-hm.letReq:
					{
						if !ok {
							isChanAlive = false
							fmt.Println("INFO: Start closing history manager")
							break IL
						}
						processGetReq(req)
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
					curSub.wake <- histLen
					hm.subs[curSubsCnt] = curSub
					curSubsCnt++
				}
			}
			if curSubsCnt < oldSubsCnt {
				hm.subs = hm.subs[:curSubsCnt]
			}
		}
	}()
}

func (hm *histManager) getHistLen() int {
	return int(hm.histLen.Load())
}

func (hm *histManager) getLetter(i int) (lt letter, ok bool) {
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

func (hm *histManager) addLetter(lt letter) bool {
	reply := make(chan bool, 1)
	hm.letReq <- histMangAddReq{
		lt:    lt,
		reply: reply,
	}
	return <-reply
}

func (hm *histManager) addSubsc() histManagerSubsc {
	wake := make(chan int, 1)
	isAlive := atomic.Bool{}
	isAlive.Store(true)
	subscription := histManagerSubsc{
		wake:    wake,
		isAlive: &isAlive,
	}
	hm.subReq <- subscription
	return subscription
}

func (hm *histManager) unSub(subscription histManagerSubsc) { // hm still may send wake signals
	subscription.isAlive.Store(false)
}

func newUserManager(hm *histManager, conn *websocket.Conn) userManager {
	sendChan := make(chan letter, 1)
	hmSubscription := hm.addSubsc()
	return userManager{
		hm:             hm,
		conn:           conn,
		uname:          "",
		sendChan:       sendChan,
		hmSubscription: hmSubscription,
	}
}

func (um *userManager) sendNotify(histLen int) error { // todo: send also left bound
	message := fmt.Sprintf("0%v", histLen)
	return um.conn.WriteMessage(websocket.TextMessage, []byte(message))
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
			txt, status, _ := readWebSocketTextMessage(um.conn)
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
	go func() { // web socket listener
		defer um.conn.Close()
		defer um.hm.unSub(um.hmSubscription)
	SL:
		for {
			select {
			case lt, ok := <-um.sendChan:
				{
					if !ok { // connection closed from reader
						break SL
					}
					message := fmt.Sprintf("1%v: %v", lt.uname, lt.txt)
					err := um.conn.WriteMessage(websocket.TextMessage, []byte(message))
					if err != nil {
						log.Printf("WARN: Client %v; Cannot send message, close connection\n", um.uname)
						break SL
					}
				}
			case <-um.hmSubscription.wake:
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
	go func() {
		uname, stat, _ := readWebSocketTextMessage(um.conn)
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
	}()
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
