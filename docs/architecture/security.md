# Security Architecture

g8e is a local-only, air-gapped, portable platform. It runs entirely via `docker-compose` on-premises — no cloud deployment, no SaaS backend, no external network dependency. This document is the deep-reference security guide for the platform, covering every enforcement layer in detail.

For component-level overviews, see: [g8ee](../components/g8ee.md), [VSOD](../components/vsod.md), [g8eo](../components/g8eo.md).

---

## Bedrock Principles

### User Intent

Every aspect of g8e security is designed around one foundational principle: the user makes informed decisions, and the platform exists to support that.

g8e operates in full context mode. The platform continuously learns the user's environment, history, preferences, and intent through Operators — building a working relationship over time where every session makes the assistant more capable and more personalized. The Operators give the user a level of situational awareness that is impossible without them: real-time system state, historical context across sessions, and AI-powered analysis grounded in what is actually happening on the user's infrastructure.

All of that capability exists in service of one outcome: the human is always the one making state-changing decisions. The AI is the robot on the user's shoulder — guiding, surfacing context, proposing actions — while the human traverses the environment and decides what happens next.

Human control is a first-class architectural property. Every enforcement mechanism in this document — Human-in-the-Loop approval, binding, Sentinel scrubbing, session isolation, least privilege — exists to ensure that the platform's growing capability and context remains fully in service of the user's intent.

### Additional Security Principles

1. **Zero Trust** — No component, connection, or data item is implicitly trusted. Every request is authenticated, every payload validated, every data stream scrubbed. Trust is never inherited from network position.
2. **Human-in-the-Loop** — The AI proposes; the human approves. No state-changing operation executes without explicit, informed user consent — enforced at the platform level, not bypassable by the AI or any API call.
3. **Least Privilege** — Every actor has the minimum access required to perform its function. Cloud Operators for AWS start with zero AWS access. Permissions are granted Just-in-Time and revoked immediately after use.
4. **Data Sovereignty** — Sensitive operational data stays on the Operator host by default. The platform is a stateless relay; it never stores raw command output. Only Sentinel-scrubbed metadata crosses component boundaries toward the AI.
5. **Defense in Depth** — No single control is relied upon. Authentication is layered (passkeys + sessions + context binding). Data protection is layered (TLS in transit + AES-256-GCM at rest + Sentinel scrubbing before AI). If one control fails, others hold.

---

## Platform Architecture Overview

```
Browser
  │  HTTPS + Passkey Auth (FIDO2/WebAuthn) + Encrypted Session Cookie
  ▼
VSOD  (Node.js — web gateway, SSE relay, Operator panel)
  │  Internal HTTP — X-Internal-Auth shared secret (constant-time comparison)
  ▼
g8ee   (Python/FastAPI — AI engine, Sentinel scrubbing, AI safety analysis)
  │  HTTP + WebSocket pub/sub
  ▼
VSODB (g8eo binary in --listen mode — document store, KV store, pub/sub broker)
  │
  │  WebSocket + mTLS + Certificate Pinning + Replay Protection
  ▼
g8eo   (Go binary — Operator agent on target host)
  │  Sentinel pre-execution, local SQLite vaults, Git Ledger (LFAA)
  ▼
Host Filesystem / AWS / Target System
```

### Security Boundaries

| Boundary | Mechanism |
|---|---|
| **Browser → VSOD** | HTTPS/TLS 1.3, FIDO2/WebAuthn passkeys, encrypted `HttpOnly` session cookie, `SameSite=lax` CSRF protection |
| **VSOD → g8ee** | Internal Docker network only, `X-Internal-Auth` shared secret (constant-time comparison), never exposed externally |
| **g8ee → VSODB** | Internal Docker network, `X-Internal-Auth` token (strictly enforced by VSODB/g8eo in `--listen` mode) |
| **VSOD → VSODB** | Internal Docker network, `X-Internal-Auth` token (strictly enforced by VSODB/g8eo in `--listen` mode) |
| **g8ee → LLM (AI)** | Sentinel-scrubbed data only — raw output, credentials, and PII never transmitted to any AI provider |
| **VSOD → g8eo** | WebSocket over mTLS (TLS 1.3), per-operator client certificate issued at claim time, platform CA fetched from hub at operator startup |
| **Operator → Host** | Sentinel pre-execution threat blocking, command allowlist/denylist, Human-in-the-Loop approval required for every state change |
| **Data at Rest (VSODB)** | SQLite at `0600` filesystem permissions (4 tables: documents, kv_store, sse_events, blobs); session fields encrypted at application layer by VSOD before persistence; **internal_auth_token persisted in SSL volume** |
| **Data at Rest (LFAA Vaults)** | AES-256-GCM field-level encryption (content, stdout, stderr); DEK envelope encryption; key derived on-demand from operator API key via HKDF-SHA256 |

### Network Isolation

g8e runs on a private Docker bridge network. Only two ports are bound to the host: **443** (TLS gateway for browser and Operator WebSocket) and optionally **80** (HTTP redirect). All other inter-service communication is internal and unreachable from outside. g8eo Operators initiate outbound-only connections to port 443; they open no inbound ports.

---

### Configuration Security and Precedence

g8e uses a layered configuration model designed for zero-trust local deployments. Configuration flows through a strictly enforced precedence chain, ensuring that sensitive values (like API keys and secrets) can be provided via the environment at deployment time but are managed via the platform's own persistence layer at runtime.

#### The Precedence Chain

All components resolve configuration values in the following order (highest priority wins):

1.  **User Settings (DB)**: Individual user overrides stored in VSODB `user_settings` collection.
2.  **Platform Settings (DB)**: Global platform values stored in VSODB `platform_settings` collection.
3.  **Environment Variables**: Canonical `G8E_*` variables provided at container runtime.
4.  **Schema Defaults**: Hardcoded safe defaults defined in the component's configuration service.

**Exception: Bootstrap Secrets**
For critical bootstrap secrets (`internal_auth_token`, `session_encryption_key`), the **Shared SSL Volume** is the absolute source of truth and the only place they are stored. They are never persisted in the database.

#### Bootstrap Secrets Handling

The platform handles two critical secrets that are required for component-to-component authentication and data protection:

- **`internal_auth_token`**: Shared secret for `X-Internal-Auth` header authentication.
- **`session_encryption_key`**: AES-256 key used to encrypt sensitive fields in web and operator sessions.

The `./g8e platform settings` command displays truncated versions of these active secrets (e.g., `f5037487...6c5f`) to confirm they are set and synchronized without exposing the full values.

**1. Authoritative Source (The Volume)**
The `g8es-ssl` volume (mounted at `/vsodb/ssl`) is the sole authoritative source of truth for these secrets. They are stored as plain-text files on this volume and are never written to the database.
- `internal_auth_token` is stored at `/vsodb/ssl/internal_auth_token`.
- `session_encryption_key` is stored at `/vsodb/ssl/session_encryption_key`.

**2. Generation and Persistence**
On the first platform start, **VSODB** (g8eo in `--listen` mode) checks if these secrets exist on the volume. If they are missing, VSODB generates cryptographically secure 32-byte hex values and writes them to the SSL volume files.

**3. Automatic Discovery**
VSOD and g8ee automatically discover these tokens by reading the files from the shared volume at startup. This enables a "zero-config" secure bootstrap without database dependencies for core identity.

**4. Elimination of DB Storage**
Unlike other configuration, these secrets are never stored in the `platform_settings` database document. This ensures that even a full database wipe or reset does not compromise the platform's internal authentication or session encryption keys, as long as the SSL volume is preserved.

#### Bootstrap Seeding

On the first start of a new deployment, the platform automatically **seeds** the persistent database with values found in the environment. This "Capture and Persist" strategy ensures that a deployment remains stable even if container environment variables are removed or modified after initialization.

#### Enforcement

- **Code Standard**: Direct reads from the host environment (e.g., `process.env` or `os.Getenv`) are strictly prohibited outside of the component's configuration service.
- **Transport Security**: Bootstrap transport URLs (`G8E_INTERNAL_HTTP_URL`, `G8E_INTERNAL_PUBSUB_URL`) are read once at startup to reach the database; all other configuration is then loaded from the database.
- **Sensitive Fields**: Sensitive fields in the database (API keys, session tokens) are application-layer encrypted before storage.

All `X-VSO-*` identity headers are **injected by VSOD from the verified server-side session** — never accepted from the untrusted request body. User identity cannot be forged by a client.

| Header | Purpose |
|---|---|
| `X-VSO-WebSession-ID` | Browser session identifier |
| `X-VSO-User-ID` | Authenticated user identity |
| `X-VSO-Organization-ID` | Multi-tenant isolation |
| `X-VSO-Case-ID` | Active case correlation |
| `X-VSO-Investigation-ID` | Active investigation correlation |
| `X-VSO-Task-ID` | Active task correlation |
| `X-VSO-Bound-Operators` | JSON array of all operators bound to the session |
| `X-VSO-Request-ID` | Per-request tracing identifier |
| `X-VSO-Source-Component` | Source component name (validated against `ComponentName` enum) |

---

## SSL/CA Certificate Generation and Handling

### Private Certificate Authority

g8e operates its own private CA. There is no dependency on any public CA.

- **Algorithm:** ECDSA with P-384. **Protocol:** TLS 1.3 only on all external and Operator-facing endpoints.
- **Generation:** CA and server certificates are generated at runtime by the VSODB operator binary (`--listen --ssl-dir /ssl` mode) on first start. Stored in the dedicated `g8es-ssl` volume (`/ssl` inside vsodb). Never baked into any Docker image.
- **Distribution to services:** The `g8es-ssl` named Docker volume is mounted read-only at `/vsodb/ssl` on VSOD, g8ee, and g8ep. Both services read CA and server certificates from `/vsodb/ssl/`. VSOD's `CertificateService` reads the CA cert and key from `/vsodb/ssl/ca/` to sign per-operator client certificates. These mounts are read-only.
- **Volume isolation:** SSL certs live in a dedicated volume (`g8es-ssl`) separate from the SQLite DB volume (`g8es-data`). `platform reset` wipes the DB volume but never touches the SSL volume — SSL certs survive a full rebuild without needing to be re-trusted.
- **CA trust for field operators:** The non-listen g8eo binary uses a **local-first** discovery strategy. When `--ca-url` is not set, it scans well-known volume mount paths (`/ssl/ca.crt`, `/vsodb/ca.crt`, `/vsodb/ssl/ca.crt`, `/data/ssl/ca.crt`) before attempting any network request. If no local file is found, it falls back to an HTTPS fetch from `https://<endpoint>/ssl/ca.crt` using the OS system trust store. The CA is never baked into the binary at compile time — there is no `//go:embed` and no `server_ca.crt` source file. This eliminates the circular dependency that caused x509 failures after a clean volume wipe: the operator always discovers the CA that VSODB actually generated, not a stale one. Inside the Docker network, the `vsodb-ssl` volume provides the CA at `/vsodb/ca.crt` — no network fetch occurs.
- **Per-operator client certificates:** Issued dynamically at claim time during Operator bootstrap, transmitted to the Operator exactly once. Not stored in recoverable form by VSOD.
- **Validity:** CA — 10 years (3650 days); server certificate — 90 days. Both renewed automatically on restart if expired.
- **CA private key:** Accessible only to the core authentication service; never exposed via any API.

### CA Trust Bootstrap and the TLS Kill Switch

The g8eo binary (field operator mode) loads the platform CA at startup using a two-stage strategy:

1. **Local discovery** (preferred): When `--ca-url` is not set, the binary scans `/ssl/ca.crt`, `/vsodb/ca.crt`, `/vsodb/ssl/ca.crt`, and `/data/ssl/ca.crt` in order. The first path that exists and contains valid PEM is accepted immediately via `certs.SetCA` — no network request is made. This is the normal path for containerized operators (e.g., g8ep), where the `vsodb-ssl` Docker volume provides the CA locally.
2. **Remote fetch** (fallback): If no local file is found, the binary fetches from `https://<endpoint>/ssl/ca.crt` (or the URL given by `--ca-url`) via `certs.FetchAndSetCA`. This uses Go's default `http.Client` with the OS system trust store, a 15-second timeout, and a 64 KB body limit. The fetch is equivalent to a certificate pinning operation — not a sensitive data transfer. For remote deployments using the [drop script](../components/vsod.md#operator-drop-script), the CA is pre-fetched over plain HTTP and passed to `curl --cacert` / `wget --ca-certificate` for the binary download — the operator binary then discovers it via local-first discovery or falls back to the standard HTTPS fetch.

Once stored in the runtime CA store, all subsequent TLS connections are verified against it. Public CAs are not trusted.

If both stages fail (no local file, hub unreachable, bad PEM, non-200 response), the operator exits immediately with `ExitConfigError`. If certificate verification fails at connection time, g8eo self-terminates with **exit code 7** (`ExitCertTrustFailure`). The connection is never downgraded, retried insecurely, or silently ignored.

### Mutual TLS (mTLS)

All Operator-to-VSOD connections require mutual TLS:
- **Server side:** VSOD presents its server certificate (signed by the platform CA).
- **Client side:** g8eo presents its per-operator client certificate (issued at claim time).

An Operator cannot connect without a valid platform-issued certificate. VSOD cannot be impersonated by anything not signed by the pinned CA.

### Workstation CA Trust

Because g8e uses a locally generated CA, users must configure their workstation browser and operating system to trust it before using the HTTPS UI. The platform exposes an HTTP onboarding portal on port 80 (`https://<host>`), which exists specifically to solve the trust bootstrap problem without browser certificate warnings. That page auto-selects the user's OS, presents a 1-Click Installer or raw `.crt` download as appropriate, and links trusted users forward to `https://<host>/setup`.

| OS | Installation Method |
|---|---|
| **macOS** | 1-Click Installer (`.sh`) downloads the CA from VSOD, removes old g8e certs, and installs the new CA into the system keychain. The raw `.crt` remains available for manual trust flows. |
| **Windows** | 1-Click Installer (`.bat`) self-elevates via UAC, downloads the CA from VSOD, removes old g8e certs, and installs the new CA via `certutil`. |
| **Linux** | 1-Click Installer (`.sh`) downloads the CA from VSOD, removes old g8e certs, copies the new CA into the system trust store, and refreshes trusted certificates. |
| **iOS** | Download the raw `.crt`, install the profile, then explicitly enable full trust in Certificate Trust Settings. |
| **Android** | Download the raw `.crt`, open it from Downloads, and install it as a CA certificate. |

The HTTP onboarding page is informational and bootstrap-only. Once the workstation trusts the CA, normal browser access moves to HTTPS on port 443. Operator traffic also depends on the same CA chain — browser HTTPS and Operator mTLS both anchor to the VSODB-generated platform CA.

### Certificate Revocation

Revoked certificate serials are tracked in-memory by `CertificateService` for the lifetime of the VSOD process. Revocation is triggered automatically by API key refresh, Operator decommission, or manual security response. Revoked serials are also recorded in the Operator document for audit.

---

## Web Session Security

Sessions are a critical security boundary — both web user sessions and Operator sessions are centrally managed by VSODB.

### Web Session Lifecycle

1. Created by VSOD on successful passkey verification.
2. Session ID is cryptographically secure and globally unique (`session_{timestamp}_{uuid}`).
3. Stored in VSODB KV with TTL; subject to both idle timeout (8 hours, configurable) and absolute hard lifetime (24 hours, configurable).
4. Transmitted to the browser as an encrypted, `HttpOnly`, `Secure`, `SameSite=lax` cookie.
5. Sensitive session metadata (`api_key` field) is encrypted with AES-256-GCM using `SESSION_ENCRYPTION_KEY` before write to VSODB.
6. On every request, the session is validated against the stored VSODB record and the client's context markers (IP, user-agent). IP changes are tracked; suspicious activity is flagged after 4+ IP changes.
7. Revocation is immediate — invalidating a session in VSODB takes effect on the next request.

### Web Session Security Properties

| Control | Value |
|---|---|
| Idle timeout | 8 hours (`SESSION_TTL_SECONDS`) |
| Absolute timeout | 24 hours (`ABSOLUTE_SESSION_TIMEOUT_SECONDS`) |
| Concurrent sessions | Tracked per user in VSODB KV (unlimited active allowed) |
| Cookie flags | `HttpOnly`, `Secure`, `SameSite=Lax` |
| CSRF protection | `SameSite=Lax` — no additional tokens required |
| `api_key` field | Encrypted with AES-256-GCM at application layer before VSODB write |

### `requireAuth` Middleware

`requireAuth` (`middleware/authentication.js`) is the single session validation point. It extracts the session ID from, in priority order: `web_session_id` HttpOnly cookie, `X-Session-Id` header, `Authorization: Bearer` header. After validation it attaches `req.webSessionId`, `req.session`, and `req.userId`. Route handlers must use these exclusively — never re-extract the session ID or call `validateSession()` again.

### `requireFirstRun` Middleware

`requireFirstRun` (`middleware/authentication.js`) guards the unauthenticated setup-flow registration endpoints. It calls `getSetupService().isFirstRun()` on every request. If users already exist it calls `next('route')`, causing Express to skip the setup-flow handler and fall through to the `requireAuth` handler registered on the same path (the add-passkey flow). This dual-handler pattern means the same endpoint serves two distinct flows without any branching inside the handler body — and without exposing an unauthenticated code path once the platform is initialized. If `isFirstRun()` throws, the request is rejected with 500 rather than silently passing through.

### HTTP Security Headers

Configured in `server.js` via Helmet:

| Header | Value |
|---|---|
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains; preload` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `SAMEORIGIN` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Cross-Origin-Opener-Policy` | `same-origin` |
| `Permissions-Policy` | Disables camera, microphone, geolocation |

CSP is managed via nonce-based middleware (not Helmet's built-in CSP). XSS prevention: all user-provided content and AI-generated responses are sanitized using an allowlist-based library before DOM rendering; `eval()`, `document.write()`, and other dangerous JavaScript APIs are prohibited; only a safe subset of HTML attributes is permitted in rendered content.

### Role-Based Access Control

g8e enforces a **deny-by-default** model. Absence of an explicit grant is a denial.

| Role | Access |
|---|---|
| **Standard User** | Own investigations, own Operators, own cases. No visibility into any other user's resources. |
| **Administrator** | All standard capabilities, plus: console access, user management, platform-wide monitoring, system configuration. |
| **Super-Administrator** | Full platform access including sensitive security operations and system-level maintenance. |

Role escalation is not possible through any public-facing API. Roles are validated server-side on every request. Every API call involving a resource ID first verifies the authenticated user is the legitimate owner at the service layer, not the routing layer.

### Rate Limiting

| Scope | Limit |
|---|---|
| Auth endpoints | 20 requests / 5 min |
| Chat endpoints | 30 messages / min |
| SSE connections | 30 attempts / 5 min (failed only) |
| General API | 60 requests / min |
| Global public API | 100 requests / min |

---

## Operator Session Security

### Operator Session Lifecycle

- Created on successful bootstrap authentication.
- Bound to both the Operator's API key and its permanent system fingerprint.
- Scoped to specific pub/sub channels: `cmd:{operator_id}:{operator_session_id}` (inbound commands), `results:{operator_id}:{operator_session_id}` (outbound results), `heartbeat:{operator_id}:{operator_session_id}` (heartbeat telemetry).
- `operator_id` field is encrypted with AES-256-GCM in the session document.
- Strictly isolated from web sessions — a user's web session and their bound Operator's session are separate security principals, linked via VSODB KV keys.
- API key refresh terminates the old Operator immediately, creates a new slot, and requires full re-authentication.

### Session Type Isolation

Web user sessions and Operator sessions are strictly partitioned. Authentication rules, timeout policies, and revocation mechanisms differ between the two session types. A compromised web session does not grant access to Operator commands — the Operator's own session and API key are separate credentials.

### Replay Protection

Every Operator request carries:
- `X-Request-Timestamp` — accepted within a ±5-minute window only.
- `X-Request-Nonce` (optional) — unique per request; validated against the nonce cache (in-memory by default; VSODB KV when configured, 10-minute TTL) to prevent replay of captured traffic.

Requests outside the timestamp window or with a replayed nonce are rejected outright.

---

## Operator Authentication Methods

Operators (g8eo) authenticate via one of three methods. All result in the same bootstrap outcome: a per-operator mTLS client certificate issued to the binary.

| Method | Use Case |
|---|---|
| **API Key** | Standard long-running Operator — pass `--key` or set `G8E_OPERATOR_API_KEY` |
| **Device Link (single-use)** | One-off automated deployment — token format `dlk_`, single use, time-limited |
| **Device Link (multi-use)** | Fleet deployment — configurable `max_uses` (1–10,000) and expiry (1 min–7 days) |

### Bootstrap Sequence (all methods)

1. Operator POSTs to `/api/auth/operator` with API key or device token.
2. VSOD verifies credentials and validates the system fingerprint binding (permanent after first use).
3. VSOD issues a per-operator mTLS client certificate and returns it with the operator session ID exactly once. The certificate is not stored in recoverable form by VSOD.
4. The Operator connects to VSODB pub/sub over WSS using that mTLS client certificate.

### System Fingerprint Binding

On first authentication, the Operator's system fingerprint (machine ID, CPU count, hostname) is permanently bound to that Operator slot. Any subsequent connection with a mismatched fingerprint is rejected. This binding is immutable and cannot be changed through any API — it prevents a stolen API key from being used on a different machine.

The machine ID component is resolved from the first available source in this order: `/etc/machine-id`, `/var/lib/dbus/machine-id`, `/proc/sys/kernel/random/boot_id`. The `boot_id` path is always present on any Linux kernel, including minimal Docker containers where no persistent machine-id file exists. On macOS the SystemConfiguration preferences plist is used instead.

### Duplicate Session Prevention

A second authentication attempt from the same system (matching fingerprint) is allowed as a restart. From a different system it is rejected until the existing session goes stale (no heartbeat for 60 seconds).

### Device Link Security

Device links are pre-signed, time-bounded authorization tokens that solve the bootstrap problem — how to authorize an Operator on a remote host without requiring the user to manually paste credentials — without leaving any long-lived credential exposed.

- Tokens are cryptographically random with the `dlk_` prefix and a strict, verifiable format (`dlk_[A-Za-z0-9_-]{32}`).
- Every token has both a `max_uses` ceiling and an absolute `expires_at` timestamp.
- Once consumed, the token is invalidated. Replaying a used token yields a rejection.
- Slot provisioning is atomic — concurrent multi-system deployments cannot race at the database level.
- After a device link is consumed, the only credential that matters is the Operator's API key — and that key lives only in process memory, never on disk.
- g8ep device links embed a `web_session_id` so `OPERATOR_STATUS_UPDATED` SSE events route to the correct browser tab.

### API Key Auth Layer

The API key path is the standard long-running operator authentication method, handled by `OperatorAuthService._authenticateViaApiKey` (`services/operator/operator_auth_service.js`).

**Dispatch condition:** g8eo sends `auth_mode=api_key` (or omits `auth_mode`) with `Authorization: Bearer <key>` header. `OperatorAuthService.authenticateOperator` routes to this path when `auth_mode !== AuthMode.OPERATOR_SESSION`.

**Validation sequence:**

1. **Header extraction** — API key extracted from `Authorization: Bearer <key>`. Missing or malformed header → 400.
2. **Key validation** — `ApiKeyService.validateApiKey` performs a cache-aside lookup (VSODB KV first, document store fallback). Checks: `vso_` prefix format, `status === ACTIVE`, `expires_at` not in the past. Invalid → 401.
3. **Download-only key rejection** — If the key has no `operator_id` binding (download-only key) → 403 with `DOWNLOAD_KEY_NOT_ALLOWED` code. Download keys (`G8E_DROP_KEY`) fetch the binary; they cannot authenticate an operator process.
4. **`last_used_at` update** — `ApiKeyService.updateLastUsed` called after successful validation (non-blocking; errors are logged and do not fail auth).
5. **User existence check** — `UserService.getUser(user_id)` from key data → 404 if not found.
6. **System fingerprint requirement** — `system_info.system_fingerprint` must be present in the request body → 400 if missing.
7. **Operator ownership check** — `operator.user_id` must match the authenticated `user_id` → 403 if not.
8. **Operator type immutability** — If the slot has an existing fingerprint + type, the requested type (system vs. cloud) must match → 403 with `OPERATOR_TYPE_MISMATCH` code. Operator type is permanent once set.
9. **Active operator reconnect check** — If the operator is `ACTIVE` or `BOUND`: reconnection is permitted only if the fingerprint matches (same-system restart) or the operator is stale (no heartbeat for >60s). Otherwise → 409.
10. **Fingerprint binding check** — If the operator is not active/bound: if a fingerprint is already stored and it does not match the incoming fingerprint → 403 with `FINGERPRINT_MISMATCH` code.
11. **Slot claim vs. reconnect** — `is_claiming_slot = true` when no fingerprint has been stored yet (first-ever auth). First auth claims the slot via `OperatorDataService.claimOperatorSlot` and receives the per-operator mTLS client certificate. Subsequent auths call `updateOperatorForReconnection`. If the operator was previously `BOUND`, its KV binding is refreshed to the new operator session ID.
12. **Session creation and activation** — Operator session created, operator activated, `OPERATOR_STATUS_UPDATED` SSE broadcast to all active user web sessions.
13. **Bootstrap response** — Returns `operator_session_id`, `operator_id`, `user_id`, `api_key` (echoed back for in-memory use), `config`, and `operator_cert`/`operator_cert_key` (first-claim only; `null` on reconnect).

| Check | Failure code | HTTP |
|---|---|---|
| Missing `Authorization: Bearer` | `Missing api_key` | 400 |
| Invalid/expired/inactive key | `Invalid API key` | 401 |
| Download-only key | `DOWNLOAD_KEY_NOT_ALLOWED` | 403 |
| User not found | `User not found` | 404 |
| Missing fingerprint | `System fingerprint required` | 400 |
| Wrong account | `Unauthorized` | 403 |
| Type mismatch | `OPERATOR_TYPE_MISMATCH` | 403 |
| Already active on different system | `Operator already active` | 409 |
| Fingerprint mismatch (offline) | `FINGERPRINT_MISMATCH` | 403 |
| Slot claim failure | `Failed to claim Operator slot` | 500 |

### Device Link Auth Layer

The Device Link path is a two-phase bootstrap: **registration** then **authentication**. It is handled by `DeviceLinkService.registerDevice` (phase 1) and `OperatorAuthService._authenticateViaDeviceLink` (phase 2).

**Dispatch condition:** g8eo sends `auth_mode=operator_session` with `operator_session_id` in the request body. `OperatorAuthService.authenticateOperator` routes to `_authenticateViaDeviceLink` when this condition is met.

#### Phase 1: Device Registration (`DeviceLinkService.registerDevice`)

Called by g8eo when it first presents a `--device-token dlk_...` token. This phase happens before the auth POST.

**Single-operator link (`PENDING` status):**
1. Token format validated against `dlk_[A-Za-z0-9_-]{32}`.
2. Token fetched from VSODB KV; expiry checked.
3. `REVOKED` → 403; `USED` → 403.
4. `DeviceRegistrationService.registerDevice` called with `operator_id` from the link data: creates operator session, claims or reconnects the pre-designated operator slot, activates the operator, fires `OPERATOR_STATUS_UPDATED` SSE to the linked web session.
5. Token status set to `USED` with device info recorded; token remains in KV until natural TTL expiry.

**Multi-use link (`ACTIVE` status):**
1. Same token format/expiry validation.
2. **Fingerprint dedup** — fingerprint added to `deviceLinkFingerprints(token)` SET via `kvSadd`. Returns 0 if already present → `DEVICE_ALREADY_REGISTERED`.
3. **Use counter** — atomic `kvIncr` on `deviceLinkUses(token)`. If count exceeds `max_uses`, counter and fingerprint are rolled back → `LINK_EXHAUSTED`.
4. **Distributed lock** — `kvSet(lockKey, value, PX, 10000, NX)` with retry loop (25 attempts × 200ms) prevents concurrent races on slot selection → `REGISTRATION_BUSY` on lock timeout.
5. **Slot assignment** — Finds an `AVAILABLE` operator slot for the user; creates a new slot if none exist. Both paths use `DeviceRegistrationService.registerDevice`.
6. **Claim tracking** — Claim appended to `linkData.claims`; `status` set to `EXHAUSTED` when `claims.length >= max_uses`.
7. Lock released in `finally` block (only if the lock value still matches — prevents stale release).

Both paths return `{ operator_session_id, operator_id }` on success.

#### Phase 2: Authentication (`OperatorAuthService._authenticateViaDeviceLink`)

Called by g8eo on the subsequent `POST /api/auth/operator` with `auth_mode=operator_session`.

1. **Session validation** — `OperatorSessionService.validateSession(deviceLinkSessionId)` checks the session created in Phase 1 is still live. Invalid/expired → 401.
2. **Identity extraction** — `user_id` and `operator_id` resolved from the pre-provisioned session. Missing fields → 401.
3. **User existence check** — `UserService.getUser(user_id)` → 404 if not found.
4. **API key retrieval** — `operator_api_key` fetched from the operator document for inclusion in the bootstrap response (used by g8eo for in-memory LFAA vault key derivation; never stored on disk).
5. **Bootstrap response** — Returns `operator_session_id` (the pre-provisioned session ID), `operator_id`, `user_id`, `api_key`, and `config`. `operator_cert` and `operator_cert_key` are `null` — the mTLS certificate was issued at Phase 1 slot-claim time.

**Why the two-phase design:** The device link token is consumed at registration time (Phase 1), which provisions the operator session and issues the mTLS cert. The subsequent auth request (Phase 2) uses the already-provisioned session ID — the token is gone by this point. This means the token never travels over the wire a second time and has zero value after first use.

| Check | Failure | HTTP |
|---|---|---|
| Invalid/expired operator session | `Invalid or expired session` | 401 |
| Session missing identity fields | `Invalid session` | 401 |
| User not found | `User not found` | 404 |

### Web User Authentication (FIDO2/WebAuthn Passkeys)

g8e uses passkey-only authentication. No passwords are stored anywhere in the platform.

**Login flow:**
1. On first platform access with no existing users, a guided setup creates the initial administrative account.
2. The user registers a passkey — the private key stays on their device; only the public key is stored server-side.
3. On login, VSOD generates a cryptographic challenge with a 5-minute TTL. The browser signs it with the device private key. VSOD verifies the signature against the stored public key.
4. A successful verification creates a server-side session. The browser receives an encrypted, `HttpOnly`, `Secure`, `SameSite=lax` session cookie.

**Key security properties:**
- Inherently resistant to credential stuffing, phishing, and brute force — the private key never leaves the device.
- Authenticator signature counters are tracked on every authentication to detect cloned credentials.
- Rate limits enforced on challenge generation and verification endpoints.
- Authentication events logged for anomaly detection.

---

## Operator Heartbeat
   
After authentication, the Operator enters an idle state and does exactly one thing autonomously: it sends an authenticated heartbeat to the platform every 10 seconds (interval overridable by bootstrap config). Nothing else runs without user engagement.
   
**What a heartbeat contains:**
- System metrics — hostname, CPU usage, memory usage, disk usage, network info, OS details, uptime.
- Operator session identity (authenticated on every heartbeat over the established mTLS connection).
   
**What a heartbeat does not contain:**
- No command output, no file contents, no sensitive data of any kind.
- No proactive scanning, crawling, or enumeration of the host.
   
A heartbeat from an unauthenticated or fingerprint-mismatched Operator is rejected. The platform uses the continuous heartbeat stream to track operator health — a missed heartbeat chain (60s) triggers a stale status transition.
   
**Heartbeat flow:**
```
g8eo (every 10s)
    │ WebSocket (mTLS)
    ▼
VSODB pub/sub → g8ee (OperatorHeartbeatService)
                    │ write last_heartbeat, latest_heartbeat_snapshot to VSODB
                    │ detect staleness, manage operator status transitions
                    │ publish OPERATOR_HEARTBEAT_RECEIVED internal event
                    ▼
                  VSOD (SessionAuthListener)
                    │ broadcast SSE operator.heartbeat.received
                    ▼
                  Browser
```

g8ee is the source of truth for heartbeat data and all operator status transitions. VSOD only invalidates its cache and relays the SSE event — it never writes heartbeat data to VSODB directly.

---

## Operator Security Model

### In-Memory Credentials

The Operator's API key is held in process memory for the lifetime of the process. It is never written to disk in a recoverable format. When the process is killed, the key is gone. The per-operator mTLS client certificate is also held in memory and discarded on process exit.

### Outbound-Only Architecture

g8eo Operators open no inbound ports. All communication is Operator-initiated — an outbound WebSocket to VSODB on port 443. Operators function behind any NAT, corporate firewall, or VPC without special network configuration.

### Operator Startup Security Sequence

1. Load settings from environment + CLI flags.
2. Load the platform CA certificate using local-first discovery (scan `/ssl/ca.crt`, `/vsodb/ca.crt`, `/vsodb/ssl/ca.crt`, `/data/ssl/ca.crt`), falling back to an HTTPS fetch from `https://<endpoint>/ssl/ca.crt` if no local file is found. If all paths fail, the operator exits immediately.
3. Authenticate (API key, device token, or pre-authorized session).
4. POST to `/api/auth/operator` — receive bootstrap config and per-operator mTLS certificate.
5. Initialize local storage: scrubbed vault, raw vault, audit vault.
6. Initialize LFAA audit vault and git ledger.
7. Connect to VSODB pub/sub over WSS using the issued mTLS client certificate.
8. Subscribe to `cmd:{operator_id}:{operator_session_id}` channel.
9. Begin heartbeat telemetry. Enter idle — await binding.

### g8eo Defense Layers (in order)

1. **API Key Authentication** — required for all operations before any bootstrap config is returned.
2. **mTLS** — both sides present certificates; g8eo rejects the connection if the server certificate isn't signed by the pinned CA.
3. **CA Trust Bootstrap** — platform CA fetched from hub at startup via `certs.FetchAndSetCA`; only trusts that exact CA.
4. **TLS Kill Switch** — g8eo self-terminates with exit code 7 (`ExitCertTrustFailure`) on cert verification failure; the connection is never downgraded.
5. **Fingerprint Binding** — system fingerprint permanently locked to the Operator slot on first auth; mismatches rejected.
6. **Replay Protection** — timestamp + nonce validation on every request.
7. **Explicit Session Binding** — Operator cannot receive commands until a user explicitly binds their web session.
8. **Sentinel Pre-Execution** — dangerous commands blocked before execution.
9. **Human Approval** — every state-changing command requires explicit user consent.
10. **LFAA Audit Logging** — all operations logged to the local audit vault.
11. **Process Sovereignty** — the user terminates the process; no platform action can keep a dead Operator alive.

### Hard Stop: Human Control at Every Layer

| Method | What it stops |
|---|---|
| **Cancel button in the UI** | Stops the current AI generation mid-stream; no further commands are proposed or dispatched |
| **Stop Operator in the UI** | Sends a shutdown signal to the Operator via the platform; the process exits |
| **Kill the PID on the remote machine** | Direct OS-level termination — SIGTERM or SIGKILL; the Operator process dies immediately |
| **Reboot the remote machine** | The Operator process does not survive a reboot; no autostart unless explicitly configured |
| **Close the SSH session** (`operator stream`) | The `trap EXIT` handler fires on the remote host, deleting the ephemeral binary |
| **Revoke/Refresh API key in the UI** | Immediately invalidates the Operator's credentials |
| **Unbind in the UI** | Severs the web session binding; the AI can no longer dispatch commands to that Operator |

---

## Operator Commands via Sentinel (g8eo)

All commands dispatched to an Operator pass through Sentinel before execution. Sentinel is not optional and cannot be bypassed by the AI.

### Pre-Execution Threat Detection

Before any command executes on the host, Sentinel scans it against MITRE ATT&CK-mapped threat patterns. Dangerous commands are **blocked outright** before reaching the execution layer.

| Category | Examples |
|---|---|
| **Data destruction** | Recursive root deletion (`rm -rf /`), raw disk overwrites (`dd if=/dev/zero`) |
| **System tampering** | Modifications to `/etc/passwd`, `/etc/shadow`, SSH authorized keys, sudoers |
| **Reverse shells / tunnels** | `bash -i >& /dev/tcp/...`, `nc -e`, `socat` backdoors |
| **Privilege escalation** | `chmod +s`, capability manipulation, `sudoedit` abuse |
| **Credential access** | Dumping secrets from memory, `/proc/*/mem` access |
| **Persistence mechanisms** | Cron injection, systemd unit file manipulation outside project dirs |
| **Defense evasion** | Command encoding (`base64 -d | bash`), obfuscation patterns |
| **Reconnaissance** | Mass port scanning, ARP sweeps with unusual flags |
| **Lateral movement** | SSH key injection on unauthorized paths |
| **Cryptominers** | Known miner binary signatures and connection patterns |
| **Data exfiltration** | Bulk outbound transfers of sensitive system files |
| **Resource hijacking** | Fork bombs, ulimit bypass patterns |

**Blocked threats** are rejected before the approval prompt is shown. **Flagged threats** (elevated risk, potentially legitimate) surface in the approval flow with the specific threat category and MITRE ATT&CK classification shown to the user. Every blocked or flagged attempt is recorded in the LFAA audit vault with tamper-evident logging.

### g8ee AI Safety Analysis (Pre-Dispatch)

Before dispatching any command to the Operator, g8ee runs its own AI-powered safety analysis — separate from Sentinel, operating at the platform level:

- **Command risk classification** — classifies proposed commands as LOW / MEDIUM / HIGH using structured LLM output. Fails closed to HIGH if analysis fails.
- **File operation safety** — automatically blocks writes to system paths (`/etc/`, `/usr/`, `/sys/`, `/proc/`, `/bin/`, `/sbin/`, `/boot/`, `/lib/`), destructive operations on dirty git repositories, and operations that should be backed up first.
- **Error recovery analysis** — on command failure, determines whether to auto-retry (missing deps, permission errors on project files, syntax errors) or escalate to the user (system-level failures, security issues, ambiguous errors). Maximum 2 auto-fix retries.

### Tribunal Command Refinement

Before a command is presented for human approval, it passes through an additional syntactic refinement pipeline in g8ee (`components/g8ee/app/services/ai/command_generator.py`). The Large LLM is not involved after its initial proposal.

```
Large LLM proposes command via ReAct loop
  │
  ▼
Tribunal — N concurrent generation passes (default: 3)
  Each pass: same intent + operator OS/shell/working_directory context
  Temperature: Member-specific  Model: LLM_ASSISTANT_MODEL (default: gemma3:1b)
  │
  ▼
Weighted majority vote — earlier passes weighted higher (weight 1/(i+1))
  │
  ▼
SLM Verifier (same model, temperature: 0.0)
  Returns exactly "ok" — or a corrected command string
  │
  ▼
Final command presented to human for approval
```

**Fallback guarantee:** If the tribunal fails for any reason, the original Large LLM command is used and `FALLBACK` is recorded. The human approval prompt always fires regardless.

### Command Allowlist and Denylist

Two optional operator-level controls are available as additional constraints — **disabled by default**, enabled via `ENABLE_COMMAND_WHITELISTING` and `ENABLE_COMMAND_BLACKLISTING` in `platform_settings`.

- **Allowlist (`config/whitelist.json`)** — restricts the AI to pre-approved commands with validated parameters. Each allowlisted command defines permitted options, regex-validated parameters, and a `max_execution_time`.
- **Denylist (`config/blacklist.json`)** — blocks specific commands, binaries, substrings, and regex patterns across four enforcement layers: forbidden commands, forbidden binaries, forbidden substrings, forbidden regex patterns.

When the denylist is enabled, a command matching any layer is rejected before the approval prompt — it never reaches the user for consideration.

---

## Operator Binding Implementation

Authentication is not the same as authorization to receive commands. After an Operator authenticates and is `ACTIVE`, it cannot receive commands from any user until a web session explicitly binds to it.

**Binding is an explicit user action.** On confirmation, the platform performs the following writes via `BoundSessionsService` (`services/auth/bound_sessions_service.js`):

1. Two **bidirectional VSODB KV keys** are written:
   - `sessionBindOperators(operatorSessionId)` → `webSessionId` — authoritative lookup for SSE routing
   - `sessionWebBind(webSessionId)` → `operatorSessionId` (as a SET member) — authoritative lookup for approval routing
2. A **`bound_sessions` document** is created or updated in the VSODB document store (keyed by `web_session_id`) as the durable, auditable record.

The KV keys are the authoritative runtime state; the document store is the durable backing store. Partial failures leave the platform in a detectable inconsistent state rather than a silently broken one.

### Binding Contract

- One operator session can be bound to at most one web session at a time.
- One web session can be bound to multiple operator sessions simultaneously.
- All binding mutations go through `BoundSessionsService` — routes call `getBindingService()` from `initialization.js`.
- The `sessionBindOperators` key is the fast-path lookup for SSE routing: `internal_sse_routes.js` and `internal_operator_routes.js` call `getWebSessionForOperator(operatorSessionId)` to resolve where to deliver events.
- The `sessionWebBind` key is the fast-path lookup for approval routing: `operator_approval_routes.js` calls `getBoundOperatorSessionIds(webSessionId)` to find the active operator.

### `buildVSOContext` — Bound Operator Resolution

`buildVSOContext` in `routes/platform/chat_routes.js` is the single point where VSOD resolves bound operators before every chat request. It executes at request time — **no cached result**.

**Resolution steps (per chat request):**
1. `getBindingService().getBoundOperatorSessionIds(webSessionId)` — reads `sessionWebBind` KV key.
2. For each operator session ID: `getOperatorSessionService().validateSession(operatorSessionId)` — confirms the session is live and retrieves `operator_id`.
3. Verify the reverse binding: `getBindingService().getWebSessionForOperator(operatorSessionId)` — confirms `sessionBindOperators` resolves back to this web session. Mismatch → skipped.
4. Fetch the operator document from VSODB for current `status`, `system_info`, `operator_type`.
5. Serialize each valid operator as a `BoundOperatorContext` and JSON-encode the array into `X-VSO-Bound-Operators`.

`X-VSO-Bound-Operators` is the **exclusive source of truth** for which operators are available to the AI on any given request. g8ee performs no independent operator lookup to resolve binding state — if `vso_context.bound_operators` is empty, the session has no bound operators.

---

## File Operations: Raw and Scrubbed Vault

File operation outputs on the Operator follow the LFAA dual-vault model:

### Scrubbed Vault (`.g8e/local_state.db`)

- Stores Sentinel-processed command output — credentials, PII, and sensitive patterns replaced with safe placeholders.
- **AI-accessible on demand** via `fetch_execution_output`. g8ee instructs the AI to call this tool when it receives a `stored_locally=true` metadata response from the Operator.
- The content streams back to g8ee ephemerally and is **never persisted on the platform side**.

### Raw Vault (`.g8e/raw_vault.db`)

- Stores unscrubbed, full command output and file contents.
- **Never transmitted** — remains exclusively on the Operator host.
- 30-day retention, 2 GB maximum.
- Instantiated independently from the audit vault; not part of LFAA audit logging.
- Accessible to the AI only when `sentinel_mode` is set to `raw` on the investigation (user-controlled toggle).

### Sentinel Mode

The `sentinel_mode` field on an investigation controls which vault the AI reads from:
- `scrubbed` (default) — AI sees only Sentinel-scrubbed output; maximum data sovereignty.
- `raw` — AI has access to both vaults, including full unscrubbed output.

Users can toggle this at any time via the Sentinel Toggle in the UI.

### File Operation Safety

Before any file write, g8ee's `AIResponseAnalyzer` blocks:
- Writes to system paths: `/etc/`, `/usr/`, `/sys/`, `/proc/`, `/bin/`, `/sbin/`, `/boot/`, `/lib/`.
- Destructive operations on dirty git repositories.
- Operations that should be backed up first (when no backup exists).

Automatic backups are created by `FileEditService` before any modification.

---

## Ledger Security

The Ledger (`.g8e/data/ledger/`) is a standard Git repository maintained by g8eo. Every file write committed by the AI becomes a Git commit, giving the Operator's file history a cryptographic chain that cannot be silently rewritten.

- Each commit records: exact file state before and after, timestamp, and the audit vault correlation hash.
- `restore_file` (approval-required) uses Ledger commit hashes to revert any file to a previous known state.
- `fetch_file_history` and `fetch_file_diff` give the AI read-only access to the Ledger for reasoning about file changes — no approval required for reads.
- Anyone with access to the `.g8e/data/ledger/` directory can run `git log` and see exactly what the AI changed and when, independent of the platform.
- The Ledger requires a functional `git` binary; disabled via `--no-git` or when `git` is unavailable.

---

## Sentinel Output Scrubbing

After any command executes, before its output reaches g8ee (and therefore any AI provider), Sentinel replaces sensitive patterns with safe placeholders.

**Implementation split:** The g8eo Go sentinel handles command output scrubbing. The g8ee Python scrubber handles user message scrubbing. Threat detection is Go-only (g8eo).

### 27 Scrubbing Patterns (applied sequentially)

| Category | What is scrubbed | Placeholder |
|---|---|---|
| Service tokens | g8e API keys, JWT, SendGrid, GitHub, GCP, AWS access keys, Slack, Okta, Azure AD, Twilio, NPM, PyPI, Discord, private keys | `[G8E_API_KEY]`, `[JWT]`, `[GITHUB_TOKEN]`, etc. |
| Config credentials | AWS secrets, Azure secrets, OAuth secrets, Heroku keys | `[AWS_SECRET]`, `[OAUTH_SECRET]`, etc. |
| PII with context | URLs with embedded credentials, connection strings, email addresses | `[URL_WITH_CREDENTIALS]`, `[CONN_STRING]`, `[EMAIL]` |
| Financial / generic PII | Credit card numbers, SSNs, phone numbers, IBANs | `[PII]`, `[PHONE]`, `[IBAN]` |
| Generic credentials | `password`/`secret` config patterns, bearer tokens | `[CREDENTIAL_REFERENCE]`, `[BEARER_TOKEN]` |

**What is preserved (operational data the AI needs):** IP addresses, hostnames, MAC addresses, file paths, URLs without credentials, UUIDs, AWS ARNs, AWS account IDs, filenames, hashes, base64 content.

---

## Data Sovereignty and LFAA

### The Core Guarantee

**The g8e platform never stores raw command output or file contents.** The Operator is the system of record. The platform is a stateless relay.

> *"The Platform handles routing. The Operator handles retention."*

When local storage is enabled on an Operator (the default), all tool call outputs are stored in the Operator's working directory (`.g8e/`). g8ee receives only metadata (hashes, sizes) with a `stored_locally=true` flag. The AI retrieves full output on demand via `fetch_execution_output` — content streams back ephemerally and is never persisted on the platform side.

### LFAA Components on the Operator

| Component | Path (relative to working dir) | What it records |
|---|---|---|
| **Scrubbed Vault** | `.g8e/local_state.db` | Sentinel-processed output; AI-accessible via `fetch_execution_output` |
| **Raw Vault** | `.g8e/raw_vault.db` | Unscrubbed full output; never transmitted; 30-day retention, 2 GB max |
| **Audit Vault** | `.g8e/data/g8e.db` | All session events: messages, commands (exit codes, durations), file mutations, AI responses |
| **Ledger** | `.g8e/data/ledger/` (Git) | Cryptographic version history for every file the AI has modified |

### LFAA Vault Encryption

LFAA audit vault fields (`content`, `stdout`, `stderr`) are encrypted at rest using envelope encryption:

1. **KEK derivation** — Key Encryption Key derived on-demand from the Operator's API key using HKDF-SHA256 (no salt; info string `g8e-lfaa-kek-v1`). The KEK is never stored.
2. **DEK wrapping** — Data Encryption Key generated per-vault, wrapped with the KEK using AES-256-KW (RFC 3394). The wrapped DEK is stored alongside the vault.
3. **Field encryption** — Individual sensitive fields encrypted with the DEK using AES-256-GCM with a unique random nonce per operation. AES-GCM provides authenticated encryption (AEAD); any tampering is detected on decryption.
4. **Fingerprint verification** — Before unwrapping the DEK, the platform verifies the API key against a stored fingerprint to detect wrong-key attempts.

**Key security properties:**
- The KEK is never stored — derived fresh on each vault access and cleared from memory immediately after.
- There is no backdoor or recovery mechanism. Loss of the API key means permanent loss of access to encrypted vault data.
- Key rotation (`--rekey-vault`) is atomic — re-wraps the DEK with a new KEK without re-encrypting underlying data.
- A vault reset (`--reset-vault`) permanently destroys all encryption headers and the underlying encrypted stores.

### Dual-Location Auditability

Every action through g8e is auditable from two independent locations:

**1. Platform-side (VSOD console)** — records metadata for all administrative actions, authentication events, session lifecycle events, and operator status changes. Authoritative for platform-level events.

**2. Operator-side (`.g8e/` in the deployment directory)** — the authoritative record for everything that happened on that system:
- Every user message and AI response in every session.
- Every command executed: exact command text, exit code, duration, stdout, stderr.
- Every file mutation: before and after state (via Ledger commit hashes).
- Every `fetch_execution_output` retrieval — AI access to historical data is also logged.

This dual-location design means that even if the platform itself were completely unavailable, the full audit record for every host remains intact in the host's own filesystem.

---

## Human-in-the-Loop Approval Model

The AI operates through a defined set of tools. Each tool is classified at the platform level as either requiring explicit user approval or not. This classification is fixed in the platform — the AI cannot change it.

The user's stated intent in chat is sufficient authorization for read-only activity. Any operation that **changes state** — writing a file, executing a command, modifying cloud permissions — requires an explicit approval action from the user in the UI before it executes.

Automatic Function Calling (AFC) is **permanently disabled** in G8EE. The AI cannot chain tool calls through an auto-approve path. Every step of a multi-step operation that touches state surfaces its own approval prompt.

### Function Tool Approval Classification

| Tool | Approval | What it does |
|---|---|---|
| `run_commands_with_operator` | **Required** | Execute shell commands on target systems |
| `file_create_on_operator` | **Required** | Create new files with content |
| `file_write_on_operator` | **Required** | Replace entire file contents |
| `file_update_on_operator` | **Required** | Surgical find-and-replace within a file |
| `restore_file` | **Required** | Restore a file to a previous Ledger commit |
| `grant_intent_permission` | **Required** (intent flow) | Request AWS intent permissions for cloud operators |
| `revoke_intent_permission` | **Required** (intent flow) | Revoke AWS intent permissions |
| `file_read_on_operator` | No | Read file content (with optional line ranges) |
| `list_files_and_directories_with_detailed_metadata` | No | Directory listing with metadata |
| `fetch_execution_output` | No | Retrieve command output from Operator local storage |
| `fetch_session_history` | No | Retrieve session history from Operator LFAA vault |
| `fetch_file_history` | No | Retrieve git version history for a file from the Ledger |
| `fetch_file_diff` | No | Retrieve Sentinel-scrubbed file diffs from the Operator vault |
| `check_port_status` | No | Check TCP/UDP port reachability |
| `search_web` | No | Web search (requires `vertex_search_enabled=true`) |

---

## Cloud Operator Security (AWS Zero Standing Privileges)

Cloud Operators for AWS implement a Zero Standing Privileges model — the hardest form of least privilege for cloud environments.

### Two-Role Architecture

| Role | Purpose | Cannot Do |
|---|---|---|
| **Operator Role** | Attached to EC2 instance profile or `~/.aws` credentials; executes AWS operations | Modify its own IAM policies |
| **Escalation Role** | Assumed temporarily during permission grants; attaches `Intent-*` prefixed policies | Access any AWS resources directly |

The Escalation Role requires an **external ID** (prevents confused deputy attacks) and is only assumed during the permission escalation window — credentials are cleared immediately after. A compromised AI cannot grant itself arbitrary permissions; it can only attach pre-defined intent policies authored by a human at setup time.

### Auto-Approved Self-Discovery Commands

A small set of read-only IAM introspection commands are auto-approved without user interaction:
- `aws sts get-caller-identity`
- `aws iam get-role`
- `aws iam get-role-policy`
- `aws iam list-role-policies`
- `aws iam list-attached-role-policies`
- `aws iam get-instance-profile`
- `aws iam simulate-principal-policy`

All other AWS operations require either explicit user approval or an intent grant.

### Intent-Based Permission Escalation

When the AI determines it needs AWS permissions, it calls `grant_intent_permission` which surfaces an approval prompt to the user. On approval, the Escalation Role attaches the corresponding pre-defined `Intent-*` IAM policy to the Operator Role. Permissions are revocable at any time via `revoke_intent_permission`.

---

## Threat Model

### Identity and Access

| Threat | Controls |
|---|---|
| **Session hijacking** | `HttpOnly` + `Secure` cookies; session bound to client IP + user-agent; immediate VSODB revocation |
| **Operator impersonation** | Permanent system fingerprint binding; mTLS client certificate required; duplicate sessions rejected |
| **Credential brute force** | Passkeys are inherently immune (private key never leaves device); API keys use high-entropy generation + constant-time comparison; rate limiting on all auth endpoints |
| **Session replay** | Sessions expire; Operator requests require timestamp within ±5 min + optional nonce |

### Operational and Data

| Threat | Controls |
|---|---|
| **Privilege escalation** | Role validated server-side on every request; no self-elevation API; admin middleware on all elevated endpoints |
| **Unauthorized data access** | Resource ownership enforced at service layer; every query filtered by authenticated user identity |
| **MITM / eavesdropping** | TLS 1.3 on all paths; mTLS on Operator connections; certificate pinning; no plaintext fallback |
| **Secrets in logs** | Application-layer log redaction; Sentinel scrubs all data before AI transmission |

### AI and Advanced

| Threat | Controls |
|---|---|
| **Prompt injection** | System-prompt constraints; Sentinel pre-execution blocking; all file/command content treated as untrusted data, never as instructions |
| **Indirect injection** | Content from files and command output scanned for malicious patterns before AI context injection |
| **Credential exfiltration via AI** | Sentinel 27-pattern scrubber runs on all output before it reaches any LLM provider |
| **AI-driven privilege escalation** | Human-in-the-Loop is non-bypassable; AI cannot call approval endpoints directly; every state change requires explicit user consent |
| **Malicious cloud operations** | Zero standing privileges; intent-based JIT permissions; confused deputy protection on Escalation Role |

---

## Container Security Hardening

Every service in the g8e stack runs as a dedicated non-root user (`g8e`, UID/GID 1001:1001) created in the Dockerfile. The `user:` directive in `docker-compose.yml` reinforces this by specifying the numeric UID:GID directly — the compose layer cannot be overridden by the image.

### Non-Root Users

| Service | User | UID | GID |
|---|---|---|---|
| g8ee (`g8ee`) | `g8e` | 1001 | 1001 |
| VSOD (`g8e-dashboard`) | `g8e` | 1001 | 1001 |
| VSODB (`g8es`) | `g8e` | 1001 | 1001 |
| g8e node | `g8e` | 1001 | 1001 |

No service runs as root. This is the primary container escape mitigation — a container escape from a non-root process lands as UID 1001 on the host, not root.

### Linux Capabilities

g8ee, VSOD, and g8e node use `cap_drop: ALL` and add back only the minimum required. VSODB uses default Docker capabilities:

| Service | Added capabilities | Reason |
|---|---|---|
| g8ee | none | No privileged operations |
| VSOD | none | No privileged operations |
| VSODB | default | No `cap_drop` directive |
| g8e node | none | No privileged operations |

### Docker Socket Access

VSOD and g8e node mount `/var/run/docker.sock`. The threat — the Docker socket is equivalent to host root — is mitigated by:

- Both services run as UID 1001, not root.
- `group_add: [${DOCKER_GID}]` grants socket access via the `docker` group only; root is not required.
- `no-new-privileges: true` on all services prevents setuid/setgid escalation.
- VSOD's socket interaction is isolated to a single internal service (`G8ENodeOperatorService`) and is never reachable from any unauthenticated request path.

### Secrets Model

No secrets are stored in environment variable files (`.env`), baked into images, or written to the host filesystem. All runtime secrets are managed by VSODB:

| Secret | Storage | Injection mechanism |
|---|---|---|
| `INTERNAL_AUTH_TOKEN` | VSODB `platform_settings` document | Read from VSODB at service startup |
| `SESSION_ENCRYPTION_KEY` | VSODB `platform_settings` document | Read from VSODB at service startup |
| LLM API keys | VSODB `platform_settings` document | Read from VSODB at service startup |
| SSL certificates and CA | VSODB volume (`/vsodb/ssl/`) | Mounted read-only into all services |
| Per-operator mTLS certificates | In-memory only | Issued once at claim time; never persisted |

---

## Security Posture Validation

Security scan scripts live in `components/g8ep/scripts/security/` and are volume-mounted into the container at runtime. The scanning tools (Nuclei, testssl.sh, Trivy, Grype) are lazy-installed on first use by `install-scan-tools.sh`. The scan scripts require network utilities (`wget`, `unzip`, `nmap`, etc.) that are not part of the base image; install them manually inside the container before running scans.

| Scan | Tool | What it validates |
|---|---|---|
| `scan-tls.sh` | testssl.sh | Protocol versions, cipher suites, certificate chain, vulnerability checks |
| `scan-nuclei.sh` | Nuclei | Web vulnerability scanning against the gateway |
| `scan-containers.sh` | Trivy | Container image CVE scanning (HIGH + CRITICAL findings) |
| `scan-dependencies.sh` | Grype | Source tree dependency CVE scanning — fails on CRITICAL findings |
| `run-full-audit.sh` | All of the above + nmap + curl | Full orchestrated web security audit |

```bash
./g8e test security
./g8e test security -- /app/components/g8ep/scripts/security/scan-tls.sh nginx:443
./g8e test security -- /app/components/g8ep/scripts/security/scan-containers.sh
./g8e test security -- /app/components/g8ep/scripts/security/run-full-audit.sh
```

---

## Security Review Checklist

**Authentication and Sessions**
- [ ] New endpoints have mandatory authentication middleware — no unauthenticated access paths.
- [ ] Session identifiers are not logged, not transmitted in URLs, not stored in client-accessible storage.
- [ ] New session-related fields written to VSODB are encrypted at the application layer.

**Authorization**
- [ ] Resource IDs are validated against the authenticated user's ownership before any operation.
- [ ] Admin and super-admin routes are protected by role-specific middleware, not just authentication.
- [ ] No client-supplied values are accepted as authorization context (user ID, role, org ID must come from the server-side session).

**Data Handling**
- [ ] No secrets or credentials hardcoded anywhere — all injected via environment variables or loaded from VSODB `platform_settings`.
- [ ] Logs do not contain PII, API keys, tokens, or session identifiers.
- [ ] Raw command output is never persisted on the platform side — only metadata with `stored_locally=true`.
- [ ] New wire boundary values are correctly typed — no `JSON.stringify`, `String()`, or inline coercion as a fallback.

**Sentinel and AI Safety**
- [ ] New tools that perform state changes require `approval_required=True`.
- [ ] Any new data fed into the AI that originates from external sources (files, command output, user input) is treated as untrusted data, not instructions.
- [ ] Pre-execution Sentinel patterns cover any new command categories introduced by new tools.

**Infrastructure**
- [ ] New internal API endpoints use `X-Internal-Auth` validation with constant-time comparison.
- [ ] New ports or services are not exposed outside the container network without explicit intent.
- [ ] Certificate lifecycle (issuance, revocation) is accounted for in any changes touching Operator auth.

**Audit**
- [ ] All administrative and security-sensitive operations generate audit log entries with user identity, timestamp, and action detail.
- [ ] New LFAA vault fields containing sensitive content are encrypted with AES-256-GCM.

---

## Security Contact

Report security issues to the platform maintainer per [SECURITY.md](../../SECURITY.md).
