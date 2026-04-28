package domain

type Chunk struct {
	Hash       string `gorm:"primaryKey"`
	Size       int64  `gorm:"not null"`
	StorageKey string `gorm:"not null"`
}
