---
title: Build Apps
parent: Guides
---

# Build g8e-Compatible Applications

Last Updated: 2026-06-22
Version: v1.1.6

---

## Overview

A g8e-compatible application functions strictly as a GovernanceEnvelope producer and receipt consumer. It maintains no privileged communication channels, never interacts directly with the host system, and communicates with the g8e Gateway exclusively through public ingress paths.

Security operations including Doctrine (L1Doctrine), Consensus (L2Consensus), and Notary (L3Notary) verification gates, replay defense, state binding, cryptographic audit, and human-in-the-loop authorization are fully delegated to the g8e Gateway. The application provides only the components the protocol cannot intrinsically supply: the mutation intent and optionally, consensus evidence.

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

Application identity is established via an mTLS/SPIFFE certificate. The application authenticates cryptographically and receives no ambient trust. The g8e Gateway evaluates its envelope with identical rigor to any external client.

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
- **timestamp**: Creation timestamp.
- **expires_at**: Timestamp for expiry enforcement.
- **source_component**: Component identifier (e.g., COMPONENT_CLIENT).
- **operator_id**: Target operator identifier.
- **operator_session_id**: Operator session identifier.
- **web_session_id**: Web session identifier.
- **cli_session_id**: CLI session identifier.
- **event_type**: Typed event identifier (e.g., `g8e.v1.operator.command.requested`).
- **payload**: Raw protobuf payload bytes.
- **intent_data**: Structured JSON view of the intent.
- **action_type**: UAP-compatible action type (e.g., `EXECUTE_BASH`).
- **target_resource**: UAP-compatible target resource.
- **state_merkle_root**: Current state root from the Gateway.
- **nonce**: Unique value for replay defense.
- **transaction_hash**: Deterministic hash computed from envelope fields.
- **protocol_version**: UAP-compatible protocol version (e.g., "1.0").
- **governance**: Governance metadata containing L1, L2, and L3 proofs.
- **case_id**: Optional case identifier.
- **investigation_id**: Optional investigation identifier.
- **task_id**: Optional task identifier.
- **system_fingerprint**: Optional system fingerprint.
- **tenant_id**: Optional tenant identifier.
- **binding_persona**: Optional binding persona.
- **requestor_user_id**: The human user who authorized the action (delegator).
- **acting_app_id**: The app/tool acting on behalf of the user (delegate).

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
./g8e auth enroll
```

This stores the certificate in `.g8e/cli.crt` and key in `.g8e/cli.key`.

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
- requestor_user_id
- acting_app_id

Refer to the `GenerateMessageID` function in `pkg/governance/types.go` for the exact canonicalization algorithm.

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
    "l3": {},
    "gateway_signed": false
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
      "consensus_signature": "<signature>",
      "agent_ids": ["<agent_1_id>", "<agent_2_id>"],
      "key_id": "<key_id>"
    },
    "l3": {},
    "gateway_signed": false
  }
}
```

### Step 3: Submit to Gateway

Submit the envelope with L2 signatures to the g8e Gateway. The g8e Gateway will verify the signatures as part of the L2Consensus check.

---

## Protocol Translation Integration

Applications can leverage the g8e Gateway's MCP/A2A translation layer instead of constructing envelopes directly.

### MCP Integration

For MCP-based applications, the Gateway accepts JSON-RPC tool calls and translates them into GovernanceEnvelope format:

```bash
curl -X POST https://localhost:8443/api/v1/mcp/tools/call \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"ls -la"}}}'
```

The Gateway performs L1 Doctrine validation on the tool name before envelope construction.

### A2A Integration

For A2A-based applications, the Gateway accepts HTTP/JSON task invocations and translates them into GovernanceEnvelope format:

```bash
curl -X POST https://localhost:8443/api/v1/a2a/call \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key \
  -H "Content-Type: application/json" \
  -d '{"skill_name":"read_file","payload":"{\"path\":\"/etc/hosts\"}","execution_id":"task-1"}'
```

The Gateway performs L1 Doctrine validation on the skill name before envelope construction.

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

Refer to `protocol/examples/governance_envelope/` for example envelope construction code and `pkg/governance/types.go` for the canonical hash implementation.

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)** — Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)** — Deploy and use a g8e Operator.
