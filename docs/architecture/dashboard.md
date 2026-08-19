---
title: Dashboard (g8ed)
parent: Architecture
---

# Dashboard (g8ed)

Last Updated: 2026-08-19
Version: v2.0.0

## What g8ed Is

g8ed is the g8e operator dashboard. It is a Node.js 22 / Express web application that provides the operator-facing UI for the g8e platform: a chat interface for driving agentic ensembles, an operator management panel for binding and monitoring governed operators, an audit view for inspecting signed receipts and governance events, and a settings console for platform configuration. g8ed is a first-party component of the g8e platform, reunited in-tree in v2.0.0, and lives at `dashboard/` in the repository root.

g8ed is the real product UI, distinct from `demos/frontend/` which is a minimal enrollment smoke test. See the [Demos README](../../demos/README.md) for the distinction.

## Role in the Platform

g8ed sits in the operator-facing tier of the platform. It is a consumer of the gateway surface:

- It authenticates operators through WebAuthn/FIDO2 passkeys, registered during enrollment and verified by the gateway's L3 Notary layer.
- It proxies operator management actions (bind, unbind, download, device-link) through internal routes that call the gateway over mTLS.
- It consumes the gateway's SSE event stream to surface live ensemble progress, governance approvals, and operator heartbeats in the browser.
- It serves the console SPA, chat view, audit view, and settings view as EJS-rendered pages with client-side JavaScript components.

The dashboard does not mutate hosts or submit governed intents directly. It is a control surface for the human operator: it lets the operator approve L3 transactions, monitor ensemble activity, and manage operator bindings. All mutations flow through the gateway and governed operator.

## Connection Model

g8ed connects to the gateway over HTTP from the browser (CORS-enabled) and over the internal Docker network from the server-side Express process. In the unified Docker stack (repo-root `docker-compose.yml`), the dashboard container probes the gateway's health endpoint at `http://g8edb:8080/api/v1/health` (the `g8edb` network alias) before starting. The gateway is configured with `--cors-origin`, `--passkey-rp-origin`, `--passkey-rp-id`, and `--passkey-rp-name` so it accepts browser cross-origin requests and WebAuthn passkey registrations from the dashboard origin.

The dashboard's `GATEWAY_HEALTH_URL` and `GATEWAY_HEALTH_PATH` environment variables point at the gateway's network alias and health path. The dashboard does not require direct access to the gateway's PKI volume; it authenticates through the browser via passkeys and through the server-side internal routes via the gateway's HTTP API.

## Build and Test

The dashboard ships with its own Dockerfile (`dashboard/Dockerfile`), rooted at the g8e repo root so it can `COPY dashboard/`. The Makefile provides:

- `make dashboard-test` — runs the dashboard vitest suite (`dashboard/test/`).
- `make build-dashboard` — builds the dashboard Docker image.

The dashboard test suite is co-located under `dashboard/test/` and is not moved under the repo-root `test/` directory, which remains Go-specific. The polyglot tests map onto the 3-tier test model as Tier 1 (unit, in-component) and Tier 3 (Docker E2E against the unified compose). See the [Unified Docker Stack guide](../guides/unified_stack.md) for bringing up the whole platform.

## Relationship to `demos/frontend/`

`demos/frontend/` and `dashboard/` are two different things and both ship in-tree:

- `demos/frontend/` is a minimal enrollment smoke test: a single-file nginx-served HTML app that exercises WebAuthn passkey enrollment and SSE event streaming against the gateway on an isolated demo network. It exists to prove the enrollment and CORS path end to end.
- `dashboard/` is the real product UI (g8ed): a Node.js 22 / Express app with EJS views, vitest tests, and its own `dashboard/Dockerfile`. It runs as the `dashboard` service in the unified platform compose.

Keep both. The demo proves a narrow protocol path; the dashboard is the operator-facing product.

## License

g8ed is licensed under the Business Source License 1.1 (BSL 1.1), matching the rest of the g8e platform. It converts to Apache License 2.0 on 2030-08-18. See `dashboard/LICENSE`.

## Related Documentation

- [Authentication & Authorization](./auth.md): The WebAuthn passkey enrollment and L3 notary flow g8ed integrates with.
- [SSE Streaming](./sse.md): The event stream g8ed consumes for live ensemble and operator activity.
- [Ensemble (g8ee)](./ensemble.md): The agentic ensemble whose events g8ed surfaces in the chat view.
- [Unified Docker Stack](../guides/unified_stack.md): Bringing up the whole platform including g8ed.
