---
doc_id: build_observe_frontend
title: Generator-Neutral Builder Guide
audience: SPA builders and frontend developers
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - docs/guides/build_observe_frontend.md
  - dashboard/g8e-adapter/
  - protocol/docs/reference/
related:
  - docs/guides/build_frontend.md
  - docs/guides/lovable.md
  - docs/guides/public_spectator.md
  - docs/guides/cloudflare_tunnel.md
  - docs/architecture/sse.md
  - docs/architecture/auth.md
when_to_read: Building a read-only observe frontend via SPA builder (Lovable, Notion, etc.) consuming the g8e-adapter contract pack, or understanding adapter runtime requirements and capability guarantees.
do_not_use_for:
  - Custom frontend development without builders (docs/guides/build_frontend.md)
  - Local Lovable quick setup (docs/guides/lovable.md)
  - Public Spectator deployments (docs/guides/public_spectator.md)
  - Gateway authentication details (docs/architecture/auth.md)
---

# Generator-Neutral Builder Guide

## Purpose

This guide explains the runtime capability requirements and integration guarantees that a generated observe frontend must satisfy. A generated observe frontend is a read-only browser dashboard that shows agent and run lifecycle projections, eval summaries, downloads, and a live SSE narrative. It is produced by a builder (Lovable, Notion, or other supported SPA builder) consuming the deterministic contract pack from `dashboard/g8e-adapter/contract-pack/`. The builder must produce a deployable single-page application that runs in a top-level browser tab and connects directly to a g8e Gateway with passkey authentication.

This guide covers the owner-local observe frontend specifically and the audited adapter boundary that builders must respect. It is not the anonymous [Public Spectator](../architecture/public_spectator.md) mirror; see [Public Spectator Operations Guide](./public_spectator.md) for publication and tunnel operations. For minimal Lovable setup, see [Connect a Lovable App](./lovable.md). For the full browser integration reference (WebAuthn, SSE, CORS, approvals, passkey management), see [Build a g8e-Compatible Frontend](./build_frontend.md).

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Prerequisites](#prerequisites-and-workflow)
- [Architecture](#architecture)
- [The audited adapter](#the-audited-adapter)
- [The contract pack](#the-contract-pack)
- [Invariants](#invariants)
- [Adapter capabilities](#the-audited-adapter-capabilities)
- [Display requirements](#display-requirements)
- [Prohibited displays](#prohibited-displays)
- [Accessibility](#accessibility-and-responsive-behavior)
- [Design-preview mode](#design-preview-mode)
- [Acceptance](#acceptance)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Prerequisites and workflow

The builder output is a browser SPA, not a static design or an iframe preview. Use Node.js 22 or newer and the package manager required by the generated project. The adapter contract pack is generated and validated from `dashboard/g8e-adapter/`; run its commands from that directory.

1. Consume `contract-pack/builder-prompt.md`, `runtime-config.schema.json`, `observe.openapi.json`, `event-schemas.json`, `models.ts`, and `fixtures/` as the builder inputs. Do not copy the adapter's transport implementation into generated presentation code.
2. Generate a top-level SPA that imports the built `g8e-adapter` package or its audited source modules according to the generated project's package configuration. Serve the SPA from the exact frontend origin that the Gateway allows.
3. Inject a JSON script tag with the id `g8e-runtime-config`. It must contain the validated runtime configuration, for example:

```html
<script type="application/json" id="g8e-runtime-config">
{
  "schema_version": "1.0.0",
  "gateway_base_url": "https://localhost:8443",
  "passkey_rp_id": "localhost",
  "passkey_rp_name": "g8e",
  "app_name": "g8e"
}
</script>
```

4. For a same-computer local frontend, connect its origin to the Gateway with `./g8e gw connect <frontend-origin>`, then open the built SPA in a top-level browser tab. The command configures the matching CORS and WebAuthn origin settings and verifies Gateway HTTPS and CORS; the browser still controls certificate trust, WebAuthn permission, and cookie policy.
5. In live mode, authenticate with the adapter's WebAuthn ceremony, load the typed observe bootstrap snapshot, then start `SseStream`. On `onReconcile`, reload the bootstrap snapshot because the snapshot is authoritative and SSE is incremental delivery. On `onUnauthenticated`, stop rendering authenticated data and require authentication again.
6. If EventSource is unavailable or unsuitable, start `SsePollingFallback` explicitly and stop it when the primary stream resumes. Polling returns retained event envelopes; it does not turn SSE into a mutation or authorization channel.

The configured Gateway origin must be HTTPS. The runtime-config validator permits HTTP only for loopback development origins (`localhost`, `127.0.0.1`, or `::1`); WebAuthn still requires a secure browser context, with the browser's localhost exception covering local development. For hosted or cross-origin deployments, configure the Gateway's exact frontend origin and account for browser third-party-cookie policy. See [Build a g8e-Compatible Frontend](./build_frontend.md#gateway-side-configuration) for CORS, passkey RP, and session-cookie behavior.

## Architecture

```
[Builder output (generated SPA)] → imports → [g8e-adapter (audited)]
        ↓                                          ↓
  presentation code only                    transport, auth, SSE, state
        ↓                                          ↓
        └──────────── top-level browser ────────────┘
                          ↓
              https://localhost:8443 (g8e Gateway)
```

The generated SPA wraps the audited `g8e-adapter` package. The adapter owns runtime config parsing, the endpoint allowlist, credentialed fetch, WebAuthn ceremonies, SSE normalization, reconciliation signals, typed stores, and the safe presentation registry. The host reloads the authoritative observe snapshot when those signals fire. The builder generates presentation code only; it does not rewrite adapter transport code.

## The audited adapter

The [g8e-adapter](../../dashboard/g8e-adapter/) package is the audited integration core. It is verified by 445 unit tests, including the contract-pack drift check, and ships with a minimal host, a reference frontend, and the checked-in public evaluation explorer in `evaluation-explorer/`. The evaluation explorer consumes the anonymous public adapter while retaining its own typed full-corpus presentation store. Builder-generated code imports from the adapter and calls its exported APIs.

The adapter exposes:

- `parseRuntimeConfig` — validates `FrontendRuntimeConfig` from a JSON script tag. The accepted fields are `schema_version`, `gateway_base_url`, `passkey_rp_id`, `passkey_rp_name`, `app_name`, optional `documentation_base_url`, and optional `features` (`design_preview`, `eval_publication`, and `downloads`). The current schema version is `1.0.0`.
- `createCredentialedFetch` — builds a fetch helper that enforces the endpoint allowlist, sets `credentials: 'include'`, and constructs absolute URLs from the configured origin. It parses JSON response bodies but does not perform authentication or retry policy on its own.
- `createObserveClient` — produces a typed observe API client with 7 methods (bootstrap, paginated runs, run detail, paginated evals, eval detail, paginated downloads, and download detail). List methods accept opaque `cursor` and numeric `limit` options.
- `registerPasskey`, `authenticatePasskey`, `enrollPasskey`, and `stripUrlFragment` — WebAuthn ceremonies and enrollment-token URL cleanup matching the embedded console contract. The host must provide the user or CLI enrollment inputs and wire the returned results into its auth store.
- `SseStream` — opens an absolute configured `/api/v1/sse/stream` EventSource with `withCredentials: true`, tracks durable event IDs, reconnects with exponential backoff, reports unauthenticated closure, and emits reconciliation reasons. The host must reload the observe bootstrap snapshot when reconciliation is requested.
- `SsePollingFallback` — independently polls `GET /api/v1/sse/events?since_id=<id>&limit=<n>` with credentials. It is an opt-in fallback; `SseStream` does not start it automatically.
- `normalizeGatewayEvent` — parses the outer push envelope and nested string event, validates recognized payloads, preserves unknown events as unrecognized, and prevents routing IDs from becoming application payload.
- `adapterReducer` — five separate typed stores (auth, projections, narrative, transport, and runtime features) with pure reducers. The narrative store is bounded to 200 rows by default.
- Presentation registry — escaped bounded fields, safe labels, view states for loading/stale/unavailable/unsupported/partial-verification conditions, and diagnostic rows for unknown events. It does not render raw event payloads or chain-of-thought.

## The contract pack

The `dashboard/g8e-adapter/contract-pack/` directory contains deterministic, generator-neutral inputs. Regenerate with `npm run gen:contract-pack`; verify with `npm run gen:contract-pack:check`.

| File | Purpose |
| --- | --- |
| `builder-prompt.md` | The prompt to give a builder. Encodes every hard constraint. |
| `runtime-config.schema.json` | JSON Schema for `FrontendRuntimeConfig`. |
| `observe.openapi.json` | Curated OpenAPI 3.0 for the 20 allowlisted browser operations. |
| `event-schemas.json` | The four dashboard event payloads plus two sentinel event types. |
| `models.ts` | Standalone TypeScript models and validators derived from protocol JSON. |
| `fixtures/` | 11 typed fixture scenarios for all honest view states. |
| `manifest.json` | Schema version and SHA-256 of every output. Detects drift. |

Re-running the generator against identical inputs produces byte-identical files.

## Invariants

| ID | Rule |
| --- | --- |
| INV-OBSERVE-BUILDER-01 | The adapter boundary is inviolate. Generated code MUST import from the audited adapter and call only its exported APIs (`parseRuntimeConfig`, `createCredentialedFetch`, `createObserveClient`, WebAuthn functions, `SseStream`, `SsePollingFallback`, `adapterReducer`, `normalizeGatewayEvent`, presentation registry). Generated code MUST NOT reimplement transport, auth, SSE parsing, or allowlist enforcement. |
| INV-OBSERVE-BUILDER-02 | All Gateway requests MUST execute in the top-level browser context. Server-side proxies, service-worker relays, and iframe delegation are prohibited. The browser authenticates; the SPA is the sole entry point. |
| INV-OBSERVE-BUILDER-03 | The configured Gateway origin from `FrontendRuntimeConfig` MUST be used for every request. Hardcoded origins, origins derived from `window.location`, and relative URLs are prohibited. |
| INV-OBSERVE-BUILDER-04 | Credentials MUST be included on every fetch (`credentials: 'include'`) and every EventSource (`withCredentials: true`). The Gateway session cookie is HttpOnly and Secure; SameSite behavior depends on configured cross-origin settings. |
| INV-OBSERVE-BUILDER-05 | Only allowlisted endpoints in the adapter are reachable. Generic arbitrary-path request helpers are prohibited. Routes for SSE push, producer, audit, blob, filesystem, pub/sub, MCP, A2A, approval, chat, tool, or eval-launch operations MUST NOT be called. |

## Adapter capabilities

### Transport and auth

- Preserve the adapter boundary. Generated code imports from the adapter and calls its exported APIs. Generated code never reimplements transport, auth, SSE parsing, or allowlist enforcement.
- Execute Gateway requests only in the top-level browser context. No server-side proxy, no service-worker relay, no iframe delegation.
- Use the absolute configured Gateway origin from `FrontendRuntimeConfig` for every request. Never hardcode an origin, never derive it from `window.location`, never allow a relative URL.
- Include credentials on every fetch and every EventSource. The adapter uses `credentials: 'include'` for fetch and `withCredentials: true` for EventSource. The Gateway session cookie is HttpOnly and Secure; its SameSite behavior depends on whether cross-origin origins are configured. See [Session Cookie Behavior](./build_frontend.md#session-cookie-behavior).
- Implement the exact WebAuthn and nested SSE contracts via the adapter modules. Do not hand-roll base64url conversion, attestation/assertion wire shapes, or envelope parsing.
- Call only allowlisted operations. The adapter's endpoint allowlist is the complete set of reachable Gateway routes. Do not add a generic arbitrary-path request helper. Do not call SSE push, producer, audit, blob, filesystem, pub/sub, MCP, A2A, approval, chat, tool, or eval-launch routes.

## Display requirements

Populate every section in the homepage matrix from typed stores only. Static sections remain static. Dynamic values come only from observe reads, normalized safe events, runtime health, or explicit unavailable state.

- Connection state: disconnected, connecting, connected, reconnecting, unauthenticated. Use non-color labels.
- Auth state: unauthenticated, authenticating, enrolling, authenticated, or error. Bootstrap availability is a separate boolean status from `GET /api/v1/auth/bootstrap/status`; it is not an auth state.
- Overview counters: agents running and tasks in queue, each with a freshness label (observed, stale, unavailable). Success rate only when a typed success_rate measurement exists.
- Agent roster: each agent's display name, role, status label, and freshness. Throughput only when a typed throughput measurement exists.
- Active run and recent runs: display name, run kind, status label, completed/total task counts. No fabricated task counts.
- Latest evals: run id, verification label, receipt count, metric count. Partial verification shown as `projection_validated`, never as `verified` until the complete verifier exists.
- Downloads: filename, media type, byte size, SHA-256, privacy classification, authenticated URL, and generated time. Filter the typed catalog to `public_safe`; restricted artifacts must not appear in the UI.
- Live narrative: bounded, virtualized live event rows from normalized safe events. Unknown events appear only as a bounded diagnostic row and cannot mutate projections or counters.

## Prohibited displays

Do not display throughput, CPU, RAM, VRAM, disk, parameter counts, quantization, artifact formats, file sizes, success rates, or verification claims unless the corresponding typed observed source exists. Resource and throughput cards remain unavailable until a real host telemetry collector exists. Remove dead controls and screenshot-only calls to action. "Watch Live" authenticates or focuses the stream; it never starts work.

## Accessibility and responsive behavior

- Keyboard navigation across all interactive controls with visible focus.
- Semantic landmarks and headings (header, main, nav, section).
- Non-color status labels for every state (text label plus color).
- Reduced motion support (respect `prefers-reduced-motion`).
- Bounded or virtualized live rows so a long event stream does not block the main thread.
- Screen-reader connection announcements when the SSE connection state changes.
- Mobile-first login and live-status layout.

## Design-preview mode

Design-preview mode is explicit, visibly labeled, and disabled in production unless runtime configuration deliberately enables it. Fixtures never mix with connected data. A design-preview banner is shown whenever the mode is active.

## Acceptance

The generated SPA must pass the acceptance commands in the contract pack README. Run from `dashboard/g8e-adapter/`:

```bash
npm run gen:contract-pack:check   # fail if committed outputs are stale
npm test                          # adapter + contract pack tests
npm run lint                      # type-check adapter and host
npm run build                     # build the audited adapter
npm run build:host                # build the minimal host
```

The connected page must contain no fixture leakage, fabricated values, dead controls, unsupported claims, or mutation surface. At least one supported builder can consume this pack and produce a deployable SPA that connects through the untouched audited adapter.

Real-browser acceptance (exact-origin CORS, WebAuthn authenticator, SSE credentials, two-user isolation, cross-platform trust) is an owner-operated gate documented in the release plan. It is not satisfied by unit tests alone.

## Anti-patterns

- Reimplementing transport, auth, or allowlist logic instead of using the adapter's exported APIs.
- Calling endpoints not in the adapter's allowlist or bypassing it with a generic fetch wrapper.
- Deriving the Gateway origin from `window.location` or hardcoding it instead of reading `FrontendRuntimeConfig`.
- Omitting credentials from EventSource or fetch; the adapter enforces inclusion for session authentication.
- Mixing fixture data with connected data, or displaying fixtures when design-preview mode is disabled.
- Rendering unknown SSE events as structured data instead of bounded diagnostic rows.
- Displaying unsourced metrics (success rate, throughput, resource usage) without typed observed data.
- Adding dead controls or removing the requirement for explicit user auth and action.

## Links out

- [Build a g8e-Compatible Frontend](./build_frontend.md) — Full browser integration reference including WebAuthn, SSE, approvals, and passkey management.
- [Connect a Lovable App](./lovable.md) — Minimal local Lovable setup with `gw connect`.
- [Architecture: SSE Streaming](../architecture/sse.md) — Gateway SSE push ingestion, persistence, replay, and consumer endpoints.
- [Architecture: Public Spectator Architecture and Threat Model](../architecture/public_spectator.md) — The separate anonymous public-mirror observation mode, outbound-only export, and threat model.
- [Contract Pack README](../../dashboard/g8e-adapter/contract-pack/README.md) — Deterministic generation and acceptance commands.
