package domain

import "time"

type File struct {
	ID             string    `gorm:"primaryKey"`
	OwnerID        string    `gorm:"not null"`
	Name           string    `gorm:"not null"`
	CurrentVersion int       `gorm:"default:0"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

type FileVersion struct {
	ID        string    `gorm:"primaryKey"`
	FileID    string    `gorm:"index;not null"`
	Version   int       `gorm:"not null"`
	CreatedBy string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// VersionChunk maps a version to its ordered chunks.
type VersionChunk struct {
	ID         uint   `gorm:"primaryKey;autoIncrement"`
	VersionID  string `gorm:"index;not null"`
	ChunkIndex int    `gorm:"not null"`
	ChunkHash  string `gorm:"not null"`
}
