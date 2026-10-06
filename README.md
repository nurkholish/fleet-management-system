# Fleet Management System

Real-time vehicle tracking with MQTT ingestion, PostgreSQL persistence,
state-based geofence detection, and RabbitMQ event publishing.

---

## Table of Contents

- [Highlights](#highlights)
- [Architecture](#architecture)
- [Components](#components)
- [Feature Matrix](#feature-matrix)
- [Quick Start](#quick-start)
- [API Endpoints](#api-endpoints)
- [Testing](#testing)
- [MQTT](#mqtt)
- [RabbitMQ](#rabbitmq)
- [Configuration](#configuration)
- [Project Structure](#project-structure)
- [End-to-End Verification](#end-to-end-verification)
- [Production Considerations](#production-considerations)
- [Notes on Spec Discrepancies](#notes-on-spec-discrepancies)

---

## Highlights

- **State-based geofence** — emits `geofence_entry` only on the
  outside → inside transition. No event spam while a vehicle stays inside.
- **MQTT topic ↔ payload cross-validation** — rejects spoofed messages
  where `vehicle_id` in the topic doesn't match the payload.
- **Idempotent writes** — unique constraint `(vehicle_id, timestamp)` makes
  re-delivery safe under at-least-once MQTT semantics.
- **Dead Letter Queue** — malformed messages never block the main consumer.
- **Publisher confirms** — the AMQP publisher waits for broker ack before
  returning success, with automatic reconnect on failure.
- **Graceful shutdown** — ordered draining: HTTP → MQTT → RabbitMQ → PostgreSQL.
- **Structured logging** — zerolog JSON with request IDs for full traceability.
- **Race-detector-clean tests** — `go test -race ./...` passes on the entire
  codebase, including a concurrency test for the geofence state.

---

## Architecture

```
┌──────────────┐   MQTT    ┌────────────┐
│  Publisher   │──────────▶│ Mosquitto  │
│ (mock GPS)   │           │  :1883     │
└──────────────┘           └─────┬──────┘
                                 │  /fleet/vehicle/+/location
                                 ▼
                        ┌──────────────────┐
                        │     Backend      │
                        │  (Go + Gin) :8080│
                        │  ─ MQTT sub      │
                        │  ─ Validate      │
                        │  ─ Geofence eval │
                        └────┬─────────┬───┘
                             │         │
                  INSERT     │         │   publish geofence.entry
                             ▼         ▼
                     ┌────────────┐  ┌──────────────┐
                     │ PostgreSQL │  │  RabbitMQ    │
                     │  :5432     │  │  :5672       │
                     └─────┬──────┘  └──────┬───────┘
                           │                │
                           │                ▼
                           │         ┌──────────────┐
                           │         │    Worker    │
                           │         │  (consume)   │
                           │         └──────┬───────┘
                           │                │
                           │                ▼
                           │         ┌──────────────┐
                           │         │     DLQ      │
                           │         │  (failures)  │
                           │         └──────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   REST API   │
                    │  consumers   │
                    │  (Postman/   │
                    │   Frontend)  │
                    └──────────────┘
```

---

## Components

| Service | Description | Port |
|---------|-------------|------|
| **backend** | HTTP API + MQTT subscriber + geofence evaluator | 8080 |
| **worker** | RabbitMQ consumer for geofence events | — |
| **publisher** | Mock MQTT publisher (every 2s, route simulation) | — |
| **postgres** | Vehicle location storage | 5432 |
| **rabbitmq** | Event broker + management UI | 5672, 15672 |
| **mosquitto** | MQTT broker | 1883 |
| **adminer** | Web-based PostgreSQL client | 8081 |
| **dozzle** | Web-based log viewer | 8083 |
| **frontend** | React dashboard (bonus deliverable) | 3000 |

---

## Feature Matrix

### Backend (`cmd/backend` + `internal/*`)

| Feature | Implementation | Why it matters |
|---------|---------------|----------------|
| **MQTT subscriber** with worker pool | `internal/adapter/mqtt/subscriber.go` — configurable workers, buffered jobs channel, backpressure via drop+warn | Isolates paho callback from slow DB writes; prevents paho ping timeout |
| **Strict JSON decoding** | `json.Decoder` + `DisallowUnknownFields()` + trailing-data check | Rejects malformed and injection-attempt payloads |
| **Topic ↔ payload cross-validation** | `extractVehicleIDFromTopic(topic)` compared against `loc.VehicleID` | Prevents a publisher from spoofing another vehicle's ID |
| **Plate regex validation** | `^[A-Z]{1,2}\d{1,4}[A-Z]{0,3}$` | Standard Indonesian plate format |
| **Geo-range validation** | Latitude ∈ [-90, 90], Longitude ∈ [-180, 180] | Rejects garbage coordinates before DB write |
| **Timestamp sanity** | Not > 5 min in the future, not > 30 days old | Blocks clock skew and stale replays |
| **Panic isolation per message** | `defer recover()` inside message handler | One bad message cannot kill the worker pool |
| **Idempotent INSERT** | `ON CONFLICT (vehicle_id, timestamp) DO NOTHING` | Safe under at-least-once delivery |
| **Readiness probe** | `/readyz` pings PostgreSQL + checks RabbitMQ health | Enables k8s readiness / LB removal on dependency failure |
| **Liveness probe** | `/healthz` always returns `ok` | Cheap liveness for orchestration |
| **Request ID middleware** | Reads/generates `X-Request-ID`, injects into request-scoped logger | End-to-end tracing across logs |
| **Security headers** | `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN` | Basic hardening against MIME sniffing + clickjacking |
| **Graceful shutdown** | Ordered: HTTP → MQTT (drain) → RabbitMQ → PostgreSQL | No data loss on redeploy |
| **Environment-aware Gin mode** | `gin.ReleaseMode` when `APP_ENV=production` | Reduces log noise in prod |
| **Connection pool** | pgxpool with min/max/lifetime + health check every 30s | Prevents stale connections + thundering herd |
| **Cursor pagination** | `next_cursor = last_ts + 1`, `has_more` flag | Stable, efficient pagination without OFFSET scans |

### Worker (`cmd/worker`)

| Feature | Implementation | Why it matters |
|---------|---------------|----------------|
| **Dedicated consumer binary** | Separate `cmd/worker` from backend | Independent scaling and deployment |
| **Automatic topology declaration** | Exchange + queue + DLX + bindings declared on startup | Worker does not depend on the publisher having run first |
| **QoS prefetch = 1** | `ch.Qos(1, 0, false)` | Fair dispatch — no worker hogs messages |
| **Nack → DLQ** | `msg.Nack(false, false)` on malformed payload | Malformed messages never block the main queue |
| **Ack only after success** | `msg.Ack(false)` post-processing | At-least-once semantics preserved |
| **Graceful context cancellation** | `select { ctx.Done() / delivery }` | Clean shutdown on SIGTERM |
| **Reconnect loop with backoff** | Outer loop with 5s sleep on error | Survives broker restarts |
| **Structured log per event** | Vehicle, geofence, coordinates, timestamp | Easy tracing in Dozzle / Kibana |

### Publisher (`cmd/publisher`)

| Feature | Implementation | Why it matters |
|---------|---------------|----------------|
| **Route simulation** | Exponential approach; advances waypoint when within 30m (`publisher.reach_meters`) |
| **GPS jitter** | ±3 m noise per tick | Avoids perfectly straight lines; mimics real GPS |
| **Waypoint cycling** | Reaches target → advances to next waypoint | Produces repeated geofence entries for testing |
| **Configurable via env only** | `TJ_MQTT_BROKER`, `TJ_VEHICLE_ID`, `TJ_PUBLISH_INTERVAL`, `TJ_REACH_METERS`, `TJ_ROUTE` | Decoupled from backend config |
| **QoS 1 publish** | `c.Publish(topic, 1, false, payload)` | At-least-once delivery |
| **Auto-reconnect** | Paho `SetAutoReconnect(true)` + retry | Survives broker restarts |
| **Signal handling** | SIGINT/SIGTERM → clean exit | No zombie publishers |

### MQTT Adapter (`internal/adapter/mqtt`)

| Feature | Implementation |
|---------|---------------|
| **Topic pattern** | `/fleet/vehicle/+/location` (QoS 1, CleanSession=false) |
| **Payload limit** | 1024 bytes max |
| **Unknown-field rejection** | `DisallowUnknownFields` |
| **Trailing-data rejection** | Second `Decode` + `io.EOF` check |
| **Payload ↔ topic check** | Reject when mismatch |
| **Worker pool** | 4 goroutines (default, `mqtt.worker_pool`) |
| **Job buffer** | 1000 messages (default, `mqtt.job_buffer`) |
| **Backpressure** | Drop + warn when buffer full |
| **Graceful unsubscribe** | Wait up to 2s, then disconnect |
| **Panic recovery** | Per-message recover in handler wrapper |

### Geofence State (`internal/adapter/memory`)

| Feature | Implementation |
|---------|---------------|
| **State-based, not time-based** | Only fires `entry` on outside → inside transition |
| **Collision-free key** | `vehicleID + "\x00" + geofenceID` (NUL byte separator) |
| **Concurrency-safe** | `sync.Mutex` around map access |
| **Reset / rollback** | `Reset` used on publish failure so next tick retries |
| **Multi-vehicle, multi-fence** | Independent state per `(vehicle, fence)` pair |
| **Race detector clean** | Concurrent access test with `-race` |

### PostgreSQL Adapter (`internal/adapter/postgres`)

| Feature | Implementation |
|---------|---------------|
| **Connection pool** | pgxpool, configurable min/max/lifetime |
| **Health check** | `Ping(ctx)` used by `/readyz` |
| **Idempotent write** | `ON CONFLICT (vehicle_id, timestamp) DO NOTHING` |
| **Latest lookup** | Index `(vehicle_id, timestamp DESC)` + `LIMIT 1` |
| **History range scan** | Composite index on `(vehicle_id, timestamp)` |
| **Cursor pagination** | Exclusive `timestamp > cursor` |
| **Sorted ascending** | `ORDER BY timestamp ASC` for time-series plotting |
| **Non-nil slices** | Empty result returns `[]`, not `null` (JSON contract) |

### RabbitMQ Publisher (`internal/adapter/rabbitmq/publisher.go`)

| Feature | Implementation |
|---------|---------------|
| **Publisher confirms** | Waits for broker ack with 5s timeout |
| **Persistent delivery** | `DeliveryMode: Persistent` |
| **Auto-reconnect** | Checks `conn.IsClosed()` before each publish |
| **Mutex-protected publish** | Prevents race with reconnect |
| **DLX declaration** | Idempotent `fanout` exchange + bound DLQ |
| **Health check** | `IsHealthy()` used by `/readyz` |
| **Idempotent Close** | `sync/atomic` + `CompareAndSwap` — safe to call multiple times |

### RabbitMQ Consumer (`internal/adapter/rabbitmq/consumer.go`)

| Feature | Implementation |
|---------|---------------|
| **Self-sufficient topology** | Declares exchange, queue, DLX, DLQ on startup |
| **QoS prefetch 1** | Fair dispatch across workers |
| **Nack → DLQ** | Malformed JSON routed to DLQ |
| **Ack on success** | At-least-once preserved |
| **Context-aware consume** | Clean shutdown via `ctx.Done()` |

### Retry (`pkg/retry`)

| Feature | Implementation |
|---------|---------------|
| **Exponential backoff** | 500ms → 30s (capped) |
| **Max attempts** | 10 per connection (configurable via caller) |
| **Jitter** | 0–50% of current backoff |
| **Context-aware** | Returns `ctx.Err()` on cancellation |
| **Injectable sleep** | `sleepFn` package variable → testable without real delays |
| **Attempt guard** | `maxAttempts <= 0` returns error immediately |
| **Wrap last error** | `errors.Is` works on returned error |

### Geo (`pkg/geo`)

| Feature | Implementation |
|---------|---------------|
| **Haversine distance** | Great-circle distance in meters |
| **Earth radius** | `6371000.0` m (WGS84 mean) |
| **Pure math** | Sin/Cos/Atan2 — no heap allocation on the hot path |
| **Tested** | Unit tested with boundary checks |

### Frontend (`frontend/`)

| Feature | Implementation |
|---------|---------------|
| **Real-time map** | Leaflet + OpenStreetMap with live marker |
| **Route polyline** | Blue line connecting history points |
| **Geofence overlay** | Orange circles from `/geofences` endpoint |
| **Latest location card** | Auto-updates every 2s with relative timestamp |
| **History table** | Sortable with `datetime-local` range picker |
| **Health indicator** | Footer badge polling `/readyz` |
| **Custom `usePolling` hook** | AbortController per tick, tab-visibility aware, anti-overlap |
| **Adaptive interval** | 2s active / 30s hidden (location); 10s / 60s (history) |
| **Debounced vehicle input** | 500ms debounce, uppercase normalization |
| **Immediate refresh** | On vehicle change and range change (no waiting for poll) |
| **Error boundary** | Per-section isolation with reload/reset actions |
| **Skeleton loading** | Shimmer placeholders while fetching |
| **Security headers via nginx** | `nosniff`, `SAMEORIGIN`, `Referrer-Policy` |
| **SPA fallback** | `try_files ... /index.html` in nginx |
| **API proxy** | nginx rewrites `/api/*` → backend, no CORS config needed |
| **Multi-stage build** | Node builder + nginx runtime, minimal final image |

---

## Quick Start

### Prerequisites

- Docker Engine 24+
- Docker Compose v2+
- Make (optional, but recommended)

### Run

```bash
# 1. Copy environment file
cp deploy/.env.example deploy/.env

# 2. Start everything (build + run)
make up

# 3. Wait ~30s for all services to become healthy
make ps
```

Or without Make:

```bash
docker compose -f deploy/docker-compose.yml up --build -d
```

### Stop

```bash
make down            # stop + remove volumes
# or
docker compose -f deploy/docker-compose.yml down -v
```

### Service URLs

| Service | URL | Credentials |
|---------|-----|-------------|
| Backend API | http://localhost:8080 | — |
| Frontend | http://localhost:3000 | — |
| RabbitMQ UI | http://localhost:15672 | `fleet` / `fleet_secret` |
| Adminer (PG UI) | http://localhost:8081 | `fleet` / `fleet_secret` |
| Dozzle (logs) | http://localhost:8083 | — |

---

## API Endpoints

### `GET /healthz` — Liveness

```bash
curl http://localhost:8080/healthz
```

Response `200 OK`:
```
ok
```

### `GET /readyz` — Readiness (checks PostgreSQL + RabbitMQ)

```bash
curl http://localhost:8080/readyz
```

Response `200 OK`:
```json
{"status":"ready"}
```

Response `503 Service Unavailable`:
```json
{"status":"degraded","postgres":"..."}
```

### `GET /geofences` — List active geofences

Returns all geofences from `configs/config.yaml`.

```bash
curl http://localhost:8080/geofences
```

Response `200 OK`:
```json
[
  {
    "id": "monas",
    "name": "Monas",
    "latitude": -6.175392,
    "longitude": 106.827153,
    "radius_meters": 50
  },
  {
    "id": "blok_m",
    "name": "Blok M",
    "latitude": -6.2444,
    "longitude": 106.8006,
    "radius_meters": 50
  }
]
```

### `GET /vehicles/{vehicle_id}/location` — Latest location

```bash
curl http://localhost:8080/vehicles/B1234XYZ/location
```

Response `200 OK`:
```json
{
  "vehicle_id": "B1234XYZ",
  "latitude": -6.2088,
  "longitude": 106.8456,
  "timestamp": 1730000000
}
```

Error responses:

| Status | Body | Cause |
|--------|------|-------|
| `404` | `{"error":"vehicle not found"}` | No data for this vehicle |
| `400` | `{"error":"invalid vehicle id"}` | Invalid plate format |
| `500` | `{"error":"internal error"}` | Backend failure |

### `GET /vehicles/{vehicle_id}/history?start=<unix>&end=<unix>` — History

```bash
# Last 1 hour
START=$(($(date +%s) - 3600))
END=$(date +%s)
curl "http://localhost:8080/vehicles/B1234XYZ/history?start=$START&end=$END"
```

Response `200 OK`:
```json
{
  "vehicle_id": "B1234XYZ",
  "count": 1800,
  "has_more": false,
  "next_cursor": 1730000001,
  "data": [
    {
      "vehicle_id": "B1234XYZ",
      "latitude": -6.2088,
      "longitude": 106.8456,
      "timestamp": 1730000000
    }
  ]
}
```

Query parameters:

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `start` | int64 | ✅ | Unix timestamp start (seconds) |
| `end` | int64 | ✅ | Unix timestamp end (seconds) |
| `cursor` | int64 | ❌ | Exclusive cursor for pagination |
| `limit` | int | ❌ | Max rows (default: 1000, max: 10000) |

Error responses:

| Status | Body | Cause |
|--------|------|-------|
| `400` | `{"error":"start and end query parameters are required"}` | Missing param |
| `400` | `{"error":"start must be a valid unix timestamp"}` | Non-numeric |
| `400` | `{"error":"validation error: end must be >= start"}` | `end < start` |
| `400` | `{"error":"validation error: range exceeds N seconds"}` | Range > `max_range_seconds` |

### Pagination

History uses **cursor-based pagination**:
1. First request: `?start=X&end=Y&limit=1000` → response with `has_more: true`, `next_cursor: Z`
2. Next request: `?start=X&end=Y&limit=1000&cursor=Z` → next page
3. Repeat until `has_more: false`

The cursor is **exclusive**: returns records with `timestamp > cursor`.

---

## Testing

### Unit tests

```bash
# All tests with race detector
make test

# Coverage report (HTML, opens in browser)
make test-cover

# Coverage summary only
make coverage

# Fail if total coverage is below threshold
make coverage-check
```

### Coverage by package

Run `go test ./... -cover -race` to verify locally. Expected coverage
(after applying the recommended tests):

| Package | Target Coverage | Notes |
|---------|----------------|-------|
| `internal/adapter/memory` | 100.0% | Geofence state store |
| `internal/domain` | 100.0% | Validation + plate regex |
| `internal/transport/http` | ≥96% | Handlers, middleware, router |
| `internal/usecase` | 100% | Geofence state machine + history pagination |
| `pkg/geo` | 100.0% | Haversine distance |
| `pkg/logger` | 100.0% | zerolog wrapper |
| `pkg/retry` | 100.0% | Exponential backoff + jitter |
| `internal/adapter/mqtt` | ≥33% | Payload parser (network functions verified end-to-end) |
| `internal/config` | (varies) | Viper loader |
| `internal/adapter/postgres` | 0.0% | Requires integration test (real PostgreSQL) |
| `internal/adapter/rabbitmq` | 0.0% | Requires integration test (real RabbitMQ) |
| `cmd/backend` | 0.0% | Entry point (industry-standard: excluded) |
| `cmd/worker` | 0.0% | Entry point (excluded) |
| `cmd/publisher` | 0.0% | Entry point (excluded) |

**Coverage philosophy:**

- Pure business logic (`domain`, `usecase`, `pkg/*`) is fully unit-tested.
- Network adapters (`postgres`, `rabbitmq`, `mqtt`) rely on integration
  tests via `docker compose up`.
- Entry points (`cmd/*`) are wired and verified end-to-end, not unit-tested.
- All tests run with `-race` to catch concurrency bugs.

### End-to-end verification

See [End-to-End Verification](#end-to-end-verification) below.

### Postman

Import `postman_collection.json` into Postman. Run the whole collection
to verify all endpoints.

---

## MQTT

### Topic

```
/fleet/vehicle/{vehicle_id}/location
```

### Payload

```json
{
  "vehicle_id": "B1234XYZ",
  "latitude": -6.2088,
  "longitude": 106.8456,
  "timestamp": 1730000000
}
```

### Subscription

The backend subscribes to `/fleet/vehicle/+/location` at **QoS 1**.

### Validation

The backend rejects messages when:
- Payload is empty, oversized (> 1024 bytes), or invalid JSON
- Unknown fields present (`DisallowUnknownFields`)
- `vehicle_id` in topic ≠ `vehicle_id` in payload
- `vehicle_id` doesn't match Indonesian plate regex `^[A-Z]{1,2}\d{1,4}[A-Z]{0,3}$`
- `latitude` outside `[-90, 90]` or `longitude` outside `[-180, 180]`
- `timestamp` ≤ 0, more than 5 minutes in the future, or older than 30 days

### Test MQTT

```bash
# Subscribe to see real-time messages
make mqtt-sub

# Or directly:
docker exec -it fleet-mosquitto \
  mosquitto_sub -h localhost -t '/fleet/vehicle/+/location' -v
```

---

## RabbitMQ

### Topology

| Setting | Value |
|---------|-------|
| Exchange | `fleet.events` (type: `topic`, durable) |
| Queue | `geofence_alerts` (durable) |
| Routing key | `geofence.entry` |
| Dead Letter Exchange | `fleet.events.dlx` (type: `fanout`) |
| Dead Letter Queue | `geofence_alerts.dlq` |

### Event payload

Published **only on entry** into a geofence (outside → inside transition),
not repeatedly while inside.

```json
{
  "vehicle_id": "B1234XYZ",
  "event": "geofence_entry",
  "location": {
    "latitude": -6.1754,
    "longitude": 106.8272
  },
  "timestamp": 1730000000
}
```

### Consumer behavior

- **QoS**: prefetch = 1
- Malformed JSON → `nack(requeue=false)` → routed to DLQ
- Valid messages → `ack`
- Consumer auto-reconnects via exponential backoff

### Inspecting the queue

Because the worker consumes + acks in real time, the queue is often empty
during normal operation. To inspect messages:

```bash
# 1. Stop the worker to let events accumulate
docker stop fleet-worker

# 2. Wait for the publisher to cross Monas or Blok M (~2 min)
# 3. Check queue size
docker exec fleet-rabbitmq rabbitmqctl list_queues name messages

# 4. Inspect a message via the management UI:
#    Queues → geofence_alerts → Get messages
#    Ack Mode: "Nack message requeue true" (safe, non-destructive)
#    Messages: 1

# 5. Restart the worker
docker start fleet-worker
```

### Management UI

http://localhost:15672 (`fleet` / `fleet_secret`)

---

## Configuration

Config is loaded from `configs/config.yaml`, then overridden by env vars
prefixed with `TJ_`. Dots in YAML keys become underscores in env vars.

| YAML | Env | Default |
|------|-----|---------|
| `app.env` | `TJ_APP_ENV` | `development` |
| `app.log_level` | `TJ_APP_LOG_LEVEL` | `info` |
| `http.port` | `TJ_HTTP_PORT` | `8080` |
| `http.read_timeout` | `TJ_HTTP_READ_TIMEOUT` | `10s` |
| `http.write_timeout` | `TJ_HTTP_WRITE_TIMEOUT` | `10s` |
| `http.idle_timeout` | `TJ_HTTP_IDLE_TIMEOUT` | `60s` |
| `postgres.host` | `TJ_POSTGRES_HOST` | `postgres` |
| `postgres.port` | `TJ_POSTGRES_PORT` | `5432` |
| `postgres.user` | `TJ_POSTGRES_USER` | `fleet` |
| `postgres.password` | `TJ_POSTGRES_PASSWORD` | `fleet_secret` |
| `postgres.dbname` | `TJ_POSTGRES_DBNAME` | `fleet_db` |
| `postgres.sslmode` | `TJ_POSTGRES_SSLMODE` | `disable` |
| `postgres.max_open_conns` | `TJ_POSTGRES_MAX_OPEN_CONNS` | `25` |
| `postgres.min_conns` | `TJ_POSTGRES_MIN_CONNS` | `5` |
| `postgres.conn_max_lifetime` | `TJ_POSTGRES_CONN_MAX_LIFETIME` | `30m` |
| `mqtt.broker` | `TJ_MQTT_BROKER` | `tcp://mosquitto:1883` |
| `mqtt.client_id` | `TJ_MQTT_CLIENT_ID` | `fleet-management-system-backend` |
| `mqtt.topic` | `TJ_MQTT_TOPIC` | `/fleet/vehicle/+/location` |
| `mqtt.qos` | `TJ_MQTT_QOS` | `1` |
| `mqtt.keep_alive` | `TJ_MQTT_KEEP_ALIVE` | `30s` |
| `mqtt.connect_timeout` | `TJ_MQTT_CONNECT_TIMEOUT` | `10s` |
| `mqtt.worker_pool` | `TJ_MQTT_WORKER_POOL` | `4` |
| `mqtt.job_buffer` | `TJ_MQTT_JOB_BUFFER` | `1000` |
| `mqtt.clean_session` | `TJ_MQTT_CLEAN_SESSION` | `false` |
| `rabbitmq.url` | `TJ_RABBITMQ_URL` | `amqp://fleet:fleet_secret@rabbitmq:5672/` |
| `rabbitmq.exchange` | `TJ_RABBITMQ_EXCHANGE` | `fleet.events` |
| `rabbitmq.exchange_type` | `TJ_RABBITMQ_EXCHANGE_TYPE` | `topic` |
| `rabbitmq.queue` | `TJ_RABBITMQ_QUEUE` | `geofence_alerts` |
| `rabbitmq.routing_key` | `TJ_RABBITMQ_ROUTING_KEY` | `geofence.entry` |
| `rabbitmq.dlx_exchange` | `TJ_RABBITMQ_DLX_EXCHANGE` | `fleet.events.dlx` |
| `rabbitmq.dlq_queue` | `TJ_RABBITMQ_DLQ_QUEUE` | `geofence_alerts.dlq` |
| `geofence.items` | — | 2 fences (Monas, Blok M) |
| `history.max_range_seconds` | — | `9999999999` (unlimited for demo) |
| `history.default_limit` | — | `1000` |
| `history.max_limit` | — | `10000` |
| `publisher.vehicle_id` | — | `B1234XYZ` |
| `publisher.interval` | — | `2s` |
| `publisher.reach_meters` | — | `30` |

### Geofence items

```yaml
geofence:
  items:
    - id: monas
      name: "Monas"
      latitude: -6.175392
      longitude: 106.827153
      radius_meters: 50
    - id: blok_m
      name: "Blok M"
      latitude: -6.2444
      longitude: 106.8006
      radius_meters: 50
```

---

## Project Structure

```
fleet-management-system/
├── cmd/
│   ├── backend/main.go          # API + MQTT subscriber + geofence
│   ├── publisher/main.go        # Mock GPS publisher
│   └── worker/main.go           # RabbitMQ consumer
├── internal/
│   ├── adapter/
│   │   ├── memory/              # In-memory geofence state store
│   │   ├── mqtt/                # MQTT subscriber + payload parser
│   │   ├── postgres/            # Location repository
│   │   └── rabbitmq/            # AMQP publisher + consumer
│   ├── config/                  # Viper-based loader
│   ├── domain/                  # Pure entities + interfaces
│   ├── transport/http/          # Gin handlers, middleware, router
│   └── usecase/                 # Business logic
├── pkg/
│   ├── geo/                     # Haversine distance
│   ├── logger/                  # zerolog wrapper
│   └── retry/                   # Exponential backoff + jitter
├── configs/
│   └── config.yaml
├── deploy/
│   ├── docker-compose.yml
│   ├── Dockerfile               # Multi-stage, non-root
│   ├── .env.example
│   ├── mosquitto/
│   │   └── mosquitto.conf
│   ├── postgres/
│   │   └── init.sql
│   └── rabbitmq/
│       └── rabbitmq.conf
├── frontend/                    # React dashboard (bonus)
├── Makefile
├── postman_collection.json
└── README.md
```

---

## End-to-End Verification

```bash
# 1. Start the full stack
make up

# 2. Check all services healthy
make ps
# Expect: postgres, rabbitmq, mosquitto, backend, worker, publisher, frontend

# 3. Watch MQTT messages arrive (Ctrl+C to exit)
make mqtt-sub

# 4. Watch PostgreSQL rows increase every ~2s
watch -n 2 'make db-count'

# 5. Verify latest location via API
curl -s http://localhost:8080/vehicles/B1234XYZ/location | jq

# 6. Verify history (last 1 hour)
START=$(($(date +%s) - 3600))
curl -s "http://localhost:8080/vehicles/B1234XYZ/history?start=$START&end=$(date +%s)" | jq '.count'

# 7. Verify geofence events reached RabbitMQ
make rabbit-queues
# Expect: geofence_alerts  N messages

# 8. Verify worker consumed them
make logs-worker | tail -20
# Expect log lines: "geofence event consumed"

# 9. Open frontend dashboard
open http://localhost:3000
```

### Expected results

| Step | Expected |
|------|----------|
| `make ps` | All services `Up` (healthy where applicable) |
| `make mqtt-sub` | JSON payload every ~2 seconds |
| `make db-count` | Count increases by ~1 every 2s |
| `GET /location` | Latest JSON with `latitude`, `longitude`, `timestamp` |
| `GET /history` | `count > 0`, `data` sorted ascending |
| `make rabbit-queues` | `geofence_alerts` queue with message count |
| `make logs-worker` | "geofence event consumed" logged once per event |
| Frontend | Live map, latest location card, history table |

---

## Production Considerations

**Correctness & reliability**

- ✅ **Graceful shutdown** — ordered draining: HTTP → MQTT → RabbitMQ → PostgreSQL
- ✅ **Idempotent writes** — unique constraint on `(vehicle_id, timestamp)`
- ✅ **Dead Letter Queue** — malformed messages routed to DLQ, never block main
- ✅ **Publisher confirms** — AMQP waits for broker ack before returning
- ✅ **Retry with exponential backoff + jitter** — for all infrastructure connections
- ✅ **Auto-reconnect** — MQTT (paho) and RabbitMQ (custom) reconnect on drop
- ✅ **State-based geofence** — no event spam, only outside → inside transitions
- ✅ **MQTT topic ↔ payload cross-validation** — anti-spoofing at ingest
- ✅ **Strict JSON** (`DisallowUnknownFields`) — no silent field injection
- ✅ **Panic isolation per message** — one bad payload can't kill the worker pool

**Observability**

- ✅ **Structured logging** (zerolog JSON) with request IDs (`X-Request-ID`)
- ✅ **Health + readiness** endpoints (`/healthz`, `/readyz`)
- ✅ **Dozzle log viewer** in Docker Compose
- ✅ **RabbitMQ management UI** with queue + DLX visibility

**Security**

- ✅ **Non-root container user** (UID 1000)
- ✅ **Multi-stage Docker build** with `-trimpath -ldflags="-s -w"`
- ✅ **Security headers** in nginx (`nosniff`, `SAMEORIGIN`, `Referrer-Policy`)
- ✅ **No credentials in frontend bundle** (all API access via nginx proxy)
- ✅ **Environment-aware Gin mode** (debug off in production)

**Performance**

- ✅ **Connection pooling** (pgxpool with min/max/lifetime)
- ✅ **Separate indexes** for latest lookup and history range scans
- ✅ **Cursor pagination** — no OFFSET scans
- ✅ **Worker pool** for MQTT message processing
- ✅ **Resource limits** in Docker Compose (CPU + memory per service)
- ✅ **Race-detector-clean tests** — `go test -race ./...` passes

**Developer experience**

- ✅ **Makefile targets** for all common operations
- ✅ **Single `make up`** to start everything
- ✅ **Adminer + Dozzle** for zero-install DB / log inspection
- ✅ **Postman collection** ready to import

### Scaling notes (for future)

- **Horizontal scale**: multiple `backend` instances share MQTT via broker
  (fan-out with shared subscriptions) and share geofence state via Redis
  (currently in-memory, single-instance)
- **Time-series storage**: consider TimescaleDB extension for retention
  policies and continuous aggregates
- **Batching**: batch INSERTs to PostgreSQL (flush every N records / M ms)
- **Observability**: Prometheus metrics + OpenTelemetry tracing
- **MQTT broker HA**: Mosquitto cluster or EMQX for production load

---

## Notes on Spec Discrepancies

### Table name

The technical test spec writes `vehicle_loctions` (without the letter 'a').
This implementation uses `vehicle_locations` (correct spelling) because:

1. It matches standard English
2. It's easier to maintain long-term
3. The exact spelling in the spec appears to be a typo

**If strict compatibility is required**, run this inside the PostgreSQL
container after first boot:

```sql
ALTER TABLE vehicle_locations RENAME TO vehicle_loctions;
```

Or modify `deploy/postgres/init.sql` before `make up`.

### Timestamp example

The spec's example uses `1715003456` (May 2024). This implementation
rejects timestamps older than 30 days, so use current Unix time in
tests, e.g.:

```bash
NOW=$(date +%s)
curl "http://localhost:8080/vehicles/B1234XYZ/history?start=$((NOW - 3600))&end=$NOW"
```

### `history.max_range_seconds` and demo

For this technical test, `history.max_range_seconds` is set to a very large
value in `configs/config.yaml` so wide-range queries work out of the box:

```yaml
history:
  max_range_seconds: 9999999999   # effectively unlimited (demo)
  default_limit: 1000
  max_limit: 10000
```

The choice of `9999999999` here is intentional for demonstration purposes —
it lets reviewers query arbitrary ranges without hitting a `400 Bad Request`.

Or use the pre-computed ranges in the Postman collection.

---

## License

.