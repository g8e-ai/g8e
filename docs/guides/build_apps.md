---
title: Build Apps
parent: Guides
---

# Build g8e-Compatible Applications

Last Updated: 2026-09-08
Version: v2.1.7

---

## Overview

A g8e-compatible application is an untrusted client of the g8e Gateway. It submits typed intent through a public Gateway ingress, receives governed results, and never sends mutations directly to a target host. The Gateway and the bound Governed Operator enforce the five-layer governance pipeline: L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, and L5 Actuator.

The current Gateway supports three application integration patterns:

| Pattern | Client sends | Gateway responsibility | Credential |
| --- | --- | --- | --- |
| **MCP or A2A** | JSON-RPC tool or skill intent | Builds the `GovernanceEnvelope`, binds current state, runs configured L2 deliberation, manages L3 suspension, and dispatches the action | Enrolled app or CLI mTLS certificate |
| **CommandIntent over pub/sub** | Typed `CommandIntent` on `cmd:<operator_id>:<operator_session_id>` | Validates the target session, builds the envelope, binds current state and posture, and forwards the governed command | Enrolled app mTLS certificate and app policy |
| **Direct envelope** | Complete canonical protojson `GovernanceEnvelope` | Verifies the supplied envelope and executes it synchronously; it does not add missing L2 votes | CLI or Operator mTLS certificate; app certificates are denied |

MCP and A2A are the normal application-facing surfaces. Direct envelope submission is a privileged integration for clients that already possess an authorized CLI or Operator transport identity and can construct every posture-required proof correctly.

Application working memory remains application-owned. g8e governs mutations to platform and host state; it does not use the Gateway as an application memory store unless the application explicitly writes a governed platform record.

---

## Choose an Integration Surface

### MCP

Use MCP for standard tool discovery and invocation. The unified `/mcp` endpoint accepts JSON-RPC 2.0. For `tools/call`, the Gateway translates the request into a typed envelope and runs the governance flow.

```bash
curl -X POST https://localhost:8443/mcp \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"run_shell_command","arguments":{"command":"ls -la"}}}'
```

For IDE integrations, `g8e mcp stdio` bridges stdio MCP to the Gateway. See [AI Agents and the g8e Governance Boundary](../architecture/agents.md) for credential resolution and [Connect Apps to Gateway](connect_apps_to_gateway.md) for the tool catalog.

### A2A

Use A2A for a configured downstream skill. The Gateway accepts the `a2a/call` JSON-RPC method at `/api/v1/a2a/call` and governs the resulting `A2ACallRequested` payload.

```bash
curl -X POST https://localhost:8443/api/v1/a2a/call \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"skill_name":"file.read","payload":{"path":"/etc/hosts"},"execution_id":"task-1"}}'
```

The named skill must exist on the configured A2A server. A2A applies the same posture-aware governance flow as MCP.

### CommandIntent over Pub/Sub

An enrolled app can publish a canonical `CommandIntent` to a bound Operator channel. `CommandIntent` contains target identity, event and action classification, serialized protobuf payload bytes, and application context. It does not contain a nonce, expiry, state root, transaction hash, posture, or governance proofs. The Gateway supplies the envelope fields, current state root, posture, nonce, expiry, and hash when it transforms the intent into a `GovernanceEnvelope`; the current pub/sub relay does not run L2 deliberation or L3 suspension.

This is the host-command path used by g8ee under the default `doctrine` posture. Under a posture that requires L2 or L3 for the requested action, an unproved relayed envelope fails closed at Operator verification. Use MCP or A2A when the Gateway must attach L2 votes or manage L3 approval. The Python protocol package provides `g8e.models.governance.CommandIntent` and `CommandIntent.from_payload_bytes()`.

### Direct GovernanceEnvelope

Use direct submission only when the client must control the complete envelope and already has an authorized CLI or Operator certificate. The Gateway’s privileged route registry rejects app SPIFFE identities at `/api/v1/governance/envelopes`, even when the app has a valid app policy.

The direct endpoint verifies the envelope as supplied. Under `consensus` and `notary`, the client supplies valid L2 votes. Under `ratify` and `notary`, mutations also supply a valid L3 proof. The direct endpoint does not invoke the Gateway deliberator or create an approval suspension to fill missing proofs; use MCP or A2A when the Gateway needs to perform those steps.

---

## Identity and Enrollment

### Human and Development Credentials

Run the CLI enrollment coordinator for local testing and human-authorized integrations:

```bash
./g8e gw start
./g8e auth enroll user
```

The coordinator creates or recovers the user and CLI session, generates a file-backed ECDSA P-256 key, and writes `.g8e/cli.crt` and `.g8e/cli.key`. It installs the Gateway Root CA in the OS trust store unless `--no-system-trust` is set. Passkey enrollment is required by notary posture and optional in postures that do not enforce L3.

The CLI certificate carries a SPIFFE identity of the form `spiffe://g8e.local/cli/<user_id>/<session_id>`. Direct mutation envelopes submitted with this certificate still bind a target `operator_id` or `operator_session_id`, and their `cli_session_id` must match the certificate identity.

### External Application Credentials

External applications obtain short-lived delegated credentials from `POST /api/v1/pki/apps/delegated`. The request is authenticated by an enrolled human CLI certificate and contains a P-256 CSR, `app_name`, `app_type`, and optional `organization_id`. The returned certificate is valid for one hour and contains both the app and requesting-user SPIFFE identities. The Gateway also creates the default app policy needed by app authentication.

Delegated enrollment establishes identity only. It does not grant L2 signing authority. An administrator separately enrolls trusted Ed25519 signer keys and a consensus policy when an external service produces protocol L2 votes.

The reserved first-party names `g8ed`, `g8ee`, and `g8eo` use the owner-approved platform enrollment protocol instead of delegated enrollment. That resumable flow uses the request, status, and completion endpoints under `/api/v1/auth/platform-enrollments/`; the first owner reviews requests with:

```bash
./g8e auth pending-platform-enrollments
./g8e auth approve-platform-enrollment <request-id>
```

See [Authentication and Authorization](../architecture/auth.md) for both enrollment protocols and certificate lifetimes.

### App Authorization

An app certificate is accepted only while its `AppPolicy` exists. The current authentication middleware blocks privileged routes and enforces configured request rate and payload-size limits; it also contains collection filtering for query routes, although app identities are currently denied those routes before filtering. `AppPolicy` stores allowed event types, intents, and an L3 requirement, but the current middleware does not enforce those three fields. App certificates cannot access privileged direct-envelope or query routes. Keep app and human/Operator credentials separate; do not treat a successful mTLS handshake as authorization for every endpoint.

---

## Protocol Libraries

### Go

The Go protocol packages are part of the platform module:

```bash
go get github.com/g8e-ai/g8e/v2@v2.1.7
```

Import generated types from `github.com/g8e-ai/g8e/v2/protocol/proto/g8e/...`. The module includes `GovernanceEnvelope`, `CommandIntent`, `ActionReceipt`, typed operation payloads, and SPIFFE workload identity helpers.

### Python

Install the Python protocol package from PyPI:

```bash
pip install g8e==2.1.7
```

The package requires Python 3.10 or later. It includes generated protobuf modules, Pydantic models, protocol constants, deterministic transaction hashing, and receipt parsing and verification helpers. `G8E_PROTOCOL_DIR` overrides the bundled protocol constants directory for development.

### Node

The repository generates TypeScript protobuf bindings under `protocol/node/src/gen`, but `@g8e/protocol-node` is currently private and is not a published client dependency. Node applications generate from the schemas in `protocol/proto` or consume an explicitly packaged in-repository build.

See the [g8e Protocol Library](../architecture/protocol.md) for package contents and code-generation details.

---

## Direct Envelope Construction

### Canonical Wire Format

Direct envelope requests use protobuf JSON mapping (`protojson`). The `payload` field is the base64 representation of a serialized typed protobuf message. MCP and A2A requests use their JSON-RPC contracts instead; they are not serialized `GovernanceEnvelope` messages.

The canonical envelope schema is `g8e.common.v1.GovernanceEnvelope` in `protocol/proto/g8e/common/v1/common.proto`. It contains:

- Transport and target identity: `source_component`, Operator, web, and CLI session IDs, `requestor_user_id`, and `acting_app_id`.
- Intent: `event_type`, typed `payload`, `intent_data`, `action_type`, and `target_resource`.
- State binding: `state_merkle_root`, `nonce`, `transaction_hash`, and `protocol_version`.
- Governance evidence: L1 metadata, an L2 consensus set and votes, and an optional L3 proof.
- Optional application context: case, investigation, task, system fingerprint, tenant, and binding persona.

Clients leave `posture` empty. The Gateway is the posture authority and injects its configured posture before verification. Posture is not part of the transaction hash.

### 1. Fetch the Current State Root

The state endpoint is public so clients can bind an envelope before authenticated submission:

```bash
curl https://localhost:8443/api/v1/state
```

The response contains `state_merkle_root`. `/api/v1/health` also contains the root after the Gateway becomes governance-ready. Validate the Gateway TLS chain and fail if the endpoint is unavailable or returns an empty root. A root can become stale between fetch and submission; on `TX_STATE_MISMATCH`, fetch the new root and construct a new envelope with a fresh nonce, expiry, ID, hash, and signatures.

### 2. Serialize a Typed Payload

A shell request uses `g8e.operator.v1.CommandRequested`:

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

Construct the generated protobuf message, serialize it to bytes, and base64-encode those bytes for the JSON envelope. Do not base64-encode the JSON example itself.

### 3. Compute the Transaction Hash

`id` and `transaction_hash` both equal the lowercase hexadecimal SHA-256 digest produced from these present fields in this order:

1. `action_type`
2. `target_resource`
3. `payload`, represented as standard base64
4. `state_merkle_root`
5. `nonce`
6. `expires_at`, normalized to fixed six-digit microsecond UTC
7. `intent_data`, recursively canonicalized with sorted keys
8. `requestor_user_id`
9. `acting_app_id`

Each present value is followed by `|`; absent and empty values are omitted. Numbers in `intent_data` use fixed-point decimal formatting. L3 and posture are excluded so L2 can sign before human authorization.

Use the protocol helper rather than reimplementing canonicalization:

```python
from g8e.models.governance import compute_transaction_hash

transaction_hash = compute_transaction_hash(
    action_type="EXECUTE_BASH",
    target_resource="/tmp",
    payload=payload_b64,
    state_merkle_root=state_root,
    nonce=nonce,
    expires_at=expires_at,
    intent_data=intent_data,
    requestor_user_id=user_id,
    acting_app_id=app_id,
)
```

### 4. Attach Posture-Required Proofs

L1 is always enforced by the platform; a client-provided `l1.validated` value does not bypass Gateway or Operator validation.

| Gateway posture | L2 | L3 for mutations |
| --- | --- | --- |
| `doctrine` | Audited, not required | Audited, not required |
| `consensus` | Required | Audited, not required |
| `ratify` | Audited, not required | Required |
| `notary` | Required | Required |

Each L2 vote contains `signer_key_id`, `decision`, and `consensus_signature`. The signature is lowercase hexadecimal Ed25519 over the UTF-8 string `<transaction_hash>|<decision>`, where the decision is `true` or `false`. `consensus_set_id` selects an enabled policy, signer IDs must be distinct policy members, and valid affirmative votes must meet quorum.

The Gateway’s built-in consensus service can populate L2 for MCP, A2A, and other Gateway-owned construction paths. It does not populate L2 on direct envelope submission.

### 5. Bind Transport Identity

For mutations, set `operator_id` or `operator_session_id`; normally both identify the target Operator. The envelope identity must match a URI SAN on the certificate used for the HTTP request:

- A CLI certificate matches `cli_session_id`.
- An Operator certificate matches `operator_id` and `operator_session_id`.
- App identities can match app-originated envelopes in internal verification, but app certificates are rejected by the privileged direct-envelope route before the handler runs.

Identity mismatch returns HTTP 403 without execution.

### 6. Submit

```bash
curl -X POST https://localhost:8443/api/v1/governance/envelopes \
  --cert .g8e/cli.crt \
  --key .g8e/cli.key \
  -H "Content-Type: application/json" \
  -d @envelope.json
```

The endpoint returns:

- `200 OK` with a canonical `ActionReceipt` when verification reaches execution. A failed underlying action still returns HTTP 200 with `receipt.status` set to failure because the receipt is the signed outcome.
- `400 Bad Request` for malformed, empty, oversized, or undecodable input.
- `403 Forbidden` for identity, hash, expiry, nonce, state, action, or required L2/L3 proof failures. No action executes and this direct HTTP surface does not return the internally recorded rejection receipt.
- `503 Service Unavailable` when the envelope processor is unavailable.

---

## Verify Action Receipts

An `ActionReceipt` carries the transaction identity, execution status, state roots, deterministic stage evidence, signer key ID, receipt signature, and final durable-persistence attestation. Verify both signatures before trusting the result.

```python
import json
from pathlib import Path

from g8e.receipts import (
    parse_action_receipt,
    verify_action_receipt_signature,
    verify_receipt_persistence_attestation,
)

receipt = parse_action_receipt(json.loads(Path("receipt.json").read_text()))
public_key = Path(".g8e/pki/Actuator_pub.pem").read_text()

if not verify_action_receipt_signature(receipt, public_key):
    raise ValueError("invalid action receipt signature")
if not verify_receipt_persistence_attestation(receipt, public_key):
    raise ValueError("invalid receipt persistence attestation")
```

Obtain actuator public keys through a trusted channel. Unified deployments can return receipts from multiple actuators, so select the public key whose derived key ID matches `receipt.signer_key_id`; do not assume one Gateway key verifies every Operator receipt. Supplying a public key in the same untrusted response does not establish trust.

---

## External L2 Producers

An external L2 producer is separate from an authenticated application identity. It performs these steps:

1. Creates an Ed25519 key for each independent consensus member.
2. Registers each public key as an enabled `TrustedSigner`.
3. Creates an enabled consensus policy whose member IDs match those signer IDs and whose quorum is valid.
4. Evaluates the finalized envelope intent independently.
5. Signs `<transaction_hash>|<decision>` with each member key and attaches the hex signatures to `governance.l2.votes`.
6. Submits the complete envelope through an authorized direct-envelope transport or returns the populated envelope from the consensus deliberation contract.

The app enrollment endpoint deliberately grants no L2 authority. See [Consensus](../architecture/consensus.md) for declarative bootstrap, the admin API, quorum checks, and the remote `/consensus/v1/deliberate` contract.

---

## Building an Agentic Application

An agentic application can add any internal reasoning, generation, voting, risk analysis, and memory architecture above the Gateway contract. Those application-level decisions are not protocol L2 evidence unless enrolled Ed25519 members sign the transaction hash and their votes satisfy the Gateway consensus policy.

The in-tree g8ee application currently uses two dispatch paths:

- For host operations, its five-member Tribunal generates candidate commands, requires two matching candidates, uses deterministic tie breaking, performs a second anonymized peer-review round when needed, runs Warden risk analysis, and sends the audited result as `CommandIntent` over pub/sub. The Gateway constructs the `GovernanceEnvelope` and owns protocol L2 deliberation.
- For governed platform records such as cases, investigations, memories, and reputation state, `GovernanceClient` builds direct envelopes. Because app certificates cannot access the direct endpoint, the unified stack mounts the Operator certificate read-only for this dedicated governance transport. Normal g8ee traffic continues to use its enrolled `spiffe://g8e.local/app/g8ee` identity.

This distinction is security-critical: g8ee’s Tribunal consensus improves command generation, but it does not currently emit protocol L2 signatures. The Gateway’s enrolled consensus service produces and verifies those votes according to posture.

When building a similar system:

1. Keep user intent separate from executable syntax.
2. Serialize every operation with its canonical protobuf payload type.
3. Prefer MCP or A2A when the Gateway must own state binding, L2 deliberation, and L3 suspension; use `CommandIntent` only when its proof-free relay is valid for the active posture.
4. Treat internal model voting as advisory unless it is cryptographically enrolled as protocol L2.
5. Stop on ambiguous intent and request clarification before dispatch.
6. Fail closed on missing credentials, malformed results, receipt verification failure, or required governance rejection.
7. Store only application-owned working state; consume host evidence through governed tools and signed receipts.

See the [g8ee documentation](../ensemble/index.md) for its personas, prompt assembly, Tribunal, SSE events, storage, and evaluation system.

---

## Testing

Test the exact surface and posture the application uses:

- Confirm the certificate chain and expected SPIFFE URI SAN.
- Confirm app-policy authorization separately from TLS authentication.
- Exercise MCP/A2A or `CommandIntent` under each supported posture.
- For direct envelopes, test hash mismatch, expired envelopes, nonce replay, stale state roots, transport identity mismatch, and missing or invalid L2/L3 proofs.
- Verify successful and failed-execution receipts, including deterministic stage evidence and final persistence attestations.
- Test multiple actuator keys when actions can execute on more than one host.
- Confirm that no mutation occurs after any pre-execution rejection.

Contributors to this repository run platform verification through the project wrapper rather than invoking Go tests directly:

```bash
./g8e test unit
./g8e test integration
./g8e test e2e
./g8e test lint
```

External application repositories use their own test runner against a real Gateway and Operator.

---

## Security Requirements

- Send all mutations through a governed Gateway ingress; never connect an application directly to a target host execution path.
- Validate the Gateway TLS chain and protect private keys with least-privilege file or keystore access.
- Keep app, CLI, and Operator identities separate. Never copy privileged credentials into a general application unless the deployment explicitly defines and protects that narrow transport, as the unified g8ee stack does.
- Treat missing or empty state roots as errors. Rebuild a complete envelope after a stale-root rejection; never reuse the old nonce or signatures.
- Do not weaken or silently rewrite intent after a doctrine, identity, or proof rejection. A retry is valid only when it addresses a transient condition such as a newly fetched state root while preserving the authorized intent.
- Verify the receipt signature and final persistence attestation against a trusted actuator key before consuming results.
- Rotate delegated app certificates before their one-hour expiry and revoke compromised identities immediately.

---

## Next Steps

- **[Connect Apps to Gateway](connect_apps_to_gateway.md)**: Gateway ports, protocol surfaces, authentication, and operations.
- **[g8e Protocol Library](../architecture/protocol.md)**: Generated types, models, constants, and receipt helpers.
- **[AI Agents and the g8e Governance Boundary](../architecture/agents.md)**: Agent-facing trust boundary and execution flow.
- **[Consensus](../architecture/consensus.md)**: L2 enrollment, deliberation, signatures, and quorum policy.
- **[Authentication and Authorization](../architecture/auth.md)**: CLI, app, platform, mTLS, and L3 enrollment flows.
- **[g8ee](../ensemble/index.md)**: In-tree reference agentic application.
