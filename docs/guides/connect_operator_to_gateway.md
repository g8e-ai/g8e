---
title: Connect Operator to Gateway
parent: Guides
---

# Connect Operator to a Governance Gateway

Last Updated: 2026-05-31
Version: v1.0.4

---

## Overview

This guide covers connecting a Governed Operator to a Governance Gateway and operating it, whether using the reference implementation or a custom g8e-compatible operator.

---

## Reference Operator Connection

### Local Deployment

For development or single-host deployments, start the Gateway locally:

```bash
./g8e gw start
```

This starts the Gateway in doctrine mode (L1 enforced, L2/L3 audited).

### Remote Deployment

For distributed infrastructure, deploy the operator on remote hosts:

#### 1. CSR-Based Enrollment

On the Gateway, generate a CSR for the remote host:

```bash
./g8e security pki enroll --endpoint <gateway-ip>
```

#### 2. Copy Binary and Certificates

Copy the `g8e` binary and the issued certificates to the remote host.

#### 3. Start the Operator

On the remote host, start the operator with the certificates:

The operator will:
- Establish an outbound-only mTLS tunnel to the Gateway
- Subscribe to command events on the Pub/Sub broker
- Execute mutations through the L1/L2/L3 verification layers
- Write audit entries to the local Git-backed vault

---

## Operator Configuration

### Gateway Endpoint

Specify the Gateway endpoint via the `--endpoint` flag when starting the operator:

```bash
./g8e --endpoint gateway.example.com
```

### PKI Directory

Specify the PKI directory via the `--pki-dir` flag when starting the Gateway:

```bash
./g8e gw start --pki-dir /etc/g8e/pki
```

This defaults to `.g8e/pki` in the current working directory.

---

## Health Checks

Check status:

```bash
./g8e gw status
```

This reports:
- Operator process status
- Gateway connection status
- Subscription status
- Local audit vault health

---

## Using the Operator

### MCP Tool Calls

AI clients can connect to the Gateway's MCP endpoint:

```bash
# For HTTP-based MCP
curl -X POST https://localhost:8440/api/v1/mcp/tools/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shell.execute","arguments":{"command":"ls -la"}}}'
```

### Direct Envelope Submission

For direct envelope submission to the Gateway:

```bash
curl -X POST https://localhost:8440/api/v1/governance/envelopes \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

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

For remote device enrollment, use CSR-based enrollment:

```bash
./g8e security pki enroll --endpoint <gateway-ip>
```

---

## Custom Operator Connection

For custom g8e-compatible operator implementations, connection follows the same operational pattern:

1. **Enroll with Gateway**: Use CSR-based enrollment to obtain mTLS certificates.
2. **Configure Runtime**: Set up the runtime directory, PKI directory, and audit vault.
3. **Configure Gateway URL**: Specify the Gateway endpoint for outbound mTLS connection.
4. **Start Operator**: Launch the operator in standard mode.
5. **Verify Connection**: Confirm the operator is subscribed to the Pub/Sub broker.
6. **Monitor Health**: Implement health checks for the operator process and Gateway connection.

### Configuration

Custom operators should support configuration via:
- CLI flags for runtime parameters (gateway URL, paths)
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
curl -k https://<gateway-ip>:8440/api/v1/health
```

Verify certificates are valid:

```bash
./g8e data operators list
```

Check Gateway logs for connection errors:

```bash
./g8e gw logs --follow
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

Check Gateway logs for audit vault write errors:

```bash
./g8e gw logs --follow
```

### Pub/Sub Subscription Failures

Verify Gateway is running:

```bash
./g8e gw status
```

Check Gateway logs for subscription errors:

```bash
./g8e gw logs --follow
```

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
