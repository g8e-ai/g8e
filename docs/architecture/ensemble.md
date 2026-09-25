---
title: Ensemble (g8ee)
parent: Architecture
---

# Ensemble (g8ee)

Last Updated: 2026-09-23
Version: v2.1.12

## Scope

g8ee is the optional first-party Python 3.12/FastAPI application for conversational interaction with g8e. It provides triage, model selection, streaming ReAct tool loops, Tribunal command generation, application approvals, cases, investigations, memories, model telemetry, and Gateway event publication. It is not required for Gateway, MCP, A2A, or standalone Operator operation.

g8ee is outside the trusted execution boundary. Model output, Tribunal agreement, reputation, application memory, and application approval express application intent or telemetry; they do not authorize a host or platform mutation. Governed actions still require the Gateway and executing Operator to enforce the active posture.

## Runtime and interfaces

The root Compose stack runs g8ee as the `ensemble` service in the `bootstrapped` profile. The image is built from `ensemble/Dockerfile`, exposes container port 8000, and publishes it as `${G8E_ENSEMBLE_PORT:-8000}`. Its persistent volume is `g8e-ensemble-data` mounted at `/root/.g8e`; the container also receives `/operator-state` read-only from `g8e-operator-data` for bootstrap secrets and shares the stack's `/tmp` volume. The container has no host execution boundary. Host operations target a separately enrolled Operator through Gateway pub/sub.

The FastAPI application registers three router groups:

- `/health`, `/health/live`, and `/health/details` provide process and dependency health. `/health` is the Compose healthcheck and does not require application authentication.
- `/api/v1/chat`, `/api/v1/chat/stop`, `/api/v1/chat/triage/*`, chat-session queries, case and investigation operations, settings, and evaluation trace routes provide the g8ee application surface. These routes use g8ee authentication and validated `G8eHttpContext` dependencies except for explicitly exempted operator-authentication relay routes.
- `/api/v1/operator/*`, `/api/v1/operators/*`, and `/api/v1/auth/*` provide application-side Operator workflow and credential operations. They are not a replacement for the Gateway's public protocol or governance routes.

The canonical route definitions are in `ensemble/app/constants/api_paths.json` and the implementation is registered in `ensemble/app/main.py`. This document describes the architecture rather than duplicating the generated API contract.

## Trust boundaries and relationships

| Relationship | Purpose | Boundary and identity |
| --- | --- | --- |
| Client to g8ee | Starts or resumes chat and investigations, submits turns and supported attachment references, answers triage or application approval prompts, and reads application state. | The request must carry a context that g8ee's `AuthService` authenticates and validates. Browser and CLI identity binding is mediated by the Gateway context used by the caller; g8ee does not treat arbitrary identity headers as authority. |
| g8ee to Gateway data services | Reads platform settings, user settings, documents, key-value data, and blob objects. | g8ee uses its enrolled app certificate over mTLS through DB, KV, blob, and HTTP clients. Gateway-backed services own durable application data. |
| g8ee to Gateway event bridge | Publishes typed chat, approval, command, reputation, and background events. | `EventService` sends session-targeted events through the Gateway SSE push API. Events are delivery telemetry, not governance state or durable execution evidence. Targetless events are skipped. |
| g8ee to command pub/sub | Sends a command request to one selected Operator and receives its correlated result. | g8ee authenticates to Gateway pub/sub with its app workload certificate and publishes to the exact `cmd:<operator_id>:<operator_session_id>` channel. The Gateway authorizes the publisher and the target session. |
| g8ee to governance endpoint | Writes protected application records such as cases, investigations, memories, activity, reputation, and stake resolutions. | `GovernanceClient` submits canonical envelopes over the Gateway HTTPS endpoint using the enrolled app mTLS certificate and the configured Operator session bearer value. The Gateway still applies identity binding and all required governance checks. |

The Gateway is the Policy Decision Point. The target remote Operator is the Policy Execution Point for its own runtime and independently verifies the envelope before L5 execution. The Gateway's embedded Operator is a separate execution substrate for Gateway-local ingress paths; g8ee host commands use the selected remote Operator command channel instead.

## Startup and identity

g8ee's FastAPI lifespan performs startup in a fixed order:

1. Load local bootstrap settings.
2. Load a valid enrolled app identity or run owner-approved platform enrollment for component `g8ee` and kind `ensemble`.
3. Create the app mTLS configuration and connect the DB, KV, pub/sub, and blob clients.
4. Build handler services, load platform settings through the Gateway-backed cache-aside service, and merge them with local settings.
5. Construct `GovernanceClient`, application domain services, Gateway protocol clients, the chat pipeline, and the LLM provider integration.
6. Start certificate, command-pub/sub, and HTTP services. Only then does the FastAPI lifespan yield readiness; heartbeat and Operator lifecycle processing remain in the Gateway.

Enrollment uses the Gateway's plain-HTTP discovery surface to request, poll, and complete approval. The service generates a P-256 key and CSR, persists an atomic pending attempt with restrictive permissions, resumes an unexpired pending request after restart, verifies the issued chain, SANs, public key, and component kind, and atomically installs the certificate, key, and trust bundle. Existing credentials are renewed when they are within one day of expiry. Approval occurs in the Gateway console; g8ee does not become ready while enrollment is pending. The app certificate and pending state live in g8ee's own runtime volume.

In the unified deployment, `/operator-state` supplies bootstrap material such as the audit HMAC key and secret paths. It is not a general host filesystem mount and does not make the Docker host visible to g8ee.

See [Authentication and Authorization](./auth.md) and [Unified Docker Stack](../guides/unified_stack.md) for the platform enrollment and deployment procedures.

## Conversation and tool flow

A chat turn is authenticated and context-validated before g8ee loads user settings, investigation history, memories, and referenced attachments. Triage routes the request to the Dash assistant path or Sage primary path. The selected provider streams model output; tool calls are processed by the typed tool registry and the ReAct loop, with each tool result returned to the model for a subsequent turn. Provider adapters include OpenAI, Anthropic, Gemini, Ollama, llama.cpp, a fake provider for tests, and the governed `g8e` inference provider when configured.

Host-command tools pass through the Tribunal's five independent members, candidate clustering, optional Auditor review, Marshal risk analysis, deterministic command validation, and the g8ee application approval service. Auto-approved commands skip only the g8ee prompt after passing the hard command gates; auto-approval is not protocol L3. A Tribunal result is application reasoning and does not create protocol L2 signatures.

The command tool creates a typed internal `G8eMessage`. At the pub/sub boundary, g8ee serializes it as canonical protojson `CommandIntent`, including the exact Operator and session, action type, typed protobuf payload, and request context. g8ee does not construct the governed envelope for this path and does not fetch the Gateway state root.

The Gateway's command relay rejects malformed intents, mismatched channels, invalid Operator sessions, and disallowed witness dispatches. It performs L1 screening, obtains the current state root, adds posture and identity data, constructs the canonical `GovernanceEnvelope`, and forwards it only to the matching Operator session. The Operator independently performs L1-L4 and L5 execution. Results return on the matching results channel and g8ee publishes the application result event.

When the agent reaches `AGENT_MAX_TOOL_TURNS` (currently 25), the loop requests a separate g8ee continuation approval. Approval resets the loop counter; denial or timeout stops the loop. This approval concerns application execution flow and is not protocol L3. Triage clarification similarly pauses the application workflow until the caller answers, skips, or times out.

See [g8ee Agents](../ensemble/agents.md) for persona responsibilities and Tribunal stages.

## Governed application-record path

Protected application data is read through the Gateway document, KV, and blob clients. The owning g8ee data services submit protected writes through `GovernanceClient` at `POST /api/v1/governance/envelopes`. The client maps internal payload names to canonical protocol types, serializes typed payload bytes, binds requestor, acting app, Operator/session, case and investigation identifiers, nonce, expiry, and state root into the transaction hash, and submits canonical protojson.

The client serializes submissions with a lock and retries a Gateway `TX_STATE_MISMATCH` by fetching a fresh state root, up to three retries. The Gateway verifies the supplied envelope; it does not manufacture missing L2 votes or human L3 authorization. The g8ee client may include an mTLS certificate fingerprint in the envelope's L3 metadata, but that fingerprint is not a WebAuthn assertion or signed CLI proof. Consequently, protected mutations fail closed when the active posture requires protocol evidence that the request does not contain. The application path must not be described as equivalent to Gateway MCP/A2A paths that can coordinate configured L2 deliberation or suspend supported L3 approvals.

The current implementation uses the enrolled g8ee app certificate for this HTTPS client and supplies the configured Operator session bearer value. It does not use the read-only `/operator-state` mount as a general credential or execution channel. Transport identity, session authorization, and envelope identity fields remain separate checks.

## Persistence and ownership

g8ee has no local durable application database. The Gateway owns durable application documents, KV values, and blob objects in its own runtime storage. g8ee owns application service logic and uses those Gateway-backed stores for cases, investigations, conversation history, memories, settings, Operator workflow records, agent activity, reputation, and stake resolutions.

The Gateway is the sole owner of the Operator domain: documents, auth/session state, binding, lifecycle, command dispatch, results, `latest_heartbeat_snapshot`, and denormalized `current_hostname`. It stores the canonical `operator.v1.HeartbeatResult` protojson snapshot. g8ee does not subscribe to heartbeat channels, persist Operator fields, or maintain an Operator service; it calls Gateway protocol endpoints when application features need Operator data or execution.

The g8ee process owns only ephemeral coordination: active model turns, background task tracking, pending application approvals, and command-result correlations. These do not survive an ensemble restart. Its certificate, key, trust bundle, and resumable enrollment state persist in the separate ensemble runtime volume. The executing Operator owns authoritative receipts, audit, replay, execution-vault, command output, and file-mutation evidence in its own runtime; g8ee may copy selected results into application records or model context but does not replace that evidence.

Conversation content, attachments, prompts, model outputs, and returned host output are application data and may contain sensitive user content. Model telemetry records provider/model identity, timing, usage when available, retry and finish metadata, canonical input/output hashes, and privacy-analysis metadata; it does not make a claim about provider behavior outside the governed g8e path.

## Five-layer boundary and limits

For the command-relay path, the Gateway constructs the envelope and the remote Operator verifies it. For protected application-record writes, g8ee supplies the envelope and the Gateway verifies it. In either case the active posture determines the required gates:

1. **L1 Doctrine** validates typed payloads and hard safety and threat rules.
2. **L2 Consensus** verifies enrolled-member Ed25519 votes when the posture requires consensus. Tribunal model agreement is not L2.
3. **L3 Notary** verifies WebAuthn or signed CLI authorization for mutation actions when the posture requires ratification. g8ee application approval and an mTLS fingerprint are not substitutes.
4. **L4 Warden** checks expiry, nonce replay, transaction hash, state root, target identity, and required evidence before dispatch.
5. **L5 Actuator** invokes the isolated handler with a transaction-bound capability and produces signed execution or rejection evidence.

SSE events, application approvals, memories, reputation, and model telemetry do not authorize execution or change the Gateway state root. g8ee does not open an inbound management path to a target host. The governance boundary covers only operations that traverse g8e; it does not govern native client tools, provider behavior, unrestricted network access, or other side channels.

See [Governance](./governance.md) for canonical posture semantics and [AI Agents and the g8e Governance Boundary](./agents.md) for the distinctions among MCP, A2A, direct envelopes, and command relay.

## Related documentation

- [AI Agents and the g8e Governance Boundary](./agents.md): Client integration paths and the limits of application agents.
- [Gateway Architecture](./gateway.md): Gateway services, authentication, governance coordination, and local execution.
- [Operator Architecture](./operator.md): Outbound transport, independent verification, L5 execution, and authoritative audit.
- [Governance](./governance.md): Five-layer verification and posture behavior.
- [Authentication and Authorization](./auth.md): Workload enrollment, mTLS identities, and human authorization.
- [SSE Streaming](./sse.md): Gateway event delivery and session targeting.
- [g8ee Agents](../ensemble/agents.md): Persona roster, Tribunal, Auditor, and Marshal.
- [Ensemble Storage](../ensemble/storage.md): Gateway-backed application storage and restart behavior.
- [Unified Docker Stack](../guides/unified_stack.md): Compose topology, ports, profiles, and approval workflow.
- [Documentation Guide](../devs/docs.md): Documentation audit and ownership rules.
