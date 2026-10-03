# g8e Observe Frontend Builder Prompt

This prompt instructs a generator-neutral builder (Lovable, Notion, or other supported SPA builder) to produce a deployable observability frontend that wraps the audited g8e-adapter package. The generated SPA connects to a local g8e Gateway over HTTPS, authenticates with WebAuthn passkeys, reads only typed user-scoped observe data, and renders normalized nested SSE updates. The builder generates presentation code only; it does not rewrite adapter transport code.

## Hard constraints

1. Preserve the adapter boundary. The audited g8e-adapter package owns runtime config parsing, the endpoint allowlist, credentialed fetch, WebAuthn ceremonies, SSE normalization, snapshot reconciliation, typed stores, and the safe presentation registry. Generated code imports from the adapter and calls its exported APIs. Generated code never reimplements transport, auth, SSE parsing, or allowlist enforcement.
2. Execute Gateway requests only in the top-level browser context. No server-side proxy, no service-worker relay, no iframe delegation. The SPA runs at a single top-level origin.
3. Use the absolute configured Gateway origin from FrontendRuntimeConfig for every request. Never hardcode an origin, never derive it from window.location, never allow a relative URL.
4. Include credentials on every fetch and every EventSource. All requests use credentials: 'include'. The session cookie is cross-origin Secure HttpOnly.
5. Implement the exact WebAuthn and nested SSE contracts documented in this pack. Use the adapter's webauthn and sse modules; do not hand-roll base64url conversion, attestation/assertion wire shapes, or envelope parsing.
6. Call only allowlisted operations. The adapter's endpoint allowlist is the complete set of reachable Gateway routes. Do not add a generic arbitrary-path request helper. Do not call SSE push, producer, audit, blob, filesystem, pub/sub, MCP, A2A, approval, chat, tool, or eval-launch routes.
7. Isolate visibly labeled design-preview fixtures. When features.design_preview is enabled, fixtures are shown in a clearly labeled design-preview mode that never mixes with connected data. In production, design-preview is disabled unless runtime configuration deliberately enables it.
8. Show unavailable states instead of placeholder values. Every dynamic field has a typed source or an explicit unavailable/unsupported/stale/empty/loading/error state. Never fabricate a value when the typed source is absent.

## What the SPA must display

Populate every section in the homepage matrix from typed stores only. Static sections remain static. Dynamic values come only from observe reads, normalized safe events, runtime health, or explicit unavailable state.

- Connection state: disconnected, connecting, connected, reconnecting, unauthenticated. Use non-color labels.
- Auth state: unauthenticated, bootstrapping, enrolling, authenticating, authenticated, error.
- Overview counters: agents running and tasks in queue, each with a freshness label (observed, stale, unavailable). Success rate only when a typed success_rate measurement exists.
- Agent roster: each agent's display name, role, status label, and freshness. Throughput only when a typed throughput measurement exists.
- Active run and recent runs: display name, run kind, status label, completed/total task counts. No fabricated task counts.
- Latest evals: run id, verification label, receipt count, metric count. Partial verification shown as projection_validated, never as verified until the complete verifier exists.
- Downloads: filename, media type, byte size, SHA-256, privacy classification, authenticated URL. Only public_safe artifacts appear.
- Live narrative: bounded, virtualized live event rows from normalized safe events. Unknown events appear only as a bounded diagnostic row and cannot mutate projections or counters.

## What the SPA must NOT display

Do not display throughput, CPU, RAM, VRAM, disk, parameter counts, quantization, artifact formats, file sizes, success rates, or verification claims unless the corresponding typed observed source exists. Resource and throughput cards remain unavailable until a real host telemetry collector exists. Remove dead controls and screenshot-only calls to action. "Watch Live" authenticates or focuses the stream; it never starts work.

## Accessibility and responsive behavior

- Keyboard navigation across all interactive controls with visible focus.
- Semantic landmarks and headings (header, main, nav, section).
- Non-color status labels for every state (text label plus color).
- Reduced motion support (respect prefers-reduced-motion).
- Bounded or virtualized live rows so a long event stream does not block the main thread.
- Screen-reader connection announcements when the SSE connection state changes.
- Mobile-first login and live-status layout.

## Design-preview mode

Design-preview mode is explicit, visibly labeled, and disabled in production unless runtime configuration deliberately enables it. Fixtures never mix with connected data. A design-preview banner is shown whenever the mode is active.

## Acceptance

The generated SPA must pass the acceptance commands in README.md. The connected page contains no fixture leakage, fabricated values, dead controls, unsupported claims, or mutation surface. At least one supported builder can consume this pack and produce a deployable SPA that connects through the untouched audited adapter.
