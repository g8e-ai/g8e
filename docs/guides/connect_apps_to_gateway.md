---
title: Connect Apps to Gateway
parent: Guides
---

# Connect Apps to g8e Gateway

Last Updated: 2026-06-02
Version: v1.0.8

---

## Overview

This guide covers connecting applications to the g8e Gateway. The g8e Gateway serves as the central Policy Decision Point (PDP) that enforces 3-layer Byzantine Fault Tolerant governance over all AI agent mutations. Applications connect via multiple protocol surfaces: MCP (Model Context Protocol), A2A (Agent-to-Agent), direct governance envelopes, WebSocket pub/sub, and HTTP APIs.

---

## Reference g8e Gateway Connection

### Initialization

Start the g8e Gateway to initialize the platform runtime:

```bash
./g8e gw start
```

This creates the `.g8e` directory structure:
- `.g8e/pki/` - PKI hierarchy (CA, certificates, keys)
- `.g8e/data/` - SQLite database for g8e Gateway persistence
- `.g8e/logs/` - g8e Gateway logs
- `.g8e/secrets/` - Encrypted vault for platform secrets

### Starting the g8e Gateway

Start the g8e Gateway:

```bash
./g8e gw start
```

The g8e Gateway runs in the default mode (doctrine: L1 enforced, L2/L3 audited). To run in different enforcement modes, use the `--posture` flag:

#### Doctrine Mode (Default)

Enforces L1 technical bedrock (forbidden patterns, blacklist, whitelist). L2/L3 signatures are audited but not required.

```bash
./g8e gw start --posture doctrine
```

#### Consensus Mode

Enforces L1 and L2 (multi-model Byzantine consensus). L3 signature is audited but not required.

```bash
./g8e gw start --posture consensus
```

#### Notary Mode

Enforces L1, L2, and L3 (human-in-the-loop via WebAuthn/FIDO2). This is the most secure mode.

```bash
./g8e gw start --posture notary
```

### g8e Gateway Ports

The g8e Gateway exposes four logical protocol surfaces. Each surface serves a specific purpose with distinct authentication requirements.

| Surface | Port (default) | Auth | Purpose |
|---|---|---|---|
| **Public Port** | 8443 (TLS) | Web session | Browser login, WebAuthn challenge, PKI discovery, OOB approval UI |
| **mTLS API + Pub/Sub** | 8440 (TLS) | mTLS + URI SAN | Governance envelopes, MCP/A2A APIs, document store, WebSocket pub/sub |

### Port Multiplexing

The g8e Gateway enforces strict port separation for security:
- **mTLS only**: `tls.RequireAndVerifyClientCert` for strict mutual TLS on the execution boundary
- **Public only**: TLS without client certificate requirement for browser-based access

Port mixing is prohibited. The gateway fails startup if incompatible surfaces (e.g., mTLS and Public) are assigned to the same port, as this would force a downgrade to `VerifyClientCertIfGiven` and weaken the execution boundary to an L7 check.

Ports can be customized via CLI flags.

### Health Checks

Check Gateway status:

```bash
./g8e gw status
```

This reports:
- Gateway process status
- Listening ports
- PKI hierarchy status
- Connected Operators

The Gateway provides a unified health endpoint across all services for consistent health checking.

---

## Connectivity Methods

### 1. MCP (Model Context Protocol)

MCP is a JSON-RPC 2.0 protocol for AI tool invocation. The Gateway translates MCP tool calls into governance envelopes, runs them through L1/L2/L3 verification, and dispatches to downstream MCP servers or local execution.

The Gateway provides a unified MCP endpoint architecture with a comprehensive input validation framework that enforces fail-closed security principles for all tool inputs.

#### Native Tools

The Gateway includes a registry of native tools that execute locally with full governance enforcement. These tools are categorized by domain:

**Database Tools**
- `db_discover_topology` - Scans database schemas, tables, and column data types
- `db_index_triage` - Queries fragmentation statistics and indexes
- `db_isolated_read` - Executes SELECT statements in read-only mode with SQL validation
- `db_query_validate` - Validates SQL queries using EXPLAIN QUERY PLAN

**Filesystem Tools**
- `fs_disk_profile` - Recursively calculates directory sizes and disk usage
- `log_stream_filter` - Reads log files and applies regex filtering with automatic secret scrubbing

**Network Tools**
- `net_endpoint_ping` - Performs TCP handshake or ICMP ping to verify connectivity
- `net_http_probe` - Performs lightweight HTTP requests to probe web endpoints with SSRF protection
- `net_socket_audit` - Inspects active network sockets with protocol validation

**Process Tools**
- `proc_metric_top` - Parses /proc to extract top resource-consuming processes
- `proc_signal_safe` - Sends signals to processes with denylist enforcement for protected PIDs

**System Tools**
- `sys_oom_detect` - Scans system logs for OOM killer events

All native tools include input validation to prevent SQL injection, SSRF attacks, path traversal, and other security vulnerabilities.

#### MCP Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/mcp/tools/list` | POST/GET | List available tools |
| `/api/v1/mcp/tools/call` | POST | Invoke a tool |
| `/api/v1/mcp/tools/call/sse` | POST | Invoke tool with SSE streaming |
| `/api/v1/mcp/resources/list` | POST/GET | List available resources |
| `/api/v1/mcp/resources/read` | POST | Read a resource |
| `/api/v1/mcp/prompts/list` | POST/GET | List prompt templates |
| `/api/v1/mcp/prompts/get` | POST | Get a prompt template |

#### MCP Tool Invocation

```bash
curl -X POST https://localhost:8440/api/v1/mcp/tools/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "shell.execute",
      "arguments": {
        "command": "ls -la"
      }
    },
    "id": 1
  }'
```

#### MCP Special Tool: read_field

The Gateway provides a special `read_field` tool for governed field access with L1 validation and L3 session verification:

```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "read_field",
    "arguments": {
      "collection": "cases",
      "document_id": "case-123",
      "field_path": "metadata.status",
      "operator_session_id": "op-session-abc"
    }
  },
  "id": 1
}
```

---

### 2. A2A (Agent-to-Agent)

A2A is an HTTP/JSON protocol for agent skill invocation. The Gateway wraps A2A skill calls in governance envelopes with the same L1/L2/L3 verification pipeline.

#### A2A Endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v1/a2a/call` | POST | Invoke an A2A skill |

#### A2A Skill Invocation

```bash
curl -X POST https://localhost:8440/api/v1/a2a/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "a2a/call",
    "params": {
      "skill_name": "file.read",
      "payload": {
        "path": "/etc/hosts"
      }
    },
    "id": 1
  }'
```

---

### 3. Direct Governance Envelope

For maximum control, applications can submit canonical JSON `GovernanceEnvelope` transactions directly. This is the only customer-facing mutation API on the Gateway.

#### Envelope Submission

```bash
curl -X POST https://localhost:8440/api/v1/governance/envelopes \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

The envelope `id` must match the deterministic `transaction_hash` computed from all envelope fields. The Gateway rejects envelopes with mismatched IDs.

---

### 4. WebSocket Pub/Sub

The Gateway provides real-time pub/sub via WebSocket for streaming events and command dispatch.

#### WebSocket Connection

```bash
wscat -c wss://localhost:8440/api/v1/pubsub/stream \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key
```

#### Pub/Sub Channels

- **Mutation channels**: `cmd:*` (governed, require envelope submission)
- **Non-mutation channels**: `heartbeat:*`, `results:*`, `sse:*`, `ws_session:*`, `internal:*`

#### Subscribe Protocol

```json
{
  "type": "subscribe",
  "channel": "cmd:operator-id:operator-session-id"
}
```

---

### 5. Document Store API

The Gateway provides a JSON document store with CRUD operations and query support.

#### Document Operations

```bash
# Get document
curl https://localhost:8440/api/v1/data/cases/case-123 \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key

# Set document
curl -X PUT https://localhost:8440/api/v1/data/cases/case-123 \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"status": "open", "priority": "high"}'

# Update document (merge)
curl -X PATCH https://localhost:8440/api/v1/data/cases/case-123 \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"priority": "critical"}'

# Delete document
curl -X DELETE https://localhost:8440/api/v1/data/cases/case-123 \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key

# Query documents
curl -X POST https://localhost:8440/api/v1/data/cases/_query \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "filter": {"status": "open"},
    "limit": 10
  }'
```

The document store uses the `/api/v1/data/{collection}/{id}` pattern for CRUD operations and `/api/v1/data/{collection}/_query` for queries.

#### Governed Collections

Direct `/api/v1/data/` mutations are restricted to platform infrastructure collections. Governed collections (cases, investigations, tasks, memories, reputation_state, reputation_commitments, stake_resolutions, agent_activity_metadata, etc.) must use `POST /api/v1/governance/envelopes` for mutations.

---

## Authentication

### Session Types

The Gateway enforces strict session separation to prevent cross-tenant data leakage:

| Session Type | Identifier | Authentication | Use Case |
|---|---|---|---|
| **Web Session** | `web_session_id` | WebAuthn (passkey) | Browser-based clients |
| **CLI Session** | `cli_session_id` | mTLS certificate | CLI/BYO clients |
| **Operator Session** | `operator_session_id` | mTLS certificate | In-process execution context |

### CLI Authentication (mTLS)

Generate a client certificate for CLI operations:

```bash
./g8e auth login
```

This:
1. Generates a CSR (Certificate Signing Request)
2. Receives a signed client certificate with SPIFFE URI SAN
3. Stores it in `.g8e/pki/client.crt`

CLI sessions use the mTLS certificate fingerprint as L3 proof via CLIL3Notary.

### Browser Authentication (WebAuthn)

For web-based interactions:

1. Navigate to `https://localhost:8443` (public port)
2. Follow on-screen prompts to register a security key
3. Use the key for subsequent authentication

Web sessions use WebAuthn signatures as L3 proof via PasskeyService.

### CSR-Based Enrollment

All authentication to the Gateway requires owner-approved invitations. The platform enforces a strict owner-centric model where every entity (operator, MCP server, AI client, user) must authenticate via invitation-based JIT provisioning.

#### Owner-Centric Invitation Model

- **Platform Owner**: The first human to authenticate becomes the Platform Owner
- **Universal Requirement**: All entities must use invitations for JIT provisioning
- **Strict TTL**: Sessions have a 1-hour TTL by default
- **Owner Approval**: Only users with the `owner` role can create invitations

#### Creating Invitations

Generate an invitation for external IdP authentication:

```bash
./g8e data invitations create --sub "user@example.com" --roles "user" --ttl 3600
```

#### Invitation-Based JIT Authentication Flow

1. **Owner creates invitation**: Owner generates an invitation with specific TTL and roles
2. **Client requests enrollment**: Client presents JWT with `sub` claim during registration
3. **Gateway validates**: Gateway checks JWT signature and verifies invitation exists for the `sub`
4. **User provisioning**: If invitation exists, user is provisioned and bound to owner's organization
5. **Invitation consumption**: Invitation is consumed after successful provisioning
6. **Session issuance**: Gateway issues short-lived mTLS certificate (1-day TTL) and session (1-hour TTL)
7. **Session renewal**: Client must re-authenticate before expiry

---

## PKI and Trust

### Device Enrollment (CSR-based)

Enroll a device using CSR-based enrollment with mTLS authentication. The user_id is extracted from the client certificate's SPIFFE URI SAN:

```bash
curl -X POST https://localhost:8440/api/v1/pki/devices/enroll \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "csr_pem": "-----BEGIN CERTIFICATE REQUEST-----...",
    "cli_csr_pem": "-----BEGIN CERTIFICATE REQUEST-----...",
    "system_fingerprint": "fp-123",
    "hostname": "my-host",
    "os": "linux",
    "arch": "amd64",
    "username": "user",
    "ip_address": "192.168.1.1"
  }'
```

### CSR Signing (Low-level)

Submit a CSR for low-level certificate issuance (for advanced use cases):

```bash
curl -X POST https://localhost:8440/api/v1/pki/csr/sign \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{
    "csr_pem": "-----BEGIN CERTIFICATE REQUEST-----...",
    "leaf_type": "operator",
    "operator_id": "op-123"
  }'
```

---

## Out-of-Band (OOB) Approval Flow

When a standard AI client requests a mutation without L3 proof, the Gateway suspends the transaction and returns an OOB approval URL.

### Suspension Flow

1. Client submits MCP/A2A request without L3 signature
2. Gateway stores transaction in `suspended_transactions` table
3. Gateway returns approval URL: `https://localhost:8443/approve/{tx_hash}`
4. User opens URL in browser and authenticates with passkey
5. User approves transaction via WebAuthn
6. Gateway attaches L3 proof and resumes verification
7. Transaction proceeds to execution

### Approval API

List suspended transactions:

```bash
curl https://localhost:8443/api/v1/approvals \
  --cookie "web_session=..."
```

Approve a transaction:

```bash
curl -X POST https://localhost:8443/api/v1/approvals/{tx_hash} \
  --cookie "web_session=..." \
  -H "Content-Type: application/json" \
  -d '{"action": "approve"}'
```

---

## Audit and Receipts

### Query Audit Receipts

```bash
curl https://localhost:8440/api/v1/audit/receipts?operator_session_id=op-session-abc \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key
```

### Export Audit Receipts

```bash
curl https://localhost:8440/api/v1/audit/receipts/export?operator_session_id=op-session-abc \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -o audit-export.json
```

### CLI Audit Query

```bash
./g8e data audit list --operator-session-id <session-id> --limit 100
```

---

## Custom Gateway Implementation

For custom g8e-compatible gateway implementations, connection follows the same operational pattern:

1. **Initialize PKI**: Generate root CA and intermediate CAs with SPIFFE URI SAN support
2. **Configure Persistence**: Set up document store, KV store, and blob store with state root computation
3. **Configure Ports**: Bind the four logical surfaces to appropriate ports with correct TLS settings
4. **Start Gateway**: Launch in desired mode (doctrine, consensus, or notary)
5. **Enroll Clients**: Use device-link tokens and CSR-based enrollment for operators and CLI clients
6. **Monitor Health**: Implement health checks for gateway process and connected operators

### Configuration Requirements

Custom gateways must support:
- CLI flags for runtime parameters (ports, mode, paths)
- Configuration files for complex deployments
- Multiplexed port handling with optional mTLS

### High Availability Considerations

For production deployments:
- Gateway clustering with shared persistence
- Load balancing across multiple gateway instances
- Certificate rotation and revocation automation
- Automated backup of audit vault and state store
- Circuit breaker configuration for downstream services

---

## Troubleshooting

### Gateway Fails to Start

Check if ports are already in use:

```bash
./g8e gw status
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
./g8e auth login
```

### Operator Connection Issues

Check Gateway is listening on the mTLS port:

```bash
curl -k https://localhost:8440/api/v1/health
```

### Circuit Breaker Active

If downstream MCP/A2A server is unavailable, the Gateway circuit breaker activates after 5 consecutive failures. Check Gateway logs for circuit breaker status.

---

## Next Steps

- **[Build Operator](build_operator.md)** - Build a custom g8e-compatible g8e Operator
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)** - Deploy and use a g8e Operator
- **[Build Apps](build_apps.md)** - Build g8e-compatible applications using a Gateway
- **[MCP Protocol](../protocols/mcp/mcp.md)** - Detailed MCP protocol specification
- **[A2A Protocol](../protocols/a2a/a2a.md)** - Detailed A2A protocol specification
- **[Gateway Architecture](../architecture/gateway.md)** - Gateway architecture and internals
