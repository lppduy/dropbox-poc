package service

import "context"

type UploadInitRequest struct {
	OwnerID     string
	Filename    string
	ChunkHashes []string
}

type UploadInitResponse struct {
	FileID        string
	MissingChunks []string
}

type CompleteUploadResult struct {
	Version     int
	Conflict    bool
	LoserUserID string // user whose version was overwritten (empty if no conflict)
}

type FileService interface {
	InitUpload(ctx context.Context, req UploadInitRequest) (UploadInitResponse, error)
	StoreChunk(ctx context.Context, hash string, data []byte) error
	CompleteUpload(ctx context.Context, fileID string, orderedHashes []string, ownerID string, baseVersion int) (CompleteUploadResult, error)
	GetManifest(ctx context.Context, fileID string) ([]string, error)
	GetChunk(ctx context.Context, hash string) ([]byte, error)
	GetSyncDiff(ctx context.Context, fileID string, clientVersion int) (added, removed []string, currentVersion int, err error)
}
