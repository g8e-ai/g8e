# Security Policy

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Email **security@g8e.ai** with:

- Description of the vulnerability
- Steps to reproduce
- Affected versions
- Impact assessment (if known)

We will acknowledge receipt within 48 hours and provide an initial assessment within 5 business days.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release (v0.2.x) | Yes |
| Previous minor | Security fixes only |
| Older | No |

## Scope

The following are in scope for security reports:

- **Dashboard (g8ed)** -- Web gateway, session management, and Passkey (PHP) orchestration.
- **Engine (g8ee)** -- Reasoning plane, the Tribunal consensus ensemble, and L2 signature generation.
- **Operator (g8eo)** -- Execution plane, L1 technical bedrock enforcement, and output scrubbing.
- **Governance layers** -- L1 (Hard Gates), L2 (Consensus), and L3 (Human-in-the-loop).
- **Universal Envelope** -- Cryptographic binding of identity, context, and governance metadata.
- **Inter-service communication** -- mTLS (TLS 1.3) and `X-Internal-Auth` token enforcement.
- **Local-First Audit Architecture (LFAA)** -- Encrypted SQLite audit vaults and the Git ledger.

The following are out of scope:

- Third-party LLM provider security (OpenAI, Anthropic, Azure, etc.).
- Vulnerabilities in upstream dependencies (report those to the upstream project).
- Social engineering attacks.
- Denial of service attacks against self-hosted instances.

## Disclosure Policy

- We follow coordinated disclosure. We ask that you give us 90 days to address the issue before public disclosure.
- We will credit reporters in release notes unless they prefer to remain anonymous.
- We do not currently offer a bug bounty program.

## Security Architecture

g8e implements a zero-trust, defense-in-depth model where the control plane is assumed to be potentially adversarial. Security is enforced through a **3-Layer Governance Bedrock**:

### 1. L1: Technical Bedrock (Hard Gates)
Hardcoded, non-negotiable safety invariants enforced at the Operator (`g8eo`) boundary.
- **Forbidden Patterns**: Global rejection of dangerous shell patterns (e.g., `sudo`, `su`, `pkexec`, privilege escalation).
- **Protobuf Reflection**: `g8eo` uses reflection over the `forbidden_patterns` custom option in Protobuf payloads to validate commands before dispatch.
- **Strict Filtering**: Optional allowlist/denylist for specific binary names and substrings.

### 2. L2: Consensus (The Tribunal)
A five-member agent ensemble (Axiom, Concord, Variance, Pragma, Nemesis) must reach consensus on command generation.
- **Tribunal Signing**: Commands are signed by the Engine with the `auditor_hmac_key` before reaching the Operator.
- **Warden Analysis**: Local pre-execution risk analysis on the host, featuring a **Two-Strike Circuit Breaker** that blocks high-risk operations and triggers agent conflict detection.

### 3. L3: Authorization (Proof of Human Presence)
State-changing operations require a **Proof of Human Presence (PHP)**.
- **Passkeys**: Commands must be signed by a hardware-bound Passkey on the user's workstation.
- **Auto-Approval**: Benign diagnostic commands can be auto-approved, but only *after* passing all L1 and L2 gates. L3 auto-approval NEVER bypasses hard gates.

### Additional Protections
- **mTLS Everywhere**: All component communication is secured via TLS 1.3 with mutual authentication and a private internal CA.
- **Output Scrubbing (Sentinel)**: `g8eo` scrubs terminal output for credentials, PII, and secrets before it leaves the host.
- **Data Sovereignty**: Audit logs are stored in encrypted SQLite vaults on the Operator host. A hidden Git ledger provides a diffable history of all AI-driven file mutations.
- **Secret Containment**: Authoritative secrets (HMAC keys, SSL certs) are stored in a repo-local host-owned directory (`./.g8e/ssl`) and are never bind-mounted into untrusted containers.

For full details, see [docs/architecture/security.md](docs/architecture/security.md).
