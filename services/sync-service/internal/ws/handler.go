package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/lppduy/dropbox-poc/services/sync-service/internal/hub"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type watchMsg struct {
	Action  string   `json:"action"`  // "watch" | "unwatch"
	FileIDs []string `json:"fileIds"` // "*" to watch all files
}

type Handler struct {
	hub *hub.Hub
}

func NewHandler(h *hub.Hub) *Handler {
	return &Handler{hub: h}
}

// HandleWS upgrades the connection and manages a client session.
// Query param: ?userId=<id>
func (h *Handler) HandleWS(c *gin.Context) {
	userID := c.Query("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId is required"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: upgrade: %v", err)
		return
	}

	cl := &hub.Client{
		UserID:  userID,
		FileIDs: make(map[string]bool),
		Conn:    conn,
		Send:    make(chan []byte, 256),
		Done:    make(chan struct{}),
	}
	h.hub.Register(cl)
	defer func() {
		h.hub.Unregister(userID)
		conn.Close()
	}()

	go h.writePump(cl)
	h.readPump(cl)
}

func (h *Handler) writePump(cl *hub.Client) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-cl.Send:
			cl.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				cl.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := cl.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			cl.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := cl.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-cl.Done:
			return
		}
	}
}

func (h *Handler) readPump(cl *hub.Client) {
	cl.Conn.SetReadDeadline(time.Now().Add(pongWait))
	cl.Conn.SetPongHandler(func(string) error {
		cl.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, msg, err := cl.Conn.ReadMessage()
		if err != nil {
			break
		}
		cl.Conn.SetReadDeadline(time.Now().Add(pongWait))

		var watch watchMsg
		if err := json.Unmarshal(msg, &watch); err != nil {
			continue
		}
		for _, fid := range watch.FileIDs {
			if watch.Action == "watch" {
				cl.FileIDs[fid] = true
			} else {
				delete(cl.FileIDs, fid)
			}
		}
		log.Printf("ws: user=%s action=%s files=%v", cl.UserID, watch.Action, watch.FileIDs)
	}
}

// NotifyHandler is an internal HTTP endpoint called by file-service after upload completes.
func (h *Handler) NotifyHandler(c *gin.Context) {
	var req struct {
		FileID      string `json:"fileId"`
		Version     int    `json:"version"`
		ChangedBy   string `json:"changedBy"`
		Conflict    bool   `json:"conflict"`
		LoserUserID string `json:"loserUserId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.FileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fileId, version, changedBy required"})
		return
	}

	// Broadcast file_changed to all watchers (except the uploader)
	payload, _ := json.Marshal(gin.H{
		"event":     "file_changed",
		"fileId":    req.FileID,
		"version":   req.Version,
		"changedBy": req.ChangedBy,
	})
	notified := h.hub.NotifyFileChanged(req.FileID, req.ChangedBy, payload)

	// If conflict detected, also send upload_conflict to the loser
	if req.Conflict && req.LoserUserID != "" {
		conflictPayload, _ := json.Marshal(gin.H{
			"event":     "upload_conflict",
			"fileId":    req.FileID,
			"version":   req.Version,
			"changedBy": req.ChangedBy,
			"message":   "your version was overwritten (last-write-wins)",
		})
		h.hub.NotifyUser(req.LoserUserID, conflictPayload)
	}

	c.JSON(http.StatusOK, gin.H{"notified": notified, "conflict": req.Conflict})
}
