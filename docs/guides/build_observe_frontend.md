---
title: Generator-Neutral Builder Guide
parent: Guides
---

# Generator-Neutral Builder Guide

Last Updated: 2026-09-23
Version: v2.1.12

---

## Purpose

This guide explains the runtime capability requirements a generated observe frontend must satisfy. A generated observe frontend is a read-only browser dashboard that shows agent and run lifecycle projections, eval summaries, downloads, and a live SSE narrative. It is produced by a builder (Lovable, Notion, or other supported SPA builder) consuming the deterministic contract pack. A Notion page alone is not a runtime SPA; the builder must produce a deployable single-page application that runs in a top-level browser tab and connects directly to a local g8e Gateway.

For the full browser integration reference (WebAuthn, SSE, CORS, approvals, passkey management), see [Build a g8e-Compatible Frontend](./build_frontend.md). For the minimal Lovable setup, see [Connect a Lovable App](./lovable.md). This guide covers the owner-local observe frontend specifically. It is not the anonymous [Public Spectator](../architecture/public_spectator.md) mirror; see [Public Spectator Operations Guide](./public_spectator.md) for publication and tunnel operations.

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

The `dashboard/g8e-adapter/` package is the audited integration core. It is verified by 445 unit tests, including the contract-pack drift check, and ships with a minimal host, a reference frontend, and the checked-in public evaluation explorer in `evaluation-explorer/`. The evaluation explorer consumes the anonymous public adapter while retaining its own typed full-corpus presentation store. Builder-generated code imports from the adapter and calls its exported APIs.

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
| `observe.openapi.json` | Curated OpenAPI 3.0 for the 21 allowlisted browser operations. |
| `event-schemas.json` | The four dashboard event payloads plus sentinel events. |
| `models.ts` | Standalone TypeScript models and validators derived from protocol JSON. |
| `fixtures/` | 11 typed fixture scenarios for all honest view states. |
| `manifest.json` | Schema version and SHA-256 of every output. Detects drift. |

Re-running the generator against identical inputs produces byte-identical files.

## Runtime capability requirements

A generated observe SPA must satisfy these requirements. The `builder-prompt.md` encodes them as hard constraints.

### Transport and auth

- Preserve the adapter boundary. Generated code imports from the adapter and calls its exported APIs. Generated code never reimplements transport, auth, SSE parsing, or allowlist enforcement.
- Execute Gateway requests only in the top-level browser context. No server-side proxy, no service-worker relay, no iframe delegation.
- Use the absolute configured Gateway origin from `FrontendRuntimeConfig` for every request. Never hardcode an origin, never derive it from `window.location`, never allow a relative URL.
- Include credentials on every fetch and every EventSource. The adapter uses `credentials: 'include'` for fetch and `withCredentials: true` for EventSource. The Gateway session cookie is HttpOnly and Secure; its SameSite behavior depends on whether cross-origin origins are configured. See [Session Cookie Behavior](./build_frontend.md#session-cookie-behavior).
- Implement the exact WebAuthn and nested SSE contracts via the adapter modules. Do not hand-roll base64url conversion, attestation/assertion wire shapes, or envelope parsing.
- Call only allowlisted operations. The adapter's endpoint allowlist is the complete set of reachable Gateway routes. Do not add a generic arbitrary-path request helper. Do not call SSE push, producer, audit, blob, filesystem, pub/sub, MCP, A2A, approval, chat, tool, or eval-launch routes.

### Display requirements

Populate every section in the homepage matrix from typed stores only. Static sections remain static. Dynamic values come only from observe reads, normalized safe events, runtime health, or explicit unavailable state.

- Connection state: disconnected, connecting, connected, reconnecting, unauthenticated. Use non-color labels.
- Auth state: unauthenticated, authenticating, enrolling, authenticated, or error. Bootstrap availability is a separate boolean status from `GET /api/v1/auth/bootstrap/status`; it is not an auth state.
- Overview counters: agents running and tasks in queue, each with a freshness label (observed, stale, unavailable). Success rate only when a typed success_rate measurement exists.
- Agent roster: each agent's display name, role, status label, and freshness. Throughput only when a typed throughput measurement exists.
- Active run and recent runs: display name, run kind, status label, completed/total task counts. No fabricated task counts.
- Latest evals: run id, verification label, receipt count, metric count. Partial verification shown as `projection_validated`, never as `verified` until the complete verifier exists.
- Downloads: filename, media type, byte size, SHA-256, privacy classification, authenticated URL, and generated time. Filter the typed catalog to `public_safe`; restricted artifacts must not appear in the UI.
- Live narrative: bounded, virtualized live event rows from normalized safe events. Unknown events appear only as a bounded diagnostic row and cannot mutate projections or counters.

### Prohibited displays

Do not display throughput, CPU, RAM, VRAM, disk, parameter counts, quantization, artifact formats, file sizes, success rates, or verification claims unless the corresponding typed observed source exists. Resource and throughput cards remain unavailable until a real host telemetry collector exists. Remove dead controls and screenshot-only calls to action. "Watch Live" authenticates or focuses the stream; it never starts work.

### Accessibility and responsive behavior

- Keyboard navigation across all interactive controls with visible focus.
- Semantic landmarks and headings (header, main, nav, section).
- Non-color status labels for every state (text label plus color).
- Reduced motion support (respect `prefers-reduced-motion`).
- Bounded or virtualized live rows so a long event stream does not block the main thread.
- Screen-reader connection announcements when the SSE connection state changes.
- Mobile-first login and live-status layout.

### Design-preview mode

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

## See Also

- [Build a g8e-Compatible Frontend](./build_frontend.md) — Full browser integration reference including WebAuthn, SSE, approvals, and passkey management.
- [Connect a Lovable App](./lovable.md) — Minimal local Lovable setup with `gw connect`.
- [Architecture: SSE Streaming](../architecture/sse.md) — Gateway SSE push ingestion, persistence, replay, and consumer endpoints.
- [Architecture: Public Spectator Architecture and Threat Model](../architecture/public_spectator.md) — The separate anonymous public-mirror observation mode, outbound-only export, and threat model.
- [Contract Pack README](../../dashboard/g8e-adapter/contract-pack/README.md) — Deterministic generation and acceptance commands.
