---
title: Ensemble (g8ee)
parent: Architecture
---

# Ensemble (g8ee)

Last Updated: 2026-09-08
Version: v2.1.7

## Scope

g8ee is the optional first-party agentic ensemble for g8e. It is a Python 3.12 and FastAPI service that owns conversational triage, model selection, tool loops, command generation, cases, investigations, memory, model telemetry, and user-facing progress events. It ships in the unified Docker stack, but the Gateway does not require it and other clients can use the platform through MCP, A2A, or compatible native integrations.

g8ee remains outside the trusted execution boundary. Model output, Tribunal agreement, application memory, and application-level approval do not authorize a host mutation. The Gateway and the target Operator apply the active governance posture before an operation executes.

## Platform Relationships

| Relationship | Purpose | Trust boundary |
| --- | --- | --- |
| Client to g8ee | Starts and resumes investigations, sends chat turns and attachments, answers clarification questions, and approves application workflows. | Browser requests use the dashboard's proxy identity context. CLI requests use an Operator session that g8ee validates with the Gateway. |
| g8ee to Gateway data services | Reads platform settings and application state through the database, key-value, and blob transports. | g8ee authenticates with its enrolled app workload certificate. |
| g8ee to Gateway event bridge | Publishes typed progress, question, approval, result, and reputation events for browser and CLI sessions. | The Gateway authenticates g8ee and routes each event to its declared web or CLI session. Events are delivery telemetry, not governance state. |
| g8ee to target Operator | Sends typed host-operation intent and receives correlated results over the Gateway pub/sub service. | The Gateway validates the exact Operator session and constructs the envelope. The target Operator independently verifies and executes it. |
| g8ee to Gateway governance | Writes designated governed application records, including cases, investigations, memories, activity telemetry, and reputation records. | The direct-envelope route requires an authorized Operator transport identity and a complete envelope containing every proof required by the active posture. |

The browser authentication path assumes the dashboard or another trusted proxy supplies the proxy identity context. The CLI path binds the presented Operator session, CLI session, and user identity through Gateway validation before g8ee accepts the request context.

## Conversation and Tool Flow

A conversation turn follows this flow:

1. g8ee authenticates the caller, validates the request context, loads platform and user settings, and reads the relevant case, investigation history, attachments, Operator state, and memories.
2. Triage classifies the turn. Simple turns use the Dash assistant role, while complex turns use the Sage primary role.
3. The selected model streams a response and can request tools through a sequential ReAct loop. Tool results return to the model for the next turn until the model stops requesting tools.
4. A host-command request enters the five-member Tribunal. The members generate candidates independently, the ensemble clusters and votes on the candidates, the optional Auditor checks the selected command, and the application Warden assesses execution risk.
5. The command passes deterministic command constraints and the g8ee approval workflow. Configured auto-approved commands skip this application prompt only after the command passes the hard safety checks.
6. g8ee sends a typed `CommandIntent` for each target Operator. The Gateway binds current state, posture, identity, replay controls, and the exact Operator session into a `GovernanceEnvelope`, then relays it to that Operator.
7. The Operator verifies the envelope and executes an accepted operation through its Actuator. Results return on the session-specific result channel, and g8ee publishes the corresponding client event and returns the result to the model when the tool loop continues.
8. g8ee persists the conversation and associated application records. Memory generation runs after the response path, and model calls contribute typed usage, timing, retry, finish, artifact-hash, and privacy metadata.

When the model reaches the configured tool-turn limit, g8ee requests an explicit continuation decision. Approval resets the turn counter, while denial stops the loop. Clarification questions similarly pause progress until the caller answers, skips, or times out.

See [g8ee Agents](../ensemble/agents.md) for the persona roster, Tribunal stages, and application Warden behavior.

## Governance Paths

g8ee uses two distinct governed mutation paths:

### Host operations

For commands and other Operator tools, g8ee publishes a typed `CommandIntent` with the target Operator and session. The Gateway rejects unauthorized publishers, malformed intent, a channel and target mismatch, or an invalid Operator session. It then constructs the canonical envelope with its current state root and active posture before delivering it to the target Operator.

The command relay does not obtain missing protocol L2 votes or L3 authorization proofs. g8ee's Tribunal vote is application reasoning, not Ed25519 protocol consensus, and the g8ee command approval prompt is not WebAuthn or signed CLI authorization. A relayed mutation therefore succeeds only when the constructed envelope already satisfies the active posture; a posture that requires absent L2 or L3 evidence fails closed at Operator verification.

### Governed application records

For designated application records, g8ee constructs a canonical `GovernanceEnvelope`, binds the current state root, identity, nonce, expiry, and typed payload into its transaction hash, and submits it to the Gateway's privileged governance surface. The unified deployment uses the Operator certificate only for this transport because app certificates cannot access the privileged route. Other Gateway traffic continues to use the g8ee app identity.

The Gateway binds the envelope to the authenticated Operator identity, supplies the active posture when the client leaves it unset, and verifies the envelope through its local Warden and Actuator. This route also does not create missing L2 votes or human L3 authorization. A certificate fingerprint is transport evidence and does not replace a posture-required WebAuthn or signed CLI proof.

## Five-Layer Interlock

Both paths terminate at the same governance model, with the active posture deciding which evidence is mandatory:

1. **L1 Doctrine** validates the typed payload, applies hard gates and forbidden-pattern rules, and detects MITRE ATT&CK-oriented threats.
2. **L2 Consensus** verifies Ed25519 votes from enrolled consensus members against the configured policy and quorum when the posture requires consensus.
3. **L3 Notary** verifies human authorization through WebAuthn or a signed CLI proof for mutations when the posture requires ratification.
4. **L4 Warden** checks signatures, expiry, nonce replay, transaction hash, current state root, target identity, and all posture-required evidence before dispatch.
5. **L5 Actuator** invokes the isolated handler with a transaction-bound just-in-time capability and produces signed execution or rejection receipts.

See [Governance](./governance.md) for posture behavior and [AI Agents and the g8e Governance Boundary](./agents.md) for the differences among MCP, A2A, direct envelopes, and the Operator command relay.

## Identity and Startup

g8ee enrolls as the reserved `g8ee` app through the owner-approved platform enrollment flow. On first startup it generates a P-256 key and certificate request, submits an enrollment request through the Gateway discovery surface, waits for approval of the exact request ID, proves possession of the private key, and stores the issued certificate chain and trust bundle in its own runtime volume. Existing valid credentials are reused, and an interrupted pending enrollment resumes with the same request and key material.

Enrollment completes before g8ee opens its authenticated Gateway transports or starts serving as ready. Missing, expired, or near-expiry credentials trigger enrollment; startup fails closed when g8ee cannot obtain and validate an app identity, including when the owner denies the request. The unified stack gives g8ee a separate persistent runtime volume and does not mount Gateway state into the container.

The unified deployment also mounts the Operator runtime read-only for the privileged governed-record transport. g8ee uses its app certificate for settings, data, blob, pub/sub, health, and event traffic, and uses the shared Operator certificate only for direct governance submissions. This credential sharing is a first-party deployment mechanism, not part of the public app enrollment contract.

See [Authentication and Authorization](./auth.md) for platform identity and [Unified Docker Stack](../guides/unified_stack.md) for the owner approval and startup sequence.

## Application State and Telemetry

g8ee owns conversation history, cases, investigations, generated memories, attachments, retry state, Tribunal and reputation records, and model telemetry. The Gateway is not implicit agent memory. Application reads use the Gateway-backed data transports, while designated governed writes use the direct-envelope path described above.

Each model call records provider and model identity, monotonic timing, provider-reported token usage when available, retry and finish metadata, canonical input and output hashes, and a hash-bound privacy attestation. The analytical telemetry stores scanner identity, sensitive-occurrence counts, and detected types rather than the detected values. Conversation content, attachments, prompts, and model outputs remain application data and can contain user-supplied or sensitive content.

The standalone evaluation package consumes normalized model telemetry and governance receipt evidence but is not part of the running g8ee service. See [Evals](../ensemble/evals.md) for evidence collection and verification, and [Ensemble Tests](../ensemble/tests.md) for the Python test tiers and commands.

## Security Properties and Limits

- g8ee does not open a management path to a target host. Host operations execute only through the bound Operator.
- The Gateway binds a relayed command to an exact Operator and session instead of broadcasting it.
- The target Operator independently applies L1 through L4 before L5 execution, even though g8ee generated the intent.
- Tribunal agreement, Auditor approval, reputation, and application Warden output remain advisory to protocol governance.
- Application approval and auto-approval do not satisfy protocol L3.
- Direct-envelope and command-relay paths fail when the active posture requires proofs they do not supply.
- SSE events report progress and outcomes but do not authorize execution or alter the governance state root.
- The governance boundary covers operations sent through g8e. It does not attest to model-provider behavior or activity through any side channel outside these transports.

## Related Documentation

- [AI Agents and the g8e Governance Boundary](./agents.md): Client integration paths and the distinction between application agents and protocol governance.
- [Governance](./governance.md): Five-layer verification, receipts, and posture behavior.
- [Operator Architecture](./operator.md): Remote Operator verification, execution, results, and audit.
- [SSE Streaming](./sse.md): Approval and application event delivery.
- [Authentication and Authorization](./auth.md): Workload identities, platform enrollment, and human authorization.
- [g8ee Agents](../ensemble/agents.md): Triage, Dash, Sage, Tribunal, Auditor, Warden, and support agents.
- [Build Apps](../guides/build_apps.md): Public integration and enrollment choices for third-party applications.
- [Unified Docker Stack](../guides/unified_stack.md): Deployment and startup workflow for Gateway, Operator, dashboard, and g8ee.
