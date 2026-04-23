---
title: g8es
parent: Components
---

# g8es — g8e Data Bus

## Overview

g8es is the Operator binary running in `--listen` mode. It is the platform's **single source of truth** for persistence and messaging. No new binaries, no new dependencies — the same `g8e.operator` Go binary that runs on user machines also serves as the persistence and messaging backbone when started with `--listen`.

**Zero external dependencies.** The Operator uses Go's standard library `net/http` and `github.com/gorilla/websocket`. It compiles to a single static binary that runs anywhere — Docker, bare metal, air-gapped environments.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    g8e.operator --listen             │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐ │
│  │ Document     │  │ KV Store     │  │ Blob          │ │
│  │ Store        │  │ (with TTL)   │  │ Store         │ │
│  │              │  │              │  │               │ │
│  │ /db/:coll/:id│  │ /kv/:key     │  │ /blob/:ns/:id │ │
│  └──────┬───────┘  └──────┬───────┘  └──────┬────────┘ │
│         │                 │                  │          │
│         └─────────┬───────┘──────────────────┘          │
│                   │                                     │
│           ┌───────▼────────┐                            │
│           │  SQLite (WAL)  │                            │
│           │  /data/g8e.db  │                            │
│           └────────────────┘                            │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │ WebSocket Pub/Sub Broker                         │   │
│  │ /ws/pubsub                                       │   │
│  │                                                  │   │
│  │ Channels: cmd:*, results:*, heartbeat:*          │   │
│  │ Supports exact subscribe + glob patterns         │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │ HTTP Publish                                     │   │
│  │ POST /pubsub/publish                             │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
         ▲              ▲                  ▲
         │ HTTP+WS      │ HTTP+WS          │ WebSocket
         │              │                  │
    ┌────┴────┐    ┌────┴────┐        ┌────┴────┐
    │  g8ed   │    │   g8ee   │        │ Operator│
    │ (Node)  │    │ (Python)│        │ (Go)    │
    └─────────┘    └─────────┘        └─────────┘
```
```

g8ed and g8ee both use HTTP for document store, KV operations, and blob storage, and WebSocket for pub/sub. The Operator (in normal mode) connects via WebSocket only for pub/sub.

## Transport Summary

| Concern | Transport | Why |
|---------|-----------|-----|
| **Document store** | HTTP | Request/response semantics; status codes carry errors natively |
| **KV store** | HTTP | Stateless; each operation is independent |
| **Blob store** | HTTP | Large binary data; streamed via standard HTTP |
| **SSE event buffer** | HTTP | Legacy ring buffer for reconnection replay (currently unused by g8ed) |
| **Pub/Sub** | WebSocket | Server-push required; long-lived connection; no polling. Supports HTTP publish. |
| **Operator Binaries**| HTTP | Served from the blob store under namespace `operator-binary` (`GET /blob/operator-binary/linux-{arch}`) |

## CLI

```bash
# Start in listen mode (no auth required for health, internal auth for API)
g8e.operator --listen

# With options
g8e.operator --listen --wss-listen-port 443 --http-listen-port 443 -l debug
```

## Internal Authentication

g8es requires an internal authentication token for all API access except the health check and initial bootstrap paths.

### Bootstrap Bypass

During first-start (before `internal_auth_token` is initialized), the following paths are allowed without a token:
- `GET/PUT /db/settings/platform_settings`
- `ANY /kv/*`
- `ANY /ws/*`
- `GET /ssl/ca.crt`

### Credentials Management

On first start, `CertStore.EnsureCerts` and `SecretManager` generate:

| Artifact | Path (in `--ssl-dir`) | Algorithm | Validity |
|----------|----------------------|-----------|----------|
| CA private key | `ca/ca.key` | ECDSA P-384 | — |
| CA certificate | `ca/ca.crt` | ECDSA P-384, self-signed | 10 years (3650 days) |
| Server private key | `server.key` | ECDSA P-384 | — |
| Server certificate | `server.crt` | Signed by platform CA | 90 days |
| **Internal Auth Token** | `internal_auth_token` | 32-byte random hex | — |

The `internal_auth_token` is written to the SSL volume and also stored in the `settings/platform_settings` document.

## TLS / Certificate Management

The server certificate includes the following SANs:
- **DNS:** `g8e.local`, `localhost`, `g8es`, `g8ee`, `g8ed`
- **IP:** `127.0.0.1` plus any extra IPs passed to `EnsureCerts`

The `ca.crt` mirror at the ssl root is what g8ed, g8ee, and field Operators consume.

### External Certificate Override

Pass `--tls-cert` and `--tls-key` to supply an externally-managed certificate.

## Docker Compose

```yaml
g8es:
  build:
    context: .
    dockerfile: ./components/g8es/Dockerfile
  container_name: g8es
  restart: unless-stopped
  volumes:
    - g8es-data:/data
    - g8es-ssl:/ssl
  networks:
    g8e-network:
      aliases:
        - g8es
  healthcheck:
    test: ["CMD", "curl", "-f", "--cacert", "/ssl/ca.crt", "https://localhost:9000/health"]
    interval: 10s
    timeout: 5s
    retries: 5
    start_period: 60s
```

## API Reference

### Health Check

```
GET /health
← 200 OK  {"status": "ok", "mode": "listen", "version": "<build version>"}
```

### Operator Binary Distribution

Operator binaries are stored in the blob store under namespace `operator-binary` and streamed on demand. The g8es container bakes cross-compiled binaries for each supported architecture at image build time and uploads them to the blob store on startup.

```
GET /blob/operator-binary/linux-amd64  → Stream linux/amd64 binary
GET /blob/operator-binary/linux-arm64  → Stream linux/arm64 binary
GET /blob/operator-binary/linux-386    → Stream linux/386 binary
```

### Document Store

```
GET    /db/{collection}/{id}       → Get document
PUT    /db/{collection}/{id}       → Set (create/replace) document (system fields `id`, `created_at`, `updated_at` are managed by g8es)
PATCH  /db/{collection}/{id}       → Update (merge fields) document
DELETE /db/{collection}/{id}       → Delete document
POST   /db/{collection}/_query     → Query documents
```

Query body:
```json
{
  "filters": [{"field": "status", "op": "==", "value": "active"}],
  "order_by": "created_at DESC",
  "limit": 50
}
```

Supported filter ops: `==`, `!=`, `<`, `>`, `<=`, `>=`. Filters use `json_extract` for deep matching in the `data` column.

### KV Store

```
GET    /kv/{key}              → Get value           → {"value": "..."}
PUT    /kv/{key}              → Set value           {"value": "...", "ttl": 300}
DELETE /kv/{key}              → Delete key
GET    /kv/{key}/_ttl         → Get remaining TTL   → {"ttl": N}
PUT    /kv/{key}/_expire      → Set TTL             {"ttl": 300}
POST   /kv/_keys              → List keys           {"pattern": "session:*"} → {"keys": [...]}
POST   /kv/_scan              → Paginated key scan  {"pattern": "...", "cursor": 0, "count": 100} → {"cursor": N, "keys": [...]}
POST   /kv/_delete_pattern    → Delete by pattern   {"pattern": "cache:user:*"} → {"deleted": N}
```

`ttl: 0` on `PUT /kv/{key}` means no expiration. Expired keys are cleaned up by a background goroutine every 30 seconds, and are filtered out of `GET`, `_keys`, and `_scan` results.

### SSE Event Buffer

g8es provides a per-session event ring buffer table (`sse_events`). **Note:** This is currently a legacy component. In the current architecture, `g8ee` pushes events to `g8ed` via HTTP, and `g8ed` delivers them to local SSE connections (fire-and-forget). The g8es buffer is not used for reconnection replay in the 2026 stack.

```
DELETE /db/_sse_events         → Wipe all SSE events
GET    /db/_sse_events/count   → Count rows
```

### Blob Store

g8es provides a raw binary blob store keyed by namespace and ID.

```
PUT    /blob/{namespace}/{id}       → Store blob (raw bytes, Content-Type header required, optional X-Blob-TTL seconds)
GET    /blob/{namespace}/{id}       → Retrieve blob (streams raw bytes with original Content-Type)
DELETE /blob/{namespace}/{id}       → Delete single blob
GET    /blob/{namespace}/{id}/meta  → Metadata only (no data)
DELETE /blob/{namespace}            → Delete all blobs in namespace
```

**Constraints:**
- **Max size**: 15MB per blob (hard cap)
- **TTL**: Supports expiration via `X-Blob-TTL` header (seconds)

### Pub/Sub

HTTP publish (fire-and-forget):
```
POST /pubsub/publish  {"channel": "cmd:op1:sess1", "data": {...}}
→ {"receivers": N}
```

WebSocket (subscribe + publish):
```
wss://g8es:443/ws/pubsub?token={token}

→ {"action": "subscribe",   "channel": "results:op1:sess1"}
→ {"action": "psubscribe",  "channel": "heartbeat:*"}
→ {"action": "publish",     "channel": "cmd:op1:sess1", "data": {...}}
← {"type": "subscribed",    "channel": "results:op1:sess1"}
← {"type": "message",       "channel": "results:op1:sess1", "data": {...}}
← {"type": "pmessage",      "channel": "heartbeat:op1", "pattern": "heartbeat:*", "data": {...}}
```

Each subscriber has a 4096-message send buffer. Patterns support Redis-style globbing (`*`, `?`).

## Client Libraries

### g8ed (Node.js)

g8ed uses purpose-built clients in `components/g8ed/services/clients/`:

| Client | File | Transport | Scope |
|--------|------|-----------|-------|
| `G8esDocumentClient` | `g8es_document_client.js` | HTTP | Document store CRUD (`/db/...`) |
| `KVCacheClient` | `g8es_kv_cache_client.js` | HTTP | KV store operations (`/kv/...`) |
| `G8esPubSubClient` | `g8es_pubsub_client.js` | WebSocket | Pub/sub messaging (`/ws/pubsub`) |
| `G8esHttpClient` | `g8es_http_client.js` | HTTP | Base client for HTTP operations |
| `InternalHttpClient` | `internal_http_client.js` | HTTP | General internal service communication |

**Atomicity Warning:** Compound operations (e.g., `incr`, `hset`, `rpush`) are implemented as read-modify-write cycles over HTTP and are **not atomic**.

### g8ee (Python)

g8ee uses clients in `components/g8ee/app/clients/`:

| Client | File | Transport | Scope |
|--------|------|-----------|-------|
| `DBClient` | `db_client.py` | HTTP | Document store CRUD |
| `KVCacheClient` | `kv_cache_client.py` | HTTP | KV store operations |
| `PubSubClient` | `pubsub_client.py` | WebSocket | Pub/sub messaging |
| `BlobClient` | `blob_client.py` | HTTP | Blob store operations |
| `InternalHttpClient` | `http_client.py` | HTTP | Base client for internal HTTP calls |

## SQLite Schema

Single database at `/data/g8e.db`. Tables: `documents`, `kv_store`, `sse_events`, and `blobs`.

## Management Script

`scripts/data/manage-g8es.py` provides read-only inspection of the g8es stores via the `store` subcommand.

## Air-Gap Deployment

In a fully air-gapped environment, the deployment is three processes from a single binary:

```bash
# Terminal 1: Persistence + messaging backbone
./g8e.operator --listen --data-dir ./data

# Terminal 2: Web dashboard
node components/g8ed/server.js

# Terminal 3: AI engine
python components/g8ee/app/main.py
```
