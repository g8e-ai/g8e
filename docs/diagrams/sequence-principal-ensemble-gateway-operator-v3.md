# Sequence: Principal → Ensemble → Gateway → Operator (v3)

Depicts the outbound-mode transaction flow where the Governed Operator runs as a separate process from the Governance Gateway. This is the path used by g8ee for host commands via `POST /api/v1/operators/commands`. In gateway mode, MCP/A2A ingress uses the embedded Operator substrate and replaces the pub/sub publish step with a synchronous `ProcessEnvelope` call (`internal/services/pubsub/pubsub_commands.go`).

```mermaid
sequenceDiagram
    autonumber
    participant Principal as Principal<br/>(Human)
    participant Ensemble as Agentic Ensemble<br/>(g8ee)
    participant Gateway as Governance Gateway<br/>(g8eg · PDP)
    participant Operator as Governed Operator<br/>(g8eo · PEP)

    Operator->>Gateway: Establish outbound mTLS WebSocket<br/>(subscribe cmd:&lt;id&gt;:&lt;session&gt;)

    Principal->>Ensemble: Submit chat turn / host-command intent
    Note over Ensemble: Tribunal · command validation ·<br/>application approval (not protocol L2/L3)
    Ensemble->>Gateway: POST /api/v1/operators/commands<br/>(registered event_type · target session · mTLS app identity)

    Note over Gateway: L1 screening · state root · envelope construction<br/>L2/L3 coordination when posture requires and path supports it
    Gateway->>Operator: Publish GovernanceEnvelope to cmd channel

    Note over Operator: L4 Warden verifies (fail-closed):<br/>L1 Doctrine · hash · nonce · expiry · state root · L2/L3<br/>L5 Actuator: commitment · JIT capability · dispatch · signed receipt

    Operator->>Gateway: Publish signed ActionReceipt to receipts channel
    Gateway->>Ensemble: Return correlated dispatch result (HTTP 200)
    Ensemble->>Gateway: POST /api/v1/sse/push (application telemetry)
    Gateway->>Principal: SSE delivery to bound web/cli session
```

## Alternate Ingress Paths

| Caller | Surface | Gateway behavior | Operator path |
| --- | --- | --- | --- |
| **MCP / A2A client** | `/mcp`, `/api/v1/a2a/call` | Gateway constructs envelope; may deliberate L2 or suspend for L3 | Embedded in-process Operator, or downstream MCP/A2A service |
| **g8ee host command** | `POST /api/v1/operators/commands` | Gateway constructs envelope from registered event; no missing-proof synthesis | Outbound Operator on named session |
| **g8ee protected record** | `POST /api/v1/governance/envelopes` | Gateway verifies supplied envelope; caller must include required proofs | Embedded in-process Operator |
| **CLI direct envelope** | `POST /api/v1/governance/envelopes` | Same as protected-record path with CLI/Operator transport identity | Embedded in-process Operator |

## Verification Sequence

The L4 Warden (`internal/services/governance/l4_warden.go`) orchestrates all pre-dispatch checks in a fixed order. Each stage returns a typed sentinel error on failure, causing the transaction to be rejected and logged as a blocked transaction.

1. **Nonce, Expiry, Replay**: The nonce is durably reserved in SQLite before any expensive cryptography. Expired or replayed nonces are rejected immediately.
2. **L1 Doctrine**: Stateless validation in `internal/services/governance/l1_doctrine.go`. Checks protobuf `forbidden_patterns` field extensions and performs MITRE-based threat detection on command, MCP, A2A, and file-edit payloads.
3. **State Root**: Stateful validation comparing `envelope.StateMerkleRoot` against the current root from `StateRootProvider` (`internal/services/gateway/state_root_service.go`). The bound root incorporates config, governance, filesystem mutations, and the token keymap hash.
4. **L2 Consensus**: Posture-gated Ed25519 signature verification against consensus policy. Verifies quorum of distinct consensus member signatures over the transaction hash.
5. **L3 Notary**: Posture-gated human-presence verification (`internal/services/governance/l3_notary.go`). Runs only after L2 passes when both are required, preserving the invariant that a human is never asked to authorize content the machines have not vetted.

## L5 Actuator Execution

L5 Actuator (`internal/services/governance/l5_actuator.go`) is the single execution boundary for verified transactions. The execution sequence is:

1. Sign initial `ActionReceipt` (`EXECUTING`); fail-closed if signing or audit logging fails.
2. Append a signed `CommitmentAttestation` to the local SQLite commitment chain when configured.
3. Rehydrate scrubbed payload if `ScrubbingService` is available.
4. Mint a just-in-time capability scoped to the single transaction, bound to the transaction hash.
5. Dispatch to the registered `ExecutionHandler` via MCP or A2A.
6. Dissolve the capability immediately after execution (zero standing privileges).
7. Sign the final `ActionReceipt` with execution result and state root after.
8. Log the final receipt to `SQLAuditStore` and `ConsoleAuditStore`, with receipt-persistence attestation when configured.
9. Publish the final receipt to `receipts:<operator-id>:<operator-session-id>` on a best-effort basis.

## Governance Posture

The `GovernancePosture` interface (`internal/services/governance/posture.go`) determines which layers are enforced as fail-closed gates versus audited:

- **doctrine**: L1 enforced; L2 and L3 audited but do not gate execution.
- **consensus**: L1 and L2 enforced; L3 audited but does not gate execution.
- **ratify**: L1 and L3 enforced for mutations; L2 audited but does not gate execution.
- **notary**: L1, L2, and L3 all strictly enforced as fail-closed gates for mutations.

## Gateway Mode (In-Process Operator)

In gateway mode (`internal/cli/serve/gateway.go`), the `OperatorPubSubService` is constructed in-process and wired as the envelope processor via `SetEnvelopeProcessor`. MCP/A2A and direct envelope clients on Gateway-local paths call `ProcessEnvelope` synchronously. The mTLS identity binding (`verifyEnvelopeIdentityBinding`) verifies that the client certificate's URI SANs match the envelope's identity claims before processing. The receipt is returned as HTTP 200 JSON, even for execution failures, because a signed `FAILED` receipt is cryptographic evidence of the attempt.
