# g8e Protocol Substrate

The **g8e Protocol** is the bedrock of the platform: a governance-first wire contract for trustless, human-verified action by autonomous systems.

## Core Documents

- [**Protocol Contract**](protocol.md) — The wire format (UAP JSON), transaction hashes, and payload definitions.
- [**Governance Hierarchy**](governance.md) — The 3-layer (L1/L2/L3) validation model: Hard Gates, Consensus, and Authorization.
- [**Security Principles**](security.md) — Trustless execution, host sovereignty, and cryptographic anchoring.
- [**Events & Status**](events.md) — Canonical registries for platform-wide events and state transitions.
- [**Pub/Sub & Messaging**](pubsub.md) — Real-time transaction flow and state synchronization.

## Architecture

The protocol is designed as a **substrate**: it is indifferent to the specific application or the specific operator implementation. Any system that speaks the g8e Protocol can participate in the governance network.

For the reference implementations, see:
- [**g8eo (Operator)**](../g8eo/README.md)
- [**g8ee (Engine App)**](../g8ee/README.md)
