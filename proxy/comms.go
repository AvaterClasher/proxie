package proxy

import (
	"encoding/json"
	"hash/fnv"
	"log"
	"net"
	"net/http"
	"time"
)

const (
	UNICAST_MESSAGE       = 0  
	MULTICAST_MESSAGE     = 1 
	JOIN_REQUEST_MESSAGE  = 2  
	JOIN_NOTIFY_MESSAGE   = 3  
	LEAVE_NOTIFY_MESSAGE  = 4  
	HTTP_REQUEST_MESSAGE  = 5
	HTTP_RESPONSE_MESSAGE = 6
	ELECTION_MESSAGE      = 7 
	ANSWER_MESSAGE        = 8 
	VICTORY_MESSAGE       = 9 
)

type Message struct {
	Timestamp   time.Time
	Data        []byte
	SenderUrl   string
	MessageType int
}

type TCPMessenger struct {
	Listener            net.Listener
	RecentMessageHashes map[uint32]time.Time
}

type HTTPRequest struct {
	Method        string
	RequestUrl    string
	Header        http.Header
	Body          []byte
	ContentLength int64
}

type HTTPResponse struct {
	Status        string
	RequestUrl    string
	Header        http.Header
	Body          []byte
	ContentLength int64
}

func InitTCPMessenger(url string) *TCPMessenger {
	rv := &TCPMessenger{}
	rv.RecentMessageHashes = make(map[uint32]time.Time)
	l, err := net.Listen("tcp", url)
	if err != nil {
		log.Fatal(err)
	}
	rv.Listener = l
	return rv
}

func CreateMessage(message []byte, sender_url string, message_type int) Message {
	rv := Message{
		Timestamp:   time.Now(),
		SenderUrl:   sender_url,
		Data:        message,
		MessageType: message_type,
	}
	return rv
}

func MessageToBytes(message Message) []byte {
	b, err := json.Marshal(message)
	if err != nil {
		log.Fatal(err)
	}
	return b
}

func BytesToMessage(bytes []byte) Message {
	rv := Message{}
	json.Unmarshal(bytes, &rv)
	return rv
}

func HashBytes(b []byte) uint32 {
	h := fnv.New32a()
	h.Write(b)
	return h.Sum32()
}

func (m TCPMessenger) PruneStoredMessages() {
	now := time.Now()
	for key := range m.RecentMessageHashes {
		if now.After(m.RecentMessageHashes[key].Add(time.Duration(1.0 * time.Second))) {
			delete(m.RecentMessageHashes, key)
		}
	}
}

func (m TCPMessenger) HasMessageStored(hash uint32) bool {
	_, ok := m.RecentMessageHashes[hash]
	return ok
}

func HttpRequestToBytes(r HTTPRequest) []byte {
	b, err := json.Marshal(r)
	if err != nil {
		log.Fatal(err)
	}
	return b
}

func BytesToHttpRequest(b []byte) HTTPRequest {
	rv := HTTPRequest{}
	json.Unmarshal(b, &rv)
	return rv
}

func HttpResponseToBytes(r HTTPResponse) []byte {
	b, err := json.Marshal(r)
	if err != nil {
		log.Fatal(err)
	}
	return b
}

func BytesToHttpResponse(b []byte) HTTPResponse {
	rv := HTTPResponse{}
	json.Unmarshal(b, &rv)
	return rv
}