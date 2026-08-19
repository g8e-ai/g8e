---
title: Ensemble (g8ee)
parent: Architecture
---

# Ensemble (g8ee)

Last Updated: 2026-08-19
Version: v2.0.0

## What g8ee Is

g8ee is the first g8e-compatible agentic ensemble. It is a Python 3.12 / FastAPI service that connects to a g8e gateway over mTLS, submits governed intents through the MCP surface, and streams progress and results back to operators through the SSE event bridge. g8ee is a first-party component of the g8e platform, reunited in-tree in v2.0.0, and lives at `ensemble/` in the repository root.

g8ee is a reference ensemble, not a required one. The gateway treats every AI client as an untrusted principal, so any g8e-compatible client (an MCP client, a custom A2A server, or a third-party ensemble) can drive the platform. g8ee exists to provide a complete, supported, in-tree example of an agentic ensemble that exercises the full governance pipeline end to end, and to serve as the ensemble that ships with the unified Docker stack.

## Role in the Platform

g8ee sits in the AI client tier of the platform. It is a consumer of the gateway surface, not part of the gateway or operator:

- It authenticates to the gateway with an app workload identity (mTLS, enrolled via the PKI API).
- It submits intents through the MCP endpoint, which the gateway wraps in `GovernanceEnvelope` transactions and routes through the five-layer governance pipeline.
- It publishes user-facing events (progress, questions, results) to the gateway through `POST /api/v1/sse/push`, which the gateway streams to browser and CLI clients.
- It never touches a target host directly. All host mutations flow through the governed operator, which re-verifies every proof locally before execution.

See [AI Agents and the g8e Governance Boundary](./agents.md) for the client surface contract and [SSE Streaming](./sse.md) for the event bridge g8ee publishes through.

## Connection Model

g8ee connects to the gateway over mTLS using an app workload identity. In the unified Docker stack (repo-root `docker-compose.yml`), the ensemble container mounts the gateway's data volume read-only at `/root/.g8e` so it can read the gateway's issued app cert/key (`pki/issued/apps/g8ee.crt`, `g8ee.key`) and the trust bundle (`pki/trust/g8eg-ca-bundle.pem`). The ensemble's `G8E_GATEWAY_URL`, `G8E_OPERATOR_URL`, `G8E_OPERATOR_PUBSUB_URL`, `G8E_RUNTIME_DIR`, `G8E_PKI_DIR`, and `G8E_CA_CERT_PATH` environment variables point at the gateway's network alias and runtime tree.

The ensemble does not enroll itself. The gateway issues the app cert during its own PKI bootstrap, and the ensemble reads it from the shared volume at startup. This is the same model the sauvren scratch pad used, re-derived for g8e's `/root/.g8e` layout.

## In-Tree Protocol Dependency

g8ee depends on the g8e Python protocol package (`g8e>=1.7.8`). In the monorepo model, this dependency resolves to the in-tree `protocol/python/` package, not the published PyPI package. The ensemble's `pyproject.toml` declares the path dependency through `[tool.uv.sources]`, and the ensemble Dockerfile installs `protocol/python` before the ensemble so pip resolves `g8e` from the already-installed local package. See [Protocol Library](./protocol.md) for the protocol package structure.

## Build and Test

The ensemble ships with its own Dockerfile (`ensemble/Dockerfile`), rooted at the g8e repo root so it can `COPY protocol/python` and `COPY ensemble`. The Makefile provides:

- `make ensemble-test` — runs the ensemble pytest unit suite (`ensemble/tests/unit/`).
- `make ensemble-lint` — runs ruff and pyright on the ensemble.
- `make build-ensemble` — builds the ensemble Docker image.

The ensemble test suite is co-located under `ensemble/tests/` and `ensemble/evals/`. It is not moved under the repo-root `test/` directory, which remains Go-specific. The polyglot tests map onto the 3-tier test model as Tier 1 (unit, in-component) and Tier 3 (Docker E2E against the unified compose). See the [Unified Docker Stack guide](../guides/unified_stack.md) for bringing up the whole platform.

## License

g8ee is licensed under the Business Source License 1.1 (BSL 1.1), matching the rest of the g8e platform. It converts to Apache License 2.0 on 2030-08-18. See `ensemble/LICENSE`.

## Related Documentation

- [AI Agents and the g8e Governance Boundary](./agents.md): The client surface contract g8ee implements.
- [SSE Streaming](./sse.md): The event bridge g8ee publishes through.
- [Protocol Library](./protocol.md): The in-tree Python protocol package g8ee depends on.
- [Unified Docker Stack](../guides/unified_stack.md): Bringing up the whole platform including g8ee.
- [Dashboard (g8ed)](./dashboard.md): The operator dashboard that consumes g8ee's SSE events.
