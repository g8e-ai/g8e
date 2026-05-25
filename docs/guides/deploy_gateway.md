---
title: Deploy Gateway
parent: Guides
---

# Deploy a Governance Gateway

Last Updated: 2026-05-25
Version: v0.2.6

---

## Overview

This guide covers deploying and operating a g8e-compatible Governance Gateway, whether using the reference implementation or a custom g8e-compatible gateway.

---

## Reference Gateway Deployment

### Initialization

Initialize the platform runtime:

```bash
./g8e platform init
```

This creates the `.g8e` directory structure:
- `.g8e/pki/` — PKI hierarchy (CA, certificates, keys)
- `.g8e/data/` — SQLite database for Gateway persistence
- `.g8e/logs/` — Gateway logs
- `.g8e/secrets/` — Encrypted vault for platform secrets

### Starting the Gateway

Start the Gateway in the appropriate mode for your use case:

#### Doctrine Mode (Default)

Enforces L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2 and L3 are audited but not enforced.

```bash
./g8e platform start --mode doctrine
```

#### Consensus Mode

Enforces L1 and L2 (multi-model Byzantine consensus). L3 is audited but not enforced.

```bash
./g8e platform start --mode consensus
```

#### Notary Mode

Enforces L1, L2, and L3 (human-in-the-loop via WebAuthn/FIDO2). This is the most secure mode.

```bash
./g8e platform start --mode notary
```

### Gateway Ports

The Gateway exposes four logical surfaces:

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **Bootstrap** | 8441 (plain HTTP) | None | Trust bundle download, device-link enrollment, CSR signing |
| **Public Port** | 8442 (TLS) | Web session | Browser login, WebAuthn challenge, PKI discovery |
| **mTLS API + Pub/Sub** | 8440 (TLS) | mTLS + URI SAN | `/api/governance/envelope`, `/db`, `/ws/pubsub` |

Ports can be customized via CLI flags or environment variables.

### Health Checks

Check Gateway status:

```bash
./g8e platform status
```

This reports:
- Gateway process status
- Listening ports
- PKI hierarchy status
- Connected Operators

---

## Authentication

### CLI Authentication

The CLI uses mTLS for authentication. Generate a client certificate:

```bash
./g8e login
```

This:
1. Generates a CSR (Certificate Signing Request)
2. Submits it to the Gateway's bootstrap endpoint
3. Receives a signed client certificate
4. Stores it in `.g8e/pki/client.crt`

### Browser Authentication

For web-based interactions, the Gateway supports WebAuthn/FIDO2:

1. Navigate to `https://localhost:8442` (or your configured public port)
2. Follow the on-screen prompts to register a security key
3. Use the key for subsequent authentication

### Device-Link Enrollment

For operator enrollment, generate a device-link token:

```bash
./g8e auth device-link create --name "prod-db-node"
```

This token is used during operator enrollment to authenticate the host.

---

## Using the Gateway

### Envelope Submission

Submit governance envelopes via the HTTP API:

```bash
curl -X POST https://localhost:8440/api/governance/envelope \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

### Protocol Translation

The Gateway automatically translates MCP and A2A requests:

#### MCP (Model Context Protocol)

```bash
curl -X POST https://localhost:8440/api/mcp/v1/tools/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"tool": "shell.execute", "arguments": {"command": "ls -la"}}'
```

#### A2A (Agent-to-Agent)

```bash
curl -X POST https://localhost:8440/api/a2a/v1/skills/execute \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"skill": "file.read", "parameters": {"path": "/etc/hosts"}}'
```

### Data Queries

Query the document store:

```bash
./g8e data store --collection users --document-id <id>
```

Query audit events:

```bash
./g8e data audit list --operator-session-id <session-id>
```

---

## Maintenance

### Log Management

View Gateway logs:

```bash
./g8e platform logs
```

Logs are stored in `.g8e/logs/operator-listen.log`.

### Restart

Restart the Gateway:

```bash
./g8e platform restart
```

### Reset

Reset Gateway data and secrets while preserving the CA:

```bash
./g8e platform reset
```

### Clean

Destructively remove all Gateway state:

```bash
./g8e platform clean
```

Use with caution. This removes all runtime data under `.g8e/`.

### Certificate Management

View device-link tokens:

```bash
./g8e data device-links list
```

Delete a device-link token:

```bash
./g8e data device-links delete --token <token>
```

---

## Reports and Auditing

### Audit Vault

Query audit events for a specific operator session:

```bash
./g8e data audit list --operator-session-id <session-id> --limit 100
```

### Chaos Test Summary

View chaos test summary from the audit vault:

```bash
./g8e data audit summary
```

### Action Receipts

ActionReceipts are emitted for every governed mutation and stored in the audit vault. They are queryable via the protected audit API.

---

## Custom Gateway Deployment

For custom g8e-compatible gateway implementations, deployment follows the same operational pattern:

1. **Initialize PKI**: Generate root CA and intermediate CAs.
2. **Configure Persistence**: Set up the document store, KV store, and blob store.
3. **Configure Ports**: Bind the four logical surfaces to appropriate ports.
4. **Start Gateway**: Launch the gateway in the desired mode (doctrine, consensus, or notary).
5. **Enroll Clients**: Use device-link tokens and CSR-based enrollment to authenticate operators and CLI clients.
6. **Monitor Health**: Implement health checks for the gateway process and connected operators.

### Configuration

Custom gateways should support configuration via:
- CLI flags for runtime parameters (ports, mode, paths)
- Environment variables for deployment-specific settings
- Configuration files for complex deployments

### High Availability

For production deployments, consider:
- Gateway clustering with shared persistence
- Load balancing across multiple gateway instances
- Certificate rotation and revocation automation
- Automated backup of the audit vault and state store

---

## Troubleshooting

### Gateway Fails to Start

Check if ports are already in use:

```bash
./g8e platform status
```

Verify PKI initialization:

```bash
ls -la .g8e/pki/
```

### Authentication Failures

Verify client certificate exists:

```bash
ls -la .g8e/pki/client.crt
```

Re-run login if certificate is missing or expired:

```bash
./g8e login
```

### Operator Connection Issues

Verify device-link token is valid:

```bash
./g8e data device-links list
```

Check Gateway is listening on the mTLS port:

```bash
curl -k https://localhost:8440/healthz
```

---

## Next Steps

- **[Build Operator](build_operator.md)** — Build a custom g8e-compatible Governed Operator.
- **[Deploy Operator](deploy_operator.md)** — Deploy and use a Governed Operator.
- **[Build Apps](build_apps.md)** — Build g8e-compatible applications using a Gateway.
