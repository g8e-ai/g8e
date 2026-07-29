# Sequence: Principal → Ensemble → Gateway → Operator (v3)

First appeared in commit `8992215d`. Depicts the outbound-mode transaction flow where the Governed Operator runs as a separate process from the Governance Gateway. In gateway mode, the Operator service runs in-process and the pub/sub steps are replaced by a synchronous call to `ProcessEnvelope` (`internal/services/pubsub/pubsub_commands.go`).

```mermaid
sequenceDiagram
    autonumber
    participant Principal as Principal<br/>(Human / AI Agent)
    participant Ensemble as Agentic Ensemble<br/>(g8e-compatible / BYO / MCP client)
    participant Gateway as Governance Gateway<br/>(g8eg)
    participant Operator as Governed Operator<br/>(g8eo)

    Operator->>Gateway: Establish mTLS WebSocket connection<br/>(wss://, subscribe to cmd channel)

    Principal->>Ensemble: Submit intent (MCP / A2A / tool call)
    Note over Ensemble: Collect L2 Consensus votes (Ed25519)<br/>Wrap in signed GovernanceEnvelope
    Ensemble->>Gateway: POST envelope to /api/v1/governance/envelopes<br/>(mTLS identity binding via URI SAN)

    Gateway->>Operator: Publish GovernanceEnvelope to cmd channel

    Note over Operator: L4 Warden verifies (fail-closed):<br/>L1 Doctrine, State Root, L2 Consensus, L3 Notary<br/>L5 Actuator: mint JIT capability, dispatch, sign ActionReceipt<br/>Log to SQLAuditStore and ledger

    Operator->>Gateway: Publish scrubbed signed ActionReceipt
    Gateway->>Principal: Return signed ActionReceipt
```

## Verification Sequence

The L4 Warden (`internal/services/governance/l4_warden.go`) orchestrates all pre-dispatch checks in a fixed order. Each stage returns a typed sentinel error on failure, causing the transaction to be rejected and logged as a blocked transaction.

1. **Nonce, Expiry, Replay**: The nonce is durably reserved in SQLite before any expensive cryptography. Expired or replayed nonces are rejected immediately.
2. **L1 Doctrine**: Stateless validation in `internal/services/governance/l1_doctrine.go`. Checks protobuf `forbidden_patterns` field extensions and performs MITRE-based threat detection on command, MCP, A2A, and file-edit payloads.
3. **State Root**: Stateful validation comparing `envelope.StateMerkleRoot` against the current root from `StateRootProvider` (`internal/services/gateway/state_root_service.go`). The bound root incorporates config, governance, filesystem mutations, and the token keymap hash.
4. **L2 Consensus**: Posture-gated Ed25519 signature verification against consensus policy. Verifies quorum of distinct consensus member signatures over the transaction hash.
5. **L3 Notary**: Posture-gated human-presence verification (`internal/services/governance/l3_notary.go`). Runs only after L2 passes, preserving the invariant that a human is never asked to authorize content the machines have not vetted.

## L5 Actuator Execution

L5 Actuator (`internal/services/governance/l5_actuator.go`) is the single execution boundary for verified transactions. The execution sequence is:

1. Sign initial `ActionReceipt` (intent to execute); fail-closed if signing or audit logging fails.
2. Rehydrate scrubbed payload if `ScrubbingService` is available.
3. Mint a just-in-time capability scoped to the single transaction, bound to the transaction hash.
4. Dispatch to the registered `ExecutionHandler` via MCP or A2A.
5. Dissolve the capability immediately after execution (zero standing privileges).
6. Sign the final `ActionReceipt` with execution result and state root after.
7. Log the final receipt to `SQLAuditStore` and `ConsoleAuditStore`.

## Governance Posture

The `GovernancePosture` interface (`internal/services/governance/posture.go`) determines which layers are enforced as fail-closed gates versus audited:

- **doctrine**: L1 enforced; L2 and L3 audited but do not gate execution.
- **consensus**: L1 and L2 enforced; L3 audited but does not gate execution.
- **notary**: L1, L2, and L3 all strictly enforced as fail-closed gates.

## Gateway Mode (In-Process Operator)

In gateway mode (`internal/cli/cmd/gateway.go`), the `OperatorPubSubService` is constructed in-process and wired as the envelope processor via `SetEnvelopeProcessor`. BYO clients POST `GovernanceEnvelope` messages to `/api/v1/governance/envelopes` (`internal/services/gateway/governance_controller.go`), which calls `ProcessEnvelope` synchronously. The mTLS identity binding (`verifyEnvelopeIdentityBinding`) verifies that the client certificate's URI SANs match the envelope's identity claims before processing. The receipt is returned as HTTP 200 JSON, even for execution failures, because a signed `FAILED` receipt is cryptographic evidence of the attempt.
