package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/api/controller"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/api/routes"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/client"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/config"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/repository"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/service"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/storage"
)

func main() {
	cfg := config.Load()

	db, err := repository.NewPostgresDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("file-service: connect postgres: %v", err)
	}

	minioClient, err := storage.NewMinioClient(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey)
	if err != nil {
		log.Fatalf("file-service: connect minio: %v", err)
	}

	chunkRepo := repository.NewChunkRepository(db)
	fileRepo := repository.NewFileRepository(db)
	fileSvc := service.NewFileService(chunkRepo, fileRepo, minioClient)
	syncClient := client.NewSyncClient(cfg.SyncServiceURL)
	fileCtrl := controller.NewFileController(fileSvc, syncClient)

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	routes.Register(r, fileCtrl)

	log.Printf("file-service listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("file-service: run: %v", err)
	}
}
