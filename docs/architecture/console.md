---
doc_id: console
title: Console Architecture and Development Guide
audience: maintainers and coding agents
status: current
last_updated: 2026-10-03
version: v2.3.0
owners:
  - console/
  - internal/services/gateway/console/
  - internal/services/gateway/ensemble_browser_proxy_controller.go
  - internal/services/gateway/operator_browser.go
  - internal/tools/constgen/generate_console.go
related:
  - gateway.md
  - auth.md
  - sse.md
  - ensemble.md
  - ../guides/build_frontend.md
  - ../devs/devs.md
  - ../devs/docs.md
when_to_read: Understanding, developing, testing, or changing the g8e Console — the Gateway-embedded browser frontend for passkey authentication, approvals, Operator inventory and binding, per-role model selection, cases, investigations, and chat —, its trust boundaries, source layout, local dev, and embed workflows.
do_not_use_for:
  - WebAuthn cryptography and session internals (see auth.md)
  - SSE persistence and replay internals (see sse.md)
  - Third-party or generated observe frontends (see ../guides/build_observe_frontend.md)
---

# Console Architecture and Development Guide

## Purpose

The g8e Console is the platform's browser frontend. It is a React and TypeScript single-page application built from `console/` and embedded in the `g8e` binary; the Gateway serves it at `/console/` on its HTTPS port. There is no separate frontend service or container: the console shares the Gateway's origin, so the browser talks only to the Gateway, with no CORS configuration and a first-party `SameSite=Lax` session cookie.

This document describes the console's architecture, trust boundaries, source layout, local development workflows, tests, and the embed mechanism that ships the console inside the `g8e` binary.

The console covers what an owner needs to operate the platform from a browser:

- **Authentication**: first-owner passkey enrollment, passkey sign-in, and CLI-initiated passkey enrollment (`#enroll=1&token=…`).
- **Approvals**: L3 Notary approval of suspended transactions (`#approve=…`), CLI recovery approval (`#recovery=…`), and platform workload enrollment review (`#platform-enrollment=…`).
- **Operator inventory**: list with each Operator's role (`data`, `inference`, and the read-only witness roles `provenance` and `observer`, shown as distinct badges; the Gateway resolves `operator_role` on every listed Operator), deploy commands, bind and unbind to the browser's web session, and stop remote Operators. An enrolled, live Operator shows `Active`; once bound to a web session it shows `Bound`.
- **Cases, investigations, and chat**: a case groups investigations; each investigation is one chat session with the ensemble (g8ee), including streamed replies, governed command activity, and in-line approval of ensemble approval requests.
- **Account**: identity, passkey list, and passkey revocation.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Source Layout](#source-layout)
- [Trust Boundaries](#trust-boundaries)
- [Cases and Investigations](#cases-and-investigations)
- [Operator Binding](#operator-binding)
- [Model Selection](#model-selection)
- [API Reference](#api-reference)
- [Event Delivery](#event-delivery)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

| ID | Rule |
| --- | --- |
| INV-CONSOLE-01 | The console is served only by the Gateway at `/console/` from the embedded `internal/services/gateway/console/static` build. Its source is `console/`; the embed changes only through `make console-build console-embed`, and `make console-embed-check` fails when it is stale. |
| INV-CONSOLE-02 | The console calls only Gateway routes classified `RouteAuthNone`, `RouteAuthWebSession`, or `RouteAuthDual` in `internal/services/gateway/gateway_auth.go`. It never calls g8ee directly; chat, case, investigation, and approval-response calls go through the Gateway ensemble browser proxy. |
| INV-CONSOLE-03 | The console never sends user, web-session, or bound-Operator identity as routing input. The Gateway derives user and web session from the HttpOnly cookie, and the ensemble browser proxy stamps `context.user_id`, `context.web_session_id`, and `context.bound_operators` from its own registry, replacing any browser-supplied values. |
| INV-CONSOLE-04 | Every request uses `credentials: 'include'`; the SSE stream uses `EventSource` with `withCredentials: true` and carries only the non-secret `since_id` cursor. |
| INV-CONSOLE-05 | Secret-bearing URL fragments (enrollment and recovery tokens) are read once and cleared with `history.replaceState` before the first render. A one-time token or deep-linked approval is attempted at most once per page load. |
| INV-CONSOLE-06 | The Gateway serves the console with `Content-Security-Policy` limited to `'self'` (no inline script, no third-party origin, `frame-ancestors 'none'`). Model output renders as React nodes, never `innerHTML`. |
| INV-CONSOLE-07 | Event type strings come from `console/src/generated/events.ts`, generated by `make constants-generate` from `protocol/constants/events.json`. Unknown event types are ignored. |
| INV-CONSOLE-08 | Model provider credentials are write-only from the console. The ensemble reports a provider key only as `api_key_set`; the console sends a key only when the user types one and never persists it in the browser. |
| INV-CONSOLE-09 | The ensemble browser proxy signs every request it forwards to g8ee with the Gateway's Actuator Ed25519 key. The signature covers the method, upstream path and query, the SHA-256 of the forwarded body, the stamped user, web session, and a per-request nonce and issue time. g8ee derives proxy identity only from a request whose signature verifies, so no network position (a loopback bind, a Compose network, a shared volume) is part of the trust decision. A proxy with no signing key refuses to forward. |
| INV-CONSOLE-DEV-01 | Gateway route strings live only in `src/lib/paths.ts`; event type strings come only from `src/generated/events.ts` through `src/lib/events.ts`. |
| INV-CONSOLE-DEV-02 | Wire-protocol logic (WebAuthn encoding, SSE normalization and reconnect, the investigation timeline reducer, case grouping, fragment intents) stays in pure modules under `src/lib/` with unit tests. Components hold presentation and wiring only. |
| INV-CONSOLE-DEV-03 | A change to `console/` ships with a refreshed embed (`make console-build console-embed`) in the same commit; CI runs `make console-embed-check`. |
| INV-CONSOLE-DEV-04 | The console adds runtime dependencies only when the platform cannot reasonably do without them. Current runtime dependencies are `react` and `react-dom`. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Console source | `console/src/` | `make console-lint console-test` |
| Console package and scripts | `console/package.json` | `npm run --prefix console` |
| Unit and component tests | `console/src/**/*.test.ts(x)`, `console/tests/` | `make console-test` |
| Embed target and drift check | `Makefile` (`console-embed`, `console-embed-check`) | `make console-embed-check` |
| Event registry generator | `internal/tools/constgen/generate_console.go` | `make constants-check` |
| Embedded build and handler | `internal/services/gateway/console/` | `./g8e test unit --pkg ./internal/services/gateway/console` |
| Generated event registry | `console/src/generated/events.ts` | `make constants-check` |
| Ensemble browser proxy (identity and bound-Operator stamping) | `internal/services/gateway/ensemble_browser_proxy_controller.go` | `./g8e test unit --pkg ./internal/services/gateway --run 'TestInvestigationsQueryBody\|TestInjectBrowserContext\|TestBoundOperators'` |
| Browser Operator routes | `internal/services/gateway/operator_browser.go`, `operator_controller.go` | `./g8e test unit --pkg ./internal/services/gateway --run TestOperator` |
| API reference view | `console/src/features/api/`, `console/src/lib/openapi.ts`, `internal/services/gateway/gateway_http_router.go` (`handleSwaggerDoc`) | `make console-test`; `./g8e test unit --pkg ./internal/services/gateway --run TestHandleSwaggerDoc`; `./g8e test integration --pkg ./internal/services/gateway --run TestRouteAuthRegistry_SwaggerDocIsDualAuth` |
| Per-role model selection | `ensemble/app/services/infra/llm_role_settings.py`, `ensemble/app/llm/model_catalog.py`, `console/src/lib/inference.ts` | `ensemble/.venv/bin/python -m pytest tests/unit/services/infra/test_llm_role_settings.py tests/unit/llm/test_model_catalog.py`; `make console-test` |
| Investigation creation within a case | `ensemble/app/routers/internal_router.py` (`resource_creation.create_investigation`) | `ensemble/.venv/bin/python -m pytest tests/unit/routers/test_internal_router.py` |

## Source Layout

```text
console/
  src/
    main.tsx              Reads URL-fragment intents, mounts providers and App
    App.tsx               Signed-out vs signed-in shell, navigation, views
    lib/                  Pure modules: paths, api (credentialed fetch), webauthn,
                          sse (normalize + GatewayStream), timeline, cases,
                          fragment, events, inference (role form + wire body), openapi
                          (Swagger 2.0 reader for the API view), types
    state/                React providers: session, stream (one SSE connection),
                          operators, approvals, inference, toast
    features/             auth, cases (CasesView, Timeline, Composer),
                          operators (OperatorsView, DeployPanel), inference
                          (per-role model selection), approvals, api (API reference),
                          account
    components/           Markdown (React-node renderer: code, headings, lists, GFM tables, inline), shared UI pieces
    generated/events.ts   Generated by `make constants-generate`; do not edit
  tests/                  Component tests that drive App against a fake Gateway
```

## Trust Boundaries

The console is untrusted presentation. It holds no keys and has no authority of its own:

- **Authentication** ends at the Gateway. WebAuthn ceremonies run in the console page; the Gateway verifies them and sets the HttpOnly, Secure `g8e_web_session_cookie`. See [Build a g8e-Compatible Frontend](../guides/build_frontend.md#webauthn-flow-requirements) for the wire contract.
- **Ensemble reachability** is the Gateway's concern: the proxy forwards to `--ensemble-upstream-url` (default `http://127.0.0.1:8000`; `http://g8e-ensemble:8000` in the unified Compose stack). The console holds no ensemble address.
- **Authority to act** belongs to Operators and the governance pipeline. The ensemble proposes actions for the Operators bound to the web session; every mutation still passes L1–L5, and L3-gated actions wait for a passkey approval in the console.
- **Ensemble approvals** (command, file edit, intent, stream, and agent-continue requests) are answered with `POST /api/v1/operator/approval/respond`. They are distinct from L3 Notary approvals, which are passkey-signed and live under `/api/v1/approvals`.

## Cases and Investigations

A case is the unit of work; an investigation is one chat session within it, with its own conversation history and live events. g8ee has no case-list endpoint, so the console groups the caller's investigations (`GET /api/v1/investigations`, which the proxy rewrites to an investigation query scoped to the session user) by `case_id`.

All three start through `POST /api/v1/chat`:

| Intent | Body |
| --- | --- |
| New case | `{"message": …, "context": {}, "resource_creation": {"create_case": true}}` |
| New investigation in a case | `{"message": …, "context": {"case_id": …}, "resource_creation": {"create_investigation": true}}` |
| Continue an investigation | `{"message": …, "context": {"case_id": …, "investigation_id": …}}` |

The response returns `case_id` and `investigation_id` immediately; the reply streams over SSE. g8ee opens a new investigation only under a case the caller owns, and reports another user's case as not found. The selected case and investigation are kept in the URL query (`?case=…&investigation=…`) so a refresh restores them.

## Operator Binding

Binding attaches an Operator to the browser's web session (`POST /api/v1/operators/bind` / `unbind`). The Gateway registry records the binding on the Operator document (`bound_web_session_id`). When the console sends a chat, stop, or approval-response request, the ensemble browser proxy reads the registry and stamps the Operators bound to that web session into `context.bound_operators`. The ensemble acts only on those Operators. Binding an Operator already bound to another of the user's sessions moves it to this session.

## Model Selection

The Inference page manages one endpoint and API key per provider. A separate Model Roles section in the left navigation assigns a provider/model pair to Primary (reasoning and tool use), Assistant (supporting steps), and Lite (triage, titles, memory). Assistant and Lite may be left unset to inherit; resolution falls back from Lite to Assistant to Primary, as in [LLM Providers](../ensemble/llm-providers.md). The selectable providers and their connection requirements come from the ensemble (`POST /api/v1/settings/llm/get`), not from console code. Jev and the test-only fake provider are not offered. Every provider, `g8e` included, takes a model the user picks. The Inference Operator is a worker and never decides a role's model: the role editor offers the models its Ollama provider serves and saves the chosen one on the role.

Saving (`POST /api/v1/settings/llm`) writes provider connections and role selections as separate fields of the caller's `user_settings_{user_id}` document and invalidates its cache entry. API keys are write-only: responses expose only `api_key_set`, and an omitted key preserves the stored value while an empty key removes it. g8ee reads user settings on every chat request, so the next message uses the new selection with no reload. The composer shows the Primary model and links to its role editor, or warns when no model is selected.

`POST /api/v1/settings/llm/models` accepts only a provider and lists the models its saved connection serves (Ollama `/api/tags`, OpenAI-compatible and llama.cpp `/v1/models`, Anthropic `/v1/models`, Gemini `models`). For `g8e` governed inference, g8ee resolves the caller's sole active Inference Operator through the Gateway and requests its typed model inventory through governed Operator dispatch, and the response lists the models it serves, from which the user picks the role's model. A failed listing reports only the HTTP status or transport error, never the upstream body.

## API Reference

The API view (`?view=api`) is a browsable reference for the Gateway's HTTP API. It fetches `GET /swagger/doc.json` with the session cookie and renders the swag-generated OpenAPI 2.0 document natively: operations grouped by tag, a text and tag filter, per-operation parameters and responses, and request and response types that expand in place into their definitions. It adds no third-party code, because the console CSP allows only same-origin scripts; the standalone Swagger UI at `/swagger/` loads from a CDN and is not used here.

The spec is compiled into the Gateway (`internal/services/gateway/docs`, `docs.SwaggerJSON`), so the route works from a bare binary. `/swagger/doc.json` is classified `RouteAuthDual` (web session or mTLS), not public, because it enumerates the full route surface. The view lists only operations that carry Swagger annotations; a route without annotations does not appear. `make swagger-generate` refreshes the spec, and CI fails when the committed copy is stale.

## Event Delivery

The console opens one `EventSource` per signed-in session to `/api/v1/sse/stream?since_id=<cursor>`. The first connection requests live events only (`since_id=0`) because history is loaded over HTTP; reconnects resume from the highest durable event ID. The stream de-duplicates by ID, reconnects with exponential backoff (1 s doubling to 30 s, plus up to 500 ms of jitter), treats a source that does not open within 10 s as failed, and reconnects when the tab becomes visible.

Events the registry marks `persistence: ephemeral` (streamed model output, for example) have no durable row, so the Gateway sends them without an `id:` line. The browser's `lastEventId` still holds the previous frame's ID for such a frame, so the console resolves every ephemeral event type to ID 0 from the generated registry. An ID-0 event bypasses de-duplication and never advances the resume cursor; trusting `lastEventId` would drop it as already seen.

Events are routed by type and `data.investigation_id`: chat and tool events update the selected investigation's timeline. A turn ends (the composer leaves its Stop state) on `ai.llm.chat.iteration.text.completed`, `...iteration.failed`, or `...iteration.stopped`; `...iteration.completed` fires per tool round and does not end the turn. Stop posts `/api/v1/chat/stop`; when g8ee reports `was_active: false` no stopped event follows, so the console clears the stale busy state itself; `app.case.*` events refresh the case list; `operator.status.updated.*` events refresh the Operator inventory, including the `active` event the Gateway publishes when an Operator is newly enrolled or claims its slot, so a newly approved Operator appears without a reload. `platform.approvals.changed` (ephemeral, no payload beyond the changed list) refreshes the pending approvals — L3 suspensions and platform enrollment requests — and the console also re-lists each time the stream (re)opens, since an ephemeral event cannot be replayed. The console does not poll.

## Procedures

### Set up

```bash
cd console && npm ci
```

`make dev-node` also installs console dependencies.

### Develop against a running Gateway

The fastest loop is to rebuild and reload the Gateway-served console, which keeps the console same-origin with the Gateway:

```bash
make console-build console-embed && make build && ./g8e gw restart
```

To use the Vite dev server (`npm run dev`, `http://127.0.0.1:5174/console/`), the page and the Gateway are different origins, so the Gateway must allow the dev origin first: `./g8e gw connect http://localhost:5174`. Relative API paths then need a dev proxy or an absolute Gateway origin; keep such changes local.

### Test

```bash
make console-lint      # tsc --noEmit + ESLint
make console-test      # Vitest: lib unit tests and App component tests
./g8e test unit --pkg ./internal/services/gateway/console   # embed, headers, CSP
```

Component tests in `tests/app.test.tsx` stub `fetch` and `EventSource` with a fake Gateway keyed by method and path, record every request, and assert wire bodies (for example, that a new investigation posts `resource_creation.create_investigation`).

### Ship a change

1. `make ci` (or `make ci-console`, which typechecks, lints, tests, builds the console SPA, and refreshes the embed automatically). Alternatively, `make console-embed` will build and embed directly.
2. Commit `internal/services/gateway/console/static/` with the source change.
3. If you changed the Gateway handler's Swagger annotations, run `make swagger-generate`.

The `console-tests` CI job runs typecheck and lint, the Vitest suite, a fresh `npm run build` diffed against `internal/services/gateway/console/static`, and the `g8e-adapter` build with `npm run gen:contract-pack:check`. `make ci-console` (and `make ci`) runs lint, tests, and refreshes the embed locally after `make dev-check`; the `g8e-adapter` build and contract-pack check run only in CI.

### Verify a console change end to end

1. `make console-lint console-test` and `make constants-check`.
2. `make console-build console-embed`, then `./g8e test unit --pkg ./internal/services/gateway/console`.
3. `make build`, start the stack, and open `https://localhost:8443/console/`.
4. Sign in, bind an Operator, start a case, open a second investigation in it, approve an ensemble request, and confirm `GET /api/v1/sse/stream` stays connected in DevTools.

## Anti-patterns

- Editing files in `internal/services/gateway/console/static/` by hand.
- Adding a route string outside `src/lib/paths.ts` or an event string outside the generated registry.
- Putting protocol parsing inside components, where it cannot be unit-tested.
- Adding a UI framework, router, or state library for a feature the existing primitives cover.
- Calling g8ee, the mTLS WebSocket, or any `RouteAuthMTLS` route from the console.
- Sending `bound_operators`, `user_id`, or `web_session_id` from the browser and expecting them to be honored.
- Rendering model output with `innerHTML` or adding inline scripts, which the CSP rejects.
- Calling `/api/v1/auth/enrollment-token/validate` before token-gated registration; it consumes the token.

## Links out

- [Build a g8e-Compatible Frontend](../guides/build_frontend.md)
- [Gateway Architecture](gateway.md)
- [Authentication Architecture](auth.md)
- [SSE Architecture](sse.md)
- [Ensemble Architecture](ensemble.md)
- [Developer Guidelines](../devs/devs.md)
- [Documentation Rules](../devs/docs.md)
