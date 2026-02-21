package main

import (
	"fmt"
	"log"
	"net/http"

	"vcmsg/connection"
	"vcmsg/history"
)

func homePage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Home Page")
}

type globalManager struct { // in future - multirooms and extra non-chat-history logic like auth
	hm *history.Manager
}

func newGlobalManager(hm *history.Manager) globalManager {
	return globalManager{
		hm: hm,
	}
}

func (gm *globalManager) launch() {
	go gm.hm.Start()
	handler := func(w http.ResponseWriter, r *http.Request) {
		cm, err := connection.NewManager(gm.hm, w, r)
		if err != nil {
			return
		}
		go cm.Start()
	}
	http.HandleFunc("/ws", handler)
}

func main() {
	http.HandleFunc("/", homePage)
	halfHistLen := 1
	// halfHistLen := 4
	// halfHistLen := 1024
	hm := history.NewManager(halfHistLen)
	gm := newGlobalManager(hm)
	gm.launch()
	fmt.Println("Init finished!")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
