---
title: Build Apps
parent: Guides
---

# Build g8e-Compatible Applications

Last Updated: 2026-07-06
Version: v1.3.7

---

## Overview

A g8e-compatible application functions strictly as a GovernanceEnvelope producer and receipt consumer. It maintains no privileged communication channels, never interacts directly with the host system, and communicates with the g8e Gateway exclusively through public ingress paths.

Security operations including L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, and L5 Actuator verification gates, replay defense, state binding, cryptographic audit, and human-in-the-loop authorization are fully delegated to the g8e Gateway. The application provides only the components the protocol cannot intrinsically supply: the mutation intent and optionally, consensus evidence.

---

## Application Architecture Spectrum

The architecture of a g8e application varies based on how it satisfies the L2 Consensus requirement. All applications produce the canonical GovernanceEnvelope wire format.

### Minimal Applications

A minimal application constructs the mutation intent and builds a valid GovernanceEnvelope. This requires:

- **Typed Payload Formatting**: Format the mutation intent according to the protocol schema.
- **Transaction Hash Generation**: Generate a deterministic transaction hash from the envelope fields.
- **Envelope Construction**: Append nonce, expiry, and fetched state root to the envelope.
- **Submission**: Submit the envelope to the Gateway and consume the signed receipt.

Minimal applications do not produce L2 Consensus evidence natively. They rely on the Gateway's protocol-agnostic MCP/A2A translation layer or a trusted upstream producer to fulfill the L2 requirement.

### Maximal Applications

A maximal application performs the identical intent formulation and envelope construction, while additionally producing its own L2 Consensus evidence. It executes an internal consensus mechanism and signs the envelope directly.

A g8e-compatible agentic ensemble represents the reference implementation of a maximal application, generating the required consensus signatures.

---

## Structural Invariants

Two invariants apply to all g8e applications:

### Identity and Authentication

Application identity is established via an mTLS client certificate with SPIFFE-style URI SANs. The application authenticates cryptographically and receives no ambient trust. The g8e Gateway evaluates its envelope with identical rigor to any external client.

### State Management

Application-internal state remains the exclusive responsibility of the application. The g8e protocol governs and audits mutations to host reality; it does not manage or persist the application working memory. The Gateway maintains the canonical state root for replay defense and state binding.

---

## Protocol Requirements

### Canonical JSON Wire Format

All client-facing interactions must use canonical JSON (protojson) as the wire format. Binary protobuf is reserved for internal storage.

The envelope `id` must match the deterministic transaction_hash computed from its content. The signature basis is always the deterministic transaction hash, regardless of wire encoding.

### GovernanceEnvelope Structure

A valid GovernanceEnvelope includes the following categories of fields:

- **Identity and routing**: Transaction hash ID, timestamp, expiry, source component, operator and session identifiers, and event type.
- **Payload**: The typed protobuf payload bytes and a structured JSON view of the intent.
- **Action classification**: UAP-compatible action type (e.g., `EXECUTE_BASH`) and target resource.
- **State binding**: Current state Merkle root from the Gateway and a unique nonce for replay defense.
- **Governance metadata**: L1, L2, and L3 proofs attached by the verification pipeline.
- **Delegation**: The requestor user ID (human delegator) and acting app ID (delegate tool).
- **Optional context**: Case, investigation, and task identifiers, system fingerprint, tenant ID, and binding persona.

### Typed Payloads

The protocol defines canonical event types for all first-class operations. The following list covers the primary mutation event types used by applications:

- **Shell Operations**: `g8e.v1.operator.command.requested`, `g8e.v1.operator.command.cancel.requested`
- **File Operations**: `g8e.v1.operator.filesystem.read.requested`, `g8e.v1.operator.file.edit.requested`, `g8e.v1.operator.file.history.fetch.requested`, `g8e.v1.operator.file.diff.fetch.requested`, `g8e.v1.operator.file.restore.requested`
- **Filesystem Operations**: `g8e.v1.operator.filesystem.list.requested`, `g8e.v1.operator.filesystem.grep.requested`
- **MCP Operations**: `g8e.v1.operator.mcp.call.requested`
- **A2A Operations**: `g8e.v1.operator.a2a.call.requested`
- **Network Operations**: `g8e.v1.operator.network.ping.requested`, `g8e.v1.operator.network.port.check.requested`
- **Heartbeat**: `g8e.v1.operator.heartbeat.requested`
- **Audit Operations**: `g8e.v1.operator.audit.command.recorded`, `g8e.v1.operator.audit.ai.recorded`
- **Shutdown**: `g8e.v1.operator.shutdown.requested`

This list is not exhaustive. The complete set of event type constants and payload schema definitions are available in the protocol constants and protobuf schemas.

---

## Building a Minimal Application

### Step 1: Obtain Client Certificate

Generate an mTLS client certificate from the Gateway via CSR-based enrollment:

```bash
./g8e auth enroll
```

This generates a local keypair, submits a CSR to the Gateway CA, and stores the signed certificate in `.g8e/cli.crt` with the private key in `.g8e/cli.key`. On non-Windows platforms, enrollment also opens a browser to register a WebAuthn passkey. The Gateway must be running before enrollment (`./g8e gw start`).

### Step 2: Fetch State Root

Query the Gateway health endpoint for the current state root:

```bash
curl -X GET https://localhost:8443/api/v1/state \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key
```

The response includes the `state_merkle_root` field. Alternatively, the `/api/v1/health` endpoint also returns state root information.

### Step 3: Construct Typed Payload

Format the mutation intent according to the protocol schema. For example, a shell execute request uses the `CommandRequested` protobuf message:

```json
{
  "command": "ls -la",
  "execution_id": "unique-execution-id",
  "justification": "List directory contents",
  "vault_mode": "scrub",
  "timeout_seconds": 30,
  "intent": "Inspect filesystem",
  "environment": {},
  "working_directory": "/tmp"
}
```

The payload must be serialized as protobuf bytes and base64-encoded in the final envelope.

### Step 4: Generate Transaction Hash

Compute the deterministic transaction hash from the envelope's critical fields. The hash is a SHA-256 digest over the following fields, canonicalized in protocol field order:

- action_type
- target_resource
- payload (base64-encoded)
- state_merkle_root
- nonce
- expires_at (UTC RFC3339Nano format)
- intent_data (canonicalized map)
- requestor_user_id
- acting_app_id

The `id` field must be set to this computed hash. L3 proof is intentionally excluded from the hash so that L2 consensus can sign before the human notary is asked.

### Step 5: Build Envelope

Construct the GovernanceEnvelope:

```json
{
  "id": "<transaction_hash>",
  "timestamp": "<current_timestamp>",
  "expires_at": "<expiry_timestamp>",
  "source_component": "COMPONENT_CLIENT",
  "operator_id": "<operator_id>",
  "operator_session_id": "<operator_session_id>",
  "web_session_id": "<web_session_id>",
  "cli_session_id": "<cli_session_id>",
  "event_type": "g8e.v1.operator.command.requested",
  "payload": "<base64_encoded_protobuf_bytes>",
  "intent_data": {
    "command": "ls -la",
    "working_directory": "/tmp",
    "environment": {}
  },
  "action_type": "EXECUTE_BASH",
  "target_resource": "/tmp",
  "state_merkle_root": "<state_root>",
  "nonce": "<unique_nonce>",
  "transaction_hash": "<transaction_hash>",
  "protocol_version": "1.0",
  "requestor_user_id": "<requestor_user_id>",
  "acting_app_id": "<acting_app_id>",
  "governance": {
    "l1": {
      "validated": true,
      "violations": []
    },
    "l2": {},
    "l3": {}
  }
}
```

### Step 6: Submit to Gateway

Submit the envelope to the g8e Gateway:

```bash
curl -X POST https://localhost:8443/api/v1/governance/envelopes \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

The Gateway validates the envelope through the five-layer governance pipeline (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator) before execution.

### Step 7: Consume Receipt

The Gateway returns a signed ActionReceipt. Verify the receipt and consume the result.

---

## Building a Maximal Application

A maximal application adds L2 Consensus signature generation to the minimal application flow.

### Step 1: Execute Internal Consensus

Run an internal consensus mechanism to generate L2 signatures. This typically involves:

- Multiple independent agents analyzing the mutation intent.
- Each agent generating a signature based on their analysis.
- Aggregating signatures into a consensus proof.

### Step 2: Attach Signatures to Envelope

Add the L2 Consensus signatures to the envelope:

```json
{
  "id": "<transaction_hash>",
  "timestamp": "<current_timestamp>",
  "expires_at": "<expiry_timestamp>",
  "source_component": "COMPONENT_CLIENT",
  "event_type": "g8e.v1.operator.command.requested",
  "payload": "<base64_encoded_protobuf_bytes>",
  "intent_data": {
    "command": "ls -la",
    "working_directory": "/tmp",
    "environment": {}
  },
  "action_type": "EXECUTE_BASH",
  "target_resource": "/tmp",
  "state_merkle_root": "<state_root>",
  "nonce": "<unique_nonce>",
  "transaction_hash": "<transaction_hash>",
  "protocol_version": "1.0",
  "requestor_user_id": "<requestor_user_id>",
  "acting_app_id": "<acting_app_id>",
  "governance": {
    "l1": {
      "validated": true,
      "violations": []
    },
    "l2": {
      "tribunal_id": "<tribunal_id>",
      "votes": [
        {
          "signer_key_id": "<key_id>",
          "consensus_signature": "<ed25519_signature>",
          "decision": true
        }
      ]
    },
    "l3": {}
  }
}
```

### Step 3: Submit to Gateway

Submit the envelope with L2 signatures to the g8e Gateway. The Gateway verifies the signatures as part of the L2 Consensus check.

---

## Protocol Translation Integration

Applications can leverage the g8e Gateway's MCP/A2A translation layer instead of constructing envelopes directly.

### MCP Integration

For MCP-based applications, the Gateway accepts JSON-RPC tool calls and translates them into GovernanceEnvelope format:

```bash
curl -X POST https://localhost:8443/mcp \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"ls -la"}}}'
```

The Gateway translates the tool call into a GovernanceEnvelope and processes it through the full governance pipeline, including L1 Doctrine validation.

### A2A Integration

For A2A-based applications, the Gateway accepts HTTP/JSON task invocations and translates them into GovernanceEnvelope format:

```bash
curl -X POST https://localhost:8443/api/v1/a2a/call \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"skill_name":"read_file","payload":{"path":"/etc/hosts"},"execution_id":"task-1"}}'
```

The Gateway translates the skill invocation into a GovernanceEnvelope and processes it through the full governance pipeline, including L1 Doctrine validation.

---

## Testing

Applications should test against the reference g8e Gateway to ensure compatibility:

```bash
./g8e gw start
./g8e test e2e
```

Verify that:
- Envelopes are accepted by the Gateway
- Receipts are returned with valid Ed25519 signatures
- Mutations are executed on connected Operators
- Audit entries are written to the audit vault
- The transaction hash matches the envelope ID

---

## Security Considerations

### No Privileged Channels

Applications must not attempt to establish privileged communication channels with the g8e Gateway or g8e Operators. All communication must go through public ingress paths.

### Fail-Closed Behavior

Applications must handle verification failures gracefully. If the g8e Gateway rejects an envelope, the application must not retry with modified parameters or attempt fallback paths.

### Certificate Management

Applications must manage their mTLS certificates securely. Certificates should be stored securely and rotated before expiry.

### State Root Validation

Applications must validate the state root returned by the g8e Gateway (via health endpoint) before using it in envelope construction. This prevents man-in-the-middle attacks.

---

## Reference Implementation

A reference g8e-compatible agentic ensemble demonstrates a maximal application implementation. It includes:

- Internal consensus mechanism for L2 signature generation
- Envelope construction and submission using the canonical hash algorithm
- Receipt verification and consumption
- MCP/A2A integration

Refer to the protocol examples for sample envelope construction code.

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)**: Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)**: Deploy and use a g8e Operator.
