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

## Day 2

### Conflict detection (explicit last-write-wins)

**Problem:** Last-write-wins was implicit — the losing client had no way to know their version was overwritten.

**Solution:**
- Client sends `baseVersion` in `CompleteUpload` (the version it started editing from)
- Server detects conflict when `baseVersion < currentVersion`
- Server still saves the new version (last write wins)
- Server queries `GetVersionCreator(currentVersion)` to identify the loser
- file-service notifies sync-service with `conflict: true, loserUserId`
- sync-service calls `Hub.NotifyUser(loserUserId)` to push `upload_conflict` event directly to the losing client

**Key decision:** Use `Hub.NotifyUser` (direct user notify) vs `NotifyFileChanged` (broadcast to watchers) — conflict notification is personal, not a broadcast.

### Docs update
- All docs translated to English (GO-PATTERNS.md was mixed Vietnamese/English)
- Conflict sequence diagram added to sequence-sync.md
- GO-PATTERNS.md copied to `__projects/` root for cross-project reference
