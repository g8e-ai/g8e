# g8e v4.2.0 — Platform Hardening & Architecture Cleanup

A major stabilization release following the v4.0 platform rebuild. v4.2.0 delivers a sweeping internal refactor that eliminates architectural debt, fixes critical production bugs across the chat pipeline and operator execution path, and introduces a redesigned binary distribution system. All platform tests pass across every component.

## Major Changes

### Architecture: EventSource Elimination

Removed the `EventSource` abstraction entirely from the platform. All event routing now uses the `EventType` constants directly, which encode their source in the naming schema (e.g., `EventType.SOURCE_AI`, `EventType.SOURCE_USER`). This eliminated an entire class of bugs where the browser-native `EventSource` API was being confused with the internal `EventSource` constants object.

- Unified event handling across VSOD frontend and backend
- Replaced sender-path aliases with proper shared event type constants
- Renamed `message_types.json` to `senders.json` for clarity
- Fixed broken operator command history restoration in the dashboard

### Operator Binary Distribution via Blob Store

Redesigned how operator binaries are distributed across the platform. Binaries are now cross-compiled for all three architectures (amd64, arm64, 386) with UPX compression at VSODB image build time and stored in the VSODB blob store.

- g8e-pod fetches the operator binary from the blob store on startup via `fetch-key-and-run.sh`
- Retry logic with exponential backoff for transient VSODB unavailability
- Architecture auto-detection for correct binary selection
- Idempotent blob store upload on every VSODB container start

### Dual LLM Model Selection

Split the single "Balanced" LLM model dropdown into two distinct dropdowns in the operator terminal: **Primary** (complex tasks) and **Assistant** (simple tasks). The chat pipeline now routes model selection based on triage complexity.

### Platform Setup Command

New `./g8e platform setup` command for first-time setup. Performs a full no-cache build, starts VSODB first (waits for health), then starts all other components in the correct order.

### Operator Version Injection

The operator binary now reports the correct platform version instead of `dev`. Version is injected via Go ldflags (`-X main.version`) at build time across all build paths:

- **VSODB Dockerfile** -- receives `VERSION` build arg from `docker-compose.yml` (set via `G8E_VERSION` env var, read from `VERSION` file by `build.sh`)
- **Makefile `build` / `build-all`** -- reads `VERSION` file directly (already worked)
- **Makefile `build-local` / `build-local-all`** -- now includes version in ldflags (previously missing)

## Bug Fixes

- **Chat pipeline** -- Restored operator offline workflow; chat is fully functional end-to-end
- **SSE connection manager** -- Fixed `TypeError: EventType is not a constructor` caused by EventSource refactor mangling the browser API
- **Operator execution** -- Fixed execution path failures preventing operator commands from completing
- **KV endpoint auth** -- Fixed internal authentication on KV store endpoints
- **Settings page** -- Fixed 500 error on setup page (missing views path) and settings loading failures
- **Investigation queries** -- Fixed `select_fields` query construction for investigation lookups
- **Text completion** -- Fixed frontend handling of completed text events
- **VSOD/VSODB client** -- Fixed client communication and alignment issues
- **Initialization handlers** -- Fixed service initialization ordering bugs
- **g8e-pod CA certificate** -- Fixed chicken-and-egg TLS problem; operator now discovers CA cert from local mount instead of network fetch
- **Logger** -- Fixed Date objects rendering as `{}` in log output; `redactPii` now skips non-plain objects
- **Duplicate API key issuance** -- Eliminated double key issuance during operator slot initialization
- **Redundant KV lookup** -- Removed dead per-event operator resolution from SSE route

## Code Quality

- Eliminated unnecessary abstractions and dead code across VSOD
- Removed legacy `message_type` field from VSE conversation models
- Removed environment-specific test configuration (single environment platform)
- Cached settings reads in `G8ENodeOperatorService` to eliminate redundant DB reads per launch cycle
- Implemented proper `HttpService` with protocol-driven design
- Strict error typing and `CacheAsideProtocol` for consistent caching patterns
- Improved payload typing for execution results and command payloads

## Testing

- VSOD test suite restructured and expanded
- VSE integration test suite expanded (SSE error paths, retry loop coverage)
- VSA test fixes and listen mode improvements
- Full documentation audit with corrections across security, architecture, and component docs

## Component Summary

| Component | Changes |
|-----------|---------|
| **VSE** | 304 files, dual model selection, EventType cleanup, integration tests |
| **VSOD** | 170 files, EventSource elimination, SSE fixes, test restructuring |
| **VSA** | 31 files, test fixes, CA cert path discovery |
| **VSODB** | Multi-arch cross-compile, blob store binary upload on startup |
| **g8e-pod** | Binary fetch from blob store, retry logic, CA cert fix |

## Quick Start

```bash
git clone https://github.com/g8e-ai/g8e-ai/g8e.git && cd g8e
./g8e platform setup

# Then open https://localhost -- the setup wizard guides you through configuration
```

## Security & Privacy

v4.2.0 continues the local-first, human-in-the-loop security model. This release hardens the platform internals:

- **Operator binary integrity** -- Binaries are compiled and compressed at image build time, distributed through authenticated blob store endpoints
- **CA certificate bootstrap** -- Eliminated network-dependent CA fetch; operator discovers certificates from local volume mounts
- **Internal auth** -- Fixed KV endpoint authentication; removed redundant per-event operator resolution from SSE path
- **Logging safety** -- Logger no longer leaks object internals through Date serialization bugs

---

**g8e** -- AI-powered, human-driven infrastructure operations. Fully self-hosted. Air-gap capable. Security and privacy by design.

[Website](https://lateraluslabs.com) | [Docs](../index.md) | [License](../../LICENSE)
