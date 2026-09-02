---
title: Ensemble (g8ee)
parent: Architecture
---

# Ensemble (g8ee)

Last Updated: 2026-09-02
Version: v2.1.3

## What g8ee Is

g8ee is the first g8e-compatible agentic ensemble. It is a Python 3.12 / FastAPI service that connects to a g8e gateway over mTLS, submits governed mutations by posting `GovernanceEnvelope` transactions directly to the gateway's governance endpoint, and streams progress and results back to operators through the SSE event bridge. g8ee is a first-party component of the g8e platform, reunited in-tree in v2.0.0, and lives at `ensemble/` in the repository root.

g8ee is a reference ensemble, not a required one. The gateway treats every AI client as an untrusted principal, so any g8e-compatible client (an MCP client, a custom A2A server, or a third-party ensemble) can drive the platform. g8ee exists to provide a complete, supported, in-tree example of an agentic ensemble that exercises the full governance pipeline end to end, and to serve as the ensemble that ships with the unified Docker stack.

## Role in the Platform

g8ee sits in the AI client tier of the platform. It is a consumer of the gateway surface, not part of the gateway or operator:

- It authenticates to the gateway with an app workload identity (mTLS, enrolled via the owner-approved platform enrollment protocol on the gateway's plain-HTTP discovery surface).
- It submits governed mutations by building `GovernanceEnvelope` transactions itself (transaction hash, nonce, L3 proof) and posting them directly to `POST /api/v1/governance/envelopes` via its `GovernanceClient` (`ensemble/app/clients/governance_client.py`). The gateway then routes the envelope through the five-layer governance pipeline. The ensemble does not use the MCP endpoint; MCP is the surface for external MCP clients (Claude Code, Codex, Goose, Gemini CLI). Because the gateway's `PrivilegedRouteRegistry` blocks app certificates from the governance endpoint with `ErrPrivilegedEndpointAccess` ("external apps cannot access privileged endpoints"), the ensemble presents the operator's mTLS cert (whose SPIFFE URI carries the operator session ID) on the governance transport, not its own app cert. See [Connection Model](#connection-model) for how the operator cert is made available to the ensemble.
- It publishes user-facing events (progress, questions, results) and typed reputation updates to the gateway through `POST /api/v1/sse/push`, which the gateway streams to browser and CLI clients.
- Each model provider records monotonic timing, token usage when supplied by the provider, retry and finish metadata, canonical hashes of the model-boundary input and output, and a hash-bound privacy attestation containing only scanner identity, sensitive-occurrence counts, and detected types. The standalone eval package normalizes this telemetry with governance receipt evidence without storing raw sensitive values in analytical records.
- It never touches a target host directly. All host mutations flow through the governed operator, which re-verifies every proof locally before execution.

See [AI Agents and the g8e Governance Boundary](./agents.md) for the client surface contract and [SSE Streaming](./sse.md) for the event bridge g8ee publishes through.

## Connection Model

g8ee connects to the gateway over mTLS using an app workload identity. In the unified Docker stack (repo-root `docker-compose.yml`), the ensemble container has its own `g8e-ensemble-data` volume mounted at `/root/.g8e` (read-write). The ensemble enrolls at startup via the owner-approved platform enrollment protocol: it submits a platform enrollment request (with a P-256 CSR and system fingerprint) to the gateway's plain-HTTP discovery surface, waits for the owner to approve the request by exact request ID, signs the canonical completion transcript, and receives its own signed app cert, cert chain, trust bundle, app_id, and expiry. It stores them in its own runtime tree (`pki/issued/apps/g8ee.crt`, `pki/issued/apps/g8ee.key`, `pki/trust/hub-bundle.pem`). No gateway volume is mounted — the ensemble is a proper enrolled app, not a volume-mount shortcut for gateway state. The ensemble's `G8E_GATEWAY_HTTP_URL` (plain-HTTP discovery surface), `G8E_GATEWAY_URL` (HTTPS governance surface), `G8E_OPERATOR_URL`, `G8E_OPERATOR_PUBSUB_URL`, and `G8E_RUNTIME_DIR` environment variables point at the gateway's network alias and the ensemble's own runtime directory.

For governance envelope submission, the ensemble additionally mounts the operator's enrolled mTLS credentials read-only at `/operator-state` (`g8e-operator-data:/operator-state:ro`) and reads them via the `G8E_GOVERNANCE_OPERATOR_CERT` (`/operator-state/pki/operator.crt`) and `G8E_GOVERNANCE_OPERATOR_KEY` (`/operator-state/pki/operator.key`) environment variables. The `GovernanceClient` (`ensemble/app/clients/governance_client.py`) builds a separate `TLSConfig` from the operator cert/key for the governance transport, while all other gateway traffic (settings, SSE push, health) uses the ensemble's own app cert. The operator cert's SPIFFE URI (`spiffe://g8e.local/operator/<org_id>/<operator_id>/<operator_session_id>`) is parsed lazily on the first `submit_envelope` call so every submitted envelope is bound to the operator transport identity, satisfying the gateway's `verifyEnvelopeIdentityBinding` check. When the operator-cert env vars are not set, the `GovernanceClient` falls back to the app TLS config and the gateway rejects governance submissions with `ErrPrivilegedEndpointAccess`. This operator-cert sharing is scoped to the governance transport only; the ensemble never uses the operator cert for any other surface.

The enrollment is resumable and idempotent: on restart with an existing valid cert, the `AppEnrollmentService` (`ensemble/app/services/infra/app_enrollment_service.py`) short-circuits the reuse path and the ensemble proceeds directly to startup. On restart while a platform enrollment request is pending, the service loads the persisted pending state (private key, requester token, request ID, CSR fingerprint, expiry) from `pki/pending-enrollment/g8ee.json` and resumes polling the same request without generating new keys. The enrollment runs as Phase 0.25 of the `lifespan` in `ensemble/app/main.py`, before the TLS config is constructed and the operator clients connect. If enrollment fails, the lifespan exception handler re-raises and FastAPI fails to start, so Docker's healthcheck and restart policy surface the failure. See [auth.md](./auth.md) §1.5 for the owner-approved platform enrollment protocol and [Build Apps](../guides/build_apps.md) § Identity and Authentication for the public app enrollment contract.

The dashboard (`g8ed`) follows the same owner-approved platform enrollment contract with its own Node.js implementation: `dashboard/services/infra/app-enrollment-service.js` mirrors the ensemble's `AppEnrollmentService` (load-or-validate installed identity, load persisted pending attempt, submit request, poll status, sign completion transcript, validate response, write credentials atomically), enrolls as app name `g8ed` to obtain the SPIFFE identity `spiffe://g8e.local/app/g8ed`, and persists credentials to its own `g8e-dashboard-data` volume (mounted at `/data`) under the same runtime tree layout (`pki/issued/apps/g8ed.crt`, `pki/issued/apps/g8ed.key`, `pki/trust/hub-bundle.pem`). The dashboard's `G8E_GATEWAY_HTTP_URL` (gateway plain-HTTP discovery surface, resolved from the internal `g8eg` alias in the unified stack), `G8E_GATEWAY_URL` (browser-facing HTTPS URL), and `G8E_RUNTIME_DIR` env vars point at the gateway and the dashboard's own runtime directory. The dashboard also sets `GATEWAY_HEALTH_URL` and `GATEWAY_HEALTH_PATH` for its healthcheck probe. See [Dashboard (g8ed)](./dashboard.md) for the platform-level architecture and [Dashboard Authentication](../dashboard/auth.md) for the parallel enrollment implementation.

## In-Tree Protocol Dependency

g8ee depends on the g8e Python protocol package (`g8e>=1.7.8`). In the monorepo model, this dependency resolves to the in-tree `protocol/python/` package, not the published PyPI package. The ensemble's `pyproject.toml` declares the path dependency through `[tool.uv.sources]`, and the ensemble Dockerfile installs `protocol/python` before the ensemble so pip resolves `g8e` from the already-installed local package. See [Protocol Library](./protocol.md) for the protocol package structure.

## Build and Test

The ensemble ships with its own Dockerfile (`ensemble/Dockerfile`), rooted at the g8e repo root so it can `COPY protocol/python` and `COPY ensemble`. The Makefile provides:

- `make ensemble-test` — runs the ensemble pytest unit and in-process integration suites (`ensemble/tests/unit/` and `ensemble/tests/integration/`, excluding Tier 4 external tests).
- `make test-external` — runs the ensemble Tier 4 external tests (`ensemble/tests/integration/` with the `ai_integration`, `requires_web_search`, or `requires_api` markers). Not in CI; gated on credentials.
- `make ensemble-lint` — runs ruff and pyright on the ensemble.
- `make build-ensemble` — builds the ensemble Docker image.

The application test suite lives under `ensemble/tests/`. The evidence-grade evaluation harness is a separate Python package under `ensemble/evals/` with its own `pyproject.toml`, lockfile, CLI, and CI job. `make evals-test` runs its Tier 1 and Tier 2 tests, and `make evals-lint` runs Ruff and Pyright. Neither suite is moved under the repo-root `test/` directory, which remains Go-specific. The polyglot tests map onto the 4-tier test model as Tier 1 (unit, in-component), Tier 2 (in-process integration), Tier 3 (Docker E2E against the unified compose), and Tier 4 (external, real LLM/API). See [Ensemble Tests](../ensemble/tests.md), [Evals](../ensemble/evals.md), and the [Unified Docker Stack guide](../guides/unified_stack.md).

The eval `verify-receipts` command loads all actuator public keys in the verifier PKI directory, derives their key IDs, selects the key matching each receipt's `signer_key_id`, and verifies both the canonical receipt signature and final persistence attestation. It fails when no key matches a receipt. This supports unified-stack reports containing receipts signed by different actuators (Gateway and Operator), so a single verification pass covers multi-signer evidence without pre-configuring which key belongs to which receipt.

## License

g8ee is licensed under the Business Source License 1.1 (BSL 1.1), matching the rest of the g8e platform. It converts to Apache License 2.0 on 2030-08-18. See `ensemble/LICENSE`.

## Related Documentation

- [AI Agents and the g8e Governance Boundary](./agents.md): The client surface contract g8ee implements.
- [SSE Streaming](./sse.md): The event bridge g8ee publishes through.
- [Protocol Library](./protocol.md): The in-tree Python protocol package g8ee depends on.
- [Unified Docker Stack](../guides/unified_stack.md): Bringing up the whole platform including g8ee.
- [Dashboard (g8ed)](./dashboard.md): The operator dashboard that consumes g8ee's SSE events.
