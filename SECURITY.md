# Security Policy

**Project:** g8e — Byzantine Fault Tolerant Governance Protocol  
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
- Affected component(s): Protocol Gateway, Operator (`g8eo`), Engine (`g8ee`), Wire Contract
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

- **Protocol Gateway** — GovernanceEnvelope parsing, transaction hash binding, L1/L2/L3 verification logic
- **Operator (`g8eo`)** — execution boundary, Warden dispatcher, mTLS tunnel, LFAA audit vault, Sentinel scrubber
- **Engine (`g8ee`)** — Tribunal consensus, agent hierarchy, agentic ReAct loop
- **Wire Contract** — Protobuf schemas, canonical JSON serialization, envelope integrity
- **Authentication** — WebAuthn/FIDO2 L3 flow, Ed25519 signature verification, replay protection
- **CLI and bootstrap** — `g8e login`, device-link token generation, mTLS credential handling

### Out of Scope

- Third-party model providers (Anthropic, OpenAI, etc.)
- Vulnerabilities in dependencies that have already been publicly disclosed and are pending upstream fix
- Social engineering or phishing attacks against Lateralus Labs employees
- Denial-of-service attacks without demonstrated security impact beyond availability

---

## Security Architecture Notes

The following are structural properties of g8e, provided to help researchers understand the intended security model:

- **Fail-closed by design.** Any verification failure at the Operator boundary drops the payload and writes an audit record. There is no fallback execution path.
- **Outbound-only Operator.** `g8eo` establishes connections outbound via reverse tunnel. It does not listen for inbound connections. There are no open ports on the managed host.
- **No ambient execution authority.** No component holds standing permission to mutate state. Authority is granted per-transaction via the GovernanceEnvelope, verified independently at the Operator.
- **Local audit sovereignty.** Raw forensic material never leaves the managed host. Sentinel scrubs all outbound data before delivery to AI systems or remote clients.
- **mTLS everywhere.** All Operator-to-Gateway communication requires mutual TLS. Unauthenticated connections are rejected.

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
