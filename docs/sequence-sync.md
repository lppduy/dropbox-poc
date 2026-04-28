# Sequence Diagrams

## Upload + Sync Notification

```
Client A          file-service        MinIO          PostgreSQL      sync-service     Client B
   │                   │                 │                 │               │               │
   │ POST /upload/init │                 │                 │               │               │
   │──────────────────►│                 │                 │               │               │
   │                   │ FindExistingHashes              │               │               │
   │                   │─────────────────────────────────►               │               │
   │                   │◄────────────────────────────────                │               │
   │◄── missingChunks ─│                 │                 │               │               │
   │                   │                 │                 │               │               │
   │ PUT /chunk/:hash  │                 │                 │               │               │
   │──────────────────►│                 │                 │               │               │
   │                   │ verify SHA-256  │                 │               │               │
   │                   │ PutObject ──────►                 │               │               │
   │                   │ Save chunk ─────────────────────► │               │               │
   │◄── 200 OK ────────│                 │                 │               │               │
   │                   │                 │                 │               │               │
   │ POST /complete    │                 │                 │               │               │
   │──────────────────►│                 │                 │               │               │
   │                   │ SaveVersion (tx)────────────────► │               │               │
   │                   │ UpdateVersion ──────────────────► │               │               │
   │◄── {version:1} ───│                 │                 │               │               │
   │                   │                 │                 │               │               │
   │                   │ POST /internal/notify ────────────────────────── ►               │
   │                   │                 │                 │               │ push event ──►│
   │                   │                 │                 │               │  file_changed │
```

## Delta Sync (Client Reconnects)

```
Client (offline → online)          file-service          PostgreSQL
   │                                    │                     │
   │ local: file_v2 [h1,h2,h3]        │                     │
   │                                    │                     │
   │ POST /files/:id/sync              │                     │
   │ {clientVersion: 2}                │                     │
   │───────────────────────────────────►                     │
   │                                    │ GetCurrentVersion   │
   │                                    │────────────────────►│
   │                                    │◄── version=4        │
   │                                    │                     │
   │                                    │ GetChunksByVersion(2)
   │                                    │────────────────────►│
   │                                    │◄── [h1,h2,h3]       │
   │                                    │                     │
   │                                    │ GetCurrentChunks    │
   │                                    │────────────────────►│
   │                                    │◄── [h1,h4,h3,h5]   │
   │                                    │                     │
   │                                    │ diff:               │
   │                                    │   added=[h4,h5]     │
   │                                    │   removed=[h2]      │
   │◄── {needDownload:[h4,h5],         │                     │
   │     needDelete:[h2], current:4}   │                     │
   │                                    │                     │
   │ GET /chunks/h4, GET /chunks/h5    │                     │
   │────────────────────────────────── ►                     │
   │◄── binary data ──────────────────                       │
   │                                    │                     │
   │ reassemble: [h1,h4,h3,h5]        │                     │
   │ (sorted by chunk_index)            │                     │
```

## WebSocket Watch Protocol

```
Client                    sync-service hub
   │                            │
   │ WS connect                 │
   │ ?userId=user_bob           │
   │───────────────────────────►│ Register(client)
   │                            │
   │ {"action":"watch",         │
   │  "fileIds":["uuid1","*"]} │
   │───────────────────────────►│ client.FileIDs["uuid1"]=true
   │                            │ client.FileIDs["*"]=true
   │                            │
   │                  [file_changed event arrives]
   │                            │
   │◄── {"event":"file_changed",│
   │     "fileId":"uuid1",      │
   │     "version":3,           │
   │     "changedBy":"user_alice"}
   │                            │
   │ [client triggers sync diff]│
```
