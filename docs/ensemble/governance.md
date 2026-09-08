# Governance

## Scope

The g8e Agentic Ensemble (`g8ee`) is an optional first-party client of the g8e governance platform. It generates and evaluates operational intent, but it remains outside the trusted execution boundary. Tribunal agreement, Auditor review, application risk classification, and user approval inside g8ee do not authorize a platform transaction by themselves.

An operation becomes governed when g8ee submits typed intent through a g8e ingress. The Gateway constructs or accepts a canonical `GovernanceEnvelope`, and the executing Warden and Actuator apply the verification required by the envelope's posture. Native client tools, direct filesystem access, network access, and other paths that do not traverse a g8e ingress are outside this boundary.

See [AI Agents and the g8e Governance Boundary](../architecture/agents.md) for all supported agent integration paths and their limits.

## Trust and Execution Boundaries

The **Governance Gateway** is the Policy Decision Point. It authenticates clients, enforces transport and channel authorization, owns the active governance posture and current state root, constructs envelopes for intent-based client protocols, coordinates L2 consensus and L3 approval on supported ingress paths, and routes work to an execution site.

The **Governed Operator** is the Policy Execution Point on a managed host. It opens an outbound mTLS connection to the Gateway, receives envelopes for its exact Operator session, verifies each envelope locally, and executes accepted operations through the L5 Actuator. The Gateway also has an in-process Operator substrate for locally executed MCP, A2A, and direct-envelope operations.

The posture is embedded in each envelope so the executing Warden applies the Gateway-selected policy. Missing or invalid posture metadata fails closed.

## Application Controls Before Governance

For host command generation, g8ee applies application-level controls before publishing intent to the platform. These controls improve command quality and reduce unsafe proposals, but they are not protocol governance proofs.

### Intent and command separation

Sage describes the requested outcome without supplying shell syntax. Five Tribunal members independently generate candidate commands from that intent and the available Operator context. Each candidate is normalized and passes deterministic command-safety checks before voting.

### Tribunal voting

Tribunal members have equal vote weight. A candidate reaches the minimum threshold when at least two members produce the same normalized command. If multiple candidates tie, voting first prefers the shortest command and then a candidate without Nemesis support. A remaining tie or a round with no two matching commands triggers a second generation round using anonymized first-round clusters; failure to reach agreement in the second round stops command generation.

Tribunal voting is application-level model agreement. It does not produce the Ed25519 signatures required by platform L2 Consensus and cannot satisfy a `consensus` or `notary` posture.

### Command risk and audit

When a response analyzer is configured, the command-risk Warden evaluates the winning command before the Auditor. An unavailable model, empty response, analysis error, or inconclusive command-risk result inside that analyzer becomes `HIGH` risk and blocks the command. The first high-risk result returns contextual feedback so Sage can propose a safer alternative; a second high-risk result for the same investigation reports an agent conflict and requires human intervention. If no response analyzer is configured, g8ee skips this stage.

When enabled, the Auditor reviews the winning command and anonymized alternatives after command-risk analysis. It can accept the winner, revise it, or select another candidate. A successful audit creates a reputation commitment; failure to create that commitment stops the verdict.

File writes and replacements use a separate file-risk analysis and g8ee approval flow before dispatch. A file-risk analysis failure is logged and the operation continues to the approval gate. Error analysis classifies failed commands and controls bounded retry or escalation. These application approval and retry decisions are distinct from L3 Notary authorization.

See [Agents](agents.md) for the complete persona roster and [Architecture](architecture.md) for the ensemble workflow.

## Platform Verification

Every governed operation reaches the same L4 and L5 boundary. The logical interlock is:

```text
L1 Doctrine -> L2 Consensus -> L3 Notary -> L4 Warden -> L5 Actuator
```

- **L1 Doctrine** decodes the typed protobuf payload, applies field constraints and forbidden-pattern rules, and runs MITRE ATT&CK-oriented threat detection. L1 is mandatory in every posture and executes as part of Warden verification.
- **L2 Consensus** verifies Ed25519 votes over the transaction hash and decision against an enabled policy and trusted member keys. Required postures enforce quorum from distinct affirmative signers. Application Tribunal votes have no authority at this layer.
- **L3 Notary** verifies human authorization for mutation-classified actions under `ratify` and `notary`. Supported Gateway MCP and A2A flows can suspend a transaction for WebAuthn approval. Outbound execution verifies the corresponding approved suspended transaction and Ed25519 proof. Read-only actions do not require L3.
- **L4 Warden** reserves the nonce for replay prevention, checks expiry, validates the action type and payload, recomputes the transaction hash, verifies the current state root, runs L1, and evaluates posture-required L2 and L3 evidence. A failed universal check or required proof prevents dispatch.
- **L5 Actuator** signs and persists an `EXECUTING` receipt before invoking the action handler, appends a signed commitment when the SQL commitment ledger is active, rehydrates scrubbed payload data at the execution site, mints a transaction-bound capability, invokes the handler, dissolves the capability, and signs and persists the final result and persistence attestation.

See [Governance Pipeline](../architecture/governance.md) for the full platform transaction flow.

## Governance Postures

The Gateway posture is selected at startup with `--posture <doctrine|consensus|ratify|notary>`. L1 and the universal L4 checks remain mandatory in every posture.

| Posture | L1 | Protocol L2 | L3 for mutations |
| --- | --- | --- | --- |
| `doctrine` | Required | Not required | Not required |
| `consensus` | Required | Required | Not required |
| `ratify` | Required | Not required | Required |
| `notary` | Required | Required | Required |

Optional L2 and L3 evidence is verified when the required verifier is available and recorded when valid, but its absence does not gate a posture that does not require it. Platform enrollment bootstrap actions are exempt from the L2 requirement that they establish.

The ingress path matters. A posture does not cause every transport to acquire missing proofs automatically. g8ee must use a path that coordinates the required proofs or submit an envelope that already contains them.

## Host Operations Through `CommandIntent`

The ensemble uses `CommandIntent` for host commands, file operations, filesystem reads, log and history queries, and other outbound Operator work:

1. g8ee serializes the typed Operator protobuf payload and publishes a `CommandIntent` to the exact `cmd:<operator_id>:<operator_session_id>` channel.
2. The Gateway authenticates the app publisher, enforces the channel ACL, checks that the intent targets the same Operator and session as the channel, and validates that the session is active.
3. The Gateway adds the current state root, posture, nonce, expiry, transport-derived app identity, requestor identity, and application context, then computes the canonical transaction hash and publishes the resulting `GovernanceEnvelope` to the Operator.
4. The Operator runs L1 and L4 verification locally. Accepted operations execute through L5, while rejected operations do not reach the action handler.
5. The Operator publishes the command result to the result channel and relays its signed receipt to the Gateway. The local receipt is authoritative; the Gateway mirror is best-effort.

The `CommandIntent` relay does not perform protocol L2 deliberation or L3 suspension and does not attach L2 votes or an L3 proof. Consequently:

- `doctrine` accepts otherwise valid read and mutation intents without L2 or L3.
- `consensus` rejects ordinary relayed intents because they do not contain protocol L2 votes.
- `ratify` accepts otherwise valid read-only intents but rejects mutation intents without an L3 proof.
- `notary` rejects ordinary relayed intents because L2 is required, and mutations also require L3.

Use Gateway MCP or A2A when the Gateway must coordinate protocol consensus or human approval. See [Build Apps](../guides/build_apps.md) for integration-path selection.

## Direct Governance Envelopes

For governed platform records such as cases, investigations, memories, and agent activity, g8ee uses its `GovernanceClient` to submit a complete envelope to the synchronous governance endpoint. This is a privileged, Operator-credential path in the unified deployment, not the normal public app ingress.

The client obtains the current state root, serializes the typed payload, generates replay and expiry fields, binds requestor and acting-app attribution into the transaction hash, and submits canonical JSON over mTLS. The Gateway binds the envelope identity to the certificate SPIFFE identity, supplies the active posture when the envelope omits it, and sends the envelope through the in-process Warden and Actuator.

The client serializes submissions to reduce state-root races. If the Gateway rejects a submission because another transaction changed the state root, the client fetches the new root, rebuilds the envelope, and retries up to three times after the initial attempt.

This client does not acquire protocol L2 votes or perform a WebAuthn ceremony. A certificate fingerprint alone is transport metadata, not a complete L3 authorization proof. The direct mutation path therefore succeeds only when the active posture does not require proofs absent from the envelope, which is normally `doctrine` for g8ee's current platform-record submissions.

A successful submission returns a signed `ActionReceipt`. `GovernanceClient` exposes receipt-signature verification using the configured Actuator public key, but submission does not invoke that verification automatically.

## Transaction Integrity

The canonical transaction hash binds action type, target resource, typed payload, state root, nonce, expiry, structured intent, requestor identity, and acting-app identity. Both the envelope ID and transaction hash must equal the recomputed SHA-256 digest. L3 proof and posture metadata are outside this hash because L2 signs the transaction before human authorization and the Gateway supplies posture as policy metadata.

The Warden also requires a known action type, a decodable payload, a current state root, a live expiry, and a nonce that is neither reserved nor previously consumed. These checks fail closed in every posture. See [Protocol](protocol.md) for the canonical envelope and hashing rules.

## Security Properties and Limits

- g8ee model output has no authority to bypass L1, replay protection, state binding, or posture-required proofs.
- Application Tribunal voting is independent of protocol L2 and cannot substitute for trusted signer votes.
- g8ee approval prompts are independent of L3 and cannot substitute for a valid Notary proof.
- Operator command channels bind work to one authenticated Operator session; mismatched targets are dropped.
- A remote Operator independently verifies each envelope before changing its host.
- L5 signs and persists admitted execution outcomes, and the executing Operator retains the authoritative local receipt. A transaction rejected before execution does not produce an L5 receipt on the synchronous direct-envelope path.
- Signed receipts attest only to operations that traversed the governed path. They do not attest to activity performed through native tools or other client side channels.

## Related

- [AI Agents and the g8e Governance Boundary](../architecture/agents.md): Agent ingress paths, launcher behavior, and governance limits.
- [Governance Pipeline](../architecture/governance.md): Platform verification, postures, transaction flow, and receipts.
- [Gateway Architecture](../architecture/gateway.md): Gateway ingress, identity binding, consensus coordination, and approval suspension.
- [Operator Architecture](../architecture/operator.md): Outbound Operator transport, local verification, execution, and audit storage.
- [Authentication and Authorization](../architecture/auth.md): mTLS identities, delegated credentials, sessions, and WebAuthn.
- [Consensus](../architecture/consensus.md): Protocol L2 policy, enrollment, deliberation, and vote verification.
- [Agents](agents.md): g8ee personas, Tribunal members, Auditor, and application Warden.
- [Architecture](architecture.md): Ensemble components, protocol surfaces, and runtime flow.
- [Protocol](protocol.md): Ensemble-facing protocol models and transaction hashing.
- [Storage](storage.md): Ensemble data services and governed platform records.
