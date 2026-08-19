# PKI & Trust

## Overview

g8e uses a hierarchical Public Key Infrastructure to establish trust between the Gateway, Operators, and the Agentic Ensemble. All cryptographic operations use Ed25519 signatures.

## Trust Hierarchy

- **Platform CA** — Root certificate authority owned by the Governance Gateway
- **Operator Certificates** — Issued to governed operators via CSR signing
- **Agent Keys** — Ed25519 keypairs for each agent in the ensemble
- **Session Credentials** — Short-lived credentials for authenticated sessions

## Certificate Lifecycle

1. **Enrollment** — Operator generates keypair, submits CSR to Gateway (HTTP port 8080)
2. **Signing** — Gateway validates and signs the CSR, issuing an operator certificate
3. **Trust Bundle** — Operator downloads the platform trust bundle
4. **mTLS** — Operator uses certificate for mTLS on HTTPS surface (port 8443)
5. **Rotation** — Certificates are rotated on a defined schedule

## Agent Key Management

Each agent in the ensemble has its own Ed25519 keypair used for:

- Signing GovernanceEnvelope transactions
- Participating in L2 consensus rounds
- Establishing non-repudiation for agent decisions

## Related

- [Governance](governance.md) — How PKI enables L2/L3 governance
- [Protocol](protocol.md) — Protocol surfaces for CSR signing and trust bundles
- [Storage](storage.md) — Where keys and certificates are stored
