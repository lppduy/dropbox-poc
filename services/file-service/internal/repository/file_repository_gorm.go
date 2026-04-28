package repository

import (
	"context"
	"errors"

	"github.com/lppduy/dropbox-poc/services/file-service/internal/domain"
	"gorm.io/gorm"
)

type GormFileRepository struct {
	db *gorm.DB
}

func NewFileRepository(db *gorm.DB) *GormFileRepository {
	return &GormFileRepository{db: db}
}

func (r *GormFileRepository) SaveFile(ctx context.Context, file domain.File) error {
	return r.db.WithContext(ctx).Create(&file).Error
}

func (r *GormFileRepository) FindFileByID(ctx context.Context, id string) (domain.File, bool, error) {
	var f domain.File
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.File{}, false, nil
	}
	return f, err == nil, err
}

func (r *GormFileRepository) SaveVersion(ctx context.Context, version domain.FileVersion, chunks []domain.VersionChunk) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		for i := range chunks {
			if err := tx.Create(&chunks[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormFileRepository) GetChunksByVersion(ctx context.Context, fileID string, version int) ([]string, error) {
	var versionID string
	err := r.db.WithContext(ctx).
		Model(&domain.FileVersion{}).
		Where("file_id = ? AND version = ?", fileID, version).
		Pluck("id", &versionID).Error
	if err != nil || versionID == "" {
		return nil, err
	}

	var hashes []string
	err = r.db.WithContext(ctx).
		Model(&domain.VersionChunk{}).
		Where("version_id = ?", versionID).
		Order("chunk_index ASC").
		Pluck("chunk_hash", &hashes).Error
	return hashes, err
}

func (r *GormFileRepository) GetCurrentChunks(ctx context.Context, fileID string) ([]string, error) {
	var file domain.File
	if err := r.db.WithContext(ctx).Where("id = ?", fileID).First(&file).Error; err != nil {
		return nil, err
	}
	return r.GetChunksByVersion(ctx, fileID, file.CurrentVersion)
}

func (r *GormFileRepository) GetCurrentVersion(ctx context.Context, fileID string) (int, error) {
	var file domain.File
	err := r.db.WithContext(ctx).Where("id = ?", fileID).First(&file).Error
	return file.CurrentVersion, err
}

func (r *GormFileRepository) UpdateCurrentVersion(ctx context.Context, fileID string, version int) error {
	return r.db.WithContext(ctx).
		Model(&domain.File{}).
		Where("id = ?", fileID).
		Update("current_version", version).Error
}
