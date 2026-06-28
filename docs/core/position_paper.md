---
title: Position Paper
parent: Core
---

# Position Paper

## The Custody Problem

Every contemporary agent architecture makes the same trade: give the cloud custody of your data to get frontier reasoning. The model needs context to reason. Context is your data. So your data goes to the model. The provider accumulates it, persists it, and may train on it. You get reasoning in exchange for custody. This trade is presented as inevitable. It is not.

The trade exists because current architectures conflate two functions that should be separated: reasoning and state. The model reasons. The host remembers. When both live in the cloud, the provider holds custody by construction. When the host remembers and the cloud only reasons, custody stays with the data owner. The model receives tokenized projections and cryptographic commitments, never raw data. It returns conclusions. The host verifies, executes, and records.

g8e implements this separation. The cloud functions as a stateless reasoning co-processor. The host maintains canonical state, encryption keys, and the audit ledger. The boundary between them carries proofs in one direction and commitments in the other, never custody.

## The Sovereignty Inversion

The architectural mechanism is an inversion of control over state and trust. The gateway can live in the cloud. The operator lives at the site of the data owner. The data owner trusts nobody: not the cloud provider, not the gateway, not the network between them.

The operator opens a single outbound mTLS connection to the gateway. It listens on no ports. It accepts no inbound connections. It can sit behind NAT, firewalls, or air gaps. The gateway cannot reach into the operator; the operator pulls work when it chooses. No installation is required beyond a single static binary. No firewall rules need to be opened. No listening ports need to be configured on the managed host.

When the operator retrieves an envelope from the gateway, it does not trust the gateway's verification. It re-derives every proof from scratch against its own local state. L1 doctrine is re-evaluated locally. L2 consensus signatures are re-verified locally. L3 human authorization is re-checked locally. The state Merkle root is compared against the operator's own committed state. If any proof is stale, tampered, or missing, the transaction is rejected. The gateway is a relay, not an authority. The operator is the authority, and the operator lives where the data lives.

This inversion is what makes the cloud safe to use as a reasoning utility. A compromised gateway cannot inject actions because the operator re-verifies everything. A compromised cloud provider cannot decrypt host data because vault keys were never shared. A compromised network cannot intercept raw data because only tokenized projections cross the boundary.

## Commitments, Not Custody

The cloud reasoning layer receives commitments, not data. A commitment is a cryptographic binding to a specific state of the world at a specific moment. The transaction hash binds the intent. The state Merkle root binds the host state. The nonce and expiration timestamp bind the moment. Together, these form a commitment that the model can reason over without ever seeing the underlying values.

Before any intent material crosses the sovereignty boundary, it is tokenized and scrubbed. Secrets, regulated data, and personally identifiable information are replaced with opaque tokens. The transaction hash is computed over the tokenized payload. The model upstream reasons over a safe projection of reality. It sees structure without substance. It can infer, plan, and recommend, but it cannot exfiltrate what it cannot read.

Rehydration happens only at the L5 Actuator, at the instant of execution, on the host where the data already lives. The actuator resolves tokens to real values, executes the verified action, and records the result. The real values never leave the host. The cloud model never sees them. The gateway never sees them. Only the operator, running at the site of the data owner, with keys owned by the data owner, performs rehydration.

This is the mechanism that reduces cloud providers to co-processors. The provider supplies reasoning. The host supplies state, keys, and execution. The provider cannot reconstruct the data because it only ever held tokens. The provider cannot replay the action because the commitment is bound to a specific transaction hash, state root, and nonce. The provider cannot escalate because the permissions minted from the envelope are scoped to a single action and dissolved on completion.

## The Unified Context and Control Plane

Current agent architectures separate the control plane (what actions are permitted) from the data plane (what the agent knows). The control plane enforces policy. The data plane provides context. When these are separate, the data plane is a trusted storage layer that can be poisoned, and the control plane is a gate that can be bypassed if the data plane is compromised.

g8e unifies them. The hash-chained ledger that governs execution also serves as the context substrate. Every admitted action writes a signed `ActionReceipt` to a host-local, git-backed, hash-chained ledger before the side effect is executed. This ledger is the enforcement record of the write path and the verifiable memory of the read path. Agents derive context from this chain and verify it against live host state through governed tools.

Context delivery and action governance are the same operation on the same object. An agent whose actions are gated by cryptographic proof and whose beliefs are derived from a verifiable ledger has no trusted storage to poison. The ledger is tamper-evident: each entry is hash-chained to the previous entry, and the chain root is included in every subsequent transaction's state binding. An attacker who modifies a historical entry breaks the chain. An attacker who injects a fabricated entry cannot produce a valid signature from the actuator's Ed25519 key.

The ledger never leaves the host. The cloud provider sees commitments (transaction hashes, state roots) but not the ledger contents. The gateway sees envelopes but not the audit vault. The audit vault is encrypted at rest with AES-256-GCM using keys owned by the data owner. The ledger is memory, and memory is sovereign.

## Proof of Human Presence

High-risk mutations require proof that a human authorized the exact action being executed. g8e uses WebAuthn/FIDO2 passkey assertions computed over the transaction hash. The human signs the exact bytes of one transaction. The signature is bound to the transaction hash, the nonce, the expiration timestamp, and the state Merkle root. It cannot be transplanted to a different action, replayed against a later request, or harvested from a live session.

This is distinct from session-based authentication. A session token grants ongoing authority to act on behalf of a user. A passkey assertion over a transaction hash grants authority for one action, at one moment, against one state of the host. The approval expires with the transaction. There is no standing authorization to revoke because there was never standing authorization to begin with.

Human signatures are rare and expensive. Each one requires a physical interaction with a hardware-backed key: a touch, a face scan, a PIN entry. This cost is intentional. It makes human authorization a non-recoverable bond. When a human signs a transaction, they are expressing genuine belief that the action should proceed. The system does not ask for this belief often, and it does not accept it cheaply. The L3 Notary layer is fail-closed under notary posture: mutations without a valid human proof are rejected before execution.

## Zero Standing Privileges

The operator holds no permanent administrative credentials. Permissions are minted just-in-time from the verified intent inside the governance envelope. A capability is scoped to a single action: one tool call, one file edit, one command execution. The capability is dissolved the moment the action completes. There is no credential store to compromise, no token to steal, no role to assume.

This applies to every layer. The gateway does not hold execution authority; it relays envelopes. The operator does not hold standing admin rights; it mints scoped capabilities per action. The human does not hold a session; they sign one transaction. The model does not hold data; it reasons over commitments. No component in the system accumulates privilege over time.

A compromise of any single layer cannot exfiltrate persistent credentials because none exist. A compromised gateway cannot execute actions because it has no execution path. A compromised operator cannot escalate beyond the scoped capability minted from the verified envelope. A compromised cloud provider cannot decrypt host data because the vault keys were never shared. The system's security does not depend on the integrity of any single component. It depends on the fact that no component holds enough privilege to cause harm in isolation.

## Cryptographic Binding

Every proof in the system is rigidly attached to one action, one moment, and one host. The transaction hash is a deterministic SHA-256 over the canonical JSON serialization of the governance envelope. The envelope contains the action type, the payload, the nonce, the expiration timestamp, and the state Merkle root. Changing any field changes the hash. Changing the hash invalidates every signature attached to it.

This binding prevents replay, tampering, and substitution. A proof valid for one transaction is invalid for every other transaction. A proof valid at one moment is invalid at any other moment because the expiration timestamp is part of the hash. A proof valid against one state of the host is invalid against any other state because the state Merkle root is part of the hash. The operator re-derives the state root from its local state and compares it to the root in the envelope. If the host state has changed since the envelope was created, the proof is stale and the transaction is rejected.

The state Merkle root is the mechanism that binds proofs to the host's actual state. It is a hash of the current state of the ledger and all tracked host resources. When an action executes, the state changes, and the root changes. The next transaction must commit to the new root. This creates a chain: each transaction is bound to the state that resulted from all previous transactions. An attacker cannot insert a transaction between two existing ones because the state roots would not chain. An attacker cannot modify a past transaction because the hash would change and break the chain.

## The Ledger as Memory

The hash-chained ledger serves two roles simultaneously. It is the enforcement record of the write path: every admitted action, every rejection, every receipt. It is also the verifiable memory of the read path: the context substrate from which agents derive beliefs about the host.

These two roles are unified in a single data structure because they are the same data. The history of what was done is the context for what to do next. An agent that reads the ledger knows what actions were attempted, which succeeded, which were rejected, and what state resulted. An agent that verifies the ledger knows the chain is intact, the signatures are valid, and the state roots are consistent. An agent that extends the ledger must produce a valid envelope, clear the admission pipeline, and receive a signed receipt. Reading, verifying, and writing are governed by the same cryptographic primitives.

The ledger is git-backed. Every file mutation triggers a two-phase commit: a pre-mutation snapshot and a post-mutation snapshot. This provides rollback capability and a tamper-evident history trail. The ledger is encrypted at rest. File contents are encrypted before storage using the host's vault keys. A compromised host disk does not reveal file contents. A compromised backup does not reveal file contents. Only the operator, with an unlocked vault, can read the ledger.

The ledger never leaves the host. The cloud provider sees commitments but not ledger contents. The gateway sees envelopes but not the audit vault. The audit vault is the host's memory, and memory is sovereign.

## Encryption as a Sovereignty Guarantee

All sensitive data at rest is encrypted with AES-256-GCM using keys that never leave the host in plaintext. The vault architecture uses a layered key hierarchy: a master key wraps domain keys, domain keys wrap data keys, and data keys encrypt payloads. Key rotation is supported through rekey operations that re-wrap domain keys without re-encrypting underlying data.

The encryption layer is mandatory, not optional. Storage services fail to initialize without an unlocked vault. The vault must be unlocked at startup with the master key. If the vault is locked, the operator cannot read the audit store, cannot read the ledger, cannot read the execution vault, cannot read the token store. The system fails closed rather than operating without encryption.

This is the mechanism that makes the data owner's key ownership meaningful. The keys are generated on the host, stored on the host, and never shared with the gateway or cloud provider. A compromised cloud cannot decrypt host data. A compromised gateway cannot decrypt host data. A subpoena to the platform vendor yields no data because the vendor never held the keys. The data owner retains sole control over who can read their data.

## The Gateway-Operator Relationship

The gateway and the operator are two roles implemented by the same binary. The gateway is the Policy Decision Point: it admits signed envelopes, manages PKI, and enforces freshness and replay defense. The operator is the Policy Execution Point: it re-verifies proofs locally, executes actions, and maintains the audit ledger. The gateway does not execute. The operator does not admit. The separation is architectural, not configurational.

The gateway can run in the cloud. The operator runs at the site of the data owner. The operator initiates a single outbound mTLS connection to the gateway. The gateway does not initiate connections to the operator. The operator pulls work when it chooses and can disconnect at any time. When disconnected, the operator continues to serve the host from its local state. The gateway queues envelopes; the operator retrieves them when connectivity is restored.

This relationship is what makes the platform deployable in environments where inbound connectivity is impossible or prohibited. A hospital network that blocks inbound connections to clinical systems can still run an operator: the operator dials out to the gateway, retrieves pending envelopes, and executes them locally. A tactical edge network with intermittent connectivity can still run an operator: the operator caches work, executes when connected, and syncs receipts when bandwidth is available. An air-gapped facility can still run an operator: the operator runs standalone with locally configured doctrine, and envelopes are transferred via physical media.

The gateway and the operator share no filesystem, no database, no memory. They communicate exclusively through the mTLS channel and the governance envelope. The gateway sees envelopes and commitments. The operator sees envelopes, state, and raw data. The gateway's compromise does not expose data. The operator's compromise does not expose the gateway's PKI. Each component's failure domain is isolated.

## Domain Applications

The gateway-operator architecture is domain-agnostic. The same binary, the same protocol, and the same five-layer verification pipeline governs actions across industries. What changes between domains is the doctrine configuration, the target data, and the governance posture. The data owner configures these to match their regulatory and operational requirements.

In healthcare, the operator governs clinical AI actions on electronic health record systems. Doctrine rules enforce PHI scrubbing patterns and prior authorization workflow gates. The cloud model reasons over tokenized clinical data and returns treatment recommendations. The operator rehydrates tokens locally, executes the verified action against the EHR, and records the result in an encrypted, tamper-evident ledger. Patient data never leaves the hospital network. The cloud provider never sees PHI. The gateway never sees clinical notes.

In government and defense, the operator governs actions on classified document stores and tactical sensor systems. Doctrine rules enforce classification markings, exfiltration prevention, GPS spoofing defense, and weapons safety constraints. The operator runs on tactical edge hardware with intermittent connectivity. The gateway runs in a secure cloud or on-premises. Sensor data, RF environment data, and payload manifests remain on the edge. The cloud model reasons over tokenized projections and returns targeting or cueing recommendations. The operator re-verifies all proofs locally before any actuator command is dispatched.

In financial services, the operator governs algorithmic trading actions. Doctrine rules enforce trade limits, dual-control triggers, and counterparty exposure constraints. The cloud model reasons over tokenized market data and position information. The operator executes verified trades locally and records every action in a tamper-evident ledger that satisfies regulatory audit requirements. Trading positions and counterparty information never leave the trading floor.

In critical infrastructure, the operator governs process control actions on SCADA and industrial control systems. Doctrine rules enforce safety interlocks, configuration change controls, and operational boundaries. The operator runs on the plant floor. The gateway runs in a corporate or cloud environment. Process data, operational telemetry, and facility configurations remain on the plant network. The cloud model reasons over tokenized projections and returns optimization recommendations. The operator re-verifies all proofs before any control command is dispatched to the physical system.

In each domain, the same architectural invariants hold: state remains local, keys are owned by the data owner, the cloud is a stateless reasoning co-processor, and every action is governed by the same five-layer verification pipeline. The platform does not need domain-specific code. It needs domain-specific doctrine, which is data, not code. The data owner writes the rules. The platform enforces them.

## The Inversion in Practice

Consider a hospital that wants to use a frontier model to assist with prior authorization decisions. The model runs in the cloud. The patient records live in the hospital's EHR system. Current architectures require the hospital to send patient data to the cloud so the model can reason over it. The hospital must trust the cloud provider with PHI. The hospital must accept that the provider may persist, log, or train on that data.

With g8e, the hospital deploys an operator on the hospital network. The operator connects outbound to a gateway, which may also run on the hospital network or in a cloud the hospital controls. The clinical AI agent submits a prior authorization request as a governance envelope. The envelope contains the tokenized clinical context: diagnoses and procedures are represented as opaque tokens, not raw text. The model in the cloud reasons over the tokenized context and returns a recommendation. The recommendation is wrapped in a governance envelope and sent to the operator. The operator re-verifies all proofs, rehydrates the tokens to real clinical values locally, executes the prior authorization action against the EHR, and records the result in an encrypted, tamper-evident ledger.

The cloud model never saw the patient's name, diagnosis, or treatment plan. It saw tokens. The gateway never saw the clinical data. It saw envelopes. The operator, running on the hospital network, with keys owned by the hospital, performed the rehydration and execution. The audit ledger, encrypted with the hospital's keys, records exactly what was done, when, and by whose authority. If the cloud provider is compromised, the attacker finds tokens they cannot resolve. If the gateway is compromised, the attacker finds envelopes they cannot execute. If the network is intercepted, the attacker finds mTLS-encrypted traffic they cannot decrypt.

This is the sovereignty inversion in practice. The hospital gets frontier reasoning without surrendering custody. The cloud provider is reduced to a co-processor. The data owner retains state, keys, and audit. The platform enforces this not through policy or promise, but through cryptographic construction.

## Related Documentation

- [About g8e](./about.md): Platform overview and architectural differentiators.
- [Gateway Architecture](../architecture/gateway.md): Gateway role, capabilities, and port topology.
- [Operator Architecture](../architecture/operator.md): Operator role, native tools, and local audit.
- [Authentication](../architecture/auth.md): mTLS, SPIFFE, PKI, and the five-layer verification sequence.
- [Encryption](../architecture/encryption.md): Vault architecture, key hierarchy, and cryptographic primitives.
- [Storage Architecture](../architecture/storage.md): Audit store, ledger, execution vault, and data flow.
- [Network Architecture](../architecture/network.md): PKI, mTLS, enrollment, and outbound-only connectivity.
- [Governance](../architecture/governance.md): Five-layer pipeline, posture configurations, and transaction flow.
