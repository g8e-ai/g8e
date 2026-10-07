---
title: g8e Operator
---

# g8e Operator

Last Updated: 2026-10-06
Version: v2.3.2

The Governed Operator is the Policy Execution Point (PEP) for the runtime in which the Operator process runs. The reference implementation is the `g8e` binary started with `g8e operator start`. It receives governed `GovernanceEnvelope` transactions from a Gateway over an outbound-only mTLS WebSocket connection, verifies each transaction locally, executes accepted typed actions through the L5 Actuator, and stores authoritative local execution evidence.

The Operator is not the Gateway and does not receive inbound management connections. The Gateway is the Policy Decision Point (PDP): it authenticates ingress, owns platform coordination state and PKI, constructs or admits envelopes, coordinates L1-L3 according to the active posture, and publishes work to an exact Operator session. The Operator independently performs the L4/L5 execution path and does not trust the fact that the Gateway sent a message as authorization.

This document describes the outbound Operator. The Gateway also has an embedded in-process Operator substrate that executes only against the Gateway process runtime. See [Gateway Architecture](./gateway.md) for the distinction and [Connect Operator to Gateway](../guides/connect_operator_to_gateway.md) for deployment procedure.

## Combining Operator roles

One `g8e` binary can assume any nonempty combination of Embedded, Data, Inference, Provenance, and Observer. `OperatorRoles` is the centralized Go set type; `protocol/constants/status.json` owns the public vocabulary. Documents expose `operator_roles` arrays and `runtime_config.roles` arrays. Capability checks use membership, and runtime configuration reports every enabled role.

```bash
# One remote process with three capabilities
./g8e operator start --endpoint gateway.example --roles provenance,observer,inference \
  --model-storage-root /srv/ollama/models --inference-ollama-endpoint http://127.0.0.1:11434

# Add governed Data commands to either inference or a witness
./g8e operator start --endpoint gateway.example --roles inference,data
./g8e operator start --endpoint gateway.example --roles provenance,data --model-storage-root /srv/ollama/models

# The Gateway's embedded operator can assume all capabilities
./g8e gw start --roles embedded,data,inference,provenance,observer \
  --model-storage-root /srv/ollama/models --inference-ollama-endpoint http://127.0.0.1:11434

# The same embedded runtime is available through the operator startup command
./g8e operator start --roles embedded,data,inference --working-dir /srv/gateway
```

`--roles` accepts comma-separated values and repeated occurrences. Existing enable flags are additive. No role flags means Data for a remote worker and Embedded plus Data for the Gateway. Provenance needs `--model-storage-root`; Inference uses the configured Ollama endpoint. Witness-only processes reject generic command execution. Adding Data permits governed commands in the same process. Evaluation target selection accepts active Remote and configured Embedded sessions by capability membership: Data for command execution and Inference for model requests. Session and hardware selection remain exact; multiple eligible sessions require an explicit selection.

Embedded runs the Gateway in process, including its enrollment and governance substrate. It does not become a remote worker by enabling another capability. `operator_type` continues to distinguish `embedded` and `remote` deployment. Cloud remains a deployment setting (`--cloud` and `--provider`), and Cloud, System, and Node are not entries in the operator type or role vocabulary. “Inference Node” names the Inference topology, not a separate operator type.

The singular `operator_role` and runtime `role` fields are replaced by role arrays. Deploy remote workers with `operator deploy --roles data,provenance`; role settings are forwarded intact to startup. `operator deploy` places up to 5000 Operators per target (`--count`, `--start-index`, each in its own `op-NNNNN` directory under `--dest-dir`) on the local system (`--local`), over SSH (`--hosts`), or as one Docker container per Operator (`--docker-context` with `--docker-image`). The owner-facing Gateway address is the global `--endpoint`; `--operator-endpoint` selects the worker-facing address the deployed Operators use and defaults to `--endpoint`. `--background` starts the workers, and `--approve` approves each enrollment request as the authenticated owner and verifies the sessions are online. Run `./g8e operator deploy --help` for the flag inventory. Role identity and fingerprints include the entire canonical set, so flag order and duplicate values do not change identity.

## Runtime and trust boundaries

A remote Operator is sovereign only for the runtime visible to its process. Its filesystem, process table, services, network, and container runtime are those of that runtime; an Operator container is not automatically the Docker host. In the root Compose deployment, `g8e-gateway` and `g8e-data-operator` are separate containers with separate process and network namespaces and separate named volumes. Neither receives host-root, host-PID, host-network, or Docker-socket access by default. The Gateway's embedded Operator targets the Gateway container in that deployment; the outbound Operator targets the Operator container.

The Operator opens the connection to the Gateway and exposes no inbound MCP, A2A, or command listener. The Gateway publishes commands to a channel bound to the Operator ID and Operator session ID. The Operator publishes heartbeats, command results, and signed receipt projections back to Gateway-owned channels. SSE is delivery telemetry, not authorization or durable governance state.

The Operator's workload certificate carries a SPIFFE URI SAN in the `g8e.local` trust domain. The normal remote identity format is `spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>`. The Gateway validates certificate identity, session binding, and revocation for authenticated requests and WebSocket handshakes. See [Network Architecture](./network.md) and [Authentication and Authorization](./auth.md) for the PKI and identity contracts.

## Enrollment and startup

`g8e operator start` runs in the foreground. It creates the `.g8e/` runtime tree, loads an explicit or installed trust bundle and Operator certificate/key, and initializes the local services required for execution. With `--endpoint` and no installed Operator credentials, it fetches the Gateway trust bundle over the Gateway discovery HTTP listener and starts the owner-approved platform enrollment flow. The flow creates Operator and companion CLI CSRs, persists resumable pending state, waits for owner approval, verifies the completion transcript, and atomically writes the issued credentials. A restart from the same launch directory resumes the pending request.

After credentials are available, the worker connects to the Gateway over mTLS, obtains bootstrap information including its Operator and session identity, and subscribes to an exact command channel:

```text
cmd:<operator-id>:<operator-session-id>
```

It publishes an immediate heartbeat and continues at the configured interval, which defaults to 30 seconds. An automatic heartbeat is Operator-originated liveness, not a governed operation: it is published directly and produces no receipt or ledger commitment, so it never appears in operational compliance evidence. The Gateway stamps each heartbeat's receipt time on the Operator document, and sixty seconds without one marks the Operator `stale` (see [Liveness and Staleness](#liveness-and-staleness)). The worker holds its command-channel subscription for as long as the service runs: a lost subscription is retried with capped exponential backoff and never abandoned, so a Gateway that is down for any length of time (a restart or an upgrade) does not leave an Operator that heartbeats but cannot receive a command. A TLS certificate failure on the channel is not retried; it shuts the worker down. It initializes encrypted local storage and execution services before accepting governed work. The execution vault is required by the outbound startup path; setting `--execution-vault=false` fails closed during initialization.

The startup command accepts numerous flags controlling enrollment, runtime behavior, and specialized roles. The primary options are:

| Option | Description |
| --- | --- |
| `-e, --endpoint <host>` | Gateway HTTP discovery endpoint (global flag); defaults to `localhost` when omitted. |
| `-p, --port <int>` | Gateway HTTPS/mTLS port (global flag); overrides default 8443 when used with `--endpoint`. |
| `--cert <path>` | Path to Operator client certificate for mTLS enrollment. |
| `-k, --key <path>` | Path to Operator private key matching the certificate. |
| `--trust-bundle <path>` | Explicit Gateway CA bundle; otherwise uses installed runtime bundle or endpoint discovery. |
| `--working-dir <path>` | Working directory for command execution; does not relocate `.g8e/` runtime tree. |
| `-c, --cloud` | Enables cloud Operator mode. |
| `--provider <aws\|gcp\|azure>` | Selects cloud provider when used with `--cloud`. |
| `--heartbeat-interval <int>` | Sets heartbeat frequency in seconds (default: 30; accepted range 0-30, where 0 selects the default). Startup rejects larger values because the Gateway marks an Operator stale after 60 seconds without a heartbeat, so the interval must leave room for one missed beat. |
| `-s, --execution-vault` | Enables execution vault (default: true); required for outbound startup. |
| `-G, --no-git` | Disables Git integration for file ledger while retaining encrypted audit storage. |
| `-l, --log <info\|error\|debug>` | Sets log level (default: info). |
| `--inference-enabled` | Enables governed LLM inference backend (g8ellama). |
| `--inference-ollama-endpoint <url>` | Remote Ollama provider endpoint (default: http://127.0.0.1:11434). The Inference Operator configures no models; the user picks each role's model in the Console and every governed request carries it. |
| `--inference-keep-alive <duration>` | Ollama keep-alive duration (default: -1 for infinite). |
| `--provider-boundary-observer-enabled` | Enables read-only provider-boundary hardware observation. |
| `--provider-boundary-observer-id <id>` | Stable observer identity pseudonym. |
| `--provenance-operator-enabled` | Enables storage-side model provenance attestation at model file site. |
| `--provenance-operator-id <id>` | Stable provenance Operator identity pseudonym. |
| `--model-storage-root <path>` | Root directory containing content-addressed model weight blobs (e.g., ~/.ollama/models). |
| `--lattice-endpoint <url>` | Lattice gRPC endpoint URL. |
| `--lattice-client-id <id>` | OAuth2 client ID for Lattice. |
| `--lattice-client-secret <secret>` | OAuth2 client secret for Lattice. |
| `--lattice-entity-name <name>` | Entity display name for Lattice. |
| `--lattice-posture-floor <posture>` | Minimum governance posture for Lattice (default: consensus). |
| `--lattice-sandboxes-token <token>` | Sandbox authorization token for Lattice. |

The inference, observer, provenance, and Lattice options configure specialized roles that supplement the Operator's core governance path. See [Evaluations](./evals.md) and [Model Provenance](./model-provenance.md) for evaluation roles. Use `./g8e operator start --help` as the authoritative command-surface reference. An Inference Operator validates its endpoint configuration at startup but does not require the provider to be reachable. It remains connected and heartbeating during provider outages. An inference call returns the provider error for that request; subsequent calls can succeed when the provider recovers, without restarting or re-enrolling the Operator. Operator liveness does not imply provider or model availability.

## Multi-Operator Coexistence and Role Separation on the Same System

The g8e Operator is a compact binary, and multiple operator processes can execute concurrently on the exact same host system for entirely different purposes. For instance, an **Inference Operator** (g8ellama), a **Provenance Operator** (weight attestation), an **Observer Operator** (hardware metrics), and a **Data Operator** (governed tools/command execution) may all run on the same physical host or container runtime.

### Disambiguation and Composite Fingerprinting

Operators running on the same host are differentiated and separated by four key factors:
1. **Local Directory (`local_dir`)**: The working and runtime root where the instance maintains its sovereign `.g8e/` state and execution tree.
2. **Launching Account (`account`)**: The operating system account or user profile that spawned the process.
3. **Port (`port`)**: The HTTP/HTTPS port dialed or bound by the operator instance.
4. **Operational Roles (`operator_roles`)**: The complete typed set of capabilities enabled by startup flags.

The canonical `system_fingerprint` is a SHA-256 composite hash of immutable host properties (`os`, `arch`, `cpu_count`, `machine_id`, `hostname`) combined with `local_dir`, `account`, `port`, and the canonical `roles` set. This ensures that each operator running on the same host produces a distinct, collision-free identity in the Gateway operator registry and SQLite document store, allowing idempotent re-enrollment and unambiguous slot binding. Platform enrollment (`operatorFingerprintOptions` in `internal/cli/serve/operator.go`) and runtime bootstrap (`internal/services/auth/bootstrap.go`) derive the complete role set from the same startup flags and both include `port`. An Operator enrolled by an earlier release computed its enrollment fingerprint without `port`, so re-enrolling it after an upgrade yields a different fingerprint and does not supersede the earlier document; revoke the earlier enrollment explicitly.

Re-enrollment is a replacement, not an addition. When platform enrollment issues an Operator whose `system_fingerprint` matches a non-terminated remote Operator document the same owner already holds, the Gateway terminates the earlier document (`termination_reason` names the replacement) and deactivates its Operator session in the same governed issuance step. The new document is persisted first, so a failure leaves a redundant lease rather than none, and a retried issuance supersedes whatever it left behind. Certificate revocation remains an explicit `revoke` intent. This keeps roles that must resolve to exactly one Operator (provider-boundary observer, provenance, inference) unambiguous after an Operator is restarted with fresh enrollment.

### Liveness and Staleness

The Gateway stamps `last_heartbeat_at` with its own clock whenever a heartbeat arrives; the Operator never supplies it. A remote Operator document in `active` status whose last sign of life is older than 60 seconds (`constants.OperatorHeartbeatStaleAfter`, two missed beats at the default interval) is moved to `stale`. The last sign of life is `last_heartbeat_at`, falling back to `claimed_at` and then `created_at` for an Operator that has not heartbeated yet.

The transition runs inside the Gateway document store before any read of the `operators` collection (`DocGet`, `DocQuery`, `DocList`, `GetField`) and is persisted with a conditional update, so registry listings, session validation, SSE, the data API, and every capability selector (Data, Inference, Provenance, Observer) see the same status and a concurrent heartbeat or stop is never overwritten. If reconciliation cannot run, the read fails closed with `ErrOperatorStalenessReconcile`. A Gateway sweep repeats the same reconciliation every 15 seconds (`constants.OperatorStalenessSweepInterval`) so a silent Operator goes stale, and is announced, without waiting for a reader. Each transition the Gateway persists into `stale`, `stopped`, `terminated`, or back to `active` on a heartbeat recovery is pushed as the matching `g8e.v1.operator.status.updated.<state>` SSE event to every unexpired web session of the Operator's owner (`internal/services/gateway/operator_status_events.go`). The push is telemetry: a failed delivery is logged and never fails the transition, and the operator document remains the source of truth. The Gateway also announces `active` when an Operator first appears: claiming an offline slot (`RegistrationService.completeRegistration`) and issuing a platform-enrolled Operator (`DocumentStoreService.NotifyOperatorEnrolled`) each push `g8e.v1.operator.status.updated.active`, so a console that is already connected lists the Operator without a reload. A stale Operator may still authenticate so it can recover; its next heartbeat restores `active`. The `--heartbeat-interval` flag is capped at half the stale window so a correctly configured Operator can miss one heartbeat without being marked stale. `g8e operator show` prints `last_heartbeat_at` so a stale status can be traced to the last beat the Gateway saw. Heartbeats never revive `stopped` or `terminated` Operators, and the embedded Operator, which is live exactly when the Gateway is, is never marked stale. CLI and harness commands that require an `active` Operator (`operator run`, `operator deploy`, eval bind and inference discovery) therefore reject a silent Operator without any client-side check.

### Role Boundaries and Verification Gates

Each capability is enabled independently. Data discovery uses role membership, and witness command restrictions apply when neither Data nor Inference is enabled:

| Role | Activation Flags | Responsibilities | Execution Boundaries |
| --- | --- | --- | --- |
| **Inference Operator** | `--inference-enabled` | Governed LLM inference (g8ellama), model registry sync, Ollama provider dispatch | Serves LLM requests; executes model pull/release/residency/inventory commands (the ensemble requests inventory through governed dispatch for the console's model picker). Adding Data includes the same process in general tool-execution discovery. |
| **Provenance Operator** | `--provenance-operator-enabled`, `--model-storage-root` | Storage-side model provenance attestation over local model weights | Read-only witness. Witness-only command execution is rejected by `ValidateWitnessCommand`; Data may be enabled in the same process. |
| **Observer Operator** | `--provider-boundary-observer-enabled`, `--provider-boundary-observer-id` | Read-only hardware, temperature, power, and residency observation on approved host | Read-only witness. Witness-only command execution is rejected by `ValidateWitnessCommand`; Data may be enabled in the same process. |
| **Data Operator** | Default (or `--roles data`) | Governed tool execution, command execution, local filesystem triage, and execution vault | Primary PEP for tool execution. Discovered by Gateway session service for tool and workflow dispatch. |

The Gateway's `OperatorDocument` stores `operator_roles`, `local_dir`, `account`, and `port` alongside `system_fingerprint`. Session resolution helpers (`IsDataOperator`, `SelectDataOperator`, `SelectInferenceOperatorForHardware`, `SelectProviderBoundaryObserverForHardware`) verify that callers cannot dispatch general commands to witness operators or confuse distinct operators sharing the same machine. When the unified Docker stack is running, `SelectDataOperator` prioritizes the container session with hostname `data-operator` (`constants.DataOperatorHostname`); in host-native and local development environments without the Docker container, `SelectDataOperator` resolves the active host-native data operator session.

## Command and receipt channels

The Gateway's client-facing MCP, A2A, CLI, and enrolled-app HTTP dispatch paths are distinct ingress paths. For governed operator dispatch, an enrolled application posts a registered request `event_type` and serialized protobuf payload to `POST /api/v1/operators/commands`. The Gateway validates the event against the registry, derives `action_type`, validates the target session, obtains the current Gateway state root, applies L1 screening, sets identity and correlation fields, adds posture and transaction metadata, and publishes a canonical protojson `GovernanceEnvelope` to the matching `cmd:<operator_id>:<operator_session_id>` session. WebSocket publishers cannot inject `CommandIntent` on `cmd:` channels; outbound Operators subscribe there and receive Gateway-constructed envelopes only. The dispatch path does not synthesize missing L2 votes or L3 proofs.

The Operator decodes only the typed envelope format supported by the current protocol and rejects malformed JSON, unknown action types, missing payloads, and invalid identity or channel binding. Operator-originated heartbeat and result messages are not transformed by the Gateway broker. Receipt publications use a separate `receipts:<operator-id>:<operator-session-id>` channel. The Gateway verifies the signed `ActionReceipt` against the Operator's registered Actuator key and mirrors accepted receipts to its SQL audit store. That mirror is best effort; the Operator's local persisted receipt remains authoritative.

The CLI-directed `operator run` path is a separate Gateway route. It lets an enrolled CLI target one or more sessions whose Operator is `active` (a `stale`, `stopped`, or `terminated` session is rejected before dispatch), while the Gateway remains the envelope-construction authority and waits for a terminal result per target. `operator stop` requests governed shutdown for the selected remote session, with TERM/KILL escalation for a matched local Linux worker that does not exit promptly. Bare `g8e operator stop` stops all local workers owned by the current user, including unregistered workers. The Gateway marks a target stopped only after the exact session acknowledges the shutdown. Revocation is different from stopping: revocation invalidates the workload identity, deactivates its sessions, disconnects matching pub/sub channels, and is terminal for that enrollment.

## Five-layer execution boundary

The complete posture and proof behavior is canonical in [Governance](./governance.md). The following describes the Operator-side responsibility without duplicating that document.

### L1 Doctrine, L2 Consensus, and L3 Notary

The Gateway owns or coordinates the Policy Decision Point work for L1-L3 on its client-facing construction paths. A remote Operator does not assume that upstream screening is sufficient; its L4 Warden decodes the typed payload and independently verifies L1 Doctrine, then checks L2 and L3 evidence required by the posture carried in the envelope.

The envelope's `Posture` field is the authoritative source for Operator-side verification gating. Posture enforcement rules are:

- **doctrine**: L1 enforced, L2/L3 audited (minimum verification).
- **consensus**: L1/L2 enforced, L3 audited.
- **ratify**: L1 enforced, L3 enforced for mutation actions, L2 audited.
- **notary**: L1/L2 enforced, L3 enforced for mutation actions (maximum verification).

A command relay does not synthesize missing proofs, so a relay path must carry evidence sufficient for the selected posture.

### L4 Warden

`L4Warden.VerifyEnvelope` performs pre-dispatch verification in five sequential stages:

1. **In-flight nonce tracking**: Tracks the nonce in process memory and reserves it in the durable replay store before expensive validation to prevent race conditions during crashes.
2. **Expiry and nonce validation**: Checks that nonce and expiry fields are present, the transaction is not expired, and the nonce has not been replayed in the local store.
3. **Stateless validation**: Validates protocol version, action type, decodes the typed protobuf payload, runs local L1 Doctrine validation against the decoded payload, recomputes the transaction hash, and verifies it matches both `transaction_hash` and the envelope `id`.
4. **Stateful validation**: Fetches the current state root from the configured provider and verifies it matches the envelope's `StateMerkleRoot`. In outbound mode, the provider obtains the Gateway state root; the Operator never substitutes its local ledger root for the Gateway root.
5. **Posture validation**: Reads the envelope's governance posture from its `Posture` field and verifies posture-required L2 and L3 evidence (signatures and proofs) according to the active posture's enforcement model.

A rejected transaction does not reach the L5 Actuator. The Warden produces deterministic stage evidence for each verification stage and releases the nonce reservation when validation fails. When the Actuator and its audit dependencies are available, rejections are recorded as signed failed receipts with stage evidence.

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

Native tools are dispatched only after the request has crossed a governed Gateway ingress or arrived as a complete envelope and passed the applicable verification path. A third-party MCP server is governed only when it is configured as Gateway downstream egress, so its tool calls traverse the full pipeline and produce signed receipts; a server that a client talks to directly gains none of that merely by running alongside g8e. Client-native tools, direct filesystem access, unrestricted network access, and other side channels remain outside this boundary. See [AI Agents and the g8e Governance Boundary](./agents.md).

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
