---
title: Build Apps
parent: Guides
---

# Build g8e-Compatible Applications

Last Updated: 2026-09-02
Version: v2.1.3

---

## Overview

A g8e-compatible application functions strictly as a GovernanceEnvelope producer and receipt consumer. It maintains no privileged communication channels, never interacts directly with the host system, and communicates with the g8e Gateway exclusively through public ingress paths.

Security operations including L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, and L5 Actuator verification gates, replay defense, state binding, cryptographic audit, and human-in-the-loop authorization are fully delegated to the g8e Gateway. The application provides only the components the protocol cannot intrinsically supply: the mutation intent and optionally, consensus evidence.

This guide covers the full spectrum of g8e application development: from minimal envelope-producing clients to maximal agentic ensembles that implement multi-persona reasoning, Byzantine consensus, and signed governance envelope production. The [Building an Agentic System](#building-an-agentic-system) section documents the practical steps for building a g8e-compliant agentic ensemble, the reference pattern for anyone building an AI reasoning layer on top of the g8e protocol surface.

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

The envelope `id` must match the deterministic transaction hash computed from its content. The signature basis is always the deterministic transaction hash, regardless of wire encoding.

### GovernanceEnvelope Structure

A valid GovernanceEnvelope includes identity and routing fields, a typed protobuf payload with a structured JSON view of the intent, UAP-compatible action classification, state binding (Merkle root and nonce), governance metadata (L1, L2, and L3 proofs), delegation fields, and optional context (case, investigation, task identifiers, tenant ID, binding persona).

### Typed Payloads

The protocol defines canonical event types for all first-class operations, including shell commands, file edits, filesystem reads, MCP tool calls, A2A skill invocations, network checks, heartbeats, audit recording, and shutdown. The complete set of event type constants and payload schema definitions is available in the [Protocol Library](../architecture/protocol.md).

### Protocol Library Dependencies

Applications constructing `GovernanceEnvelope` transactions or parsing `ActionReceipt` responses should use the g8e Protocol Library, which is published as both a Go module and a Python package. Both share the same version number as the platform binary.

#### Go Module

The protocol is part of the root Go module `github.com/g8e-ai/g8e/v2`. Add it to your project:

```bash
go get github.com/g8e-ai/g8e/v2@v2.1.3
```

The Go package provides protobuf message types for governance envelopes, governance metadata (L1, L2, L3), and all typed payload messages for first-class operations. It also provides SPIFFE workload identity helpers for URI SAN generation and validation.

See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference and example programs.

#### Python Package

Install from PyPI:

```bash
pip install g8e==2.1.3
```

The package provides runtime loaders for JSON protocol constants, dynamic enum generation from those constants, and Pydantic v2 models for protocol data structures including request contexts, platform settings, SSE event wire models, and `GovernanceEnvelope` with deterministic transaction hash generation.

Requires Python 3.10+. Set `G8E_PROTOCOL_DIR` to override the default protocol constants directory. See the [Protocol Library documentation](../architecture/protocol.md) for the full API reference and example scripts.

---

## Building a Minimal Application

### Step 1: Obtain Client Certificate

Generate an mTLS client certificate from the Gateway via CSR-based enrollment:

```bash
./g8e auth enroll user
```

This generates a local keypair, submits a CSR to the Gateway CA, and stores the signed certificate in `.g8e/cli.crt` with the private key in `.g8e/cli.key`. CLI keys are file-backed ECDSA P-256 on all platforms. The Gateway must be running before enrollment (`./g8e gw start`).

The command installs the gateway Root CA into the OS trust store before opening the browser for the passkey ceremony. Before installation, it checks for stale g8e Root CA anchors from previous gateway instances and prompts for removal if found. Use `--no-system-trust` only if an administrator has already installed the Root CA. After trust installation or stale anchor removal, close all open browser windows before clicking the enrollment link.

On Windows, the signed certificate is imported into the Windows Certificate Store for Windows Hello native API access.

### Step 2: Fetch State Root

Query the Gateway state endpoint for the current state root:

```bash
curl -X GET https://localhost:8443/api/v1/state \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key
```

The response includes the `state_merkle_root` field. The `/api/v1/health` endpoint also returns state root information.

### Step 3: Construct Typed Payload

Format the mutation intent according to the protocol schema. For example, a shell execute request uses the `CommandRequested` protobuf message:

```json
{
  "command": "ls -la",
  "execution_id": "unique-execution-id",
  "justification": "List directory contents",
  "vault_mode": "scrubbed",
  "timeout_seconds": 30,
  "intent": "Inspect filesystem",
  "environment": {},
  "working_directory": "/tmp"
}
```

The payload must be serialized as protobuf bytes and base64-encoded in the final envelope.

### Step 4: Generate Transaction Hash

Compute the deterministic transaction hash from the envelope's critical fields. The hash is a SHA-256 digest over the canonicalized action type, target resource, payload, state root, nonce, expiry, intent data, requestor user ID, and acting app ID.

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

The Gateway validates the envelope through the five-layer governance pipeline (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator) before execution. See [Gateway Architecture](../architecture/gateway.md) for the full pipeline description.

### Step 7: Consume Receipt

The Gateway returns a canonical protojson `ActionReceipt` containing deterministic governance-stage evidence and a final durable-persistence attestation. Parse and verify both signatures before consuming the result:

```python
import json
from pathlib import Path

from g8e.receipts import parse_action_receipt, verify_action_receipt_signature, verify_receipt_persistence_attestation

receipt = parse_action_receipt(json.loads(Path("receipt.json").read_text()))
public_key = Path(".g8e/pki/warden_pub.pem").read_text()
if not verify_action_receipt_signature(receipt, public_key):
    raise ValueError("invalid action receipt signature")
if not verify_receipt_persistence_attestation(receipt, public_key):
    raise ValueError("invalid receipt persistence attestation")
```

The application obtains the actuator public key through a trusted out-of-band channel; signature verification does not establish trust in the supplied key.

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
      "consensus_set_id": "<consensus_set_id>",
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

Submit the envelope with L2 signatures to the g8e Gateway. The Gateway verifies the signatures as part of the L2 Consensus check. See [Consensus Architecture](../architecture/consensus.md) for the signature verification and quorum policy details.

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

- Envelopes are accepted by the Gateway.
- Receipts are returned with valid Ed25519 signatures.
- Mutations are executed on connected Operators.
- Audit entries are written to the audit vault.
- The transaction hash matches the envelope ID.

---

## Security Considerations

### No Privileged Channels

Applications must not attempt to establish privileged communication channels with the g8e Gateway or g8e Operators. All communication must go through public ingress paths.

### Fail-Closed Behavior

Applications must handle verification failures gracefully. If the g8e Gateway rejects an envelope, the application must not retry with modified parameters or attempt fallback paths.

### Certificate Management

Applications must manage their mTLS certificates securely. Certificates should be stored securely and rotated before expiry.

### State Root Validation

Applications must validate the state root returned by the g8e Gateway before using it in envelope construction. This prevents man-in-the-middle attacks.

---

## Reference Implementation

The reference implementation of a maximal g8e-compatible agentic ensemble is **g8ee** (the "g8e Agentic Ensemble"), a first-party Python / FastAPI agentic ensemble located in-tree at `ensemble/` in the repository root. g8ee is a first-class g8e client: it holds no privileged Gateway role, authenticates over mTLS, and produces signed governance envelopes like any other L2 consensus producer. It includes an internal consensus mechanism for L2 signature generation, envelope construction and submission using the canonical hash algorithm, receipt verification, and MCP/A2A integration.

The design patterns documented in the [Building an Agentic System](#building-an-agentic-system) section below are derived from g8ee as the canonical worked example.

---

## Building an Agentic System

This section documents the practical steps for building a g8e-compliant agentic ensemble. For the architecture-level overview of the g8e governance boundary and the agent client surface, see [AI Agents and the g8e Governance Boundary](../architecture/agents.md). For the consensus signature verification and quorum policy details, see [Consensus Architecture](../architecture/consensus.md).

A g8e-compliant agentic system is an L2 consensus producer. It consumes the Gateway's protocol surface (MCP tool calls, A2A messaging, governance envelope submission) and produces typed, signed `GovernanceEnvelope` transactions. It has no privileged Gateway role: it is a client that produces consensus signatures.

### Core Principles

- **Intent-Driven Execution**: Reasoning agents never write shell commands directly. They articulate natural-language intent to a Consensus that translates it.
- **Ensemble Consensus**: No single model has mutation authority. Commands are produced by an independent multi-member panel with unique technical lenses.
- **Information Isolation**: Consensus members are blind to each other's candidates. The Auditor receives anonymized candidates to prevent source bias.
- **Fail-Closed Verification**: Any missing signature, stale state root, or L1 violation results in immediate rejection.
- **Host Sovereignty**: The Governed Operator distrusts all upstream inputs and re-verifies everything.
- **Interrogation Gate**: Agents can pause execution to ask clarifying questions via structured interrogation blocks, preventing guessing when context is missing.

### Persona Architecture

The system uses a tiered persona architecture that separates reasoning (intent generation) from consensus (command translation) and defense (risk classification). Every persona is defined with a stable identifier, display name, functional role, model tier, authorized tools, identity block, mission statement, and autonomy boundary.

The layers are:

- **Reasoning Layer**: A triage classifier routes simple turns to a fast responder and complex turns to a primary reasoner. The primary reasoner plans investigations and articulates intent to the Consensus but never writes shell syntax. Security-sensitive requests are always classified as complex.
- **Consensus Layer**: A five-member collective converts intent into commands through Byzantine consensus. A final auditor verifies the consensus output against the original intent and can approve, revise, or swap to a dissenting candidate.
- **Defense Layer**: A coordinator orchestrates risk sub-agents that classify shell command risk, file operation risk, and failure recoverability into consolidated pre-execution verdicts.
- **Utility Layer**: Support agents generate case titles, extract durable user preferences for cross-conversation memory, and evaluate benchmark performance.

See [AI Agents and the g8e Governance Boundary](../architecture/agents.md) for the platform boundary and client surface that constrains every g8e-compatible ensemble.

### Consensus Cascade

The Consensus converts natural-language intent into an executable shell command through a multi-stage cascade with Byzantine fault tolerance. Each stage is independently configurable across providers and models so a single compromised model cannot drive a mutation end-to-end.

1. **Generation**: The intent is dispatched in isolation to five consensus members, each with a unique lens (composition, safety, edge cases, convention, adversary). Each member emits exactly one command string.
2. **Voting**: Members vote with uniform weighting. Minimum consensus is two of five. Ties are broken by a deterministic ladder (shortest command, non-adversary preference, alphabetical fallback).
3. **Round 2**: If round 1 fails to reach consensus, members re-emit with anonymized peer-review context. If round 2 also fails, the error routes back to the reasoner to re-articulate intent.
4. **Risk Analysis**: A Warden coordinator classifies pre-execution risk through sub-agents. Any HIGH risk or inconclusive analysis routes back to the reasoner for a safer alternative. A two-strike circuit breaker forces human intervention on repeated HIGH verdicts.
5. **Auditor Verification**: A final auditor verifies the winning candidate against the original intent and can approve, revise, or swap to a dissenting candidate.
6. **L1 Re-validation**: Any revised or swapped command is re-checked against forbidden patterns before leaving the ensemble.
7. **Envelope Wrap**: The verified command is packaged as a typed payload inside a `GovernanceEnvelope` signed by the L2 Consensus key.
8. **Approval Pipeline**: State-changing operations trigger an approval request, halting execution until a human approves or auto-approval policy applies. L3 auto-approval never bypasses L1 or L2.
9. **Gateway Admission**: The signed envelope is submitted over mTLS to the Gateway, which independently re-runs the full fail-closed validation gauntlet. The ensemble has no privileged channel.

See [Consensus Architecture](../architecture/consensus.md) for the signature format, quorum policy, and posture-dependent enforcement details.

### Memory Model

The system maintains cross-conversation memory that personalizes subsequent turns without storing sensitive data. A background agent runs after each turn to extract durable signals such as communication preferences, technical background, and problem-solving approach from the latest conversation slice. All identifiers (hostnames, IPs, credentials) are redacted from summaries before storage. On subsequent turns, the memory is injected into the prompt assembly as learned context.

The invariant is that the agent reconstructs its prompt from references and summaries, not by holding a database, filesystem, or host session. It receives the minimum useful projection for the current turn.

### Data Sovereignty

Scrubbing is the privacy-preserving default for cloud-model operation. Sensitive categories (API keys, tokens, passwords, private keys, OAuth secrets, credit cards, SSNs) are scrubbed before LLM delivery and before result publication back to the ensemble. Operational data (IPs, hostnames, file paths, URLs without embedded credentials, AWS ARNs) is preserved for troubleshooting. Raw host evidence stays in the Operator Raw Vault and is never AI-readable; AI-facing history comes from scrubbed vaults or typed result payloads.

### Building Your Own

The **g8ee** reference app is the canonical, native implementation of everything above. Read it alongside this guide when building your own ensemble. The steps are language- and provider-agnostic; g8ee is one concrete realization of them.

To build a g8e-compliant agentic system in any language:

1. **Implement the persona model**: Define your agents with stable identifiers, roles, model tiers, authorized tools, identity blocks, mission statements, and autonomy boundaries.
2. **Assemble modular prompts**: Keep static sections first for prefix caching, append dynamic context last. Use structural boundaries to prevent prompt leakage.
3. **Implement the ReAct loop**: Provider turn, tool dispatch, iteration. Route gated tools through your Consensus.
4. **Implement the Consensus cascade**: Multi-member generation, voting, round 2, risk analysis, auditor verification, L1 re-validation, envelope wrap.
5. **Sign envelopes**: Use Ed25519 to sign the transaction hash and decision with your L2 Consensus key. Register the public key as a trusted signer with the Gateway.
6. **Submit over mTLS**: Send the signed `GovernanceEnvelope` to the Gateway's admission endpoint. The Gateway and Operator independently re-verify everything.
7. **Handle results**: Receive pub/sub result envelopes, scrub output, feed back into the ReAct loop.
8. **Maintain memory**: Run a background agent after each turn to extract durable preferences and scrubbed summaries.

The g8e protocol does not care what language your ensemble is written in, what LLM provider you use, or how many agents you run. It cares that your envelopes are correctly signed, bound to the current state root, and pass all L1/L2/L3 gates. Everything above that line is yours to design.

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)**: Connect to, authenticate, use, maintain, and pull reports from a Gateway.
- **[Connect Operator to Gateway](connect_operator_to_gateway.md)**: Deploy and use a g8e Operator.
