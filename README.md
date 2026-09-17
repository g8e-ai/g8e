# g8e

**Govern AI data. Verify AI execution. Keep custody at the edge.**

[![License](https://img.shields.io/badge/license-BSL%201.1-blue.svg)](LICENSE) [![CI](https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml/badge.svg)](https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml) [![FIPS 140-3](https://img.shields.io/badge/FIPS%20140--3-Go%20Cryptographic%20Module-006400.svg)](docs/reference/fips140-3.md) [![MCP](https://img.shields.io/badge/MCP-governed-5D3FD3.svg)](protocol/docs/mcp.md)

g8e is an AI data and execution governance suite. It places a fail-closed control plane between AI systems and the infrastructure they use, so models can reason over governed context without receiving direct authority over hosts, raw data, or long-lived credentials.

Every governed mutation is a typed, signed, state-bound `GovernanceEnvelope`. The Gateway admits it, the Operator independently verifies it, the Actuator executes it at the data owner’s boundary, and the platform records signed evidence of the result.

[Get started](docs/guides/getting_started.md) · [Run the full suite](docs/guides/unified_stack.md) · [Architecture](docs/architecture/overview.md) · [Proof-backed compliance](docs/reference/compliance-evidence.md) · [Native evaluations](docs/ensemble/evals.md) · [Protocol](protocol/docs/spec.md)

## Proof, not promises

g8e separates measured evidence from architecture claims. Published results remain scoped to the exact artifacts, versions, environments, and trust inputs that produced them.

| Proof | Published result | Boundary |
| --- | --- | --- |
| [Native core execution-boundary evaluation](docs/ensemble/evals.md) | The Go-native suite passed 10/10 required invariants against the unified Docker stack, and an independent `g8e eval verify` invocation returned valid with zero failures. | One doctrine-posture deployment, one exact remote Operator session, one controlled target, and one networkless observer. |
| [Clean offline compliance verification](docs/release_notes/v2.1.x/v2.1.7-offline-acceptance.md) | A fresh network-disabled, read-only container reproduced a signed compliance bundle and passed all 10 verification checks. Four protected-source, renderer, and signature mutations failed closed. | One v2.1.7 candidate and one point-in-time assessment scope. This is not certification or recurring operating effectiveness. |
| [Compliance evidence model](docs/reference/compliance-evidence.md) | Typed assertions, evidence-grade scenarios, explicit control classifications, and fail-closed verification. | Catalog and verifier coverage do not imply customer compliance, authorization, or external attestation. |
| [Live CI](https://github.com/g8e-ai/g8e/actions/workflows/build-and-test.yml) | Build and test status is published as a live external signal. | Live CI is not frozen release evidence. |

The repository does not publish a zero-leakage, certification, broad model-quality, or production-suitability claim without evidence that supports that exact statement.

## Run the suite

The unified stack starts the four components, enrolls the first owner, and walks through workload approval.

```bash
make build
./g8e docker build
./g8e docker start --full
```

The default deployment exposes the Gateway API and console, g8ee API, and g8ed dashboard. Workloads remain unready until the owner approves their exact enrollment requests. See the [Unified Docker Stack guide](docs/guides/unified_stack.md) for prerequisites, ports, topology, and enrollment.

## Four components, one governance boundary

| Component | Responsibility | Trust position |
| --- | --- | --- |
| **g8eg, Governance Gateway** | Policy Decision Point for identity, PKI, policy admission, L1 Doctrine, L2 Consensus, L3 Notary, routing, and platform APIs. | Core. Coordinates governance but cannot bypass Operator verification. |
| **g8eo, Governed Operator** | Policy Execution Point on each managed host. Pulls work over outbound mTLS, runs L4 Warden and L5 Actuator, and stores local receipts and state evidence. | Core. Sole authority for mutation at the data owner’s boundary. |
| **g8ee, Agentic Ensemble** | First-party multi-agent reasoning application. Converts requests into governed intent and publishes typed progress and results. | Untrusted application. Proposes actions but never authorizes or executes them directly. |
| **g8ed, Dashboard** | First-party browser interface for passkey authentication and live platform events. | Untrusted interface. Observes and requests through Gateway surfaces without execution authority. |

g8eg and g8eo share one statically linked Go binary running in separate modes. g8ee is a Python service, and g8ed is a JavaScript application served by Node.js.

## How it works

1. A user, AI client, application, or g8ee submits intent through an authenticated Gateway surface.
2. g8eg binds identity, state, replay controls, and the active governance posture to the transaction.
3. L1 Doctrine always applies. L2 Consensus and L3 Notary become required gates when the selected posture requires them.
4. The bound g8eo session pulls the envelope over outbound mTLS and independently verifies its hash, signatures, nonce, expiry, state root, and required proofs.
5. L5 executes through the single Actuator boundary and persists signed execution evidence.

Read the [governance architecture](docs/architecture/governance.md) for posture semantics and the [protocol specification](protocol/docs/spec.md) for the canonical wire contract.

## Native evaluations

The native evaluator proves the remote execution boundary against the real unified stack. It does not invoke Python, g8ee, a model provider, synthetic simulators, or a compatibility reader.

```bash
./g8e eval run core-execution-boundary
./g8e eval verify <run-id>
./g8e eval show <run-id>
```

The run command selects one exact active remote Operator, submits an allowed typed mutation through the authenticated Gateway ingress, proves exactly one effect through a separate networkless Compose observer, submits the doctrine-prohibited equivalent through the same ingress, and proves rejection without another effect. It persists canonical `report.json`, `verification.json`, and digest-named evidence under `.g8e/data/eval/runs/<run-id>/`.

See [Native Evaluations](docs/ensemble/evals.md) for acceptance invariants, trust boundaries, and JSON output.

## What the boundary enforces

- **Data custody stays local.** Raw data, vault keys, execution state, and authoritative audit evidence remain with the Operator runtime.
- **Execution is host-authorized.** The Gateway admits work; the Operator independently verifies it before any side effect.
- **Operators are outbound-only.** g8eo opens an mTLS connection to g8eg and listens on no inbound management port.
- **Mutations are state-bound and replay-resistant.** Envelopes bind typed intent to identity, target, state root, nonce, expiry, and policy evidence.
- **Human approval is transaction-bound.** Required L3 approvals use WebAuthn or signed CLI proofs over the transaction hash.
- **Receipts precede and follow execution.** The Actuator signs and persists execution evidence before dispatch and records the final outcome with durable-persistence evidence.

The boundary governs operations that traverse g8e. It does not sandbox an AI client or govern native tools and side channels that remain enabled outside the platform. See [AI agents and the governance boundary](docs/architecture/agents.md).

## Documentation

| Need | Start here |
| --- | --- |
| Install and operate g8e | [Getting Started](docs/guides/getting_started.md), [Unified Stack](docs/guides/unified_stack.md), and [Operator Connection](docs/guides/connect_operator_to_gateway.md) |
| Understand trust and execution | [Architecture Overview](docs/architecture/overview.md), [Governance](docs/architecture/governance.md), [AI Agent Boundary](docs/architecture/agents.md), [Authentication](docs/architecture/auth.md), and [Network](docs/architecture/network.md) |
| Build an integration | [Build Apps](docs/guides/build_apps.md), [Connect Apps](docs/guides/connect_apps_to_gateway.md), [MCP](protocol/docs/mcp.md), and [A2A](protocol/docs/a2a.md) |
| Evaluate execution | [Native Evaluations](docs/ensemble/evals.md) |
| Evaluate evidence and claims | [Compliance Evidence](docs/reference/compliance-evidence.md), [Compliance Alignment](docs/reference/compliance-alignment.md), and [Sovereignty Gauntlet](docs/guides/sovereignty_gauntlet.md) |
| Develop and release | [Developer Guidelines](docs/devs/devs.md), [Code Map](docs/devs/codemap.md), [Testing](docs/devs/tests.md), and [Release Process](docs/devs/release_process.md) |

## Build and contribute

```bash
make build
./g8e test unit
./g8e test integration
./g8e test lint
make ensemble-test
make dashboard-test
```

See [CONTRIBUTING.md](.github/CONTRIBUTING.md), the [developer guide](docs/devs/devs.md), and the [documentation guide](docs/devs/docs.md).

## Support OpenDevOps.ai

[OpenDevOps.ai](https://opendevops.ai) is a fully independent, verifiable LLM benchmarking project. I am completely self-funded and refuse to take venture capital. If this data helps you, please [sponsor the work on GitHub](https://github.com/sponsors/Badoot) to help keep the servers running and the pipeline unbiased.

## Pilots and partnerships

Lateralus Labs works with teams evaluating governed AI execution, sovereign data workflows, and proof-backed compliance reporting. Contact [danny@lateraluslabs.com](mailto:danny@lateraluslabs.com), [schedule a call](https://calendly.com/danny-lateraluslabs/quick_discovery), or connect on [LinkedIn](https://www.linkedin.com/in/dannybarbour/).

---

Business Source License 1.1. Converts to Apache 2.0 on 2030-08-18. Built by Lateralus Labs.
