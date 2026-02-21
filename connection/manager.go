package connection

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/gorilla/websocket"

	"vcmsg/history"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type readStatus int

const (
	oKRead readStatus = iota
	closeRead
	errRead
)

type Manager struct {
	hm                         *history.Manager
	conn                       *websocket.Conn
	uname                      string
	sendChan                   chan history.Message
	historyManagerSubscription history.Subscription
}

func NewManager(hm *history.Manager, w http.ResponseWriter, r *http.Request) (*Manager, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WARN: Cannot upgrade connection")
		return nil, err
	}

	sendChan := make(chan history.Message, 1)
	hmSubscription := hm.Subscribe()
	return &Manager{
		hm:                         hm,
		conn:                       conn,
		uname:                      "",
		sendChan:                   sendChan,
		historyManagerSubscription: hmSubscription,
	}, nil
}

func (cm *Manager) send(content string) error {
	return cm.conn.WriteMessage(websocket.TextMessage, []byte(content))
}

func (cm *Manager) read() (string, readStatus, error) {
	messageType, message, err := cm.conn.ReadMessage()
	if err != nil {
		return "", closeRead, err
	}
	if messageType != websocket.TextMessage || !utf8.Valid(message) {
		return "", errRead, nil
	}
	return string(message), oKRead, nil
}

func (cm *Manager) sendNotify(histLen int) error { // todo: send also left bound
	message := fmt.Sprintf("0%v", histLen)
	return cm.send(message)
}

func (cm *Manager) processMessage(txt string) (errmsg string, ok bool) {
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
			msg, ok := cm.hm.GetMessage(ind)
			if !ok {
				return fmt.Sprintf("Request for non-existing message %v", ind), false
			}
			cm.sendChan <- msg
			return "", true
		}
	case '1':
		{ // write
			msg := history.Message{
				Uname: cm.uname,
				Txt:   txt[1:],
			}
			ok := cm.hm.AddMessage(msg)
			if !ok { // now it's always ok
				return "History overflow", false
			}
			return "", true
		}
	default:
		return "Invalid request", false
	}
}

func (cm *Manager) startListener() {
	defer close(cm.sendChan)
LL:
	for {
		txt, status, _ := cm.read()
		switch status {
		case closeRead:
			{
				log.Printf("INFO: Client %v disconnected\n", cm.uname)
				break LL
			}
		case errRead:
			{
				log.Printf("WARN: Client %v sent invalid message", cm.uname)
				continue LL
			}
		}
		errmsg, ok := cm.processMessage(txt)
		if !ok {
			log.Printf("WARN: Client %v; %v", cm.uname, errmsg)
		}
	}
}

func (cm *Manager) startWriter(sendedHistLen int) {
	defer cm.conn.Close()
	defer cm.hm.UnSub(cm.historyManagerSubscription)

	histLen := sendedHistLen
SL:
	for {
		select {
		case msg, ok := <-cm.sendChan:
			{
				if !ok { // connection closed from reader
					break SL
				}
				message := fmt.Sprintf("1%v: %v", msg.Uname, msg.Txt)
				err := cm.send(message)

				if err != nil {
					log.Printf("WARN: Client %v; Cannot send message, close connection\n", cm.uname)
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
	uname, stat, _ := cm.read()
	if stat != oKRead {
		log.Println("WARN: Cannot login client")
		return
	}
	cm.uname = uname
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
	go cm.startWriter(histLen)
}
