package repository

import (
	"context"
	"errors"

	"github.com/lppduy/dropbox-poc/services/file-service/internal/domain"
	"gorm.io/gorm"
)

type GormChunkRepository struct {
	db *gorm.DB
}

func NewChunkRepository(db *gorm.DB) *GormChunkRepository {
	return &GormChunkRepository{db: db}
}

func (r *GormChunkRepository) FindExistingHashes(ctx context.Context, hashes []string) ([]string, error) {
	var found []string
	err := r.db.WithContext(ctx).
		Model(&domain.Chunk{}).
		Where("hash IN ?", hashes).
		Pluck("hash", &found).Error
	return found, err
}

func (r *GormChunkRepository) Save(ctx context.Context, chunk domain.Chunk) error {
	return r.db.WithContext(ctx).
		Where(domain.Chunk{Hash: chunk.Hash}).
		FirstOrCreate(&chunk).Error
}

func (r *GormChunkRepository) FindByHash(ctx context.Context, hash string) (domain.Chunk, bool, error) {
	var c domain.Chunk
	err := r.db.WithContext(ctx).Where("hash = ?", hash).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Chunk{}, false, nil
	}
	return c, err == nil, err
}
