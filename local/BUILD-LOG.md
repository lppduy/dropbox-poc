# Build Log

## Day 1

### Stack decisions
- Go + Gin (consistent with ecom-poc)
- MinIO (self-hosted S3) — avoids AWS dependency, has built-in web UI
- gorilla/websocket — most widely used Go WebSocket library
- GORM + postgres (consistent with ecom-poc)
- google/uuid for ID generation

### Architecture decisions
- 2 services: file-service (stateful) + sync-service (stateless)
- No Kafka — HTTP call from file-service to sync-service is sufficient
- In-memory hub for sync — scales to 1 instance, good enough for PoC
- Content-addressable storage: SHA-256 hash = primary key for chunks

### Key insight: deduplication protocol
`InitUpload` asks "which of these hashes do you already have?"
→ Server checks chunk table → returns only missing ones
→ Client uploads only what's needed
→ Two identical files share all chunks — zero re-upload

This is the same model used by Git, Btrfs, ZFS.

### Challenges
- `VersionChunk` needs an auto-increment PK (not composite) for GORM compatibility
- MinIO `BucketExists` called at startup to auto-create bucket
- WebSocket upgrade in Gin requires `CheckOrigin: return true` for local dev
- Sync notify runs in goroutine to not block upload response
