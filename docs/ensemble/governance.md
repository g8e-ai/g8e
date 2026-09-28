---
doc_id: ensemble-governance
title: Ensemble Governance
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - ensemble/app/services/operator/
  - internal/services/gateway/
  - internal/services/governance/
  - internal/services/pubsub/
related:
  - ../architecture/governance.md
  - ../architecture/gateway.md
  - ../architecture/operator.md
  - agents.md
  - architecture.md
  - protocol.md
  - storage.md
when_to_read: Understanding ensemble integration with the platform governance boundary, posture behavior, dispatch and envelope paths, and how application-layer intent flows through the five-layer verification model.
do_not_use_for:
  - Protocol-layer governance (docs/architecture/governance.md)
  - Platform gateway architecture (docs/architecture/gateway.md)
  - Operator substrate and deployment (docs/architecture/operator.md)
  - Agent personas and command generation (ensemble/agents.md)
  - Consensus deliberation and L2 signing (docs/architecture/consensus.md)
---

# Ensemble Governance

## Purpose

Documents how the g8e Agentic Ensemble (`g8ee`) submits typed intent to the platform governance boundary, where the Gateway and Operator enforce the five-layer verification model. Application-layer Tribunal voting, Marshal risk analysis, and Auditor review express intent and reasoning; they do not authorize host or platform mutations. An operation becomes governed when g8ee submits typed intent through a platform ingress. The Gateway constructs or accepts a canonical `GovernanceEnvelope`, applies authentication and transport authorization, and the executing Operator locally verifies and executes the transaction through the L4 Warden and L5 Actuator.

Native client tools, direct filesystem access, and operations that do not traverse a platform ingress remain outside this governance boundary.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Invariant groups: [Verification layers](#verification-layers-inv-verify), [Governance postures](#governance-postures-inv-posture), [Ingress paths](#ingress-paths-inv-ingress), [Envelope construction](#envelope-construction-inv-envelope).

## Invariants

### Verification layers (`INV-VERIFY`)

| ID | Rule |
| --- | --- |
| INV-VERIFY-01 | L1 Doctrine decodes the typed protobuf payload, applies field constraints and forbidden-pattern rules, and runs MITRE ATT&CK-oriented threat detection. L1 is mandatory in every posture. |
| INV-VERIFY-02 | L2 Consensus verifies Ed25519 votes over the transaction hash and decision against an enabled policy and trusted member keys. Application Tribunal votes have no authority at L2; required postures enforce quorum from distinct affirmative signers. |
| INV-VERIFY-03 | L3 Notary verifies human authorization for mutation-classified actions under `ratify` and `notary` postures. Read-only actions do not require L3. |
| INV-VERIFY-04 | L4 Warden reserves the nonce, checks expiry, validates action type and payload, recomputes the transaction hash, verifies the current state root, runs L1, and evaluates posture-required L2 and L3 evidence. A failed universal check prevents dispatch. |
| INV-VERIFY-05 | L5 Actuator signs and persists an `EXECUTING` receipt before invoking the action handler, appends a signed commitment when the SQL commitment ledger is active, rehydrates scrubbed payload data, mints a transaction-bound capability, invokes the handler, and signs and persists the final result. |

### Governance postures (`INV-POSTURE`)

| ID | Rule |
| --- | --- |
| INV-POSTURE-01 | The Gateway posture is selected at startup with `--posture <doctrine\|consensus\|ratify\|notary>`. L1 and universal L4 checks remain mandatory in every posture. |
| INV-POSTURE-02 | `doctrine` posture requires only L1 and universal L4 checks; L2 and L3 are not required. |
| INV-POSTURE-03 | `consensus` posture requires L1, universal L4 checks, and L2 verification for mutation and sensitive read-only actions. L3 is not required. |
| INV-POSTURE-04 | `ratify` posture requires L1, universal L4 checks, and L3 verification for mutations. L2 is not required. |
| INV-POSTURE-05 | `notary` posture requires L1, universal L4 checks, L2 verification, and L3 authorization. Both are mandatory. |
| INV-POSTURE-06 | Optional L2 and L3 evidence is verified when present and recorded when valid, but its absence does not gate a posture that does not require it. The ingress path matters: a posture does not cause every transport to acquire missing proofs automatically. |

### Ingress paths (`INV-INGRESS`)

| ID | Rule |
| --- | --- |
| INV-INGRESS-01 | Host-command dispatch through `POST /api/v1/operators/commands` authenticates the app caller over mTLS, adds the current state root and posture, computes the canonical transaction hash, and publishes the resulting `GovernanceEnvelope` to the Operator. This path cannot mint L3 human proofs; mutations requiring L3 are rejected early. |
| INV-INGRESS-02 | Direct governance-envelope submission through `POST /api/v1/governance/envelopes` over enrolled g8ee app mTLS identity submits a complete envelope to the Gateway. The client serializes submissions to reduce state-root races. The direct mutation path succeeds only when the active posture does not require proofs absent from the envelope. |
| INV-INGRESS-03 | Gateway MCP and A2A flows coordinate protocol consensus and can suspend a transaction for WebAuthn approval. These paths require explicit configuration and are distinct from direct relay and dispatch flows. |

### Envelope construction (`INV-ENVELOPE`)

| ID | Rule |
| --- | --- |
| INV-ENVELOPE-01 | The canonical transaction hash binds action type, target resource, typed payload, state root, nonce, expiry, structured intent, requestor identity, and acting-app identity. Both the envelope ID and transaction hash must equal the recomputed SHA-256 digest. |
| INV-ENVELOPE-02 | Gateway construction (via `BuildGovernanceEnvelope` in [dispatch_service.go](internal/services/gateway/dispatch_service.go)) runs L1 doctrine screening, verifies the current state root, generates replay and expiry fields, and fails closed on missing L3 proofs for mutations. |
| INV-ENVELOPE-03 | An envelope expiry is 5 minutes from construction. A dispatched request has 30 seconds to complete the round-trip from in-process publish through operator L4/L5 execution and back. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Five-layer verification | `internal/services/governance/l4_warden.go`, `internal/services/governance/l5_actuator.go` | `NewL4Warden`, `NewL5Actuator`, verify `VerifiedTransaction` and `ActionReceipt` |
| Posture definitions and requirements | `internal/constants/platform.go` | `PostureDoctrine`, `PostureConsensus`, `PostureRatify`, `PostureNotary`, `GetGovernancePostureRequirements` |
| Host-command dispatch endpoint | `internal/services/gateway/dispatch_service.go` | `BuildGovernanceEnvelope`, `DispatchService.HandleDispatch` handles `POST /api/v1/operators/commands` |
| Envelope submission endpoint | `internal/services/gateway/governance_controller.go` | `POST /api/v1/governance/envelopes`, mTLS identity binding, `EnvelopeProcessor` interface |
| Application dispatch client | `ensemble/app/clients/gateway_operator_client.py` | `GatewayOperatorClient` class, `dispatch()` method |
| Application execution service | `ensemble/app/services/operator/execution_service.py` | `OperatorExecutionService` submits through `GatewayOperatorClient` with registered `EventType` |
| Governance envelope protobuf | `protocol/proto/g8e/common/v1/governance.proto` | `GovernanceEnvelope` message, transaction hash fields, identity binding |
| Operator command protobuf | `protocol/proto/g8e/operator/v1/operator.proto` | `OperatorCommandRequest`, action-type payload unions, `ActionReceipt` |
| Receipt verification | `internal/services/governance/l5_actuator.go` | `ActionReceipt` signature field, public-key export via `ActuatorPublicKeyExport` |

## Procedures

### Host-command dispatch through gateway

1. The application submits typed Operator protobuf payload by calling `POST /api/v1/operators/commands` with the registered request `event_type`, delegated Operator session, and application context.
2. The Gateway authenticates the app caller over mTLS, validates the event against the registry, derives `action_type` from the event-type mapping, and checks that the Operator session is active.
3. The Gateway constructs a canonical `GovernanceEnvelope` via `BuildGovernanceEnvelope`. The builder runs L1 doctrine screening on the decoded typed payload, generates nonce and expiry (5 minutes), computes the canonical transaction hash, and adds the current state root and posture. If the posture requires L3 and the action is a mutation, the builder rejects the envelope early with `ErrTxL3ProofUnmintable`.
4. The Gateway publishes the resulting `GovernanceEnvelope` to the Operator through the mTLS transport (WebSocket or HTTP). The transport carries application context (CaseID, InvestigationID, TaskID).
5. The Operator runs L4 Warden verification locally. The L4 Warden checks the action type, decodes the payload, recomputes the transaction hash, verifies the current state root, checks expiry and nonce, runs L1, and evaluates posture-required L2 and L3 evidence. Accepted operations proceed to L5 Actuator execution; rejected operations do not reach the action handler.
6. The L5 Actuator signs an `EXECUTING` receipt before invoking the handler, executes the handler, and signs and persists the final result with a signed `ActionReceipt`.
7. The Gateway receives the correlated result envelope containing the signed receipt in the HTTP response. The Operator's signed receipt is authoritative; the Gateway mirror is best-effort for correlation and audit.

Because the dispatch path cannot coordinate protocol L2 deliberation or mint L3 human proofs, postures behave as follows:

- `doctrine` accepts otherwise valid read and mutation intents without L2 or L3.
- `consensus` rejects ordinary relayed intents because they do not contain protocol L2 votes.
- `ratify` accepts otherwise valid read-only intents but rejects mutation intents without an L3 proof.
- `notary` rejects ordinary relayed intents because L2 is required, and mutations also require L3.

Use Gateway MCP or A2A when the Gateway must coordinate protocol consensus or human approval. See [Build Apps](../guides/build_apps.md) for integration-path selection.

### Direct governance-envelope submission

1. The application calls `POST /api/v1/governance/envelopes` over enrolled g8ee app mTLS identity, submitting a complete `GovernanceEnvelope` in canonical protojson form.
2. The Gateway extracts the mTLS certificate's URI SANs and verifies they match the envelope's internal identity claims (cli_session_id, operator_session_id, operator_id, acting_app_id, source_component). This prevents an app cert from impersonating another workload's envelope.
3. The Gateway binds the envelope's identity to the certificate SPIFFE identity, supplies the active posture when the envelope omits it, and sends the envelope through the in-process L4 Warden and L5 Actuator.
4. The L4 Warden verifies L1, universal checks, and posture-required L2 and L3 evidence. The L5 Actuator executes and returns a signed `ActionReceipt`.
5. A successful submission returns a signed `ActionReceipt`. The client exposes receipt-signature verification using the configured Actuator public key, but submission does not invoke that verification automatically.

The client serializes submissions to reduce state-root races. If the Gateway rejects a submission because another transaction changed the state root, the client fetches the new root, rebuilds the envelope, and retries up to three times after the initial attempt. This client does not acquire protocol L2 votes or perform a WebAuthn ceremony. The optional `agent_ids` argument only populates the envelope's `consensus_set_id`; it does not turn application personas into enrolled protocol signers or attach votes.

Platform-record submissions (cases, investigations, memories, agent activity) run under `doctrine` posture in the default unified deployment. A certificate fingerprint alone is transport metadata, not a complete L3 authorization proof. The direct mutation path succeeds only when the active posture does not require proofs absent from the envelope.

### Application-layer controls before governance

The application applies independent quality and safety checks before publishing intent to the platform. These controls improve command quality and reduce unsafe proposals, but they are not protocol governance proofs.

**Intent and command separation.** The application represents the requested outcome (via `SageOperatorRequest`) without supplying shell syntax. The Tribunal generates multiple candidate commands from that intent and available Operator context.

**Tribunal voting.** Five Tribunal members independently generate candidates. Each candidate is normalized and passes deterministic command-safety checks before voting. Tribunal members have equal vote weight. A candidate reaches consensus when at least two members produce the same normalized command. Ties prefer the shortest command, then a candidate without Nemesis support. Failure to reach consensus in the initial round triggers a second generation round using anonymized first-round clusters. Tribunal voting is application-level model agreement; it does not produce the Ed25519 signatures required by platform L2 Consensus and cannot satisfy a `consensus` or `notary` posture.

**Command risk and audit.** When a response analyzer is configured, Marshal evaluates the winning command before the Auditor. An unavailable analyzer, empty response, analysis error, or inconclusive result becomes `HIGH` risk and blocks the command. The first high-risk result returns contextual feedback; a second consecutive high-risk result for the same investigation reports an agent conflict and requires human intervention. If no analyzer is configured, the application skips this stage.

When enabled, the Auditor reviews the winning command and anonymized alternatives after Marshal risk analysis. The Auditor can accept the winner, revise it, or select another candidate, and validates a selected or revised command against deterministic safety rules. A passing audit creates a reputation commitment; failure to create that commitment stops the verdict. When the Auditor is disabled, the application accepts the Tribunal winner without this model-review stage or reputation commitment.

File writes and replacements use a separate file-risk analysis and application approval flow before dispatch. A file-risk analysis failure is logged and the operation continues to the approval gate; an analysis that marks the operation unsafe blocks it. Error analysis classifies failed commands and controls bounded retry or escalation. These application approval and retry decisions are distinct from L3 Notary authorization.

## Anti-patterns

- Treating application Tribunal voting as platform L2 consensus. Tribunal voting is independent reasoning; it does not authorize platform mutations.
- Assuming application approval prompts replace L3 Notary authorization. Application approvals are distinct from platform verification and cannot satisfy `ratify` or `notary` postures.
- Dispatching mutations through `POST /api/v1/operators/commands` under `ratify` or `notary` postures without pre-acquired L3 proof. The dispatch path cannot mint L3 proofs; early rejection is guaranteed.
- Mismatching transport identity to envelope fields. The Gateway verifies mTLS certificate URI SANs against envelope identity claims; mismatches are rejected at the transport boundary.
- Relying on unsigned application SSE events for proof of execution. Signed receipts from the Operator (L5) are authoritative; application telemetry is best-effort.
- Updating envelope metadata or payload after signature. The transaction hash binds all identity, payload, and state-root fields; modifications invalidate the hash.
- Confusing Operator-side receipt verification with Gateway-side result acceptance. The Operator's signed receipt is authoritative; the Gateway mirror is for correlation and audit only.

## Links out

- [Platform Governance Pipeline](../architecture/governance.md) — Five-layer verification, posture behavior, and transaction flow.
- [Platform Gateway Architecture](../architecture/gateway.md) — Gateway ingress, identity binding, consensus coordination, and approval suspension.
- [Platform Operator Architecture](../architecture/operator.md) — Operator substrate, outbound transport, local verification, and execution.
- [Platform Agents and Boundaries](../architecture/agents.md) — Agent ingress paths, launcher behavior, and governance limits.
- [Consensus and L2 Signing](../architecture/consensus.md) — Protocol L2 policy, enrollment, deliberation, and vote verification.
- [Agent Personas](agents.md) — Sage, Tribunal, Marshal, Auditor, and Nemesis personalities and consensus mechanism.
- [Ensemble Architecture](architecture.md) — Application runtime, services, and model hierarchy.
- [Protocol Definitions](protocol.md) — Ensemble-facing protocol models and envelope hashing rules.
- [Data and Storage](storage.md) — Ensemble data services and governed platform records.
- [Documentation Guide](../devs/docs.md) — Documentation audit and ownership rules.
