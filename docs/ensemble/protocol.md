# Protocol

## Scope

g8ee uses the g8e protocol library for typed messages, canonical JSON, workload identity, governance envelopes, and signed execution receipts. It uses two governed operation paths: a command relay for work on a bound Operator and direct envelope submission for designated application records. The Gateway MCP and A2A interfaces are separate public integration paths and are not part of g8ee's internal transport.

g8ee remains outside the trusted execution boundary. Model output, Tribunal agreement, application approval, and application Warden results can shape intent, but they do not replace protocol consensus, human authorization, Operator verification, or signed receipts. See [Platform Protocol](../architecture/protocol.md) for the protocol packages and canonical wire contracts.

## Transport Identity

g8ee enrolls as the reserved app workload `spiffe://g8e.local/app/g8ee`. It uses that app certificate for Gateway data services, pub/sub, session validation, health checks, and application event delivery. The Gateway derives the acting application identity from the authenticated transport rather than trusting a caller-supplied identity claim.

Direct governance submission is a privileged transport. In the unified deployment, g8ee uses the enrolled Operator certificate mounted read-only for this path because its app certificate cannot access the privileged route. The Gateway binds each submitted mutation to the authenticated Operator identity before processing it.

See [PKI and Trust](pki.md) for g8ee enrollment and [Authentication and Authorization](../architecture/auth.md) for platform workload identities.

## Host Operation Relay

g8ee uses the command relay for shell commands, file operations, filesystem inspection, history access, port checks, and other work executed by a bound Operator:

1. g8ee validates the request context and registers for results from the exact Operator and session.
2. The application serializes the typed Operator request and publishes a `CommandIntent` for that exact target.
3. The Gateway authenticates the app publisher, verifies that the channel and intent name the same Operator session, and confirms that the session is active.
4. The Gateway constructs the canonical `GovernanceEnvelope`. It adds the authenticated app identity, current state root, active posture, nonce, expiry, transaction hash, and application context before relaying the envelope.
5. The target Operator applies L1 through L4 verification. Its L5 Actuator executes an accepted operation and produces signed audit evidence.
6. The Operator publishes a correlated result for the same Operator session. g8ee validates the result envelope and resolves the waiting tool operation by execution identifier.

The relay does not obtain missing L2 consensus votes or L3 human authorization proofs. Tribunal voting is application-level reasoning rather than Ed25519 protocol consensus, and a g8ee approval prompt is not WebAuthn or signed CLI authorization. A relayed operation fails closed when the active posture requires evidence that the constructed envelope does not contain.

Operator channels are scoped to an exact Operator and session. The Gateway rejects unauthorized publishing and subscription, mismatched targets, malformed typed intent, and inactive sessions rather than broadcasting or forwarding the request.

## Governed Application Records

g8ee uses direct envelopes for designated record mutations, including cases, investigations, memories, agent activity, and reputation data:

1. The application converts the requested mutation to its typed protocol payload.
2. g8ee reads the current Gateway state root and constructs a canonical `GovernanceEnvelope` containing the request identity, target, nonce, expiry, and typed payload.
3. g8ee computes the transaction hash over the protocol-defined intent fields and submits canonical JSON over the privileged mTLS transport.
4. The Gateway binds the envelope to the authenticated Operator identity, supplies its active posture when the envelope omits it, and passes the request to its local Warden and Actuator.
5. The Gateway returns the signed `ActionReceipt` for the verified execution attempt. A receipt can report either successful or failed handler execution.

The direct route verifies the evidence supplied in the envelope but does not create missing L2 votes or L3 authorization. A certificate fingerprint alone is not a complete L3 proof; signed CLI authorization also requires a transaction signature, while browser authorization uses WebAuthn. Direct mutations therefore fail closed when the active posture requires evidence g8ee did not supply.

g8ee serializes direct submissions to reduce state-root races. If another accepted mutation changes the state root before verification, g8ee fetches the new root, rebuilds the envelope, and retries the state-mismatch rejection up to three times.

## Five-Layer Interlock

Both g8ee operation paths terminate at the platform's five-layer governance boundary:

1. **L1 Doctrine** validates the typed payload and applies hard gates, forbidden-pattern matching, and MITRE ATT&CK-oriented threat detection.
2. **L2 Consensus** verifies Ed25519 votes from enrolled members against the configured policy and quorum when the posture requires consensus.
3. **L3 Notary** verifies WebAuthn or signed CLI authorization for mutations when the posture requires human ratification.
4. **L4 Warden** verifies expiry, nonce replay protection, transaction hash, current state root, target identity, and posture-required evidence before dispatch.
5. **L5 Actuator** executes the accepted operation with a transaction-bound just-in-time capability and produces signed execution and persistence evidence.

The active posture determines whether L2 and L3 are required gates. See [Governance](../architecture/governance.md) for posture behavior, proof verification, rejection evidence, and receipt semantics.

## Data and Event Traffic

g8ee reads settings and application state through authenticated Gateway data services and stores attachments through blob storage. Designated mutations use the direct-envelope path; read and delivery traffic does not become authorized merely because it uses an authenticated application connection.

Progress, clarification, approval, model output, tool results, and reputation updates use the Gateway event bridge. These events are session-scoped delivery telemetry, not governance proofs, and they do not change the platform state root. Events without a web or CLI routing session are not delivered.

The target Operator retains the authoritative local execution receipt and audit record. Gateway receipt mirroring and g8ee result handling do not replace that local evidence. Active result correlations are process-local, so restarting g8ee interrupts operations that are still waiting for a result.

## Failure Behavior

Both transports fail closed on malformed canonical JSON, invalid typed payloads, identity mismatch, unknown actions, expired envelopes, replayed nonces, stale state roots, or missing posture-required proofs. The command relay drops an invalid intent before delivery. Direct submission returns a validation or governance error unless execution reached the Actuator, in which case the signed receipt is the authoritative outcome.

A successful command publish only confirms that g8ee wrote the intent frame to its Gateway WebSocket connection. It does not prove that the Gateway accepted or delivered the intent, or that the Operator executed it. Execution is established by the correlated result and signed Operator receipt.

## Related

- [Architecture](architecture.md): g8ee relationships, conversation flow, state ownership, and security limits.
- [Platform Protocol](../architecture/protocol.md): Protocol packages, schemas, constants, and conformance testing.
- [AI Agents and the Governance Boundary](../architecture/agents.md): MCP, A2A, command relay, and direct-envelope integration paths.
- [Governance](../architecture/governance.md): Five-layer verification, postures, and receipts.
- [Operator Architecture](../architecture/operator.md): Remote verification, host execution, result delivery, and local audit.
- [Server-Sent Events](../architecture/sse.md): Application event routing and approval notifications.
