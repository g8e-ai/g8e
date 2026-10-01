---
title: Connect Operator to Gateway
parent: Guides
---

# Connect g8e Operator to g8e Gateway

Last Updated: 2026-10-01
Version: v2.2.6

---

## Overview

The g8e Operator is the Policy Execution Point (PEP) for the runtime visible to its process. It opens an outbound mTLS WebSocket connection to the Gateway, subscribes to its identity- and session-scoped command channel, verifies governed transactions, executes approved actions in its own runtime, and publishes signed receipts and heartbeats. The Operator exposes no inbound application listener. In the root Compose deployment, the Operator runs as a separate container with its own process and network namespace; it does not access the Docker host or the Gateway container's volume.

Connecting a new Operator has four parts:

1. Start the Gateway.
2. Enroll the first Gateway owner if not already bootstrapped.
3. Start the Operator with the Gateway endpoint. The Operator submits a platform enrollment request and waits for approval.
4. Approve the request from an enrolled owner CLI. The Operator receives its issued credentials and connects to the pub/sub channel automatically.

The reference Gateway and Operator are subcommands in the same `g8e` binary. See [Build Operator](build_operator.md) for build and runtime state internals and [Operator Architecture](../architecture/operator.md) for the service design.

---

## Prerequisites

**Gateway host requirements:**

- A running `g8e` Gateway, started with `./g8e gw start`.
- TCP ports 8080 and 8443 reachable from the Operator host. Port 8080 serves the discovery and enrollment bootstrap routes, including `/.well-known/g8e/pki/ca-bundle`. Port 8443 serves authenticated APIs and the mTLS WebSocket pub/sub connection for active Operators.
- A certificate identity that matches the hostname the Operator uses to reach it. The default certificate mode detects hostnames and IP addresses at Gateway startup; `--cert-mode localhost` is suitable only for same-host connections.
- An enrolled owner CLI identity to approve Operator enrollment requests.

**Operator host requirements:**

- The `g8e` binary and a writable launch directory. The process creates its `.g8e/` runtime tree below that directory and uses it across restarts to store credentials, vault keys, and local state.
- Outbound access to the Gateway on ports 8080 (HTTP discovery) and 8443 (mTLS bootstrap and pub/sub).
- A working directory for command execution. Use `--working-dir` to select this directory; the `.g8e/` runtime tree is not affected.

No inbound port is required on the Operator host.

### Endpoint Requirements

Use a bare hostname with `g8e operator start --endpoint`, without an `http://` or `https://` scheme and without a port suffix. The worker uses Gateway port 8080 for discovery and enrollment bootstrap and port 8443 for mTLS authentication, state queries, receipt publication, and pub/sub channel subscription. The global `--port` flag does not affect `operator start`; non-default Gateway ports are not supported by this command path.

The hostname must resolve on the Operator host and must match the Gateway certificate identity. For a remote Gateway without DNS, map the Gateway IP to the built-in `g8e.local` identity on the Operator host and use `g8e.local` as the endpoint:

```text
192.0.2.10 g8e.local
```

Replace `192.0.2.10` with the Gateway IP address. Do not use `--cert-mode localhost` for remote connections; the default mode generates a certificate with the detected hostname and IP addresses.

---

## Connect a New Operator

### 1. Start the Gateway

On the Gateway host, start the service:

```bash
./g8e gw start
```

The Gateway runs as a background process in its own session by default. Use `--follow` (`-f`) to run it in the foreground, or use `./g8e gw logs -f` to follow a background process. The default posture is `doctrine`. Use `--posture consensus`, `--posture ratify`, or `--posture notary` to select a different governance layer configuration.

To preview settings with the interactive wizard before starting, run:

```bash
./g8e gw setup
```

This runs the wizard to resolve posture, consensus configuration, passkey settings, CORS origins, downstream routes, public URL, and certificate settings. It prints the result but does not persist or start the Gateway. Use `gw start --interactive` to run the wizard and start the Gateway with the resolved configuration in one invocation.

Verify the Gateway process is running:

```bash
./g8e gw status
```

### 2. Enroll the Gateway Owner

A new Gateway starts without any users and does not issue platform workload credentials until the first owner enrolls. On the Gateway host or on an owner workstation with network access to it, run:

```bash
./g8e auth enroll user --endpoint <gateway-host>
```

For a same-host Gateway, omit `--endpoint` or use `localhost`. Enrollment creates the first user and CLI session on an unbootstrapped Gateway. By default, it installs the Gateway root CA into the operating system trust store and runs the browser-based passkey ceremony. Use `--no-system-trust` only if an administrator has already installed the root CA separately. Use `--headless` to create an mTLS-only CLI identity when another enrolled owner can approve the recovery request later; a headless identity does not support web Console sessions.

The first enrolled user becomes the persistent owner authorized to approve platform workload enrollment requests.

### 3. Start the Operator

On the Operator host, start the worker in the foreground:

```bash
./g8e operator start --endpoint <gateway-host>
```

For a same-host deployment, use:

```bash
./g8e operator start --endpoint localhost
```

If Operator credentials are not already installed in `.g8e/pki/`, the process executes these steps:

1. Fetches the Gateway trust bundle from `http://<gateway-host>:8080/.well-known/g8e/pki/ca-bundle`.
2. Generates Operator and CLI ECDSA P-256 keys and certificate signing requests (CSRs).
3. Submits a platform enrollment request to the Gateway and waits for owner approval.
4. Persists the request state to `.g8e/pki/pending-enrollment/g8eo.json` with mode 0600, allowing the process to resume after restart.
5. Prints the request ID, component name, hostname, CSR fingerprints, and the approval command, then polls the Gateway for a decision.

Keep this process running while the owner approves the request. If the process exits, rerunning the same command from the same launch directory resumes the pending enrollment with the same CSRs and key material. A pending request expires if not approved within the server-provided enrollment window.

Operator enrollment happens automatically during `operator start` when credentials are missing. There is no separate `g8e auth enroll operator` command.

### 4. Inspect and Approve the Request

From the enrolled owner CLI, list pending platform enrollment requests:

```bash
./g8e auth enroll pending --endpoint <gateway-host>
```

Compare the displayed component name, hostname, system fingerprint, and Operator and CLI key fingerprints with the output from the waiting Operator process. Then approve or deny the matching request:

```bash
./g8e auth enroll approve <request-id> --endpoint <gateway-host>
./g8e auth enroll deny <request-id> --endpoint <gateway-host>
```

Each command displays the request details and prompts for confirmation. For non-interactive operation after independent validation, use `--yes`. Use `--reason` to add a decision note.

Only the first enrolled owner can approve or deny requests. The Gateway enforces this authorization by checking the owner's authenticated CLI identity (mTLS) or web Console session.

### 5. Wait for the Connection

After approval, the Operator signs the completion transcript with both private keys, receives the issued certificates, validates the response, and writes them to:

- `.g8e/pki/operator.crt` - Operator certificate
- `.g8e/pki/operator.key` - Operator private key
- `.g8e/pki/cli.crt` - CLI certificate (for local enrollment operations)
- `.g8e/pki/cli.key` - CLI private key
- `.g8e/pki/trust/g8eg-ca-bundle.pem` - Gateway trust bundle

Once these writes succeed, the process removes the pending state and then authenticates to the Gateway over mTLS, receives its Operator ID, Operator session ID, runtime limits, governance posture, and heartbeat interval configuration. It initializes encrypted local storage, execution vaults, and replay protection, then subscribes to its command channel:

```text
cmd:<operator-id>:<operator-session-id>
```

A successful connection prints `Channel established - Ready to receive` in the logs. The Operator sends an immediate heartbeat and continues at the configured interval, which defaults to 30 seconds and cannot exceed 30 seconds. The Gateway marks an Operator `stale` after 60 seconds without a heartbeat.

---

## Connect the Root Compose Operator

The root `docker-compose.yml` runs the Gateway and Operator as separate containers with separate process and network namespaces and named volumes. The Gateway publishes host ports 8080 and 8443 by default. The Operator container reaches the Gateway as `g8e.local:8080` and `g8e.local:8443` and exposes no host port. Each container has its own volume for credentials, vault keys, and local state; the Operator volume does not mount the Docker host filesystem.

All core services (Gateway, Data Operator, Inference Operator, ensemble, and dashboard) start together in the default Compose profile; there is no bootstrap profile. Workloads submit their platform enrollment requests and poll for owner approval, so enroll the owner from the repository host once the Gateway is healthy:

```bash
docker compose up -d
./g8e auth enroll user -e localhost
```

List pending enrollment requests:

```bash
./g8e auth enroll pending
```

Approve the request for the Data Operator (container `g8e-data-operator`, hostname `data-operator`), then verify the remote session:

```bash
./g8e auth enroll approve <data-operator-request-id> --yes
./g8e operator list
```

The container runs `operator start -e g8e.local`. The same owner-approval workflow applies to the Inference Operator, dashboard, and ensemble. For profile details, volume ownership, and cleanup procedures, see [Unified Docker Stack](unified_stack.md).

---

## Use Pre-Provisioned Credentials

To start with an existing Operator identity, provide the certificate, private key, and Gateway trust bundle explicitly:

```bash
./g8e operator start \
  --endpoint <gateway-host> \
  --cert /path/to/operator.crt \
  --key /path/to/operator.key \
  --trust-bundle /path/to/g8eg-ca-bundle.pem
```

Without explicit paths, the worker looks for `.g8e/pki/operator.crt`, `.g8e/pki/operator.key`, and `.g8e/pki/trust/g8eg-ca-bundle.pem` below its launch directory. The certificate and key must be present and form a valid matching pair. Partial credential state does not trigger automatic enrollment; the process fails closed if any required file is missing.

The worker runs in the foreground by default. Use the operating system's service manager or container orchestrator to supervise it in production and preserve its launch directory and `.g8e/vault/` keys across restarts.

---

## Deploy the Binary to a Remote Host

Use `g8e operator cp <target>` to copy the current binary to a local target directory:

```bash
./g8e operator cp /tmp/g8e
```

Use `g8e operator scp <user@host:path>` to copy it to a remote host:

```bash
./g8e operator scp user@192.0.2.10:/opt/g8e
```

After copying, make the binary executable and start it on the target host:

```bash
ssh user@192.0.2.10 chmod +x /opt/g8e
ssh user@192.0.2.10 /opt/g8e operator start --endpoint <gateway-host>
```

`g8e operator deploy --hosts user@192.0.2.10 --remote-dir /opt/g8e-operator --background --endpoint <gateway-host>` performs the same copy and start in one step (see [Build Operator](build_operator.md#deployment-commands)). Approve the resulting enrollment request as described above.

`operator deploy` uploads the binary as `<remote-dir>/g8e.new` and renames it over `<remote-dir>/g8e`, so redeploying into a directory whose Operator is still running does not fail with "text file busy". With `--background` it also stops any Operator previously started from that directory before starting the new one. Each distinct `--remote-dir` is a distinct Operator identity.

### Connect Many Operators

The Gateway allows at most three live (non-terminal) Operator enrollment requests at once, platform-wide; further `operator start` processes are rejected with HTTP 429. `operator deploy` handles that pacing itself:

```bash
./g8e operator deploy --hosts localhost --endpoint <gateway-host> \
  --remote-dir ~/fleet --count 10 --background --approve
```

`--count` puts each Operator in its own `<remote-dir>/op-NNNNN` directory. `--approve` starts them one at a time, reads each worker's enrollment request ID from that directory's `start.log`, approves exactly that request as the owner, restarts a worker the Gateway rejected (up to three attempts), and then waits until every approved Operator is active. It prints each Operator's session ID; pass them to `operator bind` and `operator run` as described in [Bind the CLI to Operators and Run Commands](#bind-the-cli-to-operators-and-run-commands). Redeploying over a directory whose Operator is already enrolled replaces the running worker and needs no new approval. To tear a fleet down, `operator stop <operator-session-id>` each Operator, `auth enroll revoke <request-id>` each printed request ID, and remove the directories.

---

## Verify and Operate the Connection

### Check Gateway Health

```bash
./g8e gw status
```

This checks the Gateway HTTP health endpoint, checks the local process manager for foreground mode, and reports Docker Compose stack status if running. It verifies the Gateway, not connected Operator processes.

### List Connected Operators

From an enrolled CLI identity, list Operators associated with your user:

```bash
./g8e operator list --endpoint <gateway-host>
```

This displays each Operator's ID, type (e.g., data, dashboard, ensemble), hostname, session ID, and status. A status of `stale` means the Gateway has not received a heartbeat from that Operator for more than 60 seconds; it returns to `active` on the next heartbeat. A stopped or crashed Operator process therefore leaves `active` within about a minute without any manual cleanup. Use `g8e operator show <operator-id-or-session-id>` to display the last heartbeat time, the latest heartbeat snapshot, and performance metrics. Check the Operator process log for the `Channel established - Ready to receive` message as confirmation that it established its pub/sub channel subscription.

### Bind the CLI to Operators and Run Commands

Bind the enrolled CLI session to one or more Operators when automation needs them as targets. Any bound session is accepted as a request's operator identity, not only the first. Pass every Operator session ID to a single `bind` call; do not loop over Operators, because each call issues a replacement CLI session:

```bash
./g8e operator bind <operator-session-id> [<operator-session-id>...]
./g8e operator bind list
./g8e operator bind unbind
```

The Gateway validates every target (active and owned by the authenticated user) before changing anything; if any target is rejected, none is bound and the existing binding is kept. The first ID is the primary binding that the auth middleware stamps on the CLI session; the rest are recorded alongside it and reported by `bind list`. One call accepts up to 5000 sessions, and rebinding the identical list is idempotent and does not rotate the CLI session. Use `--yes` to skip the confirmation prompt.

Execute a governed shell command on active Operators in parallel:

```bash
./g8e operator run <operator-session-id> [<operator-session-id>...] \
  --cmd "uname -a"
```

Each target Operator must belong to the authenticated user. Binding changes and `operator run` both require an enrolled CLI identity. See [Build Operator](build_operator.md#operate-remote-operators-from-the-cli) and [Authentication and Authorization](../architecture/auth.md#cli-operator-session-binding).

### View Gateway Logs

```bash
./g8e gw logs -f
```

### Restart the Gateway

```bash
./g8e gw restart
```

Gateway restart restores the complete validated launch profile from the last successful `gw start`, including posture, CORS, passkey, port, and service URL settings. If the profile is missing or invalid, restart fails closed. A running Operator detects a closed pub/sub stream and retries with bounded backoff; supervise the worker to restart it if retry limits are exhausted.

### Stop the Gateway

```bash
./g8e gw stop
```

### Stop the Operator

From an enrolled CLI, `g8e operator stop <operator-session-id> [--reason <text>]` stops a remote Operator through its governed shutdown channel. On the Operator host, send `SIGINT` or `SIGTERM` to the foreground Operator process. It cancels the service context, stops the heartbeat scheduler and pub/sub service, closes local services, and exits after graceful shutdown.

---

## Troubleshooting

### The Operator Waits Before Submitting an Enrollment Request

The Gateway is running but has not been bootstrapped with an owner. The Operator retries with bounded backoff. Run `g8e auth enroll user --endpoint <gateway-host>` on the Gateway host to create the first owner, then allow the Operator process to retry or restart it.

### The Operator Waits for Approval

List pending enrollment requests from the owner CLI:

```bash
./g8e auth enroll pending --endpoint <gateway-host>
```

Compare the request's component, hostname, system fingerprint, and key fingerprints with the Operator's output, then approve:

```bash
./g8e auth enroll approve <request-id> --endpoint <gateway-host>
```

Restarting the Operator from the same launch directory resumes the persisted request with the same CSRs and key material. Starting from a different directory creates or uses a different `.g8e/` runtime tree.

### Trust-Bundle Fetch Fails

Confirm the Operator host can reach the Gateway discovery port and verify the endpoint is a bare resolvable hostname:

```bash
curl -fsS http://<gateway-host>:8080/.well-known/g8e/pki/ca-bundle
```

If the request fails, check routing, firewall rules, Gateway health, and hostname resolution. If it succeeds but Operator startup rejects the bundle, inspect the Gateway and Operator logs for certificate parsing or trust errors instead of bypassing TLS validation.

### mTLS Bootstrap or Pub/Sub Connection Fails

Verify all of the following:

- The Operator uses the same launch directory containing `.g8e/pki/operator.crt` and `.g8e/pki/operator.key`.
- The endpoint has no `http://` or `https://` scheme and no port suffix.
- The endpoint hostname resolves on the Operator host.
- The hostname matches the Gateway certificate identity (check `gw status`).
- TCP port 8443 is reachable.
- The Operator certificate and key are a valid, non-expired pair.
- The trust bundle exists at `.g8e/pki/trust/g8eg-ca-bundle.pem`.

For IP-only remote deployments, map the Gateway IP to `g8e.local` locally and use `--endpoint g8e.local`. Do not disable certificate verification.

### Startup Reports Missing Operator Session ID

The mTLS bootstrap request must receive the Operator ID and session ID before initialization completes. Verify that enrollment was approved, the issued certificate identifies the Operator correctly, and the Gateway still contains the session record. Note that `auth enroll user` manages CLI credentials only; it does not repair Operator identities.

### Execution Vault Initialization Fails

The execution vault is enabled by default and required for replay protection. Do not start the worker with `--execution-vault=false`. Ensure the launch directory is writable and that any existing `.g8e/vault/key` can decrypt the vault state. See [Build Operator](build_operator.md#local-runtime-state) for runtime state layout and vault administration.

---

## Security Properties

**Owner-approved enrollment:** A new workload receives no platform certificate until the first enrolled owner approves its request through an authenticated CLI session (mTLS) or web Console session. Approval is owner-scoped and persisted on the Gateway.

**Proof of key possession:** The Operator signs the enrollment completion transcript with both its generated private keys before the Gateway issues its certificate. Possession of both private keys is required to complete enrollment.

**Outbound-only worker:** The Operator initiates all connections to the Gateway and exposes no inbound listener. It cannot be reached by external clients or the host network.

**Scoped command channel:** The Operator subscribes only to a pub/sub channel scoped by its Operator ID and session ID. Commands not bound to its session are rejected.

**Fail-closed execution:** Before applying Operator actions (L5), the Operator verifies transaction structure, expiry, replay state, hash binding, state binding, L1 doctrine matching, and all L2/L3 evidence required by the configured posture. Missing evidence fails closed.

**Local-first evidence:** The Operator persists signed execution evidence and encrypted local state to `.g8e/` before publishing result projections to the Gateway, ensuring evidence is never lost if the Gateway connection is disrupted.

**mTLS and revocation:** Gateway authenticated routes validate certificate identity, revocation status, and TLS chain in addition to standard TLS hostname and expiry checks.

---

## Next Steps

[Build Operator](build_operator.md) — Build the reference Operator, understand the `.g8e/` runtime state layout, and review the local execution contract.

[Operator Architecture](../architecture/operator.md) — Review the Operator service design, pub/sub channel format, and governance layer boundaries.

[Build Gateway](build_gateway.md) — Build the reference Gateway, understand configuration persistence, and review PKI and enrollment flows.

[Build Apps](build_apps.md) — Connect MCP, A2A, and direct-envelope clients to the Gateway.
