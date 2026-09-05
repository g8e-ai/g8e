# g8ee

**g8e-Compliant Agentic Ensemble** — Reference AI reasoning system for g8e infrastructure operations.

g8ee is an agentic ensemble that acts as an L2 producer, emitting typed, signed GovernanceEnvelope transactions to the g8e Gateway for validation and execution through the five-layer verification pipeline (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator). It integrates with g8e operator services for secure, governed infrastructure management.

## Documentation

| Document | Description |
| --- | --- |
| [Getting Started](getting-started.md) | Prerequisites, installation, and quick start |
| [Architecture](architecture.md) | System architecture and component overview |
| [Governance](governance.md) | Five-layer verification pipeline and governance postures |
| [Agents](agents.md) | Agent hierarchy, personas, and ensemble structure |
| [Protocol](protocol.md) | g8e protocol reference for Gateway integration |
| [Prompts](prompts.md) | Prompt architecture and templating |
| [Thinking](thinking.md) | L2 consensus, provider reasoning, and thought signatures |
| [PKI & Trust](pki.md) | Public Key Infrastructure and trust management |
| [Storage](storage.md) | Storage tiers and data sovereignty |
| [LLM Providers](llm-providers.md) | LLM provider implementations and configuration |
| [Server-Sent Events (SSE)](sse.md) | SSE streaming pipeline and real-time event delivery |
| [Development](devs.md) | Dev setup, guidelines, and coding standards |
| [Testing](tests.md) | Testing framework and practices |
| [Evals](evals.md) | Evaluation suite and benchmarks |
| [Constants](constants.md) | Constants and configuration reference |

## Related Platform Documentation

- [Platform Overview](../architecture/overview.md) — Three-component g8e platform architecture and service topology
- [Ensemble Architecture](../architecture/ensemble.md) — Platform-level summary of g8ee's role in the g8e platform
- [Governance Gateway](../architecture/gateway.md) — Gateway architecture, protocol surfaces, and PKI authority
- [Governed Operator](../architecture/operator.md) — Operator architecture, L4 Warden, and L5 Actuator execution boundary
- [Governance Pipeline](../architecture/governance.md) — Five-layer verification pipeline and governance postures
- [Protocol Reference](../architecture/protocol.md) — Canonical wire contracts, GovernanceEnvelope schema, and SPIFFE identifiers
- [Authentication & Authorization](../architecture/auth.md) — mTLS, WebAuthn, SPIFFE workload identity, and trust bundles
- [SSE Streaming](../architecture/sse.md) — Gateway-side SSE push ingestion, filtering, and consumer endpoints
- [Getting Started Guide](../guides/getting_started.md) — Platform installation, quick start, and unified stack deployment
- [Unified Docker Stack](../guides/unified_stack.md) — Docker Compose deployment for Gateway, Operator, Ensemble, and Dashboard
- [Dashboard (g8ed)](../dashboard/index.md) — First-party browser interface that consumes ensemble SSE events
