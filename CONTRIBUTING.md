# Contributing to g8e

g8e is a zero-trust execution platform for agentic infrastructure. We welcome contributions that strengthen the platform's security invariants, improve protocol compliance, and expand the ecosystem of BYO clients and agents.

## Architectural Foundation

Before contributing, ensure you understand the core platform architecture:

- **Governance Gateway (g8eg)**: The central, BFT-governed Policy Decision Point (PDP).
- **g8e Operator (g8eo)**: The host-side Policy Execution Point (PEP) and MCP server.
- **g8e Protocol**: The canonical protocol definitions (protobuf schemas and constant registries).
- **3-Layer Governance Bedrock**:
    - **L1 Doctrine (L1Doctrine)**: Technical hard gates (forbidden patterns, threat detection).
    - **L2 Consensus (L2Consensus)**: Multi-agent consensus via Ed25519 signatures.
    - **L3 Notary (L3Notary)**: Human-in-the-loop authorization (WebAuthn/mTLS).
    - **L4 Warden (L4Warden)**: Pre-dispatch verification gate (transaction hash, expiry, nonce, state root).
    - **L5 Actuator (L5Actuator)**: Execution boundary issuing signed `ActionReceipts`.

## Finding Work

We maintain a list of "good first issues" directly in the codebase using `TODO` comments. You can find these by running:

```bash
make first-issues
```

This will show you a list of pending tasks, improvements, and architectural migrations.

## Filing Issues

When [filing an issue](https://github.com/g8e-ai/g8e/issues/new), please include:

1. **Version**: Output of `./g8e version`.
2. **Environment**: OS and processor architecture.
3. **Traceability**: If applicable, include the `transaction_hash` or relevant entries from the `AuditVaultService`.
4. **Reproduction**: A minimal set of steps to reproduce the behavior.
5. **Expected vs Actual**: Clear description of what you expected to see and what happened instead.

Security vulnerabilities should be reported directly to security@g8e.ai.

## Documentation Contributions

Documentation is treated as code. If you are updating documentation, follow the **`updatedocs`** workflow:

1. **Source of Truth**: Locate canonical implementation in `protocol/proto/`, `protocol/constants/*.json`, or `internal/services/`.
2. **Trace Code Path**: Verify that the documented behavior matches the actual execution path through L1-L5 layers.
3. **Terminology**: Use exact Go symbols and proto definitions (e.g., `GovernanceEnvelope`, `ActionReceipt`).
4. **No Redundancy**: Each fact lives in exactly one place; cross-link rather than repeat.

## Coding Standards

Please read the [Developer Guidelines](docs/devs/devs.md) before submitting patches. Key directives include:

- **Rip and Replace**: Delete/replace broken paths. **No backwards compatibility** for technical debt.
- **Fail-Closed**: If a validation or security check fails, the system must halt.
- **Explicit over Implicit**: No magic, no hidden side effects, and no "guessing".
- **Zero Tech Debt**: PRs must leave the codebase cleaner than found.

## Licensing

Unless otherwise noted, the g8e source files are distributed under the Apache 2.0 license found in the LICENSE file. By contributing, you grant Lateralus Labs, LLC a license to use your work under these terms.
