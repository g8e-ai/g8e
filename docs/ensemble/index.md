# g8ee

**g8e-Compliant Agentic Ensemble** — Reference AI reasoning system for g8e infrastructure operations.

g8ee is an optional agentic application that performs triage, model reasoning, tool loops, command generation, memory management, and event publication. It dispatches host operations through Gateway HTTP (`POST /api/v1/operators/commands` with a registered request `event_type`) and submits canonical governance envelopes for designated application-record writes; the Gateway and executing Operator perform the platform's five-layer verification pipeline (L1 Doctrine, L2 Consensus, L3 Notary, L4 Warden, L5 Actuator). g8ee does not use Gateway pub/sub.

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
| [Evals](evals.md) | How g8ee uses g8e evals (see [platform Evaluations](../architecture/evals.md) and [Model Provenance](../architecture/model-provenance.md)) |

## Related Platform Documentation

- [Platform Overview](../architecture/overview.md) — Three-component g8e platform architecture and service topology
- [Evaluations](../architecture/evals.md) — Platform evaluation programs, Observer and Provenance Operator roles, evidence, and verification
- [Model Provenance](../architecture/model-provenance.md) — Storage-side weight attestation and chain of custody
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
- [Documentation Guide](../devs/docs.md) — Repository-wide audit, ownership, generation, cross-linking, and versioning rules
