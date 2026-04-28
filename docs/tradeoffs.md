# Trade-offs

## Chunking: Fixed-size vs Content-Defined (CDC)

**Chosen:** Fixed 4MB chunks

Fixed-size is simple and predictable. CDC (rolling hash like Rabin fingerprint) gives better deduplication across file edits — inserting 1 byte shifts all subsequent fixed chunks, but CDC chunks stay stable. For a PoC demonstrating the concept, fixed-size is sufficient.

## Conflict Resolution: Last-write-wins vs Merge

**Chosen:** Last-write-wins

Binary files (PDF, images) cannot be merged generically. Text merge requires diff3 and file-format awareness. Last-write-wins preserves full version history — the "losing" user can restore from any previous version. Acceptable for PoC scope.

## Hub Storage: In-memory vs Redis Pub/Sub

**Chosen:** In-memory

In-memory hub works for a single sync-service instance. Redis pub/sub would be needed for horizontal scaling (multiple sync-service instances). For PoC, in-memory is sufficient and avoids extra infrastructure.

## Notification: HTTP call vs Kafka vs Redis Pub/Sub

**Chosen:** HTTP call (file-service → sync-service)

Kafka adds significant complexity (brokers, consumer groups, offset management). Redis pub/sub would work but adds a dependency. For a single-instance PoC, a direct HTTP call is simple and observable. Fire-and-forget goroutine ensures upload response isn't blocked by notification latency.

## Session State: In-memory upload sessions vs DB

**Chosen:** No upload session — file record created immediately in `InitUpload`

Tracking upload sessions (pending chunks) in Redis or DB adds complexity. Current approach creates the file record immediately and allows `CompleteUpload` to be called at any time after all chunks are stored. Stale incomplete uploads (abandoned after init) are acceptable for PoC.
