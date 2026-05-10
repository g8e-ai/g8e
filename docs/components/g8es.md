---
title: Operator Listen Mode
parent: Components
---

# Operator Listen Mode (Host-Native)

Last Updated: 2026-05-09
Version: v0.3.0

## Overview

Operator Listen Mode is the `g8e.operator` binary running in `--listen` mode as a host process. It serves as the platform's single source of truth for persistence and messaging. The same Go binary that executes commands on operator machines also becomes the central data bus when started with `--listen`.

**Why host-native?** Docker elimination reduces operational overhead, removes network bridge complexity, and provides direct access to host resources. The listen mode is a distinct operational mode of the same binary, not a separate component.

**Zero C dependencies.** Uses only Go's standard library and pure-Go implementations (e.g., `modernc.org/sqlite`). This ensures the binary is truly static and cross-compiles easily to any target architecture without requiring a C toolchain or shared libraries.

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

**Data Flow:** Dashboard and Engine use HTTP for document store, KV operations, and blob storage. WebSocket is used for pub/sub messaging. All communication occurs on `localhost` (ports 9000/9001).

**Persistence:** The single database file at `.g8e/data/g8e.db` contains all platform state.

## Transport Summary

| Concern | Transport | Why |
|---------|-----------|-----|
| **Document store** | HTTP | Request/response semantics; status codes carry errors natively; authenticated via `X-Internal-Auth` header |
| **KV store** | HTTP | Stateless; each operation is independent; authenticated via `X-Internal-Auth` header |
| **Blob store** | HTTP | Large binary data; streamed via standard HTTP; authenticated via `X-Internal-Auth` header |
| **SSE event buffer** | HTTP | Legacy ring buffer for reconnection replay (currently unused by g8ed) |
| **Pub/Sub** | WebSocket | Server-push required; long-lived connection; no polling. Supports HTTP publish. |
| **Operator Binaries**| HTTP | Served from the blob store under namespace `operator-binary` (`GET /blob/operator-binary/linux-{arch}`) |

## Runtime Lifecycle

The operator listen process is managed by `./g8e` (via `scripts/core/build.sh`):

1.  **Start the listen server** as a background host process.
2.  **Binary Distribution:** Operator binaries for all architectures are built and synced to the internal blob store on startup.
3.  **Bootstrap:** Secrets (tokens, keys, CA certs) are read from `.g8e/ssl`.

**Why background upload?** This keeps process startup fast — the health check returns before the uploads complete, allowing other services (g8ed, g8ee) to begin connecting immediately. The upload runs as a fire-and-forget background job.

### Operator Binary Distribution

The build script cross-compiles the operator binary for three architectures (amd64, arm64, 386) with UPX compression. These binaries are uploaded to the blob store under namespace `operator-binary` on every listen mode startup.

**Why bake binaries into the image?** This enables remote operator deployment without external dependencies. g8ed can stream the appropriate architecture from the blob store when deploying operators to new machines.

### CLI

```bash
# Start in listen mode

Last Updated: 2026-05-07
Version: v0.2.0
g8e.operator --listen

# With custom ports

Last Updated: 2026-05-07
Version: v0.2.0
g8e.operator --listen --wss-listen-port 9001 --http-listen-port 9000 -l debug
```

## Internal Authentication

operator requires an internal authentication token for all API access except the health check and initial bootstrap paths.

**Why internal auth?** The platform components (g8ed, g8ee, operators) run in a trusted network but still require authentication to prevent accidental misconfiguration and provide defense in depth. The token is a shared secret passed via the `X-Internal-Auth` header or WebSocket `token` query parameter.

### Bootstrap Bypass

During first-start (before `internal_auth_token` is initialized), the following paths are allowed without a token to enable platform bootstrap:

- `GET/PUT /db/settings/platform_settings` - Allows seeding the initial settings document
- `ANY /kv/*` - Enables coordination during bootstrap
- `ANY /ws/*` - Allows WebSocket connections for pub/sub
- `GET /ssl/ca.crt` - Allows Operators to fetch the CA certificate (Note: This is allowed by auth middleware but must be served by the platform)

### Credentials Management

On first start, `CertStore.EnsureCerts` and `SecretManager` generate:

| Artifact | Path (in `--ssl-dir`) | Algorithm | Validity |
|----------|----------------------|-----------|----------|
| CA private key | `ca/ca.key` | ECDSA P-384 | — |
| CA certificate | `ca/ca.crt` | ECDSA P-384, self-signed | 10 years (3650 days) |
| Server private key | `server.key` | ECDSA P-384 | — |
| Server certificate | `server.crt` | Signed by platform CA | 90 days |
| **Internal Auth Token** | `internal_auth_token` | 32-byte random hex | — |

The `internal_auth_token` is written to the SSL volume and also stored in the `settings/platform_settings` document for redundancy.

## TLS / Certificate Management

The server certificate includes the following SANs:
- **DNS:** `g8e.local`, `localhost`
- **IP:** `127.0.0.1` plus any host IPs detected at runtime

The `ca.crt` mirror at the bootstrap root is what g8ed, g8ee, and field Operators consume.

**Why auto-generated certificates?** This eliminates the need for external certificate management during initial deployment. The platform CA is self-signed and trusted only within the platform infrastructure. The 90-day server certificate validity balances security with operational simplicity — it renews automatically on startup if expiring within 30 days.

### External Certificate Override

Pass `--tls-cert` and `--tls-key` to supply an externally-managed certificate for production deployments that require corporate PKI integration.

## Docker Compose

```yaml
operator:
  build:
    context: .
    dockerfile: ./components/operator/Dockerfile
  container_name: operator
  restart: unless-stopped
  volumes:
    - operator-data:/data
    - operator-ssl:/ssl
  networks:
    g8e-network:
      aliases:
        - operator
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

The health check is the only endpoint that does not require authentication. It returns `503 Service Unavailable` during initialization or if `platform_settings` are missing.

### Operator Binary Distribution

Operator binaries are stored in the blob store under namespace `operator-binary` and streamed on demand. The build script cross-compiles binaries for each supported architecture and uploads them to the blob store on listen mode startup.

```
GET /blob/operator-binary/linux-amd64  → Stream linux/amd64 binary
GET /blob/operator-binary/linux-arm64  → Stream linux/arm64 binary
GET /blob/operator-binary/linux-386    → Stream linux/386 binary
```

### Document Store

The document store provides collection-based JSON storage with automatic timestamp management.

```
GET    /db/{collection}/{id}       → Get document
PUT    /db/{collection}/{id}       → Set (create/replace) document
PATCH  /db/{collection}/{id}       → Update (merge fields) document
DELETE /db/{collection}/{id}       → Delete document
POST   /db/{collection}/_query     → Query documents
```

**Why collection-based?** This provides a flexible schema where each collection represents a domain entity (users, sessions, operators, cases). The primary key is `(collection, id)`, allowing efficient queries within a collection while keeping all data in a single table.

System fields (`id`, `created_at`, `updated_at`) are managed by operator — clients cannot override them. The `created_at` timestamp is set once on insert and never changes; `updated_at` refreshes on every write.

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

The KV store provides string key/value storage with optional time-to-live (TTL) expiration.

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

**Why a separate KV store?** The document store is optimized for complex queries and structured data. The KV store is optimized for simple key-based lookups with expiration, making it ideal for caching, session state, and coordination primitives. TTL is handled at the storage layer.

`ttl: 0` on `PUT /kv/{key}` means no expiration. Expired keys are cleaned up by a background goroutine every 30 seconds, and are filtered out of results.

### SSE Event Buffer

operator provides a per-session event ring buffer table (`sse_events`). **Note:** This is currently a legacy component. In the current architecture, `g8ee` pushes events to `g8ed` via HTTP, and `g8ed` delivers them to local SSE connections.

```
DELETE /db/_sse_events         → Wipe all SSE events
GET    /db/_sse_events/count   → Count rows
```

### Blob Store

The blob store provides raw binary storage keyed by namespace and ID.

```
PUT    /blob/{namespace}/{id}       → Store blob (raw bytes, Content-Type header required, optional X-Blob-TTL seconds)
GET    /blob/{namespace}/{id}       → Retrieve blob (streams raw bytes with original Content-Type)
DELETE /blob/{namespace}/{id}       → Delete single blob
GET    /blob/{namespace}/{id}/meta  → Metadata only (no data)
DELETE /blob/{namespace}            → Delete all blobs in namespace
```

**Why a separate blob store?** Storing large binary data in the document store would bloat the database and impact query performance. The blob store keeps binaries in a separate table with efficient streaming.

**Constraints:**
- **Max size**: 15MB per blob (hard cap at the transport layer)
- **TTL**: Supports expiration via `X-Blob-TTL` header (seconds)

### Pub/Sub

The pub/sub broker provides real-time messaging via WebSocket with optional HTTP publish for fire-and-forget scenarios.

HTTP publish (fire-and-forget):
```
POST /pubsub/publish  {"channel": "cmd:op1:sess1", "data": {...}}
→ {"receivers": N}
```

WebSocket (subscribe + publish):
```
wss://localhost:9001/ws/pubsub?token={token}

→ {"action": "subscribe",   "channel": "results:op1:sess1"}
→ {"action": "psubscribe",  "channel": "heartbeat:*"}
→ {"action": "publish",     "channel": "cmd:op1:sess1", "data": {...}}
← {"type": "subscribed",    "channel": "results:op1:sess1"}
← {"type": "message",       "channel": "results:op1:sess1", "data": {...}}
← {"type": "pmessage",      "channel": "heartbeat:op1", "pattern": "heartbeat:*", "data": {...}}
```

**Why WebSocket for pub/sub?** Server-push is required for real-time command output and heartbeat streams. Pattern matching supports Redis-style globbing (`*`, `?`).

## Client Libraries

### g8ed (Node.js)

g8ed uses purpose-built clients in `components/g8ed/services/clients/` and `components/g8ed/services/platform/`:

| Client | File | Transport | Scope |
|--------|------|-----------|-------|
| `OperatorDocumentClient` | `operator_document_client.js` | HTTP | Document store CRUD (`/db/...`) |
| `KVCacheClient` | `operator_kv_cache_client.js` | HTTP | KV store operations (`/kv/...`) |
| `OperatorPubSubClient` | `operator_pubsub_client.js` | WebSocket | Pub/sub messaging (`/ws/pubsub`) |
| `OperatorBlobClient` | `operator_blob_client.js` | HTTP | Blob store operations (`/blob/...`) |
| `OperatorHttpClient` | `operator_http_client.js` | HTTP | Base client for HTTP operations |
| `InternalHttpClient` | `internal_http_client.js` | HTTP | General internal service communication |

**Atomicity Warning:** Compound operations (e.g., `increment`, `arrayUnion`) are implemented as read-modify-write cycles over HTTP and are **not atomic**.

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

The canonical schema is defined in `components/g8eo/services/listen/schema.sql` and embedded into the binary via `//go:embed`.

Single database at `.g8e/data/g8e.db` with the following tables:
- `documents` - Collection-based JSON storage
- `kv_store` - Key/value storage with TTL support
- `sse_events` - Per-session event ring buffer (legacy)
- `blobs` - Raw binary storage with namespace isolation

## Management Script

`scripts/data/manage-operator.py` provides read-only inspection of the operator stores via the `store` subcommand.

## Air-Gap Deployment

In a fully air-gapped environment, the platform runs as three processes from a single binary:

```bash
# Terminal 1: Persistence + messaging backbone

Last Updated: 2026-05-07
Version: v0.2.0
./g8e.operator --listen --data-dir ./data

# Terminal 2: Web dashboard

Last Updated: 2026-05-07
Version: v0.2.0
node components/g8ed/server.js

# Terminal 3: AI engine

Last Updated: 2026-05-07
Version: v0.2.0
python components/g8ee/app/main.py
```

**Why this works in air-gap?** The operator binary is self-contained with no C dependencies. All components communicate via localhost HTTP/WebSocket to the listen mode instance. No external network access is required after initial deployment.
