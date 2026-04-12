# g8es — Operator Listen Mode Architecture

## Overview

g8es is the Operator binary running in `--listen` mode. It is the platform's **single source of truth** for persistence and messaging. No new binaries, no new dependencies — the same `g8e.operator` Go binary that runs on user machines also serves as the persistence and messaging backbone when started with `--listen`.

**Zero external dependencies.** The Operator uses Go's standard library `net/http` and `github.com/gorilla/websocket`. It compiles to a single static binary that runs anywhere — Docker, bare metal, air-gapped environments.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    g8e.operator --listen             │
│                                                         │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐ │
│  │ Document     │  │ KV Store     │  │ SSE Event     │ │
│  │ Store        │  │ (with TTL)   │  │ Buffer        │ │
│  │              │  │              │  │               │ │
│  │ /db/:coll/:id│  │ /kv/:key     │  │ /sse/:session │ │
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

g8ed and g8ee both use HTTP for document store and KV operations, and WebSocket for pub/sub. The Operator (in normal mode) connects via WebSocket only for pub/sub.

## Transport Summary

| Concern | Transport | Why |
|---------|-----------|-----|
| **Document store** | HTTP | Request/response semantics; status codes carry errors natively |
| **KV store** | HTTP | Stateless; each operation is independent |
| **Blob store** | HTTP | Large binary data; streamed via standard HTTP |
| **SSE event buffer** | HTTP | Row management via DELETE/GET; table-only for now |
| **Pub/Sub** | WebSocket | Server-push required; long-lived connection; no polling |
| **Operator Binaries**| HTTP | Static binary distribution via `/binary/{os}/{arch}` |

## CLI

```bash
# Start in listen mode (no auth required for health, internal auth for API)
g8e.operator --listen

# With options
g8e.operator --listen --wss-listen-port 9001 --http-listen-port 9000 -l debug
```

## TLS / Certificate Management

g8es manages its own private CA and server certificate when started without explicit TLS flags. All certificate logic lives in `components/g8eo/services/listen/listen_certs.go` (`CertStore`).

This CA is the root of trust for the entire platform:
- g8ed uses the g8es-generated server certificate for browser HTTPS on port 443
- g8ed reads the CA from `/ssl/ca.crt` to power the workstation trust portal
- Field Operators discover the CA locally from the `g8es-ssl` volume, or fetch it over HTTPS from `https://<endpoint>:9000/ssl/ca.crt` as a fallback

### Auto-Generated Certificates (default)

On first start, `CertStore.EnsureCerts` generates:

| Artifact | Path (in `--ssl-dir`) | Algorithm | Validity |
|----------|----------------------|-----------|----------|
| CA private key | `ca/ca.key` | ECDSA P-384 | — |
| CA certificate | `ca/ca.crt` | ECDSA P-384, self-signed | 10 years (3650 days) |
| CA cert (mirror) | `ca.crt` | — | — |
| Server private key | `server.key` | ECDSA P-384 | — |
| Server certificate | `server.crt` | Signed by platform CA | 90 days |
| **Internal Auth Token** | `internal_auth_token` | 32-byte random hex | — |
| **Session Encryption Key** | `session_encryption_key` | 32-byte random hex | — |

The `ca.crt` mirror at the ssl root is what g8ed, g8ee, and field Operators consume.

#### Bootstrap Secrets Handling

The `internal_auth_token` and `session_encryption_key` are the critical bootstrap secrets.

- **Authoritative Source**: The files in the SSL volume (`--ssl-dir`) are the absolute source of truth.
- **Generation**: At startup, g8es ensures these exist. If missing, it generates 32-byte random hex values.
- **DB Backup**: Stored in `settings/platform_settings` document. If SSL volume is wiped, g8es restores them from the DB.
- **Enforcement**: All g8es HTTP and WebSocket routes (except `/health`) strictly require `X-Internal-Auth` header.
- **Discovery**: g8ed and g8ee read these files from the shared volume at startup.

The server certificate includes the following SANs:
- **DNS:** `g8e.local`, `localhost`, `g8es`, `g8ee`, `g8ed`
- **IP:** `127.0.0.1` plus any extra IPs passed to `EnsureCerts`

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
    test: ["CMD", "curl", "-f", "-k", "https://localhost:9000/health"]
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

Operator binaries are served directly from the filesystem in g8es.

```
GET /binary/linux/amd64        → Stream linux/amd64 binary
GET /binary/linux/arm64        → Stream linux/arm64 binary
GET /binary/linux/386          → Stream linux/386 binary
```

### Document Store

```
GET    /db/{collection}/{id}       → Get document
PUT    /db/{collection}/{id}       → Set (create/replace) document
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

Supported filter ops: `==`, `!=`, `<`, `>`, `<=`, `>=`.

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

`ttl: 0` on `PUT /kv/{key}` means no expiration. Expired keys are cleaned up by a background goroutine every 30 seconds.

### SSE Event Buffer

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
wss://g8es:9001/ws/pubsub?token={token}

→ {"action": "subscribe",   "channel": "results:op1:sess1"}
→ {"action": "psubscribe",  "channel": "heartbeat:*"}
← {"type": "subscribed",    "channel": "results:op1:sess1"}
← {"type": "message",       "channel": "results:op1:sess1", "data": {...}}
```

Each subscriber has a 4096-message send buffer.

## Client Libraries

### g8ed (Node.js)

g8ed uses three separate purpose-built clients in `components/g8ed/services/clients/`:

| Client | File | Transport | Scope |
|--------|------|-----------|-------|
| `G8esDocumentClient` | `g8es_document_client.js` | HTTP | Document store CRUD (`/db/...`) |
| `KVCacheClient` | `g8es_kv_cache_client.js` | HTTP | KV store operations (`/kv/...`) |
| `G8esPubSubClient` | `g8es_pubsub_client.js` | WebSocket | Pub/sub messaging (`/ws/pubsub`) |

**Atomicity Warning:** Compound operations (e.g., `incr`, `hset`, `rpush`) are implemented as read-modify-write cycles over HTTP and are **not atomic**.

### g8ee (Python)

g8ee uses two clients in `components/g8ee/app/clients/`:

| Client | File | Transport | Scope |
|--------|------|-----------|-------|
| `DBClient` | `db_client.py` | HTTP | Document store CRUD |
| `KVClient` | `kv_client.py` | HTTP + WS | KV and Pub/Sub |

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
