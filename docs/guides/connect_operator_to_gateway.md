---
title: Connect Operator to Gateway
parent: Guides
---

# Connect g8e Operator to g8e Gateway

Last Updated: 2026-06-26
Version: v1.2.4

---

## Overview

This guide covers connecting the g8e platform to the gateway. The gateway serves as the central policy decision point, while the operator executes governed mutations through the five-layer verification pipeline.

---

## Reference Operator Connection

### Local Deployment

For development or single-host deployments, start the gateway locally:

```bash
./g8e gw start
```

This starts the gateway in doctrine mode (L1 enforced, L2/L3 audited). The gateway performs network identity detection at startup and defaults to full certificate identity mode (all hostnames/IPs).

### Remote Deployment

For distributed infrastructure, deploy the operator on remote hosts.

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

This command copies the operator binary to remote hosts via SCP, makes it executable via SSH, and optionally starts it in the background. Requires `./g8e auth enroll` first. Additional flags include `-P` for SSH port and `-i` for SSH identity file.

**Stream operator binary to remote hosts:**

```bash
./g8e operator stream <host1> <host2> --endpoint <gateway-ip>
```

This command streams the operator binary to remote hosts via native Go crypto/ssh and executes it directly on each host. Supports concurrent streaming, structured JSON output, and advanced SSH configuration. When `--endpoint` is provided, the operator starts automatically on each remote host after the binary is injected. Without `--endpoint`, the binary is saved to a temporary file on the remote host with instructions for manual startup.

Hosts are specified as positional arguments or via `--hosts <file>` (one host per line, or `-` for stdin). Additional flags include `--arch` (target architecture: amd64, arm64, 386), `--concurrency` (max parallel SSH sessions), `--timeout` (per-host dial and inject timeout in seconds), `--ssh-config` (path to SSH config file), `--known-hosts` (path to SSH known_hosts file), `--ssh-identity-file` (SSH identity file path), `--ssh-user` (SSH username), `--ssh-passphrase` (passphrase for encrypted SSH private keys), `--binary-dir` (directory containing architecture-specific operator builds), `--no-git` (disable ledger), and `--preflight` (enable pre-flight SSH connectivity check before binary transfer).

**Copy operator binary locally:**

```bash
./g8e operator cp <target-path>
```

**Copy operator binary to remote host via SCP:**

```bash
./g8e operator scp <user@host:path>
```

This command uses SCP to copy the operator binary to a remote host. Supports common SCP flags including `-P` for SSH port, `-i` for identity file, `-r` for recursive copy, `-p` to preserve attributes, `-v` for verbose output, and `-C` for compression. Use `--prompt` to interactively configure options.

#### 3. CSR-Based Enrollment

On the remote host, generate a CSR and enroll with the gateway:

```bash
./g8e gw security pki enroll -e <gateway-ip>
```

The endpoint is the gateway IP address. The HTTP port (8080) is appended automatically. This command generates an operator CSR, submits it to the gateway, and saves the signed certificates to the PKI directory.

#### 4. Start the Operator

On the remote host, start the operator with the enrolled certificates:

```bash
./g8e operator run -e <gateway-ip>
```

The operator will:
- Load the mTLS certificates from the PKI directory
- Connect to the gateway control plane on port 8443 (HTTPS) and bootstrap on port 8080 (HTTP)
- Initialize the local in-process pub/sub broker
- Initialize the SQLite-backed audit vault with Git ledger
- Execute mutations through the L1-L5 verification pipeline

---

## Operator Configuration

### Gateway Endpoint

The gateway endpoint is configured via the `-e` or `--endpoint` flag during CSR enrollment. The gateway itself does not require an endpoint flag at startup, as it binds to the configured ports.

### PKI Directory

Specify the PKI directory via the `--pki-dir` flag when starting the gateway:

```bash
./g8e gw start --pki-dir /etc/g8e/pki
```

This defaults to `.g8e/pki` in the current working directory. The PKI directory contains the root CA, intermediate CA, operator service certificates, and trust bundles.

---

## Health Checks

Check status:

```bash
./g8e gw status
```

This reports:
- Gateway running state (RUNNING or STOPPED)
- Gateway endpoint URLs (Operator Bootstrap, Public API, MCP HTTP)
- Process PID (when detected via ProcessManager fallback)

---

## Using the Operator

### MCP Tool Calls

AI clients can connect to the gateway's MCP endpoint:

**For mTLS-based MCP:**

```bash
curl -X POST https://localhost:8443/mcp \
  --cert .g8e/credentials/cli.crt \
  --key .g8e/credentials/cli.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shell.execute","arguments":{"command":"ls -la"}}}'
```

> Note: Documentation uses default ports: HTTP 8080, HTTPS 8443.

**For plain HTTP MCP (non-mTLS):**

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shell.execute","arguments":{"command":"ls -la"}}}'
```

**View MCP client configuration matrix:**

```bash
./g8e mcp agent show <agent>
```

Replace `<agent>` with a supported agent (claude, codex, cursor, devin, vscode, continue, aider, codeium, tabby, ollama, gemini, goose, generic). This displays configurations side-by-side for different transport modes (g8e.local mTLS, IP Address mTLS, Stdio Transport).

### Direct Envelope Submission

For direct envelope submission to the gateway:

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

View gateway logs:

```bash
./g8e gw logs -f
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
./g8e auth enroll
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

Note: This is now handled automatically by `./g8e auth enroll` on Windows. This command is for advanced use cases or manual re-enrollment.

---

## Custom Operator Connection

For custom g8e-compatible implementations, the gateway follows the same operational pattern:

1. **Enroll with Gateway**: Use CSR-based enrollment to obtain mTLS certificates via `./g8e gw security pki enroll`.
2. **Configure Runtime**: Set up the data directory, PKI directory, and secrets directory.
3. **Start Gateway**: Launch the gateway with `./g8e gw start`.
4. **Authenticate CLI**: Run `./g8e auth enroll` to obtain client credentials.
5. **Verify Connection**: Confirm the gateway is running via `./g8e gw status`.
6. **Monitor Health**: Implement health checks for the gateway process and audit vault.

### Configuration

Custom operators should support configuration via:
- CLI flags for runtime parameters (gateway URL, paths)
- Configuration files for complex deployments
- Environment variables for vault settings (G8E_VAULT_DIR, G8E_VAULT_KEY, G8E_VAULT_REQUIRE_UNLOCK)

### High Availability

For production deployments, consider:
- Multiple operator instances per host for redundancy
- Automatic restart on failure
- Health check integration with orchestration systems
- Log aggregation and monitoring

---

## Troubleshooting

### Gateway Fails to Start

Verify the gateway is not already running:

```bash
./g8e gw status
```

Check gateway logs for startup errors:

```bash
./g8e gw logs -f
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
./g8e auth enroll
```

If the trust bundle is stale after gateway PKI regeneration:

```bash
./g8e auth logout && ./g8e auth enroll
```

### Audit Vault Errors

Verify audit vault directory exists:

```bash
ls -la .g8e/data/
ls -la .g8e/data/ledger/
```

Check gateway logs for audit vault write errors:

```bash
./g8e gw logs -f
```

Verify write permissions on the data directory:

```bash
./g8e gw security validate --pki-dir .g8e/pki --secrets-dir .g8e/secrets
```

### Authentication Failures

Verify the gateway is running:

```bash
./g8e gw status
```

Verify you have valid client credentials:

```bash
ls -la .g8e/credentials/
```

Re-authenticate if credentials are missing or invalid:

```bash
./g8e auth enroll
```

---

## Security Considerations

### Outbound-Only Connectivity

The gateway operates as a zero-trust boundary. Clients establish outbound-only mTLS connections to the gateway. No inbound ports are required on client hosts. This eliminates NAT traversal requirements and reduces the remote attack surface.

### Local-First Audit

All audit entries are written to the local SQLite-backed audit vault with Git ledger before execution. Raw data, forensic context, and execution history never leave the host. Only sovereignty-scrubbed projections cross the wire via the gateway APIs.

### Fail-Closed Execution

The gateway executes mutations only through the five-layer governance pipeline (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator). Any failure in any layer results in a typed rejection and audit entry. No fallback paths or silent retries exist.

### Certificate Revocation

Certificate revocation is enforced on every mTLS handshake. The gateway maintains a CRL (Certificate Revocation List) and revocation bundle. Revoked certificates are immediately rejected, preventing unauthorized access.

---

## Next Steps

- **[Build Apps](./build_apps.md)**: Build g8e-compatible applications using a gateway.
