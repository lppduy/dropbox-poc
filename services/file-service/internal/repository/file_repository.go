package repository

import (
	"context"

	"github.com/lppduy/dropbox-poc/services/file-service/internal/domain"
)

type FileRepository interface {
	SaveFile(ctx context.Context, file domain.File) error
	FindFileByID(ctx context.Context, id string) (domain.File, bool, error)
	SaveVersion(ctx context.Context, version domain.FileVersion, chunks []domain.VersionChunk) error
	GetChunksByVersion(ctx context.Context, fileID string, version int) ([]string, error)
	GetCurrentChunks(ctx context.Context, fileID string) ([]string, error)
	GetCurrentVersion(ctx context.Context, fileID string) (int, error)
	UpdateCurrentVersion(ctx context.Context, fileID string, version int) error
}
