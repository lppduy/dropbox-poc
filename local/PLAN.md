# Dropbox PoC — Build Plan

## Phase 1: Core Upload/Download ✅
- [x] file-service skeleton (config, domain, repo, service, api)
- [x] MinIO integration (chunk store)
- [x] SHA-256 verification on chunk upload
- [x] Deduplication via `FindExistingHashes`
- [x] File versioning (version_chunks table)
- [x] Download: manifest + chunk fetch

## Phase 2: Sync ✅
- [x] sync-service WebSocket hub
- [x] file-service → sync-service HTTP notify on complete upload
- [x] Delta sync endpoint (`/files/:id/sync`)

## Phase 3: Scripts & Docs ✅
- [x] smoke-test.sh
- [x] e2e-happy-path.sh
- [x] README, architecture.md, tradeoffs.md, sequence diagrams

## Phase 4: Polish (optional)
- [ ] Pagination for manifest (large files with many chunks)
- [ ] File listing endpoint (`GET /files?ownerId=...`)
- [ ] Version history endpoint (`GET /files/:id/versions`)
- [ ] Prometheus metrics
- [ ] Rate limiting on chunk upload
- [ ] Content-Defined Chunking (CDC) as alternative chunking strategy
