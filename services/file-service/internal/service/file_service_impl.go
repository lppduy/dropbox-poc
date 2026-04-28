package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/domain"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/repository"
	"github.com/lppduy/dropbox-poc/services/file-service/internal/storage"
)

type fileServiceImpl struct {
	chunkRepo repository.ChunkRepository
	fileRepo  repository.FileRepository
	minio     *storage.MinioClient
}

func NewFileService(
	chunkRepo repository.ChunkRepository,
	fileRepo repository.FileRepository,
	minio *storage.MinioClient,
) FileService {
	return &fileServiceImpl{
		chunkRepo: chunkRepo,
		fileRepo:  fileRepo,
		minio:     minio,
	}
}

func (s *fileServiceImpl) InitUpload(ctx context.Context, req UploadInitRequest) (UploadInitResponse, error) {
	existing, err := s.chunkRepo.FindExistingHashes(ctx, req.ChunkHashes)
	if err != nil {
		return UploadInitResponse{}, err
	}

	existingSet := make(map[string]bool, len(existing))
	for _, h := range existing {
		existingSet[h] = true
	}

	var missing []string
	for _, h := range req.ChunkHashes {
		if !existingSet[h] {
			missing = append(missing, h)
		}
	}
	if missing == nil {
		missing = []string{}
	}

	fileID := uuid.New().String()
	if err := s.fileRepo.SaveFile(ctx, domain.File{
		ID:             fileID,
		OwnerID:        req.OwnerID,
		Name:           req.Filename,
		CurrentVersion: 0,
	}); err != nil {
		return UploadInitResponse{}, err
	}

	return UploadInitResponse{
		FileID:        fileID,
		MissingChunks: missing,
	}, nil
}

func (s *fileServiceImpl) StoreChunk(ctx context.Context, hash string, data []byte) error {
	computed := sha256.Sum256(data)
	if hex.EncodeToString(computed[:]) != hash {
		return domain.ErrHashMismatch
	}

	storageKey := "chunks/" + hash
	if err := s.minio.Put(ctx, storageKey, data); err != nil {
		return err
	}

	return s.chunkRepo.Save(ctx, domain.Chunk{
		Hash:       hash,
		Size:       int64(len(data)),
		StorageKey: storageKey,
	})
}

func (s *fileServiceImpl) CompleteUpload(ctx context.Context, fileID string, orderedHashes []string, ownerID string, baseVersion int) (CompleteUploadResult, error) {
	currentVersion, err := s.fileRepo.GetCurrentVersion(ctx, fileID)
	if err != nil {
		return CompleteUploadResult{}, err
	}

	// Detect conflict: client based on an older version that has since been updated
	var loserUserID string
	conflict := baseVersion > 0 && baseVersion < currentVersion
	if conflict {
		loserUserID, _ = s.fileRepo.GetVersionCreator(ctx, fileID, currentVersion)
	}

	newVersion := currentVersion + 1
	versionID := uuid.New().String()
	fv := domain.FileVersion{
		ID:        versionID,
		FileID:    fileID,
		Version:   newVersion,
		CreatedBy: ownerID,
	}

	chunks := make([]domain.VersionChunk, len(orderedHashes))
	for i, h := range orderedHashes {
		chunks[i] = domain.VersionChunk{
			VersionID:  versionID,
			ChunkIndex: i,
			ChunkHash:  h,
		}
	}

	if err := s.fileRepo.SaveVersion(ctx, fv, chunks); err != nil {
		return CompleteUploadResult{}, err
	}
	if err := s.fileRepo.UpdateCurrentVersion(ctx, fileID, newVersion); err != nil {
		return CompleteUploadResult{}, err
	}

	return CompleteUploadResult{
		Version:     newVersion,
		Conflict:    conflict,
		LoserUserID: loserUserID,
	}, nil
}

func (s *fileServiceImpl) GetManifest(ctx context.Context, fileID string) ([]string, error) {
	return s.fileRepo.GetCurrentChunks(ctx, fileID)
}

func (s *fileServiceImpl) GetChunk(ctx context.Context, hash string) ([]byte, error) {
	chunk, found, err := s.chunkRepo.FindByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, domain.ErrChunkNotFound
	}
	return s.minio.Get(ctx, chunk.StorageKey)
}

func (s *fileServiceImpl) GetSyncDiff(ctx context.Context, fileID string, clientVersion int) (added, removed []string, currentVersion int, err error) {
	currentVersion, err = s.fileRepo.GetCurrentVersion(ctx, fileID)
	if err != nil {
		return nil, nil, 0, err
	}
	if clientVersion == currentVersion {
		return []string{}, []string{}, currentVersion, nil
	}

	clientChunks, err := s.fileRepo.GetChunksByVersion(ctx, fileID, clientVersion)
	if err != nil {
		return nil, nil, 0, err
	}
	currentChunks, err := s.fileRepo.GetCurrentChunks(ctx, fileID)
	if err != nil {
		return nil, nil, 0, err
	}

	clientSet := toSet(clientChunks)
	currentSet := toSet(currentChunks)

	for h := range currentSet {
		if !clientSet[h] {
			added = append(added, h)
		}
	}
	for h := range clientSet {
		if !currentSet[h] {
			removed = append(removed, h)
		}
	}
	if added == nil {
		added = []string{}
	}
	if removed == nil {
		removed = []string{}
	}
	return added, removed, currentVersion, nil
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, v := range items {
		s[v] = true
	}
	return s
}
