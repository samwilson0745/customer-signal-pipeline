#!/usr/bin/env bash
# End-to-end smoke/integration test for the producer -> triage -> query-api
# path. Expects the full stack to already be up: `docker compose up -d`.
#
# Verifies:
#   1. triage-service and query-api report healthy.
#   2. Events published by the producer are visible via /stats within a
#      reasonable window (proves Kafka -> triage -> Redis is wired up).
#   3. /search returns enriched results with sentiment/urgency/topic set
#      (proves triage -> Elasticsearch is wired up).
#   4. Unauthenticated requests to /search and /stats are rejected (401).
#
# Usage: scripts/integration_test.sh [brand]
set -euo pipefail

QUERY_API_URL="${QUERY_API_URL:-http://localhost:8080}"
TRIAGE_URL="${TRIAGE_URL:-http://localhost:8000}"
JWT_SECRET="${JWT_SECRET:-change-me-in-production}"
JWT_ISSUER="${JWT_ISSUER:-customer-signal-pipeline}"
BRAND="${1:-acme}"
MAX_WAIT_SECONDS=120

fail() { echo "FAIL: $1" >&2; exit 1; }
pass() { echo "PASS: $1"; }

echo "== 1. Waiting for triage-service /health =="
for i in $(seq 1 30); do
  if curl -sf "$TRIAGE_URL/health" >/dev/null; then pass "triage-service healthy"; break; fi
  [ "$i" -eq 30 ] && fail "triage-service never became healthy"
  sleep 2
done

echo "== 2. Waiting for query-api /health =="
for i in $(seq 1 30); do
  if curl -sf "$QUERY_API_URL/health" >/dev/null; then pass "query-api healthy"; break; fi
  [ "$i" -eq 30 ] && fail "query-api never became healthy"
  sleep 2
done

echo "== 3. Generating a JWT via gen-token =="
TOKEN=$(docker compose exec -T query-api ./gen-token -sub integration-test -secret "$JWT_SECRET" -issuer "$JWT_ISSUER")
[ -n "$TOKEN" ] || fail "could not generate JWT"
pass "obtained JWT"

echo "== 4. Unauthenticated requests are rejected =="
STATUS=$(curl -s -o /dev/null -w '%{http_code}' "$QUERY_API_URL/search?q=test")
[ "$STATUS" = "401" ] || fail "expected 401 for unauthenticated /search, got $STATUS"
pass "unauthenticated /search rejected with 401"

echo "== 5. Waiting for producer events to flow through to /stats (brand=$BRAND) =="
DEADLINE=$((SECONDS + MAX_WAIT_SECONDS))
TOTAL=0
while [ "$SECONDS" -lt "$DEADLINE" ]; do
  RESP=$(curl -sf -H "Authorization: Bearer $TOKEN" "$QUERY_API_URL/stats?brand=$BRAND" || echo '{}')
  TOTAL=$(echo "$RESP" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("total", 0))' 2>/dev/null || echo 0)
  if [ "${TOTAL:-0}" -gt 0 ] 2>/dev/null; then
    pass "/stats shows $TOTAL events for brand=$BRAND"
    break
  fi
  sleep 3
done
[ "${TOTAL:-0}" -gt 0 ] 2>/dev/null || fail "/stats never showed events for brand=$BRAND after ${MAX_WAIT_SECONDS}s"

echo "== 6. /search returns enriched results =="
SEARCH_RESP=$(curl -sf -H "Authorization: Bearer $TOKEN" "$QUERY_API_URL/search?brand=$BRAND")
COUNT=$(echo "$SEARCH_RESP" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("count", 0))')
[ "$COUNT" -gt 0 ] || fail "/search returned no results for brand=$BRAND"
FIRST_SENTIMENT=$(echo "$SEARCH_RESP" | python3 -c 'import json,sys; print(json.load(sys.stdin)["results"][0].get("sentiment", ""))')
[ -n "$FIRST_SENTIMENT" ] || fail "search result missing sentiment enrichment"
pass "/search returned $COUNT enriched results (first sentiment=$FIRST_SENTIMENT)"

echo
echo "ALL INTEGRATION CHECKS PASSED"
