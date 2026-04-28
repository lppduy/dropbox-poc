package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/api/controller"
)

func Register(r *gin.Engine, fc *controller.FileController) {
	r.POST("/upload/init", fc.InitUpload)
	r.PUT("/upload/chunk/:hash", fc.UploadChunk)
	r.POST("/files/:id/complete", fc.CompleteUpload)
	r.GET("/files/:id/manifest", fc.GetManifest)
	r.GET("/chunks/:hash", fc.DownloadChunk)
	r.POST("/files/:id/sync", fc.SyncDiff)
}
