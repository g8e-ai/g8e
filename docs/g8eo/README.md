# g8eo — Reference Operator

**g8eo** is the reference Go implementation of the **Operator** role defined by the g8e Protocol. It is a sovereign, single-binary execution boundary that enforces the protocol's 3-layer governance hierarchy.

## Core Documents

- [**Operator Architecture**](architecture.md) — Lifecycle, operating modes (Hub vs. Satellite), and verification pipeline.
- [**Storage & LFAA**](storage.md) — Deep dive into the Local-First Audit Architecture, Multi-Ledger (git), and SQLite vaults.
- [**PKI & Identity**](pki.md) — Certificate Authority, mTLS, and SPIFFE URI SAN identity.

## Operating Modes

A single `g8eo` binary runs in one of two primary roles:

1.  **Hub (`--listen`)**: The platform substrate. Provides central persistence (authoritative DB + optional KV cache), PKI authority, and pub/sub for a fleet.
2.  **Satellite**: The execution role. Runs on a managed host, dials the Hub via outbound-only mTLS, and executes protocol-governed transactions.

## Capabilities

- **Protocol Enforcement**: Verifies L1/L2/L3 governance before any execution.
- **Warden Execution**: A defensive on-host execution boundary that captures results into the LFAA ledger.
- **Local-First Audit**: Every mutation is anchored to an operator session-scoped, git-backed ledger on the host.
- **Outbound-only Connectivity**: Satellites require no inbound ports; all traffic is over secure mTLS WebSockets.

For the protocol contract this implementation enforces, see [**g8e Protocol**](../protocol/README.md).
