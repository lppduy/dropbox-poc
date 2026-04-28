package hub

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	UserID  string
	FileIDs map[string]bool
	Conn    *websocket.Conn
	Send    chan []byte
	Done    chan struct{}
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*Client),
	}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c.UserID] = c
}

func (h *Hub) Unregister(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.clients[userID]; ok {
		close(c.Done)
		delete(h.clients, userID)
	}
}

// NotifyFileChanged pushes an event to all clients watching fileID, except the uploader.
func (h *Hub) NotifyFileChanged(fileID, changedBy string, payload []byte) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	notified := 0
	for _, c := range h.clients {
		if c.UserID == changedBy {
			continue
		}
		if c.FileIDs[fileID] || c.FileIDs["*"] {
			select {
			case c.Send <- payload:
				notified++
			default:
				// channel full, drop
			}
		}
	}
	return notified
}

// NotifyUser pushes a direct event to a specific user by ID (regardless of watch list).
func (h *Hub) NotifyUser(userID string, payload []byte) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.clients[userID]
	if !ok {
		return false
	}
	select {
	case c.Send <- payload:
		return true
	default:
		return false
	}
}

func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
