---
title: Connect Operator to Gateway
parent: Guides
---

# Connect g8e Operator to g8e Gateway

Last Updated: 2026-09-08
Version: v2.1.7

---

## Overview

The g8e Operator is the host-side Policy Execution Point. It opens an outbound mTLS WebSocket connection to the Gateway, subscribes to its identity- and session-scoped command channel, verifies governed transactions, executes approved actions on its host, and publishes signed receipts and heartbeats. The Operator does not expose an inbound MCP, A2A, or command listener.

Connecting a new Operator has four parts:

1. Start the Gateway.
2. Enroll the first Gateway owner if the Gateway is not already bootstrapped.
3. Start the Operator with the Gateway endpoint. The Operator submits a platform enrollment request and waits.
4. Approve the request from an enrolled owner CLI. The Operator installs its credentials and connects automatically.

The reference Gateway and Operator are commands in the same `g8e` binary. See [Build Operator](build_operator.md) for build and execution internals and [Operator Architecture](../architecture/operator.md) for the service design.

---

## Prerequisites

The Gateway host requires:

- A running `g8e` Gateway.
- TCP ports 8080 and 8443 reachable from the Operator host. Port 8080 serves discovery, trust-bundle, and platform-enrollment routes. Port 8443 serves the mTLS API and WebSocket pub/sub connection.
- A certificate identity that matches the hostname used by the Operator. The default `full` certificate mode detects hostnames and IP addresses at Gateway startup; `--cert-mode localhost` is suitable only for same-host connections.
- An enrolled owner CLI to approve the Operator request.

The Operator host requires:

- The `g8e` binary and a writable launch directory. The process creates its `.g8e/` runtime tree below that directory.
- Outbound access to Gateway ports 8080 and 8443.
- A working directory containing the files and resources that governed actions may access. Use `--working-dir` to select the command-execution directory; this option does not relocate the `.g8e/` runtime tree.

No inbound port is required on the Operator host.

### Endpoint Requirements

Use a bare hostname with `g8e operator start --endpoint`, without an `http://` or `https://` scheme and without a port. The current worker uses Gateway port 8080 for discovery and enrollment and port 8443 for mTLS bootstrap, state queries, receipt publication, and pub/sub. Although the root command displays a global `--port` flag, `operator start` does not apply it to the worker configuration in this version; non-default Gateway ports are not supported by this command path.

The hostname must resolve on the Operator host and must appear in the Gateway certificate. For a remote Gateway without DNS, map the Gateway address to the built-in `g8e.local` identity on the Operator host, then use `g8e.local` as the endpoint. For example, add this host mapping using the administration mechanism for the target operating system:

```text
192.0.2.10 g8e.local
```

Replace `192.0.2.10` with the Gateway address. Do not use `--cert-mode localhost` for this remote connection.

---

## Connect a New Operator

### 1. Start the Gateway

On the Gateway host, start the service:

```bash
./g8e gw start
```

The Gateway starts in the background by default. Use `--follow` to run it in the foreground or `./g8e gw logs -f` to follow a background process. The default posture is `doctrine`; `consensus`, `ratify`, and `notary` are also available through `--posture`.

To preview settings with the interactive wizard, run:

```bash
./g8e gw setup
```

The wizard resolves posture, consensus, passkey, CORS, downstream, public URL, and certificate settings and prints the result; `gw setup` does not persist or start that configuration. Use `gw start --interactive` to run the wizard and start the Gateway with the resolved values in the same invocation.

Verify the Gateway process and HTTP health endpoint:

```bash
./g8e gw status
```

### 2. Enroll the Gateway Owner

A new Gateway starts with no users and does not issue platform workload credentials until the first owner enrolls. On the Gateway host, or on an owner workstation configured to reach it, run:

```bash
./g8e auth enroll user --endpoint <gateway-host>
```

For a same-host Gateway, omit `--endpoint` or use `localhost`. Enrollment creates the first user and CLI session on an unbootstrapped Gateway, installs the Gateway root CA into the operating-system trust store by default, and runs the browser passkey ceremony. Use `--no-system-trust` only when an administrator has already installed the root CA. Use `--headless` for an mTLS-only CLI identity when another enrolled owner can approve the recovery request; a headless identity cannot authenticate to the Console SPA.

The first enrolled user is the persistent owner authorized to approve platform workload enrollment requests.

### 3. Start the Operator

On the Operator host, start the foreground worker:

```bash
./g8e operator start --endpoint <gateway-host>
```

For a same-host deployment, use:

```bash
./g8e operator start --endpoint localhost
```

If Operator credentials are not installed, the process:

1. Fetches the Gateway trust bundle from `http://<gateway-host>:8080/.well-known/g8e/pki/ca-bundle`.
2. Generates Operator and CLI ECDSA P-256 keys and certificate signing requests.
3. Submits an owner-approved platform enrollment request.
4. Writes resumable pending state to `.g8e/pki/pending-enrollment/g8eo.json` with private-file permissions.
5. Prints the request ID, CSR fingerprints, and approval command, then polls for a decision.

Leave this process running while the owner approves the request. If the process stops, rerunning the same command from the same launch directory resumes the request with the same pending state and key material. A pending request expires if it is not approved within its server-provided enrollment window.

`g8e auth enroll operator` is not a current CLI command. Operator enrollment is part of `g8e operator start --endpoint`.

### 4. Inspect and Approve the Request

From the enrolled owner CLI, list pending platform enrollment requests:

```bash
./g8e auth pending-platform-enrollments --endpoint <gateway-host>
```

Compare the displayed component, hostname, system fingerprint, and Operator and CLI key fingerprints with the requesting Operator's output. Approve the matching request:

```bash
./g8e auth approve-platform-enrollment <request-id> --endpoint <gateway-host>
```

The command displays the request details and asks for confirmation. For non-interactive operation after independently validating the request, add `--yes`. To reject a request, use `--deny`; `--reason` attaches an optional decision note.

Only a valid, non-revoked CLI identity belonging to the first enrolled owner can approve or deny the request. The Gateway enforces this authorization.

### 5. Wait for the Connection

After approval, the Operator signs the completion transcript with both generated private keys, receives the issued certificates, validates the response, and writes:

- `.g8e/pki/operator.crt`
- `.g8e/pki/operator.key`
- `.g8e/pki/cli.crt`
- `.g8e/pki/cli.key`
- `.g8e/pki/trust/g8eg-ca-bundle.pem`

The process removes the pending state after those writes succeed. It then authenticates to the Gateway over mTLS, receives its Operator ID, Operator session ID, runtime limits, posture, and heartbeat configuration, initializes encrypted local storage and execution services, and subscribes to:

```text
cmd:<operator-id>:<operator-session-id>
```

A successful connection logs `Channel established - Ready to receive`. The Operator sends an immediate heartbeat and continues at the configured interval, which defaults to 30 seconds.

---

## Use Pre-Provisioned Credentials

To start with an existing Operator identity, provide the certificate, matching private key, and Gateway trust bundle explicitly:

```bash
./g8e operator start \
  --endpoint <gateway-host> \
  --cert /path/to/operator.crt \
  --key /path/to/operator.key \
  --trust-bundle /path/to/g8eg-ca-bundle.pem
```

Without explicit paths, the worker looks for `.g8e/pki/operator.crt`, `.g8e/pki/operator.key`, and `.g8e/pki/trust/g8eg-ca-bundle.pem` below its launch directory. Both the certificate and key must be present and form a valid pair. Partial credential state does not trigger automatic enrollment and startup fails closed.

The current worker runs in the foreground. Use the target operating system's service manager or container orchestrator to supervise it in production and preserve its launch directory across restarts.

---

## Deploy the Binary to a Remote Host

`g8e operator cp <target>` copies the current binary to a local target. `g8e operator scp <user@host:path>` invokes the system `scp` client to copy it to a remote host. After copying, make the binary executable where required and start it on the target host with `g8e operator start --endpoint <gateway-host>`.

Do not use `operator deploy --background` as an Operator rollout path in this version: its current implementation starts `gw start` on each target rather than `operator start`. The current Cobra wrapper for `operator stream` also consumes its public flags before its native parser receives them, so `--endpoint`, `--hosts`, and related options do not provide a reliable automated rollout. Use `cp`, `scp`, or an external deployment system and start the worker explicitly.

---

## Verify and Operate the Connection

### Verify Gateway Health

```bash
./g8e gw status
```

This checks the Gateway HTTP health endpoint, falls back to the local process manager for host mode, and also reports the Docker Compose stack. It verifies the Gateway, not the remote Operator process.

### Inspect Enrolled Operators

From an enrolled CLI identity:

```bash
./g8e operator list --endpoint <gateway-host>
```

The command lists Operator records associated with the enrolled user, including Operator ID, type, cloud subtype, Operator session ID, and recorded status. Use the Operator process log and the `Channel established - Ready to receive` message as the direct confirmation that the current worker established its pub/sub channel.

### View Gateway Logs

```bash
./g8e gw logs -f
```

### Restart or Stop the Gateway

```bash
./g8e gw restart
./g8e gw stop
```

Gateway restart preserves the persisted posture. A running Operator detects a closed pub/sub stream and retries the connection with bounded backoff; supervise the worker so it restarts if its retry limit is exhausted.

### Stop the Operator

Send `SIGINT` or `SIGTERM` to the foreground Operator process. It cancels the service context, stops the heartbeat scheduler and pub/sub service, closes local services, and exits after graceful shutdown.

---

## Troubleshooting

### The Operator Waits Before Submitting an Enrollment Request

The Gateway is running but has not been bootstrapped with its first owner. The Operator retries submission with bounded backoff. Run `g8e auth enroll user` against the Gateway, then leave or restart the Operator process so it can submit the platform request.

### The Operator Waits for Approval

List requests from the owner CLI:

```bash
./g8e auth pending-platform-enrollments --endpoint <gateway-host>
```

Approve the matching request ID after comparing its fingerprints. Restarting the requesting Operator from the same launch directory resumes the persisted request; starting it from another directory creates or uses a different `.g8e/` runtime tree.

### Trust-Bundle Fetch Fails

Confirm that the Operator host can reach the Gateway discovery port and that the endpoint is a bare resolvable hostname:

```bash
curl -fsS http://<gateway-host>:8080/.well-known/g8e/pki/ca-bundle
```

If this request fails, check routing, firewall rules, Gateway health, and hostname resolution. If it succeeds but Operator startup rejects the bundle, inspect the Gateway and Operator logs for certificate parsing or trust errors rather than bypassing TLS validation.

### mTLS Bootstrap or Pub/Sub Fails

Confirm all of the following:

- The Operator uses the same launch directory that contains its enrolled `.g8e/pki/` state.
- The endpoint has no URL scheme or port.
- The endpoint hostname resolves on the Operator host.
- The hostname appears in the Gateway certificate identity generated at startup.
- TCP port 8443 is reachable.
- The Operator certificate and key are a matching, non-expired pair.
- The canonical trust bundle is `.g8e/pki/trust/g8eg-ca-bundle.pem`.

For an IP-only remote deployment, map the Gateway IP to `g8e.local` and use `--endpoint g8e.local`. Do not disable certificate verification.

### Startup Reports Missing Operator Session ID

The mTLS reauthentication/bootstrap request must return the Operator ID and Operator session ID before local audit services start. Confirm that enrollment completed, the certificate identifies the enrolled Operator, and the Gateway still contains that Operator session. `auth enroll user` manages CLI credentials and does not repair an Operator identity.

### Execution Vault Initialization Fails

The execution vault is enabled by default and is required for replay protection. Do not start the worker with `--execution-vault=false`. Ensure the launch directory is writable and that the existing `.g8e/vault/` key can unlock its encrypted state. See [Build Operator](build_operator.md#local-runtime-state) for the Operator runtime layout and vault administration commands.

---

## Security Properties

- **Owner-approved enrollment:** A new workload receives no platform certificate until the persistent Gateway owner approves its request over an authenticated mTLS CLI session.
- **Proof of key possession:** The Operator signs the enrollment completion transcript with both generated private keys before credentials are issued.
- **Outbound-only worker:** The Operator initiates its Gateway connections and exposes no inbound application listener.
- **Scoped command channel:** The Operator subscribes to a channel bound to its Operator and session identities.
- **Fail-closed execution:** The Operator verifies transaction structure, expiry, replay state, hash and state binding, doctrine, and posture-required L2 and L3 evidence before L5 execution.
- **Local-first evidence:** The Operator persists signed execution evidence and encrypted local state before publishing result projections to the Gateway.
- **mTLS and revocation:** Gateway authenticated routes validate the certificate identity and revocation state in addition to normal TLS chain and hostname checks.

---

## Next Steps

- [Build Operator](build_operator.md): Build the reference worker, understand runtime state, and review the current processing contract.
- [Operator Architecture](../architecture/operator.md): Review the Operator service stack and execution boundary.
- [Build Apps](build_apps.md): Connect MCP, A2A, and direct-envelope clients to the Gateway.
- [Protocol Library](../architecture/protocol.md): Use the public protobuf schemas, constants, and workload identity helpers.
