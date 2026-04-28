# Go Patterns — PoC Reference

Patterns học được từ ecom-poc và dropbox-poc. Dùng lại cho mọi project Go tiếp theo.

---

## 1. Interface → Implementation

Định nghĩa contract bằng interface, implement bằng struct riêng biệt.

```go
// service/file_service.go — contract
type FileService interface {
    InitUpload(ctx context.Context, req UploadInitRequest) (UploadInitResponse, error)
    StoreChunk(ctx context.Context, hash string, data []byte) error
}

// service/file_service_impl.go — implementation
type fileServiceImpl struct {   // lowercase = unexported
    chunkRepo repository.ChunkRepository
    fileRepo  repository.FileRepository
    minio     *storage.MinioClient
}

func NewFileService(chunkRepo ..., fileRepo ..., minio ...) FileService {
    return &fileServiceImpl{...}  // trả về interface, không phải concrete type
}
```

**Tại sao:** Controller chỉ biết interface → có thể mock trong test, swap implementation.

---

## 2. Repository Interface + GORM Impl

Tách layer database khỏi business logic.

```go
// repository/chunk_repository.go — interface
type ChunkRepository interface {
    FindExistingHashes(ctx context.Context, hashes []string) ([]string, error)
    Save(ctx context.Context, chunk domain.Chunk) error
    FindByHash(ctx context.Context, hash string) (domain.Chunk, bool, error)
}

// repository/chunk_repository_gorm.go — GORM implementation
type GormChunkRepository struct {
    db *gorm.DB
}

func NewChunkRepository(db *gorm.DB) *GormChunkRepository {
    return &GormChunkRepository{db: db}
}
```

---

## 3. (value, bool, error) Return Pattern

Khi record có thể không tồn tại — trả về `bool` thay vì dùng sentinel error.

```go
// bool = found/existed, không cần check ErrNotFound
func (r *GormFileRepository) FindFileByID(ctx context.Context, id string) (domain.File, bool, error) {
    var f domain.File
    err := r.db.WithContext(ctx).Where("id = ?", id).First(&f).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return domain.File{}, false, nil  // không phải error, chỉ là không có
    }
    return f, err == nil, err
}

// Call site rõ ràng:
file, found, err := repo.FindFileByID(ctx, id)
if err != nil { ... }
if !found { httpx.NotFound(...); return }
```

---

## 4. Domain Errors + errors.Is

Định nghĩa sentinel errors trong domain package, map ra HTTP status ở controller.

```go
// domain/errors.go
var (
    ErrFileNotFound  = errors.New("file not found")
    ErrHashMismatch  = errors.New("chunk hash mismatch")
    ErrEmptyCart     = errors.New("cart is empty")          // ecom-poc
    ErrOrderNotFound = errors.New("order not found")        // ecom-poc
)

// controller — map domain error → HTTP status
if err := c.svc.StoreChunk(ctx, hash, data); err != nil {
    if errors.Is(err, domain.ErrHashMismatch) {
        httpx.BadRequest(ctx, "chunk hash mismatch")
        return
    }
    httpx.InternalError(ctx, "failed to store chunk")
    return
}
```

**Rule:** Không dùng string comparison (`err.Error() == "..."`) — dùng `errors.Is`.

---

## 5. Transaction với GORM

Dùng `db.Transaction` để đảm bảo atomic — nếu 1 bước fail, rollback toàn bộ.

```go
func (r *GormFileRepository) SaveVersion(ctx context.Context, version domain.FileVersion, chunks []domain.VersionChunk) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Create(&version).Error; err != nil {
            return err  // auto rollback
        }
        for i := range chunks {
            if err := tx.Create(&chunks[i]).Error; err != nil {
                return err  // auto rollback
            }
        }
        return nil  // commit
    })
}
```

**Note:** Dùng `tx` (transaction) bên trong, không dùng `r.db` — sai là không atomic.

---

## 6. Config Load với getEnv

```go
// config/config.go
type Config struct {
    Port           string
    DatabaseURL    string
    MinioEndpoint  string
}

func Load() Config {
    return Config{
        Port:        getEnv("PORT", "8081"),
        DatabaseURL: getEnv("DATABASE_URL", "postgres://dev:dev@localhost:5432/db?sslmode=disable"),
    }
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
```

**Rule:** Default values luôn trỏ localhost → chạy được local mà không cần set env.

---

## 7. Dependency Injection trong main.go

Wiring toàn bộ ở `main.go` — không dùng DI framework.

```go
func main() {
    cfg := config.Load()

    // infra layer
    db, err := repository.NewPostgresDB(cfg.DatabaseURL)
    if err != nil { log.Fatalf("connect postgres: %v", err) }

    minioClient, err := storage.NewMinioClient(cfg.MinioEndpoint, ...)
    if err != nil { log.Fatalf("connect minio: %v", err) }

    // repository layer
    chunkRepo := repository.NewChunkRepository(db)
    fileRepo  := repository.NewFileRepository(db)

    // service layer
    fileSvc := service.NewFileService(chunkRepo, fileRepo, minioClient)

    // external clients
    syncClient := client.NewSyncClient(cfg.SyncServiceURL)

    // controller layer
    fileCtrl := controller.NewFileController(fileSvc, syncClient)

    // router
    r := gin.Default()
    routes.Register(r, fileCtrl)
    r.Run(":" + cfg.Port)
}
```

**Pattern:** Từ dưới lên — infra → repo → service → controller → router.

---

## 8. httpx Helpers

Tạo package httpx để wrap `c.JSON` — tránh hard-code status codes.

```go
// api/httpx/response.go
func OK(c *gin.Context, data any)          { c.JSON(200, data) }
func Created(c *gin.Context, data any)     { c.JSON(201, data) }
func BadRequest(c *gin.Context, msg string){ c.JSON(400, gin.H{"error": msg}) }
func NotFound(c *gin.Context, msg string)  { c.JSON(404, gin.H{"error": msg}) }
func InternalError(c *gin.Context, msg string) { c.JSON(500, gin.H{"error": msg}) }

// Controller usage
httpx.Created(ctx, gin.H{"fileId": result.FileID})
httpx.BadRequest(ctx, "ownerId is required")
```

---

## 9. AutoMigrate tại startup

```go
// repository/postgres.go
func NewPostgresDB(dsn string) (*gorm.DB, error) {
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil { return nil, err }

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
```

**Note:** Chỉ dùng AutoMigrate cho PoC. Production dùng migration files (goose, migrate).

---

## 10. Fire-and-forget Goroutine

Khi cần notify external service mà không muốn block response.

```go
// controller — upload complete xong, notify sync trong background
version, err := c.svc.CompleteUpload(ctx, ...)
// ...

go func() {
    if err := c.syncClient.Notify(context.Background(), client.NotifyRequest{
        FileID:    fileID,
        Version:   version,
        ChangedBy: req.OwnerID,
    }); err != nil {
        log.Printf("sync notify: %v", err)  // log nhưng không fail request
    }
}()

httpx.OK(ctx, gin.H{"fileId": fileID, "version": version})
```

**Tại sao:** Client không cần chờ notification gửi xong. Nếu sync-service down thì upload vẫn thành công.

---

## 11. WebSocket Hub — Goroutine + Channel + Mutex

Pattern cho real-time notification đến nhiều clients.

```go
type Client struct {
    UserID  string
    FileIDs map[string]bool
    Conn    *websocket.Conn
    Send    chan []byte   // buffered channel — không block khi send
    Done    chan struct{} // close để signal goroutine dừng
}

type Hub struct {
    mu      sync.RWMutex        // RWMutex: nhiều reader, 1 writer
    clients map[string]*Client
}

// Ghi: exclusive lock
func (h *Hub) Register(c *Client) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.clients[c.UserID] = c
}

// Đọc: shared lock — nhiều goroutine đọc cùng lúc
func (h *Hub) NotifyFileChanged(fileID, changedBy string, payload []byte) int {
    h.mu.RLock()
    defer h.mu.RUnlock()
    for _, c := range h.clients {
        select {
        case c.Send <- payload:  // non-blocking send
        default:                 // full buffer → drop, đừng block
        }
    }
}
```

**Rule:** `select { case ch <- v: default: }` = non-blocking send. Tránh deadlock khi buffer đầy.

---

## 12. Background Worker với ticker + ctx.Done()

Pattern cho periodic tasks (outbox relay, cleanup jobs...).

```go
// ecom-poc: outbox relay
func StartRelay(ctx context.Context, outbox repository.OutboxRepository, pub Publisher) {
    go func() {
        ticker := time.NewTicker(3 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():   // graceful shutdown
                return
            case <-ticker.C:    // mỗi 3s
                if err := runOnce(ctx, outbox, pub); err != nil {
                    log.Printf("relay error: %v", err)
                }
            }
        }
    }()
}
```

**Tại sao `select`:** Không thể `for { tick(); work() }` vì sẽ bị block mãi — không bắt được ctx.Done().

---

## 13. content-addressable Storage

Dùng hash của nội dung làm key — tự động dedup.

```go
// Chunk PK = SHA-256 hash của data
type Chunk struct {
    Hash       string `gorm:"primaryKey"`  // hash = identity
    StorageKey string                       // "chunks/<hash>" trong MinIO
}

// Service: verify + store
func (s *fileServiceImpl) StoreChunk(ctx context.Context, hash string, data []byte) error {
    computed := sha256.Sum256(data)
    if hex.EncodeToString(computed[:]) != hash {
        return domain.ErrHashMismatch  // client gửi sai hash
    }
    storageKey := "chunks/" + hash
    s.minio.Put(ctx, storageKey, data)
    s.chunkRepo.Save(ctx, domain.Chunk{Hash: hash, StorageKey: storageKey})
}

// Dedup: FindExistingHashes → client chỉ upload những gì server chưa có
existing, _ := s.chunkRepo.FindExistingHashes(ctx, req.ChunkHashes)
// → WHERE hash IN ('aaa','bbb','ccc') → trả về những hash đã có
```

**Ứng dụng tương tự:** Git lưu blob/tree/commit bằng SHA. ZFS/Btrfs dùng cho block dedup.

---

## 14. Set-Diff Algorithm

Tìm added/removed giữa 2 tập hợp — dùng cho delta sync.

```go
func (s *fileServiceImpl) GetSyncDiff(...) (added, removed []string, ...) {
    clientSet  := toSet(clientChunks)   // version client đang có
    currentSet := toSet(currentChunks)  // version server hiện tại

    for h := range currentSet {
        if !clientSet[h] { added = append(added, h) }   // mới ở server
    }
    for h := range clientSet {
        if !currentSet[h] { removed = append(removed, h) } // bị xóa ở server
    }
}

func toSet(items []string) map[string]bool {
    s := make(map[string]bool, len(items))
    for _, v := range items { s[v] = true }
    return s
}
```

**Pattern:** `map[string]bool` làm Set trong Go. Lookup O(1).

---

## 15. Layered Error Wrapping

```go
// Wrap với context để dễ trace:
if err := pub.Publish(ctx, topic, eventType, payload); err != nil {
    return fmt.Errorf("publish event %d: %w", e.ID, err)
}

// Unwrap ở nơi khác:
if errors.Is(err, someSpecificError) { ... }
```

**Rule:** `%w` để wrap (có thể unwrap), `%v` chỉ format string (không unwrap được).

---

## Summary — Checklist cho project Go mới

```
□ domain/    → structs + sentinel errors (không import gì từ infra)
□ repository/→ interface + gorm impl + NewPostgresDB (AutoMigrate)
□ service/   → interface + impl (inject repos + clients)
□ client/    → HTTP client gọi service khác
□ api/
│  ├── httpx/     → OK/Created/BadRequest/NotFound/InternalError
│  ├── controller/→ parse request → call service → map error → return
│  └── routes/    → Register(r *gin.Engine, ctrl ...)
□ config/    → Load() + getEnv(key, fallback)
□ cmd/api/main.go → wire everything từ dưới lên
```
