package main

import (
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

func homePage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Home Page")
}

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

func readWebSocketTextMessage(conn *websocket.Conn) (string, readStatus, error) {
	messageType, message, err := conn.ReadMessage()
	if err != nil {
		return "", closeRead, err
	}
	if messageType != websocket.TextMessage || !utf8.Valid(message) {
		return "", errRead, nil
	}
	return string(message), oKRead, nil
}
