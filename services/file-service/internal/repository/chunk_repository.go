package repository

import (
	"context"

	"github.com/lppduy/dropbox-poc/services/file-service/internal/domain"
)

type ChunkRepository interface {
	FindExistingHashes(ctx context.Context, hashes []string) ([]string, error)
	Save(ctx context.Context, chunk domain.Chunk) error
	FindByHash(ctx context.Context, hash string) (domain.Chunk, bool, error)
}
