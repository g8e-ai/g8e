<div align="left">

# g8e

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8.svg)](https://go.dev) [![CI](https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml/badge.svg)](https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml) [![Go Report Card](https://goreportcard.com/badge/github.com/g8e-ai/g8e)](https://goreportcard.com/report/github.com/g8e-ai/g8e) [![Latest Release](https://img.shields.io/github/v/release/g8e-ai/g8e)](https://github.com/g8e-ai/g8e/releases) [![Status](https://img.shields.io/badge/status-active%20development-orange.svg)](#status) [![Compliance](https://img.shields.io/badge/compliance-SOC2%20ISO%20GDPR-006400.svg)](docs/reference/compliance-alignment.md) [![Secure MCP](https://img.shields.io/badge/Secure-MCP-5D3FD3.svg)](docs/protocols/mcp/mcp.md) [![Protocol g8e](https://img.shields.io/badge/Protocol-g8e-FF6B6B.svg)](docs/architecture/g8e.md)

</div>

<style>
img[alt="License"], img[alt="Go"], img[alt="CI"], img[alt="Go Report Card"], img[alt="Latest Release"], img[alt="Status"], img[alt="Compliance"], img[alt="Secure MCP"], img[alt="Protocol g8e"] {
  height: 28px;
}
</style>

g8e is a reference monitor for agentic infrastructure that provides a fail-closed admission boundary and a sovereign context plane. It is implemented as a single static Go binary. The platform governs state-changing actions on a host and maintains a tamper-evident record of those actions for agent context.

## Architectural Model

The g8e platform treats cloud providers as stateless inference utilities. The cloud model functions as a reasoning coprocessor rather than a stateful execution environment. This design ensures that canonical state resides within the Local-First Audit Architecture (LFAA) on the host.

Context is composed locally from the hash-chained ledger and live host state accessed through governed tools. Only tokenized and scrubbed intent material crosses the sovereignty boundary to the cloud. Payload rehydration occurs at the L5 Actuator layer on the host where the data resides. The model reasons over references while the underlying data remains on the host.

This approach integrates the control plane and data plane into a single system. The proof chain that governs execution also serves as the context substrate. Context delivery and action governance are performed as a single operation on the same object.

### State Settlement and Sovereignty

The platform operates as a context settlement layer where the cloud reasoning layer possesses zero custody of underlying data. The cloud provider is restricted to viewing commitments, such as tokenized payloads, transaction hashes, and state roots. Real state is maintained on the host and updated per transaction, with each update cryptographically superseding the previous state.

The hash-chained ledger serves as a state history. Settlement is performed through execution at the L5 layer and verified against the latest committed state. The system enforces state freshness through the L4 Warden, which rejects any envelope bound to a stale Merkle root.

The platform maintains an asymmetric trust topology where the host is sovereign and the cloud is an untrusted utility. Trust is not extended to the cloud; instead, cloud exposure is limited to cryptographic commitments and dispute resolution.

## Technical Overview

The g8e platform operates as a reference monitor that is tamper-evident, always invoked, and verifiable. It is built as a pure-Go static binary with zero external dependencies. The system functions in two primary roles:

- **Governance Gateway (`g8e gw`)**: This role serves as the Policy Decision Point. It admits signed `GovernanceEnvelope` transactions, manages the platform PKI (mTLS, SPIFFE workload identities), and enforces freshness and replay defense. The gateway relays envelopes to operators and does not possess privileged bypass or execution authority. It does not initiate connections to operators.

- **Governed Operator (`g8e op`)**: This role serves as the Policy Execution Point. It initiates outbound-only mTLS connections to the gateway and does not listen on any ports. It re-verifies every proof locally against its internal state and is the only component authorized to mutate the host.

g8e is actor-agnostic and governs actions rather than actors. AI agents, human users, CI/CD pipelines, and scheduled tasks submit actions through the same admission API. Any component that produces a conformant `GovernanceEnvelope` is treated as a principal.

## System Architecture

g8e integrates action and context planes into a single architectural model.

### Action Plane
Every mutation must clear a five-layer admission pipeline at the host before execution. The system drops and records any actions that are stale, unsigned, unauthorized, or non-compliant with policy. The default state is closed.

### Context Plane
Every admitted action writes a signed `ActionReceipt` to a host-local, git-backed, hash-chained ledger called the Local-First Audit Architecture (LFAA). This occurs before the side effect is executed. The ledger provides a cryptographically provable chain of intent, interpretation, and outcome. Agents derive context from this chain and verify it against live host state through governed tools.

## Admission Pipeline

The admission pipeline consists of five layers with independent failure domains:

1. **L1 Doctrine**: Deterministic static analysis. It enforces rules against forbidden patterns and MITRE ATT&CK indicators. This layer is active for every action.
2. **L2 Consensus**: Multi-model consensus. It requires Ed25519 signing over the canonical SHA-256 transaction hash.
3. **L3 Notary**: Hardware-bound human authorization. It utilizes WebAuthn/FIDO2 passkey assertions computed over the transaction hash.
4. **L4 Warden**: Fail-closed verification authority. It re-verifies all proofs against local state, signatures, freshness, and the state Merkle root.
5. **L5 Actuator**: Single dispatch path. It handles tool invocation and enforces data sovereignty.

## Data Sovereignty

The platform enforces data sovereignty through several mechanisms:
- Raw data remains on the host. Tokenization and scrubbing occur before intent material crosses the boundary.
- The transaction hash is computed over the tokenized payload.
- Transport credentials function as evidence within the envelope rather than bypass mechanisms.
- The audit record is written before any side effect occurs.

## Quick Start

The binary is available for linux, darwin, and windows on amd64 and arm64 architectures. It can also be built from source.

```bash
# Start the Gateway
./g8e gw start

# Authenticate the CLI
./g8e auth login

# Deploy an Operator to remote hosts
./g8e operator deploy --hosts <host1,host2> --background

# Check Gateway status
./g8e gw status

# Query the audit vault
./g8e gw data audit list --operator-session-id <session-id>
```

### Posture Configurations

The gateway supports three posture configurations:

| Posture | L1 Doctrine | L2 Consensus | L3 Notary |
| --- | --- | --- | --- |
| `doctrine` | enforced | audited | audited |
| `consensus` | enforced | enforced | audited |
| `notary` | enforced | enforced | enforced |

L4 Warden and L5 Actuator layers are always active in all configurations.

## Compliance and Standards

The g8e platform is designed for environments requiring zero trust architecture as defined in NIST 800-207. It aligns with NIST AI RMF, CMMC, FedRAMP, ISO 42001, and SOC 2 requirements. The LFAA ledger provides a continuous evidence trail for these frameworks.

## Status

**v1.1.1**: Current release. Includes core protocol, gateway and operator roles, five-layer pipeline, PKI/mTLS identity, WebAuthn notary, MCP/A2A protocol translation, LFAA audit vault, native tools, and multi-platform support.

## Documentation

Documentation is available in the `docs/` directory:

- [Getting Started](docs/guides/getting_started.md)
- [Position Paper](docs/core/position_paper.md)
- [Operator Architecture](docs/architecture/operator.md)
- [Gateway Architecture](docs/architecture/gateway.md)
- [Security Model](docs/architecture/auth.md)
- [MCP Integration](docs/protocols/mcp/mcp.md)
- [CLI Reference](docs/guides/cli.md)
- [Glossary](docs/reference/glossary.md)

---

Apache 2.0. Built by Lateralus Labs.
