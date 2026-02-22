package connection

import (
	"encoding/json"
	"fmt"
)

type messageType int

// explicit enumerate: it's part of API
const (
	serverNotifyInd      messageType = 0
	serverSendMessageInd messageType = 1
	clientAskMessageInd  messageType = 2
	clientSendMessageInd messageType = 3
	clientLoginInd       messageType = 4
)

type wsMessage struct {
	typeInd messageType
	content any
}

type serverNotify struct {
	HistoryLen int
}
type serverSendMessage struct {
	Uname        string
	Txt          string
	RequestedInd int
	ResultInd    int
	HistoryLen   int
}

type clientAskMessage struct {
	Ind int
}
type clientSendMessage struct {
	Txt string
}
type clientLogin struct {
	Uname string
}

func unmarshalClientReq(msg []byte) (*wsMessage, error) {
	type reqJson struct {
		Type    int
		Content json.RawMessage
	}
	typeResolvedReq := reqJson{}
	if err := json.Unmarshal(msg, &typeResolvedReq); err != nil {
		return nil, err
	}
	typeInd := typeResolvedReq.Type
	var dst any
	switch typeResolvedReq.Type {
	case int(clientAskMessageInd):
		dst = &clientAskMessage{}
	case int(clientSendMessageInd):
		dst = &clientSendMessage{}
	case int(clientLoginInd):
		dst = &clientLogin{}
	default:
		return nil, fmt.Errorf("Invalid client request type: %v", typeInd)
	}
	if err := json.Unmarshal(typeResolvedReq.Content, dst); err != nil {
		return nil, err
	}
	return &wsMessage{typeInd: messageType(typeInd), content: dst}, nil
}

func (notify serverNotify) toJsonMessage() ([]byte, error) {
	return json.Marshal(&wsMessage{
		typeInd: serverNotifyInd,
		content: notify,
	})
}
func (msg serverSendMessage) toJsonMessage() ([]byte, error) {
	return json.Marshal(&wsMessage{
		typeInd: serverSendMessageInd,
		content: msg,
	})
}
