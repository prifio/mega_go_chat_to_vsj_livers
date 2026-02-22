package connection

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"vcmsg/history"
)

type Manager struct {
	hm                         *history.Manager
	conn                       *websocket.Conn
	uname                      string
	sendChan                   chan history.GetResponse
	historyManagerSubscription history.Subscription
}

func NewManager(hm *history.Manager, w http.ResponseWriter, r *http.Request) (*Manager, error) {
	conn, err := getConn(w, r)
	if err != nil {
		return nil, err
	}
	hmSubscription := hm.Subscribe()
	return &Manager{
		hm:                         hm,
		conn:                       conn,
		uname:                      "",
		sendChan:                   make(chan history.GetResponse, 1),
		historyManagerSubscription: hmSubscription,
	}, nil
}

func (cm *Manager) processMessage(req *wsMessage) error {
	switch req.typeInd {
	case clientAskMessageInd:
		{
			content := req.content.(clientAskMessage)
			msg, ok := cm.hm.GetMessage(content.Ind)
			if !ok {
				return fmt.Errorf("Request for non-existing message %v", content.Ind)
			}
			cm.sendChan <- msg
			return nil
		}
	case clientSendMessageInd:
		{
			content := req.content.(clientSendMessage)
			msg := history.Message{
				Uname: cm.uname,
				Txt:   content.Txt,
			}
			ok := cm.hm.AddMessage(msg)
			if !ok { // now it's always ok
				return fmt.Errorf("History overflow")
			}
			return nil
		}
	case clientLoginInd:
		return fmt.Errorf("Unexpected relogin")
	default:
		{
			log.Println("ERR: process invalid client request type, should be unreachable!")
			return fmt.Errorf("Invalid client request type: %v", req.typeInd)
		}
	}
}

func (cm *Manager) startListener() {
	defer close(cm.sendChan)
LL:
	for {
		req, status, err := cm.read()
		switch status {
		case closeRead:
			{
				log.Printf("INFO: Client %v disconnected\n", cm.uname)
				break LL
			}
		case invalidMsgTypeRead:
			{
				log.Printf("WARN: Client %v sent invalid message", cm.uname)
				continue LL
			}
		case parseErrRead:
			{
				log.Printf("WARN: Cannot parse client request: %v\n", err)
				continue LL
			}
		}
		if err := cm.processMessage(req); err != nil {
			log.Printf("WARN: Client %v; %v", cm.uname, err)
		}
	}
}

func (cm *Manager) startWriter(sendedHistLen int) {
	histLen := sendedHistLen
SL:
	for {
		select {
		case msg, ok := <-cm.sendChan:
			{
				if !ok { // connection closed from reader
					break SL
				}
				if err := cm.sendMessage(&msg); err != nil {
					log.Printf("WARN: Client %v; Cannot send message, close connection; Err: %v\n", cm.uname, err)
					break SL
				}
			}
		case <-cm.historyManagerSubscription.Wake:
			{
				newHistLen := cm.hm.GetHistLen()
				if newHistLen > histLen { // seems always true
					cm.sendNotify(newHistLen)
					histLen = newHistLen
				}
			}
		}
	}
}

func (cm *Manager) Start() {
	defer cm.conn.Close()
	defer cm.hm.UnSub(cm.historyManagerSubscription)

	if err := cm.loginClient(); err != nil {
		log.Println("WARN: Cannot login client")
		return
	}
	histLen := cm.hm.GetHistLen()
	if histLen > 0 {
		err := cm.sendNotify(histLen)
		if err != nil {
			log.Printf("WARN: Cannot init client %v\n", cm.uname)
			return
		}
	}
	log.Printf("INFO: Client %v successfully logged in\n", cm.uname)
	go cm.startListener()
	cm.startWriter(histLen)
}
