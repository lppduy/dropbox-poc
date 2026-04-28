# Dropbox PoC

Distributed file storage system with chunking, deduplication, delta sync, and real-time WebSocket notifications.

**Stack:** Go + Gin + PostgreSQL + MinIO + WebSocket (gorilla)

## Architecture

```
Client
  │
  ├── file-service (port 8081)   ← upload, download, versioning, sync diff
  │     └── PostgreSQL            ← file/chunk metadata
  │     └── MinIO                 ← binary chunk storage
  │
  └── sync-service (port 8082)  ← WebSocket hub, real-time notify
```

## Core Concepts

| Concept | How it works |
|---|---|
| **Chunking** | Client splits file into fixed-size chunks before upload |
| **Deduplication** | SHA-256 hash = chunk identity; duplicate content = zero upload |
| **Delta sync** | Server diffs client version vs current → only changed chunks |
| **Versioning** | Every `complete` creates a new version; history preserved |
| **Real-time** | WebSocket push when file changes → client auto-syncs |

## API Reference

### file-service

| Method | Path | Description |
|---|---|---|
| `POST` | `/upload/init` | Start upload session, get missing chunks list |
| `PUT` | `/upload/chunk/:hash` | Upload a single chunk (SHA-256 verified) |
| `POST` | `/files/:id/complete` | Finalize upload, create new version |
| `GET` | `/files/:id/manifest` | Get ordered chunk hashes for a file |
| `GET` | `/chunks/:hash` | Download a single chunk |
| `POST` | `/files/:id/sync` | Get diff between client version and current |

### sync-service

| Method | Path | Description |
|---|---|---|
| `GET` | `/ws?userId=<id>` | WebSocket connection |
| `POST` | `/internal/notify` | Internal endpoint called by file-service |

### WebSocket protocol

**Client → Server** (after connect):
```json
{ "action": "watch", "fileIds": ["file-uuid-1", "*"] }
```

**Server → Client** (when file changes):
```json
{ "event": "file_changed", "fileId": "...", "version": 3, "changedBy": "user_alice" }
```

## Quick Start

```bash
# Start infrastructure
cd infra && docker compose up -d

# Wait for services to be healthy, then run smoke test
./scripts/smoke-test.sh

# Full E2E happy path
./scripts/e2e-happy-path.sh
```

**MinIO Web UI:** http://localhost:9001 (minioadmin / minioadmin)

## Upload Flow Example

```bash
# 1. Split file into chunks (client-side)
split -b 4m bigfile.pdf chunk_

# 2. Compute hashes
sha256sum chunk_* | awk '{print $1}' > hashes.txt

# 3. Init upload
curl -X POST http://localhost:8081/upload/init \
  -H "Content-Type: application/json" \
  -d '{"ownerId":"user1","filename":"bigfile.pdf","chunkHashes":["hash0","hash1","hash2"]}'
# → {"fileId":"<uuid>","missingChunks":["hash1"]}  ← only upload what's missing

# 4. Upload missing chunks
curl -X PUT http://localhost:8081/upload/chunk/hash1 \
  -H "Content-Type: application/octet-stream" \
  --data-binary @chunk_ab

# 5. Finalize
curl -X POST http://localhost:8081/files/<uuid>/complete \
  -H "Content-Type: application/json" \
  -d '{"ownerId":"user1","orderedHashes":["hash0","hash1","hash2"]}'
```

## Delta Sync Flow Example

```bash
# Client reconnects after being offline
curl -X POST http://localhost:8081/files/<uuid>/sync \
  -H "Content-Type: application/json" \
  -d '{"clientVersion":2}'
# → {"currentVersion":4,"needDownload":["hashX","hashY"],"needDelete":["hashB"]}
# Client only downloads hashX + hashY
```

## WebSocket Example

```bash
# Connect and watch a file
wscat -c "ws://localhost:8082/ws?userId=user_bob"
> {"action":"watch","fileIds":["<file-uuid>"]}

# When user_alice uploads a new version, Bob receives:
< {"event":"file_changed","fileId":"<uuid>","version":3,"changedBy":"user_alice"}
```
