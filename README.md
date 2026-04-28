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

> Client muốn lưu file lên server

```
Client                          file-service
  │                                  │
  │── POST /upload/init ────────────>│  gửi tất cả chunk hashes
  │<── { missingChunks: [...] } ─────│  server chỉ trả về hash nào chưa có
  │                                  │
  │── PUT /upload/chunk/:hash ──────>│  upload từng chunk còn thiếu
  │   (bỏ qua chunk đã tồn tại)      │  server verify SHA-256
  │                                  │
  │── POST /files/:id/complete ─────>│  gửi orderedHashes + baseVersion
  │<── { version: 2 } ──────────────│  tạo version mới, notify sync-service
```

**Dedup:** nếu file khác đã upload chunk giống nhau → skip, không upload lại.

---

### Flow 2 — Download

> Client muốn tải file về

```
Client                          file-service          MinIO
  │                                  │                  │
  │── GET /files/:id/manifest ──────>│                  │
  │<── { chunks: [h0,h1,h2] } ──────│  danh sách hash theo thứ tự
  │                                  │                  │
  │── GET /chunks/h0 ───────────────>│── get object ───>│
  │<── binary data ─────────────────│<─────────────────│
  │   (lặp lại cho h1, h2)          │                  │
  │                                  │                  │
  │ ghép h0+h1+h2 thành file gốc    │                  │
```

---

### Flow 3 — Real-time Sync (WebSocket)

> Bob muốn biết ngay khi Alice sửa file

```
Bob                     sync-service            Alice → file-service
  │                          │                         │
  │── WS connect ───────────>│                         │
  │── watch { fileId } ─────>│  đăng ký theo dõi       │
  │                          │                         │
  │                          │<── /internal/notify ────│  Alice upload xong
  │                          │                         │
  │<── file_changed ─────────│  push qua WebSocket     │
  │   { version: 3 }         │                         │
```

---

### Flow 4 — Delta Sync

> Client offline một thời gian, muốn sync lại mà không tải toàn bộ file

```
Client                          file-service
  │                                  │
  │── POST /files/:id/sync ─────────>│  gửi clientVersion: 2
  │<── {                             │
  │     currentVersion: 5,           │  so sánh chunk của v2 vs v5
  │     needDownload: [hashX,hashY], │  chunk mới → tải về
  │     needDelete:   [hashB]        │  chunk bị xóa → xóa local
  │   }                              │
  │                                  │
  │── tải hashX, hashY               │  chỉ tải phần thay đổi
```

---

### Flow 5 — Conflict (Last-Write-Wins)

> Alice và Bob cùng sửa file từ version 1, Bob upload trước

```
Bob                       file-service              Alice
  │                            │                      │
  │── complete (base=1) ──────>│                      │
  │<── { version: 2 } ─────────│  Bob lưu v2          │
  │                            │                      │
  │                            │       Alice cũng upload (base=1)
  │                            │<── complete (base=1) ─│
  │                            │  base(1) < current(2) → CONFLICT
  │                            │  lưu v3, loser = Bob  │
  │                            │                      │
  │<── upload_conflict ────────│─────────────────────>│  Alice thắng (v3)
  │   "your v2 was overwritten"│                      │  Bob nhận conflict
```

**Quy tắc:** version nào upload sau thắng. Người thua nhận `upload_conflict` qua WebSocket. Mọi version đều được giữ lại trong DB.

---

## API Quick Reference

```bash
# Upload
POST /upload/init          # bắt đầu, nhận missingChunks
PUT  /upload/chunk/:hash   # upload từng chunk
POST /files/:id/complete   # hoàn tất, tạo version mới

# Download
GET /files/:id/manifest    # lấy danh sách chunk hashes
GET /chunks/:hash          # tải 1 chunk

# Sync
POST /files/:id/sync       # diff clientVersion vs current

# WebSocket (sync-service)
GET  /ws?userId=<id>       # kết nối
```

## Quick Start

```bash
cd infra && docker compose up -d
./scripts/smoke-test.sh
./scripts/e2e-happy-path.sh
```

**MinIO Web UI:** http://localhost:9001 (minioadmin / minioadmin)
