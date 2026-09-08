---
title: L2 Consensus
parent: Architecture
---

# L2 Consensus

Last Updated: 2026-09-08
Version: v2.1.7

## Scope

L2 Consensus is the machine-authorization layer in the g8e five-layer interlock. It is a K-of-N Ed25519 signature policy, not a Byzantine fault-tolerant protocol, leader-election system, or replicated state machine. An enabled policy identifies trusted signer identities and the number of affirmative votes required to authorize a transaction.

The bundled Consensus service is the reference vote producer. It applies the same deterministic L1 Doctrine analysis for every locally configured member and signs each result with that member's available key. It does not run heterogeneous models or perform application-level agent reasoning. The g8ee Tribunal and other multi-model voting systems remain advisory unless enrolled L2 identities produce valid protocol votes.

See [Governance](./governance.md) for the complete verification pipeline, [AI Agents and the g8e Governance Boundary](./agents.md) for the distinction between protocol consensus and application reasoning, and [Authentication and Authorization](./auth.md) for trusted identities.

---

## Place in the Five-Layer Interlock

1. **L1 Doctrine** decodes the typed payload and applies field constraints, forbidden-pattern checks, and MITRE ATT&CK-oriented threat detection.
2. **L2 Consensus** verifies member votes over the transaction hash against an enabled policy and trusted signer keys.
3. **L3 Notary** verifies human authorization for mutation actions when the posture requires it.
4. **L4 Warden** performs replay, expiry, hash, state-root, L1, L2, and L3 checks before dispatch.
5. **L5 Actuator** executes the admitted operation and produces signed receipt evidence.

The active posture determines whether L2 is a required gate:

| Posture | L2 behavior |
| --- | --- |
| `doctrine` | Not required; supplied evidence is non-gating. |
| `consensus` | Required for non-bootstrap transactions. |
| `ratify` | Not required; supplied evidence is non-gating. |
| `notary` | Required for non-bootstrap transactions. |

When L2 is not required, missing or invalid votes do not reject a transaction. The Warden evaluates supplied votes only when the vote set, signer store, and policy store are available. The signed action receipt reports L2 as not required even when optional vote evidence is valid.

---

## Policy and Trust Model

A consensus policy contains a stable identifier, a set of member application IDs, a quorum, a duplicate-vote rule, and an enabled state. Every member ID resolves to an enabled trusted signer whose Ed25519 public key verifies that member's votes. The policy and trusted signer records live in the Gateway's canonical document store, while an executing Warden reads the selected policy through the generic L2 policy interface.

Policy writes fail closed. A new policy requires a valid identifier, at least one unique non-empty member, a quorum from one through the member count, enabled trusted signers for every member, and `enabled=true`. An existing policy cannot be silently overwritten while enabled; the supported update path disables it, and deletion removes it.

The quorum counts affirmative votes from trusted policy members. Each signer identity contributes at most one vote. When distinctness is required, a duplicate member identity rejects L2 under an L2-enforcing posture; otherwise the duplicate is ignored. Votes from identities outside the policy, missing or disabled trusted signers, malformed signatures, invalid signatures, and negative decisions do not contribute to quorum.

---

## Vote Production

The Gateway-owned MCP tool, MCP resource, MCP prompt, and A2A call paths construct a canonical `GovernanceEnvelope`. Under `consensus` and `notary`, those paths send the completed envelope to the configured L2 deliberator before L4 verification. The reference Gateway wires an in-process deliberator from the policy selected by `--consensus-id`.

Direct envelope submission does not add missing votes, so its caller must supply a complete vote set under `consensus` or `notary`. The outbound Operator `CommandIntent` relay also constructs an envelope without running L2 deliberation, but its intent format cannot carry L2 votes. Non-bootstrap commands sent through that relay therefore fail L2 verification under an L2-enforcing posture.

The Gateway exposes an mTLS-protected deliberation route only while running a posture that requires L2. The route accepts and returns canonical protojson envelopes and limits request bodies to 1 MiB. The `--consensus-url` and `G8E_CONSENSUS_URL` settings are retained in Gateway configuration, but the reference Gateway does not construct a remote HTTP deliberation client from them; its automatic MCP and A2A flow uses the in-process service.

### Reference Deliberation Behavior

The reference service recomputes the envelope ID from the hash-bound transaction fields and rejects a mismatch. It evaluates structured intent data when present, otherwise it evaluates the typed payload bytes. Every member with an available private key applies the same L1 Doctrine analysis, treats any block-recommended signal as unsafe, and signs its Boolean decision.

Each signature covers the UTF-8 string `transaction_hash|decision`, where the decision is `true` or `false`; the Ed25519 signature is hex-encoded. The service attaches the selected consensus ID and all produced votes to the envelope. Deliberation fails if no member key is available, but it does not decide whether quorum is met; the L4 Warden makes that authorization decision.

---

## Configuring the Reference Gateway

Start an L2-enforcing Gateway with `--posture consensus` or `--posture notary` and select the policy with `--consensus-id`. The equivalent environment setting is `G8E_CONSENSUS_ID`. The Gateway logs an advisory warning and continues startup when the ID is absent or the selected policy is missing or disabled; subsequent non-bootstrap transactions fail closed at L4.

The local Consensus service is assembled during Gateway construction. A policy created or replaced through the admin surface after startup does not hot-load a new local deliberator, so restart the Gateway after changing the selected policy or its local member keys. Disabling or deleting the active policy takes effect at verification because the Warden reads the policy store for each transaction, causing later L2-gated transactions to fail.

### Declarative Bootstrap

`--consensus-bootstrap`, or `G8E_CONSENSUS_BOOTSTRAP`, points to a JSON bootstrap file. The file identifies the consensus, member application IDs, and quorum; it can also provide `member_seeds` or `seed_hex`. Bootstrap runs before Gateway service construction, so the selected policy and local deliberator are available in the same startup when `--consensus-id` matches the bootstrapped ID.

`member_seeds` takes precedence and requires a seed for every listed member. The Gateway derives each public key, enrolls it as that member's trusted signer, and stores the corresponding private seed with restricted permissions for local deliberation. This mode gives every identity distinct key material, although the reference single-binary deployment still holds all local member keys in one Gateway runtime.

If `member_seeds` is absent, `seed_hex` supplies one key pair for every member identity. If both seed forms are absent, the Gateway generates one key pair and shares it across every member identity. These fallback modes satisfy identity-based quorum checks but do not provide independent key custody, so they are suitable only for controlled demonstrations and single-key deployments, not independent K-of-N authorization.

Bootstrap always creates an enabled policy with distinct votes required. If a policy with the same ID already exists, bootstrap skips signer enrollment, key persistence, and policy creation without reconciling the existing state. Changes to the bootstrap file therefore have no effect while that policy remains in the store.

### Administrative Enrollment

The first enrolled user can create, list, disable, and delete policies through the authenticated admin surface. Member trusted signers must already exist and be enabled before policy creation. Administrative policy creation does not provision local private keys, and changing a policy does not rewire the running in-process deliberator.

At runtime, local key resolution first checks the stored member seed. If no file key is available and a member ID equals the Gateway Actuator key ID, the reference service can use the Actuator key for that member. Other members without available private keys remain policy members but produce no local vote, which can prevent the configured quorum from being reached.

---

## L4 Verification

For every envelope, L4 recomputes the transaction hash and verifies each candidate vote against that computed value rather than trusting the declared hash. Under `consensus` and `notary`, verification requires a non-empty vote set, configured signer and policy stores, and an enabled policy named by the vote set. Store failures, duplicate identities when distinctness is required, and insufficient affirmative signatures reject the transaction.

The Warden excludes non-members and votes whose trusted key cannot be loaded or whose signature is invalid. A valid negative vote authenticates the member's decision but does not count toward quorum. Authorization succeeds only when the number of valid affirmative member votes reaches the policy quorum.

Under `doctrine` and `ratify`, the same evidence is non-gating. Missing dependencies or invalid evidence produce an L2 result of false without rejecting the transaction, including duplicate identities. This optional result is internal verification evidence; the action receipt records the posture-level status as L2 not required.

### Bootstrap Enrollment Exception

The platform enrollment lifecycle creates the identities and sessions that later governance depends on. Its bootstrap action types are exempt from L2 enforcement under every posture to avoid requiring a consensus body before that body can be enrolled. L1 and the universal L4 integrity checks still apply, while any supplied L2 evidence remains non-gating.

---

## End-to-End Flow

1. A client uses a Gateway path that constructs an envelope, or submits a complete canonical envelope.
2. The producer binds executable intent, target, state root, replay controls, and principal attribution into the transaction hash.
3. Under `consensus` or `notary`, Gateway-owned MCP and A2A paths ask the local Consensus service for member votes when it is configured.
4. L4 recomputes the hash, applies universal checks, loads the vote set's policy, verifies trusted member signatures, and enforces quorum when required.
5. L5 executes an admitted transaction and signs a receipt whose L2 status reflects whether the active posture required and accepted consensus.

---

## Security Properties and Limits

- L2 authorizes an exact transaction hash and Boolean decision; changing a hash-bound field invalidates every vote.
- Policy membership alone does not grant signing capability. A vote also requires the corresponding private key and an enabled trusted public key.
- Identity distinctness is not key distinctness. Shared bootstrap keys allow one key holder to sign as multiple configured member IDs.
- The bundled vote producer is deterministic and local. It does not provide model diversity, network fault tolerance, or independent host custody by itself.
- L4, not the vote producer, is the authorization boundary. Vote production can return fewer votes than quorum, while L4 still fails closed under an enforcing posture.
- Direct envelopes and outbound command relays do not receive missing L2 proofs automatically.
- The selected policy and local deliberator are construction-time dependencies; policy administration is not a hot-reload mechanism.

---

## Related Documentation

- [Governance](./governance.md): Five-layer verification, posture behavior, and signed receipts.
- [AI Agents and the g8e Governance Boundary](./agents.md): Client ingress paths and the separation between protocol L2 and application-level voting.
- [Gateway Architecture](./gateway.md): Gateway service composition, ingress, and local execution.
- [Operator Architecture](./operator.md): Remote L4/L5 verification and execution.
- [Authentication and Authorization](./auth.md): mTLS identities, trusted signers, and L3 authorization.
- [Build a Gateway](../guides/build_gateway.md): Gateway startup flags and deployment workflow.
