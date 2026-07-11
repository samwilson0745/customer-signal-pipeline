# Customer Signal Pipeline

A small but real distributed backend system that ingests customer messages
from multiple channels (Twitter, live chat, email, app reviews), streams
them through Kafka, uses an LLM to automatically classify each message
(sentiment, urgency, topic), stores raw and enriched data in the right
stores, and exposes a secured search/stats API in front of it all.

This is a scaled-down Unified Customer Experience Management (Unified-CXM)
pipeline — the same category of problem large CX platforms solve at
enterprise scale.

## Architecture

```
[Producer service]                                                  [Query API]
 (synthetic customer      --publish-->   [Kafka topic:      <--search--   (Go, JWT auth,
  messages: twitter,                      customer-signals]              rate limiting,
  chat, email, reviews)                          |                       circuit breaker)
                                                  |                              |
                                                  v                              v
                                      [Triage Service (consumer)]        [Elasticsearch]
                                       - consumes from Kafka              (searchable,
                                       - classifies via LLM               enriched events)
                                         (sentiment / urgency / topic)          ^
                                       - writes raw event -----------> [Cassandra]
                                       - writes enriched doc ---------------^
                                       - increments live counters --> [Redis]
                                                                             ^
                                                                    [Query API reads
                                                                     stats from Redis]
```

| Component | Language | Responsibility |
|---|---|---|
| [`producer/`](producer) | Node.js + TypeScript (`kafkajs`) | Generates a synthetic stream of customer messages across 4 channels and publishes them to Kafka. |
| [`triage-service/`](triage-service) | Python (FastAPI) | Consumes from Kafka, classifies each message (LLM or heuristic fallback), writes raw events to Cassandra, enriched docs to Elasticsearch, and live counters to Redis. Never drops a message silently — failures go to a Kafka dead-letter topic. |
| [`query-api/`](query-api) | Go | The only externally exposed service. Self-contained JWT auth, Redis-backed token-bucket rate limiting, and a circuit breaker in front of Elasticsearch/Redis. Serves `/search`, `/stats`, `/health`. |
| [`dashboard/`](dashboard) | Static HTML/JS | Read-only live view of `/stats` and `/health`, polled directly from the browser. No build step, no server-side code. |

## Event schema

```json
{
  "event_id": "evt_<timestamp>_<seq>",
  "channel": "twitter | chat | email | app_review",
  "author": "string",
  "text": "string",
  "lang": "en",
  "created_at": "ISO-8601 timestamp",
  "brand": "string"
}
```

## Quick start

Requires Docker + Docker Compose.

```bash
cp .env.example .env
# Optional: set GEMINI_API_KEY in .env to use the real LLM classifier.
# Without a key, triage-service falls back to a deterministic keyword-based
# heuristic classifier so the whole pipeline still runs end-to-end.

docker compose up -d --build
docker compose ps        # everything should report (healthy)
docker compose logs -f producer triage-service
```

Bring it down with `docker compose down` (add `-v` to also drop the
Cassandra/Elasticsearch volumes).

### Getting a JWT

The Query API has no user store — it only verifies bearer tokens signed
with `JWT_SECRET`. Use the bundled CLI to mint one for testing:

```bash
TOKEN=$(docker compose exec -T query-api ./gen-token -sub demo-client -secret change-me-in-production -issuer customer-signal-pipeline)
echo "$TOKEN"
```

### Live dashboard

A static, read-only dashboard polls `/stats` and `/health` directly from the
browser — no server-side code of its own. It comes up automatically with
`docker compose up` on `http://localhost:${DASHBOARD_PORT:-8090}`.

Open it, paste the `$TOKEN` from above into the "JWT" field, pick a brand
(`acme`, `globex`, `initech`), and it'll show live totals plus
sentiment/urgency breakdowns, refreshing on the interval you choose. The
Query API allows cross-origin requests specifically so this page (served
from a different port) can call it with `fetch()`.

### Example calls

```bash
# Health check (also reflects circuit breaker state)
curl -s http://localhost:8080/health | jq

# Full-text + filtered search (JWT-protected)
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/search?q=refund&brand=acme&sentiment=negative" | jq

# Near-real-time sentiment/urgency breakdown for a brand (from Redis)
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/stats?brand=acme" | jq
```

Example `/stats` response:

```json
{
  "brand": "acme",
  "total": 86,
  "sentiment": { "positive": 22, "neutral": 45, "negative": 19 },
  "urgency": { "low": 58, "medium": 8, "high": 12, "critical": 8 }
}
```

## Resilience

- **JWT auth** — `/search` and `/stats` require `Authorization: Bearer <token>`; invalid/expired/missing tokens get `401`.
- **Rate limiting** — a Redis-backed token bucket keyed by JWT subject (`RATE_LIMIT_PER_MINUTE`, default 60/min with a burst of the same size). Exceeding it returns `429`.
- **Circuit breaker** — Elasticsearch and Redis calls are each wrapped in an independent breaker (`internal/circuitbreaker`). After `CB_FAILURE_THRESHOLD` consecutive failures it opens and short-circuits (`503`) without hitting the downstream; after `CB_COOLDOWN_SECONDS` it allows a single half-open trial call, closing again on success or re-opening on failure. `/health` reports both breakers' state.
- **Triage retries / DLQ** — LLM calls and downstream writes (Cassandra/Elasticsearch/Redis) retry with exponential backoff via `tenacity`. If classification or a write still fails after retries, the original raw event is published to `customer-signals-dlq` with the failure reason — never silently dropped.

## Tests

```bash
# Triage-service: classifier prompt-parsing / retry / fallback logic (mocked LLM, no network)
cd triage-service && python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
python -m pytest tests/ -v

# Query API: circuit breaker + Redis token-bucket rate limiter (miniredis, no real Redis needed)
cd query-api
go test ./... -v

# Full integration test against the live stack (producer -> triage -> query-api)
docker compose up -d
./scripts/integration_test.sh acme
```

## Configuration

All connection strings, topic names, and API keys are environment
variables — see [`.env.example`](.env.example). No secrets are committed.

Key variables:

| Variable | Purpose |
|---|---|
| `EVENTS_PER_SECOND` | Producer publish rate |
| `BRANDS` | Comma-separated brands the producer/triage/query-api all share |
| `GEMINI_API_KEY` | If unset, triage-service uses the heuristic classifier instead of calling an LLM |
| `JWT_SECRET`, `JWT_ISSUER` | Query API auth |
| `RATE_LIMIT_PER_MINUTE` | Query API token-bucket size/refill rate |
| `CB_FAILURE_THRESHOLD`, `CB_COOLDOWN_SECONDS` | Query API circuit breaker tuning |
| `DASHBOARD_PORT` | Port the static dashboard is served on (default 8090) |

## Init scripts

- [`scripts/init_cassandra.cql`](scripts/init_cassandra.cql) — reference copy of the `raw_events` schema (triage-service also applies this automatically on startup).
- [`scripts/init_es_mapping.json`](scripts/init_es_mapping.json) — reference copy of the `customer-signals` index mapping (also applied automatically by triage-service on startup).
- [`scripts/integration_test.sh`](scripts/integration_test.sh) — end-to-end smoke test against a running `docker compose` stack.

## Stretch goals not implemented

- Swapping the synthetic producer for a real Reddit/Twitter sample stream.
- Prometheus metrics / OpenTelemetry tracing from the Query API.

The dead-letter Kafka topic (`customer-signals-dlq`) *is* implemented — see
[`triage-service/app/kafka_consumer.py`](triage-service/app/kafka_consumer.py).
The read-only live dashboard *is* implemented — see
[`dashboard/`](dashboard) and "Live dashboard" above.

## Screenshots
<img width="1470" height="768" alt="Screenshot 2026-07-11 at 6 14 59 PM" src="https://github.com/user-attachments/assets/cf6e8f21-3130-4f78-9ef3-49cc5f90c7a1" />

