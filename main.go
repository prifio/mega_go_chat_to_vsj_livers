package main

import (
	"fmt"
	"log"
	"net/http"
	"vcmsg/connection"
	"vcmsg/toplevel"
)

func homePage(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Home Page")
}

// type globalManager struct { // in future - multirooms and extra non-chat-history logic like auth
// 	hm *history.Manager
// }

// func newGlobalManager(hm *history.Manager) globalManager {
// 	return globalManager{
// 		hm: hm,
// 	}
// }

// func (gm *globalManager) launch() {
// 	go gm.hm.Start()
// 	handler := func(w http.ResponseWriter, r *http.Request) {
// 		cm, err := connection.NewManager(gm.hm, w, r)
// 		if err != nil {
// 			return
// 		}
// 		go cm.Start()
// 	}
// 	http.HandleFunc("/ws", handler)
// }

func main() {
	http.HandleFunc("/", homePage)
	tlm := toplevel.InitTestManager()
	tlm.LaunchRooms()
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		cm, err := connection.NewManager(w, r, tlm)
		if err != nil {
			log.Println("Attem to connect failed")
			return
		}
		go cm.Start()
	})

	fmt.Println("Init finished!")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
