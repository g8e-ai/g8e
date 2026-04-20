# Security Architecture

g8e is a local-only, air-gapped, portable platform. It runs entirely via `docker-compose` on-premises — no cloud deployment, no SaaS backend, no external network dependency. This document is the deep-reference security guide for the platform, covering every enforcement layer in detail.

For component-level overviews, see: [g8ee](../components/g8ee.md), [g8ed](../components/g8ed.md), [g8eo](../components/g8eo.md).

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

### Technical Positioning

g8e is often compared to existing access control and remote execution tools. Here is how it differs:

- **vs. SSH**: SSH is a secure pipe; g8e is a governor. SSH has no concept of AI intent, multi-model consensus, or granular scrubbing. g8e uses the pipe to enforce a governance model that SSH cannot.
- **vs. Teleport / Boundary**: These are fine-grained access control systems for **human** administrators. g8e is a governance system for **AI-powered automation** acting on behalf of humans. It assumes the AI control plane is potentially adversarial or error-prone.
- **vs. Ansible / Terraform**: These are deterministic configuration tools. g8e is for non-deterministic, context-aware investigation and remediation where the AI must reason about real-time system state before proposing an action.

---

## Platform Architecture Overview

```
Browser
  │  HTTPS + Passkey Auth (FIDO2/WebAuthn) + Encrypted Session Cookie
  ▼
g8ed  (Node.js — web gateway, SSE relay, Operator panel)
  │  Internal HTTP — X-Internal-Auth shared secret (constant-time comparison)
  ▼
g8ee   (Python/FastAPI — AI engine, Sentinel scrubbing, AI safety analysis)
  │  HTTP + WebSocket pub/sub
  ▼
g8es (g8eo binary in --listen mode — document store, KV store, pub/sub broker)
  │
  │  WebSocket + mTLS + Certificate Pinning + Replay Protection
  ▼
g8eo   (Go binary — Operator daemon on target host)
  │  Sentinel pre-execution, local SQLite vaults, Git Ledger (LFAA)
  ▼
Host Filesystem / AWS / Target System
```

### Security Boundaries

| Boundary | Mechanism |
|---|---|
| **Browser → g8ed** | HTTPS/TLS 1.3, FIDO2/WebAuthn passkeys, encrypted `HttpOnly` session cookie, `SameSite=lax` CSRF protection |
| **g8ed → g8ee** | Internal Docker network only, `X-Internal-Auth` shared secret (constant-time comparison), never exposed externally |
| **g8ee → g8es** | Internal Docker network, `X-Internal-Auth` token (strictly enforced by g8es/g8eo in `--listen` mode) |
| **g8ed → g8es** | Internal Docker network, `X-Internal-Auth` token (strictly enforced by g8es/g8eo in `--listen` mode) |
| **g8ee → LLM (AI)** | Sentinel-scrubbed data only — raw output, credentials, and PII never transmitted to any AI provider |
| **g8ed → g8eo** | WebSocket over mTLS (TLS 1.3), per-operator client certificate issued at claim time, platform CA fetched from hub at operator startup |
| **Operator → Host** | Sentinel pre-execution threat blocking, command allowlist/denylist, Human-in-the-Loop approval required for every state change |
| **Data at Rest (g8es)** | SQLite at `0600` filesystem permissions (4 tables: documents, kv_store, sse_events, blobs); session fields encrypted at application layer by g8ed before persistence; **bootstrap secrets (`internal_auth_token`, `session_encryption_key`) encrypted with AES-256-GCM at rest on the `g8es-ssl` volume using `G8E_SECRETS_KEY`, mirrored into the `platform_settings` document for consistency** |
| **Data at Rest (LFAA Vaults)** | AES-256-GCM field-level encryption (content, stdout, stderr); DEK envelope encryption; key derived on-demand from operator API key via HKDF-SHA256 |

### Network Isolation

g8e runs on a private Docker bridge network. Only two ports are bound to the host: **443** (TLS gateway for browser and Operator WebSocket) and optionally **80** (HTTP redirect). All other inter-service communication is internal and unreachable from outside. g8eo Operators initiate outbound-only connections to port 443; they open no inbound ports.

---

### Configuration Security and Precedence

g8e uses a layered configuration model designed for zero-trust local deployments. Configuration flows through a strictly enforced precedence chain, ensuring that sensitive values (like API keys and secrets) can be provided via the environment at deployment time but are managed via the platform's own persistence layer at runtime.

#### The Precedence Chain

All components resolve configuration values in the following order (highest priority wins):

1.  **User Settings (DB)**: Individual user overrides stored in g8es `user_settings` collection.
2.  **Platform Settings (DB)**: Global platform values stored in g8es `platform_settings` collection.
3.  **Environment Variables**: Canonical `G8E_*` variables provided at container runtime.
4.  **Schema Defaults**: Hardcoded safe defaults defined in the component's configuration service.

**Exception: Bootstrap Secrets**
For critical bootstrap secrets (`internal_auth_token`, `session_encryption_key`), the **Shared SSL Volume** is the source of truth consumed by g8ed and g8ee at startup. g8es additionally mirrors the same values into the `platform_settings` document on startup so that the on-disk files and the cached DB document stay in sync (see `SecretManager.InitPlatformSettings` in `components/g8eo/services/listen/secret_manager.go`).

#### Bootstrap Secrets Encryption

To prevent unauthorized access to secrets via `docker exec` or direct volume inspection, bootstrap secrets are encrypted at rest on the `g8es-ssl` volume using AES-256-GCM:

- **Encryption Key**: `G8E_SECRETS_KEY` is a 32-byte (64 hex chars) key generated during first-time setup and stored in `~/.g8e/secrets_key` with `0600` permissions
- **Encryption**: g8es encrypts secrets before writing to the volume using Go's `crypto/aes` and `crypto/cipher` packages with GCM mode
- **Decryption**: g8ed, g8ee, and g8ep entrypoints decrypt secrets at startup using Python's `cryptography` library (compatible AES-256-GCM implementation)
- **Format**: Encrypted secrets are base64-encoded with the nonce prepended (12 bytes for GCM)
- **Backward Compatibility**: If `G8E_SECRETS_KEY` is not set, secrets are stored/retrieved in plain text (existing installations continue to work)

This ensures that even if an attacker gains Docker host access and can execute `docker exec`, they cannot read the actual secrets without the encryption key, which is only available in the container environment and the local `~/.g8e/secrets_key` file.

#### Bootstrap Secrets Handling

The platform handles two critical secrets that are required for component-to-component authentication and data protection:

- **`internal_auth_token`**: Shared secret for `X-Internal-Auth` header authentication.
- **`session_encryption_key`**: AES-256 key used to encrypt sensitive fields in web and operator sessions.

The `./g8e platform settings` command displays truncated versions of these active secrets (e.g., `f5037487...6c5f`) to confirm they are set and synchronized without exposing the full values.

**1. Authoritative Runtime Source (The Volume)**
The `g8es-ssl` volume is the authoritative source used by runtime consumers. It is mounted at `/g8es` on g8ed, g8ee, and g8ep (read-only), and at `/ssl` inside g8es itself. The secrets are stored as `0600` plain-text files on this volume.
- `internal_auth_token` is stored at `/g8es/internal_auth_token`.
- `session_encryption_key` is stored at `/g8es/session_encryption_key`.

**2. Generation and Synchronization**
On every g8es startup, `SecretManager.InitPlatformSettings` reconciles the volume files with the `platform_settings` document in g8es:
- If the `platform_settings` document does not exist and files are missing, g8es generates cryptographically secure 32-byte hex values, creates the document with both secrets, and writes the files.
- If the document already exists, any file value (when present) takes precedence and is written back into the document; otherwise the document value is used to (re)populate the file.
- After each successful write, g8es re-reads both files and aborts startup if their contents no longer match the DB document (`verifyDBMatchesFile`), closing the window where a partial write or concurrent writer could leave the two authorities silently disagreeing.
- g8es then writes a tamper-evidence manifest at `/ssl/bootstrap_digest.json` (`writeDigestManifest`) containing the SHA-256 digest of each secret alongside a manifest version and UTC timestamp. The file is written atomically (`.tmp` + rename) with `0600` permissions.
- The `platform_settings` cache-aside KV entry is warmed after any write so that the first authenticated request does not race the cold cache.

**3. Automatic Discovery and Verification**
g8ed and g8ee discover these tokens by reading the files from the shared volume at startup via their `BootstrapService` (`components/g8ed/services/platform/bootstrap_service.js`, `components/g8ee/app/services/infra/bootstrap_service.py`). Neither component reads the bootstrap secrets from the database.

Before authenticating with a loaded secret, each consumer calls `verifyAgainstManifest` (g8ed) / `verify_against_manifest` (g8ee), which:
- Reads `bootstrap_digest.json` from the same volume;
- Computes SHA-256 of the value it just loaded;
- Aborts startup with a `BootstrapSecretTamperError` if the digest disagrees with the manifest entry for that secret.

A missing manifest is treated as a transitional warning (not an error) so that upgrade ordering is not a footgun; once a g8eo with manifest support has booted, every subsequent consumer start is fully verified. This closes the silent coupling between `SecretManager` (sole writer) and the consumer `BootstrapService`s (sole readers): a divergent volume file now surfaces as a clear startup abort instead of an opaque 401 during the first downstream API call.

**4. Dual-Location Persistence**
These secrets live in two places: the SSL volume files (read by g8ed/g8ee) and the `platform_settings` document (written and kept in sync by g8es). The volume file is the boot-time source of truth; the DB copy exists to make the secrets visible to the platform settings view and to survive restarts where only the DB is inspected. A full database wipe is still non-destructive to identity because the next g8es startup will repopulate the document from the surviving volume files.

#### Bootstrap Seeding

On the first start of a new deployment, the platform automatically **seeds** the persistent database with values found in the environment. This "Capture and Persist" strategy ensures that a deployment remains stable even if container environment variables are removed or modified after initialization.

#### Enforcement

- **Code Standard**: Direct reads from the host environment (e.g., `process.env` or `os.Getenv`) are strictly prohibited outside of the component's configuration service.
- **Transport Security**: Bootstrap transport URLs (`G8E_INTERNAL_HTTP_URL`, `G8E_INTERNAL_PUBSUB_URL`) are read once at startup to reach the database; all other configuration is then loaded from the database.
- **Sensitive Fields**: Sensitive fields in the database (API keys, session tokens) are application-layer encrypted before storage.

All `X-G8E-*` identity headers are **injected by g8ed from the verified server-side session** — never accepted from the untrusted request body. User identity cannot be forged by a client.

| Header | Purpose |
|---|---|
| `X-G8E-WebSession-ID` | Browser session identifier |
| `X-G8E-User-ID` | Authenticated user identity |
| `X-G8E-Organization-ID` | Multi-tenant isolation |
| `X-G8E-Case-ID` | Active case correlation |
| `X-G8E-Investigation-ID` | Active investigation correlation |
| `X-G8E-Task-ID` | Active task correlation |
| `X-G8E-Bound-Operators` | JSON array of all operators bound to the session |
| `X-G8E-Request-ID` | Execution tracking identifier |
| `X-G8E-New-Case` | Boolean signal for inline resource creation |
| `X-G8E-Source-Component` | Source component name (validated against `ComponentName` enum) |

---

## SSL/CA Certificate Generation and Handling

### CA Private Key Protection

The platform CA private key (`ca.key`) is the most sensitive asset in the platform. It is protected by the following:

- **Volume Isolation**: It is stored exclusively in the `g8es-ssl` Docker volume, which is separate from the application data volume.
- **Service Access**: It is mounted into `g8ed` (for signing operator certificates) and `g8es` (for generating the CA) but is never exposed via any API.
- **Read-Only Mounts**: On all other services, the `g8es-ssl` volume is mounted read-only.
- **No Persistence in Hub**: The key is never stored in the SQLite database or the environment.

---

g8e operates its own private CA. There is no dependency on any public CA.

- **Algorithm:** ECDSA with P-384. **Protocol:** TLS 1.3 only on all external and Operator-facing endpoints.
- **Generation:** CA and server certificates are generated at runtime by the g8es operator binary (`--listen --ssl-dir /ssl` mode) on first start. Stored in the dedicated `g8es-ssl` volume (`/ssl` inside g8es). Never baked into any Docker image.
- **Distribution to services:** The `g8es-ssl` named Docker volume is mounted read-only at `/g8es` on g8ed, g8ee, and g8ep, and read-write at `/ssl` on g8es itself. Consumers read the CA from `/g8es/ca.crt` (or `/g8es/ca/ca.crt`). g8ed's `CertificateService` reads the CA cert and key from the same location to sign per-operator client certificates; per-operator client certs are written to `/g8es/certs/`.
- **Volume isolation:** SSL certs live in a dedicated volume (`g8es-ssl`) separate from the SQLite DB volume (`g8es-data`). `platform reset` wipes the DB volume but never touches the SSL volume — SSL certs survive a full rebuild without needing to be re-trusted.
- **CA trust for field operators:** The non-listen g8eo binary uses a **local-first** discovery strategy. When `--ca-url` is not set, it scans well-known volume mount paths (`/ssl/ca.crt`, `/g8es/ca.crt`, `/g8es/ssl/ca.crt`, `/data/ssl/ca.crt`) before attempting any network request. If no local file is found, it falls back to an HTTPS fetch from `https://<endpoint>/ssl/ca.crt` using the OS system trust store. The CA is never baked into the binary at compile time — there is no `//go:embed` and no `server_ca.crt` source file. This eliminates the circular dependency that caused x509 failures after a clean volume wipe: the operator always discovers the CA that g8es actually generated, not a stale one. Inside the Docker network, the `g8es-ssl` volume provides the CA at `/g8es/ca.crt` — no network fetch occurs.
- **Per-operator client certificates:** Issued dynamically during Operator slot creation and stored in the operator document. The certificate is not transmitted in the authentication response; the Operator retrieves it from the operator document after slot creation.
- **Validity:** CA — 10 years (3650 days); server certificate — 90 days. Both renewed automatically on restart if expired.
- **CA private key:** Accessible only to the core authentication service; never exposed via any API.

### CA Trust Bootstrap and the TLS Kill Switch

The g8eo binary (field operator mode) loads the platform CA at startup using a two-stage strategy:

1. **Local discovery** (preferred): When `--ca-url` is not set, the binary scans `/ssl/ca.crt`, `/g8es/ca.crt`, `/g8es/ssl/ca.crt`, and `/data/ssl/ca.crt` in order. The first path that exists and contains valid PEM is accepted immediately via `certs.SetCA` — no network request is made. This is the normal path for containerized operators (e.g., g8ep), where the `g8es-ssl` Docker volume provides the CA locally.
2. **Remote fetch** (fallback): If no local file is found, the binary fetches from `https://<endpoint>/ssl/ca.crt` (or the URL given by `--ca-url`) via `certs.FetchAndSetCA`. This uses Go's default `http.Client` with the OS system trust store, a 15-second timeout, and a 64 KB body limit. The fetch is equivalent to a certificate pinning operation — not a sensitive data transfer. For remote deployments using the [deployment script](../components/g8ed.md#operator-deployment-script), the CA is pre-fetched over plain HTTP and passed to `curl --cacert` / `wget --ca-certificate` for the binary download — the operator binary then discovers it via local-first discovery or falls back to the standard HTTPS fetch.

Once stored in the runtime CA store, all subsequent TLS connections are verified against it. Public CAs are not trusted.

If both stages fail (no local file, hub unreachable, bad PEM, non-200 response), the operator exits immediately with `ExitConfigError`. If certificate verification fails at connection time, g8eo self-terminates with **exit code 7** (`ExitCertTrustFailure`). The connection is never downgraded, retried insecurely, or silently ignored.

### Mutual TLS (mTLS)

All Operator-to-g8ed connections require mutual TLS:
- **Server side:** g8ed presents its server certificate (signed by the platform CA).
- **Client side:** g8eo presents its per-operator client certificate (issued at claim time).

An Operator cannot connect without a valid platform-issued certificate. g8ed cannot be impersonated by anything not signed by the pinned CA.

### Workstation CA Trust

Because g8e uses a locally generated CA, users must configure their workstation browser and operating system to trust it before using the HTTPS UI.

#### Primary Method: Terminal One-Liner

The recommended approach is a single curl command run from an elevated terminal on the user's machine. The platform serves a pipe-friendly trust script from `http://<host>/trust` that automatically detects the operating system and performs the full trust workflow: download the CA, remove any stale g8e certificates, and install the new CA into the system trust store.

**macOS / Linux** (run in Terminal):
```
curl -fsSL http://<host>/trust | sudo sh
```

**Windows** (run in an elevated PowerShell terminal):
```
irm http://<host>/trust | iex
```

The `/trust` endpoint detects the caller's platform from the `User-Agent` header and returns the appropriate script — a POSIX shell script for macOS/Linux (which distinguishes between the two at runtime via `uname -s`), or a PowerShell script for Windows.

#### Alternative: HTTP Onboarding Page

The platform also exposes an HTTP onboarding portal on port 80 (`http://<host>`), which provides a browser-based UI for the same trust bootstrap. That page auto-selects the user's OS, presents a downloadable trust script or raw `.crt` as appropriate, and links trusted users forward to `https://<host>/setup`.

| OS | Installation Method |
|---|---|
| **macOS** | Trust script (`.sh`) downloads the CA from g8ed, removes old g8e certs, and installs the new CA into the system keychain. The raw `.crt` remains available for manual trust flows. |
| **Windows** | Trust script (`.bat`) self-elevates via UAC, downloads the CA from g8ed, removes old g8e certs, and installs the new CA via `certutil`. |
| **Linux** | Trust script (`.sh`) downloads the CA from g8ed, removes old g8e certs, copies the new CA into the system trust store, and refreshes trusted certificates. |
| **iOS** | Download the raw `.crt`, install the profile, then explicitly enable full trust in Certificate Trust Settings. |
| **Android** | Download the raw `.crt`, open it from Downloads, and install it as a CA certificate. |

The HTTP onboarding page is informational and bootstrap-only. Once the workstation trusts the CA, normal browser access moves to HTTPS on port 443. Operator traffic also depends on the same CA chain — browser HTTPS and Operator mTLS both anchor to the g8es-generated platform CA.

### Certificate Revocation

Revoked certificate serials are tracked in-memory by `CertificateService` for the lifetime of the g8ed process. Revocation is triggered automatically by API key refresh, Operator decommission, or manual security response. Revoked serials are also recorded in the Operator document for audit.

---

## Web Session Security

Sessions are a critical security boundary — both web user sessions and Operator sessions are centrally managed by g8es.

### Web Session Lifecycle

1. Created by g8ed on successful passkey verification.
2. Session ID is cryptographically secure and globally unique (`session_{timestamp}_{uuid}`).
3. Stored in g8es KV with TTL; subject to both idle timeout (8 hours, configurable) and absolute hard lifetime (24 hours, configurable).
4. Transmitted to the browser as an encrypted, `HttpOnly`, `Secure`, `SameSite=lax` cookie.
5. Sensitive session metadata (`api_key` field) is encrypted with AES-256-GCM using `SESSION_ENCRYPTION_KEY` before write to g8es.
6. On every request, the session is validated against the stored g8es record and the client's context markers (IP, user-agent). IP changes are tracked; suspicious activity is flagged after 4+ IP changes.
7. Revocation is immediate — invalidating a session in g8es takes effect on the next request.

### Web Session Security Properties

| Control | Value |
|---|---|
| Idle timeout | 8 hours (`SESSION_TTL_SECONDS`) |
| Absolute timeout | 24 hours (`ABSOLUTE_SESSION_TIMEOUT_SECONDS`) |
| Concurrent sessions | Tracked per user in g8es KV (unlimited active allowed) |
| Cookie flags | `HttpOnly`, `Secure`, `SameSite=Lax` |
| CSRF protection | `SameSite=Lax` — no additional tokens required |
| `api_key` field | Encrypted with AES-256-GCM at application layer before g8es write |

### `requireAuth` Middleware

`requireAuth` (`middleware/authentication.js`) is the single session validation point. It extracts the session ID from the `web_session_id` HttpOnly cookie. In non-production environments, it also accepts `X-G8E-WebSession-ID` or `Authorization: Bearer` as fallbacks for testing. After validation it attaches `req.webSessionId`, `req.session`, and `req.userId`. Route handlers must use these exclusively — never re-extract the session ID or call `validateSession()` again.

### `requireFirstRun` Middleware

`requireFirstRun` (`middleware/authentication.js`) guards the unauthenticated setup-flow registration endpoints. It checks `platform_settings.setup_complete` on every request. If setup is already complete, it calls `next('route')`, causing Express to skip the setup-flow handler and fall through to the `requireAuth` handler registered on the same path (the add-passkey flow). This dual-handler pattern means the same endpoint serves two distinct flows without any branching inside the handler body — and without exposing an unauthenticated code path once the platform is initialized. If the settings check fails, the request is rejected with 500.

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
- The `api_key` field is encrypted with AES-256-GCM in the session document before persistence; other fields (including `operator_id`) are stored unencrypted so they can be used as lookup keys.
- Strictly isolated from web sessions — a user's web session and their bound Operator's session are separate security principals, linked via g8es KV keys.
- API key refresh terminates the old Operator immediately, creates a new slot, and requires full re-authentication.

### Session Type Isolation

Web user sessions and Operator sessions are strictly partitioned. Authentication rules, timeout policies, and revocation mechanisms differ between the two session types. A compromised web session does not grant access to Operator commands — the Operator's own session and API key are separate credentials.

### Replay Protection

Every Operator request carries:
- `X-Request-Timestamp` — accepted within a ±5-minute window only.
- `X-Request-Nonce` (optional) — unique per request; validated against the nonce cache (in-memory by default; g8es KV when configured, 10-minute TTL) to prevent replay of captured traffic.

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
2. g8ed verifies credentials and validates the system fingerprint binding (permanent after first use).
3. g8ed returns the operator session ID, operator ID, user ID, API key, and configuration. The per-operator mTLS client certificate is generated and stored in the operator document during slot creation (not returned in the auth response).
4. The Operator connects to g8es pub/sub over WSS using the mTLS client certificate that was provisioned during slot creation.

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
2. **Key validation** — `ApiKeyService.validateApiKey` performs a cache-aside lookup (g8es KV first, document store fallback). Checks: `g8e_` prefix format, `status === ACTIVE`, `expires_at` not in the past. Invalid → 401.
3. **Download-only key rejection** — If the key has no `operator_id` binding (download-only key) → 403 with `DOWNLOAD_KEY_NOT_ALLOWED` code. Download keys (`G8E_DOWNLOAD_KEY`) fetch the binary; they cannot authenticate an operator process.
4. **`last_used_at` update** — `ApiKeyService.updateLastUsed` called after successful validation (non-blocking; errors are logged and do not fail auth).
5. **User existence check** — `UserService.getUser(user_id)` from key data → 404 if not found.
6. **System fingerprint requirement** — `system_info.system_fingerprint` must be present in the request body → 400 if missing.
7. **Operator ownership check** — `operator.user_id` must match the authenticated `user_id` → 403 if not.
8. **Operator type immutability** — If the slot has an existing fingerprint + type, the requested type (system vs. cloud) must match → 403 with `OPERATOR_TYPE_MISMATCH` code. Operator type is permanent once set.
9. **Active operator reconnect check** — If the operator is `ACTIVE` or `BOUND`: reconnection is permitted only if the fingerprint matches (same-system restart) or the operator is stale (no heartbeat for >60s). Otherwise → 409.
10. **Fingerprint binding check** — If the operator is not active/bound: if a fingerprint is already stored and it does not match the incoming fingerprint → 403 with `FINGERPRINT_MISMATCH` code.
11. **Slot claim vs. reconnect** — `is_claiming_slot = true` when no fingerprint has been stored yet (first-ever auth). First auth claims the slot via `OperatorDataService.claimOperatorSlot`. The per-operator mTLS client certificate is generated during slot creation (not in the auth response). Subsequent auths call `updateOperatorForReconnection`. If the operator was previously `BOUND`, its KV binding is refreshed to the new operator session ID.
12. **Session creation and activation** — Operator session created, operator activated, `OPERATOR_STATUS_UPDATED` SSE broadcast to all active user web sessions.
13. **Bootstrap response** — Returns `operator_session_id`, `operator_id`, `user_id`, `api_key` (echoed back for in-memory use), and `config`. The per-operator mTLS client certificate is generated and stored in the operator document during slot creation (not returned in the auth response).

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
2. Token fetched from g8es KV; expiry checked.
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
5. **Bootstrap response** — Returns `operator_session_id` (the pre-provisioned session ID), `operator_id`, `user_id`, `api_key`, and `config`. The per-operator mTLS client certificate is generated and stored in the operator document during slot creation (not returned in the auth response).

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
3. On login, g8ed generates a cryptographic challenge with a 5-minute TTL. The browser signs it with the device private key. g8ed verifies the signature against the stored public key.
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
g8es pub/sub → g8ee (OperatorHeartbeatService)
                    │ write last_heartbeat, latest_heartbeat_snapshot to g8es
                    │ detect staleness, manage operator status transitions
                    │ publish OPERATOR_HEARTBEAT_RECEIVED internal event
                    ▼
                  g8ed (SessionAuthListener)
                    │ broadcast SSE operator.heartbeat.received
                    ▼
                  Browser
```

g8ee is the source of truth for heartbeat data and all operator status transitions. g8ed only invalidates its cache and relays the SSE event — it never writes heartbeat data to g8es directly.

---

## Operator Security Model

### In-Memory Credentials

The Operator's API key is held in process memory for the lifetime of the process. It is never written to disk in a recoverable format. When the process is killed, the key is gone. The per-operator mTLS client certificate is also held in memory and discarded on process exit.

### Outbound-Only Architecture

g8eo Operators open no inbound ports. All communication is Operator-initiated — an outbound WebSocket to g8es on port 443. Operators function behind any NAT, corporate firewall, or VPC without special network configuration.

### Operator Startup Security Sequence

1. Load settings from environment + CLI flags.
2. Load the platform CA certificate using local-first discovery (scan `/ssl/ca.crt`, `/g8es/ca.crt`, `/g8es/ssl/ca.crt`, `/data/ssl/ca.crt`), falling back to an HTTPS fetch from `https://<endpoint>/ssl/ca.crt` if no local file is found. If all paths fail, the operator exits immediately.
3. Authenticate (API key, device token, or pre-authorized session).
4. POST to `/api/auth/operator` — receive bootstrap config (session ID, operator ID, user ID, API key, config). Retrieve the per-operator mTLS client certificate from the operator document.
5. Initialize local storage: scrubbed vault, raw vault, audit vault.
6. Initialize LFAA audit vault and git ledger.
7. Connect to g8es pub/sub over WSS using the retrieved mTLS client certificate.
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

### Sentinel Enforcement Layers

It is important to distinguish between the two distinct layers of Sentinel enforcement:

1.  **Threat Detection (Non-Optional)**: All commands are scanned against MITRE ATT&CK patterns. This layer is always active and cannot be disabled by the user or the AI. Dangerous patterns (e.g., `rm -rf /`) are blocked before the user ever sees them.
2.  **Explicit Constraints (Optional)**: The **Command Allowlist** and **Command Denylist** are user-defined constraints. These are disabled by default and can be enabled for environments requiring strict "known-good" command sets.

---

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
Tribunal — N concurrent generation passes (default: 3, `llm_command_gen_passes`)
  Each pass: same intent + operator OS/shell/working_directory context
  Temperature: model default (via get_model_config)
  Model: resolved via `assistant_model -> primary_model` (Ollama default: qwen3-5-122b)
  Members cycle through Axiom / Concord / Variance
  │
  ▼
Weighted majority vote — earlier passes weighted higher (weight 1/(i+1))
  │
  ▼
SLM Verifier (same model, temperature: model default; disabled via `llm_command_gen_verifier=false`)
  Returns exactly "ok" — or a corrected command string
  │
  ▼
Final command presented to human for approval
```

**Fallback guarantee:** If the tribunal fails for any reason, the original Large LLM command is used and `FALLBACK` is recorded. The human approval prompt always fires regardless.

### Command Allowlist and Denylist

Two optional operator-level controls are available as additional constraints — **disabled by default**, configured via user settings.

- **Allowlist (whitelist)** — restricts the AI to pre-approved commands with validated parameters. Each allowlisted command defines permitted options, regex-validated parameters, and a `max_execution_time`.
- **Denylist (blacklist)** — blocks specific commands, binaries, substrings, and regex patterns across four enforcement layers: forbidden commands, forbidden binaries, forbidden substrings, forbidden regex patterns.

When the blacklist is enabled, a command matching any layer is rejected before the approval prompt — it never reaches the user for consideration.

#### Configuration

Command validation is configured per-user via the `command_validation` field in user settings:

```json
{
  "user_id": "...",
  "settings": {
    "command_validation": {
      "enable_whitelisting": false,
      "enable_blacklisting": false
    },
    "llm": { ... },
    "search": { ... },
    "eval_judge": { ... }
  }
}
```

- `enable_whitelisting` (bool, default: `false`) — When enabled, only commands in the whitelist are permitted
- `enable_blacklisting` (bool, default: `false`) — When enabled, commands matching blacklist patterns are blocked

Users can configure these settings through:
1. **Settings UI** — Navigate to Settings → Command Validation to enable/disable whitelist and blacklist
2. **API** — Update user settings via the `/api/settings/user` endpoint

The AI is informed of active command constraints via the `get_command_constraints` tool, which returns the current whitelist and blacklist state for Tribunal awareness during command generation.

---

## Operator Binding Implementation

Authentication is not the same as authorization to receive commands. After an Operator authenticates and is `ACTIVE`, it cannot receive commands from any user until a web session explicitly binds to it.

**Binding is an explicit user action.** On confirmation, the platform performs the following writes via `BoundSessionsService` (`services/auth/bound_sessions_service.js`):

1. Two **bidirectional g8es KV keys** are written:
   - `sessionBindOperators(operatorSessionId)` → `webSessionId` — authoritative lookup for SSE routing
   - `sessionWebBind(webSessionId)` → `operatorSessionId` (as a SET member) — authoritative lookup for approval routing
2. A **`bound_sessions` document** is created or updated in the g8es document store (keyed by `web_session_id`) as the durable, auditable record.

The KV keys are the authoritative runtime state; the document store is the durable backing store. Partial failures leave the platform in a detectable inconsistent state rather than a silently broken one.

### Binding Contract

- One operator session can be bound to at most one web session at a time.
- One web session can be bound to multiple operator sessions simultaneously.
- All binding mutations go through `BoundSessionsService` — routes call `getBindingService()` from `initialization.js`.
- The `sessionBindOperators` key is the fast-path lookup for SSE routing: `internal_sse_routes.js` and `internal_operator_routes.js` call `getWebSessionForOperator(operatorSessionId)` to resolve where to deliver events.
- The `sessionWebBind` key is the fast-path lookup for approval routing: `operator_approval_routes.js` calls `getBoundOperatorSessionIds(webSessionId)` to find the active operator.

### `buildG8eContext` — Bound Operator Resolution

`buildG8eContext` in `routes/platform/chat_routes.js` is the single point where g8ed resolves bound operators before every chat request. It executes at request time — **no cached result**.

**Resolution steps (per chat request):**
1. `getBindingService().getBoundOperatorSessionIds(webSessionId)` — reads `sessionWebBind` KV key.
2. For each operator session ID: `getOperatorSessionService().validateSession(operatorSessionId)` — confirms the session is live and retrieves `operator_id`.
3. Verify the reverse binding: `getBindingService().getWebSessionForOperator(operatorSessionId)` — confirms `sessionBindOperators` resolves back to this web session. Mismatch → skipped.
4. Fetch the operator document from g8es for current `status`, `system_info`, `operator_type`.
5. Serialize each valid operator as a `BoundOperatorContext` and JSON-encode the array into `X-G8E-Bound-Operators`.

`X-G8E-Bound-Operators` is the **exclusive source of truth** for which operators are available to the AI on any given request. g8ee performs no independent operator lookup to resolve binding state — if `g8e_context.bound_operators` is empty, the session has no bound operators.

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

### Scrubbing Patterns (applied sequentially)

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

**1. Platform-side (g8ed console)** — records metadata for all administrative actions, authentication events, session lifecycle events, and operator status changes. Authoritative for platform-level events.

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
| `grant_intent_permission` | **Required** (intent flow) | Request AWS intent permissions for cloud operators |
| `revoke_intent_permission` | **Required** (intent flow) | Revoke AWS intent permissions |
| `file_read_on_operator` | No | Read file content (with optional line ranges) |
| `list_files_and_directories_with_detailed_metadata` | No | Directory listing with metadata |
| `fetch_file_history` | No | Retrieve git version history for a file from the Ledger |
| `fetch_file_diff` | No | Retrieve Sentinel-scrubbed file diffs from the Operator vault |
| `check_port_status` | No | Check TCP/UDP port reachability |
| `query_investigation_context` | No | Retrieve case/investigation context from g8es |
| `get_command_constraints` | No | Return the active whitelist/blacklist state for Tribunal awareness |
| `g8e_web_search` | No | Web search (only registered when a `WebSearchProvider` is configured) |

The tool declarations that the AI can actually invoke are built in `AIToolService.__init__` (`components/g8ee/app/services/ai/tool_service.py`). The `restore_file`, `fetch_execution_output`, `fetch_session_history`, and `read_file_content` enum members exist in `OperatorToolName` but are not currently surfaced as AI-callable tool declarations — file restores and execution-output retrieval are driven directly by g8ee services rather than being exposed to the model.

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

## Out of Scope and Assumptions

To maintain technical integrity, we explicitly state what g8e does **not** protect against and the assumptions made by the security model:

- **Compromised Hub OS**: If the host machine running the g8e platform (the hub) is compromised at the root level, the attacker can access the SSL volume, the database, and the CA key.
- **Compromised Workstation**: If the user's browser or OS is compromised, an attacker could hijack the active session or capture the FIDO2/WebAuthn interaction (though passkeys are resistant to many forms of this).
- **Already-Compromised Operator Host**: If an Operator is deployed to a host that is already root-compromised, the attacker can intercept the binary, its memory, and its local vaults.
- **Denial of Service (DoS)**: g8e is not designed to mitigate high-volume network-level DoS attacks.
- **Physical Access**: The platform assumes that physical access to the hub and the managed hosts is restricted.

---

## Threat Model

### Identity and Access

| Threat | Controls |
|---|---|
| **Session hijacking** | `HttpOnly` + `Secure` cookies; session bound to client IP + user-agent; immediate g8es revocation |
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
| g8ed (`g8ed`) | `g8e` | 1001 | 1001 |
| g8es (`g8es`) | `g8e` | 1001 | 1001 |
| g8e node | `g8e` | 1001 | 1001 |

No service runs as root. This is the primary container escape mitigation — a container escape from a non-root process lands as UID 1001 on the host, not root.

### Linux Capabilities

g8ee, g8ed, and g8e node use `cap_drop: ALL` and add back only the minimum required. g8es uses default Docker capabilities:

| Service | Added capabilities | Reason |
|---|---|---|
| g8ee | none | No privileged operations |
| g8ed | none | No privileged operations |
| g8es | default | No `cap_drop` directive |
| g8e node | none | No privileged operations |

### Docker Socket Access

g8ed and g8e node mount `/var/run/docker.sock`. The threat — the Docker socket is equivalent to host root — is mitigated by:

- Both services run as UID 1001, not root.
- `group_add: [${DOCKER_GID}]` grants socket access via the `docker` group only; root is not required.
- `no-new-privileges: true` on all services prevents setuid/setgid escalation.
- g8ed's socket interaction is isolated to a single internal service (`G8ENodeOperatorService`) and is never reachable from any unauthenticated request path.

### Secrets Model

No secrets are stored in environment variable files (`.env`), baked into images, or written to the host filesystem. All runtime secrets are managed by g8es:

| Secret | Storage | Injection mechanism |
|---|---|---|
| `internal_auth_token` | `g8es-ssl` volume file (`/g8es/internal_auth_token`), mirrored into `platform_settings` | Read from the volume file at service startup by `BootstrapService` |
| `session_encryption_key` | `g8es-ssl` volume file (`/g8es/session_encryption_key`), mirrored into `platform_settings` | Read from the volume file at service startup by `BootstrapService` |
| LLM API keys and other tenant-configurable secrets | g8es `platform_settings` / `user_settings` documents | Read from g8es at service startup and on settings changes |
| SSL certificates and CA | `g8es-ssl` volume (`/g8es/ca.crt`, `/g8es/ca/ca.crt`, `/g8es/certs/`) | Mounted read-only into g8ed, g8ee, and g8ep |
| Per-operator mTLS client certificates | Issued at slot-claim time and stored in the operator document; held in memory by the running operator | Operator retrieves its cert from the operator document after slot creation |

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
- [ ] New session-related fields written to g8es are encrypted at the application layer.

**Authorization**
- [ ] Resource IDs are validated against the authenticated user's ownership before any operation.
- [ ] Admin and super-admin routes are protected by role-specific middleware, not just authentication.
- [ ] No client-supplied values are accepted as authorization context (user ID, role, org ID must come from the server-side session).

**Data Handling**
- [ ] No secrets or credentials hardcoded anywhere — all injected via environment variables or loaded from g8es `platform_settings`.
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
