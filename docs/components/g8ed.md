---
title: g8ed
parent: Components
---

# g8ed — g8e Dashboard

## Overview

g8ed is the authentication, session management, and dashboard backend for g8e. It serves the browser-facing web UI, manages user sessions, proxies AI interactions to g8ee, controls Operator lifecycle, and provides real-time updates to the frontend via Server-Sent Events.

> For deep-reference security documentation — internal auth token, SSL/CA handling, web session security, operator session security, operator auth methods, operator binding, and the full threat model — see [architecture/security.md](../architecture/security.md).

**Core responsibilities:**
- Single external entry point — all inbound traffic (browser HTTPS, Operator auth, Operator pub/sub) enters through g8ed
- User authentication (passkey/FIDO2/WebAuthn) and session lifecycle
- WebSession persistence and encryption via g8es KV (with durability in g8es document store)
- Operator binding, status tracking, and heartbeat registration relay
- Chat/AI streaming proxy to g8ee
- WebSocket proxy — `/ws/pubsub` upgrade requests forwarded to g8es internally
- Binary distribution of the g8e Operator (`g8e.operator`)
- Device Link — secure multi-use Operator deployment and single-use claiming
- Audit log, admin console, and platform settings
- SSE delivery of real-time events to browser clients (AI, Operator status, investigations)
- MCP Gateway (Model Context Protocol) — bridging external AI tools to internal platform capabilities

---

## Architecture

```
          Browser / Operator / MCP Client
               │  HTTPS (cookies + SSE + auth + JSON-RPC)
               │  WSS /ws/pubsub (proxied to g8es)
               ▼
┌─────────────────────────────────────────────────────────┐
│                       g8ed                              │
│                                                         │
│  Auth & Sessions    Operator Mgmt      Chat Proxy       │
│  ─────────────────  ──────────────     ────────────     │
│  passkey auth       bind / unbind      stream → g8ee     │
│  WebSession KV      heartbeat relay    stop / cases     │
│  encrypt fields     device links       audit log        │
│                                                         │
│  SSE Service        Internal API       Admin            │
│  ─────────────      ────────────       ─────            │
│  fan-out events     g8ee ↔ g8ed         console          │
│  session-scoped     cluster-only       settings         │
│                                                         │
│  MCP Gateway                                            │
│  ─────────────                                          │
│  tools/list → g8ee                                     │
│  tools/call → g8ee                                     │
│                                                         │
│  WS Proxy                                               │
│  ──────────                                             │
│  /ws/pubsub → g8es:9001                                 │
└─────────────────────────────────────────────────────────┘
          │  HTTP KV / Doc / WSS            │  HTTP
          ▼                                 ▼
       g8es                               g8ee
  (Sessions, KV,                    (AI Engine, Operator
   Document Store,                   Heartbeat Source,
   Pub/Sub)                          Command Execution)
```

**Key design principles:**
- g8ed is the single external entry point — browsers, Operators, and MCP clients all connect to g8ed on port 443
- g8ed also owns the HTTP trust bootstrap on port 80 — plain HTTP delivers the workstation CA trust portal, the operator deployment script (`/g8e`), and then hands users off to HTTPS
- g8ed is stateless between requests — all state lives in g8es
- g8ee is the source of truth for heartbeat data; g8ed relays events via SSE
- Frontend never computes operator status — it consumes backend-provided values
- Runtime configuration flows from g8es via `SettingsService.initialize()` after Phase 2 of `initialization.js` — zero `process.env` reads in g8ed production code
- WebSocket pub/sub (`/ws/pubsub`) is proxied by g8ed to g8es on port 9001 — g8es has no external port bindings
- Route handlers are thin: parse → validate → call service → respond. No business logic, no direct infrastructure access, no orchestration sequences in handlers.

---

## Environment & Configuration

g8ed reads **zero** environment variables via `process.env` at runtime. Bootstrap transport URLs are defined as constants in `constants/http_client.js`. Bootstrap secrets (auth token, CA cert, session encryption key) are read from the **Shared SSL Volume** by `BootstrapService`. All other configuration flows through `SettingsService` from the g8es `settings` collection.

The following variables are consumed by `docker-compose.yml` or standard container practices. They are **not** read by g8ed application code at runtime (except `NODE_ENV` for non-production logic):

| Variable | Default | Description |
|----------|---------|-------------|
| `G8E_INTERNAL_HTTP_URL` | `https://g8es:9000` | g8es internal HTTP URL (mapped to constant) |
| `G8E_INTERNAL_PUBSUB_URL` | `wss://g8es:9001` | g8es internal pub/sub WebSocket URL (mapped to constant) |
| `G8EE_URL` | `https://g8ee` | g8ee internal URL (mapped to constant/setting) |
| `G8E_SSL_DIR` | `/g8es` | Directory containing platform certificates and secrets |

### Settings Pipeline

All configuration — LLM provider, API keys, passkey config, cert paths, session tuning, app URL, CORS — flows through `SettingsService`. The service is initialized in `initialization.js` and provides access to both platform-wide and user-specific settings.

#### Precedence Chain

g8ed enforces a strict precedence chain when resolving configuration values. This allows for sensible defaults and runtime persistence in g8es.

Precedence: **User Settings > Platform Settings > Schema Default**

For **User-configurable settings** (`USER_SETTINGS`):
1.  **User Settings (DB)**: Individual user overrides stored in the `settings` collection, document ID is `user_settings:{userId}`.
2.  **Platform Settings (DB)**: Values stored in g8es `settings` collection, `platform_settings` document.
3.  **Defaults**: Defined in the schema (`components/g8ed/models/settings_model.js`).

For **Platform settings** (`PLATFORM_SETTINGS`):
1.  **Platform Settings (DB)**: Values stored in g8es `settings` collection, `platform_settings` document.
2.  **Defaults**: Defined in the schema.

**Exception: Bootstrap Secrets**
Critical bootstrap secrets (`internal_auth_token`, `session_encryption_key`, `ca.crt`) use the **Shared SSL Volume** as the absolute source of truth. They are read directly from `/g8es/` at startup and are **never stored in the database**. This decouples platform identity from the database lifecycle.

#### Seeding and Protection

On the first boot of a new deployment, g8ed automatically **seeds** the `settings` collection in g8es with configuration values from the environment (e.g. LLM keys, URLs).

**Core Secrets Protection:**
Unlike other settings, `internal_auth_token` and `session_encryption_key` are managed exclusively by g8es on the SSL volume. They are not present in the Settings UI and cannot be overridden via the database. This ensures the integrity of the platform's authoritative credentials even across full database wipes or resets.

---

## Data Connectivity

g8ed connects to g8es using two separate transports — one per concern.

| Concern | Transport | Rationale |
|---------|-----------|-----------|
| Document store / KV | HTTP | Request/response semantics; stateless; standard error codes |
| Pub/Sub | WebSocket | Server-push required; long-lived connection; no polling |
| User/Operator Event | HTTP (Incoming) | Incoming events from g8ee delivered directly to browser via SSE |

### Client Classes

| Class | File | Purpose |
|-------|------|---------|
| `G8esDocumentClient` | `services/clients/g8es_document_client.js` | HTTP document store (collections API). All requests authenticated via `X-Internal-Auth`. |
| `KVCacheClient` | `services/clients/g8es_kv_cache_client.js` | HTTP KV store. All requests authenticated via `X-Internal-Auth`. |
| `G8esPubSubClient` | `services/clients/g8es_pubsub_client.js` | WebSocket pub/sub. Connection authenticated via `X-Internal-Auth` (header or query param). |
| `g8esBlobClient` | `services/platform/g8es_blob_client.js` | HTTP blob store for large binary data (e.g. attachments). |
| `InternalHttpClient` | `services/clients/internal_http_client.js` | Outbound HTTP client for g8ee communication. |

`services/initialization.js` handles the composition and injection of these clients into higher-level services.

### KV Key Schema

All keys follow the format `g8e:{domain}:{...segments}`. The `g8e` prefix (`CACHE_PREFIX` from `constants/kv_keys.js`) allows atomic namespace invalidation by bumping the version in `shared/constants/kv_keys.json`. **Never construct key strings manually — always use `KVKey` builders from `constants/kv_keys.js`.**

For the complete KV key namespace (all patterns, builders, owners, TTLs), document collection registry, and cache-aside implementation details, see [architecture/storage.md](../architecture/storage.md).

### Cache-Aside Pattern

g8ed uses the cache-aside pattern for consistency between g8es KV and the document store. Document store is authoritative; KV is the read cache. Invalidate-on-update — never update-in-place.

**Implementation:** `services/cache/cache_aside_service.js`

| Service | Usage |
|---------|-------|
| `WebSessionService` | WebSession persistence and caching |
| `OperatorSessionService` | Operator session persistence and caching |
| `BoundSessionsService` | Session binding persistence and caching |
| `UserService` | User profile caching |
| `ApiKeyDataService` | API key caching |
| `LoginSecurityService` | Audit log writes; KV fast-path via injected `kvClient` |
| `DeviceLinkService` | Device link token operations via injected `kvClient` |
| `OrganizationModel` | Organization document caching |
| `OperatorDataService` | Operator document caching |

All services follow the same contract: document store written first, KV invalidated on writes, KV populated on reads. KV-only fast-path services (`LoginSecurityService`, `DeviceLinkService`) receive `kvClient` as a direct constructor argument — they do not go through `CacheAsideService` internals.

### Pub/Sub Channels

All channel prefix constants are defined in `constants/channels.js` (`PubSubChannel`). The canonical channel listing is in [components/g8es.md — Channel Naming Convention](g8es.md#channel-naming-convention).

g8ed subscribes to `auth.publish:*` channels via `SessionAuthListener` to handle g8eo API key and WebSession authentication requests. Command, results, and heartbeat channels are brokered transparently by g8es between g8ee and g8eo.

### Internal HTTP Communication (g8ed → g8ee)

g8ed communicates with g8ee via direct HTTP using `X-Internal-Auth` for authentication and `G8eHttpContext` headers for routing. Communication is split by concern:

1.  **Generic / Case Management (`InternalHttpClient`):** Chat messages, investigation queries, and case deletions.
2.  **Operator Orchestration (`OperatorRelayService`):** Operator lifecycle (stop, deregister), approvals, and direct command relays.

The internal HTTP layer enforces:

- `X-Internal-Auth` is **required** for all calls (shared secret strictly enforced)
- `web_session_id` is **required** for all g8ee calls (via `X-G8E-WebSession-ID`)
- `user_id` is **required** for all g8ee calls (via `X-G8E-User-ID`)
- Bound operators are resolved via `requireOperatorBinding` middleware and carried in `req.boundOperators` for internal relay orchestration
- `X-G8E-Case-ID` and `X-G8E-Investigation-ID` are always present — real IDs for existing cases, `new-case-via-g8ed` sentinels for new cases
- Bound operators are sent as a JSON array via `X-G8E-Bound-Operators` (operator_id, operator_session_id, status)

#### New Case Protocol (`X-G8E-New-Case`)

When a user sends their first message in a new conversation, `chat_routes.js` receives a request body with no `case_id`. `InternalHttpClient.buildG8eContextHeaders` detects the missing `case_id` as the new-case signal:

```
request body has no case_id
    → g8eContext.case_id = null
    → buildG8eContextHeaders detects !context.case_id
    → sets X-G8E-New-Case: true
    → sets X-G8E-Case-ID: new-case-via-g8ed
    → sets X-G8E-Investigation-ID: new-case-via-g8ed
    → g8ee creates case + investigation inline
    → returns ChatStartedResponse with new case_id + investigation_id
```

**Type guard rule:** `chat_routes.js` uses `(typeof case_id === 'string' && case_id.length > 0) ? case_id : ''` — never `case_id || ''`. This prevents non-string values (`0`, `false`) from a malformed client being silently treated as a valid case ID.

The frontend cannot forge `X-G8E-New-Case: true` — the signal is generated server-side by g8ed based on the authenticated session context, not on any client-supplied header.

The `X-G8E-New-Case` header name is defined in `shared/constants/headers.json` and consumed by both `constants/headers.js` (g8ed) and `app/constants/headers.py` (g8ee).

### MCP Gateway (`/mcp`)

> For comprehensive MCP architecture, provider-agnostic design, and translation layer patterns, see [architecture/mcp.md](../architecture/mcp.md).

g8ed exposes a single Streamable HTTP MCP endpoint at `POST /mcp` for external MCP clients such as Claude Code. The endpoint accepts JSON-RPC 2.0 requests and dispatches by `method`:

| Method | Behaviour |
|--------|-----------|
| `initialize` | Returns server info and capabilities (`{ tools: {} }`) |
| `notifications/initialized` | Returns 204 (notification, no body) |
| `ping` | Returns `{}` |
| `tools/list` | Proxies to g8ee `POST /api/internal/mcp/tools/list` |
| `tools/call` | Proxies to g8ee `POST /api/internal/mcp/tools/call` |

**Auth:** Supports two authentication methods:

1. **Session Token** (standard web authentication)
   - Uses `requireAuth` middleware
   - Caller passes a valid session token as `Authorization: Bearer <token>`
   - Same session token used by the browser works for MCP clients
   - Requires an active web session

2. **OAuth Client ID** (for Claude Code connector)
   - Caller passes OAuth Client ID via `x-oauth-client-id` header or `oauth_client_id` query parameter
   - OAuth Client ID is validated as a G8eKey (API key) via `ApiKeyService.validateKey()`
   - Does not require a web session
   - Bound operators are resolved by user ID instead of web session ID

**Context:** `requireOperatorBinding` middleware resolves bound operators before every request:
- For session authentication: resolves by web session ID via `resolveBoundOperators(webSessionId)`
- For OAuth Client ID authentication: resolves by user ID via `resolveBoundOperatorsForUser(userId)`

If no operators are bound, `tools/list` returns only non-operator tools (e.g. web search); `tools/call` for operator tools returns an error.

**Files:** `routes/platform/mcp_routes.js`, `services/clients/internal_http_client.js` (`mcpToolsList`, `mcpToolsCall`).

### Bound Operator Resolution (`requireOperatorBinding`)

`requireOperatorBinding` in `middleware/authentication.js` is the single point where g8ed resolves bound operators before every chat and MCP request. It executes at request time — **no cached result** — and its output is the only source g8ee uses to identify which operators are available to the AI.

**Resolution steps (per request):**

1. Call `getBindingService().resolveBoundOperators(webSessionId)` (or `resolveBoundOperatorsForUser(userId)`).
2. Service reads the `bound_sessions` collection document for durability.
3. For each bound operator ID, it fetches the operator document via `operatorService.getOperator(id)`.
4. It returns an array of `BoundOperatorContext` objects containing `operator_id`, `operator_session_id`, and `status`.
5. Middleware attaches this array to `req.boundOperators`.
6. `InternalHttpClient` serializes this into the `X-G8E-Bound-Operators` header for g8ee.

**Accessor:** `getBindingService().resolveBoundOperators(webSessionId)` from `services/auth/bound_sessions_service.js`.

**Contract:**
- g8ee reads `g8e_context.bound_operators` (parsed from `X-G8E-Bound-Operators`) as the **exclusive source of truth** for which operators are bound. g8ee performs no independent operator lookup to resolve binding state.
- Only operators with `status == 'bound'` are used by g8ee when building chat context and determining workflow type (`OPERATOR_BOUND` vs `OPERATOR_NOT_BOUND`).
- If `X-G8E-Bound-Operators` is absent or empty, g8ee operates in advisory mode — no operator commands are available to the AI.

---

## Session Management

### WebSession Structure

Sessions are stored in both the g8es document store (`web_sessions` / `operator_sessions` collections) and the g8es KV store (fast path). The `api_key` field is encrypted with AES-256-GCM using `session_encryption_key` (bootstrap secret).

**KV key types:**
- Web session: `g8e:session:web:{id}` — `KVKey.webSessionKey(id)` — 8h idle / 24h absolute TTL
- Operator session: `g8e:session:operator:{id}` — `KVKey.operatorSessionKey(id)` — 8h idle / 24h absolute TTL

**Binding keys:**
- `KVKey.sessionBindOperators(operatorSessionId)` → operator session → bound web session ID (STRING)
- `KVKey.sessionWebBind(webSessionId)` → web session → bound operator session IDs (SET)

For the complete session document field schema (all fields, types, encryption details, and TTL values), see [architecture/storage.md — Session Documents](../architecture/storage.md#session-documents).

### Operator Bind Orchestration (`BindOperatorsService`)

`services/operator/operator_bind_service.js` owns the full bind/unbind orchestration lifecycle for the user-facing bind routes. It encapsulates ownership verification, idempotency checking, stale session cleanup, status gating, binding writes, document updates, and SSE event publishing for all four bind/unbind operations (`bindOperator`, `bindOperators`, `unbindOperator`, `unbindOperators`). Route handlers delegate entirely to this service — they never reach past it to `BoundSessionsService`, `OperatorSessionService`, or `CacheAsideService` directly.

**Accessor:** `getBindOperatorsService()` from `services/initialization.js`.

### Operator–Web Session Binding (`BoundSessionsService`)

`services/auth/bound_sessions_service.js` owns the full binding lifecycle between an Operator session and a Web session. It is the single source of truth for all binding state — no other service reads or writes the bind KV keys directly.

**Backing stores:**
- **g8es KV** (fast path) — two bidirectional keys per binding (STRING and SET), read on every routed request
- **g8es Document Store via `CacheAside`** — `bound_sessions` collection, one document per web session (`id = webSessionId`), for durability and audit

**Public API:**

| Method | Description |
|--------|-------------|
| `bind(operatorSessionId, webSessionId, userId, operatorId)` | Create a bidirectional binding. Writes both KV keys and persists a `BoundSessionsDocument`. |
| `unbind(operatorSessionId, webSessionId, operatorId)` | Remove a single binding. Deletes both KV keys and removes the operator from the stored document. |
| `getBoundOperatorSessionIds(webSessionId)` | Returns the list of operator session IDs currently bound to a web session. Reads from `sessionWebBind` KV key (SET). |
| `getWebSessionForOperator(operatorSessionId)` | Returns the web session ID bound to a given operator session, or `null` if none. Reads from `sessionBindOperators` KV key (STRING). |
| `resolveBoundOperators(webSessionId)` | Resolves all live bound operators for a web session by reading the binding document and fetching operator docs. |
| `resolveBoundOperatorsForUser(userId)` | Resolves all live bound operators for a user by scanning all bound sessions. |

**Binding contract:**
- One operator session can be bound to at most one web session at a time
- One web session can be bound to multiple operator sessions simultaneously
- All binding mutations go through `BoundSessionsService` — routes call `getBindingService()` from `initialization.js`
- The `sessionBindOperators` key is the authoritative fast-path lookup for SSE routing: `internal_sse_routes.js` and `internal_operator_routes.js` call `getWebSessionForOperator(operatorSessionId)` to resolve where to deliver events
- `requireOperatorBinding` middleware calls `resolveBoundOperators(webSessionId)` or `resolveBoundOperatorsForUser(userId)` to resolve live bound operators for chat and MCP requests

**Accessor:** `getBindingService()` from `services/initialization.js` — throws if called before `initializeServices()`.

### `requireAuth` Middleware

`requireAuth` (`middleware/authentication.js`) is the single session validation point for all authenticated routes. It extracts the session ID in priority order:

1. `web_session_id` secure cookie
2. `X-Session-Id` request header
3. `Authorization: Bearer <sessionId>` header

After validation it attaches to `req`:
- `req.webSessionId` — validated session ID
- `req.session` — fully validated and refreshed session object
- `req.userId` — shorthand for `req.session.user_id`

**Security:** Authentication middleware rejects requests that include `web_session_id` in URL query parameters to prevent session leakage in logs.

Route handlers behind `requireAuth` must use these properties exclusively — never re-extract the session ID or call `validateSession()` again.

### Security Features

- **Idle timeout:** 8 hours (configurable via `SESSION_TTL`)
- **Absolute timeout:** 24 hours max (configurable via `ABSOLUTE_SESSION_TIMEOUT`)
- **Concurrent session limit:** 5 per user; oldest evicted on overflow
- **IP binding:** Validates IP on every request; flags suspicious activity after 4+ IP changes
- **Encryption:** AES-256-GCM on `api_key` (web sessions) and `operator_id` (operator sessions)
- **Cookie security:** `HttpOnly`, `Secure`, `SameSite=Lax`
- **CSRF protection:** Provided by `SameSite=Lax` — no additional tokens required

### Rate Limits

| Scope | Limit |
|-------|-------|
| Auth endpoints | 20 requests / 5 min |
| Passkey endpoints | 20 requests / 5 min |
| Chat endpoints | 30 messages / min |
| SSE connections | 30 attempts / 5 min (failed only) |
| General API | 60 requests / min |
| Global public API | 100 requests / min |

#### Rate Limiter Wiring Pattern

Most limiters are built inside `createRateLimiters()` in `middleware/rate-limit.js` and plumbed through `server.js` → `app_factory.js` into each route factory via the `rateLimiters` argument.

**Exception — auth-sensitive limiters must be module-level exports.** Static-analysis tools (notably CodeQL's `js/missing-rate-limiting` query) cannot trace middleware through a factory → returned-object → destructured-param chain. Any limiter that protects a brute-forceable authentication surface must therefore be:

1. Declared at module scope in `middleware/rate-limit.js` and exported directly (e.g. `export const passkeyRateLimiter = rateLimit({ ... })`).
2. Imported directly by the route file (e.g. `import { passkeyRateLimiter } from '../../middleware/rate-limit.js'`) and applied to the relevant route(s) as middleware.

Currently the following limiters follow this pattern and **must not** be re-introduced into the factory's declaration body:

| Limiter | Protected surface |
|---------|-------------------|
| `passkeyRateLimiter` | `POST /api/auth/passkey/*` |
| `authRateLimiter` | `POST /api/auth/register` |
| `operatorAuthRateLimiter` + `operatorAuthIpBackstopLimiter` | `POST /api/auth/operator` |
| `deviceLinkRateLimiter` | `POST /auth/link/:token/register` (public device register) |

The `createRateLimiters()` factory still re-exports the same bindings in its returned object (via module closure) for back-compat with the middleware unit test suite; routes that rely on the limiter for security must not depend on that indirection. When adding a new auth-sensitive limiter, follow the same pattern.

Tests that would otherwise exhaust the real limiter's window should use `vi.mock('@g8ed/middleware/rate-limit.js', ...)` with `vi.importActual` to pass-through the specific limiter(s) under test — see `test/unit/routes/auth/passkey_routes.unit.test.js` for the canonical example.

---

## Users & Authentication

### User Model

**Collection:** `users` — implemented in `models/user_model.js`

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | UUID, primary key |
| `email` | string | User's email |
| `name` | string | Display name |
| `passkey_credentials` | array | FIDO2/WebAuthn credential list (id, public_key, counter, transports) |
| `passkey_challenge` | string \| null | Pending WebAuthn challenge (stripped by `forClient()`) |
| `passkey_challenge_expires_at` | string \| null | Challenge expiry timestamp (stripped by `forClient()`) |
| `provider` | string | Auth provider (`passkey`) |
| `g8e_key` | string \| null | User's download API key |
| `g8e_key_created_at` | string \| null | ISO timestamp of download API key creation |
| `g8e_key_updated_at` | string \| null | ISO timestamp of last download API key refresh |
| `organization_id` | string | User's own org (equals `id`) |
| `roles` | string[] | `user`, `admin`, `superadmin` |
| `operator_id` | string \| null | Associated operator ID |
| `operator_status` | string \| null | Cached operator status |
| `last_login` | string | ISO timestamp |
| `provider` | string | Auth provider (`passkey`) |
| `sessions` | array | Session tracking (stripped by `forClient()`) |
| `profile_picture` | string \| null | Profile picture URL |
| `dev_logs_enabled` | boolean | Per-user dev logging (default true) |

`forClient()` strips `passkey_credentials`, `passkey_challenge`, `passkey_challenge_expires_at`, `g8e_key`, and `sessions`. `dev_logs_enabled` is user-visible and is not stripped.

Users are cached directly by `UserService`. Cache key: `KVKey.doc('users', userId)` → `g8e:cache:doc:users:{userId}`.

### Organization Model

**Collection:** `organizations` — implemented in `models/organization_model.js`

Each user has their own org (`org_id` equals their `user_id`). Fields include `owner_id`, `name`, `org_admin`, `team_members`, and `stats`.

### Passkey Credential Schema

Each entry in `passkey_credentials`:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Base64url credential ID |
| `public_key` | string | Base64url COSE public key |
| `counter` | number | Signature counter (increments on each auth) |
| `transports` | string[] | Authenticator transports (e.g. `internal`, `usb`) |
| `created_at` | string | ISO timestamp |
| `last_used_at` | string \| null | ISO timestamp of last successful authentication |

### Authentication Flow

**Returning user (valid session cookie):**

```
GET /  →  server validates cookie  →  redirect to /chat
/chat loads  →  validateSession() resolves immediately  →  all components initialize
```

**Returning user (no/expired cookie):**

```
GET /  →  renders login.ejs
User clicks Sign In  →  POST /api/auth/passkey/auth-challenge  →  browser prompts passkey
                     →  POST /api/auth/passkey/auth-verify    →  session cookie set  →  navigate to /chat
```

**First run (no users exist):**

```
HTTP onboarding:

GET https://<host>
    → serves CA trust portal
    → user trusts the g8es-generated platform CA on their workstation
    → user continues to https://<host>/setup

GET /  →  hasAnyUsers() returns false  →  redirect to /setup

GET /setup  →  renders views/setup.ejs
              (embeds the LLM provider catalog from components/g8ed/constants/ai.js
               as a JSON script tag consumed by setup-page.js — no browser-side
               duplicate of the catalog exists)

4-step browser wizard:

  Step 1 — Account
    User enters email (required) + optional full name

  Step 2 — AI Providers
    User enters API keys/URLs for any subset of providers
    (Gemini / Anthropic / OpenAI / Ollama — Ollama host:port is required; API key is optional).
    Three custom dropdowns (Primary / Assistant / Lite model) populate from the
    union of configured providers; each model id is server-driven from the
    injected catalog. Defaults are **mid-tier by design** (e.g. Sonnet/Flash instead of Opus/Pro) to prevent surprise billing and ensure compatibility with commodity hardware for local providers. The canonical source for these defaults is `PROVIDER_DEFAULT_MODELS` in `components/g8ed/constants/ai.js`. The user may also type a custom model id via "Custom…".
    llm_primary_provider / llm_assistant_provider / llm_lite_provider are
    derived from the selected model ids.

  Step 3 — Web Search (optional)
    Google: vertex_search_project_id, vertex_search_engine_id, vertex_search_api_key
    (or reuse the Gemini key via a checkbox). None: web search stays disabled.

  Step 4 — Finish
    Summary card shown
    →  POST /api/auth/register                         →  body { email, name, settings };
                                                          first-run branch creates the
                                                          superadmin atomically via
                                                          setupService.performFirstRunSetup
                                                          and returns { user_id, challenge_options }
    →  navigator.credentials.create(publicKey=options) →  browser passkey prompt
    →  POST /api/auth/passkey/register-verify-setup    →  attestation verified;
                                                          credential persisted;
                                                          session cookie set
                                                       →  redirect to /chat
```

- Setup redirect triggers **only** when the users collection is entirely empty
- First-run state is tracked by `platform_settings.setup_complete`. `setupService.isFirstRun()` returns `true` while that flag is unset or `false`; `setupService.completeSetup()` sets it to `true` at the end of a successful `register-verify-setup`
- During first-run, `POST /api/auth/register` takes the first-run branch (`setupService.performFirstRunSetup`) and creates the superadmin together with a WebAuthn challenge. After first-run completes it falls through to the standard registration path — a public `authRateLimiter`-protected endpoint that rejects duplicate emails
- `POST /api/auth/passkey/register-verify-setup` is gated by the `requireFirstRun` middleware. It additionally rejects the request with `Setup already complete` if the target user already has any credential, so an admin credential cannot be silently overwritten
- Settings collected by the wizard are stored in g8es as a **user settings** document (per-user, not platform-wide). The flat UI payload is converted to the nested `{ llm, search, eval_judge }` shape by `settings_model.js` before storage. `internal_auth_token` and `session_encryption_key` are read from the SSL volume at startup — they are not injected via environment variables
- No default credentials are ever created
- `hasAnyUsers()` throws on DB error — never silently returns false
- First-run user is created with `UserRole.SUPERADMIN`
- If the passkey attestation fails or is cancelled, the user row created by `/api/auth/register` remains. Because no credential has been persisted, the user can click Finish again to obtain a fresh challenge and retry the passkey prompt; `setup_complete` stays `false` until verification succeeds

### Workstation Certificate Trust Portal

g8ed serves a public HTTP onboarding page specifically for workstation CA trust bootstrap. This page is not the application itself — it exists so the browser can reach g8e without certificate warnings before the workstation trusts the platform CA.

**Behavior:**
- Auto-detects OS and pre-selects the matching trust flow
- Exposes 1-Click Installers for macOS (`/trust.sh`), Windows (`/trust.bat`), and Linux (`/trust-linux.sh`)
- Exposes raw CA download at `/ca.crt` for manual trust flows and mobile devices
- Presents a direct link to `https://<host>/setup` for users who already trust the CA
- Serves the **operator deployment script** at `/g8e` — a POSIX shell script that automates operator deployment on any remote Linux system (see [Operator Deployment Script](#operator-deployment-script))

### `/trust` Endpoint

The `/trust` endpoint (HTTP port 80) provides a unified, OS-agnostic entry point for certificate trust installation. It automatically detects the client's operating system from the `User-Agent` header and returns the appropriate trust script.

**Usage:**

**macOS / Linux** (run in Terminal):
```bash
curl -fsSL http://<host>/trust | sudo sh
```

**Windows** (run in an elevated PowerShell terminal):
```powershell
irm http://<host>/trust | iex
```

**Behavior:**
- Detects OS from `User-Agent` header (checks for `windows`, `win32`, `win64`, `powershell`)
- For Windows: Returns a PowerShell script (`windowsPowerShellTrustScript`) that downloads and trusts the CA via `certutil`
- For macOS/Linux: Returns a POSIX shell script (`universalTrustScript`) that detects the OS at runtime via `uname -s` and performs the appropriate trust operation
- Scripts are generated per-request using the client's `Host` header, so they work from localhost, LAN IP, or hostname
- Non-default HTTP ports are automatically included in the generated script URLs

**Implementation:** `utils/cert-installers.js` → `windowsPowerShellTrustScript()` and `universalTrustScript()`, served by the HTTP server in `server.js` (lines 507-528).

**Responsibility split:**
- g8es generates and stores the CA and server certificates
- g8ed reads those certificates from the g8es SSL volume (`/g8es/`)
- g8ed serves the browser trust portal, OS-specific installer scripts, and the operator deployment script
- All scripts are generated per-request using the client's `Host` header, so they work from localhost, LAN IP, or hostname

### Operator Deployment Script

g8ed serves a POSIX shell script at `GET /g8e` (HTTP port 80) that automates operator deployment on any remote Linux system. The script is generated per-request with the platform's host and port baked in.

**Usage:**
```bash
curl -fsSL http://<host>/g8e | sh -s -- <device-link-token>
```

**What the script does:**
1. Detects the system architecture (`amd64`, `arm64`, or `386` via `uname -m`)
2. Fetches the platform CA certificate over plain HTTP from `http://<host>/ca.crt`
3. Downloads the operator binary over HTTPS from `/operator/download/linux/<arch>`, using the fetched CA for TLS verification and the device link token as Bearer auth
4. Launches the operator with `--device-token` and `--endpoint` flags

**Design constraints:**
- Pure POSIX `sh` — no bashisms, works on ancient and modern Linux alike
- Supports both `curl` and `wget` (prefers curl, falls back to wget)
- CA certificate is held in a temp file and cleaned up before the operator launches
- Token can be passed as a positional argument, via `G8E_TOKEN` env var, or interactively when running from a terminal
- The script errors with a clear usage message if piped without a token
- Non-default HTTPS/HTTP ports are handled automatically — the generated script includes the correct port suffixes and `--wss-port`/`--http-port` flags

**Implementation:** `utils/cert-installers.js` → `g8eDeploy(host, httpsPort, httpPort)`, served by the HTTP server in `server.js`.

---

## Operator Management

### Operator Statuses

| Status | Authenticated | Bound | Heartbeat (< 60s) | User Unbind | User Stop |
|--------|:---:|:---:|:---:|:---:|:---:|
| `available` | No | No | No | No | No |
| `offline` | Yes | No | No | No | No |
| `stale` | Yes | — | No | — | — |
| `active` | Yes | No | Yes | Yes | No |
| `bound` | Yes | Yes | Yes | No | No |
| `stopped` | Yes | Yes | No | No | Yes |
| `unavailable` | Yes | — | — | — | — |
| `terminated` | Yes | — | — | — | — |

**Key rules:**
- `available` is the default state — operator has been provisioned but never authenticated
- `stale` is set by g8ee's `HeartbeatStaleMonitorService` when a heartbeat has not been received within the stale threshold (60s). g8ee is authoritative for heartbeat status decay since it ingests all heartbeats.
- `terminated` — decommissioning completed; document is preserved for audit but excluded from UI lists. Termination is a status transition; the operator document is preserved for audit; only TERMINATED-status operators are excluded from default listings.
- Successful auth transitions directly from `available` to `active`
- Binding is **always manual** — user clicks "Bind" in the UI
- Each web session can bind to **multiple** operators simultaneously
- An operator can only be bound to **one** web session at a time
- `unavailable` and `terminated` are filtered out of UI lists
- Status logic lives in `computeStatusDisplayFields()` in `constants/operator.js` — the frontend never computes status locally

### Service Architecture

`OperatorDataService` follows a subservice architecture to maintain logical boundaries and avoid monolithic bloat.

| Subservice | Responsibility |
|------------|----------------|
| `slots` | Slot initialization, claiming, and API key management. Operator slots are provisioned during user login or upfront when Device Links are created to fulfill the slot limit. |
| `relay` | Outbound communication to g8ee (Stop, Direct Command, Heartbeat Registration, Approvals). |
| `notifications` | SSE event broadcasting to browser sessions (Operator list updates). |

Route handlers and other services interact with these via the main `OperatorService` coordinator.

### OperatorSlot Projection for SSE Optimization

Operator list events use `OperatorSlot` projections instead of full `OperatorDocument` objects to reduce SSE payload size. `OperatorSlot` is a lightweight model containing only the ~10 fields needed by the operator list UI:

- `operator_id` — unique identifier
- `name` — operator name
- `status` — current status
- `status_display` — human-readable status string
- `status_class` — CSS class for status badge
- `bound_web_session_id` — bound web session (if any)
- `is_g8ep` — g8e node operator flag
- `first_deployed` — first deployment timestamp
- `last_heartbeat` — last heartbeat timestamp
- `system_info` — minimal system info (hostname, os, internal_ip, public_ip)
- `latest_heartbeat_snapshot` — most recent performance metrics snapshot

**Projection path:** `OperatorDocument` → `OperatorSlot.fromOperator()` → `forClient()` → SSE payload

**Route changes:**
- `GET /api/operator/:operatorId/details` now returns `OperatorSlot` instead of full `OperatorDocument`
- `getUserOperators()` returns `OperatorSlot[]` via `OperatorListUpdatedEvent`
- SSE keepalive events embed `OperatorSlot[]` in `OperatorListData`

This optimization significantly reduces bandwidth for operator list updates, especially for users with many operators.

### SSE Connection Initialization

When the browser establishes a new SSE connection, `SSEService.pushInitialState(userId, webSessionId, organizationId)` fires two parallel side effects:

1. **LLM config push** — reads user/platform settings to determine the active provider, then assembles provider-specific models lists and publishes an `LLMConfigEvent`.
2. **Investigation list push** — queries g8ee via `InternalHttpClient` and publishes an `InvestigationListEvent` for case navigation.
3. **Operator list push** — queries the `operators` collection and publishes an `OperatorListUpdatedEvent` containing all user operators.

Additionally, `OperatorService.syncSessionOnConnect(userId, webSessionId)` handles:
1. **Operator list repair** — repairs any stale `BOUND` web session links (tab swap detection).

`SSEService` receives its dependencies at construction time (or via `setDependencies` in `initialization.js`) to avoid circularity.

**Implementation Note:** The SSE route handler uses `getOperatorService()` from `initialization.js` at request time to resolve the `OperatorService` instance. This factory pattern prevents circular dependencies during the multi-phase boot process.

### Heartbeat Architecture

g8eo sends heartbeats every 30 seconds directly to g8es pub/sub. g8ee subscribes, validates, and persists `last_heartbeat` / `latest_heartbeat_snapshot` to the g8es document store, then notifies g8ed via HTTP POST so g8ed can broadcast the metrics envelope over SSE.

Staleness reconciliation is owned by g8ee: `HeartbeatStaleMonitorService` (`app/services/operator/heartbeat_stale_monitor.py`) runs on a timer inside g8ee and reconciles operator `status` against the age of `last_heartbeat`. Transitions are bidirectional:

- `BOUND → STALE` and `ACTIVE → OFFLINE` when `(now - last_heartbeat) > 60s`
- `STALE → BOUND` and `OFFLINE → ACTIVE` when a fresh heartbeat resumes

On each transition the updated status is persisted via g8ee's `CacheAsideService` and an `OPERATOR_STATUS_UPDATED_*` SSE event is published to g8ed for fanning out to the owning user's active sessions.

```
g8eo (every 30s)
    │ WebSocket
    ▼
g8es pub/sub  →  g8ee (OperatorHeartbeatService)
                      │ write last_heartbeat, latest_heartbeat_snapshot to g8es
                      │ HTTP POST /api/internal/sse/push
                      ▼
                    g8ed
                      │ broadcast SSE operator.heartbeat
                      │ HeartbeatMonitorService (timer) reconciles status:
                      │   BOUND↔STALE, ACTIVE↔OFFLINE based on last_heartbeat age
                      │ broadcast SSE operator.status.updated.{stale|bound|offline|active}
                      ▼
                    Browser (operator-panel.js)
                      └── uses data.status + data.status_class directly
```

**g8es document store fields (managed by g8e):**
- `last_heartbeat` — timestamp of most recent heartbeat
- `heartbeat_history` — rolling buffer of last 10 heartbeats
- `latest_heartbeat_snapshot` — most recent metrics for UI
- `system_info` — static system data

**Heartbeat SSE envelope (g8ee → g8ed → browser):** canonical shape in `shared/models/wire/heartbeat_sse.json`. The envelope splits authorship:

- **Envelope (g8ee-owned):** `operator_id`, `status` (authoritative value from `OperatorDocument`).
- **`metrics` (g8eo-authored telemetry):** `timestamp`, `heartbeat_type`, system identity (`hostname`, `os`, `architecture`, `cpu_count`, `memory_mb`, `current_user`), performance (`cpu_percent`, `memory_percent`, `disk_percent`, `network_latency`, `memory_used_mb`, `memory_total_mb`, `disk_used_gb`, `disk_total_gb`), network (`public_ip`, `internal_ip`, `interfaces`), uptime (`uptime`, `uptime_seconds`), version (`operator_version`, `version_status`), capability flags (`local_storage_enabled`, `git_available`, `ledger_enabled`), and nested detail objects (`os_details`, `user_details`, `disk_details`, `memory_details`, `environment`).

The typed producer is `HeartbeatSSEEnvelope` in `components/g8ee/app/models/operators.py`. The browser reads `data.operator_id`, `data.status`, and `data.metrics.*`.

### Batch Command Approval

When a command targets multiple operators, g8ed shows a **unified approval dialog** — one approval covers all impacted systems.

**Flow:**
1. g8ee sends approval request with `target_systems` array and `is_batch_execution=True`
2. g8ed displays a single approval UI listing all impacted hostnames
3. User clicks "Approve for N Systems" — single POST to `/api/operator/approval/respond`
4. g8ee fans out the command to each operator in parallel (bounded by `command_validation.max_batch_concurrency`), correlating per-operator events by a shared `batch_id`

### Anchored Operator Terminal

The chat interface includes a persistent terminal anchored at the bottom of the chat view, providing SSH-like direct command execution without AI involvement.

**Direct command flow:**
```
User types command
    → POST /api/operator/approval/direct-command
    → g8ed validates session, gets bound operator
    → **`OperatorService.relay.relayDirectCommandToG8ee()`**
    → POST to g8ee `/api/internal/operator/direct-command`
    → g8ee publishes to g8es pub/sub cmd:{operator_id}:{session_id}
    → g8eo executes command
    → result → g8ee → HTTP POST to g8ed → SSE → terminal
```

All operator events (approval requests, execution output, intent results) are routed to the anchored terminal via `chat.js` event routing methods.

---

## Operator Binary Distribution

Operator binaries are stored in the g8es blob store (namespace `operator-binary`). g8ed has no local binary storage — every download request fetches directly from g8es's blob store via `OperatorDownloadService` (`GET /blob/operator-binary/{os}-{arch}`).

**Supported platforms:** Linux amd64, arm64, 386

**Authentication:** `Authorization: Bearer <api_key>` (Operator API key or Device Link token)

**Download auth is handled by `DownloadAuthService`** (`services/auth/download_auth_service.js`). All download routes call `getDownloadAuthService().validate(req)` — there is no inline auth logic in the route handlers. The service handles three token branches in priority order:

| Token type | Source | Consumed on use |
|---|---|---|
| `dlk_` Device Link token | g8es KV (ephemeral) | No |
| `g8e_key` | User document (`users` collection) — looked up via `UserService.getUserByApiKey()` | No |
| Operator API key | g8es document store via `ApiKeyService` | No |

`dlk_` tokens are matched first by prefix. All other tokens are checked against the user document `g8e_key` field first; if no match, the token is validated as an operator API key. `DownloadAuthService` is initialized in Phase 3 of `services/initialization.js` with `{ cacheAside, userService, apiKeyService }` and accessed via `getDownloadAuthService()`.

**Binary availability** is reported at `GET /operator/health` (internal only).

### Device Link — Pre-Authorized Deployment

Device Link is the **recommended** way to deploy operators. The user generates a time-limited token in the Operator Panel; the token is embedded in a ready-to-run command for the target system.

**User flow:**
1. User clicks "Add Operator" → "Device Link" in the Operator Panel
2. User specifies the number of operator slots needed (`max_uses`)
3. g8ed generates a `dlk_<32-char>` token and automatically provisions any missing operator slots to fulfill the requested slot limit
4. g8ed returns an operator command with the device link token
5. User runs the command on the target system
6. Binary collects system fingerprint and registers with g8ed (operator does not need to know its `operator_id` beforehand)
7. g8ed assigns an available operator slot, generates credentials, and returns `{ api_key, operator_cert, operator_cert_key, operator_id }` to the binary
8. Operator uses the returned API key for the bootstrap process and activates immediately
9. Operator appears in the panel as active

**Token format:** `dlk_[A-Za-z0-9_-]{32}` — 24 cryptographically random bytes, validated by regex before any g8es operations.

**Token statuses:**

| Status | Description |
|--------|-------------|
| `active` | Valid, has remaining uses |
| `pending` | Created but not yet claimed |
| `used` | Single-use token that has been consumed |
| `exhausted` | All uses consumed (`max_uses` reached) |
| `expired` | TTL elapsed |
| `revoked` | User manually revoked |

**Device Link Token Format:** `dlk_[A-Za-z0-9_-]{32}` — 24 cryptographically random bytes.

**Security controls:**
- Token format validated before any g8es operations (injection prevention)
- Time-limited TTL (1 min–7 days, default 1h)
- Count-limited claims (`max_uses` 1–10,000, default 1)
- Same system cannot claim the same link twice (fingerprint dedup)
- Atomic claim via g8es KV prevents race conditions
- Slot provisioning — uses an existing AVAILABLE slot or creates one on-demand if none exist
- Instant revocation via `DELETE /api/device-links/:token`
- All events logged to audit trail

---

## Response Model Architecture

g8ed uses structured response models instead of generic `data` containers to ensure type safety and self-documenting APIs. All response models extend `G8eBaseModel` and declare explicit fields.

### G8eBaseModel Constructor Pattern

The `G8eBaseModel` constructor uses `fields` as the parameter name (not `data`) to prevent the anti-pattern of generic data containers:

```javascript
// Correct - uses structured fields
new UserStatsResponse({
  success: true,
  total: 10,
  activity: { lastDay: 5 },
  newUsersLastWeek: 2
})

// Incorrect - old pattern (eliminated)
new ConsoleDataResponse({
  success: true,
  data: { total: 10, activity: { lastDay: 5 } }  // Generic container
})
```

### Response Model Categories

**Console Metrics Responses** - Each console endpoint has its own response model:
- `PlatformOverviewResponse` - Platform-wide metrics snapshot
- `UserStatsResponse` - User activity and growth statistics  
- `OperatorStatsResponse` - Operator status and health metrics
- `SessionStatsResponse` - Web and operator session counts
- `AIUsageStatsResponse` - Investigation and AI usage metrics
- `LoginAuditStatsResponse` - Security event statistics
- `RealTimeMetricsResponse` - Live system performance data
- `ComponentHealthResponse` - Service health status

**Data Access Responses** - For KV and database operations:
- `KVScanResponse` - Cursor-based KV key iteration
- `KVKeyResponse` - Individual KV key lookup with metadata
- `DBQueryResponse` - Document collection queries
- `DBCollectionsResponse` - Collection registry listing

**Generic Responses** - Reusable across endpoints:
- `SimpleSuccessResponse` - Basic success confirmation
- `ErrorResponse` - Error information (no `data` field)
- `ChatMessageResponse` - Chat responses (intentionally generic `data` field for different response types)

### Migration Notes

The following deprecated patterns have been eliminated:
- `ConsoleDataResponse` - replaced with specific response models
- Generic `data` fields in response models - replaced with explicit fields
- `ErrorResponse.data` field - removed to prevent future anti-patterns

All new response models must declare explicit fields and avoid generic containers.

---

## Frontend Model Architecture

The browser uses a parallel model tier to the server-side `G8eBaseModel` hierarchy. Frontend models extend `FrontendBaseModel` (from `public/js/models/base.js`) and are used for data received from the wire (SSE events, API responses) in the browser.

### FrontendBaseModel

`FrontendBaseModel` mirrors the server-side `G8eBaseModel` API with browser-specific adaptations:

- **Field definition:** Uses the same `F` tokens (`string`, `boolean`, `number`, `date`, `object`, `array`, `any`)
- **Validation:** `parse(raw)` validates and coerces incoming wire data, throwing on validation errors
- **Serialization:** `forWire()` converts models to plain JSON (Date → ISO string) for outbound fetch
- **Date handling:** Date objects live inside the application boundary; serialized to ISO strings by `forWire()`

### Frontend Model Files

Browser-side models are organized in `public/js/models/`:

| File | Purpose |
|------|---------|
| `base.js` | `FrontendBaseModel` and `FrontendIdentifiableModel` base classes |
| `agent-models.js` | AI pipeline result models (TriageResult, CommandGenerationResult, etc.) |
| `operator-models.js` | Operator domain models for browser (HeartbeatSnapshot, etc.) |

### Boundary Enforcement

**Critical rule:** Browser code must never import from server-side directories. All imports in `public/js/**/*.js` must resolve to files within `public/js/`. Cross-boundary imports (e.g., `../../../models/operator_model.js`) cause 404 errors because server files are outside the Express static root (`components/g8ed/public/`).

When adding a new model needed by the browser:
1. Create a browser-side model in `public/js/models/` extending `FrontendBaseModel`
2. Mirror the server model's fields and factory methods (e.g., `empty()`, `fromHeartbeat()`)
3. Add frontend unit tests in `test/unit/frontend/models/`
4. Update browser imports to use the new browser-side path

### Example: HeartbeatSnapshot

The `HeartbeatSnapshot` model in `public/js/models/operator-models.js` mirrors the server-side model:

```javascript
export class HeartbeatSnapshot extends FrontendBaseModel {
    static fields = {
        timestamp:       { type: F.date,   default: null },
        cpu_percent:     { type: F.number, default: null },
        memory_percent:  { type: F.number, default: null },
        disk_percent:    { type: F.number, default: null },
        network_latency: { type: F.any,    default: null },
        uptime:          { type: F.any,    default: null },
        uptime_seconds:  { type: F.any,    default: null },
    };

    static empty() {
        return HeartbeatSnapshot.parse({});
    }

    static fromHeartbeat(heartbeat, timestamp) {
        const hb = heartbeat || {};
        const perf = hb.performance_metrics || {};
        const uptime = hb.uptime_info || {};

        return HeartbeatSnapshot.parse({
            timestamp,
            cpu_percent:     perf.cpu_percent ?? null,
            memory_percent:  perf.memory_percent ?? null,
            disk_percent:    perf.disk_percent ?? null,
            network_latency: perf.network_latency ?? null,
            uptime:          uptime.uptime ?? null,
            uptime_seconds:  uptime.uptime_seconds ?? null,
        });
    }
}
```

This model is used by `operator-panel.js` to process heartbeat SSE events in the browser.

---

## Attachments

Attachments uploaded with a chat message are stored in the g8es Blob Store. Metadata is stored in g8es KV at `g8e:investigation:{inv_id}:attachment:{att_id}` (1h TTL). An ordered index of attachment IDs for the investigation is maintained at `g8e:investigation:{inv_id}:attachment.index`.

### Attachment Lifecycle

| Phase | What happens |
|---|---|
| **Upload** | `AttachmentService.storeAttachments()` validates, sanitizes filename, writes binary to g8es Blob Store, then writes metadata to g8es KV |
| **Metadata read** | `AttachmentService.getAttachment(kvKey)` — returns `AttachmentRecord` |
| **Data read** | `AttachmentService.getAttachmentWithData(kvKey)` — hydrates base64 from Blob Store |
| **Investigation delete** | `AttachmentService.deleteAttachmentsForInvestigation()` — deletes KV keys and Blob Store objects |

### Constraints

| Constraint | Value | Source |
|---|---|---|
| Max single file size | 10 MB | `MAX_ATTACHMENT_SIZE` in `constants/service_config.js` |
| Max total per investigation | 30 MB | `MAX_TOTAL_ATTACHMENT_SIZE` |
| Allowed content types | `text/plain`, `application/pdf`, `image/png`, `image/jpeg`, `image/gif`, `image/webp` | `ALLOWED_ATTACHMENT_CONTENT_TYPES` |
| KV metadata TTL | 1 hour | `CacheTTL.ATTACHMENT` |

### Initialization

`AttachmentService` is initialized in **Phase 5** of `services/initialization.js`. The service is accessible via `getAttachmentService()` from `initialization.js`.

---

## Security Model

For the full deep-reference security documentation covering internal auth token, SSL/CA handling, web session security, operator session security, operator auth methods, heartbeat security, operator binding implementation, Sentinel scrubbing, LFAA vault encryption, and the Ledger, see [architecture/security.md](../architecture/security.md).

### Authorization Layers

1. **Authentication** — HttpOnly `web_session_id` cookie or Bearer API key; `SameSite=Lax` provides CSRF protection without tokens
2. **Ownership validation** — users can only access their own operators and sessions
3. **Role-based access** — admin-only routes gated by `requireAdmin`/`requirePageAdmin`
4. **Internal origin restriction** — cluster-internal endpoints verified via `X-Internal-Auth` header using `crypto.timingSafeEqual`

### Security Middleware (in `middleware/authorization.js`)

| Middleware | Purpose |
|------------|---------|
| `requireAdmin` | Requires `admin` or `superadmin` role |
| `requireSuperAdmin` | Requires `superadmin` role |
| `requirePageAuth` | `requireAuth` variant for HTML pages (redirects instead of JSON error) |
| `requirePageAdmin` | `requireAdmin` variant for HTML pages |
| `requireOwnership` | Validates `user_id` in request matches authenticated session |
| `requireOperatorOwnership` | Validates operator belongs to authenticated user |
| `requireSessionOwnership` | Validates session belongs to authenticated user |
| `requireInternalOrigin` | Restricts endpoint to cluster-internal calls only (verified via `X-Internal-Auth`) |
| `optionalAuth` | Attaches session if present but does not require it |
| `requireFirstRun` | Restricts endpoint to initial setup phase |

### Endpoint Access Tiers

| Tier | Examples |
|------|---------|
| **Public** | `GET /`, passkey endpoints (unauthenticated setup flow + auth flow), static assets |
| **Authenticated** | All `/api/user/*`, `/api/operators/*`, `/api/chat/*`, `/api/audit/*`, `/sse/events`, `/api/device-links` |
| **Admin only** | All `/api/console/*`, `/api/settings` |
| **Internal only** | All `/api/internal/*`, `/health/store`, `/health/details`, `/health/cache-stats` |

---

## API Route Reference

### Public Routes

| Route | Method | Purpose |
|-------|--------|---------|
| `/` | GET | Landing page — login or first-run redirect |
| `/setup` | GET | First-run setup wizard (redirects to `/` when users exist). The canonical LLM provider catalog (`components/g8ed/constants/ai.js`) is injected into the template as a JSON script tag and consumed by `setup-page.js` |
| `/health` | GET | Simple health check |
| `/health/live` | GET | Liveness probe |
| `/api/auth/register` | POST | Create user account and return a passkey registration challenge. During first-run this branches into `setupService.performFirstRunSetup` to atomically create the initial superadmin; otherwise it creates an ordinary user. Body: `{ email, name, settings }`. Returns `{ user_id, challenge_options }` |
| `/api/auth/validate` | POST | Validate current session |
| `/api/auth/passkey/register-challenge` | POST | Generate a passkey registration challenge for post-setup add-passkey flows |
| `/api/auth/passkey/register-verify-setup` | POST | First-run only — verify the attestation for the admin user created by `/api/auth/register`, persist the credential, and return the initial session. Rejected once any user exists |
| `/api/auth/passkey/register-verify-initial` | POST | First-passkey-for-a-new-user flow (no session yet). Rejected if the user already has a credential |
| `/api/auth/passkey/register-verify` | POST | Authenticated add-passkey flow — appends a new credential to the current user |
| `/api/auth/passkey/auth-challenge` | POST | Generate passkey authentication challenge |
| `/api/auth/passkey/auth-verify` | POST | Verify passkey and set session cookie |
| `/auth/link/:token/register` | POST | Device registration (public, rate limited) |

### Authenticated Routes

**Auth & Session**

| Route | Method | Purpose |
|-------|--------|---------|
| `/api/auth/web-session` | GET | Get current session (`authenticated: true`, session object) |
| `/api/auth/logout` | POST | End session |
| `/api/auth/link/generate` | POST | Generate device link token |
| `/api/auth/link/:token/authorize` | POST | Approve a pending device link (DLT owner required) |
| `/api/auth/link/:token/reject` | POST | Reject a pending device link (DLT owner required) |
| `/api/auth/operator` | POST | g8eo Operator authentication (API key) |
| `/api/auth/operator/refresh` | POST | Refresh Operator session |
| `/api/auth/admin/locked-accounts` | GET | List locked accounts (admin) |
| `/api/auth/admin/unlock-account` | POST | Unlock account (admin) |
| `/api/auth/admin/account-status/:id` | GET | Check account lock status (admin) |

**Operators**

| Route | Method | Purpose |
|-------|--------|---------|
| `/api/operators/bind` | POST | Bind operator to web session |
| `/api/operators/bind-all` | POST | Bind multiple operators |
| `/api/operators/unbind` | POST | Unbind operator from session |
| `/api/operators/unbind-all` | POST | Unbind multiple operators |
| `/api/operators/:id/details` | GET | Get operator details (ownership required) |
| `/api/operators/:id/api-key` | GET | Fetch operator API key (ownership required) |
| `/api/operators/:id/refresh-api-key` | POST | Refresh operator API key |
| `/api/operators/:id/stop` | POST | Stop operator (ownership required) |
| `/api/operators/g8ep/reauth` | POST | Relaunch user's g8ep operator |
| `/api/operators/health` | GET | Operator binary availability health check |
| `/api/operator/approval/respond` | POST | Approve or deny command request |
| `/api/operator/approval/direct-command` | POST | Execute direct command on bound operator |

**Chat & Cases**

| Route | Method | Purpose |
|-------|--------|---------|
| `/api/chat/send` | POST | Send chat message to g8ee; returns case/investigation IDs immediately; AI response delivered via SSE |
| `/api/chat/stop` | POST | Stop active AI processing |
| `/api/chat/cases/:caseId` | DELETE | Delete case and all related data |
| `/api/chat/investigations` | GET | Query investigations for current user |
| `/api/chat/investigations/:investigationId` | GET | Get single investigation |
| `/api/chat/health` | GET | Chat routes health |

**Audit Log**

| Route | Method | Purpose |
|-------|--------|---------|
| `/api/audit/events` | GET | Fetch audit events as plain JSON (`limit`, `offset`, `type`, `actor`, `from`, `to`) |
| `/api/audit/download` | GET | Download audit log as JSON or CSV |

**User, Settings, Docs**

| Route | Method | Purpose |
|-------|--------|---------|
| `/api/user/me` | GET | Get current user profile |
| `/api/user/me/refresh-g8e-key` | POST | Refresh (rotate) the user's download API key |
| `/api/user/me/dev-logs` | PATCH | Toggle dev logging (admin only) |
| `/api/settings` | GET | Get platform settings (admin only) |
| `/api/settings` | PUT | Update platform settings (admin only) |
| `/api/docs/tree` | GET | Get documentation tree |
| `/api/docs/file` | GET | Get documentation file content |
| `/api/system/network-interfaces` | GET | Get host network interfaces |
| `/api/device-links` | POST | Create device link |
| `/api/device-links` | GET | List device links |
| `/api/device-links/:token` | DELETE | Revoke/delete device link |

**Operator Binary**

| Route | Method | Purpose |
|-------|--------|---------|
| `/operator/download/:os/:arch` | GET | Download operator binary (API key or DLT) |
| `/operator/download/:os/:arch/sha256` | GET | Download binary checksum |

**Admin Console**

All console endpoints now return structured response models with explicit fields instead of generic `data` containers. Each endpoint has its own response model with typed fields.

| Route | Method | Purpose | Response Model |
|-------|--------|---------|----------------|
| `/api/console/overview` | GET | Platform overview | `PlatformOverviewResponse` |
| `/api/console/metrics/users` | GET | User statistics | `UserStatsResponse` |
| `/api/console/metrics/operators` | GET | Operator statistics | `OperatorStatsResponse` |
| `/api/console/metrics/sessions` | GET | Session statistics | `SessionStatsResponse` |
| `/api/console/metrics/ai` | GET | AI usage statistics | `AIUsageStatsResponse` |
| `/api/console/metrics/login-audit` | GET | Login audit trail | `LoginAuditStatsResponse` |
| `/api/console/metrics/realtime` | GET | Real-time metrics (uncached) | `RealTimeMetricsResponse` |
| `/api/console/cache/clear` | POST | Force-refresh metrics cache | `SimpleSuccessResponse` |
| `/api/console/components/health` | GET | Live component health | `ComponentHealthResponse` |
| `/api/console/kv/scan` | GET | Cursor-based KV key scan | `KVScanResponse` |
| `/api/console/kv/key` | GET | Single KV key value + TTL | `KVKeyResponse` |
| `/api/console/db/collections` | GET | List valid collection names | `DBCollectionsResponse` |
| `/api/console/db/query` | GET | Query documents from a collection | `DBQueryResponse` |
| `/api/console/logs/stream` | GET | SSE stream — replays ring buffer then streams live entries | SSE events (no response model) |

**SSE**

| Route | Method | Purpose |
|-------|--------|---------|
| `/sse/events` | GET | Establish SSE connection |

**Frontend Pages**

| Route | Purpose |
|-------|---------|
| `/setup` | First-run setup wizard (public, redirects to `/` once users exist) |
| `/chat` | Chat interface (authenticated) |
| `/docs` | Documentation page (authenticated) |
| `/audit` | Audit log page (authenticated) |
| `/settings` | Platform settings (admin only) |
| `/console` | Admin console (admin only) |

### Internal-Only Routes (Cluster Access)

All guarded by `requireInternalOrigin` which validates `X-Internal-Auth` using `crypto.timingSafeEqual`.

| Route | Method | Purpose |
|-------|--------|---------|
| `/api/internal/sse/push` | POST | g8ee pushes SSE events to g8ed |
| `/api/internal/health` | GET | Internal API health |
| `/api/internal/operators/user/:userId` | GET | List user operators |
| `/api/internal/operators/user/:userId/initialize-slots` | POST | Initialize operator slots |
| `/api/internal/operators/user/:userId/reauth` | POST | Relaunch g8ep operator |
| `/api/internal/operators/:id/status` | GET | Get operator status |
| `/api/internal/operators/:id` | GET | Get operator details |
| `/api/internal/operators/:id/with-session-context` | GET | Get operator with session context |
| `/api/internal/operators/:id/context` | POST | Update operator context |
| `/api/internal/operators/:id/reset-cache` | POST | Reset operator to fresh state |
| `/api/internal/session/:sessionId` | GET | Get session by ID |
| `/api/internal/users` | GET / POST | List or create users |
| `/api/internal/users/stats` | GET | User statistics |
| `/api/internal/users/:userId` | GET / DELETE | Get or delete user |
| `/api/internal/users/email/:email` | GET | Get user by email |
| `/api/internal/users/:userId/passkeys` | GET | List passkey credentials |
| `/api/internal/users/:userId/passkeys/:credentialId` | DELETE | Revoke a specific passkey |
| `/api/internal/users/:userId/passkeys` | DELETE | Revoke all passkeys |
| `/api/internal/users/:userId/roles` | PATCH | Update user roles |
| `/api/internal/device-links/user/:userId` | GET | List device links for user |
| `/api/internal/device-links/user/:userId` | POST | Create device link for user |
| `/api/internal/device-links/:token` | DELETE | Revoke device link |
| `/health/store` | GET | Readiness probe (g8es connectivity) |
| `/health/details` | GET | Detailed health with service status |
| `/health/cache-stats` | GET | Cache performance metrics |
| `/api/chat/health` | GET | Chat routes health |
| `/api/metrics/health` | GET | System health metrics |
| `/api/system/network-interfaces` | GET | Get host network interfaces |
| `/operator/health` | GET | Operator binary availability (alias for /api/operators/health) |
| `/sse/health` | GET | SSE service health and connection metrics |

---

## Frontend

### Authentication Flow

Session validation runs automatically on every page load. `nav-public.ejs` injects an inline module that instantiates `AuthManager` and calls `init()`, which calls `validateSession()` against `/api/auth/validate`. The `/chat` page additionally initializes `AuthManager` via `app.js`.

- `window.authState` is the global `AuthManager` instance — correct entry point for `passkeyLogin()`, `startPasskeyRegistration()`, `logout()`, and auth state subscriptions
- `webSessionService` (`public/js/utils/web-session-service.js`) — for reading session state: session model, web session ID, API key, auth status
- `operatorSessionService` (`public/js/utils/operator-session-service.js`) — for reading operator session state: bound operator list, bound operator ID

**Rule:** Components that only need session data must use `webSessionService` or `operatorSessionService` — not `window.authState.getState()`.

### Navigation Partial System

All public-facing pages use `views/partials/nav-public.ejs`. Standard navigation structure: Product, Company, Trust, Legal. When a user authenticates, `body.user-authenticated` is added and the nav categories menu is hidden via CSS; the operator panel overlays content below the header.

`nav-public.ejs` exposes `window.navAuthManager`. Pages that need to trigger the auth modal must reuse this instance rather than creating a second `AuthManager`.

`service-client.js` must be loaded before `nav-public.ejs` renders its inline module — it is included in `head-public.ejs` for all public pages, and in the `<head>` of `chat.ejs`. Never move it to the bottom of `<body>` on any page that uses `nav-public.ejs`.

### Theme System

Dark mode is the default. The full flow:

1. **Server middleware** reads the `theme` cookie on every request; falls back to `'dark'`. Sets `res.locals.theme`.
2. **EJS templates** render `<body data-theme="...">` from `res.locals.theme` — correct theme is in the HTML from first byte, no flash.
3. **`theme-manager.js`** (non-module, in `<head>`) persists the server-rendered value to the cookie on `DOMContentLoaded`.
4. **Cookie:** `theme=dark|light; path=/; max-age=31536000; SameSite=Lax` — written by `ThemeManager` only, never by server code.
5. **CSS:** `:root` defines dark-mode as the baseline; `[data-theme="light"]` overrides the differing subset. There is no `[data-theme="dark"]` block.

### SSE Connection

The browser establishes a single long-lived SSE connection via `SSEConnectionManager` (`public/js/utils/sse-connection-manager.js`). It connects to `ApiPaths.sse.events()` (`/sse/events`) with `withCredentials: true` so the HttpOnly session cookie is sent automatically — the session ID is never passed in the URL.

Key behaviors:
- Exponential backoff with ±25% jitter on reconnect (up to `SSEClientConfig.MAX_RECONNECT_ATTEMPTS`)
- Keepalive timeout (`SSEClientConfig.KEEPALIVE_TIMEOUT_MS` — 120s) force-closes half-open connections
- Tab visibility changes trigger reconnect on becoming visible if the connection is dead
- `EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED` and `EventType.PLATFORM_SSE_KEEPALIVE_SENT` are silently discarded via `_INFRASTRUCTURE_EVENTS` — never forwarded to the `EventBus`
- All other events are emitted directly on the `EventBus` by event type string; components subscribe via `EventType.*` constants
- AUTH events (`g8e.v1.platform.auth.*`) are internal bus events only — the backend never sends them over SSE

`SSEConnectionManager` is the only component that may call `new EventType()`. All other components receive events via `EventBus` subscriptions.

#### SSE Event Naming

SSE event type constants follow a strict naming convention:

- **Backend** (`constants/events.js` — `EventType`): flat frozen object sourced from `shared/constants/events.json`; values are the wire strings sent to the browser
- **Frontend** (`public/js/constants/events.js` — `EventType`): auto-generated flat frozen object matching `shared/constants/events.json`; backend and frontend share the same flat key names
- **Infrastructure events**: `EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED` and `EventType.PLATFORM_SSE_KEEPALIVE_SENT` are silently discarded in `SSEConnectionManager` via `_INFRASTRUCTURE_EVENTS` — never forwarded to the `EventBus`

`SSEConnectionManager` emits all non-infrastructure events directly via `eventBus.emit(eventType, payload)` — there is no `SSE_EVENT_MAP`. All components subscribe to `EventType.*` constants on the `EventBus` directly.

`public/js/constants/sse-constants.js` contains only `SSEClientConfig` (reconnect timing/limits). All event type strings live in `public/js/constants/events.js`.

#### Operator SSE State (`OperatorPanel`)

`OperatorPanel` subscribes directly to canonical wire events — there is no intermediate aggregation layer. It takes only `eventBus` as a constructor parameter.

**Initialization order in `app.js`:**
1. `SSEConnectionManager` constructed — registers the `EventType`
2. `OperatorPanel` constructed — immediately calls `_setupWireListeners()`, which subscribes to `EventType.OPERATOR_PANEL_LIST_UPDATED`, `EventType.OPERATOR_HEARTBEAT_RECEIVED`, and all `OPERATOR_STATUS_UPDATED_*` events on the `EventBus`
3. SSE connection opens (on `EventType.PLATFORM_AUTH_COMPONENT_INITIALIZED_AUTHSTATE`)
4. `OperatorPanel.init()` called (on `EventType.PLATFORM_AUTH_COMPONENT_INITIALIZED_CHAT`) — renders DOM, then applies any `_pendingRender` payload that arrived before render

`OperatorPanel` maintains the authoritative operator state snapshot directly (`_operators`, `_totalOperatorCount`, `_activeOperatorCount`, `_usedSlots`, `_maxSlots`). Events that arrive before `init()` completes are buffered in `_pendingRender` and applied immediately after render — no data is lost and no DOM methods are called before the panel is rendered.

### API Paths (Frontend)

All frontend HTTP endpoint strings are defined in `public/js/constants/api-paths.js` via the `ApiPaths` builder object. Never hardcode path strings in any browser component — import `ApiPaths` and call the appropriate builder.

```js
import { ApiPaths } from '../constants/api-paths.js';
serviceClient.get(ComponentName.G8ED, ApiPaths.operator.list());
new EventType(ApiPaths.sse.events(), { withCredentials: true });
```

`ApiPaths` mirrors the server-side `constants/api_paths.js`. When a new backend route is added, a corresponding frontend builder must be added to `api-paths.js` before any component references that path.

### Template Architecture

g8ed separates HTML templates from JS component logic. Templates live in `public/js/components/templates/` as `.html` files and are loaded via `templateLoader` (`public/js/utils/template-loader.js`).

- Templates preload in the component constructor (async, non-blocking)
- All render operations are synchronous once templates are loaded
- Template variable syntax: `{{variableName}}`
- Components emit events when templates become available, eliminating race conditions

Key operator panel components: `operator-panel.js` (orchestrator), with mixins applied via `Object.assign(OperatorPanel.prototype, ...)` for download, device link, bind, device auth, layout, list rendering, and metrics display. The panel receives `operatorSSEHandler` as a constructor parameter and never reads from `window.operatorSSEHandler` directly.

### CSP-Compliant Styling

g8ed uses Content Security Policy with dynamic nonces. All styles must be in CSS files — no inline `style=` attributes, no `style.display` manipulation, no dynamically created `<style>` elements.

| Class | Purpose |
|-------|---------|
| `.initially-hidden` | Hide elements (replaces `style="display: none;"`) |
| `.visible` | Show with opacity |
| `.icon-spin` | Spinning animation |
| `.icon-14`, `.icon-16`, `.icon-18` | Material icon sizes |

Component-specific styles go in dedicated CSS files (`chat.css`, `operator-panel.css`, etc.). Utility styles go in `components.css`.

### File Attachments

g8ed uses a **local-first attachment model**. Files are read as base64 on the client and stored in the g8es Blob Store by g8ed. Only metadata (with g8es key references) is forwarded to G8EE.

**Flow:**
1. Client reads file via `FileReader` as base64; image previews use `data:` URIs (CSP-compliant — no `blob:` URLs)
2. On send: g8ed receives base64, stores in g8es Blob Store via `AttachmentService`, then writes metadata to g8es KV. Forwards only metadata + `kv_key` to g8ee.
3. g8ee retrieves base64 from the g8es Blob Store on demand (via metadata hydration) for LLM processing.

**Security controls:**
- Type whitelist, 10 MB per file, 30 MB total per investigation
- Filename sanitization (special chars stripped, max 255 chars)
- Base64 format check + size cross-check against declared `file_size`
- g8es KV keys scoped to `attachment:{investigation_id}:{attachment_id}`
- Attachments auto-expire after 1 hour
- Max 3 attachments per message

### Audit Log

The audit log page (`/audit`) fetches all investigation history events via a plain JSON request to `GET /api/audit/events` (no SSE, no polling).

**Features:** complete history across all investigations, date filtering, JSON/CSV export, real-time stats.

**Event structure:** `investigation_id`, `case_id`, `case_title`, `event_type`, `actor` (user / g8e_ai / system), `summary`, `timestamp`, `details`, `source` (history_trail or conversation).

**Access:** profile dropdown → "Audit Log", or navigate directly to `/audit`.

### Investigation URL State

The chat interface persists investigation state in the URL (`/chat?investigation=<case_id>`), enabling page refresh to restore the active conversation. Navigation uses `pushState` / `replaceState` in `cases-manager.js`.

### Settings Page

The settings page (`/settings`) is admin-only. It combines:

- **Platform settings** — dynamic key/value config loaded from `/api/settings`, grouped into sections with a left-hand navigation. Changes are batched in a dirty map and flushed via `PUT /api/settings`. Only `USER_SETTINGS` keys are exposed here — `PLATFORM_SETTINGS` keys are deployment-time only and never shown in the UI.
- **User preferences** — per-user toggles rendered directly by EJS at page load. The `dev_logs_enabled` toggle calls `PATCH /api/user/me/dev-logs` and takes effect on the next full page load.

`window.__DEV_LOGS_ENABLED` is inlined before module scripts via `dev-logs-bootstrap.ejs` (included in `head-public.ejs`). `devLogger` (`public/js/utils/dev-logger.js`) reads this value at call time — not at import time.

---

## Health & Monitoring

| Endpoint | Access | Purpose |
|----------|--------|---------|
| `GET /health` | Public | Simple health check |
| `GET /health/live` | Public | Liveness probe |
| `GET /health/store` | Internal | Readiness probe (g8es connectivity) |
| `GET /health/details` | Internal | Detailed health with service status |
| `GET /health/cache-stats` | Internal | Cache performance metrics |
| `GET /api/chat/health` | Internal | Chat routes health |
| `GET /api/metrics/health` | Internal | System health metrics |
| `GET /operator/health` | Internal | Binary availability per platform |
| `GET /sse/health` | Internal | SSE connection metrics |

**Monitoring signals:**
- Active sessions, creation rate, timeout rate (admin console)
- Audit trail for security events
- g8es KV key count, pub/sub connections, document store size
- Rate limiting — blocked requests by endpoint
