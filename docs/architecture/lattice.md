---
title: Lattice Adapter
parent: Architecture
---

# Lattice Adapter

Last Updated: 2026-07-26
Version: v1.6.5

The Lattice adapter integrates the g8e Operator with Anduril's Lattice Common Operating Picture (COP). It publishes the Operator as a live entity in Lattice, subscribes to task assignments, and reports execution status back. The adapter communicates with Lattice over gRPC using TLS and OAuth2 client credentials authentication.

---

## Architecture

The adapter connects to two Lattice gRPC services:

- **EntityManager**: Receives entity presence publications. The Operator registers itself as a live entity with a 5-minute soft-state expiry. Presence is republished periodically via the Operator's heartbeat system to maintain liveness.
- **TaskManager**: Streams task assignments to the Operator via a server-side streaming RPC. The adapter filters incoming tasks by specification type and governance posture before dispatching them to the Operator's execution pipeline.

The adapter manages its own lifecycle within the Operator's startup and shutdown sequence. It initializes after the pub/sub system starts and stops before the execution service shuts down.

---

## Configuration

The adapter activates when `--lattice-endpoint` is set or the `LATTICE_ENDPOINT` environment variable is present. All other fields fall back to environment variables when their CLI flags are empty.

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--lattice-endpoint` | `LATTICE_ENDPOINT` | empty | Lattice gRPC endpoint URL |
| `--lattice-client-id` | `LATTICE_CLIENT_ID` | empty | OAuth2 client ID |
| `--lattice-client-secret` | `LATTICE_CLIENT_SECRET` | empty | OAuth2 client secret |
| `--lattice-sandboxes-token` | `SANDBOXES_TOKEN` | empty | Sandbox authorization token |
| `--lattice-entity-name` | `LATTICE_ENTITY_NAME` | empty | Entity display name |
| `--lattice-posture-floor` | `LATTICE_POSTURE_FLOOR` | `consensus` | Minimum governance posture |

`Latitude`, `Longitude`, and `TaskCatalog` are advanced fields not exposed as CLI flags. They are configured via config file or programmatic construction.

---

## Entity Presence Lifecycle

On startup, the adapter loads or generates a persistent entity ID (stored in the `.g8e/` runtime directory). It publishes the entity to Lattice with a 5-minute expiry time. The Operator's heartbeat system then republishes presence on each heartbeat cycle, which must fire more frequently than every 4 minutes to guarantee the entity remains live.

If the initial presence publish fails, the adapter logs a warning and continues operating. The next heartbeat cycle republishes the entity.

---

## Task Stream Subscription

The adapter opens a streaming RPC to Lattice's TaskManager and receives task assignments in real time. Incoming messages are filtered:

1. **Heartbeat messages** are ignored.
2. **Task catalog filtering**: If a task catalog is configured, only tasks with matching specification URLs are accepted. An empty catalog accepts all tasks.
3. **Posture floor enforcement**: Tasks are rejected when the active governance posture is below the configured floor. This prevents task execution under a less strict posture than required.

Accepted tasks are dispatched to the Operator's task handler in separate goroutines. Task status is reported back to Lattice via the UpdateStatus RPC, including success, rejection, or failure with error details.

The stream reconnects automatically with exponential backoff on disconnection, capped at 30 seconds between attempts.

---

## Posture Floor Enforcement

The posture floor prevents the adapter from executing Lattice tasks when the Operator's governance posture is less strict than required. The ranking, from least to most strict, is: `doctrine`, `consensus`, `notary`. An unknown or empty active posture is treated as `doctrine` (fail-closed).

For example, if the floor is `consensus` and the active posture is `doctrine`, tasks are rejected because doctrine does not enforce L2 consensus signatures.

---

## Graceful Shutdown

When the Operator stops, the adapter:

1. Unregisters its heartbeat sink to stop periodic presence republishing.
2. Cancels its internal context, which stops the task stream subscription goroutine.
3. Waits for in-flight task handlers to complete.
4. Closes the gRPC connection.

The adapter stops after the pub/sub system stops (to prevent new task dispatches) but before the execution service shuts down (to allow final status reports).

---

## Security

- **TLS**: All gRPC connections use TLS with the Operator's certificate.
- **OAuth2**: The adapter authenticates using OAuth2 client credentials. Tokens are acquired automatically and refreshed proactively. On `Unauthenticated` errors, the adapter forces a token refresh and retries the RPC once.
- **Sandbox token**: An optional sandbox authorization token is included as a header for sandboxed Lattice environments.
- **Retry**: Transient gRPC failures (unavailable, deadline exceeded, resource exhausted) are retried with exponential backoff and jitter. Non-retryable errors propagate immediately.

---

## See Also

- [Operator Architecture](./operator.md) for the Operator service lifecycle and boot sequence
- [Governance](./governance.md) for posture configuration and the 5-layer verification pipeline
- [Encryption](./encryption.md) for TLS and certificate management details
