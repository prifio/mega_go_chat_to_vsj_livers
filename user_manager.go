package main

import (
	"fmt"
	"log"
	"net/http"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

type userManager struct {
	hm                         *historyManager
	conn                       *websocket.Conn
	uname                      string
	sendChan                   chan letter
	historyManagerSubscription historyManagerSubscription
}

func newUserManager(hm *historyManager, w http.ResponseWriter, r *http.Request) (*userManager, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WARN: Cannot upgrade connection")
		return nil, err
	}

	sendChan := make(chan letter, 1)
	hmSubscription := hm.addSubsc()
	return &userManager{
		hm:                         hm,
		conn:                       conn,
		uname:                      "",
		sendChan:                   sendChan,
		historyManagerSubscription: hmSubscription,
	}, nil
}

func (um *userManager) send(message string) error {
	return um.conn.WriteMessage(websocket.TextMessage, []byte(message))
}

func (um *userManager) sendNotify(histLen int) error { // todo: send also left bound
	message := fmt.Sprintf("0%v", histLen)
	return um.conn.WriteMessage(websocket.TextMessage, []byte(message))
}

func (um *userManager) read() (string, readStatus, error) {
	messageType, message, err := um.conn.ReadMessage()
	if err != nil {
		return "", closeRead, err
	}
	if messageType != websocket.TextMessage || !utf8.Valid(message) {
		return "", errRead, nil
	}
	return string(message), oKRead, nil
}
