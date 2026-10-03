---
doc_id: ensemble_architecture
title: Ensemble Architecture
audience: platform and feature developers, coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - ensemble/app/
  - ensemble/app/main.py
  - ensemble/app/services/
  - ensemble/app/clients/
related:
  - agents.md
  - governance.md
  - storage.md
  - sse.md
  - ../architecture/agents.md
  - ../architecture/gateway.md
  - ../architecture/operator.md
  - ../architecture/governance.md
  - ../architecture/auth.md
when_to_read: Understanding g8ee component interactions, debugging application startup and service dependencies, designing features that traverse the platform boundary, or reviewing governance-path changes.
do_not_use_for:
  - Persona models and Tribunal stages (agents.md)
  - Five-layer verification and posture semantics (governance.md)
  - Storage tiers and cache behavior (storage.md)
  - SSE streaming and event delivery (sse.md)
  - Platform topology and Operator independent verification (../architecture/gateway.md, ../architecture/operator.md)
---

# Ensemble Architecture

## Purpose

Documents the g8ee application architecture: component startup, request authentication, conversation flow, governed dispatch, application-record writes, and service dependencies. Establishes the trust boundary between g8ee intent and platform authorization through the Gateway and Operator.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Component lifecycle](#component-lifecycle-inv-ens-life), [Dispatch and envelopes](#dispatch-and-envelopes-inv-ens-disp), [Service boundaries](#service-boundaries-inv-ens-svc), [Trust and authorization](#trust-and-authorization-inv-ens-auth).

## Invariants

IDs are stable. Append the next free number in a topic. Do not renumber.

### Component lifecycle (`INV-ENS-LIFE`)

| ID | Rule |
| --- | --- |
| INV-ENS-LIFE-01 | g8ee startup in `ensemble/app/main.py:lifespan()` follows explicit phases: (1) local settings bootstrap, (2) app enrollment service, (3) client connections (DB, KV, Blob, HTTP), (4) cache-aside coordination, (5) platform settings load, (6) all domain services via ServiceFactory, (7) HTTP client injection into LLM provider factory, (8) service startup. `FastAPI` readiness is not signaled until all phases complete. |
| INV-ENS-LIFE-02 | Three routers are registered in `_build_app()`: `health_router`, `chat_router`, and `internal_router`. All are imported from `ensemble/app/routers/` and registered via `application.include_router()`. Router prefixes and paths are defined in `ensemble/app/constants/api_paths.json` and generated into `ensemble/app/constants/generated_paths.py`. |
| INV-ENS-LIFE-03 | Shutdown in `lifespan()` finally-block waits up to five seconds for tracked chat tasks, stops all domain services, closes client connections (KV, Blob, HTTP, DB), and logs completion. Graceful shutdown does not attempt to cancel in-flight host operations; the Operator owns those receipts and audit logs independently. |
| INV-ENS-LIFE-04 | The app enrollment service loads or creates an enrolled app identity during startup. Existing credentials within one day of expiry are renewed automatically. The process does not become ready while enrollment is pending or invalid. The enrolled certificate is used for DB, KV, Blob, and HTTP client connections. |

### Dispatch and envelopes (`INV-ENS-DISP`)

| ID | Rule |
| --- | --- |
| INV-ENS-DISP-01 | Host-command requests flow through `OperatorExecutionService.dispatch_command()`, which serializes the typed `G8eMessage` payload to protobuf bytes and calls `GatewayOperatorClient.dispatch()` with the registered request `event_type`, delegated Operator session ID, and application context fields. The call posts to `POST /api/v1/operators/commands` over mTLS. |
| INV-ENS-DISP-02 | `GovernanceClient` submits protected application-record writes to `POST /api/v1/governance/envelopes`. The client constructs canonical `GovernanceEnvelope` with typed payload bytes, requestor identity, acting app, delegated Operator/session, case/investigation IDs, nonce, expiry, and state root transaction hash. Submissions are serialized with a lock; `TX_STATE_MISMATCH` retries up to three times by fetching a fresh state root and rebuilding the envelope. |
| INV-ENS-DISP-03 | The Gateway validates the envelope, binds app transport identity to envelope acting-app and session fields, injects its active posture when the client omits one, and executes through the governance pipeline. If the posture requires protocol evidence (L2 votes, L3 proof) that the envelope does not contain, the mutation fails closed without creating default governance votes or suspending for approval. |
| INV-ENS-DISP-04 | g8ee does not construct the governed envelope for dispatch or fetch Gateway state root for this path. g8ee does not open an inbound management path to target Operators. Governance coordination, state root management, and L4 Warden verification are Gateway-owned. |

### Service boundaries (`INV-ENS-SVC`)

| ID | Rule |
| --- | --- |
| INV-ENS-SVC-01 | `ServiceFactory` (in `ensemble/app/services/service_factory.py`) is the sole builder of all domain services. No service is constructed outside the factory. The factory method `create_all_services()` receives settings, cache-aside service, and client handlers (DB, KV, Blob, HTTP) and returns a `ServiceContainer` with all constructed services. Services are bound to app state via `bind_to_app_state()`. |
| INV-ENS-SVC-02 | `AuthService` authenticates either a bearer Operator session after Gateway validation or a proxy context containing required user identity fields and the Gateway's signature over the request (INV-AUTH-ID-07). It then checks request context ownership and session bindings. Operator-authentication relay routes are explicit workflow exceptions; they do not make arbitrary unauthenticated internal routes valid. |
| INV-ENS-SVC-03 | The `InternalHttpClient` is constructed and owned by the application lifecycle and injected into the LLM provider factory at `ensemble/app/llm/factory.py:set_internal_http_client()`. The factory uses it to route governed-dispatch inference requests through the Gateway. g8ee does not control client lifecycle; it manages only injection timing. |

### Trust and authorization (`INV-ENS-AUTH`)

| ID | Rule |
| --- | --- |
| INV-ENS-AUTH-01 | g8ee remains outside the trusted execution boundary. Model output, Tribunal agreement, application memory, and application-level approval express application intent or telemetry; they do not authorize a host or platform mutation. The Gateway and executing Operator apply the active governance posture before an operation executes. |
| INV-ENS-AUTH-02 | Conversation content, attachments, prompts, model outputs, and returned host output are application data and may contain sensitive user content. Model telemetry records provider and model identity, timing, usage, retry and finish metadata, canonical input/output hashes, and privacy-analysis metadata. It does not attest to provider behavior outside the governed g8e path. |
| INV-ENS-AUTH-03 | SSE events, application approvals, memories, reputation, and model telemetry do not authorize execution or change the Gateway state root. The governance boundary covers only operations that traverse g8e; it does not govern native client tools, provider behavior, unrestricted network access, or other side channels. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Application startup and lifespan | `ensemble/app/main.py` | Phases 1-8 in `lifespan()`, three routers in `_build_app()` |
| API path constants | `ensemble/app/constants/api_paths.json` | Canonical route definitions; generated into `generated_paths.py` |
| Health router | `ensemble/app/routers/health.py` | `/health`, `/health/live`, `/health/details` without authentication |
| Chat router | `ensemble/app/routers/chat.py` | Chat triage and session queries with `require_authenticated_context` |
| Internal router | `ensemble/app/routers/internal_router.py` | Chat start/stop, cases, investigations, settings, approvals, operator lifecycle |
| Service factory | `ensemble/app/services/service_factory.py` | Single source of all domain service construction |
| Authentication service | `ensemble/app/services/auth/auth_service.py` | Bearer token and proxy context validation |
| Operator execution | `ensemble/app/services/operator/execution_service.py` | Command dispatch through `GatewayOperatorClient` |
| Gateway operator client | `ensemble/app/clients/gateway_operator_client.py` | Dispatch to `POST /api/v1/operators/commands` |
| Governance client | `ensemble/app/clients/governance_client.py` | Envelope submission to `POST /api/v1/governance/envelopes` |
| Conversation and tool flow | `ensemble/app/services/ai/` | Triage, provider selection, ReAct loop, tool results |
| Tribunal command generation | `ensemble/app/services/ai/generator.py` | Five independent persona passes and consensus voting |

## Procedures

### Startup and Service Initialization

1. Application creates local settings from environment and bootstrap files.
2. App enrollment service loads or creates an enrolled app identity with Gateway approval.
3. Client connections are established for DB, KV, Blob, and internal HTTP.
4. Cache-aside service coordinates DB and KV for cache-read and cache-write patterns.
5. Platform settings are loaded through cache-aside and overlaid on bootstrap settings.
6. `GovernanceClient` is constructed with mTLS config and operator session ID.
7. `ServiceFactory.create_all_services()` constructs all domain services in one call.
8. `InternalHttpClient` is injected into the LLM provider factory for governed-dispatch inference.
9. All services transition to started state; readiness is signaled only after this completes.

### Request Authentication and Routing

1. Incoming request arrives at one of the three routers (health, chat, internal).
2. Health router endpoints (`/health`, `/health/live`, `/health/details`) do not require authentication.
3. Chat and internal routes check `require_authenticated_context` middleware.
4. `AuthService` authenticates either a bearer Operator session (validated with Gateway) or proxy headers containing user identity with a valid Gateway signature.
5. Request context ownership and session bindings are checked.
6. Operator-authentication relay routes are explicit exceptions and are documented per route.
7. Request proceeds to handler with authenticated `G8eHttpContext`.

### Conversation and Command Dispatch

1. Authenticated request loads user settings, investigation history, memories, and attachments.
2. Triage routes simple turns to Dash and complex turns or triage failures to Sage.
3. Selected provider streams response through sequential ReAct loop.
4. Tool results return to model for subsequent turns.
5. When operator command is needed, Tribunal generates candidates through five independent persona passes (Axiom, Concord, Variance, Pragma, Nemesis).
6. Tribunal voting selects a command with at least two votes; ties are broken deterministically.
7. Command passes Marshal risk analysis, deterministic validation, and g8ee approval service.
8. `OperatorExecutionService.dispatch_command()` serializes `G8eMessage` to protobuf and calls `GatewayOperatorClient.dispatch()`.
9. Gateway validates envelope, derives `action_type` from event registry, constructs `GovernanceEnvelope`, forwards to matching Operator session.
10. Operator independently performs verification and execution stages.
11. HTTP response carries correlated result envelope; g8ee decodes it and publishes application result event.

### Application-Record Writes

1. Protected application data (cases, investigations, conversation history, memories, settings, reputation, stake resolutions) is read through Gateway document, KV, and blob clients.
2. Owning g8ee data services submit writes through `GovernanceClient` at `POST /api/v1/governance/envelopes`.
3. Client maps internal payload names to canonical protocol types.
4. Client binds requestor, acting app, Operator/session, case/investigation IDs, nonce, expiry, and state root into transaction hash.
5. Client submits canonical protojson with mTLS authentication.
6. Submissions are serialized with a lock; `TX_STATE_MISMATCH` errors trigger retry with fresh state root (up to three retries).
7. Gateway verifies supplied envelope, binds app transport identity, injects active posture, executes through governance pipeline.
8. If posture requires protocol evidence not present in envelope, mutation fails closed without creating default governance state.

## Anti-patterns

- Constructing services outside `ServiceFactory` (INV-ENS-SVC-01).
- Altering `last_updated` or `version` without conducting an end-to-end audit (INV-DOC-FMT-02 from docs.md).
- Hard-wrapping source prose or embedding source code line numbers (INV-DOC-STYLE-01 from docs.md).
- Treating Tribunal agreement as protocol L2 consensus or application approvals as governance authorization (INV-ENS-AUTH-01).
- Attempting to open inbound management paths to Operators or construct governance envelopes outside g8ee's responsibility boundary (INV-ENS-DISP-04).
- Assuming SSE events, memories, reputation, or model telemetry authorize execution or change platform state (INV-ENS-AUTH-03).

## Links out

- [Agents](agents.md): Persona models, Tribunal seats, Marshal and Auditor stages, and agent pipeline.
- [Governance](governance.md): Five-layer verification pipeline and posture semantics.
- [Storage](storage.md): Gateway-backed storage tiers, cache-aside coordination, and restart behavior.
- [SSE Streaming](sse.md): Gateway event delivery and session-targeted push ingestion.
- [AI Agents and the g8e Governance Boundary](../architecture/agents.md): Client integration paths and platform boundary limits.
- [Gateway Architecture](../architecture/gateway.md): Gateway services, authentication, governance coordination, and dispatch forwarding.
- [Operator Architecture](../architecture/operator.md): Independent verification, L5 execution, and authoritative audit.
- [Governance Pipeline](../architecture/governance.md): Five-layer verification pipeline and posture behavior.
- [Authentication and Authorization](../architecture/auth.md): Workload enrollment, mTLS identities, and human authorization.
- [Documentation Guide](../devs/docs.md): Documentation audit and ownership rules.
