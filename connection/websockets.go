package connection

import (
	"fmt"
	"log"
	"net/http"
	"vcmsg/history"

	"github.com/gorilla/websocket"
)

type readStatus int

const (
	oKRead readStatus = iota
	closeRead
	invalidMsgTypeRead
	parseErrRead
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func getConn(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WARN: Cannot upgrade connection: %v\n", err)
	}
	return conn, err
}

func (cm *Manager) send(msg interface{ toJsonMessage() ([]byte, error) }) error {
	bytesMsg, err := msg.toJsonMessage()
	if err != nil {
		return err
	}
	return cm.conn.WriteMessage(websocket.TextMessage, bytesMsg)
}

func (cm *Manager) read() (*wsMessage, readStatus, error) {
	messageType, message, err := cm.conn.ReadMessage()
	if err != nil {
		return nil, closeRead, err
	}
	if messageType != websocket.TextMessage {
		return nil, invalidMsgTypeRead, nil
	}
	req, err := unmarshalClientReq(message)
	if err != nil {
		return nil, parseErrRead, err
	}
	return req, oKRead, nil
}

func (cm *Manager) sendNotify(shift int, historyLen int) error {
	return cm.send(serverNotify{
		HistoryLen:     historyLen,
		FirstAvailable: shift,
	})
}

func (cm *Manager) sendMessage(msg *history.GetResponse, histLen int) error {
	return cm.send(serverSendMessage{
		Uname:        msg.Msg.Uname,
		Txt:          msg.Msg.Txt,
		RequestedInd: msg.RequestedInd,
		ResultInd:    msg.ResultInd,
		HistoryLen:   histLen,
	})
}

func (cm *Manager) loginClient() error {
	req, status, _ := cm.read()
	if status != oKRead || req.TypeInd != clientLoginInd {
		return fmt.Errorf("Cannot login client")
	}
	cm.uname = req.Content.(*clientLogin).Uname
	return nil
}
