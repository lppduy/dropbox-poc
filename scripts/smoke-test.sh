#!/usr/bin/env bash
set -euo pipefail

FILE_SVC="${FILE_SERVICE_URL:-http://localhost:8081}"
SYNC_SVC="${SYNC_SERVICE_URL:-http://localhost:8082}"

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

pass() { echo -e "${GREEN}✓ $1${NC}"; }
fail() { echo -e "${RED}✗ $1${NC}"; exit 1; }

echo "=== Dropbox PoC Smoke Test ==="
echo ""

# health checks
echo "--- Health Checks ---"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$FILE_SVC/health")
[ "$STATUS" = "200" ] && pass "file-service /health" || fail "file-service /health (got $STATUS)"

STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$SYNC_SVC/health")
[ "$STATUS" = "200" ] && pass "sync-service /health" || fail "sync-service /health (got $STATUS)"

echo ""
echo "--- Upload Flow ---"

# create a small test chunk (4 bytes) and compute sha256
CHUNK_DATA="test"
CHUNK_HASH=$(echo -n "$CHUNK_DATA" | sha256sum | awk '{print $1}')
pass "computed chunk hash: $CHUNK_HASH"

# init upload
INIT_RESP=$(curl -s -X POST "$FILE_SVC/upload/init" \
  -H "Content-Type: application/json" \
  -d "{\"ownerId\":\"user1\",\"filename\":\"smoke.txt\",\"chunkHashes\":[\"$CHUNK_HASH\"]}")

FILE_ID=$(echo "$INIT_RESP" | grep -o '"fileId":"[^"]*"' | cut -d'"' -f4)
MISSING=$(echo "$INIT_RESP" | grep -o '"missingChunks":\[[^]]*\]')

[ -n "$FILE_ID" ] && pass "init upload → fileId=$FILE_ID" || fail "init upload failed: $INIT_RESP"
echo "  missingChunks: $MISSING"

# upload missing chunk
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$FILE_SVC/upload/chunk/$CHUNK_HASH" \
  -H "Content-Type: application/octet-stream" \
  --data-raw "$CHUNK_DATA")
[ "$STATUS" = "200" ] && pass "upload chunk" || fail "upload chunk (got $STATUS)"

# complete upload
COMPLETE_RESP=$(curl -s -X POST "$FILE_SVC/files/$FILE_ID/complete" \
  -H "Content-Type: application/json" \
  -d "{\"ownerId\":\"user1\",\"orderedHashes\":[\"$CHUNK_HASH\"]}")

VERSION=$(echo "$COMPLETE_RESP" | grep -o '"version":[0-9]*' | cut -d: -f2)
[ -n "$VERSION" ] && pass "complete upload → version=$VERSION" || fail "complete upload failed: $COMPLETE_RESP"

echo ""
echo "--- Download Flow ---"

# get manifest
MANIFEST=$(curl -s "$FILE_SVC/files/$FILE_ID/manifest")
[ -n "$MANIFEST" ] && pass "get manifest: $MANIFEST" || fail "get manifest failed"

# download chunk
DL_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$FILE_SVC/chunks/$CHUNK_HASH")
[ "$DL_STATUS" = "200" ] && pass "download chunk" || fail "download chunk (got $DL_STATUS)"

echo ""
echo "--- Deduplication ---"

# upload same file again — server should report 0 missing chunks
INIT2=$(curl -s -X POST "$FILE_SVC/upload/init" \
  -H "Content-Type: application/json" \
  -d "{\"ownerId\":\"user2\",\"filename\":\"smoke-dup.txt\",\"chunkHashes\":[\"$CHUNK_HASH\"]}")

MISSING2=$(echo "$INIT2" | grep -o '"missingChunks":\[[^]]*\]')
if echo "$MISSING2" | grep -q '\[\]'; then
  pass "dedup: 0 missing chunks for identical content"
else
  fail "dedup: expected empty missingChunks, got: $MISSING2"
fi

echo ""
echo "--- Sync Diff ---"

DIFF=$(curl -s -X POST "$FILE_SVC/files/$FILE_ID/sync" \
  -H "Content-Type: application/json" \
  -d '{"clientVersion":0}')
[ -n "$DIFF" ] && pass "sync diff: $DIFF" || fail "sync diff failed"

echo ""
echo "=== All smoke tests passed ==="
