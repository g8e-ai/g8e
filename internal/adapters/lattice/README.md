# Lattice Adapter

Last Updated: 2026-07-28
Version: v1.6.6

The Lattice adapter integrates the g8e Operator with Anduril's Lattice Common Operating Picture (COP). It publishes the Operator as a live entity in Lattice, subscribes to task assignments, and reports execution status back to the COP. The adapter communicates over gRPC using TLS and OAuth2 client credentials authentication.

Vendored protobuf definitions for the Lattice SDK live in `third_party/anduril/anduril/`; see that directory's README for proto source, pinning, and code generation details.

---

## Architecture

The adapter connects to two central Lattice gRPC service boundaries:

- **Entity Manager**: Handles entity presence publications. The Operator registers itself as a live asset with a 5-minute soft-state expiry. Presence is republished periodically via the Operator's heartbeat system to maintain entity liveness.
- **Task Manager**: Streams task assignments to the Operator over a server-side streaming RPC. The adapter filters incoming tasks by specification type and governance posture before dispatching them to the execution pipeline.

The adapter manages its own lifecycle within the Operator startup and shutdown sequence. It initializes after the pub/sub system starts and stops before the execution service shuts down.

---

## Configuration

The adapter activates when `--lattice-endpoint` is set or the `LATTICE_ENDPOINT` environment variable is present. All other parameters fall back to environment variables when CLI flags are omitted.

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--lattice-endpoint` | `LATTICE_ENDPOINT` | empty | Lattice gRPC endpoint URL |
| `--lattice-client-id` | `LATTICE_CLIENT_ID` | empty | OAuth2 client ID |
| `--lattice-client-secret` | `LATTICE_CLIENT_SECRET` | empty | OAuth2 client secret |
| `--lattice-sandboxes-token` | `SANDBOXES_TOKEN` | empty | Sandbox authorization token |
| `--lattice-entity-name` | `LATTICE_ENTITY_NAME` | empty | Entity display name |
| `--lattice-posture-floor` | `LATTICE_POSTURE_FLOOR` | `consensus` | Minimum governance posture |

Advanced parameters such as platform location coordinates and task catalog specification filters are configured through file-based settings or programmatic construction.

---

## Entity Presence Lifecycle

On startup, the adapter loads or generates a persistent entity identifier saved in the `.g8e/` runtime directory. It publishes the entity to Lattice with a 5-minute expiry duration. The Operator heartbeat system republishes presence on each heartbeat cycle to guarantee continuous liveness. Heartbeat intervals must be configured to fire faster than every 4 minutes.

If initial presence publication fails, the adapter logs a warning and continues startup. The next scheduled heartbeat cycle attempts presence republication automatically.

---

## Task Subscription and Execution

The adapter opens a real-time streaming RPC connection to Lattice's Task Manager to receive work assignments. Incoming stream messages pass through a strict filtering sequence:

1. **Heartbeat filtering**: Inbound stream heartbeats are discarded without processing.
2. **Task catalog filtering**: If a task catalog is defined, tasks are accepted only if their specification match configured catalog entries. An empty catalog accepts all specifications.
3. **Posture floor validation**: Tasks are rejected when the active governance posture falls below the configured floor.

Accepted tasks execute asynchronously in separate worker routines. Task execution outcomes, whether successful completion, posture rejection, or processing error, are reported back to Lattice with complete status details.

Disconnections trigger automatic stream reconnection using exponential backoff capped at 30 seconds between retry attempts.

---

## Governance Posture Floor and Interlock Integration

The posture floor prevents execution of external tasks when the Operator operates under a governance posture less strict than required. Posture ranking orders from least to most strict: `doctrine`, `consensus`, and `notary`. An unknown or empty active posture defaults to `doctrine` to fail closed.

Before dispatching tasks, the adapter verifies that the active posture meets or exceeds the required posture floor. If accepted, the task enters the five-layer interlock sequence:

- **L1 Doctrine**: Hard gates, forbidden pattern matching, MITRE threat detection.
- **L2 Consensus**: Multi-agent consensus signature verification (Ed25519).
- **L3 Notary**: Human-in-the-loop authorization (WebAuthn or signed CLI proofs).
- **L4 Warden**: Pre-dispatch verification (signatures, replay prevention, expiry, nonces, Merkle root).
- **L5 Actuator**: Isolated tool dispatch (MCP/A2A), JIT capability minting, and signed receipt production.

If the active posture is below the configured posture floor, the task is rejected immediately and recorded as a policy failure.

---

## Practical Setup and Operations

Operators enable Lattice integration during startup by supplying connection parameters via command-line flags or environment variables.

1. **First-time configuration**: Obtain OAuth2 client credentials and gRPC endpoint URLs from the Lattice administrator. Set `LATTICE_ENDPOINT`, `LATTICE_CLIENT_ID`, and `LATTICE_CLIENT_SECRET` in the environment.
2. **Starting the Operator**: Run `g8e operator start` with `--lattice-endpoint` and `--lattice-posture-floor consensus`. The Operator initializes, creates its persistent entity identity, and publishes initial presence.
3. **Daily monitoring**: Monitor log output for presence republication events and task execution receipts. Entity liveness and stream connectivity are maintained automatically through background heartbeats and backoff retries.

---

## Graceful Shutdown

During Operator shutdown, the adapter performs a structured cleanup sequence:

1. Unregisters the heartbeat sink to cease presence republication.
2. Cancels the internal adapter context to terminate stream subscription routines.
3. Waits for all active task handler routines to finish in-flight work.
4. Closes the underlying gRPC transport connection.

This sequence stops task ingestion after pub/sub shutdown while keeping status reporting available until final task completion.

---

## Security and Resiliency

- **TLS Transport**: All gRPC transport channels enforce TLS encryption using loaded client certificates.
- **OAuth2 Authentication**: Authentication relies on OAuth2 client credentials. Tokens are acquired automatically and refreshed proactively at two-thirds of their lifetime with randomized jitter.
- **Sandbox Environment Support**: Sandboxed environments attach a dedicated authorization header (`Anduril-Sandbox-Authorization`) to both token requests and gRPC RPC metadata.
- **Fault Recovery**: Transient gRPC transport errors trigger exponential retry backoff with jitter. Authentication failures force immediate token renewal followed by a single RPC retry.

---

## See Also

- [Operator Architecture](../../../docs/architecture/operator.md) for the Operator service lifecycle and boot sequence
- [Governance](../../../docs/architecture/governance.md) for posture configuration and the 5-layer verification pipeline
- [Encryption](../../../docs/architecture/encryption.md) for TLS and certificate management details
- [Vendored Lattice Protos](../../../third_party/anduril/anduril/README.md) for proto source, pinning, and code generation
