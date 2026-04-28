package controller

import (
	"errors"
	"io"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/api/response"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/client"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/domain"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/service"
)

type FileController struct {
	svc        service.FileService
	syncClient *client.SyncClient
}

func NewFileController(svc service.FileService, syncClient *client.SyncClient) *FileController {
	return &FileController{svc: svc, syncClient: syncClient}
}

func (c *FileController) InitUpload(ctx *gin.Context) {
	var req struct {
		OwnerID     string   `json:"ownerId"`
		Filename    string   `json:"filename"`
		ChunkHashes []string `json:"chunkHashes"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.OwnerID == "" || req.Filename == "" {
		response.BadRequest(ctx, "ownerId, filename, and chunkHashes are required")
		return
	}

	result, err := c.svc.InitUpload(ctx.Request.Context(), service.UploadInitRequest{
		OwnerID:     req.OwnerID,
		Filename:    req.Filename,
		ChunkHashes: req.ChunkHashes,
	})
	if err != nil {
		response.InternalError(ctx, "failed to init upload")
		return
	}

	response.Created(ctx, gin.H{
		"fileId":        result.FileID,
		"missingChunks": result.MissingChunks,
	})
}

func (c *FileController) UploadChunk(ctx *gin.Context) {
	hash := ctx.Param("hash")
	if hash == "" {
		response.BadRequest(ctx, "hash is required")
		return
	}

	data, err := io.ReadAll(ctx.Request.Body)
	if err != nil || len(data) == 0 {
		response.BadRequest(ctx, "empty chunk data")
		return
	}

	if err := c.svc.StoreChunk(ctx.Request.Context(), hash, data); err != nil {
		if errors.Is(err, domain.ErrHashMismatch) {
			response.BadRequest(ctx, "chunk hash mismatch")
			return
		}
		response.InternalError(ctx, "failed to store chunk")
		return
	}

	response.OK(ctx, gin.H{"stored": hash})
}

func (c *FileController) CompleteUpload(ctx *gin.Context) {
	fileID := ctx.Param("id")
	var req struct {
		OwnerID       string   `json:"ownerId"`
		OrderedHashes []string `json:"orderedHashes"`
		BaseVersion   int      `json:"baseVersion"` // 0 means first upload (no conflict detection)
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.OwnerID == "" || len(req.OrderedHashes) == 0 {
		response.BadRequest(ctx, "ownerId and orderedHashes are required")
		return
	}

	result, err := c.svc.CompleteUpload(ctx.Request.Context(), fileID, req.OrderedHashes, req.OwnerID, req.BaseVersion)
	if err != nil {
		response.InternalError(ctx, "failed to complete upload")
		return
	}

	go func() {
		if err := c.syncClient.Notify(ctx.Request.Context(), client.NotifyRequest{
			FileID:      fileID,
			Version:     result.Version,
			ChangedBy:   req.OwnerID,
			Conflict:    result.Conflict,
			LoserUserID: result.LoserUserID,
		}); err != nil {
			log.Printf("sync notify: %v", err)
		}
	}()

	response.OK(ctx, gin.H{
		"fileId":   fileID,
		"version":  result.Version,
		"conflict": result.Conflict,
	})
}

func (c *FileController) GetManifest(ctx *gin.Context) {
	fileID := ctx.Param("id")
	chunks, err := c.svc.GetManifest(ctx.Request.Context(), fileID)
	if err != nil {
		response.InternalError(ctx, "failed to get manifest")
		return
	}

	response.OK(ctx, gin.H{"fileId": fileID, "chunks": chunks})
}

func (c *FileController) DownloadChunk(ctx *gin.Context) {
	hash := ctx.Param("hash")
	data, err := c.svc.GetChunk(ctx.Request.Context(), hash)
	if err != nil {
		if errors.Is(err, domain.ErrChunkNotFound) {
			response.NotFound(ctx, "chunk not found")
			return
		}
		response.InternalError(ctx, "failed to get chunk")
		return
	}

	ctx.Data(200, "application/octet-stream", data)
}

func (c *FileController) SyncDiff(ctx *gin.Context) {
	fileID := ctx.Param("id")
	var req struct {
		ClientVersion int `json:"clientVersion"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.BadRequest(ctx, "clientVersion is required")
		return
	}

	added, removed, current, err := c.svc.GetSyncDiff(ctx.Request.Context(), fileID, req.ClientVersion)
	if err != nil {
		response.InternalError(ctx, "sync diff failed")
		return
	}

	response.OK(ctx, gin.H{
		"currentVersion": current,
		"needDownload":   added,
		"needDelete":     removed,
	})
}
