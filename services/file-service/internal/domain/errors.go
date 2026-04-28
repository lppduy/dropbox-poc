package domain

import "errors"

var (
	ErrFileNotFound  = errors.New("file not found")
	ErrChunkNotFound = errors.New("chunk not found")
	ErrHashMismatch  = errors.New("chunk hash mismatch")
)
