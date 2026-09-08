package socket

import (
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/asynchronomatic/speakeasy/pkg/security"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  64,
	WriteBufferSize: 64,
	CheckOrigin:     allowUpgrade,
}

func allowUpgrade(r *http.Request) bool {
	site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
	if site == "cross-site" || site == "nested-cross-origin" {
		return false
	}
	return security.OriginOK(r)
}

type Notifier struct {
	lock    sync.Mutex
	clients map[*Client]bool
	action  chan *Action
}

func (n *Notifier) Broadcast() {
	select {
	case n.action <- &Action{Action: "broadcast"}:
	default:
	}
}

func (n *Notifier) Register(c *Client) {
	n.action <- &Action{Action: "register", Client: c}
}

func (n *Notifier) Deregister(c *Client) {
	n.action <- &Action{Action: "deregister", Client: c}
}

func (n *Notifier) Poll() {
	for action := range n.action {
		n.lock.Lock()
		switch action.Action {
		case "register":
			n.clients[action.Client] = true
		case "deregister":
			delete(n.clients, action.Client)
			action.Client.Close()
		case "broadcast":
			for client := range n.clients {
				client.Wakeup()
			}
		}
		n.lock.Unlock()
	}
}

func (n *Notifier) Handle(w http.ResponseWriter, r *http.Request) {
	if !allowUpgrade(r) {
		http.Error(w, "origin mismatch", http.StatusForbidden)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &Client{
		notifier: n,
		conn:     conn,
		wakeup:   make(chan bool, 8),
	}

	n.Register(client)

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}

func NewNotifier() *Notifier {
	return &Notifier{
		clients: make(map[*Client]bool),
		action:  make(chan *Action, 64),
	}
}
