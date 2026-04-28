package repository

import (
	"github.com/lppduy/dropbox-poc/services/file-service/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func NewPostgresDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&domain.File{},
		&domain.FileVersion{},
		&domain.VersionChunk{},
		&domain.Chunk{},
	); err != nil {
		return nil, err
	}
	return db, nil
}
