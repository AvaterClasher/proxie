package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type NodeInfo struct {
	Host     string
	Port     int
	Url      string
	IsLeader bool
	ID       uint32
}

type ProxyNode struct {
	Config               *ProxyConfig
	Info                 *NodeInfo
	PeerInfo             []*NodeInfo
	Messenger            *TCPMessenger
	Responses            *LocalCache
	Lock                 *sync.Mutex
	CV                   *sync.Cond
	CurrentForwardingIdx int
	LeaderUrl            string
}

func CreateProxyNode(host string, port int, leader bool, config_path string) *ProxyNode {
	rv := &ProxyNode{}
	rv.Config = LoadProxyConfig(config_path)

	rv.Info = CreateNodeInfo(host, port, leader)
	rv.Messenger = InitTCPMessenger(rv.Info.Url)
	rv.Responses = CreateLocalCache()

	rv.Lock = &sync.Mutex{}
	rv.CV = sync.NewCond(rv.Lock)

	rv.CurrentForwardingIdx = 0
	rv.LeaderUrl = ""
	go rv.StartBackgroundChecker()
	return rv
}

func CreateNodeInfo(host string, port int, leader bool) *NodeInfo {
	rv := &NodeInfo{}
	rv.Host = host
	rv.Port = port
	rv.Url = fmt.Sprintf("%s:%d", host, port)
	rv.IsLeader = leader
	rv.ID = HashBytes([]byte(rv.Url))
	return rv
}

func (p *ProxyNode) HandleHttpRequest(w http.ResponseWriter, r *http.Request) {

	if p.Config.SiteIsBlocked(r.Host) {
		log.Println("Blocked site!")
		fmt.Fprintf(w, "Site is blocked!\n")
		return
	}

	req := HTTPRequest{
		Method:        r.Method,
		RequestUrl:    fmt.Sprintf("%s%s", r.Host, r.URL.Path),
		Header:        r.Header,
		ContentLength: r.ContentLength,
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		log.Panic(err)
	}
	req.Body = b

	p.Lock.Lock()
	cached := p.ContainsResponse(req.RequestUrl)
	p.Lock.Unlock()
	if cached {
		println("cached!")
		res := p.Responses.CacheGet(req.RequestUrl)
		for key, slice := range res.Header {
			for _, val := range slice {
				w.Header().Add(key, val)
			}
		}
		_, err = io.Copy(w, bytes.NewReader(res.Body))
		if err != nil {
			log.Panic(err)
		}
		return
	}

	req_bytes := HttpRequestToBytes(req)

	msg := CreateMessage(req_bytes, p.Info.Url, HTTP_REQUEST_MESSAGE)

	succeeded := false
	for i := 0; i < len(p.PeerInfo); i++ {
		p.CurrentForwardingIdx = (p.CurrentForwardingIdx + 1) % len(p.PeerInfo)
		if p.Unicast(MessageToBytes(msg), p.PeerInfo[p.CurrentForwardingIdx].Url) {
			succeeded = true
			break
		}
	}
	if !succeeded {
		println("failed!")
	}

	p.Lock.Lock()
	for !p.ContainsResponse(req.RequestUrl) {
		p.CV.Wait()
	}
	res := p.Responses.CacheGet(req.RequestUrl)
	p.Lock.Unlock()

	for key, slice := range res.Header {
		for _, val := range slice {
			w.Header().Add(key, val)
		}
	}
	_, err = io.Copy(w, bytes.NewReader(res.Body))
	if err != nil {
		log.Panic(err)
	}
}

func (p *ProxyNode) HandleRequests() {
	if p.Info.IsLeader {
		http.HandleFunc("/", p.HandleHttpRequest)
		go func() {
			log.Fatal(http.ListenAndServe(p.Config.PublicUrl, nil))
		}()
	}

	l := p.Messenger.Listener
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		var buf bytes.Buffer
		io.Copy(&buf, conn)
		b := buf.Bytes()
		go p.HandleRequest(b)
		conn.Close()
	}
}

func (p *ProxyNode) HandleRequest(b []byte) {
	message := BytesToMessage(b)
	message_hash := HashBytes(b)

	p.Lock.Lock()
	p.Messenger.PruneStoredMessages()
	message_found := p.Messenger.HasMessageStored(message_hash)
	p.Lock.Unlock()

	if !message_found && message.SenderUrl != p.Info.Url {
		p.Lock.Lock()
		p.Messenger.RecentMessageHashes[message_hash] = message.Timestamp
		p.Lock.Unlock()

		if message.MessageType == MULTICAST_MESSAGE {
			p.Multicast(b)
		} else if message.MessageType == JOIN_REQUEST_MESSAGE {
			m := string(message.Data)
			tokens := strings.Split(m, ":")
			port, _ := strconv.Atoi(tokens[1])
			new_node_info := CreateNodeInfo(tokens[0], port, false)

			if !p.ContainsUrl(new_node_info.Url) {
				log.Printf("%s has joined!", new_node_info.Url)
				p.PeerInfo = append(p.PeerInfo, new_node_info)
			}

			msg := p.ConstructNodeJoinedMessage()
			p.Multicast(MessageToBytes(msg))

		} else if message.MessageType == JOIN_NOTIFY_MESSAGE {
			p.Multicast(b)
			peer_infos := strings.Split(string(message.Data), " ")
			for _, info := range peer_infos {
				tokens := strings.Split(info, ":")
				port, _ := strconv.Atoi(tokens[1])
				url := fmt.Sprintf("%s:%d", tokens[0], port)

				if url == p.Info.Url {
					continue
				}
				if !p.ContainsUrl(url) {
					new_node_info := CreateNodeInfo(tokens[0], port, false)
					p.PeerInfo = append(p.PeerInfo, new_node_info)
					log.Printf("%s has joined!", new_node_info.Url)
				}
			}

			if p.LeaderUrl == "" {
				p.LeaderUrl = message.SenderUrl
			}

		} else if message.MessageType == LEAVE_NOTIFY_MESSAGE {
			url_to_remove := string(message.Data)
			log.Printf("Node has died with URL %s!", url_to_remove)
			p.RemoveNodeFromPeers(url_to_remove)
			p.Multicast(b)
		} else if message.MessageType == HTTP_REQUEST_MESSAGE {
			r := BytesToHttpRequest(message.Data)

			request_path := fmt.Sprintf("http://%s", r.RequestUrl)
			new_request, err := http.NewRequest(r.Method, request_path, bytes.NewReader(r.Body))

			log.Printf("Sending %s request to %s\n", r.Method, request_path)
			client := &http.Client{}
			res, err := client.Do(new_request)
			if err != nil {
				log.Panic(err)
			}
			defer res.Body.Close()

			body_bytes, err := io.ReadAll(res.Body)
			if err != nil {
				log.Panic(err)
			}

			res_to_send := HTTPResponse{
				Status:        res.Status,
				RequestUrl:    r.RequestUrl,
				Header:        res.Header,
				Body:          body_bytes,
				ContentLength: res.ContentLength,
			}

			bytes_to_send := HttpResponseToBytes(res_to_send)
			msg := CreateMessage(bytes_to_send, p.Info.Url, HTTP_RESPONSE_MESSAGE)
			p.Unicast(MessageToBytes(msg), p.LeaderUrl)
		} else if message.MessageType == HTTP_RESPONSE_MESSAGE {
			res := BytesToHttpResponse(message.Data)
			p.Lock.Lock()
			p.Responses.CacheSet(res.RequestUrl, res, p.Config.CacheTimeout)
			p.Lock.Unlock()
			p.CV.Broadcast()
		} else if message.MessageType == ELECTION_MESSAGE {
			p.StartLeaderElection()
		} else if message.MessageType == VICTORY_MESSAGE {
			p.LeaderUrl = message.SenderUrl
			p.Unicast(MessageToBytes(p.ConstructAnswerMessage()), p.LeaderUrl)
			log.Printf("%s is the new leader!\n", p.LeaderUrl)
		} else if message.MessageType == ANSWER_MESSAGE {

			p.Lock.Lock()
			if !p.Info.IsLeader {
				http.HandleFunc("/", p.HandleHttpRequest)
				go func() {
					log.Fatal(http.ListenAndServe(p.Config.PublicUrl, nil))
				}()
				p.Info.IsLeader = true
				log.Println("Current Node is now the leader!")
			}
			p.Lock.Unlock()
		} else if message.MessageType == UNICAST_MESSAGE {
			println(string(message.Data))
		}
	}
}

func (p *ProxyNode) Unicast(message []byte, url string) bool {
	conn, err := net.Dial("tcp", url)
	if err != nil {
		p.RemoveNodeFromPeers(url)
		log.Printf("Node has died with URL %s!", url)

		msg := p.ConstructNodeLeftMessage(url)
		p.Multicast(MessageToBytes(msg))
		return false
	}

	defer conn.Close()

	_, err = conn.Write(message)
	if err != nil {
		log.Panic(err)
	}
	return true
}

func (p *ProxyNode) Multicast(message []byte) {
	succeeded := false
	for !succeeded {
		good := true
		for _, info := range p.PeerInfo {
			url := info.Url
			if !p.Unicast(message, url) {
				good = false
				break
			}
		}
		succeeded = good
	}
}

func (p *ProxyNode) ConstructNodeJoinedMessage() Message {
	rv := p.Info.Url
	for _, info := range p.PeerInfo {
		rv += " "
		rv += info.Url
	}
	msg := CreateMessage([]byte(rv), p.Info.Url, JOIN_NOTIFY_MESSAGE)
	return msg
}

func (p *ProxyNode) ConstructNodeLeftMessage(url string) Message {
	msg := CreateMessage([]byte(url), p.Info.Url, LEAVE_NOTIFY_MESSAGE)
	return msg
}

func (p *ProxyNode) ContainsUrl(url string) bool {
	for _, info := range p.PeerInfo {
		if url == info.Url {
			return true
		}
	}
	return false
}

func (p *ProxyNode) IndexFromString(url string) int {
	for i, info := range p.PeerInfo {
		if info.Url == url {
			return i
		}
	}
	return -1
}

func (p *ProxyNode) RemoveNodeFromPeers(url string) {
	idx := p.IndexFromString(url)
	if idx == -1 {
		return
	}
	p.PeerInfo[idx] = p.PeerInfo[len(p.PeerInfo)-1]
	p.PeerInfo[len(p.PeerInfo)-1] = nil
	p.PeerInfo = p.PeerInfo[:len(p.PeerInfo)-1]
}

func (p *ProxyNode) ContainsResponse(url string) bool {
	res := p.Responses.CacheGet(url)
	if res != nil {
		return true
	}
	return false
}

func (p *ProxyNode) StartBackgroundChecker() {
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case t := <-ticker.C:
			p.Multicast([]byte(t.String()))

			if !p.Info.IsLeader && p.LeaderUrl != "" {
				conn, err := net.Dial("tcp", p.LeaderUrl)
				if err != nil {
					p.StartLeaderElection()
				} else {
					conn.Close()
				}
			}
		}
	}
}
func (p *ProxyNode) ConstructAnswerMessage() Message {
	rv := ""
	msg := CreateMessage([]byte(rv), p.Info.Url, ANSWER_MESSAGE)
	return msg
}

func (p *ProxyNode) ConstructElectionMessage() Message {
	rv := ""
	msg := CreateMessage([]byte(rv), p.Info.Url, ELECTION_MESSAGE)
	return msg
}

func (p *ProxyNode) ConstructVictoryMessage() Message {
	rv := ""
	msg := CreateMessage([]byte(rv), p.Info.Url, VICTORY_MESSAGE)
	return msg
}

func (p *ProxyNode) StartLeaderElection() {
	highest := true

	for _, elem := range p.PeerInfo {
		if elem.ID > p.Info.ID {
			highest = false
			p.Unicast(MessageToBytes(p.ConstructElectionMessage()), elem.Url)
		}
	}
	if highest {
		p.Multicast(MessageToBytes(p.ConstructVictoryMessage()))
	}
}

