package main

import (
	"fmt"
	"log"
	"net/http"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

type UserManager struct {
	hm                         *HistoryManager
	conn                       *websocket.Conn
	uname                      string
	sendChan                   chan Message
	historyManagerSubscription HistoryManagerSubscription
}

func newUserManager(hm *HistoryManager, w http.ResponseWriter, r *http.Request) (*UserManager, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WARN: Cannot upgrade connection")
		return nil, err
	}

	sendChan := make(chan Message, 1)
	hmSubscription := hm.addSubsc()
	return &UserManager{
		hm:                         hm,
		conn:                       conn,
		uname:                      "",
		sendChan:                   sendChan,
		historyManagerSubscription: hmSubscription,
	}, nil
}

func (um *UserManager) send(message string) error {
	return um.conn.WriteMessage(websocket.TextMessage, []byte(message))
}

func (um *UserManager) sendNotify(histLen int) error { // todo: send also left bound
	message := fmt.Sprintf("0%v", histLen)
	return um.conn.WriteMessage(websocket.TextMessage, []byte(message))
}

func (um *UserManager) read() (string, ReadStatus, error) {
	messageType, message, err := um.conn.ReadMessage()
	if err != nil {
		return "", closeRead, err
	}
	if messageType != websocket.TextMessage || !utf8.Valid(message) {
		return "", errRead, nil
	}
	return string(message), oKRead, nil
}
