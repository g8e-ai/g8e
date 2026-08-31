# Security Policy

**Project:** g8e — Byzantine Fault Tolerant Governance Platform  
**Maintained by:** [Lateralus Labs](https://lateraluslabs.com)

---

## Supported Versions

| Version | Supported |
|---|---|
| `main` (latest) | ✅ |
| Older releases | ❌ — upgrade to latest |

---

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Report privately to: **security@lateraluslabs.com**

Include as much of the following as you can:

- Description of the vulnerability and its potential impact
- Affected component(s): g8e Gateway (PDP), g8e Operator (PEP), g8e Protocol
- Steps to reproduce or a minimal proof-of-concept
- Your assessment of severity (Critical / High / Medium / Low)
- Whether you believe the issue is currently being exploited

We will acknowledge receipt within **48 hours** and provide an initial assessment within **5 business days**.

---

## Disclosure Policy

g8e follows **coordinated disclosure**:

1. You report privately to us.
2. We confirm, assess, and develop a fix.
3. We release the fix and credit you (unless you prefer anonymity).
4. You may publish after the fix is released, or after **90 days** from initial report — whichever comes first.

We will not pursue legal action against researchers acting in good faith under this policy.

---

## Scope

### In Scope

- **g8e Gateway (PDP)** — `GovernanceEnvelope` parsing, deterministic transaction hash binding, L1-L4 verification logic (Doctrine, Consensus, Notary, Warden), WebAuthn/Passkey L3 brokerage
- **g8e Operator (PEP)** — execution boundary (L5 Actuator), signed `ActionReceipt` issuance, mTLS tunnel, `SQLAuditStore`, Sovereign Execution Boundary (scrubbing, rehydration)
- **g8e Protocol** — Protobuf schemas (`common.proto`, `operator.proto`), canonical JSON (protojson) serialization, envelope integrity
- **Authentication** — WebAuthn/FIDO2 L3 Notary flow, Ed25519 signature verification (L2 Consensus, L4 Warden, L5 Actuator), replay protection (Nonce)
- **CLI and bootstrap** — `g8e auth login`, mTLS credential and PKI handling

### Out of Scope

- Third-party model providers (Anthropic, OpenAI, etc.)
- Vulnerabilities in dependencies that have already been publicly disclosed and are pending upstream fix
- Social engineering or phishing attacks against Lateralus Labs employees
- Denial-of-service attacks without demonstrated security impact beyond availability

---

## Security Architecture Notes

The following are structural properties of g8e, provided to help researchers understand the intended security model:

- **Fail-closed by design.** Any verification failure at the L1-L4 layers (Doctrine, Consensus, Notary, Warden) drops the payload and writes an audit record. There is no fallback execution path.
- **Sovereign Execution Boundary.** The `g8e Operator` (PEP) acts as the sovereign boundary. It refuses to mutate host reality unless the transaction satisfies every L2 Consensus and L3 human-authorization proof required by the active posture.
- **No ambient execution authority.** No component holds standing permission to mutate state. Authority is granted strictly per-transaction via the `GovernanceEnvelope`, verified independently at the PEP.
- **Local audit sovereignty.** Raw forensic material is stored locally in the `SQLAuditStore`. The Sovereign Execution Boundary scrubs all outbound data before delivery to remote clients or AI systems.
- **Mandatory encryption at rest.** All storage services require an unlocked vault for initialization. Sensitive data (command stdout/stderr, file diffs, content) is encrypted at rest using AES-256-GCM with per-operation nonces. Encryption operations fail-closed if the vault is locked.
- **mTLS everywhere.** All platform communication (Operator-to-Gateway) requires mutual TLS. Unauthenticated or unverified connections are rejected.
- **State & Replay Protection.** Transactions are bound to a `state_merkle_root`, protected by a unique `nonce`, and carry a temporal `expires_at` deadline.

If your finding demonstrates a bypass of any of these properties, treat it as **Critical**.

---

## CVE and Dependency Scanning

g8e runs automated dependency scanning on every build. If you identify a dependency vulnerability not yet captured by our tooling, please report it via the channel above.

---

## Hall of Fame

We gratefully acknowledge security researchers who responsibly disclose vulnerabilities. With your permission, your name or handle will be listed here.

*No entries yet — be the first.*

---

## Contact

**Security:** security@lateraluslabs.com  
**General:** hello@lateraluslabs.com  
**Website:** https://lateraluslabs.com
