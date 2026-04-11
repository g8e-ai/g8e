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
| Latest release | Yes |
| Previous minor | Security fixes only |
| Older | No |

## Scope

The following are in scope for security reports:

- g8e platform components (Engine, Dashboard, Store)
- Operator binary (System Operator, Cloud Operator)
- Sentinel pre-execution and post-execution security layers
- Authentication and session management
- Inter-service communication (mTLS, tokens)
- Local-First Audit Architecture (LFAA) encryption

The following are out of scope:

- Third-party LLM provider security (OpenAI, Azure, etc.)
- Vulnerabilities in upstream dependencies (report those to the upstream project)
- Social engineering attacks
- Denial of service attacks against self-hosted instances

## Disclosure Policy

- We follow coordinated disclosure. We ask that you give us 90 days to address the issue before public disclosure.
- We will credit reporters in release notes unless they prefer to remain anonymous.
- We do not currently offer a bug bounty program.

## Security Architecture

g8e implements defense-in-depth:

- **Sentinel pre-execution** -- 58 MITRE ATT&CK-mapped threat detectors block dangerous commands before execution
- **Sentinel post-execution** -- 27 scrubbing patterns remove credentials, PII, and secrets before data leaves the Operator
- **Forbidden operations** -- `sudo`, `su`, `pkexec`, and privilege escalation are unconditionally blocked
- **mTLS** -- three-layer Operator authentication: API key + server certificate pinning + mTLS client certs
- **CA cert containment** -- the platform CA never leaves the `vsodb-data` volume; `./g8e security certs trust` streams it directly from the `g8ep` container via `docker exec` with no intermediate file written to the host; remote workstation instructions use SSH streaming (`ssh <host> "docker exec g8ep cat /vsodb/ssl/ca.crt"`) for the same reason
- **Encrypted sessions** -- AES-256-GCM on sensitive session fields
- **Local-First Audit** -- raw command output is stored only on the Operator in encrypted SQLite vaults

For full details, see [docs/architecture/security.md](docs/architecture/security.md).
