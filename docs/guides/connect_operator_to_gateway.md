---
title: Connect Operator to Gateway
parent: Guides
---

# Connect g8e Operator to g8e Gateway

Last Updated: 2026-06-10
Version: v1.0.11

---

## Overview

This guide covers connecting the g8e platform to the g8e Gateway. The g8e Gateway serves as the central policy decision point, while the g8e Operator executes governed mutations through the five-layer verification pipeline.

---

## Reference Operator Connection

### Local Deployment

For development or single-host deployments, start the g8e Gateway locally:

```bash
./g8e gw start
```

This starts the g8e Gateway in doctrine mode (L1 enforced, L2/L3 audited). The g8e Gateway performs network identity detection at startup and defaults to full certificate identity mode (all hostnames/IPs).

### Remote Deployment

For distributed infrastructure, deploy the g8e Operator on remote hosts.

#### 1. Copy/Paste Deploy Scripts (Gateway-Served)

The gateway embeds deploy scripts and serves them over HTTP on port 8080. After starting the gateway on the host machine, run these commands on remote hosts to download and deploy the g8e binary:

**Linux/macOS:**

```bash
curl -fsSL http://<gateway-ip>:8080/g8e-operator.sh | bash
```

**Windows:**

```powershell
iwr http://<gateway-ip>:8080/g8e-operator.ps1 -UseBasicParsing | iex
```

#### 2. Operator Remote Management CLI Commands

The gateway provides CLI commands to deploy and manage operators on remote hosts via SSH:

**Deploy operator binary to remote hosts:**

```bash
./g8e operator deploy --hosts <host1,host2> --background
```

This command copies the operator binary to remote hosts via SSH and optionally starts it in the background. Requires `./g8e auth login` first.

**Stream operator binary to remote hosts:**

```bash
./g8e operator stream --hosts <host1,host2>
```

This command streams the operator binary via SSH and executes it directly on remote hosts without copying. Useful for quick deployments or air-gapped scenarios.

**Copy operator binary locally:**

```bash
./g8e operator cp <target-path>
```

**Copy operator binary to remote host via scp:**

```bash
./g8e operator scp <user@host:path>
```

#### 3. CSR-Based Enrollment

On the remote host, generate a CSR and enroll with the g8e Gateway:

```bash
./g8e gw security pki enroll -e <gateway-ip>
```

The endpoint is the g8e Gateway IP address. The HTTP port (8080) is appended automatically. This command generates g8e Operator and CLI CSRs, submits them to the g8e Gateway, and saves the signed certificates to the PKI directory.

#### 4. Start the g8e Gateway

On the remote host, start the g8e Gateway with the enrolled certificates:

```bash
./g8e gw start
```

The g8e Gateway will:
- Load the mTLS certificates from the PKI directory
- Establish the control plane on port 8443 (HTTPS) and bootstrap on port 8080 (HTTP)
- Initialize the local in-process Pub/Sub broker
- Initialize the SQLite-backed audit vault with Git ledger
- Execute mutations through the L1/L2/L3/L4/L5 verification pipeline

---

## g8e Operator Configuration

### g8e Gateway Endpoint

The g8e Gateway endpoint is configured via the `--endpoint` flag during CSR enrollment. The g8e Gateway itself does not require an endpoint flag at startup, as it binds to the configured ports.

### PKI Directory

Specify the PKI directory via the `--pki-dir` flag when starting the g8e Gateway:

```bash
./g8e gw start --pki-dir /etc/g8e/pki
```

This defaults to `.g8e/pki` in the current working directory. The PKI directory contains the root CA, intermediate CA, g8e Gateway service certificates, and trust bundles.

---

## Health Checks

Check status:

```bash
./g8e gw status
```

This reports:
- Gateway process status and PID
- Gateway endpoint URLs (Operator Bootstrap on HTTPS, Public API on HTTPS, MCP HTTP)
- Gateway running state (RUNNING or STOPPED)

---

## Using the Operator

### MCP Tool Calls

AI clients can connect to the Gateway's MCP endpoint:

**For mTLS-based MCP:**

```bash
curl -X POST https://localhost:8443/api/v1/mcp/tools/call \
  --cert .g8e/credentials/cli.crt \
  --key .g8e/credentials/cli.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shell.execute","arguments":{"command":"ls -la"}}}'
```

**For plain HTTP MCP (non-mTLS):**

```bash
curl -X POST http://localhost:8080/api/v1/mcp/tools/call \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shell.execute","arguments":{"command":"ls -la"}}}'
```

**View MCP client configuration matrix:**

```bash
./g8e mcp show
```

This displays configurations side-by-side for different transport modes (stdio, HTTP, HTTPS).

### Direct Envelope Submission

For direct envelope submission to the Gateway:

```bash
curl -X POST https://localhost:8443/api/v1/governance/envelopes \
  --cert .g8e/credentials/cli.crt \
  --key .g8e/credentials/cli.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

The envelope must include `transaction_hash`, `nonce`, `expires_at`, and `state_merkle_root` fields. L4 Warden validates these before execution.

---

## Maintenance

### Log Management

View Gateway logs:

```bash
./g8e gw logs --follow
```

### Restart

Restart:

```bash
./g8e gw restart
```

### Stop

Stop:

```bash
./g8e gw stop
```

### Certificate Renewal

When the mTLS certificate expires, re-authenticate using:

```bash
./g8e auth login
```

This command automatically checks certificate expiry and performs auto-renewal if needed. For remote device enrollment, use CSR-based enrollment:

```bash
./g8e gw security pki enroll -e <gateway-ip>
```

**Windows Passkey Enrollment:**

On Windows, enroll via the Windows Certificate Store:

```bash
./g8e auth enroll-windows
```

Note: This is now handled automatically by `./g8e auth login` on Windows. This command is for advanced use cases or manual re-enrollment.

---

## Custom Operator Connection

For custom g8e-compatible implementations, the Gateway follows the same operational pattern:

1. **Enroll with Gateway**: Use CSR-based enrollment to obtain mTLS certificates via `./g8e gw security pki enroll`.
2. **Configure Runtime**: Set up the data directory, PKI directory, and secrets directory.
3. **Start Gateway**: Launch the Gateway with `./g8e gw start`.
4. **Authenticate CLI**: Run `./g8e auth login` to obtain client credentials.
5. **Verify Connection**: Confirm the Gateway is running via `./g8e gw status`.
6. **Monitor Health**: Implement health checks for the Gateway process and audit vault.

### Configuration

Custom operators should support configuration via:
- CLI flags for runtime parameters (gateway URL, paths)
- Configuration files for complex deployments
- Environment variables for vault settings (G8E_VAULT_DIR, G8E_VAULT_KEY, G8E_VAULT_REQUIRE_UNLOCK)

### High Availability

For production deployments, consider:
- Multiple Operator instances per host for redundancy
- Automatic restart on failure
- Health check integration with orchestration systems
- Log aggregation and monitoring

---

## Troubleshooting

### Gateway Fails to Start

Verify the Gateway is not already running:

```bash
./g8e gw status
```

Check Gateway logs for startup errors:

```bash
./g8e gw logs --follow
```

Verify PKI directory exists and contains valid certificates:

```bash
ls -la .g8e/pki/
```

### Certificate Errors

Verify PKI directory exists and contains valid certificates:

```bash
ls -la .g8e/pki/
```

Verify credentials directory exists and contains client certificates:

```bash
ls -la .g8e/credentials/
```

Re-enroll if certificates are missing or expired:

```bash
./g8e auth login
```

If the trust bundle is stale after Gateway PKI regeneration:

```bash
./g8e auth logout && ./g8e auth login
```

### Audit Vault Errors

Verify audit vault directory exists:

```bash
ls -la .g8e/data/
ls -la .g8e/data/ledger/
```

Check Gateway logs for audit vault write errors:

```bash
./g8e gw logs --follow
```

Verify write permissions on the data directory:

```bash
./g8e gw security validate
```

### Authentication Failures

Verify the Gateway is running:

```bash
./g8e gw status
```

Verify you have valid client credentials:

```bash
ls -la .g8e/credentials/
```

Re-authenticate if credentials are missing or invalid:

```bash
./g8e auth login
```

---

## Security Considerations

### Outbound-Only Connectivity

The Gateway operates as a zero-trust boundary. Clients establish outbound-only mTLS connections to the Gateway. No inbound ports are required on client hosts. This eliminates NAT traversal requirements and reduces the remote attack surface.

### Local-First Audit

All audit entries are written to the local SQLite-backed audit vault with Git ledger before execution. Raw data, forensic context, and execution history never leave the host. Only sovereignty-scrubbed projections cross the wire via the Gateway APIs.

### Fail-Closed Execution

The Gateway executes mutations only through the five-layer governance pipeline (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator). Any failure in any layer results in a typed rejection and audit entry. No fallback paths or silent retries exist.

### Certificate Revocation

Certificate revocation is enforced on every mTLS handshake. The Gateway maintains a CRL (Certificate Revocation List) and revocation bundle. Revoked certificates are immediately rejected, preventing unauthorized access.

---

## Next Steps

- **[Build Apps](build_apps.md)** — Build g8e-compatible applications using a Gateway.
