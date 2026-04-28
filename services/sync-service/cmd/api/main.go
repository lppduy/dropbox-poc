package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/lppduy/dropbox-poc/services/sync-service/internal/api/routes"
	"github.com/lppduy/dropbox-poc/services/sync-service/internal/config"
	"github.com/lppduy/dropbox-poc/services/sync-service/internal/hub"
	"github.com/lppduy/dropbox-poc/services/sync-service/internal/ws"
)

func main() {
	cfg := config.Load()

	h := hub.NewHub()
	handler := ws.NewHandler(h)

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "connections": h.Count()})
	})
	routes.Register(r, handler)

	log.Printf("sync-service listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("sync-service: run: %v", err)
	}
}
