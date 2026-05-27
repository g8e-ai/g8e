---
title: Connect Operator to Gateway
parent: Guides
---

# Connect Operator to a Governance Gateway

Last Updated: 2026-05-25
Version: v1.0.0

---

## Overview

This guide covers connecting a Governed Operator to a Governance Gateway and operating it, whether using the reference implementation or a custom g8e-compatible operator.

---

## Reference Operator Connection

### Local Deployment

For development or single-host deployments, start the Gateway locally:

```bash
./g8e platform start
```

This starts the Gateway in doctrine mode (L1 enforced, L2/L3 audited).

### Remote Deployment

For distributed infrastructure, deploy the operator on remote hosts:

#### 1. Generate Device-Link Token

On the Gateway, generate a device-link token for the remote host:

```bash
./g8e data device-links create --user-id "prod-db-node"
```

#### 2. Copy Binary and Token

Copy the `g8e` binary and the device-link token to the remote host.

#### 3. Start the Operator

On the remote host, start the operator with the token:

```bash
./g8e --device-token <token> --endpoint <gateway-ip>
```

The operator will:
- Establish an outbound-only mTLS tunnel to the Gateway
- Subscribe to command events on the Pub/Sub broker
- Execute mutations through the L1/L2/L3 gauntlet
- Write audit entries to the local Git-backed vault

### MCP Mode

For editor integrations that require stdio-based MCP (Cursor, Claude Code):

```bash
./g8e --mcp-serve
```

This spins up a local proxy that forwards stdio JSON-RPC calls to the Gateway's mTLS API.

---

## Operator Configuration

### Gateway Endpoint

Specify the Gateway endpoint via the `--endpoint` flag:

```bash
./g8e --endpoint gateway.example.com
```

### Device-Link Token

Specify the device-link token via the `--device-token` flag:

```bash
./g8e --device-token <token>
```

### Working Directory

Specify the working directory via the `--working-dir` flag:

```bash
./g8e --working-dir /var/lib/g8e
```

This defaults to the current working directory. All data is stored in `.g8e/` within this directory.

### PKI Directory

Specify the PKI directory via the `--pki-dir` flag:

```bash
./g8e --pki-dir /etc/g8e/pki
```

This defaults to `.g8e/pki` in the current working directory.

---

## Health Checks

Check status:

```bash
./g8e platform status
```

This reports:
- Operator process status
- Gateway connection status
- Subscription status
- Local audit vault health

---

## Using the Operator

### MCP Tool Calls

AI clients can connect to the operator's MCP endpoint:

```bash
# For HTTP-based MCP
curl -X POST http://localhost:9000/tools/call \
  -H "Content-Type: application/json" \
  -d '{"tool": "shell.execute", "arguments": {"command": "ls -la"}}'
```

### A2A Skill Invocations

A2A skill invocations are similarly translated:

```bash
curl -X POST http://localhost:9000/skills/execute \
  -H "Content-Type: application/json" \
  -d '{"skill": "file.read", "parameters": {"path": "/etc/hosts"}}'
```

### Direct Envelope Submission

For direct envelope submission to the operator:

```bash
curl -X POST http://localhost:9000/api/governance/envelope \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

---

## Maintenance

### Log Management

View operator logs:

```bash
tail -f .g8e/logs/operator.log
```

Logs are stored in `.g8e/logs/operator.log`.

### Restart

Restart:

```bash
./g8e platform restart
```

### Stop

Stop:

```bash
./g8e platform stop
```

### Certificate Renewal

When the mTLS certificate expires, re-authenticate using the device-link token:

```bash
./g8e auth login
```

---

## Custom Operator Connection

For custom g8e-compatible operator implementations, connection follows the same operational pattern:

1. **Enroll with Gateway**: Use device-link token and CSR-based enrollment to obtain mTLS certificates.
2. **Configure Runtime**: Set up the runtime directory, PKI directory, and audit vault.
3. **Configure Gateway URL**: Specify the Gateway endpoint for outbound mTLS connection.
4. **Start Operator**: Launch the operator in standard mode or MCP mode.
5. **Verify Connection**: Confirm the operator is subscribed to the Pub/Sub broker.
6. **Monitor Health**: Implement health checks for the operator process and Gateway connection.

### Configuration

Custom operators should support configuration via:
- CLI flags for runtime parameters (gateway URL, device token, paths)
- Environment variables for deployment-specific settings
- Configuration files for complex deployments

### High Availability

For production deployments, consider:
- Multiple operator instances per host for redundancy
- Automatic restart on failure
- Health check integration with orchestration systems
- Log aggregation and monitoring

---

## Troubleshooting

### Operator Fails to Connect to Gateway

Verify Gateway is reachable:

```bash
curl -k https://<gateway-ip>:8440/healthz
```

Verify device-link token is valid:

```bash
./g8e data device-links list
```

Check operator logs for connection errors:

```bash
tail -f .g8e/logs/operator.log
```

### Certificate Errors

Verify PKI directory exists and contains valid certificates:

```bash
ls -la .g8e/pki/
```

Re-enroll if certificates are missing or expired:

```bash
./g8e auth login
```

### Audit Vault Errors

Verify audit vault directory exists:

```bash
ls -la .g8e/data/
```

Check operator logs for audit vault write errors.

### Pub/Sub Subscription Failures

Verify Gateway Pub/Sub broker is running:

```bash
./g8e platform status
```

Check operator logs for subscription errors.

---

## Security Considerations

### Outbound-Only Connectivity

The operator establishes an outbound-only mTLS tunnel to the Gateway. No inbound ports are required on the operator host. This eliminates NAT traversal requirements and reduces the remote attack surface.

### Local-First Audit

All audit entries are written to the host-local Git-backed vault before execution. Raw data, forensic context, and execution history never leave the host. Only sovereignty-scrubbed projections cross the wire to the Gateway.

### Fail-Closed Execution

The operator executes mutations only through the Actuator, the single fail-closed dispatch path. Any failure in L1, L2, or L3 results in a typed rejection and audit entry. No fallback paths or silent retries exist.

### Certificate Revocation

Certificate revocation is enforced on every mTLS handshake. Revoked certificates are immediately rejected, preventing unauthorized access.

---

## Next Steps

- **[Build Apps](build_apps.md)** — Build g8e-compatible applications using a Gateway.
