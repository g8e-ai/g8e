# Contributing to g8e

g8e is a zero-trust execution platform for agentic infrastructure. We welcome contributions that strengthen the platform's security invariants, improve protocol compliance, and expand the ecosystem of BYO clients and agents.

## Architectural Foundation

Before contributing, ensure you understand the core platform architecture:

- **g8e Gateway (g8eg)**: The central Policy Decision Point (PDP) for identity, policy admission, and posture-aware governance.
- **g8e Operator (g8eo)**: The host-side Policy Execution Point (PEP) and MCP server.
- **g8e Protocol**: The canonical protocol definitions (protobuf schemas and constant registries).
- **5-Layer Governance Gauntlet**:
    - **L1 Doctrine (L1Doctrine)**: Technical hard gates (forbidden patterns, threat detection).
    - **L2 Consensus (L2Consensus)**: Multi-agent consensus via Ed25519 signatures.
    - **L3 Notary (L3Notary)**: Human-in-the-loop authorization through WebAuthn or signed CLI proofs.
    - **L4 Warden (L4Warden)**: Pre-dispatch verification gate (transaction hash, expiry, nonce, state root).
    - **L5 Actuator (L5Actuator)**: Execution boundary issuing signed `ActionReceipts`.

## Finding Work

New here? Start with [Early testers](../EARLY_TESTERS.md) — small cohorts, async only, batch replies, no SLA. The most useful first contributions don't require understanding the full platform:

**For anyone (no build required)**
1. **First Run Report on your OS.** Take any README door (explorer, protocol, binary, full stack) and file the First Run Report issue form with where you got stuck. Windows, macOS arm64, and Linux arm64 reports are especially valuable.
2. **Explorer feedback.** Browse [OpenDevOps.ai](https://opendevops.ai) for 10 minutes and file what was clear, what was cryptic, and what you wanted to see but couldn't find.
3. **README cold read.** Read the README as a stranger and report the first sentence that made you want to stop. One sentence is a complete contribution.

**For Python folks (protocol only, ~30 min)**
4. **Run the protocol examples.** `pip install g8e==2.3.2`, run `protocol/python/examples/`, and report Python version, OS, and anything surprising.
5. **Conformance test report.** Run `protocol/conformance/` tests and file the result. A green run on a new environment is useful signal.

**For careful readers (~1 hr)**
6. **Guide vs binary audit — one guide.** Pick one file in `docs/guides/`, follow it literally, and file every place the text and actual command output disagree.
7. **Threat-model question.** Read `docs/architecture/overview.md` and `governance.md`, then file one question about the trust boundary the docs don't answer. A good unanswered question becomes a docs fix.

Look for issues labeled `first-run`, `good-first-task`, and `docs`. Experienced contributors can also search the Go source for architectural work, but newcomers should start above — the starter tasks are the curated on-ramp.

## Local Development Setup

Choose the smallest environment that fits your work:

- **Gateway only:** clone the repository, then run `make up`. A native build needs Go 1.26.6 and Make. Fresh clones use the committed frontend embeds; Node.js 22+ and npm are needed only to rebuild the Console or Evaluation Explorer. Python, Ollama, and model SDKs are not needed for this track.
- **Full platform:** run `make ensemble-env` once, then `make full`. This
  provisions Python 3.12+ through `uv` and installs the Ensemble runtime
  dependencies. Use `make dev-python` when you also need Ensemble test and lint
  dependencies.

`make full` also needs an approved `G8E_OLLAMA_ENDPOINT` in `.env` or the process environment. For interactive configuration, use `make full-setup`. Linux and macOS contributors can install the complete development toolchain with their platform's setup script; use `--build-only` for Gateway evaluation. See the [setup instructions](../docs/guides/getting_started.md#clone-the-repository).

For browser enrollment, run `./g8e auth enroll user -e localhost`; for a
headless environment, add `--headless`. The [Getting Started guide](../docs/guides/getting_started.md)
covers setup, enrollment, and runtime configuration.

## Selecting Checks

Run the narrowest check that covers your changes, then the broader owning
suite when practical:

| Change | Command |
|---|---|
| Go unit tests | `./g8e test unit` |
| Ensemble tests | `make ensemble-test` |
| Setup and CI helper scripts | `make ci-scripts` |

`make ensemble-test` needs `make dev-python` first. `make ci-scripts` runs the complete `make dev-check` preflight; for isolated script checks with the Python dependencies installed, use `.venv/bin/python -m unittest discover -s scripts/tests`.

Platform test suites go through `./g8e test ...` or their owning Make target,
as required by [INV-TEST-01](../docs/devs/devs.md#testing-inv-test).

## Pull Requests

- Read the [Developer Guidelines](../docs/devs/devs.md) and follow the
  relevant invariants for the code and tests you change.
- Keep each pull request focused; update current-state documentation with
  behavior changes and generated artifacts through their owning generators.
- Describe the user-visible behavior, implementation, and checks run. Include
  exact commands and note any checks that could not run.
- Confirm the CI workflow passes and respond to review feedback with follow-up
  commits on the same pull request.

## Filing Issues

First time running g8e and got stuck installing or enrolling? Use the **First Run Report** form in the [issue chooser](https://github.com/g8e-ai/g8e/issues/new/choose) — it asks for the door you tried, your OS, `./g8e version`, and the exact step.

For a bug in a running deployment, [file a bug report](https://github.com/g8e-ai/g8e/issues/new) and include:

1. **Version**: Output of `./g8e version`.
2. **Environment**: OS and processor architecture.
3. **Traceability**: If applicable, include the `transaction_hash` or relevant entries from the `SQLAuditStore`.
4. **Reproduction**: A minimal set of steps to reproduce the behavior.
5. **Expected vs Actual**: Clear description of what you expected to see and what happened instead.

Security vulnerabilities should be reported through the [Security Policy](SECURITY.md), not through a public issue.

## Documentation Contributions

Documentation is treated as code. Follow the [Documentation Guide](../docs/devs/docs.md), which defines the complete audit workflow, source-of-truth matrix, generated-document ownership, first-party documentation catalog, cross-linking rules, version policy, and validation matrix.

1. **Audit end to end**: Read the complete affected document and verify every behavioral claim, example, link, and limitation.
2. **Trace ownership**: Locate the owning implementation, schema, registry, configuration, test, generator, or scope-bound evidence rather than relying on existing prose.
3. **Update related documents**: Keep one canonical explanation and cross-link every affected current-state summary or component index.
4. **Generate and validate**: Update templates, annotations, schemas, or reviewed evidence inputs before generated outputs, then run the owning checks.
5. **Version last**: Update document date and version metadata only after the complete audit and related-document reconciliation.

## Coding Standards

Please read the [Developer Guidelines](../docs/devs/devs.md) before submitting patches. Key directives include:

- **Rip and Replace**: Delete/replace broken paths. **No backwards compatibility** for technical debt.
- **Fail-Closed**: If a validation or security check fails, the system must halt.
- **Explicit over Implicit**: No magic, no hidden side effects, and no "guessing".
- **Zero Tech Debt**: PRs must leave the codebase cleaner than found.

## Licensing

Unless otherwise noted, the g8e source files are distributed under the Business Source License 1.1 (BSL 1.1) found in the LICENSE file. The license converts to Apache 2.0 on the Change Date (2030-08-18). By contributing, you grant Lateralus Labs, LLC a license to use your work under these terms.
