# VSODB — Operator Listen Mode Architecture

## Overview

VSODB is the Operator binary running in `--listen` mode. It is the platform's **single source of truth** for persistence and messaging. No new binaries, no new dependencies — the same `g8e.operator` Go binary that runs on user machines also serves as the persistence and messaging backbone when started with `--listen`.

**Zero external dependencies.** The Operator uses `modernc.org/sqlite` (pure Go, zero CGo), Go's standard library `net/http`, and `github.com/gorilla/websocket`. It compiles to a single static binary that runs anywhere — Docker, bare metal, air-gapped environments.

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
│           │  /data/g8e │                            │
│           │  .db           │                            │
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
    │  VSOD   │    │   g8ee   │        │ Operator│
    │ (Node)  │    │ (Python)│        │ (Go)    │
    └─────────┘    └─────────┘        └─────────┘
```

VSOD and g8ee both use HTTP for document store and KV operations, and WebSocket for pub/sub. The Operator (in normal mode) connects via WebSocket only for pub/sub.

## Transport Summary

| Concern | Transport | Why |
|---------|-----------|-----|
| **Document store** | HTTP | Request/response semantics; status codes carry errors natively |
| **KV store** | HTTP | Stateless; each operation is independent |
| **Blob store** | HTTP | Large binary data; streamed via standard HTTP |
| **SSE event buffer** | — | Table defined in schema; management endpoints implemented |
| **Pub/Sub** | WebSocket | Server-push required; long-lived connection; no polling |

## CLI

```bash
# Start in listen mode (no auth required, no outbound connections)
g8e.operator --listen

# With options
g8e.operator --listen --wss-listen-port 443 --http-listen-port 443 -l debug

# With TLS
g8e.operator --listen --tls-cert /path/cert.pem --tls-key /path/key.pem
```

## TLS / Certificate Management

VSODB manages its own private CA and server certificate when started without explicit TLS flags. All certificate logic lives in `components/g8eo/services/listen/listen_certs.go` (`CertStore`).

This CA is not only for internal service trust. It is the root of trust for the entire platform:
- VSOD uses the VSODB-generated server certificate for browser HTTPS on port 443
- VSOD reads the CA from `/vsodb/ssl/ca.crt` to power the workstation trust portal on port 80
- Field Operators discover the CA locally from the `vsodb-ssl` volume when running inside the Docker network (e.g., g8ep at `/vsodb/ca.crt`), or fetch it over HTTPS from `https://<endpoint>/ssl/ca.crt` as a fallback for remote deployments

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

The `ca.crt` mirror at the ssl root is what VSOD, g8ee, and field Operators consume — it is written on every start to ensure it is always present even after volume surgery.

#### Bootstrap Secrets Handling

The `internal_auth_token` and `session_encryption_key` are the critical bootstrap secrets for the platform.

- **Authoritative Source**: The files in the SSL volume (`--ssl-dir`) are the absolute source of truth.
- **Generation**: At startup, VSODB ensures these secret files exist on the volume. If a file is missing, VSODB generates a new random 32-byte hex value and writes it to the volume.
- **DB Backup**: These secrets are also stored in the `components/platform_settings` document in the database. If the SSL volume is wiped but the DB remains, VSODB restores the secret files from the DB. If both are missing, it generates new ones.
- **Enforcement**: All VSODB HTTP and WebSocket routes strictly require the `internal_auth_token` in the `X-Internal-Auth` header (or `token` query parameter for WebSockets).
- **Discovery**: VSOD and g8ee automatically discover these tokens by reading the files from the shared volume at startup.

The server certificate includes the following SANs:
- **DNS:** `g8e.local`, `localhost`, `vsodb`, `vsod`
- **IP:** `127.0.0.1` plus all non-loopback IPv4 addresses detected on the host at startup

On subsequent starts, existing certificates are loaded from disk. The server certificate is renewed automatically when it is within 30 days of expiry. If the CA cannot be loaded, both CA and server certificates are regenerated.

### External Certificate Override

Pass `--tls-cert` and `--tls-key` to supply an externally-managed certificate instead of auto-generation. When external certs are used, `CertStore` is not initialized and the `/ssl/ca.crt` endpoint returns `503 Service Unavailable`.

```bash
g8e.operator --listen --tls-cert /path/cert.pem --tls-key /path/key.pem
```

### CA Certificate Distribution

The platform CA is served over plain HTTP so downstream consumers can fetch it without a chicken-and-egg TLS dependency:

```
GET /ssl/ca.crt   (HTTPS port 443)
← 200 OK  Content-Type: application/x-pem-file
← 503     {"error": "certificates not initialized"}  (external cert mode)
```

VSOD and g8ee also access the CA via the `g8es-ssl` Docker volume, which is mounted read-only at `/vsodb/ssl` on both services. Field Operators use a local-first strategy — scanning well-known volume mount paths before falling back to `https://<endpoint>/ssl/ca.crt`. Inside the Docker network, the CA is discovered locally at `/vsodb/ca.crt` without any network fetch. See [architecture/security.md — CA Trust Bootstrap](../architecture/security.md#ca-trust-bootstrap-and-the-tls-kill-switch) for the full discovery sequence.

For browser users, VSOD turns that same CA into a workstation trust flow:
- `GET https://<host>` serves the public CA trust portal
- `/ca.crt` serves the raw certificate to browsers and mobile devices
- `/install-cert.bat`, `/install-cert.sh`, and `/install-cert-linux.sh` serve OS-specific trust installers that download `/ca.crt` from VSOD and install it locally
- Once trusted, users continue to `https://<host>/setup`

For full security policy — mTLS, per-operator client certificates, certificate revocation, and the TLS kill switch — see [architecture/security.md — SSL/CA Certificate Generation](../architecture/security.md#sslca-certificate-generation-and-handling).

## Docker Compose

```yaml
vsodb:
  build:
    context: .
    dockerfile: ./components/vsodb/Dockerfile
  container_name: g8es
  restart: unless-stopped
  volumes:
    - vsodb-data:/data
    - vsodb-ssl:/ssl
  networks:
    vso-network:
      aliases:
        - vsodb
  healthcheck:
    test: ["CMD", "curl", "-f", "-k", "https://localhost/health"]
    interval: 10s
    timeout: 5s
    retries: 5
    start_period: 60s
```

**Volume layout:** Two separate named volumes:
- `g8es-data` — SQLite DB only, mounted at `/data`. Wiped by `platform reset`.
- `g8es-ssl` — TLS certs only, mounted at `/ssl`. **Never wiped** by `reset` or `wipe` — survives all lifecycle operations except `platform clean`.

**Security:** The container runs as a non-root `g8e` user. VSODB has no external port bindings — it is only reachable within the internal Docker network by VSOD and G8EE. The `g8es-ssl` volume is mounted read-only at `/vsodb/ssl` on VSOD, g8ee, and g8ep so they can read the platform CA and server certificates without direct HTTP. VSOD is the only public-facing service: it converts the VSODB-generated certificates into the browser HTTPS endpoint and the HTTP certificate-trust bootstrap flow.

## API Reference

### Health Check

```
GET /health
← 200 OK  {"status": "ok", "mode": "listen", "version": "<build version>"}
```

### CA Certificate

```
GET /ssl/ca.crt   (HTTPS port 443)
← 200 OK  Content-Type: application/x-pem-file
```

Serves the PEM-encoded platform CA certificate. Available only when VSODB is managing its own certificates (auto-generated mode). Returns `503` if an external TLS certificate was supplied via `--tls-cert`/`--tls-key`.

### Operator Binary Distribution

Operator binaries are stored in the VSODB blob store under the `operator-binary` namespace. All 3 architectures (amd64, arm64, 386) are cross-compiled and UPX-compressed at VSODB image build time and baked into the image at `/opt/operator-binaries/`. On container startup, the entrypoint uploads them to the blob store automatically. The `./g8e operator build` and `./g8e operator build-all` commands in g8ep can override these by uploading fresh builds.

VSOD fetches binaries on demand from the blob store — no local disk cache, no filesystem serving.

```
GET /blob/operator-binary/linux-amd64        → Stream linux/amd64 binary
GET /blob/operator-binary/linux-arm64        → Stream linux/arm64 binary
GET /blob/operator-binary/linux-386          → Stream linux/386 binary
GET /blob/operator-binary/linux-amd64/meta   → Metadata only (availability check)
```

Example:
```
GET /blob/operator-binary/linux-amd64
← 200 OK  Content-Type: application/octet-stream  (binary stream)
← 404     {"error": "blob not found"}
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

Supported filter ops: `==`, `!=`, `<`, `>`, `<=`, `>=`. Filters use `json_extract` against the JSON `data` column. `order_by` is `"field"` or `"field DESC"`. `limit: 0` means no limit.

`PATCH` returns the merged document. `PUT` (upsert) auto-sets `created_at` (if absent) and `updated_at`, and injects `id` into the stored JSON. It removes `id`, `created_at`, and `updated_at` from the input body before storing in the `data` column to avoid duplication.

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

TTL semantics for `GET /kv/{key}/_ttl`:
- `N > 0` — seconds remaining
- `-1` — key exists with no expiry
- `-2` — key not found or already expired

`ttl: 0` on `PUT /kv/{key}` means no expiration. Expired keys are filtered at read time and cleaned up by a background goroutine every 30 seconds.

**KV Delete Pattern:** Uses SQL `GLOB` pattern matching (`*` for any sequence, `?` for any single character).

### SSE Event Buffer

The `sse_events` table is defined in `components/vsodb/schema.sql`. Management endpoints are implemented:

```
DELETE /db/_sse_events         → Wipe all SSE events
GET    /db/_sse_events/count   → Count rows
```

Direct event streaming via HTTP is not yet implemented; consumers should use the Pub/Sub WebSocket for real-time events.

### Blob Store

VSODB provides a raw binary blob store keyed by namespace and ID.

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
- **Namespacing**: Blobs are isolated by namespace to prevent ID collisions between different subsystems (e.g. `screenshots`, `uploads`).

### Pub/Sub

HTTP publish (fire-and-forget):
```
POST /pubsub/publish  {"channel": "cmd:op1:sess1", "data": {...}}
→ {"receivers": N}
```

WebSocket (subscribe + publish):
```
wss://vsodb/ws/pubsub

→ {"action": "subscribe",   "channel": "results:op1:sess1"}
→ {"action": "psubscribe",  "channel": "heartbeat:*"}
→ {"action": "unsubscribe", "channel": "results:op1:sess1"}
→ {"action": "publish",     "channel": "cmd:op1:sess1", "data": {...}}

← {"type": "subscribed", "channel": "results:op1:sess1"}
← {"type": "message",  "channel": "results:op1:sess1", "data": {...}}
← {"type": "pmessage", "pattern": "heartbeat:*", "channel": "heartbeat:op1:sess1", "data": {...}}
```

The same WebSocket connection supports both publishing and subscribing. Each subscriber has a 4096-message send buffer; a full buffer causes the subscriber to be dropped.

#### Wire Format — Client → Server (`PubSubMessage`)

| Field | Type | Description |
|-------|------|-------------|
| `action` | string | `subscribe`, `psubscribe`, `unsubscribe`, `publish` |
| `channel` | string | Exact channel name or glob pattern (`*`, `?` supported) |
| `data` | any JSON | Payload — only present for `publish` |

#### Wire Format — Server → Client (`PubSubEvent`)

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `message` (exact match) or `pmessage` (pattern match) |
| `channel` | string | The channel the message was published to |
| `pattern` | string | The glob pattern that matched — only present on `pmessage` |
| `data` | any JSON | The published payload |

#### Channel Naming Convention

All channel prefix constants are defined in `components/vsod/constants/channels.js` (`PubSubChannel`) and mirrored in `components/g8ee/app/constants.py` (`PubSubChannel`). The canonical source is `shared/constants/channels.json`.

| Channel | Direction | Purpose |
|---------|-----------|--------|
| `auth.publish:{api_key_hash}` | g8eo → VSOD | API key auth request |
| `auth.publish:session:{session_hash}` | g8eo → VSOD | WebSession auth request |
| `auth.response:{api_key_hash}` | VSOD → g8eo | API key auth response |
| `auth.response:session:{hash}` | VSOD → g8eo | WebSession auth response |
| `cmd:{operator_id}:{operator_session_id}` | g8ee → Operator | Command dispatch |
| `results:{operator_id}:{operator_session_id}` | Operator → g8ee | Command results |
| `heartbeat:{operator_id}:{operator_session_id}` | Operator → g8ee | Health telemetry |

#### KV Key Format

All KV keys follow the canonical schema `g8e:{domain}:{...segments}`. The `v1` version prefix is `CACHE_PREFIX` — single source of truth in `components/vsod/constants/kv_keys.js`, mirrored in `components/g8ee/app/constants.py`. All key construction uses `KVKey` builders — never hardcode key strings.

For the complete key namespace (all patterns, builders, owners, TTLs, and categories), see [architecture/storage.md — KV Store](../architecture/storage.md#kv-store).

## Client Libraries

### VSOD (Node.js)

`components/vsod/services/clients/db_client.js` — Re-export barrel. VSOD uses **three separate purpose-built clients**:

| Client | File | Transport | Scope |
|--------|------|-----------|-------|
| `VSODBDocumentClient` | `vsodb_document_client.js` | HTTP | Document store CRUD (`/db/...`) |
| `KVClient` | `vsodb_kv_cache_client.js` | HTTP | KV store operations (`/kv/...`) |
| `VSODBPubSubClient` | `vsodb_pubsub_client.js` | WebSocket | Pub/sub messaging (`/ws/pubsub`) |

All HTTP clients share `VSODBHttpClient` (`vsodb_http_client.js`) as a base with timeout, error logging, and auth header propagation.

**`VSODBDocumentClient` — Document store**:
- `getDocument(collection, id)` → `{success, data, error}`
- `setDocument(collection, id, data)` → `{success, error}`
- `updateDocument(collection, id, updates)` → `{success, data, error}`
- `queryDocuments(collection, filters, limit)` → `{success, data, error}`
- `queryDocumentsOrdered(collection, filters, orderBy, limit)` → `{success, data, error}`
- `createDocument(collection, data)` → `{success, id, error}` (generates UUID if `data.id` absent)
- `deleteDocument(collection, id)` → `{success, notFound, error}`
- `runTransaction(collection, id, updateFn)` → `{success, data, error}`
- `VSODBFieldValue.serverTimestamp()`, `.increment(n)`, `.arrayUnion(...)`, `.arrayRemove(...)`, `.delete()` (exported as `VSODBDocumentClient.FieldValue`)

**`KVClient` — KV store**:
- `get(key)`, `set(key, value, 'EX', ttl)`, `set(key, value, 'PX', ms)`, `set(key, value, 'NX')`, `setex(key, seconds, value)`
- `del(...keys)`, `keys(pattern)`, `exists(key)`, `incr(key)`, `decr(key)`
- `expire(key, seconds)`, `ttl(key)`
- `scan(cursor, 'MATCH', pattern, 'COUNT', n)` → `[nextCursor, keys]`
- `hset(key, field, value)`, `hget(key, field)`, `hgetall(key)`, `hdel(key, ...fields)`
- `rpush(key, ...values)`, `lpush(key, ...values)`, `lrange(key, start, stop)`, `llen(key)`, `ltrim(key, start, stop)`
- `sadd(key, ...members)`, `srem(key, ...members)`, `smembers(key)`, `scard(key)`
- `zadd(key, score, member)`, `zrem(key, ...members)`, `zrange(key, start, stop)`, `zrevrange(key, start, stop)`
- `xadd(key, id, ...fieldValues)`, `xrange(key, start, end)`

**`VSODBPubSubClient` — Pub/Sub** (WebSocket, lazy-connected):
- `subscribe(channel)`, `psubscribe(pattern)`, `unsubscribe(channel)`, `publish(channel, data)`
- `on('message', handler)`, `on('pmessage', handler)`
- `duplicate()` — returns a new `VSODBPubSubClient` instance for a separate connection

`publish` sends over the WebSocket connection, not via HTTP `POST /pubsub/publish`.

**Atomicity Warning:** Compound operations (e.g., `incr`, `hset`, `rpush`, `sadd`, `zadd`) are implemented as read-modify-write cycles over HTTP and are **not atomic** under concurrent access.

**Lifecycle**: `terminate()` / `disconnect()` / `quit()` — closes WebSocket and marks client as terminated.

### g8ee (Python)

`components/g8ee/app/clients/db_client.py` — Async client (`KVClient`) for VSODB KV and pub/sub. Uses `aiohttp` for async HTTP and WebSocket. No document store methods, no SSE buffer methods.

**Atomicity Warning:** Like the Node.js client, compound operations are not atomic.

**Connection lifecycle**: `connect()`, `close()`, `health_check()`, `is_healthy()`

**KV store**:
- `get(key)`, `set(key, value, ex=N)`, `set(key, value, px=N)`, `setex(key, seconds, value)`
- `delete(*keys)`, `exists(*keys)`, `keys(pattern)`, `delete_pattern(pattern)`
- `expire(key, seconds)`, `ttl(key)`
- `get_json(key)`, `set_json(key, value, ex=None)` — `ex` is `int | None` (seconds, `None` = no expiration)
- `incr(key, amount=1)`, `decr(key, amount=1)`
- `ping()` — hits `/health`, returns `True`/`False`
- `hset(key, field, value)`, `hget(key, field)`, `hgetall(key)`, `hdel(key, *fields)`
- `rpush(key, *values)`, `lpush(key, *values)`, `lrange(key, start, stop)`, `llen(key)`, `ltrim(key, start, stop)`

**Pub/Sub** (WebSocket, lazy-connected):
- `subscribe(channel)`, `psubscribe(pattern)`, `unsubscribe(channel)`, `punsubscribe(pattern)`, `publish(channel, data)`
- `on_message(handler)` — registers async handler `(channel, data)` for all messages
- `on_pmessage(pattern, handler)` — registers async handler `(pattern, channel, data)` for a specific glob pattern
- `on_disconnect(handler)` / `off_disconnect(handler)` — registers/removes coroutine called on WebSocket disconnect

**Domain pub/sub** (built into `KVClient`):
- `publish_command(operator_id, operator_session_id, command_data)` — publishes to `cmd:{id}:{session}`
- `subscribe_execution_results(operator_id, operator_session_id, callback)` — subscribes to exact `results:{id}:{session}` channel, routes to `callback(channel, data)`
- `unsubscribe_execution_results(operator_id, operator_session_id, callback)` — unsubscribes from results channel
- `subscribe_heartbeats(operator_id, operator_session_id, callback)` — subscribes to exact `heartbeat:{id}:{session}` channel, routes to `callback(channel, data)`
- `unsubscribe_heartbeats(operator_id, operator_session_id, callback)` — unsubscribes from heartbeat channel
- `check_operator_online(operator_id, operator_session_id)` — publishes a ping and returns `receivers > 0`
- `on_channel_message(channel, handler)` / `off_channel_message(channel, handler)` — per-channel handler registration

## SQLite Schema

Single database at `/data/g8e.db`. Four tables: `documents`, `kv_store`, `sse_events`, and `blobs`. The canonical schema is `components/vsodb/schema.sql`, which is mirrored in `components/g8eo/services/listen/listen_db.go`.

For the full DDL with column definitions, indexes, upsert behavior, and SQLite PRAGMA configuration, see [architecture/storage.md — VSODB SQLite Schema](../architecture/storage.md#vsodb-sqlite-schema).

## Management Script

`scripts/data/manage-vsodb.py` — Unified data management CLI. Dispatches to individual resource scripts in `scripts/data/`. Runs inside `g8ep` and communicates with VSODB directly via the HTTP API.

The `store` subcommand provides read-only inspection of the document store and KV store. Other subcommands (`users`, `operators`, `settings`, `device-links`, `audit`) manage their respective resources via the VSOD internal API. See [architecture/scripts.md — Data Management](../architecture/scripts.md#data-management) for the full CLI reference.

### Store Commands

| Command | Description |
|---------|-------------|
| `store stats` | Database statistics: record counts, collections |
| `store operators` | List operators (summary: id, status, name, hostname, os, public\_ip, private\_ip) |
| `store web_sessions` | List web sessions |
| `store operator_sessions` | List operator sessions |
| `store investigations` | List investigations |
| `store cases` | List cases |
| `store users` | List users |
| `store organizations` | List organizations |
| `store api_keys` | List API keys |
| `store login_audit` | List login audit events |
| `store network` | Operator network view: hostname, public\_ip, private\_ip, per-interface IPs, os, arch |
| `store find` | Search any collection for documents where a top-level field exactly matches a value |
| `store doc` | Print a single document as JSON (`--collection` + `--id` required) |
| `store kv` | List KV store keys (optional `--pattern` with `*`/`?` wildcards) |
| `store kv-get` | Print a single KV value (`--key` required) |
| `store wipe` | Clear all app data (preserves platform settings) |
| `store get-setting` | Read a single platform setting value |

### Examples

```bash
python manage-vsodb.py store stats
python manage-vsodb.py store operators
python manage-vsodb.py store network
python manage-vsodb.py store find --collection operators --field status --value active
python manage-vsodb.py store find --collection operators --field hostname --value my-server
python manage-vsodb.py store doc --collection operators --id <id>
python manage-vsodb.py store kv --pattern "g8e:session:*"
python manage-vsodb.py store kv-get --key "g8e:session:web:session_123"
python manage-vsodb.py store --json investigations
python manage-vsodb.py users list
python manage-vsodb.py settings show --section llm
python manage-vsodb.py operators list --email user@example.com
python manage-vsodb.py device-links list --email user@example.com
python manage-vsodb.py audit --db-path /path/to/g8e.db sessions
```

### Operator Summary Fields

The `store operators` and `store network` commands extract IPs from operator documents:

- `hostname` — from `system_info.hostname` or `latest_heartbeat_snapshot.system_identity.hostname`
- `public_ip` (`operators`) — from `system_info.public_ip`
- `public_ip` (`network`) — from `system_info.public_ip`, falling back to `latest_heartbeat_snapshot.network.public_ip`
- `private_ip` — first interface IP from `latest_heartbeat_snapshot.network.connectivity_status` that does not start with `172.` or `127.`
- `interfaces` (`network` command only) — all `name=ip` pairs from `connectivity_status`

## Air-Gap Deployment

In a fully air-gapped environment, the deployment is three processes from a single binary:

```bash
# Terminal 1: Persistence + messaging backbone
./g8e.operator --listen --data-dir ./data

# Terminal 2: Web dashboard
node components/vsod/server.js

# Terminal 3: AI engine
python components/g8ee/app/main.py
```

No Docker required. No internet required. No external dependencies. The Operator binary IS the infrastructure.
