---
title: g8e Operator
parent: Architecture
---

# g8e Operator

Last Updated: 2026-09-23
Version: v2.1.12

The Governed Operator is the Policy Execution Point (PEP) for the runtime in which the Operator process runs. The reference implementation is the `g8e` binary started with `g8e operator start`. It receives governed `GovernanceEnvelope` transactions from a Gateway over an outbound-only mTLS WebSocket connection, verifies each transaction locally, executes accepted typed actions through the L5 Actuator, and stores authoritative local execution evidence.

The Operator is not the Gateway and does not receive inbound management connections. The Gateway is the Policy Decision Point (PDP): it authenticates ingress, owns platform coordination state and PKI, constructs or admits envelopes, coordinates L1-L3 according to the active posture, and publishes work to an exact Operator session. The Operator independently performs the L4/L5 execution path and does not trust the fact that the Gateway sent a message as authorization.

This document describes the outbound Operator. The Gateway also has an embedded in-process Operator substrate, but that substrate executes only against the Gateway process runtime. See [Gateway Architecture](./gateway.md) for the distinction and [Connect Operator to Gateway](../guides/connect_operator_to_gateway.md) for deployment procedure.

## Runtime and trust boundaries

A remote Operator is sovereign only for the runtime visible to its process. Its filesystem, process table, services, network, and container runtime are those of that runtime; an Operator container is not automatically the Docker host. In the root Compose deployment, `g8e-gateway` and `g8e-operator` are separate containers with separate process and network namespaces and separate named volumes. Neither receives host-root, host-PID, host-network, or Docker-socket access by default. The Gateway's embedded Operator targets the Gateway container in that deployment; the outbound Operator targets the Operator container.

The Operator opens the connection to the Gateway and exposes no inbound MCP, A2A, or command listener. The Gateway publishes commands to a channel bound to the Operator ID and Operator session ID. The Operator publishes heartbeats, command results, and signed receipt projections back to Gateway-owned channels. SSE is delivery telemetry, not authorization or durable governance state.

The Operator's workload certificate carries a SPIFFE URI SAN in the `g8e.local` trust domain. The normal remote identity format is `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`. The Gateway validates certificate identity, session binding, and revocation for authenticated requests and WebSocket handshakes. See [Network Architecture](./network.md) and [Authentication and Authorization](./auth.md) for the PKI and identity contracts.

## Enrollment and startup

`g8e operator start` runs in the foreground. It creates the `.g8e/` runtime tree, loads an explicit or installed trust bundle and Operator certificate/key, and initializes the local services required for execution. With `--endpoint` and no installed Operator credentials, it fetches the Gateway trust bundle over the Gateway discovery HTTP listener and starts the owner-approved platform enrollment flow. The flow creates Operator and companion CLI CSRs, persists resumable pending state, waits for owner approval, verifies the completion transcript, and atomically writes the issued credentials. A restart from the same launch directory resumes the pending request.

After credentials are available, the worker connects to the Gateway over mTLS, obtains bootstrap information including its Operator and session identity, and subscribes to an exact command channel:

```text
cmd:<operator-id>:<operator-session-id>
```

It publishes an immediate heartbeat and continues at the configured interval, which defaults to 30 seconds. The worker retries a closed pub/sub connection with bounded backoff. It initializes encrypted local storage and execution services before accepting governed work. The execution vault is required by the outbound startup path; setting `--execution-vault=false` fails closed during initialization.

The main startup options are:

| Option | Runtime behavior |
| --- | --- |
| `-e, --endpoint <host>` | Gateway discovery host; defaults to `localhost` when omitted. |
| `--cert <path>` and `-k, --key <path>` | Explicit Operator client certificate and matching private key. |
| `--trust-bundle <path>` | Explicit Gateway CA bundle; otherwise the installed runtime bundle or endpoint discovery is used. |
| `--working-dir <path>` | Working directory used by governed command execution; it does not relocate the `.g8e/` runtime tree. |
| `-c, --cloud` and `--provider <aws\|gcp\|azure>` | Enables cloud Operator configuration and records the selected provider. |
| `-s, --execution-vault` | Enables the execution vault; it is enabled by default and required for outbound startup. |
| `-G, --no-git` | Disables Git integration for the file ledger while retaining encrypted audit storage. |
| `--heartbeat-interval <seconds>` | Sets heartbeat frequency; the default is 30 seconds. |
| `--inference-enabled` | Enables the governed inference backend for an inference Operator. |
| `--provider-boundary-observer-enabled` | Enables read-only provider-boundary observation. |
| `--provenance-operator-enabled` and `--model-storage-root <path>` | Enables storage-side model provenance attestation over the selected local model tree. |
| `--lattice-*` | Configures the optional Lattice adapter. The current CLI exposes these flags, but the adapter task handler does not provide a completed governed task-dispatch integration. |

The inference, Observer, Provenance, and Lattice behavior is specialized configuration, not a replacement for the Operator's general governance path. See [Evaluations](./evals.md) and [Model Provenance](./model-provenance.md) for the evaluation roles. Use `./g8e operator start --help` as the complete command-surface reference.

## Command and receipt channels

The Gateway's client-facing MCP, A2A, CLI, and enrolled-app HTTP dispatch paths are distinct ingress paths. For governed operator dispatch, an enrolled application posts a registered request `event_type` and serialized protobuf payload to `POST /api/v1/operators/commands`. The Gateway validates the event against the registry, derives `action_type`, validates the target session, obtains the current Gateway state root, applies L1 screening, sets identity and correlation fields, adds posture and transaction metadata, and publishes a canonical protojson `GovernanceEnvelope` to the matching `cmd:<operator_id>:<operator_session_id>` session. WebSocket publishers cannot inject `CommandIntent` on `cmd:` channels; outbound Operators subscribe there and receive Gateway-constructed envelopes only. The dispatch path does not synthesize missing L2 votes or L3 proofs.

The Operator decodes only the typed envelope format supported by the current protocol and rejects malformed JSON, unknown action types, missing payloads, and invalid identity or channel binding. Operator-originated heartbeat and result messages are not transformed by the Gateway broker. Receipt publications use a separate `receipts:<operator-id>:<operator-session-id>` channel. The Gateway verifies the signed `ActionReceipt` against the Operator's registered Actuator key and mirrors accepted receipts to its SQL audit store. That mirror is best effort; the Operator's local persisted receipt remains authoritative.

The CLI-directed `operator run` path is a separate Gateway route. It lets an enrolled CLI target one or more active sessions, while the Gateway remains the envelope-construction authority and waits for a terminal result per target. `operator stop` is also a governed command sent only to the selected remote session. The Gateway marks a target stopped only after the exact session acknowledges the shutdown. Revocation is different from stopping: revocation invalidates the workload identity, deactivates its sessions, disconnects matching pub/sub channels, and is terminal for that enrollment.

## Five-layer execution boundary

The complete posture and proof behavior is canonical in [Governance](./governance.md). The following describes the Operator-side responsibility without duplicating that document.

### L1 Doctrine, L2 Consensus, and L3 Notary

The Gateway owns or coordinates the Policy Decision Point work for L1-L3 on its client-facing construction paths. A remote Operator does not assume that upstream screening is sufficient. Its L4 Warden decodes the typed payload and runs the local L1 Doctrine validator, then verifies the L2 and L3 evidence required by the posture carried in the envelope.

The envelope is the authoritative source of posture for Operator-side gating. `doctrine` requires L1 and audits L2/L3; `consensus` requires L1/L2 and audits L3; `ratify` requires L1 and L3 for mutation action types; `notary` requires L1/L2 and L3 for mutation action types. A command relay does not add missing protocol proofs, so a relay path must satisfy the selected posture with the evidence it carries.

### L4 Warden

`L4Warden.VerifyEnvelope` performs the pre-dispatch checks in this order:

1. Tracks the nonce in process and reserves it in the durable replay store before expensive validation.
2. Checks that expiry and nonce fields are present and that the transaction is not expired or replayed.
3. Validates the known action type, decodes its typed protobuf payload, and runs local L1 Doctrine validation.
4. Recomputes the transaction hash and requires it to match both `transaction_hash` and the envelope `id`.
5. Fetches the current state root from the configured provider and requires it to match the envelope state Merkle root. In outbound mode this provider obtains the Gateway state root; the Operator's local ledger root is not substituted for the Gateway root.
6. Reads the envelope posture and verifies posture-required L2 and L3 evidence, including the trusted signer and notary checks.

A rejected transaction does not reach the handler. The Warden produces deterministic stage evidence and releases a nonce reservation when validation fails. When the Actuator and its audit dependencies are available, the rejection is recorded as a signed failed receipt.

### L5 Actuator

`L5Actuator` is the single execution boundary for a verified transaction. It does not re-run L2 or L3; those checks belong to L4, whose `VerifiedTransaction` result it accepts. Before invoking the handler, L5:

1. Builds, signs, and persists an `EXECUTING` `ActionReceipt`. Signing or initial audit failure prevents execution.
2. Appends a signed `CommitmentAttestation` to the local SQLite commitment chain when the SQL audit store is enabled.
3. Rehydrates locally tokenized payload values through the scrubbing service.
4. Mints a transaction-bound, short-lived capability and places it in the execution context.
5. Dispatches the typed action to the registered execution handler.
6. Dissolves the capability after the handler returns, including when the handler fails.
7. Captures the resulting state root, signs and persists a final `COMPLETED` or `FAILED` receipt, and records receipt-persistence evidence.
8. Publishes the final signed receipt to the Gateway on a best-effort basis.

The Actuator returns execution errors after final receipt processing. The host-local receipt and commitment record remain the authoritative execution evidence even when publication to the Gateway fails.

## Local state and native actions

The Operator's `.g8e/` runtime tree contains local PKI material and enrollment state, encrypted SQLite data including replay protection and audit records, the execution-vault key and state, and optional Git-backed file-ledger data. Runtime paths are owned by the Operator process and its local volume; they are not shared with the Gateway unless deployment configuration explicitly mounts them.

The reference binary registers 32 native tools in the MCP service. The catalog includes database triage, log digestion, process and resource inspection, network and TLS checks, system introspection, file operations, cloud and Kubernetes inspection, Git operations, shell execution, Operator deployment, and governed audit-receipt queries. The native catalog is compiled into the binary and is not evidence that an arbitrary external MCP server is governed.

Native tools are dispatched only after the request has crossed a governed Gateway ingress or arrived as a complete envelope and passed the applicable verification path. An external MCP wrapper that forwards requests directly to another MCP server is a separate integration path and does not gain L2-L5 governance or signed Operator receipts merely by running alongside g8e. Client-native tools, direct filesystem access, unrestricted network access, and other side channels remain outside this boundary. See [AI Agents and the g8e Governance Boundary](./agents.md).

## Evidence and limitations

The Operator stores signed `ActionReceipt` records in its local SQL audit store. Receipts contain transaction identity, execution status, state roots, L2/L3 status, signer information, and deterministic stage evidence. The commitment ledger independently chains admitted execution commitments. Governed file operations may also use the optional Git-backed file ledger. These stores are local to the executing runtime; Gateway copies are coordination mirrors or separately owned Gateway evidence.

The Operator is not a general host-isolation mechanism. It can constrain governed handlers and reject unauthorized protocol transactions, but it does not sandbox the AI client or prevent activity performed through tools and channels outside g8e. Its observation and execution scope is limited to the process runtime and permissions granted by the deployment.

The protocol packages expose schemas and identity helpers, not a reusable reference Operator SDK or an independent conformance command. An independent implementation must reproduce canonical protojson handling, enrollment and scoped pub/sub, transaction hashing, replay and expiry checks, state-root verification, posture-aware proof verification, signed receipts, local persistence, scrubbing, and typed dispatch to claim behavioral compatibility.

## Related documentation

- [Gateway Architecture](./gateway.md): Gateway PDP responsibilities, embedded Operator runtime, and deployment boundaries.
- [Governance](./governance.md): Canonical five-layer and posture behavior.
- [Network Architecture](./network.md): PKI hierarchy, SPIFFE identities, mTLS, and channel transport.
- [Authentication and Authorization](./auth.md): Enrollment, CLI sessions, revocation, and approval.
- [Storage Architecture](./storage.md): Audit, vault, and ledger persistence.
- [AI Agents and the g8e Governance Boundary](./agents.md): MCP, A2A, governed HTTP dispatch, direct-envelope, and external-wrapper limits.
- [Connect Operator to Gateway](../guides/connect_operator_to_gateway.md): Enrollment and day-two operations.
- [Build Operator](../guides/build_operator.md): Build instructions, startup options, runtime layout, and processing contract.
- [Evaluations](./evals.md): Inference, Observer, and Provenance Operator roles.
