# Architecture

## Services

### file-service (port 8081)
Handles all file operations: upload lifecycle, download, versioning, sync diff.

**Dependencies:**
- PostgreSQL — stores `files`, `file_versions`, `version_chunks`, `chunks` metadata
- MinIO — binary chunk storage (S3-compatible object store)
- sync-service — HTTP call to `/internal/notify` after upload completes

**Key design decisions:**
- SHA-256 hash is the primary key for chunks (content-addressable storage)
- `InitUpload` returns only missing chunks → deduplication at protocol level
- `CompleteUpload` is transactional (version + version_chunks saved atomically)
- File versioning preserves full history — no data loss on conflict

### sync-service (port 8082)
Stateful WebSocket hub. Pushes events to watching clients.

**Dependencies:** None (no external infra — state is in-memory only)

**Key design decisions:**
- In-memory hub with RWMutex (sufficient for single-instance PoC)
- `file_changed` event is push-only; client decides when to sync
- Supports `"*"` wildcard fileId to watch all files

## Data Model

```
files
  id (PK, UUID)
  owner_id
  name
  current_version
  created_at

file_versions
  id (PK, UUID)
  file_id (FK → files)
  version
  created_by
  created_at

version_chunks
  id (PK, autoincrement)
  version_id (FK → file_versions)
  chunk_index        ← ordering
  chunk_hash (FK → chunks)

chunks
  hash (PK, SHA-256)  ← content-addressable
  size
  storage_key        ← MinIO object key: "chunks/<hash>"
```

## Object Storage Layout (MinIO)

```
bucket: dropbox-poc
  chunks/
    <sha256_hash_1>   ← binary data
    <sha256_hash_2>
    ...
```

Chunks are keyed by hash → automatic deduplication at storage level.

## Chunking Strategy

- **Fixed-size chunking** (4MB default) for simplicity
- Client-side: split `[]byte` by chunk size, compute SHA-256 per chunk
- Server verifies hash on `PUT /upload/chunk/:hash` (prevents corrupt uploads)

## Sync Protocol

```
1. Client reconnects → POST /files/:id/sync {clientVersion: N}
2. Server computes: current_chunks - client_chunks = added
                    client_chunks - current_chunks = removed
3. Client downloads `added`, deletes `removed` from local cache
4. Client reassembles from new chunk manifest
```

## Conflict Resolution

**Strategy: last-write-wins with explicit detection**

1. Client sends `baseVersion` in `CompleteUpload` — the version it started editing from
2. Server compares `baseVersion` vs `currentVersion`:
   - `baseVersion == currentVersion` → no conflict, save new version
   - `baseVersion < currentVersion` → conflict detected (someone uploaded in between)
3. Server **always saves the new version** (last write wins) regardless of conflict
4. If conflict: server queries `GetVersionCreator(currentVersion)` to identify the losing user
5. sync-service sends `upload_conflict` event directly to the loser via `Hub.NotifyUser`

```
No conflict:  base=2, current=2 → save v3, broadcast file_changed to watchers
Conflict:     base=1, current=2 → save v3, broadcast file_changed + push upload_conflict to loser
```

Full version history is always preserved — the losing user can restore from any previous version.

Rationale: simple and predictable. Merge strategies require file-format awareness (text diff3 vs binary). Not suitable for generic PoC scope.

## Trade-offs

| Decision | Chosen | Rejected | Reason |
|---|---|---|---|
| Chunk size | Fixed 4MB | Content-Defined (CDC) | CDC gives better dedup but adds complexity (rolling hash) |
| Conflict resolution | Last-write-wins | 3-way merge | Binary files can't be merged generically |
| Sync notification | WebSocket push | Polling / SSE | Lower latency, bidirectional |
| Infra notification | HTTP call | Redis pub/sub / Kafka | Minimal moving parts for PoC |
| Hub storage | In-memory | Redis | Single instance sufficient for PoC |
