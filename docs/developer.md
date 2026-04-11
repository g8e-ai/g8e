# g8e Developer Guide

> **© 2026 Lateralus Labs, LLC.**  
> g8e is licensed under the [Apache License, Version 2.0](LICENSE).

---

## Origins and Why This Codebase Exists

The full story is in [README.md — Origins](../README.md#origins). Short version for context here:

g8e started as a cloud-backed PoC — the Operator executed commands on one system at a time, GCP handled the backend, no LFAA, no Sentinel. That was what first shipped after about eight months of stealth development. Through sandbox testing it became clear the Operator was capable of far more than command execution. Every design decision started revolving around the Operator, and before long, the Operator with LFAA was fully capable of being the backend for the entire platform. That made the cloud backend feel wrong. GCP was dropped, the architecture was rebuilt around the Operator, and the platform became what it is now — fully self-hosted, air-gap capable, zero cloud dependencies.

What you are looking at is an MVP / alpha. There is no legacy cloud compatibility layer to maintain, and there should not be. When something is wrong, the right move is to fix it correctly and leave no trace — that principle is encoded throughout the engineering commitments below.

g8e is developed by Lateralus Labs, LLC — a Certified Veteran Owned Small Business (VOSB). AI governance and safety research is the actual reason the company exists... every architectural decision in this codebase is supposed to be an expression of that.

---

## Engineering Commitments

### User Intent is the guiding principle

Every architectural decision in this codebase derives from one foundational principle: the user makes informed decisions, and the platform exists to support that. g8e operates in full context mode — through Operators it builds a continuously improving picture of the user's environment, history, and preferences. That context compounds over time into a working relationship where the platform knows the user's infrastructure and how they work. All of that capability exists in service of one outcome: the human is always the one making state-changing decisions. Human control is a first-class architectural property, not an afterthought. Every commitment below is a direct expression of that principle — hold it as the lens through which every contribution should be evaluated.

Before you look at the code... a heads up. A significant portion of the characters in this codebase were laid down by AI — directed and reviewed by me, and actively refactored by both of us over time, but AI had a substantial hand in the actual implementation. I've been maintaining some level of order by documenting the guidelines you'll find throughout [docs/](../docs/), but there is still genuinely smelly code in here that I'm actively working to clean up. If you find a section that's poorly designed, over-engineered, or just wrong — please submit a PR. That's not a polite thing to say... the project actually needs it. Code quality PRs are as welcome as feature PRs, maybe more so.

With that said, here are the non-negotiable constraints that every contribution needs to respect — load-bearing walls, treat them accordingly.

### Human agency is the security model

The AI proposes, the human approves, nothing executes without explicit user consent — that is how the security model works, the whole thing is built around that chain. Automatic Function Calling is permanently disabled. Every state-changing operation surfaces its own approval prompt... you can't batch-approve a multi-step operation and walk away, that's intentional. The human-in-the-loop requirement is enforced at the platform level and can't be bypassed by the AI model or by any API call.

Any change that increases AI autonomy without a corresponding, explicit increase in human control and auditability is a no.

### Security and privacy are bedrock, not features

Security is the first constraint — functionality gets built inside it, not the other way around. Before writing code for any new feature, ask yourself: does it introduce a new inbound port or network exposure? Does it persist raw command output or file contents on the platform side? Does it let client-supplied values be used as authorization context? Does it give the AI any path around the human-in-the-loop requirement? If any of those are yes, stop and re-read [docs/architecture/security.md](architecture/security.md) first — the Security Review Checklist at the end of that doc is a mandatory gate for anything touching security-relevant code.

### No shims, no backwards-compatibility layers, no tech debt

The codebase went through a full transition from cloud SaaS to self-hosted — there's no legacy compatibility layer to maintain, and there shouldn't be. When something is wrong, rip it out and replace it correctly... leave no trace. The prohibitions in [Code Quality](#code-quality) below are hard stops — `ensure*()`, `getOrCreate*()`, `JSON.stringify` as a type coercion fallback, `map[string]interface{}` for known shapes, `Any` in service signatures — any of these showing up in new code will be rejected at review time, full stop.

### Data sovereignty is a hard guarantee

The platform is a stateless relay — raw command output and file contents stay on the Operator host, encrypted, and never touch the platform side in persistent form. This is tested in the contracts and enforced by the LFAA architecture... a feature request that requires relaxing it is a feature request that gets declined.

### The shared/ directory is the source of truth

All wire-protocol values and cross-component document schemas live in `shared/` — no component gets to define its own wire values, no component invents a schema field that doesn't already exist in `shared/models/`. Wire-protocol breaking changes start in `shared/` first, always, before touching any component implementation.

### Service Hierarchy Principle (Domain vs. Data)

To ensure long-term maintainability and strict separation of concerns, all component services must adhere to a two-tier hierarchy:

1.  **Domain Layer (Orchestration):** High-level services (e.g., `InvestigationService`) that host business logic, coordinate multiple data services, manage complex state transitions, and record audit trails.
2.  **Data Layer (CRUD):** Low-level services (e.g., `InvestigationDataService`) that provide pure CRUD operations for a specific collection or resource. These services MUST NOT contain business logic, history management, or cross-service orchestration.

**Dependency Management:** Services MUST interact via **Protocols** (defined in `app/services/protocols.py`) rather than concrete classes. This enables clean mocking in tests and prevents circular dependencies between layers.

---

## Quick Start

```bash
# First-time setup — builds g8ep, generates SSL certs, compiles the Operator binary,
# then builds and starts all platform services
./g8e platform setup
```

Only Docker required. `platform setup` is the all-inclusive first-run command: it builds the g8ep image, generates TLS certificates if missing, compiles the Operator binary, and does a full no-cache rebuild of all platform services (G8es, g8ee, g8ed).

After `platform setup` completes, open `https://localhost` — on first run you will be redirected to the setup wizard. The wizard lets you trust the CA certificate (download button + per-OS instructions built in), register your admin account, configure the platform hostname, and set up your LLM provider.

All configuration — LLM provider, API keys, passkey config, cert paths, session tuning, app URL, CORS — flows through `SettingsService.getConfig()`. Use `./g8e platform restart` to apply changes made via CLI or UI.

Three core services run: g8es (persistence + pub/sub), g8ee (AI engine), and g8ed (web frontend + Operator gateway). All data stays on your infrastructure. The only external network connection is to the configured LLM endpoint.

### Subsequent rebuilds

```bash
# Rebuild after code changes or a git pull — expects SSL certs to already exist
./g8e platform rebuild
```

`platform rebuild` assumes SSL certificates are already present. If they are missing it will error and tell you to run `platform setup`. Use `rebuild` for day-to-day development; use `setup` only when starting from scratch or after `platform clean`.

> **Recommended LLM: Google Gemini 3.1.** The platform was designed around Gemini best practices and the Gemini integration is the most robust and extensively tested. Other providers are supported but are not part of the standard test pipeline.

For the best experience, assign a DNS name instead of using `localhost` — see **Configure DNS** below.

---
## Ollama Thinking Support

The platform includes special support for Ollama's thinking feature, which allows models to show their reasoning process. This is implemented in the `OpenAICompatibleProvider` with automatic endpoint detection and direct HTTP calls.

### Implementation Details

- **Automatic Detection**: Ollama endpoints are detected by checking for 'ollama' or '11434' in the URL
- **Direct HTTP Calls**: Bypasses the OpenAI client limitations by making direct HTTP calls to Ollama's `/api/chat` endpoint
- **Parameter Handling**: Automatically sends `think: true` parameter to enable thinking
- **Response Processing**: Extracts `thinking` field from responses and streams it as `thought=True` chunks
- **URL Conversion**: Converts OpenAI-style `/v1` endpoints to Ollama's `/api` endpoints

### Supported Models

Not all Ollama models support thinking. Use models that explicitly support the thinking feature:
- `qwen3.5:4b` - Primary model with thinking support
- `qwen3.5:9b` - Larger model with thinking support

### Testing

Run the thinking puzzle integration test to verify Ollama thinking works:

```bash
./g8e test g8ee -p ollama -e http://10.0.0.1:11434/v1 -m qwen3.5:4b -a qwen3.5:4b -- tests/integration/test_agent_thinking_puzzle_integration.py
```

---

## Web Search (optional)

The `search_web` AI tool lets the AI search the web during investigations. It requires a Vertex AI Search (Discovery Engine) app in a GCP project.

### One-time GCP setup

**Step 1 — Create an API key**

Go to `https://console.cloud.google.com/apis/credentials` and click **Create credentials** > **API key**.

Recommendation: keep it simple — restrict the key to these two APIs only:
- **Gemini for Google Cloud API** — used by the AI engine
- **Discovery Engine API** — used by web search

This single key is used for both `GEMINI_API_KEY` and `VERTEX_SEARCH_API_KEY`. Allow up to 5 minutes for the restrictions to take effect after saving.

**Step 2 — Enable the Discovery Engine API**

`https://console.cloud.google.com/apis/library/discoveryengine.googleapis.com`

**Step 3 — Create a search app**

Go to `https://console.cloud.google.com/gen-app-builder/engines`, click **Create app** > **Custom search (general)**, and follow the prompts to create a Website data store and add your domains. Note the App ID after creation.

### Configure g8e

```bash
./g8e search setup
```

Walks through collecting and validating the GCP project ID, engine ID, API key, and location, then writes them to the platform database (g8es). Run `./g8e platform restart` to apply.

Non-interactive (CI/scripted):

```bash
./g8e search setup \
  --project-id your-gcp-project \
  --engine-id  your-engine-id \
  --api-key    your-api-key
```

To remove the configuration:

```bash
./g8e search disable
```

---

## SSL Setup

g8e runs over HTTPS. g8es (g8es) generates the CA and server certificates automatically on first start and stores them in its data volume (`g8es-data`). You only need to interact with cert management to trust the CA in your browser (once, to eliminate warnings) or to force regeneration after a `reset` or `clean` (both destroy the g8es volume). `wipe` preserves certs — it only clears the g8ee and g8ed app data volumes.

### 1. Check Certificate Status

```bash
./g8e security certs status
```

### 2. Generate or Rotate Certificates

Certs are generated automatically when g8es starts — no manual step required for a fresh install. If you need to force regeneration:

```bash
./g8e security certs generate   # Ensure certs exist — no-op if already present
./g8e security certs rotate     # Force-regenerate CA + server cert
```

After `rotate`, restart the platform so g8ed and g8ee reload the new certificates. The Operator binary does not need to be rebuilt — it discovers the CA at startup (from the local SSL volume or via HTTPS fetch), so it will automatically pick up the new CA on next launch.

```bash
./g8e platform restart
```

### 3. Configure DNS

For a local install, the simplest approach is to add `g8e.local` to your `/etc/hosts` file pointing to `127.0.0.1`. Then set `APP_URL` to `https://g8e.local` via the browser settings wizard (Settings → General → Application URL). This is optional — `localhost` works without any configuration.

If g8e is running on a **remote host**, add an entry to `/etc/hosts` (or your DNS server) pointing to the server's IP address:

```
# /etc/hosts
192.168.1.x   g8e.local
```

Then regenerate the server certificate:

```bash
./g8e security certs rotate
```

### 4. Install the CA Certificate

See [Security Architecture > Workstation CA Trust](architecture/security.md#workstation-ca-trust) for the authoritative guide on trusting the g8e CA on macOS, Windows, and Linux workstations.

**macOS (Manual):**

```bash
sudo security add-trusted-cert -d -r trustRoot \
    -k /Library/Keychains/System.keychain ~/Downloads/g8e-ca.crt
```

**Linux (Debian/Ubuntu Manual):**

```bash
sudo cp ~/Downloads/g8e-ca.crt /usr/local/share/ca-certificates/g8e-ca.crt
sudo update-ca-certificates
```

**Linux (RHEL/Fedora Manual):**

```bash
sudo cp ~/Downloads/g8e-ca.crt /etc/pki/ca-trust/source/anchors/g8e-ca.crt
sudo update-ca-trust
```

**Windows (Automated):**

g8e provides a 1-Click Installer (`.bat`) via the web UI at `https://127.0.0.1/setup` that self-elevates and installs the certificate automatically.

Alternatively, use the remote PowerShell script:

Copy `scripts/security/trust-ca.ps1` to your machine and run it in an **Administrator PowerShell** prompt:

```powershell
.\trust-ca.ps1 -Server admin@10.0.0.2
```

This removes any old g8e CA cert, fetches the new one via SSH, and imports it — all in one step. The `-Server` parameter accepts `user@host`, a bare hostname, or an SSH config alias.

Manual equivalent (Administrator PowerShell):

```powershell
# Remove old cert
Get-ChildItem Cert:\LocalMachine\Root | Where-Object { $_.Subject -like '*g8e*' } | Remove-Item

# Fetch and save
ssh admin@10.0.0.2 "docker exec g8ep cat /g8es/ssl/ca.crt" | Out-File -Encoding ascii $env:USERPROFILE\Downloads\g8e-ca.crt

# Import
Import-Certificate -FilePath "$env:USERPROFILE\Downloads\g8e-ca.crt" -CertStoreLocation Cert:\LocalMachine\Root
```

---

## Infrastructure

### Docker Compose

`docker-compose.yml` is the single deployment configuration. All services, profiles, and volumes are defined here.

```
docker-compose.yml
├── g8es       # Persistence + pub/sub (g8e.operator --listen)
├── g8ee         # AI engine (Python/FastAPI)
├── g8ed        # Web frontend + Gateway Protocol (Node.js/Express)
└── g8ep    # Unified test/security/build environment
```

```bash
./g8e platform start            # Start all services
./g8e platform stop             # Stop all services (data preserved)
./g8e platform wipe             # Clear app data (g8ee, g8ed); preserve g8es
./g8e platform reset            # Wipe ALL data volumes and rebuild from scratch (destructive)
```

**Profiles:**

| Profile | Services Added | Purpose |
|---------|---------------|---------|
| *(none)* | `g8ep` | Unified test/security/build environment — no Docker profile; runs alongside core services but excluded from managed `up`/`rebuild`/`wipe`/`clean` operations unless explicitly targeted (e.g. `./g8e platform rebuild g8ep`) |

### Configuration and Environment Variables

Platform settings (LLM provider, API keys, passkey config, web search, SSL, etc.) are stored in g8es (`components/platform_settings`) and managed via the browser Settings page, `./g8e llm setup`, or `./g8e search setup`. On first boot, g8es auto-generates bootstrap secrets (`INTERNAL_AUTH_TOKEN`, `SESSION_ENCRYPTION_KEY`) during database initialization and persists them to the `platform_settings` document — no manual pre-configuration required.

**Configuration flows through the platform_settings pipeline — not environment variables.** At runtime, g8ed reads all settings via `SettingsService` (populated from g8es on startup) and g8ee reads all settings via `Settings` (loaded from g8es via `Settings.from_db()`). Neither component reads `process.env` or `os.environ` for any runtime configuration.

The only two legitimate environment variable reads at runtime are the bootstrap transport URLs used by g8ed's `initialization.js` before g8es is reachable:

| Variable | Purpose | Default |
|----------|---------|--------|
| `G8E_INTERNAL_HTTP_URL` | g8ed HTTP URL — needed to reach g8es before settings can be loaded | `https://g8es` |
| `G8E_INTERNAL_PUBSUB_URL` | g8es pub/sub WebSocket URL — needed to establish the pub/sub connection before settings can be loaded | `wss://g8es` |

All other variables in the table below are **Docker Compose environment block entries** that feed into the g8es `platform_settings` document on first boot only. They do not persist as runtime env var reads in the application code.

Never use `.env` files for secrets.

#### Internal Secret Genesis

`INTERNAL_AUTH_TOKEN` and `SESSION_ENCRYPTION_KEY` follow a volume-only lifecycle:

1.  On first start, **g8es** (`g8e.operator --listen`) checks for these secrets on its SSL volume (`g8es-ssl`, mounted at `/ssl` in the container).
2.  If missing, g8es generates cryptographically secure 32-byte hex values and writes them to `/ssl/internal_auth_token` and `/ssl/session_encryption_key`.
3.  **g8ed** and **g8ee** mount the same volume at `/g8es/ssl` and read these secrets directly from the filesystem at startup.
4.  These secrets are **never stored in the database** and cannot be managed via the UI. This ensures platform identity and session encryption are decoupled from the database lifecycle, surviving full database wipes or resets as long as the SSL volume is preserved.

To rotate these secrets, manually delete the files from the volume and restart the platform. g8es will regenerate them on the next boot.

**Docker Compose environment entries (feed into g8es on first boot — not runtime env var reads):**

| Variable | Purpose | Default |
|----------|---------|---------|
| `APP_URL` | Platform base URL — source for passkey and hostname defaults | `https://localhost` |
| `G8E_OPERATOR_PUBSUB_URL` | g8es pub/sub WebSocket URL for operators — external connection via g8ed on port 443 | `wss://localhost:443` |
| `LLM_PROVIDER` | LLM backend type | `gemini` |
| `OPENAI_ENDPOINT` | OpenAI API base URL | `https://api.openai.com/v1` |
| `OLLAMA_ENDPOINT` | Ollama API base URL | `https://localhost:11434/v1` |
| `ANTHROPIC_ENDPOINT` | Anthropic API base URL | `https://api.anthropic.com/v1` |
| `LLM_MODEL` | Primary reasoning model | `gemini-3.1-flash` |
| `LLM_ASSISTANT_MODEL` | Lightweight model for fast tasks | `gemini-3.1-flash` |
| `OPENAI_API_KEY` | OpenAI API key | *(empty)* |
| `OLLAMA_API_KEY` | Ollama API key | *(empty)* |
| `GEMINI_API_KEY` | Gemini API key | *(empty)* |
| `ANTHROPIC_API_KEY` | Anthropic API key | *(empty)* |
| `G8E_DB_PATH` | SQLite path per component | `/data/g8e.db` |
| `INTERNAL_AUTH_TOKEN` | Shared secret for inter-service authentication (g8ed ↔ g8ee ↔ g8es) | *(auto-generated on first boot)* |
| `PASSKEY_RP_ID` | WebAuthn relying party ID — must match the hostname users browse to (no scheme, no port) | *(derived from `APP_URL`)* |
| `PASSKEY_ORIGIN` | WebAuthn expected origin — must match the exact origin users browse to (`scheme://host`) | *(derived from `APP_URL`)* |
| `PASSKEY_RP_NAME` | WebAuthn relying party display name | `g8e` |
| `SESSION_ENCRYPTION_KEY` | AES-256 key (64 hex chars / 32 bytes) for encrypting sensitive session fields — generate with `openssl rand -hex 32` | *(auto-generated on first boot)* |

### Docker-Only Workflow

The only hard prerequisite for running and managing the full platform is Docker. No Go, Python, openssl, or other tools are required on the host.

The `g8e` bash script is the authoritative router — host commands (platform, operator build/deploy, llm) run directly on the host; in-pod commands (test, security, data) `docker exec` into the g8ep exactly once. The Operator binary is never built automatically — run `./g8e platform setup` explicitly before using any operator functionality.

```bash
./g8e platform setup              # Build all images, generate certs, and build operator (alias: setup)
./g8e platform settings          # Show effective platform settings (requires platform running)
./g8e platform update            # Pull latest changes (with confirmation) and rebuild
./g8e platform rebuild           # No-cache rebuild of g8es, g8ee, g8ed + restart (volumes preserved)
./g8e platform wipe              # Clear app data (g8ee, g8ed); preserve g8es
./g8e platform reset             # Wipe ALL data volumes and rebuild from scratch (destructive)
./g8e platform start             # Start the platform (no rebuild)
./g8e platform restart           # Restart all services (no rebuild)
./g8e platform stop              # Stop all services
./g8e platform clean             # Remove all managed Docker resources (containers, images, volumes, networks)
./g8e platform status            # Show container status and versions
./g8e platform logs [service]    # Tail service logs
./g8e test g8ee                   # Run g8ee tests
./g8e security certs status      # Show certificate status and expiry
./g8e data users list             # List users
./g8e data operators list --email <e>   # List operators for a user
./g8e --help                     # Full command reference
```

### Building g8eo

```bash
# From components/g8eo/
make build-local-all  # All Linux platforms (amd64, arm64, 386) — release build
make build-local      # Linux amd64 only — fast, uploads to local g8es
```

---

## Testing

See [testing.md](testing.md) for complete testing documentation — shared principles, g8ep environment, test infrastructure, fixtures, mocks, CI workflows, and component-specific guidelines.

---

## Code Quality

### Universal Rules

These apply to every component — g8eo, g8ee, g8ed — without exception.

- **No emojis** in application code, comments, log messages, or any runtime strings (markdown docs only)
- **No inline styles** in HTML — use proper CSS, leverage existing definitions before creating new ones
- **No `.env` files for secrets — all runtime configuration via the platform_settings pipeline (`SettingsService` / `Settings`), never via environment variables
- **No backwards-compatibility shims** — rip and replace, leave no trace
- **No unnecessary comments** — only leave comments that are necessary or extremely important
- **Async Generator Safety** — Never use `ContextVar.reset()` or any state-modifying `finally` blocks inside an async generator. Python executes async generator cleanup in a different `asyncio` Context, which causes `ContextVar.reset()` to raise `ValueError`. Move lifecycle management to the caller (e.g., `run_with_sse`).
- **Settings Merging** — Always use `SettingsService` for merging configuration. The precedence is `Defaults < Env < Platform < User`. Never implement manual merging logic in individual services or routes.

#### Prohibited Patterns

**`ensure*()`** — Forbidden. Hides control flow, conflates read and write concerns. If something needs to exist, create it explicitly at a known point in the lifecycle. If it might not exist, return null and let the caller decide.

**`getOrCreate*()`** (and variants: `findOrCreate`, `upsertIfMissing`, etc.) — Forbidden. Masks whether a write occurs and makes idempotency assumptions that are usually wrong. Use explicit `get` and `create` as separate operations.

**The rule:** every function does exactly one thing. Reads read. Writes write. Startup code that needs documents to exist creates them explicitly.

---

## Shared Constants and Models

**`shared/` is the canonical source of truth for all wire-protocol values and cross-component document schemas.** Every component derives its types from `shared/` — never define wire-protocol string values or document field shapes independently in a component.

### `shared/constants/`

| File | Contains |
|------|----------|
| `events.json` | All pub/sub event type strings |
| `status.json` | All status strings — operator status, execution status, heartbeat type, vault mode, component name, platform, etc. |
| `channels.json` | Pub/sub channel prefix patterns and auth channel patterns |
| `senders.json` | `EventType` sender paths, `StreamChunkType` |
| `collections.json` | g8es collection names |
| `kv_keys.json` | KV key prefix patterns and cache version |
| `headers.json` | Shared HTTP header names (`x-g8e.*` and standard HTTP) |
| `pubsub.json` | Pub/sub wire protocol actions, event types, and fields |
| `document_ids.json` | Canonical document IDs for database collections |
| `intents.json` | Intent type strings for AI command classification |
| `prompts.json` | Prompt key constants and agent mode strings |
| `timestamp.json` | Timestamp field names and format constants |

**Rule:** To change a wire-protocol value (event type, status string, channel prefix), update the relevant JSON in `shared/constants/` first. All component constant files are consumers — they import or mirror from `shared/`. Never edit a component's constants file to introduce a new value that does not exist in the corresponding shared JSON.

### `shared/models/`

| File | Authoritative writer |
|------|---------------------|
| `operator_document.json` | g8ed |
| `conversation.json` | g8ed |
| `user.json` | g8ed |
| `investigation.json` | g8ee |
| `case.json` | g8ee |
| `conversation_message.json` | g8ee (publisher), g8ed (consumer) |
| `tool_results.json` | g8ee (publisher), g8ed (consumer) |
| `wire/envelope.json` | g8eo (publisher), g8ee/g8ed (consumers) |
| `wire/heartbeat.json` | g8eo (publisher), g8ee/g8ed (consumers) |
| `wire/result_payloads.json` | g8eo (publisher), g8ee/g8ed (consumers) |
| `wire/command_payloads.json` | g8ed/g8ee (publishers), g8eo (consumer) |
| `wire/system_info.json` | g8eo (publisher), g8ed (consumer) |

**Rule:** When adding or renaming a field on any shared model, update `shared/models/` first. Then update all component implementations in the same change. A field mismatch between the JSON definition and a component struct is a wire-protocol breaking change.

### How Each Component Consumes `shared/`

The three components use different consumption mechanisms suited to their language and runtime:

#### g8eo (Go) — compile-time mirroring

g8eo is a compiled binary. It does not load the shared JSON at runtime. Instead, the Go constants in `constants/` **duplicate** the shared JSON values as typed Go constants. The values are hardcoded at compile time and must exactly match the JSON source of truth.

Contract tests in `contracts/` are the enforcement mechanism:

| Test file | What it verifies |
|-----------|-----------------|
| `contracts/shared_constants_test.go` | Reads `shared/constants/*.json` at test time and asserts every Go constant in `constants/events.go`, `constants/status.go`, `constants/channels.go`, and `constants/headers.go` exactly matches the corresponding JSON value |
| `contracts/shared_wire_models_test.go` | Reads `shared/models/wire/*.json` at test time and asserts every `json:` struct tag in the g8eo wire model structs (`models/wire.go`, `models/commands.go`, etc.) exactly matches the field names defined in the JSON schema |
| `contracts/constants_enforcement_test.go` | Uses Go's AST parser to scan g8eo source files in `services/`, `models/`, and `config/` for raw string literals that match any enforced constant value — fails if any raw string duplicates a constant that should be referenced by name |

**Access path:** `contracts/` resolves `shared/` via `filepath.Abs(filepath.Join(g8eoRoot, "../../shared/..."))` — two levels up from `components/g8eo/` to the repo root.

When a shared JSON value changes: update the JSON, then update the matching Go constant, then run `./g8e test g8eo -- ./contracts/...` to verify.

#### g8ee (Python) — selective runtime loading

Some files in `app/constants/` open shared JSON directly via `os.path` when the module is first imported. The path is resolved relative to the constants file itself:

```python
_SHARED_DIR = PATHS["infra"]["shared_constants_dir"]
```

This resolves to `/app/shared/constants` in the container. Each constants file that loads from shared JSON opens the file and constructs classes whose member values are pulled directly from the JSON dict. If a shared JSON file is missing or malformed, the `RuntimeError` raised at import time will abort g8ee startup — there is no silent fallback.

The `shared/` directory is mounted read-only into the g8ee container:

```yaml
volumes:
  - ./shared:/app/shared:ro
```

| g8ee constants file | Shared JSON consumed |
|--------------------|---------------------|
| `app/constants/api_paths.py` | `api_paths.json` |
| `app/constants/status.py` | `status.json` |
| `app/constants/collections.py` | `collections.json` |
| `app/constants/kv_keys.py` | `kv_keys.json` |
| `app/constants/errors.py` | `errors.json` (from shared/models) |

`app/constants/events.py`, `app/constants/channels.py`, `app/constants/intents.py`, `app/constants/prompts.py`, `app/constants/headers.py` contain hardcoded constants and do not load from shared JSON. `app/constants/config.py` and `app/constants/platform.py` contain g8ee-internal constants only.

#### g8ed (Node.js) — runtime loading via `constants/shared.js`

All shared JSON is loaded through a single entry point: `constants/shared.js`. It uses Node's `createRequire` to load every shared JSON file at module import time and re-exports each as a named binding:

```js
const require = createRequire(import.meta.url);
export const _EVENTS      = require('../../../shared/constants/events.json');
export const _STATUS      = require('../../../shared/constants/status.json');
// ... etc.
```

Individual g8ed constants files (`events.js`, `channels.js`, `collections.js`, etc.) import their needed binding from `shared.js` and construct `Object.freeze`d constants objects by pulling values from the JSON dict. This means the JSON is loaded once when `shared.js` is first imported and every downstream constants file reads from the already-parsed object.

The `shared/` directory is mounted read-only into the g8ed container at `/shared`:

```yaml
volumes:
  - ./shared:/shared:ro
```

`constants/shared.js` resolves the path from the root `/shared/constants/` in production, or via relative paths in development/test.

| g8ed constants file | Shared binding(s) consumed |
|---------------------|---------------------------|
| `constants/events.js` | `_EVENTS` |
| `constants/channels.js` | `_CHANNELS`, `_PUBSUB` |
| `constants/collections.js` | `_COLLECTIONS` |
| `constants/kv_keys.js` | `_KV` |
| `constants/headers.js` | `_HEADERS` |
| `constants/operator.js` | `_STATUS` |
| `constants/auth.js` | `_STATUS`, `_HEADERS` |
| `constants/ai.js` | `_STATUS` |
| `constants/session.js` | `_STATUS` |
| `constants/chat.js` | `_STATUS`, `_MSG` |
| `constants/operator_defaults.js` | `_INTENTS` |
| `constants/api_paths.js` | `_API_PATHS` (from shared/constants/api_paths.json) |

Three contract test files enforce correctness:

| Test file | What it verifies |
|-----------|-----------------|
| `test/contracts/shared-loader.test.js` | `constants/shared.js` resolves to the correct path and loads all JSON without error |
| `test/contracts/shared-pubsub-constants.test.js` | Verifies PubSubAction and PubSubMessageType values against `shared/constants/pubsub.json` |
| `test/contracts/shared-definitions.test.js` | Every g8ed constant loaded from shared JSON exactly matches the value in the source JSON — catches key renames and value drift |
| `test/contracts/constants-enforcement.test.js` | Scans `services/`, `routes/`, `middleware/`, `models/`, and `utils/` for raw string literals that duplicate a value already defined in `constants/` |
| `test/frontend/constants/api-paths.unit.test.js` | Verifies that `InternalApiPaths` matches the shared `api_paths.json` contract |

---

## g8ee ↔ g8ed Integration

g8ee and g8ed are the two halves of the g8e backend. Every user action that reaches g8ee originates in g8ed. Every SSE event the browser receives originates in g8ee and is routed through g8ed. The integration surface is the set of HTTP calls between them, the shared constant and model definitions that both sides must agree on, and the `G8eHttpContext` headers that carry identity and business context across the boundary.

```
Browser ←→ g8ed (Node.js/Express) ←→ g8ee (Python/FastAPI) ←→ g8eo (Go operator binary)
                ↕                           ↕
              g8es (pub/sub + document store)
```

### Communication Patterns

| Direction | Mechanism | Purpose |
|-----------|-----------|---------|
| g8ed → g8ee | HTTP (`InternalHttpClient`) | Chat, operator lifecycle, approval, direct command, MCP gateway |
| g8ee → g8ed | HTTP (`InternalHttpClient `) | SSE push, intent grant/revoke, heartbeat broadcast, operator context update |
| g8ee → g8eo | g8es pub/sub | Command dispatch, shutdown |
| g8eo → g8ee | g8es pub/sub | Execution results, heartbeats |

### Integration Surface Map

This is the complete set of HTTP calls between g8ed and g8ee with the request model name on both sides. Both sides must agree on field names and types.

#### g8ed → g8ee (via `InternalHttpClient`)

| g8ed caller | Path | g8ed request model | g8ee handler | g8ee request model |
|-------------|------|--------------------|-------------|-------------------|
| `chat_routes.js` | `POST /api/internal/chat` | `ChatMessageRequest.forWire()` | `internal_router.py::internal_chat` | `ChatMessageRequest` (internal_api.py) |
| `chat_routes.js` | `POST /api/internal/chat/stop` | `StopAIRequest.forWire()` | `internal_router.py::stop_ai_processing` | `StopAIRequest` (internal_api.py) |
| `chat_routes.js` | `GET /api/internal/investigations` | `URLSearchParams` | `internal_router.py::query_investigations` | — |
| `chat_routes.js` | `GET /api/internal/investigations/:id` | — | `internal_router.py::get_investigation` | — |
| `chat_routes.js` | `DELETE /api/internal/cases/:id` | — | `internal_router.py::delete_case` | — |
| `OperatorRelayService` (via approval route) | `POST /api/internal/operator/approval/respond` | `ApprovalRespondRequest.forWire()` | `internal_router.py::operator_approval_respond` | `OperatorApprovalResponse` (internal_api.py) |
| `OperatorRelayService` (via approval route) | `POST /api/internal/operator/direct-command` | `DirectCommandRequest.forWire()` | `internal_router.py::execute_direct_command` | `DirectCommandRequest` (internal_api.py) |
| `OperatorService` | `POST /api/internal/operators/register-operator-session` | `OperatorSessionRegistrationRequest.forWire()` | `internal_router.py::register_operator_session` | `OperatorSessionRegistrationRequest` (internal_api.py) |
| `OperatorService` | `POST /api/internal/operators/deregister-operator-session` | `OperatorSessionRegistrationRequest.forWire()` | `internal_router.py::deregister_operator_session` | `OperatorSessionRegistrationRequest` (internal_api.py) |
| `OperatorService` | `POST /api/internal/operators/stop` | `StopOperatorRequest.forWire()` | `internal_router.py::stop_operator` | `StopOperatorRequest` (internal_api.py) |
| `mcp_routes.js` | `POST /api/internal/mcp/tools/list` | `{}` (empty body) | `internal_router.py::mcp_tools_list` | — (uses G8eHttpContext only) |
| `mcp_routes.js` | `POST /api/internal/mcp/tools/call` | `{ tool_name, arguments, request_id }` | `internal_router.py::mcp_tools_call` | `MCPToolCallRequest` (internal_api.py) |

#### g8ee → g8ed (via `InternalHttpClient `)

| g8ee caller | Path | g8ee request model | g8ed handler | g8ed response model |
|------------|------|-------------------|--------------|---------------------|
| `g8ed_event_service.py` | `POST /api/internal/sse/push` | `SSEPushPayload` | `internal_sse_routes.js` | `{ success, delivered }` → `SSEPushResponse` |
| `internal_http_client.py` (intent_service) | `POST /api/internal/operators/:id/grant-intent` | `IntentRequestPayload` | `internal_operator_routes.js` | `{ success, granted_intents, expires_at }` → `GrantIntentResponse` |
| `internal_http_client.py` (intent_service) | `POST /api/internal/operators/:id/revoke-intent` | `IntentRequestPayload` | `internal_operator_routes.js` | `{ success, granted_intents }` → `RevokeIntentResponse` |

### Cross-Boundary Model Name Alignment

The following model pairs are the same shape on both sides of the g8ed→g8ee boundary and must remain synchronized. Any field rename must happen in both simultaneously.

| Concept | g8ed model | g8ee model | Notes |
|---------|-----------|-----------|-------|
| Chat message | `ChatMessageRequest` (request_models.js) | `ChatMessageRequest` (internal_api.py) | Body only — identity via headers |
| Stop AI | `StopAIRequest` (request_models.js) | `StopAIRequest` (internal_api.py) | Aligned |
| Approval respond | `ApprovalRespondRequest` (request_models.js) | `OperatorApprovalResponse` (internal_api.py) | Names differ — wire body carries `approval_id`, `approved`, `reason`; g8ee router enriches `operator_session_id`/`operator_id` from `G8eHttpContext.bound_operators[0]` before passing to `approval_service` |
| Direct command | `DirectCommandRequest` (request_models.js) | `DirectCommandRequest` (internal_api.py) | Aligned — body-only payload (`command`, `execution_id`, `hostname`); all identity via `G8eHttpContext` |
| Operator session registration | `OperatorSessionRegistrationRequest` (request_models.js) | `OperatorSessionRegistrationRequest` (internal_api.py) | Aligned |
| Stop operator | `StopOperatorRequest` (request_models.js) | `StopOperatorRequest` (internal_api.py) | Aligned |
| Bound operator context | `BoundOperatorContext` (request_models.js) | `BoundOperator` (http_context.py) | Names differ but shapes aligned — both serialize into `X-G8E-Bound-Operators` JSON array |

**Name divergence (`ApprovalRespondRequest` vs `OperatorApprovalResponse`):** The g8ed model is named a *Request* and the g8ee model is named a *Response*. Both carry only approval-specific fields (`approval_id`, `approved`, `reason`). All identity and routing context (`user_id`, `web_session_id`, `case_id`, `investigation_id`, `task_id`, `bound_operators`) travels via `G8eHttpContext` headers. The naming asymmetry is confusing but not a wire bug.

**`BoundOperatorContext` vs `BoundOperator`:** g8ed serializes `BoundOperatorContext` instances into the `X-G8E-Bound-Operators` JSON header. g8ee deserializes them as `BoundOperator` instances. The field shapes are aligned (`operator_id`, `operator_session_id` (optional), `status`, `operator_type`, `system_info`). The name difference is safe as long as `forWire()` output matches `BoundOperator` field names exactly.

### `G8eHttpContext` Flow — End-to-End

These traces show how context flows from a browser action through g8ed to g8ee for the critical paths.

#### Chat message (new conversation)

```
Browser: POST /api/chat/send  { message, sentinel_mode, llm_primary_model, llm_assistant_model }
  g8ed chat_routes.js:
    g8eContext = { web_session_id, user_id, org_id, case_id: null, investigation_id: null, bound_operators }
    ChatMessageRequest.forWire() → body (message, attachments, sentinel_mode, llm_primary_model, llm_assistant_model)
  → InternalHttpClient.sendChatMessage(body, g8eContext)
  → HTTP POST /api/internal/chat + X-G8E-* headers
  g8ee internal_router.py::internal_chat:
    g8e_context = get_g8e_http_context(...)  ← case_id/investigation_id ABSENT
    not g8e_context.case_id → True
    → creates case + investigation inline
    → g8e_context = g8e_context.model_copy(update={"case_id": ..., "investigation_id": ...})
    → fires asyncio.create_task(chat_pipeline.run_chat(...))
    → returns ChatStartedResponse(case_id, investigation_id)
```

New vs existing conversation is derived entirely from `g8e_context.case_id` — empty string means g8ee creates case + investigation inline; non-empty means existing conversation, no creation.

#### Chat message (existing conversation)

```
Browser: POST /api/chat/send  { message, case_id: "...", investigation_id: "...", ... }
  G8ED:
    g8eContext = { ..., case_id: "...", investigation_id: "..." }
  → HTTP POST /api/internal/chat + X-G8E-Case-ID + X-G8E-Investigation-ID headers
  G8EE:
    g8e_context.case_id = "..."  (populated)
    not g8e_context.case_id → False
    → skips case/investigation creation
    → fires run_chat with existing case/investigation
```

#### Operator approval response

```
Browser: POST /api/operators/approval/respond  { approval_id, approved, reason, case_id, investigation_id, task_id }
  g8ed operator_approval_routes.js:
    ApprovalRespondRequest.parse(req.body) → validates approval_id, approved, reason
    { case_id, investigation_id, task_id } read directly from req.body for G8eHttpContext
    bound_operators = await resolveBoundOperators(web_session_id)
    g8eContext = { web_session_id, user_id, org_id, case_id, investigation_id, task_id, bound_operators }
    ApprovalRespondRequest.forWire() → body (approval_id, approved, reason)
  → HTTP POST /api/internal/operator/approval/respond + all X-G8E-* headers
  g8ee internal_router.py::operator_approval_respond:
    g8e_context = get_g8e_http_context(...)  ← case_id + investigation_id from headers
    response = OperatorApprovalResponse(approval_id, approved, reason)
    response.operator_session_id = g8e_context.bound_operators[0].operator_session_id
    response.operator_id = g8e_context.bound_operators[0].operator_id
    → approval_service.handle_approval_response(response: OperatorApprovalResponse)
```

`case_id`, `investigation_id`, and `task_id` are context fields sent by the browser but consumed exclusively via `G8eHttpContext` headers — they are not part of `ApprovalRespondRequest`. The body carries only the user's approval action (`approval_id`, `approved`, `reason`). Identity fields (`user_id`, `web_session_id`) are resolved server-side from the authenticated session and never sent by the frontend. Operator identity is resolved via `resolveBoundOperators()` and travels in `G8eHttpContext.bound_operators`.

#### Direct command (anchored terminal)

```
Browser: POST /api/operators/approval/direct-command  { command, hostname, case_id?, investigation_id? }
  g8ed operator_approval_routes.js:
    bound_operators = await resolveBoundOperators(web_session_id)
    g8eContext = { web_session_id, user_id, org_id, case_id, investigation_id, bound_operators }
    DirectCommandRequest.forWire() → body (command, execution_id, hostname)
  → HTTP POST /api/internal/operator/direct-command + all X-G8E-* headers
  g8ee internal_router.py::execute_direct_command:
    g8e_context = get_g8e_http_context(...)  ← operator resolved from bound_operators[0]
    → operator_data_service.send_command_to_operator(command_payload, g8e_context)
       resolves operator_id/operator_session_id from g8e_context.bound_operators[0]
    → operator_data_service.send_direct_exec_audit_event(command, execution_id, g8e_context)
       reads case_id/investigation_id from g8e_context
```

No investigation fallback query. All identity flows through `G8eHttpContext`.

### Settings Contract — g8ed ↔ g8ee

g8ee reads platform configuration at startup via `Settings.from_db()`, which loads the `components/platform_settings` document created by g8es during initialization and managed by g8ed.

```
Admin UI → PUT /api/settings → g8ed SettingsService.saveSettings() → DB document
                                                                              ↓
                                              g8ee Settings.from_db() reads on startup
```

g8ed is the **write boundary**. g8ee is the **read boundary**. Invalid values must be rejected by g8ed before they reach the DB — g8ee must never need to fall back to a default due to a bad value that g8ed accepted.

#### Settings document structure

g8ed writes settings into two collections:
- `platform_settings`: Global configuration (e.g. LLM provider, URLs, cert paths). Document ID is `platform_settings`.
- `user_settings`: Individual user overrides for overridable keys (e.g. primary model, temperature). Document ID is the `userId`.

Values are stored as their native JSON types (strings, booleans, numbers) in the `settings` field of these documents. The `USER_SETTINGS` schema in `settings_model.js` declares the canonical type for each key — string options have string values, boolean toggles have boolean values. g8ee's `PlatformSettingsData` and `UserSettingsData` use Pydantic's `coerce_numbers_from_str=True` and a `_coerce_booleans` model validator to handle both native and legacy string representations at read time.

#### Precedence Resolution

Settings are resolved with the following priority (highest wins):
1. **User DB** (`user_settings` collection)
2. **Platform DB** (`platform_settings` collection)
3. **Environment Variable** (`G8E_*` prefix)
4. **Schema Default** (Defined in `SettingsService`)

#### Field alignment

Every g8ee `LLMSettings` field must have a corresponding g8ed `USER_SETTINGS` entry and be present in the `getConfig()` result, all sharing the same default value. `PLATFORM_SETTINGS` keys (cert paths, SSL dirs, session tuning) are deployment-time only and have no g8ee counterpart.

| DB key | g8ed `USER_SETTINGS` default | g8ee `LLMSettings` field | g8ee type | g8ee default |
|--------|-------------------------------|-------------------------|----------|-------------|
| `llm_provider` | `''` (empty string) | `provider` | `LLMProvider` | `OLLAMA` |
| `llm_model` | Provider-specific (OpenAI: GPT_5_4, Ollama: GEMMA3_1B, Gemini: FLASH_PREVIEW, Anthropic: CLAUDE_OPUS_4_5) | `primary_model` (alias: `llm_model`) | `str\|None` | `None` |
| `llm_assistant_model` | Provider-specific | `assistant_model` (alias: `llm_assistant_model`) | `str\|None` | `None` |
| `openai_endpoint` | Provider-specific | `openai_endpoint` | `str\|None` | `None` |
| `openai_api_key` | `''` | `openai_api_key` | `str\|None` | `None` |
| `ollama_endpoint` | Provider-specific | `ollama_endpoint` | `str\|None` | `None` |
| `ollama_api_key` | `''` | `ollama_api_key` | `str\|None` | `None` |
| `gemini_api_key` | `''` | `gemini_api_key` | `str\|None` | `None` |
| `anthropic_endpoint` | Provider-specific | `anthropic_endpoint` | `str\|None` | `None` |
| `anthropic_api_key` | `''` | `anthropic_api_key` | `str\|None` | `None` |
| `llm_temperature` | `''` (validated 0.0–2.0) | `llm_temperature` | `float\|None` | `None` |
| `llm_max_tokens` | `''` (validated 1–1000000) | `llm_max_tokens` | `int\|None` | `None` |
| `llm_command_gen_enabled` | `''` (boolean select) | `llm_command_gen_enabled` | `bool` | `True` |
| `llm_command_gen_verifier` | `''` (boolean select) | `llm_command_gen_verifier` | `bool` | `True` |
| `llm_command_gen_passes` | `''` (validated 1–10) | `llm_command_gen_passes` | `int` | `3` |
| `llm_command_gen_temp` | `''` (validated 0.0–2.0) | `llm_command_gen_temp` | `float\|None` | `None` |

#### Validation at the write boundary

The `USER_SETTINGS` schema declares a `validate` function on numeric entries. `SettingsService.saveSettings()` enforces these before writing — invalid values are never persisted.

#### g8ee read behaviour

g8ee loads settings from g8es on startup and during session initialization. It respects the same precedence logic when fetching from the `platform_settings` and `user_settings` collections.

#### Settings invariant

Defaults must be declared in exactly one place: the `LLMSettings` field definition. `from_db` must not re-declare any default. `USER_SETTINGS` schema and `env_vars.json` must agree with the `LLMSettings` field default for every key. Any drift between these is a bug.

---

## Component Code Quality

### g8eo (Go)

#### Constants

- Always use a named typed constant from `constants/` for any domain string field. Use typed constants directly — `constants.ExecutionStatusCompleted`, never the raw string `"completed"`.
- If a required constant does not exist in `constants/`, create it there before using it. Never define an ad-hoc string constant at the call site.
- Channel names are built using the builder functions in `constants/channels.go` — never hand-rolled `fmt.Sprintf` strings.
- `constants/events.go` and `constants/status.go` mirror `shared/constants/events.json` and `shared/constants/status.json` respectively. Values must match exactly.

| File | What belongs there |
|------|-------------------|
| `constants/events.go` | All pub/sub event type strings — mirrors `shared/constants/events.json` |
| `constants/status.go` | All status strings and typed constants — mirrors `shared/constants/status.json` |
| `constants/channels.go` | Pub/sub channel builder functions — mirrors `shared/constants/channels.json` |
| `constants/env_vars.go` | Environment variable name constants |
| `constants/headers.go` | Shared HTTP header name constants |
| `constants/exit_codes.go` | All process exit codes and `ExitCodeFromError` |
| `constants/network.go` | Default operator endpoint hostname constant |

**Enforcement:** `contracts/constants_enforcement_test.go` uses Go's AST parser to scan for raw string literals that match any enforced constant value. `contracts/shared_constants_test.go` asserts that every g8eo constant value exactly matches the corresponding shared JSON entry. `contracts/shared_wire_models_test.go` verifies every g8eo wire struct JSON tag matches `shared/models/wire/`.

#### Models

- Never use `map[string]interface{}` or `json.RawMessage` for a known structured payload — always define a typed struct with JSON tags.
- All inbound payload types live in `models/commands.go`. All outbound result types live in `models/wire.go`, `models/file_edit.go`, or `models/fs_list.go`.
- Internal structs use `*time.Time` pointer fields set to UTC. Wire models use RFC3339Nano strings.
- g8eo does not own any document models — it never writes to the g8es document store directly.

#### Services and Error Handling

- Never construct service structs directly — always use `New*` constructors.
- Validate payload fields immediately after unmarshal; return early on missing required fields.
- Default `sentinel_mode` to `constants.Status.VaultMode.Raw` when the field is absent.
- Wrap errors with `%w` at service boundaries. Log once at the handler boundary — never both log and return a wrapped error in the same layer.
- Always use the injected `*slog.Logger` with structured key/value pairs — never `log.Printf` or `fmt.Println`.
- Always use typed exit codes from `constants/exit_codes.go` — never hardcode exit integers.

#### Pub/Sub Services

g8eo's operator runtime is built around a pub/sub dispatch loop. All inbound commands and outbound results flow through the `services/pubsub/` package.

**Core types**

| File | Type | Responsibility |
|------|------|----------------|
| `g8es_pubsub_client.go` | `G8esPubSubClient` | WebSocket connection to g8es pub/sub. `Subscribe` blocks until the broker confirms the subscription is registered before returning the message channel — this prevents the race where a published message arrives before the subscriber is live. |
| `dispatch_service.go` | `DispatchService` | Top-level pub/sub loop. Subscribes to the operator channel, demultiplexes inbound messages by event type, and routes each to the appropriate handler service. |
| `command_service.go` | `CommandService` | Handles `OPERATOR_COMMAND_REQUESTED` and `OPERATOR_COMMAND_CANCEL_REQUESTED`. Runs the sentinel pre-execution guard, drives the execution goroutine with a periodic status ticker, then delegates all dual-vault writes to `VaultWriter`. |
| `file_ops_service.go` | `FileOpsService` | Handles file edit, fs list, and fs read requests. Runs the sentinel file-op guard for mutating operations, delegates dual-vault writes to `VaultWriter`. |
| `results_service.go` | `ResultsService` | Publishes execution results, file edit results, and heartbeat responses back to g8ee over the results pub/sub channel. |
| `vault_writer.go` | `VaultWriter` | Owns all dual-vault persistence. Raw vault receives unscrubbed data; scrubbed vault receives sentinel-processed data. Both writes are best-effort — failures are logged and never propagate to callers. |

**Injection pattern**

`PubSubCommandService` orchestrates all sub-services using a unified configuration pattern:

```go
svc, err := NewPubSubCommandService(CommandServiceConfig{
    Config:         cfg,
    Logger:         logger,
    Execution:      execSvc,
    FileEdit:       fileEditSvc,
    PubSubClient:   pubSubClient,
    ResultsService: resultsSvc,
    Sentinel:       sentinel,
    RawVault:       rawVault,
    LocalStore:     localStore,
    AuditVault:     auditVault,
    Ledger:         ledger,
    HistoryHandler: historyHandler,
})
```

A `nil` field in `CommandServiceConfig` is treated as disabled — the service proceeds without that capability. For example, if `Sentinel` is `nil`, no pre-execution analysis or output scrubbing is performed. `VaultWriter` handles `nil` storage services internally to safely skip optional writes.

**Loopback tests**

`services/pubsub/` has two loopback test files that run an in-process `PubSubBroker` via `httptest.NewServer` — no external g8es required:

| File | Coverage |
|------|----------|
| `g8es_pubsub_loopback_test.go` | Client subscribe/publish, ACK waiting, message delivery |
| `g8es_pubsub_dispatch_loopback_test.go` | End-to-end dispatch: command execution, cancellation, file ops, heartbeat |

#### Tests

See [testing.md — g8eo](testing.md#g8eo--go) for test infrastructure, `testutil/` helpers, mock locations, and how to run tests.

- Always use typed constants in test assertions — never raw strings; the contract test enforces this in application code and the same discipline applies in test code.
- Call `defer svc.Close()` for every service that holds a database connection.

---

### g8ee (Python/FastAPI)

#### Constants

- Always use a `str, Enum` from `app/constants/` for any constrained string field. Use enum members directly — `InvestigationStatus.OPEN`, never `InvestigationStatus.OPEN.value` (exception: `int, Enum` requires `.value` when a raw int is needed).
- If a required constant does not exist in `app/constants/`, create it there before using it.

#### Models

- Only files under `app/models/` may import directly from `pydantic`. All other files import `Field`, `ValidationError`, and `G8eBaseModel` exclusively from `app.models.base`.
- Never use `Dict[str, Any]` for a known structured shape — always define a typed `G8eBaseModel` subclass.
- Never use `Any` in service, handler, or router code for a parameter or return type that has a known shape — always use the concrete typed model.
- Service method signatures must use concrete typed models. Never accept raw dicts or untyped collections (`list[object]`, `dict`) at the application barrier — callers are responsible for constructing typed instances before calling service methods. No `isinstance` + `model_validate` fallbacks inside service methods.
- Never call `model_dump()` outside `app/models/base.py`. Use the named boundary methods:
  - `flatten_for_db()` — immediately before `create_document` / `update_document`
  - `flatten_for_wire()` — immediately before any outbound HTTP payload, pub/sub publish, or KV cache write
  - `flatten_for_llm()` — inside `Part.from_tool_response()` in `agent.py` only

All g8ee domain models inherit from one of four base classes in `app/models/base.py`:

| Class | Use for |
|-------|---------|
| `G8eBaseModel` | Value objects, request DTOs, config structs, response shapes |
| `G8eTimestampedModel` | Sub-documents that track mutation time but are not persisted as their own document |
| `G8eIdentifiableModel` | Persisted entities with a stable document identity |
| `G8eAuditableModel` | Identifiable entities that also need actor-level audit trails |

`id` is inherited from `G8eIdentifiableModel` — never redeclare it in a subclass.

#### `g8edEventData` — Correlation ID Contract

`g8edEventData` owns three correlation ID fields at the envelope level: `investigation_id`, `case_id`, and `web_session_id`. Payload models passed to `g8edEventData` (or to `EventService.publish_event_to_g8ed`) must not declare fields with any of these names.  Fix the payload model; never add a workaround at the call site.

#### Services

All core services are instantiated once at startup in `app/main.py` and stored on `app.state`. They are never constructed inside request handlers or other services. Services are accessed exclusively via the dependency functions in `app/dependencies.py`.

**Operator service layer:** `OperatorCommandService` is a pure injection target — it receives all sub-services via its constructor and contains no construction logic. All 10 focused sub-services are constructed and wired by `build_operator_command_service()` in `app/services/operator/factory.py`. Service contracts are defined as `Protocol` types in `app/services/protocols.py`. Never instantiate operator sub-services directly outside `factory.py` and test construction helpers.

| Service file | Class | Responsibility |
|--------------|-------|----------------|
| `pubsub_service.py` | `OperatorPubSubService` | g8es pub/sub lifecycle — publish, wait, session register/deregister |
| `approval_service.py` | `OperatorApprovalService` | Approval request/response/query surface |
| `execution_service.py` | `OperatorExecutionService` | Command dispatch, operator resolution, security constraints |
| `file_service.py` | `OperatorFileService` | File edit operations |
| `filesystem_service.py` | `OperatorFilesystemService` | Filesystem list/read operations |
| `audit_service.py` | `OperatorAuditService` | Fetch logs, session history, file history, file diffs, file restores |
| `intent_service.py` | `OperatorIntentService` | Intent grant/revoke via g8ed |
| `lfaa_service.py` | `OperatorLFAAService` | LFAA audit event publishing |
| `port_service.py` | `OperatorPortService` | Port check operations |

The circular dependency between `OperatorPubSubService` is resolved by the factory: `pubsub_service.set_result_handler(result_handler_service.handle)` is called after both are constructed.

**Pub/sub subscribe-and-wait rule:** Every channel subscription must confirm the broker ack before any publish occurs on that channel. `KVClient.subscribe()` enforces this — it sends the subscribe action, waits for the `{"type":"subscribed"}` ack from the broker (5-second timeout), and only then returns. `OperatorPubSubService.register_operator_session()` relies on this guarantee: it calls `subscribe()` before recording the session as active, so `publish_command()` is never reachable on an unconfirmed channel. Never publish to a channel before `subscribe()` has returned. Never introduce `asyncio.sleep` as a substitute for the ack — that is a race condition, not a fix.

**Pyright strict gate:** `pyrightconfig.services.json` enforces `typeCheckingMode: strict` on `app/services/`. Run via `./g8e test g8ee --pyright` before submitting changes to service code. Any type error in `app/services/` blocks the gate. Never use bare `Any` in a service method signature — if a type has a known shape, define a `Protocol` or `G8eBaseModel` for it.

#### HTTP Context — `G8eHttpContext`

All internal HTTP calls from g8ed to g8ee carry a `G8eHttpContext` (`app/models/http_context.py`) built from `X-G8E-*` request headers. It is the single object that propagates session identity, business context, and bound operator metadata across the cluster.

`G8eHttpContext` fields:

| Field | Required | Description |
|-------|----------|-------------|
| `web_session_id` | yes | Browser session — used to route SSE events to the correct tab |
| `user_id` | yes | Authenticated user — used for data ownership checks |
| `source_component` | yes | Component that issued the call (`g8ed`) |
| `case_id` | yes | Active case identifier (falls back to NEW_CASE_ID when new_case is true) |
| `investigation_id` | yes | Active investigation (AI chat session) identifier (falls back to NEW_CASE_ID when new_case is true) |
| `organization_id` | no | Multi-tenant isolation key |
| `task_id` | no | Active task identifier |
| `bound_operators` | no | All operators bound to the web session — serialised from `X-G8E-Bound-Operators` JSON header |
| `execution_id` | no | Per-request trace ID for logging correlation |
| `new_case` | no | True when g8ed signals that no prior case exists and g8ee must create one inline |
| `timestamp` | no | Timestamp of context creation |

**g8ee rules:**
- Always use the `get_g8e_http_context` or `get_g8e_http_context_for_chat` dependency from `app/dependencies.py` via `Depends()` to obtain `G8eHttpContext` in a router handler. Never parse `X-G8E-*` headers manually.
- The dependency enforces all required fields (`web_session_id`, `user_id`, `source_component`, `case_id`, `investigation_id`) and raises `AuthenticationError` if any are absent — do not add duplicate checks in handlers.
- Thread `g8e_context` through to every service method and down-call that needs it. Never reconstruct it or read request headers from inside a service.
- Use `g8e_context.bound_operators` to access operator context. The `parse_bound_operators` validator on the model handles deserialisation from the JSON header string.

#### Data Access — `CacheAsideService`

All reads and writes to the g8es document store must go through `CacheAsideService` (`app/services/cache/cache_aside.py`). Never call `DBClient` or `KVClient` directly from a service or handler for document operations.

**Pattern:** DB is authoritative for all writes. KV cache is the read path. Cache failure is never fatal to a write.

| Operation | Method | Behavior |
|-----------|--------|----------|
| Create | `create_document(collection, document_id, data)` | Writes to DB, then **invalidates** the cache key (next read re-populates from DB). Raises `DatabaseError` on DB failure. |
| Update | `update_document(collection, document_id, data, merge=True)` | Writes to DB, then **invalidates** the cache key (next read re-populates from DB). Raises `DatabaseError` on DB failure. |
| Delete | `delete_document(collection, document_id)` | Deletes from DB, then evicts cache key. Raises `DatabaseError` on DB failure. |
| Read | `get_document(collection, document_id)` | KV first; on miss, reads from DB and warms cache. Returns `None` if not found. |
| Bulk write | `batch_create_documents(operations)` | Batch DB write, then warms KV cache for each document. Raises `DatabaseError` on failure. |
| Query cache read | `get_query_result(collection, query_params, ttl=None)` | KV first on a hashed query key; returns cached list or `None` on miss. |
| Query cache write | `set_query_result(collection, query_params, results, ttl=None)` | Caches a list of plain dicts under a hashed query key. |
| Cache invalidation | `invalidate_document(collection, document_id)` | Evicts a single document's KV key. |
| Collection invalidation | `invalidate_collection(collection)` | Pattern-deletes all document KV keys for a collection. |
| Query invalidation | `invalidate_query_cache(collection)` | Pattern-deletes all query KV keys for a collection. |

**Rules:**
- Always pass data through `flatten_for_db()` before passing to `create_document` or `update_document`. The service accepts plain dicts — it does not call `flatten_for_db()` itself.
- `get_query_result` / `set_query_result` operate on lists of plain dicts. Flatten all model instances before passing to `set_query_result`.
- Never call `kv.set_json` / `kv.get_json` / `db_client.*` directly from outside `CacheAsideService` for document-level operations.

#### Error Handling

All exceptions must be `G8eError` subclasses from `app/errors.py`. Never raise `HTTPException`, `Exception`, `ValueError`, or `RuntimeError` directly.

| Class | HTTP | When to use |
|-------|------|-------------|
| `ResourceNotFoundError` | 404 | Document or entity not found |
| `DatabaseError` | 500 | DB write/read failure |
| `NetworkError` | 500 | HTTP or WebSocket call to external service failed |
| `ValidationError` | 400 | Invalid input |
| `BusinessLogicError` | 400 | Precondition not met |
| `AuthenticationError` | 401 | Missing or invalid credentials |
| `AuthorizationError` | 403 | Authenticated user lacks access |
| `ServiceUnavailableError` | 503 | Required service not initialised on `app.state` |
| `ResourceConflictError` | 409 | Conflict with an existing resource |
| `G8eTimeoutError` | 504 | Operation timed out |
| `ConfigurationError` | 500 | Invalid or missing configuration |
| `PubSubError` | 500 | Pub/sub connection or publish failure |
| `StorageError` | 500 | Storage layer connection or operation failure |
| `RateLimitError` | 429 | API rate limit exceeded |
| `ExternalServiceError` | 502 | Call to an external service failed |

#### Tests

See [testing.md — g8ee](testing.md#g8ee--python) for test infrastructure, pytest fixtures, mock locations, markers, and how to run tests.

- Always use enum members for domain values in test assertions — never string literals; this is the same rule that applies to application code.
- Always use typed model instances when calling service methods in tests — never raw dicts.
- Never import directly from `tests.conftest` — import factory functions and mock classes from `tests.fixtures.*`.

---

### g8ed (Node.js/Express)

#### Constants

- Use constants from `constants/` for every domain string — status values, event types, result codes, and all other string-valued domain values. If a constant does not exist, create it there first.
- Every constant object in `constants/` must be declared with `Object.freeze`.
- Wire-protocol values must be backed by `_STATUS`, `_EVENTS`, or `_MSG` from `constants/shared.js`, which loads and re-exports `shared/constants/*.json` at module level.

`constants/shared.js` re-exports:

| JSON file | Re-exported as | Used for |
|-----------|----------------|----------|
| `shared/constants/events.json` | `_EVENTS` | SSE event type strings, pub/sub event routing |
| `shared/constants/status.json` | `_STATUS` | All status enums |
| `shared/constants/senders.json` | `_MSG` | `EventType` sender paths, `StreamChunkType` |
| `shared/constants/collections.json` | `_COLLECTIONS` | g8es collection name strings |
| `shared/constants/kv_keys.json` | `_KV` | KV key prefix constants |
| `shared/constants/channels.json` | `_CHANNELS` | Pub/sub channel prefix constants |
| `shared/constants/pubsub.json` | `_PUBSUB` | Pub/sub wire protocol constants |
| `shared/constants/document_ids.json` | `_DOCUMENT_IDS` | Canonical document ID constants |
| `shared/constants/intents.json` | `_INTENTS` | Intent type strings |
| `shared/constants/prompts.json` | `_PROMPTS` | Prompt key constants |
| `shared/constants/timestamp.json` | `_TIMESTAMP` | Timestamp field name constants |
| `shared/constants/headers.json` | `_HEADERS` | Shared HTTP header name constants |

**Enforcement:** `test/contracts/constants-enforcement.test.js` fails if any value in `services/`, `routes/`, `middleware/`, `models/`, or `utils/` duplicates a value already defined in `constants/`.

All HTTP endpoint path strings live in `constants/api_paths.js` (server-side route definitions) and `public/js/constants/api-paths.js` (`ApiPaths` builders — frontend only) and `constants/http_client.js`. Never hardcode path strings in services, routes, or frontend components.

#### Frontend Constants (`public/js/constants/`)

All magic strings and numbers used in frontend JS must be defined in `public/js/constants/` — no inline literals permitted in frontend component code.

| File | Contains |
|------|----------|
| `api-paths.js` | `ApiPaths` builders — all g8ed endpoint path strings consumed by the frontend |
| `service-client-constants.js` | `ServiceName`, `ServiceUrl`, `RequestTimeout`, `RetryConfig`, `RequestPath`, `HttpMethod`, `ServiceClientEvent`, HTTP header name constants, rate-limit constants — all values used by `ServiceClient` |
| `events.js` | `EventType` (auto-generated flat frozen object from `shared/constants/events.json`), `StreamChunkType`, `MessageType`, `CitationLayout`, `TribunalOutcome`, `TribunalFallbackReason`, `ToolDisplayCategory`, `ThinkingActionType` — all SSE and AI event constants |
| `auth-constants.js` | `UserRole`, `OperatorSessionRole`, `AuthProvider`, `DeviceLinkStatus`, `IntentStatus` — authentication and authorisation enums |
| `operator-constants.js` | `OperatorStatus`, `OperatorType` — operator state enums |
| `investigation-constants.js` | `InvestigationStatus` — investigation lifecycle states |
| `sse-constants.js` | `SSEClientConfig` — SSE connection timing and reconnect configuration |
| `timestamp-constants.js` | `TimestampFormat` — display formatting constants for relative and absolute timestamps |
| `ui-constants.js` | `CssClass`, `WheelDelta` — UI layout and DOM class constants |
| `app-constants.js` | `AppPaths` — frontend route paths |

**Rule:** If a constant does not exist in `public/js/constants/`, create it there first. Never define ad-hoc string or number literals in component, utility, or template code.

#### Models

Use `G8eBaseModel` or `G8eIdentifiableModel` from `models/base.js`:

| Base class | Use for |
|---|---|
| `G8eBaseModel` | Transient value objects, SSE event payloads, request/response models, nested sub-objects |
| `G8eIdentifiableModel` | Every persistent document stored in g8es — adds `id`, `created_at`, `updated_at` |

- Declare `static fields` on every model subclass. `parse(raw)` is the factory for all inbound data (DB reads, wire payloads, request bodies) — it validates, coerces, and strips unknown fields. Use `new ClassName(fields)` only for already-typed data inside the application boundary.

**Important:** The `G8eBaseModel` constructor parameter is named `fields` (not `data`) to prevent the anti-pattern of generic data containers. Always construct models with explicit field names rather than wrapping data in a generic `data` field.
- Use `Object.freeze` for all constant objects. Use `now()`, `addSeconds()`, and `toISOString()` from `models/base.js` for all time operations.

#### Serialization

Call the named boundary method at every boundary crossing. Never call `_flatten` directly.

| Method | Boundary |
|--------|----------|
| `.forDB()` | Called internally by `CacheAsideService` and `G8esDocumentClient` when a `G8eBaseModel` instance is passed. Callers pass model instances directly — never call `.forDB()` at the call site. For plain-object patches (partial updates), pre-flatten any nested model values with `.forDB()` before including them in the patch object. |
| `.forWire()` | Outbound fetch to g8ee only. Never call at `publishEvent` call sites — `SSEService.publishEvent` requires a typed model instance and calls `.forWire()` internally. |
| `.forClient()` | `res.json()` to the browser — strips secrets (`api_key`, `operator_cert`). |
| `.forKV()` | Returns a plain object. Pass to `client.set_json(key, model.forKV(), ttl)` — the client serializes. For pub/sub: pass `.forKV()` as the `data` argument to `G8esPubSubClient.publish()` — the client owns serialization. |

`SSEService.publishEvent(sessionId, event)` requires a typed `G8eBaseModel` instance — it enforces this at runtime and calls `.forWire()` internally. Never pass a pre-serialized plain object.

Every value that crosses an inbound wire boundary (DB read, pub/sub message, HTTP request body) must be parsed through the model's `.parse(raw)` factory before use. All date fields must be declared `F.date` and will be coerced to `Date` objects at the boundary — never compare them to raw ISO strings.

**No type coercion fallbacks.** Never use `JSON.stringify`, `String()`, or any inline coercion as a fallback for an untyped value at a boundary. If a value is not the expected type, that is a model violation — fix the model or the caller. Fallbacks hide contract bugs and produce silent wire corruption.

**g8es pub/sub wire protocol.** The g8es broker (`g8e.operator --listen`) defines `PubSubMessage.Data` as `json.RawMessage` — the `data` field on the wire must be a raw JSON object, never a pre-serialized string. `G8esPubSubClient.publish(channel, data)` enforces this: it accepts a plain object and owns serialization. Callers pass `model.forKV()` directly — never `JSON.stringify(model.forKV())`. Passing a string produces double-encoded JSON that subscribers cannot unmarshal.

#### Operator Document Ownership

g8ee owns all operator document writes after initial operator authentication. g8ed writes only during the auth/lifecycle flow (create, activate, bind, stop). Heartbeat payloads from g8eo are forwarded by g8ed to pub/sub as a gateway — g8ed never writes to the operator document from a heartbeat. g8ee processes heartbeats, updates `latest_heartbeat_snapshot`, `last_heartbeat`, and all other operator activity fields, then sends the processed data back to g8ed via HTTP for SSE broadcast to the frontend only. g8ed does not persist any of that data.

#### Service Architecture and Dependency Injection

g8ed uses a factory-based dependency injection (DI) pattern for its services and routers. This ensures that services are decoupled, easier to test, and have clear lifecycle management.

#### 1. Service Factory Pattern

All services are defined as classes and instantiated via a centralized `initializeServices` function in `@g8ed/services/initialization.js`. Services are accessed via getter functions (e.g., `getCacheAsideService()`, `getUserService()`) rather than a service locator object.

#### 2. Router Factory Pattern

Routers are defined as factory functions that accept the initialized services they require.

```js
// @g8ed/routes/operator/operator_routes.js
export function createOperatorRouter({ settings, operatorDownloadService, downloadAuthService, authorizationMiddleware }) {
    const router = express.Router();
    
    router.get('/', requireInternalOrigin, async (req, res) => {
        const binaryStatus = await operatorDownloadService.getPlatformAvailability();
        // ... handler logic
    });
    
    return router;
}
```

#### 3. Application Bootstrapping

The main entry point (`server.js`) calls `initializeServices`, then builds a services object by calling the individual getter functions, and passes this to `createG8edApp()`.

```js
// server.js
this.settings = await initializeServices();
this.services = {
    organizationModel: getOrganizationModel(),
    cacheAsideService: getCacheAsideService(),
    userService: getUserService(),
    operatorService: getOperatorService(),
    // ... all other services
};
const app = createG8edApp({ services, /* other options */ });
```

#### 4. Testing with DI

When writing unit tests, you should instantiate the service class directly and manually inject mock dependencies.

```js
// @g8ed/test/unit/services/operator/operator-service.unit.test.js
const mocks = {
    operatorDataService: { /* mock methods */ },
    userService: { /* mock methods */ },
    // ... other dependencies
};
const service = new OperatorService(mocks);
```

Route handlers must be thin: **parse → validate → call service → respond**. This is a hard rule with no exceptions.

- No business logic, orchestration sequences, or multi-step side effects in handlers
- No direct access to infrastructure clients (`cacheAside`, `kvClient`, `dbClient`, KV or DB clients) from a handler — all data access goes through service methods
- No inline construction of complex objects that belong in a service
- `req.webSessionId`, `req.session`, and `req.userId` are set by `requireAuth` middleware — handlers use these directly and never re-validate the session
- All path strings are constants from `constants/api_paths.js` — never raw string literals

**Service ownership for handler delegation:**

| Handler concern | Service |
|---|---|
| Operator bind / unbind (all four variants) | `BindOperatorsService` via `getBindOperatorsService()` |
| Operator CRUD, stats, broadcasting | `OperatorDataService` via `getOperatorService()` |
| SSE connection initialization (operator list, LLM config, investigation list) | `SSEService.onConnectionEstablished()` — called fire-and-forget from the SSE route |
| Console health, KV scan, KV key lookup, collection query | `ConsoleMetricsService` via `getConsoleMetricsService()` |
| Session binding state | `BoundSessionsService` via `getBindingService()` |

#### Services

All service instances are accessed via getter functions from `services/initialization.js` — never instantiated directly in routes or handlers.

All backend HTTP calls from g8ed to g8ee go through `InternalHttpClient` from `services/clients/internal_http_client.js`. Never use raw `fetch()` for g8ee calls. All frontend HTTP calls go through `window.serviceClient` — never use raw `fetch()` in browser component code.

#### HTTP Context — `G8eHttpContext`

All g8ed→g8ee HTTP calls must carry a `G8eHttpContext` built at the route level and passed to `InternalHttpClient` as `options.g8eContext`. The client translates it into `X-G8E-*` headers via `buildG8eContextHeaders()` and enforces required fields — it will throw if `web_session_id` is missing.

Context is assembled explicitly at the route handler with named fields:

```js
const boundOperators = await getBindingService().resolveBoundOperators(req.webSessionId);
const g8eContext = {
    web_session_id: req.webSessionId,
    user_id:        req.userId,
    organization_id: req.session?.organization_id || null,
    bound_operators: boundOperators,
    case_id:        req.body.case_id || null,
    investigation_id: req.body.investigation_id || null,
    execution_id:     `req_${Date.now()}`,
};
```

`resolveBoundOperators(webSessionId)` on `BoundSessionsService` looks up all operator sessions bound to the web session, validates each reverse binding, and returns a `BoundOperatorContext[]`.

**g8ed rules:**
- Always construct `g8eContext` explicitly with named fields at the route level before any g8ee call. Never build it inside a service.
- Pass context to `InternalHttpClient.request()` as `options.g8eContext` — never set `X-G8E-*` headers manually.
- Include all known business context (`case_id`, `investigation_id`, `task_id`) when available. g8ee uses these fields for data ownership enforcement and event correlation downstream — omitting them will cause missing correlation IDs on SSE events.
- Never derive or reconstruct context from session fields inside a service or helper function. The route handler owns context construction.

#### Data Access — `CacheAsideService`

All reads and writes to the g8es document store must go through `CacheAsideService` (`services/cache/cache_aside_service.js`). Never call the g8es document client or KV client directly from a service or route for document operations.

**Pattern:** DB is authoritative for all writes. KV cache is the read path. Cache failure on write is non-fatal — logged as a warning; the DB write still succeeds.

| Operation | Method | Behavior |
|-----------|--------|----------|
| Create | `createDocument(collection, documentId, data)` | Writes to DB, then populates KV cache. Returns `{ success, documentId, cached }`. |
| Update | `updateDocument(collection, documentId, data, merge=true)` | Writes to DB; if DB returns the updated document, **populates** cache with it; if DB returns no data, **evicts** the cache key. Returns `{ success, documentId }`. |
| Delete | `deleteDocument(collection, documentId)` | Deletes from DB, then evicts cache key. Returns `{ success, notFound, documentId }`. |
| Read | `getDocument(collection, documentId)` | KV first; on miss, reads from DB and warms cache. Returns a plain object or `null`. |
| Query | `queryDocuments(collection, filters, limit)` | Query cache first; on miss, queries DB, populates cache, and returns results. Always use this instead of calling `db.queryDocuments` directly. |
| Query cache read | `getQueryResult(collection, queryParams)` | KV first on a hashed query key. Returns cached array or `null` on miss — caller is responsible for executing the query and calling `setQueryResult`. |
| Query cache write | `setQueryResult(collection, queryParams, results)` | Caches an array of plain objects under a hashed query key. `results` must be an array — throws otherwise. |
| Cache invalidation | `invalidateDocument(collection, documentId)` | Evicts a single document's KV key. |
| Collection invalidation | `invalidateCollection(collection)` | Pattern-deletes all document KV keys for a collection. |
| Query invalidation | `invalidateQueryCache(collection)` | Pattern-deletes all query KV keys for a collection. |
| Operator read | `getOperator(operatorId)` | Typed shortcut — returns a validated `OperatorDocument` or `null`. Does not re-cache `STOPPED` operators. |

**Rules:**
- Pass `G8eBaseModel` instances directly to `createDocument` — the service calls `.forDB()` internally. For plain-object patches passed to `updateDocument`, pre-flatten any nested model fields with `.forDB()` before including them in the patch object.
- Never call `.forDB()` on the data before passing to `createDocument` — the service handles serialization. Calling it twice produces double-serialized nested objects.
- `getQueryResult` returns `null` on a miss — always check for `null` and fall back to the DB query, then call `setQueryResult` with the result.
- Collection-specific TTLs are applied automatically based on `TTL_STRATEGIES`. Do not pass a custom `ttl` unless the collection has a non-standard retention requirement.
- Never call `kvClient.set_json` / `kvClient.get_json` / `dbClient.*` directly from outside `CacheAsideService` for document-level operations.

#### Tests

See [testing.md — g8ed](testing.md#g8ed--nodejs) for test infrastructure, fixture files, mock factories, helpers, contract tests, and how to run tests.

- Create test users only via `userModel.createUser()` (`userModel` is the `userService` alias returned by `getTestServices()`) — it writes g8es, KV cache, and org in one operation.
- Never use `setTimeout(resolve, N)` to wait for async behavior — use `vi.useFakeTimers()` + `vi.advanceTimersByTime()`.
- The `// @vitest-environment jsdom` directive is the correct way to activate jsdom — never mix it with `new JSDOM()` + manual `global.*` assignment.
- Use status enum constants in all assertions — never raw string literals.

---

## Project Structure

```
├── g8e             # Platform management CLI (./g8e <command>)
├── shared/
│   ├── constants/      # Canonical wire-protocol strings (events, status, channels, etc.)
│   └── models/         # Canonical document schemas (operator, user, conversation, wire payloads)
├── components/
│   ├── g8eo/            # Operator binary source (Go)
│   ├── g8ee/            # AI engine (Python/FastAPI)
│   │   └── app/
│   │       ├── db/             # SQLite coordination store
│   │       ├── llm/            # LLM provider abstraction (factory + providers)
│   │       ├── services/
│   │       │   ├── ai/         # Chat pipeline, agent, tools (AIToolService), grounding
│   │       │   ├── operator/   # OperatorCommandService + 10 focused sub-services
│   │       │   │   ├── factory.py          # build_operator_command_service()
│   │       │   │   ├── protocols.py        # Protocol types for all service boundaries
│   │       │   │   ├── command_service.py  # Pure injection target (coordinator)
│   │       │   │   └── pending_commands_store.py  # CacheAside-backed in-flight tracking
│   │       │   ├── cache/      # CacheAsideService
│   │       │   ├── data/       # DBService, investigation, memory
│   │       │   └── infra/      # EventService, InternalHttpClient  wrappers
│   │       ├── security/       # Sentinel scrubber, command validation
│   │       └── prompts_data/   # Modular prompt system
│   ├── g8ed/           # Web frontend (Node.js/Express)
│   │   ├── routes/         # API, Gateway Protocol, pub/sub proxy, page routes
│   │   ├── services/       # Auth, sessions, operators, g8es client
│   │   ├── middleware/     # Auth, CSP, rate limiting
│   │   ├── views/          # EJS templates
│   │   └── public/         # Static assets (JS, CSS)
│   ├── g8es/          # g8es Dockerfile (runs g8e.operator --listen)
├── scripts/            # Internal build, test, and security scripts (run via g8ep)
├── docs/               # Full documentation
└── docker-compose.yml  # Single deployment configuration
```

---

## Documentation

Full index at [index.md](index.md).

| Category | Location |
|----------|---------|
| **Components** | [components/](components/) — g8eo, g8ee, g8ed, g8es, g8e node (authoritative component reference) |
| **Architecture** | [architecture/ai_agents.md](architecture/ai_agents.md) — AI agent pipeline; [architecture/storage.md](architecture/storage.md) — storage and data flows; [architecture/security.md](architecture/security.md) — security architecture |
| **Testing** | [testing.md](testing.md) — shared principles, g8ep environment, component test guides, CI workflow |
| **Glossary** | [glossary.md](glossary.md) — all platform terminology |
