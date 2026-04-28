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

## Flows

### Flow 1 — Upload

> Client wants to save a file to the server

```
Client                          file-service
  │                                  │
  │── POST /upload/init ────────────>│  send all chunk hashes
  │<── { missingChunks: [...] } ─────│  server returns only hashes it doesn't have
  │                                  │
  │── PUT /upload/chunk/:hash ──────>│  upload each missing chunk
  │   (skip chunks already stored)   │  server verifies SHA-256
  │                                  │
  │── POST /files/:id/complete ─────>│  send orderedHashes + baseVersion
  │<── { version: 2 } ──────────────│  create new version, notify sync-service
```

**Dedup:** if another file already uploaded the same chunk → skip, no re-upload.

---

### Flow 2 — Download

> Client wants to download a file

```
Client                          file-service          MinIO
  │                                  │                  │
  │── GET /files/:id/manifest ──────>│                  │
  │<── { chunks: [h0,h1,h2] } ──────│  ordered list of chunk hashes
  │                                  │                  │
  │── GET /chunks/h0 ───────────────>│── get object ───>│
  │<── binary data ─────────────────│<─────────────────│
  │   (repeat for h1, h2)           │                  │
  │                                  │                  │
  │ reassemble h0+h1+h2 → original file                 │
```

---

### Flow 3 — Real-time Sync (WebSocket)

> Bob wants to be notified immediately when Alice edits a file

```
Bob                     sync-service            Alice → file-service
  │                          │                         │
  │── WS connect ───────────>│                         │
  │── watch { fileId } ─────>│  register as watcher    │
  │                          │                         │
  │                          │<── /internal/notify ────│  Alice finishes upload
  │                          │                         │
  │<── file_changed ─────────│  push via WebSocket     │
  │   { version: 3 }         │                         │
```

---

### Flow 4 — Delta Sync

> Client was offline, wants to sync without re-downloading the entire file

```
Client                          file-service
  │                                  │
  │── POST /files/:id/sync ─────────>│  send clientVersion: 2
  │<── {                             │
  │     currentVersion: 5,           │  diff chunks of v2 vs v5
  │     needDownload: [hashX,hashY], │  new chunks → download
  │     needDelete:   [hashB]        │  removed chunks → delete locally
  │   }                              │
  │                                  │
  │── download hashX, hashY          │  only transfer what changed
```

---

### Flow 5 — Conflict (Last-Write-Wins)

> Alice and Bob both edit the file from version 1, Bob uploads first

```
Bob                       file-service              Alice
  │                            │                      │
  │── complete (base=1) ──────>│                      │
  │<── { version: 2 } ─────────│  Bob saves v2        │
  │                            │                      │
  │                            │       Alice also uploads (base=1)
  │                            │<── complete (base=1) ─│
  │                            │  base(1) < current(2) → CONFLICT
  │                            │  save v3, loser = Bob │
  │                            │                      │
  │<── upload_conflict ────────│─────────────────────>│  Alice wins (v3)
  │   "your v2 was overwritten"│                      │  Bob gets conflict event
```

**Rule:** last upload wins. The losing client receives `upload_conflict` via WebSocket. All versions are preserved in the DB.

---

## API Quick Reference

```bash
# Upload
POST /upload/init          # start upload, receive missingChunks
PUT  /upload/chunk/:hash   # upload a single chunk
POST /files/:id/complete   # finalize upload, create new version

# Download
GET /files/:id/manifest    # get ordered chunk hashes
GET /chunks/:hash          # download a single chunk

# Sync
POST /files/:id/sync       # diff clientVersion vs current

# WebSocket (sync-service)
GET  /ws?userId=<id>       # connect
```

## Quick Start

```bash
cd infra && docker compose up -d
./scripts/smoke-test.sh
./scripts/e2e-happy-path.sh
```

**MinIO Web UI:** http://localhost:9001 (minioadmin / minioadmin)
