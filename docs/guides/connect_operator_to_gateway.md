---
title: Connect Operator to Gateway
parent: Guides
---

# Connect g8e Operator to g8e Gateway

Last Updated: 2026-10-06
Version: v2.3.2

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

The hostname must resolve on the Operator host and must match the Gateway certificate identity. A raw IP endpoint remains the network dial target while g8e verifies the Gateway as its built-in `g8e.local` TLS identity. This keeps certificate verification enabled without requiring a hosts-file entry or `InsecureSkipVerify`. You may still map a stable address to `g8e.local` and use that hostname explicitly:

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

A successful connection prints `Channel established - Ready to receive` in the logs. The Operator sends an immediate heartbeat and continues at the configured interval, which defaults to 30 seconds and may be set up to 300 seconds with `--heartbeat-interval`. The Operator declares the interval at session start and the Gateway marks it `stale` after twice that interval, never less than 60 seconds.

---

## Connect the Root Compose Operator

The root `docker-compose.yml` runs the Gateway and Operator as separate containers with separate process and network namespaces and named volumes. The Gateway publishes host ports 8080 and 8443 by default. The Operator container reaches the Gateway as `g8e.local:8080` and `g8e.local:8443` and exposes no host port. Each container has its own volume for credentials, vault keys, and local state; the Operator volume does not mount the Docker host filesystem.

All core services (Gateway, Data Operator, Inference Operator, and ensemble) start together in the default Compose profile; there is no bootstrap profile. Workloads submit their platform enrollment requests and poll for owner approval, so enroll the owner from the repository host once the Gateway is healthy:

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

The container runs `operator start -e g8e.local`. The same owner-approval workflow applies to the Inference Operator and ensemble. For profile details, volume ownership, and cleanup procedures, see [Unified Docker Stack](unified_stack.md).

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

`operator deploy` installs one binary per host in `<dest-dir>/.deploy-bin/g8e`, then hard-links it into each Operator directory. Replacing a binary uses an atomic rename, so running workers remain safe. Each directory has independent runtime state, credentials, logs, and process identity.

### Deploy to a Docker Context

`operator deploy` can create one isolated container and one persistent named volume per Operator on an explicit Docker context. Prepare an image first; deployment never guesses a registry, builds source, changes the current Docker context, publishes ports, or removes an Operator volume.

Build the shared Gateway/Operator image from the repository root using the root Dockerfile. It compiles the unified binary from source and includes the Gateway connectivity preflight used by Docker deployment. The root `.dockerignore` excludes local runtime state and credentials:

```bash
docker --context livingroom-node build --platform linux/amd64 --pull=false \
  --file Dockerfile --tag g8e:local .
```

Then deploy a small batch. Here the owner CLI talks to its local Gateway while containers dial the Gateway through the machine's LAN address:

```bash
./g8e operator deploy \
  --docker-context livingroom-node \
  --docker-image g8e:local \
  --dest-dir /operators/livingroom-data \
  --count 10 --roles data \
  --endpoint localhost --operator-endpoint 192.168.1.2 \
  --background --approve
```

When the Gateway listens on non-default ports, add `--gateway-http-port` and `--gateway-https-port` so the workers dial them; `--operator-endpoint` stays a bare host or IP. Every Docker command names the selected context explicitly. Deployment resolves the image to one immutable image ID for the batch, checks daemon/image platform compatibility, and checks HTTP discovery reachability from a temporary container before creating fleet resources. Stable ownership labels protect containers and volumes from accidental adoption. A redeploy recreates only a matching owned container and retains its volume and enrollment identity. After enrollment and command-channel readiness, its restart policy becomes `unless-stopped`; failed initial enrollment does not enter an unlimited restart loop.

`--dest-dir` is an absolute path inside each container. The private volume is mounted there and is also the container working directory. A repeatable `--docker-mount type=bind,source=/remote/path,target=/container/path,readonly` can expose a remote-host model store to a provenance Operator. Bind source paths belong to the remote Docker host. Mounts must be read-only and cannot cover the private runtime root or `/g8e`. Observer hardware access is host-specific and is not granted automatically; deployment never adds `--privileged`.

Without `--background`, Docker deployment prepares stopped containers. With `--background` but without `--approve`, containers start with restart policy `no` while awaiting manual enrollment. No Operator container has a published port because all Gateway traffic is outbound.

#### Expose a WSL Gateway on the Windows LAN

For WSL in NAT mode, run the checked-in helper from an elevated Windows PowerShell. Inspect first, then apply rules scoped to the Docker machine (or a deliberately selected LAN subnet):

```powershell
.\scripts\configure-gateway-lan.ps1 -Action Inspect
.\scripts\configure-gateway-lan.ps1 -Action Apply -RemoteScope 192.168.1.53
```

The helper discovers the current Windows LAN and WSL addresses unless they are passed as `-LanAddress` and `-WslAddress`, checks WSL listeners and Gateway health, displays existing matching rules, and manages only `192.168.1.2:8080`/`:8443`-style forwards and its two named firewall rules. Use `-WhatIf` for a dry run and `-Action Remove` to remove those rules. WSL addresses can change after restart, so inspect and reapply the helper when that happens. It does not create a scheduled task or change WSL networking mode. With WSL mirrored networking, inspect the current listeners/routing first and do not add redundant NAT forwarding.

Before a fleet rollout, verify both ports from the remote Docker host and deploy one Operator through full mTLS enrollment and WebSocket readiness. A successful HTTP health check alone is not an acceptance test. Increase to ten only after the one-container redeploy preserves its named volume and identity. Measure memory, CPU, file descriptors, startup time, Gateway load, and heartbeat delays before attempting 100 or 1000 containers.

### Connect Many Operators

Deploy up to 5000 Operators per host. Use `--local` to run on this system without SSH, or `--hosts host1,host2` for remote hosts. `--dest-dir` selects the destination; `--remote-dir` remains an alias.

```bash
./g8e operator deploy --local --endpoint localhost \
  --dest-dir .local/tmp/operator-fleet --count 10 \
  --roles data --background --approve

# Add another 100 without replacing the first 10.
./g8e operator deploy --local --endpoint localhost \
  --dest-dir .local/tmp/operator-fleet --start-index 11 --count 100 \
  --roles data --background --approve

# A separate provenance batch; role flags are forwarded to operator start.
./g8e operator deploy --hosts storage-host --endpoint gateway-host \
  --dest-dir /opt/provenance --count 20 --roles provenance \
  --model-storage-root /srv/models --background --approve
```

`--count N` creates `op-00001` through `op-NNNNN`. A single Operator uses the destination itself, unless `--start-index` is explicitly supplied. The selected numbered range must fit within 1..5000. Repeating the same range replaces only those workers, retaining their enrollment credentials. Use different destinations or non-overlapping ranges for different roles.

`--roles` accepts comma-separated combinations of `data` (default), `provenance`, `inference`, and `observer`. The corresponding `operator start` enable flags and settings are also supported, including `--inference-ollama-endpoint`, `--inference-keep-alive`, `--model-storage-root`, and the provenance/observer ID flags. Roles are additive; one process can enable any combination of these capabilities. Provenance and observer IDs default to stable values unique to the deployment host and directory. Explicit flag values may include `{host}`, `{name}` (directory basename), and `{dir}` (absolute working directory), for example `--provenance-operator-id '{host}-{name}'`.

`--parallel` bounds staging workers (default 100, range 1..100). Deployment prepares and starts the entire cohort before approving anything. With `--approve`, it resolves only its own request IDs against one pending snapshot, sends one fingerprint-bound batch decision, then waits for sessions and command subscriptions. A staging failure prevents automatic approval of the cohort. Without `--approve`, it returns after collecting pending request IDs; the owner can review them and run `g8e auth enroll approve --all --yes` on an isolated Gateway. `--all` includes every request in that pending snapshot, including requests from other deployments on a shared Gateway. The Gateway admits at most 2048 live Operator requests independently of launch concurrency; other component kinds retain their smaller limits. Deployment discovers request/session IDs and subscription readiness from the non-secret `OperatorDeploymentState` record under the runtime deployment directory. A per-deployment launch ID rejects records from an earlier process, including on hosts with different clocks. Local reads use `RuntimeFileService`; SSH and Docker call `operator deployment-state --working-dir <dir>` in the target runtime. Missing state reports JSON `null` until the worker publishes progress; malformed state and recorded failures fail deployment. Resumed pending attempts retain their original request and keys. Enrollment requests carry no client-side HTTP timeout; the pending request expiry bounds the approval wait and the Gateway write timeout bounds creation and completion.

After approval, deployment waits for each new process to establish its command subscription and reads the session ID from that ready record, then prints the session IDs for [binding and running commands](#bind-the-cli-to-operators-and-run-commands). The ready phase follows the Gateway's subscription acknowledgement; deployment does not poll the registry for an active status that is assigned during issuance. Any failed deployment or failed readiness check produces a nonzero exit. Readiness is independent of the worker log level. Subscription loss returns the phase to enrolled, and reconnect publishes ready again. Startup failures publish a terminal failed phase with a non-secret error category; detailed diagnostics remain in the startup logs. A completed enrollment is retained on retry. Local/SSH deployments retain `start.log` and `operator.pid` in each directory; Docker reads container logs only to explain a stopped container. Stop workers with `operator stop <operator-session-id>` and revoke enrollment with `auth enroll revoke <request-id>` when retiring them.

The 5000 limit is a deployment range, not a promise that every host can sustain 5000 processes. Size host memory, process/file limits, and Gateway capacity for the intended fleet.

---

## Verify and Operate the Connection

### Check Gateway Health

```bash
./g8e gw status
```

This checks the Gateway HTTP health endpoint, process manager status, connected operators, and Docker Compose stack status (if containers are running).

### List Connected Operators

From an enrolled CLI identity, list Operators associated with your user:

```bash
./g8e operator list --endpoint <gateway-host>
```

This displays each Operator's ID, type (e.g., data, ensemble), hostname, session ID, and status. A status of `stale` means the Gateway has not received a heartbeat from that Operator for more than twice its declared interval (at least 60 seconds); it returns to `active` on the next heartbeat. A stopped or crashed Operator process therefore leaves `active` within about a minute without any manual cleanup. Use `g8e operator show <operator-id-or-session-id>` to display the last heartbeat time, the latest heartbeat snapshot, and performance metrics. Check the Operator process log for the `Channel established - Ready to receive` message as confirmation that it established its pub/sub channel subscription.

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

Use `--all-active` instead of session IDs to target every active Operator you own, and `--concurrency N` to bound in-flight dispatches. Each target Operator must belong to the authenticated user. Binding changes and `operator run` both require an enrolled CLI identity. See [Build Operator](build_operator.md#operate-remote-operators-from-the-cli) and [Authentication and Authorization](../architecture/auth.md#operator-binding).

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

From an enrolled CLI, `g8e operator stop <operator-session-id> [--reason <text>]` stops a remote Operator through its governed shutdown channel. On Linux, a matched local worker gets two seconds to exit, then TERM and another two seconds before KILL; `--grace <duration>` adjusts both waits. Bare `g8e operator stop` stops all local `g8e operator start` workers owned by the current user, including workers missing from the gateway registry or waiting for enrollment. It attempts governed shutdown when a unique session can be identified, and falls back to local termination if the gateway is unavailable. Local termination does not synthesize a gateway shutdown acknowledgement. On the Operator host, send `SIGINT` or `SIGTERM` to the foreground Operator process. It cancels the service context, stops the heartbeat scheduler and pub/sub service, closes local services, and exits after graceful shutdown.

---

## Troubleshooting

### Enrollment Fails Because the Gateway Has No Owner

The Gateway is running but has not been bootstrapped with an owner. The Operator reports the enrollment rejection immediately. Run `g8e auth enroll user --endpoint <gateway-host>` on the Gateway host to create the first owner, then restart the Operator.

### The Operator Waits for Approval

The Operator holds one token-authenticated status request until the owner approves or denies it. It receives the committed decision through the existing Gateway pub/sub owner; it does not poll or back off between status requests. See [Platform workload enrollment](../architecture/auth.md#platform-workload-enrollment).

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
