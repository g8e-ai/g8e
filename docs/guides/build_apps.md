---
title: Build Apps
parent: Guides
---

# Build g8e-Compatible Applications

Last Updated: 2026-06-01
Version: v1.0.5

---

## Overview

A g8e-compatible application functions strictly as a GovernanceEnvelope producer and receipt consumer. It maintains no privileged communication channels, never interacts directly with the host system, and communicates with the Governance Gateway exclusively through public ingress paths.

Security operations including Doctrine (L1Doctrine), Consensus (L2Consensus), and Notary (L3Notary) verification gates, replay defense, state binding, cryptographic audit, and human-in-the-loop authorization are fully delegated to the Governance Gateway. The application provides only the components the protocol cannot intrinsically supply: the mutation intent and optionally, consensus evidence.

---

## Application Architecture Spectrum

The architecture of a g8e application varies based on how it satisfies the Consensus (L2Consensus) requirement. All applications produce the canonical GovernanceEnvelope wire format.

### Minimal Applications

A minimal application constructs the mutation intent and builds a valid GovernanceEnvelope. This requires:

- **Typed Payload Formatting**: Format the mutation intent according to the protocol schema.
- **Transaction Hash Generation**: Generate a deterministic transaction hash from the envelope fields.
- **Envelope Construction**: Append nonce, expiry, and fetched state root to the envelope.
- **Submission**: Submit the envelope to the Gateway and consume the signed receipt.

Minimal applications do not produce L2 consensus evidence natively. They rely on the Gateway's protocol-agnostic MCP/A2A translation layer or a trusted upstream producer to fulfill the L2 requirement.

### Maximal Applications

A maximal application performs the identical intent formulation and envelope construction, while additionally producing its own Consensus (L2Consensus) consensus evidence. It executes an internal consensus mechanism and signs the envelope directly.

A g8e-compatible agentic ensemble represents the reference implementation of a maximal application, generating the required consensus signatures.

---

## Structural Invariants

Two invariants apply to all g8e applications:

### Identity and Authentication

Application identity is established via an mTLS/SPIFFE certificate. The application authenticates cryptographically and receives no ambient trust. The Governance Gateway evaluates its envelope with identical rigor to any external client.

### State Management

Application-internal state remains the exclusive responsibility of the application. The g8e protocol governs and audits mutations to host reality; it does not manage or persist the application working memory. The Gateway maintains the canonical state root for replay defense and state binding.

---

## Protocol Requirements

### Canonical JSON Wire Format

All client-facing interactions must use canonical JSON (protojson) as the wire format. Binary protobuf is reserved for internal storage.

The envelope `id` must match the deterministic transaction_hash computed from its content. The signature basis is always the deterministic transaction hash, regardless of wire encoding.

### GovernanceEnvelope Structure

A valid GovernanceEnvelope must include:

- **id**: Deterministic transaction hash (SHA256 of canonical fields).
- **event_type**: Typed event identifier (e.g., `g8e.v1.operator.command.requested`).
- **payload**: Raw protobuf payload bytes.
- **intent_data**: Structured JSON view of the intent.
- **action_type**: UAP-compatible action type (e.g., `EXECUTE_BASH`).
- **target_resource**: UAP-compatible target resource.
- **nonce**: Unique value for replay defense.
- **expires_at**: Timestamp for expiry enforcement.
- **state_merkle_root**: Current state root from the Gateway.
- **transaction_hash**: Deterministic hash computed from envelope fields.
- **governance**: Governance metadata containing L1, L2, and L3 proofs.

### Typed Payloads

The protocol defines canonical event types for all first-class operations. Applications must use these event types:

- **Shell Operations**: `g8e.v1.operator.command.requested`
- **File Operations**: `g8e.v1.operator.filesystem.read.requested`, `g8e.v1.operator.file.edit.requested`, `g8e.v1.operator.file.history.fetch.requested`, `g8e.v1.operator.file.diff.fetch.requested`, `g8e.v1.operator.file.restore.requested`
- **Filesystem Operations**: `g8e.v1.operator.filesystem.list.requested`, `g8e.v1.operator.filesystem.grep.requested`
- **Audit Operations**: `g8e.v1.operator.audit.command.recorded`, `g8e.v1.operator.audit.ai.recorded`
- **Shutdown**: `g8e.v1.operator.shutdown.requested`

Refer to `protocol/proto/g8e/operator/v1/operator.proto` for the canonical schema definitions and `protocol/constants/events.json` for event type constants.

---

## Building a Minimal Application

### Step 1: Obtain Client Certificate

Generate an mTLS client certificate from the Gateway (Operator in gateway mode):

```bash
./g8e auth login
```

This stores the certificate in `.g8e/pki/client.crt` and key in `.g8e/pki/client.key`.

### Step 2: Fetch State Root

Query the Gateway health endpoint for the current state root:

```bash
curl -X GET https://localhost:8443/api/v1/health \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key
```

The response includes the `state_merkle_root` field.

### Step 3: Construct Typed Payload

Format the mutation intent according to the protocol schema. For example, a shell execute request uses the `CommandRequested` protobuf message:

```json
{
  "command": "ls -la",
  "working_directory": "/tmp",
  "environment": {},
  "execution_id": "unique-execution-id",
  "justification": "List directory contents"
}
```

The payload must be serialized as protobuf bytes and base64-encoded in the final envelope.

### Step 4: Generate Transaction Hash

Compute the deterministic transaction hash from the envelope fields using the canonicalization rules defined in `pkg/governance/types.go`. The hash is computed over:

- action_type
- target_resource
- payload (base64-encoded)
- state_merkle_root
- nonce
- expires_at (UTC RFC3339Nano format)
- intent_data (canonicalized map)

Refer to the `GenerateMessageID` function in `pkg/governance/types.go` for the exact canonicalization algorithm.

### Step 5: Build Envelope

Construct the GovernanceEnvelope:

```json
{
  "id": "<transaction_hash>",
  "event_type": "g8e.v1.operator.command.requested",
  "payload": "<base64_encoded_protobuf_bytes>",
  "intent_data": {
    "command": "ls -la",
    "working_directory": "/tmp",
    "environment": {}
  },
  "action_type": "EXECUTE_BASH",
  "target_resource": "/tmp",
  "nonce": "<unique_nonce>",
  "expires_at": "<expiry_timestamp>",
  "state_merkle_root": "<state_root>",
  "transaction_hash": "<transaction_hash>",
  "timestamp": "<current_timestamp>",
  "source_component": "COMPONENT_CLIENT"
}
```

### Step 6: Submit to Gateway

Submit the envelope to the Governance Gateway:

```bash
curl -X POST https://localhost:8443/api/v1/governance/envelopes \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

The Gateway validates the envelope through the five-layer governance pipeline (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator) before execution.

### Step 7: Consume Receipt

The Gateway returns a signed ActionReceipt. Verify the receipt and consume the result.

---

## Building a Maximal Application

A maximal application adds L2Consensus signature generation to the minimal application flow.

### Step 1: Execute Internal Consensus

Run an internal consensus mechanism to generate L2 signatures. This typically involves:

- Multiple independent agents analyzing the mutation intent.
- Each agent generating a signature based on their analysis.
- Aggregating signatures into a consensus proof.

### Step 2: Attach Signatures to Envelope

Add the L2Consensus signatures to the envelope:

```json
{
  "id": "<transaction_hash>",
  "payload_type": "ShellExecuteRequested",
  "payload": {
    "command": "ls -la",
    "working_directory": "/tmp",
    "environment": {}
  },
  "nonce": "<unique_nonce>",
  "expires_at": "<expiry_timestamp>",
  "state_merkle_root": "<state_root>",
  "signatures": [
    {
      "signer_id": "<agent_1_id>",
      "signature": "<signature_1>"
    },
    {
      "signer_id": "<agent_2_id>",
      "signature": "<signature_2>"
    }
  ]
}
```

### Step 3: Submit to Gateway

Submit the envelope with L2 signatures to the Governance Gateway. The Gateway will verify the signatures as part of the L2Consensus check.

---

## Protocol Translation Integration

Applications can leverage the Governance Gateway's MCP/A2A translation layer instead of constructing envelopes directly.

### MCP Integration

For MCP-based applications, the Gateway accepts JSON-RPC tool calls and translates them into GovernanceEnvelope format:

```bash
curl -X POST https://localhost:8443/api/v1/mcp/tools/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shell_execute","arguments":{"command":"ls -la"}}}'
```

The Gateway performs L1 Doctrine validation on the tool name before envelope construction.

### A2A Integration

For A2A-based applications, the Gateway accepts HTTP/JSON task invocations and translates them into GovernanceEnvelope format:

```bash
curl -X POST https://localhost:8443/api/v1/a2a/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"skill_name":"file_read","payload_json":"{\"path\":\"/etc/hosts\"}","execution_id":"task-1"}'
```

The Gateway performs L1 Doctrine validation on the skill name before envelope construction.

---

## Testing

Applications should test against the reference Governance Gateway to ensure compatibility:

```bash
./g8e gw start
./g8e test g8eo
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

Applications must not attempt to establish privileged communication channels with the Governance Gateway or g8e Operators. All communication must go through public ingress paths.

### Fail-Closed Behavior

Applications must handle verification failures gracefully. If the Governance Gateway rejects an envelope, the application must not retry with modified parameters or attempt fallback paths.

### Certificate Management

Applications must manage their mTLS certificates securely. Certificates should be stored securely and rotated before expiry.

### State Root Validation

Applications must validate the state root returned by the Governance Gateway (via health endpoint) before using it in envelope construction. This prevents man-in-the-middle attacks.

---

## Reference Implementation

A reference g8e-compatible agentic ensemble demonstrates a maximal application implementation. It includes:

- Internal consensus mechanism for L2 signature generation
- Envelope construction and submission using the canonical hash algorithm
- Receipt verification and consumption
- MCP/A2A integration

Refer to `protocol/examples/governance_envelope/` for example envelope construction code and `pkg/governance/types.go` for the canonical hash implementation.

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)** — Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)** — Deploy and use a Governed Operator.
