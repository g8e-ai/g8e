# Protocol

## Scope

g8ee is an optional application client of the g8e protocol library. It uses typed protobuf payloads, protojson-compatible JSON, SPIFFE workload identity, `CommandIntent`, `GovernanceEnvelope`, and signed `ActionReceipt` values. Its two governed outbound paths are the Operator command relay for host work and direct envelope submission for protected application-record writes. Gateway MCP and A2A are separate client ingress paths; g8ee does not use them for these internal operations.

g8ee remains outside the trusted execution boundary. Model output, Tribunal agreement, Auditor and Marshal decisions, application approval, memory, and event telemetry express application intent or status. They do not replace Gateway or Operator verification, protocol L2 signatures, L3 human authorization, or signed execution evidence. See [Platform Protocol](../architecture/protocol.md) for the protocol packages and canonical wire contracts.

## Transport Identity and Enrollment

g8ee enrolls as the owner-approved platform application `spiffe://g8e.local/app/g8ee`. Startup loads a valid app certificate or completes platform enrollment before creating the DB, KV, pub/sub, blob, event, and HTTP clients. The app certificate and its private key are used for those Gateway-facing mTLS connections, including the governance HTTP client. The Gateway derives the authenticated app identity from the certificate's SPIFFE URI SAN rather than trusting a caller-supplied identity field.

The app certificate is not an Operator certificate and does not grant unrestricted host access. Operator and Operator-session fields in application records identify the delegated execution context supplied by g8ee's request context; the Gateway separately binds the acting application to the authenticated g8ee transport identity. The direct governance endpoint is privileged and still verifies transport-to-envelope identity binding and the active governance posture.

Platform enrollment is owner-approved. g8ee generates a P-256 key and CSR, persists resumable pending state, polls the Gateway enrollment status, signs the completion transcript after approval, validates the issued chain and expected SANs, and atomically installs the credentials. The FastAPI process does not become ready while enrollment is pending or invalid. See [PKI and Trust](pki.md) for the enrollment and trust model.

## Operator Command Relay

g8ee uses the command relay for shell commands, file operations, filesystem inspection, history and log queries, port checks, and other work executed by a bound Operator. The application-side sequence is:

1. g8ee resolves an active Operator and session, validates the request through its application command and approval workflow, and registers result handling for the exact Operator session.
2. `PubSubClient.publish_command` serializes the typed Operator payload to protobuf bytes, base64-encodes those bytes, and publishes a protojson `CommandIntent` to `cmd:<operator_id>:<operator_session_id>` over the Gateway pub/sub WebSocket.
3. The Gateway authenticates the g8ee app publisher and enforces the channel ACL. It decodes the `CommandIntent`, requires its Operator and session fields and action type, checks that they match the channel, and validates that the Operator session is active.
4. The Gateway obtains the current state root, adds transport-derived app identity, requestor and application context, nonce, expiry, posture, and the transaction hash, then constructs the canonical `GovernanceEnvelope`.
5. The Gateway applies its command-relay screening and publishes the envelope only to the matching Operator session. The Operator independently performs the required L1 and L4 checks, then its L5 Actuator executes an accepted operation and produces signed evidence.
6. The Operator publishes results on `results:<operator_id>:<operator_session_id>`. It publishes signed receipts on the receipt channel; the Gateway verifies and mirrors accepted receipts. g8ee consumes the exact result channel and resolves the waiting operation by its execution identifier.

The command relay does not construct the envelope, fetch the state root, obtain missing protocol L2 votes, or suspend for L3 approval. It carries no L2 votes or L3 proof of its own. Consequently, an ordinary relayed intent is compatible with `doctrine`, read-only intents can be compatible with `ratify`, and required L2 or mutation L3 evidence causes rejection under `consensus` or `notary` unless another supported path supplies that evidence. Tribunal voting and g8ee application approval are not protocol L2 or L3 proofs.

Operator channels are session-specific. The Gateway rejects malformed intents, unauthorized publishers, channel and target mismatches, inactive sessions, and unavailable relay dependencies without broadcasting the request. A successful WebSocket publish reports only that the intent frame was written to the connection; it does not establish Gateway acceptance, Operator execution, or receipt validity.

## Direct Governance Envelopes

g8ee uses `GovernanceClient` for protected application-record writes such as cases, investigations, conversation history, memories, agent activity, reputation, and stake-resolution data. Reads remain Gateway-backed data-service operations; they do not become governed mutations merely because the app connection is authenticated.

The direct submission sequence is:

1. g8ee converts the requested write to a typed protocol payload and maps internal event and payload names to canonical protocol action and payload types.
2. `GovernanceClient` obtains the current state Merkle root when the caller did not provide one, creates a fresh nonce and five-minute expiry, and constructs a `GovernanceEnvelope` with requestor, acting-app, Operator/session, case, investigation, task, web-session, and CLI-session context.
3. The client serializes the typed payload to protobuf bytes, base64-encodes the `payload` field, computes the protocol transaction hash over the action, target, payload, state root, nonce, expiry, intent data, and identity/context fields, and submits protojson-compatible JSON by mTLS `POST /api/v1/governance/envelopes`.
4. The Gateway verifies the supplied envelope, binds its acting application and delegated execution identity to the authenticated transport, supplies the active posture when the envelope omits it, and processes the request through its in-process L4 Warden and L5 Actuator.
5. The endpoint returns the Gateway's signed `ActionReceipt` for an execution attempt. `GovernanceClient` can verify that signature using the configured Actuator public key, but receipt verification is an explicit client operation rather than an automatic part of submission.

The client may include an mTLS certificate fingerprint in the envelope's L3 metadata and may record Tribunal identifiers in L2 metadata, but it does not create protocol L2 signatures or perform a WebAuthn or signed-CLI authorization ceremony. An mTLS fingerprint is transport metadata, not a complete L3 proof. Direct writes therefore fail closed when the active posture requires evidence that the envelope does not contain; this path is not equivalent to Gateway MCP or A2A, which can coordinate supported deliberation or approval flows.

Submissions are serialized with an async lock to reduce state-root races. If the Gateway returns `TX_STATE_MISMATCH`, the client fetches a fresh state root, rebuilds the envelope, and retries up to three times after the initial attempt. Other governance rejections, malformed-envelope responses, Gateway-not-ready responses, and transport failures propagate as typed g8ee errors. A receipt returned after the Actuator is reached is the cryptographic outcome of that execution attempt.

## Canonical Envelope and Hashing

`GovernanceEnvelope` carries protocol version, ID, timestamps, expiry, source component, event and action types, target resource, typed payload, structured intent data, state Merkle root, nonce, governance metadata, requestor and acting-app attribution, delegated Operator/session context, optional case and investigation context, posture, and transaction hash. `CommandIntent` is the pre-governance form: it carries the exact target Operator and session, action type, base64-encoded serialized Operator protobuf bytes, and application context, but no state root, nonce, expiry, transaction hash, or governance proof.

The transaction hash uses SHA-256 over the non-empty fields in protocol order: action type, target resource, base64 payload, state root, nonce, normalized expiry, canonicalized intent data, requestor user ID, acting app ID, Operator ID, Operator session ID, case ID, investigation ID, task ID, web session ID, and CLI session ID. Each present field is followed by `|`. L3 proof and posture metadata are excluded from the hash: L2 must be able to sign the transaction before a human notary proof is obtained, and posture is policy metadata supplied by the Gateway.

## Five-Layer Interlock

Both outbound paths terminate at the platform's five-layer governance boundary, although the Gateway constructs the envelope for command relay while g8ee constructs it for direct application-record writes:

1. **L1 Doctrine** decodes the typed protobuf payload and applies field constraints, hard gates, forbidden-pattern matching, and MITRE ATT&CK-oriented threat detection.
2. **L2 Consensus** verifies Ed25519 votes from trusted enrolled members against the configured policy and quorum when the posture requires protocol consensus. Tribunal model agreement is not L2.
3. **L3 Notary** verifies WebAuthn or signed CLI authorization for mutation actions when the posture requires human ratification. g8ee approval and an mTLS fingerprint are not substitutes.
4. **L4 Warden** verifies action and payload validity, expiry, nonce replay protection, transaction hash, current state root, target identity, posture, and required governance evidence before dispatch.
5. **L5 Actuator** executes an accepted operation with a transaction-bound just-in-time capability and produces signed execution and persistence evidence.

| Posture | L1 | L2 | L3 for mutations |
| --- | --- | --- | --- |
| `doctrine` | Required | Not required | Not required |
| `consensus` | Required | Required | Not required |
| `ratify` | Required | Not required | Required |
| `notary` | Required | Required | Required |

The active posture determines required L2 and L3 gates. It does not cause the command relay or direct envelope client to acquire missing proofs automatically. See [Governance](../architecture/governance.md) for canonical posture behavior and rejection semantics.

## Events, Results, and Ownership

g8ee publishes progress, clarification, approval, model-output, tool-result, reputation, and background events through the Gateway event bridge. `EventService` routes events using web or CLI session fields; events without a web or CLI routing target are skipped. These events are delivery telemetry, not governance proofs, durable execution evidence, or state-root mutations.

The Gateway-backed document, KV, and blob services own durable application records. g8ee owns process-local active turns, background tasks, pending application approvals, and command-result correlations. Restarting g8ee interrupts operations waiting for a result and does not replace the executing Operator's authoritative local receipt, audit record, replay state, or file-mutation evidence. Gateway receipt mirroring and g8ee result handling are secondary copies of execution evidence.

## Failure Behavior

The command relay and direct submission fail closed on malformed protojson, invalid typed payloads, unauthorized channels or transports, identity or target mismatch, unknown actions, inactive sessions, expired envelopes, replayed nonces, stale state roots, and missing posture-required proofs. The relay drops invalid intents before Operator delivery. Direct submission returns a validation, governance, readiness, or transport error unless execution reaches the Actuator, in which case the signed receipt reports the execution attempt's outcome.

Application-level validation and approval failures occur before either protocol path and are distinct from platform L1-L5 rejection. Native client tools, unrestricted provider behavior, direct filesystem or network access, and other side channels do not become governed because g8ee also uses a governed connection.

## Related

- [Architecture](architecture.md): g8ee relationships, identity startup, state ownership, and request flow.
- [Governance](governance.md): Application controls, protocol postures, direct envelopes, and command relay behavior.
- [Platform Protocol](../architecture/protocol.md): Protocol packages, schemas, constants, and conformance testing.
- [AI Agents and the Governance Boundary](../architecture/agents.md): Platform ingress paths and their governance limits.
- [PKI and Trust](pki.md): Enrollment, certificates, SPIFFE identities, and trust bundles.
- [Operator Architecture](../architecture/operator.md): Remote verification, host execution, result delivery, and local audit.
- [Server-Sent Events](sse.md): g8ee event publication and Gateway event routing.
