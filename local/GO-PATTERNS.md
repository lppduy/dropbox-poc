# Go Patterns — PoC Reference

Patterns learned from ecom-poc and dropbox-poc. Reuse across future Go projects.

---

## 1. Interface → Implementation

Define the contract with an interface, implement with a separate struct.

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
    return &fileServiceImpl{...}  // return interface, not concrete type
}
```

**Why:** Controller only knows the interface → easy to mock in tests, easy to swap implementations.

---

## 2. Repository Interface + GORM Impl

Decouple the database layer from business logic.

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

When a record may not exist — return `bool` instead of a sentinel error.

```go
// bool = found/existed, no need to check ErrNotFound
func (r *GormFileRepository) FindFileByID(ctx context.Context, id string) (domain.File, bool, error) {
    var f domain.File
    err := r.db.WithContext(ctx).Where("id = ?", id).First(&f).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return domain.File{}, false, nil  // not an error, record simply doesn't exist
    }
    return f, err == nil, err
}

// Call site is explicit:
file, found, err := repo.FindFileByID(ctx, id)
if err != nil { ... }
if !found { httpx.NotFound(...); return }
```

---

## 4. Domain Errors + errors.Is

Define sentinel errors in the domain package, map to HTTP status in the controller.

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

**Rule:** Never use string comparison (`err.Error() == "..."`) — always use `errors.Is`.

---

## 5. Transaction with GORM

Use `db.Transaction` to ensure atomicity — if one step fails, everything rolls back.

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

**Note:** Use `tx` (the transaction handle) inside the closure, never `r.db` — otherwise the operation is not atomic.

---

## 6. Config Loading with getEnv

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

**Rule:** Default values always point to localhost → works locally without any env vars set.

---

## 7. Dependency Injection in main.go

Wire everything in `main.go` — no DI framework needed.

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

**Pattern:** Wire bottom-up — infra → repo → service → controller → router.

---

## 8. httpx Helpers

Create an `httpx` package to wrap `c.JSON` — avoid hard-coding status codes everywhere.

```go
// api/httpx/response.go
func OK(c *gin.Context, data any)              { c.JSON(200, data) }
func Created(c *gin.Context, data any)         { c.JSON(201, data) }
func BadRequest(c *gin.Context, msg string)    { c.JSON(400, gin.H{"error": msg}) }
func NotFound(c *gin.Context, msg string)      { c.JSON(404, gin.H{"error": msg}) }
func InternalError(c *gin.Context, msg string) { c.JSON(500, gin.H{"error": msg}) }

// Controller usage
httpx.Created(ctx, gin.H{"fileId": result.FileID})
httpx.BadRequest(ctx, "ownerId is required")
```

---

## 9. AutoMigrate at Startup

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

**Note:** AutoMigrate is only for PoC. Production should use migration files (goose, migrate).

---

## 10. Fire-and-Forget Goroutine

When you need to notify an external service without blocking the response.

```go
// controller — complete upload, notify sync in the background
result, err := c.svc.CompleteUpload(ctx, ...)
// ...

go func() {
    if err := c.syncClient.Notify(context.Background(), client.NotifyRequest{
        FileID:      fileID,
        Version:     result.Version,
        ChangedBy:   req.OwnerID,
        Conflict:    result.Conflict,
        LoserUserID: result.LoserUserID,
    }); err != nil {
        log.Printf("sync notify: %v", err)  // log but don't fail the request
    }
}()

httpx.OK(ctx, gin.H{"fileId": fileID, "version": result.Version})
```

**Why:** The client doesn't need to wait for the notification to be delivered. Upload succeeds even if sync-service is down.

---

## 11. WebSocket Hub — Goroutine + Channel + Mutex

Pattern for real-time notifications to multiple concurrent clients.

```go
type Client struct {
    UserID  string
    FileIDs map[string]bool
    Conn    *websocket.Conn
    Send    chan []byte   // buffered channel — won't block on send
    Done    chan struct{} // close to signal goroutines to stop
}

type Hub struct {
    mu      sync.RWMutex        // RWMutex: many readers, one writer
    clients map[string]*Client
}

// Write: exclusive lock
func (h *Hub) Register(c *Client) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.clients[c.UserID] = c
}

// Read: shared lock — multiple goroutines can read concurrently
func (h *Hub) NotifyFileChanged(fileID, changedBy string, payload []byte) int {
    h.mu.RLock()
    defer h.mu.RUnlock()
    for _, c := range h.clients {
        select {
        case c.Send <- payload:  // non-blocking send
        default:                 // buffer full → drop, don't block
        }
    }
}

// Direct notify: target a specific user regardless of watch list
func (h *Hub) NotifyUser(userID string, payload []byte) bool {
    h.mu.RLock()
    defer h.mu.RUnlock()
    c, ok := h.clients[userID]
    if !ok { return false }
    select {
    case c.Send <- payload:
        return true
    default:
        return false
    }
}
```

**Rule:** `select { case ch <- v: default: }` = non-blocking send. Prevents deadlock when buffer is full.

**When to use Mutex vs RWMutex:**
- `sync.Mutex` — simple exclusive lock; use when reads and writes are roughly equal
- `sync.RWMutex` — use when reads heavily outnumber writes (e.g. hub notifications read the map far more than registrations write it)

---

## 12. Background Worker with Ticker + ctx.Done()

Pattern for periodic tasks (outbox relay, cleanup jobs, etc.).

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
            case <-ticker.C:    // every 3s
                if err := runOnce(ctx, outbox, pub); err != nil {
                    log.Printf("relay error: %v", err)
                }
            }
        }
    }()
}
```

**Why `select`:** A plain `for { tick(); work() }` loop would block forever and never catch `ctx.Done()`.

---

## 13. Content-Addressable Storage

Use the hash of the content as the key — automatic deduplication.

```go
// Chunk PK = SHA-256 hash of data
type Chunk struct {
    Hash       string `gorm:"primaryKey"`  // hash = identity
    StorageKey string                       // "chunks/<hash>" in MinIO
}

// Service: verify + store
func (s *fileServiceImpl) StoreChunk(ctx context.Context, hash string, data []byte) error {
    computed := sha256.Sum256(data)
    if hex.EncodeToString(computed[:]) != hash {
        return domain.ErrHashMismatch  // client sent wrong hash
    }
    storageKey := "chunks/" + hash
    s.minio.Put(ctx, storageKey, data)
    s.chunkRepo.Save(ctx, domain.Chunk{Hash: hash, StorageKey: storageKey})
}

// Dedup: FindExistingHashes → client only uploads what the server doesn't have
existing, _ := s.chunkRepo.FindExistingHashes(ctx, req.ChunkHashes)
// → WHERE hash IN ('aaa','bbb','ccc') → returns hashes already stored
```

**Similar approach:** Git stores blobs/trees/commits by SHA. ZFS/Btrfs use it for block dedup.

---

## 14. Set-Diff Algorithm

Find added/removed items between two sets — used for delta sync.

```go
func (s *fileServiceImpl) GetSyncDiff(...) (added, removed []string, ...) {
    clientSet  := toSet(clientChunks)   // chunks the client currently has
    currentSet := toSet(currentChunks)  // chunks in the current server version

    for h := range currentSet {
        if !clientSet[h] { added = append(added, h) }    // new on server
    }
    for h := range clientSet {
        if !currentSet[h] { removed = append(removed, h) } // deleted on server
    }
}

func toSet(items []string) map[string]bool {
    s := make(map[string]bool, len(items))
    for _, v := range items { s[v] = true }
    return s
}
```

**Pattern:** `map[string]bool` as a Set in Go. O(1) lookup.

---

## 15. Layered Error Wrapping

```go
// Wrap with context to make tracing easier:
if err := pub.Publish(ctx, topic, eventType, payload); err != nil {
    return fmt.Errorf("publish event %d: %w", e.ID, err)
}

// Unwrap elsewhere:
if errors.Is(err, someSpecificError) { ... }
```

**Rule:** `%w` to wrap (unwrappable via `errors.Is`), `%v` for plain string formatting (not unwrappable).

---

## Summary — Checklist for a new Go project

```
□ domain/    → structs + sentinel errors (no imports from infra)
□ repository/→ interface + gorm impl + NewPostgresDB (AutoMigrate)
□ service/   → interface + impl (inject repos + clients)
□ client/    → HTTP client to call other services
□ api/
│  ├── httpx/     → OK/Created/BadRequest/NotFound/InternalError
│  ├── controller/→ parse request → call service → map error → respond
│  └── routes/    → Register(r *gin.Engine, ctrl ...)
□ config/    → Load() + getEnv(key, fallback)
□ cmd/api/main.go → wire everything bottom-up
```
