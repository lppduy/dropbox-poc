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

## Conflict Resolution (Last-Write-Wins)

Alice and Bob both start from version 1 and upload concurrently.

```
Bob                      file-service           PostgreSQL       sync-service       Alice
 │                            │                      │                │               │
 │                            │                      │                │               │
 │ POST /complete             │                      │                │               │
 │ {baseVersion:1}            │                      │                │               │
 │───────────────────────────►│                      │                │               │
 │                            │ GetCurrentVersion    │                │               │
 │                            │─────────────────────►│                │               │
 │                            │◄── version=1         │                │               │
 │                            │ base(1)==current(1)  │                │               │
 │                            │ no conflict          │                │               │
 │                            │ SaveVersion(v2) ─────►                │               │
 │◄── {version:2}             │                      │                │               │
 │                            │                      │                │               │
 │                            │ POST /internal/notify──────────────── ►               │
 │                            │                      │ file_changed ──────────────── ►│
 │                            │                      │                │               │
 │                            │                 [Alice now uploads, still based on v1]│
 │                            │                      │                │               │
 │                            │◄── POST /complete ────────────────────────────────── │
 │                            │    {baseVersion:1}   │                │               │
 │                            │ GetCurrentVersion    │                │               │
 │                            │─────────────────────►│                │               │
 │                            │◄── version=2         │                │               │
 │                            │ base(1) < current(2) │                │               │
 │                            │ CONFLICT!            │                │               │
 │                            │ GetVersionCreator(v2)►                │               │
 │                            │◄── createdBy="bob"   │                │               │
 │                            │ SaveVersion(v3)──────►                │               │
 │                            │◄── {version:3,       │                │               │
 │                            │     conflict:true,   │                │               │
 │                            │     loser:"bob"}     │                │               │
 │                            │                      │                │               │
 │                            │ POST /internal/notify──────────────── ►               │
 │                            │  {conflict:true,     │ file_changed──────────────────►│
 │                            │   loserUserId:"bob"} │                │               │
 │                            │                      │ NotifyUser("bob")              │
 │◄── upload_conflict ────────│──────────────────────│────────────────│               │
 │  "your version was         │                      │                │               │
 │   overwritten"             │                      │                │               │
```

**Result:**
- Alice wins — her upload becomes version 3 (latest)
- Bob loses — receives `upload_conflict` WebSocket event directly
- Both version 2 (Bob's) and version 3 (Alice's) are preserved in history
- Bob can restore version 2 if needed (via version rollback, optional feature)
