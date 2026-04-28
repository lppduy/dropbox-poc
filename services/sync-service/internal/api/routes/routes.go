package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/lppduy/dropbox-poc/services/sync-service/internal/ws"
)

func Register(r *gin.Engine, h *ws.Handler) {
	r.GET("/ws", h.HandleWS)
	r.POST("/internal/notify", h.NotifyHandler)
}
