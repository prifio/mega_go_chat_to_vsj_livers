package connection

import (
	"encoding/json"
	"log"
	"net/http"
	"vcmsg/room"

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

func (cm *Manager) wsSend(msg wsMessage) (err error, isClosed bool) {
	bytesMsg, err := json.Marshal(&msg)
	if err != nil {
		log.Printf("WARN: Cannt marshal message of message index %v\n", msg.TypeInd)
		return err, false
	}
	if err := cm.conn.WriteMessage(websocket.TextMessage, bytesMsg); err != nil {
		return err, true
	} else {
		return nil, false
	}
}

func (cm *Manager) wsRead() (*wsMessage, readStatus, error) {
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

func genericSend[T interface{ toWsMessage() wsMessage }](cm *Manager, msg T) {
	cm.sends <- msg.toWsMessage()
}

func (cm *Manager) sendNotify(shift int, historyLen int, room string) {
	genericSend(cm, serverNotify{
		HistoryLen:     historyLen,
		FirstAvailable: shift,
	})
}

func (cm *Manager) sendMessage(msg *room.GetResponse) {
	genericSend(cm, serverSendMessage{
		Uname:        msg.Msg.Uname,
		Txt:          msg.Msg.Txt,
		RequestedInd: msg.RequestedInd,
		ResultInd:    msg.ResultInd,
		HistoryLen:   msg.HistLen,
	})
}

// func (cm *Manager) loginClient() error {
// 	req, status, _ := cm.read()
// 	if status != oKRead || req.TypeInd != clientLoginInd {
// 		return fmt.Errorf("Cannot login client")
// 	}
// 	cm.uname = req.Content.(*clientLogin).Uname
// 	return nil
// }
