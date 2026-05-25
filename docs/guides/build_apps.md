---
title: Build Apps
parent: Guides
---

# Build g8e-Compatible Applications

Last Updated: 2026-05-25
Version: v0.2.6

---

## Overview

A g8e-compatible application functions strictly as a GovernanceEnvelope producer and receipt consumer. It maintains no privileged communication channels, never interacts directly with the host system, and communicates with the Gateway exclusively through public ingress paths.

Security operations including Doctrine (L1Doctrine), Consensus (L2Consensus), and Notary (L3Notary) verification gates, replay defense, state binding, cryptographic audit, and human-in-the-loop authorization are fully delegated to the Gateway substrate. The application provides only the components the protocol cannot intrinsically supply: the mutation intent and optionally, consensus evidence.

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

Application identity is established via an mTLS/SPIFFE certificate. The application authenticates cryptographically and receives no ambient trust. The Gateway evaluates its envelope with identical rigor to any external client.

### State Management

Application-internal state remains the exclusive responsibility of the application. The g8e protocol governs and audits mutations to host reality; it does not manage or persist the application working memory.

---

## Protocol Requirements

### Canonical JSON Wire Format

All client-facing interactions must use canonical JSON (protojson) as the wire format. Binary protobuf is reserved for internal storage.

The envelope `id` must match the deterministic transaction_hash computed from its content. The signature basis is always the deterministic transaction hash, regardless of wire encoding.

### GovernanceEnvelope Structure

A valid GovernanceEnvelope must include:

- **id**: Deterministic transaction hash (SHA256 of canonical fields).
- **payload_type**: Typed payload identifier (e.g., `ShellExecuteRequested`).
- **payload**: Typed payload content according to the protocol schema.
- **nonce**: Unique value for replay defense.
- **expires_at**: Timestamp for expiry enforcement.
- **state_merkle_root**: Current state root from the Gateway.
- **signatures**: L2Consensus signatures (for maximal applications).
- **l3_notary_proof**: L3 authorization proof (mTLS certificate fingerprint or WebAuthn proof).

### Typed Payloads

The protocol defines canonical request payload mappings for all first-class event types. Applications must use these typed payloads:

- **Shell Operations**: `ShellExecuteRequested`, `ShellOutputReceived`
- **File Operations**: `FileReadRequested`, `FileWriteRequested`, `FileHistoryRequested`, `FileDiffRequested`, `FileRestoreRequested`
- **Audit Operations**: `AuditRequestEvent`
- **Shutdown**: `ShutdownRequested`

Refer to `protocol/proto/g8e/` for the canonical schema definitions.

---

## Building a Minimal Application

### Step 1: Obtain Client Certificate

Generate an mTLS client certificate from the Gateway:

```bash
./g8e login
```

This stores the certificate in `.g8e/pki/client.crt` and key in `.g8e/pki/client.key`.

### Step 2: Fetch State Root

Query the Gateway for the current state root:

```bash
curl -X GET https://localhost:8440/api/state/root \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key
```

### Step 3: Construct Typed Payload

Format the mutation intent according to the protocol schema. For example, a shell execute request:

```json
{
  "command": "ls -la",
  "working_directory": "/tmp",
  "environment": {}
}
```

### Step 4: Generate Transaction Hash

Compute the deterministic transaction hash from the envelope fields:

```python
import hashlib
import json

def compute_transaction_hash(payload_type, payload, nonce, expires_at, state_merkle_root):
    fields = {
        "payload_type": payload_type,
        "payload": payload,
        "nonce": nonce,
        "expires_at": expires_at,
        "state_merkle_root": state_merkle_root
    }
    canonical = json.dumps(fields, sort_keys=True, separators=(',', ':'))
    return hashlib.sha256(canonical.encode()).hexdigest()
```

### Step 5: Build Envelope

Construct the GovernanceEnvelope:

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
  "state_merkle_root": "<state_root>"
}
```

### Step 6: Submit to Gateway

Submit the envelope to the Gateway:

```bash
curl -X POST https://localhost:8440/api/governance/envelope \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

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

Submit the envelope with L2 signatures to the Gateway. The Gateway will verify the signatures as part of the L2Consensus check.

---

## Protocol Translation Integration

Applications can leverage the Gateway's MCP/A2A translation layer instead of constructing envelopes directly.

### MCP Integration

For MCP-based applications, the Gateway automatically translates JSON-RPC tool calls into GovernanceEnvelope format:

```bash
curl -X POST https://localhost:8440/api/mcp/v1/tools/call \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"tool": "shell.execute", "arguments": {"command": "ls -la"}}'
```

### A2A Integration

For A2A-based applications, the Gateway automatically translates HTTP/JSON skill invocations into GovernanceEnvelope format:

```bash
curl -X POST https://localhost:8440/api/a2a/v1/skills/execute \
  --cert .g8e/pki/client.crt \
  --key .g8e/pki/client.key \
  -H "Content-Type: application/json" \
  -d '{"skill": "file.read", "parameters": {"path": "/etc/hosts"}}'
```

---

## Testing

Applications should test against the reference Gateway to ensure compatibility:

```bash
./g8e platform start
./g8e test g8eo
```

Verify that:
- Envelopes are accepted by the Gateway
- Receipts are returned with valid signatures
- Mutations are executed on connected Operators
- Audit entries are written to the audit vault

---

## Security Considerations

### No Privileged Channels

Applications must not attempt to establish privileged communication channels with the Gateway or Operators. All communication must go through public ingress paths.

### Fail-Closed Behavior

Applications must handle verification failures gracefully. If the Gateway rejects an envelope, the application must not retry with modified parameters or attempt fallback paths.

### Certificate Management

Applications must manage their mTLS certificates securely. Certificates should be stored securely and rotated before expiry.

### State Root Validation

Applications must validate the state root returned by the Gateway before using it in envelope construction. This prevents man-in-the-middle attacks.

---

## Reference Implementation

The reference g8e-compatible agentic ensemble (g8ee) demonstrates a maximal application implementation. It includes:

- Internal consensus mechanism for L2 signature generation
- Envelope construction and submission
- Receipt verification and consumption
- MCP/A2A integration

Refer to the g8ee source code for a complete example of a g8e-compatible application.

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)** — Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)** — Deploy and use a Governed Operator.
