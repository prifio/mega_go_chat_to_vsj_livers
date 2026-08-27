package connection

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/gorilla/websocket"

	"vcmsg/room"
	"vcmsg/toplevel"
)

type Manager struct {
	conn     *websocket.Conn
	uname    atomic.Pointer[string]
	sends    chan wsMessage
	reads    chan wsMessage
	notifies chan room.HistoryNotify
	unsubs   chan room.Subscription
	toplevel *toplevel.Manager
}

type localState struct {
	uname         string
	subscriptions map[string]room.Subscription
}

func NewManager(w http.ResponseWriter, r *http.Request, toplevel *toplevel.Manager) (*Manager, error) {
	conn, err := getConn(w, r)
	if err != nil {
		return nil, err
	}
	mgr := Manager{
		conn:     conn,
		uname:    atomic.Pointer[string]{},
		sends:    make(chan wsMessage, 16),
		reads:    make(chan wsMessage, 4),
		notifies: make(chan room.HistoryNotify, 1),
		unsubs:   make(chan room.Subscription, 1),
		toplevel: toplevel,
	}
	empt := ""
	mgr.uname.Store(&empt)
	return &mgr, nil
}

func (ls *localState) processAskMessage(cm *Manager, req *clientAskMessage) error {
	sub, ok := ls.subscriptions[req.Room]
	if !ok {
		return fmt.Errorf("User %v requests message from not subscribed room %v", ls.uname, req.Room)
	}
	msg, ok := sub.TryGetMessage(req.Ind)
	if !ok {
		return fmt.Errorf("User %v requests for non-existing message %v", ls.uname, req.Ind)
	}
	cm.sendMessage(&msg)
	return nil
}

func (ls *localState) processSendMessage(req *clientSendMessage) error {
	sub, ok := ls.subscriptions[req.Room]
	if !ok {
		return fmt.Errorf("User %v sends message to not subscribed room %v", ls.uname, req.Room)
	}
	sub.AddMessage(room.Message{
		Uname: ls.uname,
		Txt:   req.Txt,
	})
	return nil
}

func (ls *localState) processLogin(cm *Manager, req *clientLogin) error {
	if ls.uname != "" {
		return fmt.Errorf("Relogin of user %v in one connection", ls.uname)
	}
	uname := req.Uname
	if uname == "" {
		return fmt.Errorf("Login with empty uname")
	}
	// TODO: add auth and etc
	log.Printf("INFO: Client %v successfully logged in\n", uname)
	ls.uname = uname
	cm.uname.Store(&uname)

	user := cm.toplevel.Users[uname]
	for room := range user.Rooms {
		sub, err := cm.toplevel.Rooms[room].Subscribe(uname)
		if err != nil {
			log.Printf("INFO: Cannot subscribe user %v to room %v\n", uname, room)
		} else {
			ls.subscriptions[room] = sub
			sub.Start(cm.notifies, cm.unsubs)
		}
	}
	return nil
}

func (ls *localState) processMessage(cm *Manager, req wsMessage) error {
	if ls.uname == "" && req.TypeInd != clientLoginInd {
		return fmt.Errorf("Non-auth request for not logined connection")
	}
	switch req.TypeInd {
	case clientAskMessageInd:
		return ls.processAskMessage(cm, req.Content.(*clientAskMessage))
	case clientSendMessageInd:
		return ls.processSendMessage(req.Content.(*clientSendMessage))
	case clientLoginInd:
		return ls.processLogin(cm, req.Content.(*clientLogin))
	default:
		{
			log.Println("ERR: process invalid client request type, should be unreachable!")
			return fmt.Errorf("Invalid client request type: %v", req.TypeInd)
		}
	}
}

func (cm *Manager) readLoop() {
	defer close(cm.reads)
LL:
	for {
		req, readStatus, err := cm.wsRead()
		switch readStatus {
		case closeRead:
			{
				log.Printf("INFO: Client %v disconnected\n", cm.uname.Load())
				break LL
			}
		case invalidMsgTypeRead:
			{
				log.Printf("WARN: Client %v sent invalid message\n", cm.uname.Load())
				continue LL
			}
		case parseErrRead:
			{
				log.Printf("WARN: Cannot parse client request: %v\n", err)
				continue LL
			}
		}
		cm.reads <- *req
	}
}

func (cm *Manager) sendLoop() {
SL:
	for msg := range cm.sends {
		if _, isClosed := cm.wsSend(msg); isClosed {
			log.Printf("INFO: User %v is disconnected, don't send sheduled messages\n", cm.uname.Load())
			break SL
		}
	}
}

func (ls *localState) unsubAll() {
	for _, sub := range ls.subscriptions {
		sub.Unsub()
	}
	clear(ls.subscriptions)
}

func (cm *Manager) Start() {
	defer cm.conn.Close()
	ls := localState{
		subscriptions: map[string]room.Subscription{},
	}
	defer ls.unsubAll()

	go cm.readLoop()

	defer close(cm.sends)
	go cm.sendLoop()

LL:
	for {
		select {
		case req, isNotClosed := <-cm.reads:
			{
				if !isNotClosed {
					break LL
				}
				ls.processMessage(cm, req)
			}
		case notify := <-cm.notifies:
			{
				room := notify.Sub.GetRoomName()
				curSub, ok := ls.subscriptions[room]
				if ok && curSub == notify.Sub {
					cm.sendNotify(notify.Shift, notify.Len, room)
				}
			}
		case sub := <-cm.unsubs:
			{
				room := sub.GetRoomName()
				curSub, ok := ls.subscriptions[room]
				if ok && curSub == sub {
					delete(ls.subscriptions, room)
				}
			}
		}
	}
}
