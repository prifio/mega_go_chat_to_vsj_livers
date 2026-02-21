package main

import (
	"fmt"
	"net/http"

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
