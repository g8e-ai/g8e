# Architecture

## Overview

g8ee is the agentic ensemble component of the 3-component g8e platform:

- **Governance Gateway (g8eg)** — Central Policy Decision Point (PDP). Owns platform-level PKI, coordination, Pub/Sub, and transaction validation/suspension. Operates under doctrine, consensus, or notary posture.
- **Governed Operator (g8eo)** — Host-side Policy Execution Point (PEP). Enforces protocol compliance, verifies L1/L2/L3 signatures, executes transactions via the Actuator stage, and maintains local-first audit ledgers. Runs on target hosts.
- **g8e Agentic Ensemble (g8ee)** — First-party g8e-compliant agentic ensemble. Acts as an L2 producer, emitting typed, signed GovernanceEnvelope transactions to the Gateway for validation and execution through the five-layer verification pipeline.

## Protocol Surfaces

The Gateway exposes two multiplexed network surfaces:

- **HTTP (Port 8080)** — Plain HTTP discovery surface used for trust bundle downloads, device-link enrollment, CSR signing, and owner-approved platform enrollment requests and polling.
- **HTTPS (Port 8443)** — Authenticated mTLS and public surface multiplexed for GovernanceEnvelope submissions, MCP and A2A APIs, document store operations, WebSocket pub/sub channels, SSE event push and streaming, browser login, WebAuthn challenge/assertion, and out-of-band approval UI.

## Code Structure

The ensemble lives in-tree under `ensemble/` at the repository root:

```
ensemble/
├── app/                 # Main application code
│   ├── clients/         # External service clients (governance, DB, pubsub, blob, KV cache, HTTP)
│   ├── constants/       # Application constants (sourced from g8e.constants and local definitions)
│   ├── db/              # Database models, KV service, DB service, and blob storage service
│   ├── llm/             # LLM provider implementations (Anthropic, Gemini, OpenAI, Ollama, llama.cpp, fake)
│   ├── middleware/      # HTTP context extraction, exception handlers, and security middleware
│   ├── models/          # Pydantic models (subclassing g8e protocol base models and personas)
│   ├── prompts_data/    # Raw prompt templates, agent instructions, system constraints, and tribunal guidelines
│   ├── proto/           # Generated Python protobuf stubs (gitignored, regenerated via make proto)
│   ├── routers/         # FastAPI route handlers (chat, internal, operators, cases, settings, SSE)
│   ├── security/        # Cryptographic verification, cert validation, and security utilities
│   ├── services/        # Business logic services (AI tribunal, auth, data, infra, investigation, operator)
│   ├── storage/         # Local storage and state abstraction layers
│   └── utils/           # Utility functions (merkle, hashing, validation, formatting, ids, timestamp)
├── tests/               # Unit, integration, and E2E test suites
├── evals/               # Evaluation benchmark suite and scoring rubrics
├── config/              # Configuration files (whitelists, blacklists, auto-approved commands)
├── pyproject.toml       # Python project configuration, dependencies, and entrypoints
├── Makefile             # Build automation (make proto, ensemble-test, ensemble-lint)
├── Dockerfile           # Multi-stage container build definition
├── requirements.txt     # Pinned Python package dependencies
└── mkdocs.yml           # Documentation configuration pointing to docs/ensemble/
```

## Model Hierarchy

g8ee models inherit from the `g8e` protocol package (`g8e>=1.7.8`, resolved in-tree to `protocol/python/`):

- **`G8eBaseModel`** — Base model from `g8e.models.base`, re-exported via `app.models.base`. Extends Pydantic's `BaseModel` with protojson-compatible serialization and UTC normalization.
- **`G8eTimestampedModel`, `G8eIdentifiableModel`, `G8eAuditableModel`** — Base lifecycle models from `app.models.base` providing standardized UTC timestamps (`created_at`, `updated_at`), UUID4 document identifiers, and actor audit tracking (`created_by`, `updated_by`).
- **`RequestContext`** — Subclasses `g8e.models.context.RequestContext` in `app.models.http_context`, adding `operator_id` and `operator_session_id` for governance envelope routing while defaulting `source_component` to `g8ee`.
- **`BoundOperator`** — Re-exported directly from `g8e.models.context`.
- **`ChatMessageRequest`** — Multiple inheritance in `app.models.internal_api` extending `g8e.models.internal_api.ChatMessageRequest` and `RequestOverrides` mixin (LLM provider and web search overrides) with typed `AttachmentMetadata` lists.
- **`ResourceCreationRequest` and `ChatStartedResponse`** — Directly re-exported from `g8e.models.internal_api`.
- **Settings Models** — Subclasses of protocol definitions from `g8e.models.settings` in `app.models.settings` (`CommandValidationSettings`, `SearchSettings`, `EvalJudgeSettings`, `LLMSettings`, `G8eeUserSettings`).
- **SSE Wire Models** — `SessionEventWire` and `BackgroundEventWire` subclass `g8e.models.events`; all 11 SSE payload classes (`AiProcessingStoppedPayload`, `AIToolLifecyclePayload`, `ChatCitationsReadyPayload`, `ChatErrorPayload`, `ChatProcessingStartedPayload`, `ChatResponseChunkPayload`, `ChatResponseCompletePayload`, `ChatRetryPayload`, `ChatThinkingPayload`, `ChatTurnCompletePayload`, `TriageClarificationQuestionsPayload`) are re-exported from `g8e.models.events`.
- **Constants** — Sourced from `g8e.constants` accessors (`collection()`, `document_id()`, `kv_key()`, `channel()`, `intent()`, `prompt()`), `GatewayAPIPaths` wrapping `g8e.constants.API_PATHS`, and enum re-exports from `g8e.enums`.
- **Protobuf Stubs** — Generated from `protocol/proto/g8e/` via `make proto` into `app/proto/` (`common_pb2.py`, `operator_pb2.py`, `pubsub_pb2.py`, gitignored).

## Governance Architecture

All business-critical mutations (case records, investigations, memories, reputation commitments, operator commands, file edits) route through `GovernanceClient` (`app/clients/governance_client.py`), which constructs and submits g8e-compliant `GovernanceEnvelope` transactions:

1. **State Merkle Root Acquisition** — Fetches the current state Merkle root from the Gateway health endpoint (`GET /healthz`). If a concurrent transaction mutates state before submission, producing a `TX_STATE_MISMATCH` rejection (HTTP 403), `GovernanceClient` automatically re-fetches the updated state root and retries envelope submission up to three times.
2. **Canonical Envelope Construction** — Builds a `GovernanceEnvelope` containing the transaction hash (deterministic SHA256 of canonical fields), a cryptographically secure random 32-byte hexadecimal nonce, expiration timestamp, case/investigation/task context, base64-encoded protobuf payload, and typed governance metadata (L2 Tribunal consensus and L3 notary proof).
3. **Payload and Component Mapping** — Maps internal action and payload types to canonical protocol identifiers (`CommandRequested`, `FileEditRequested`, `FsListRequested`, `FsReadRequested`, `FsGrepRequested`, `DocumentUpdateRequested`, `DocumentDeleteRequested`) and maps internal component names to proto enum values (`COMPONENT_AGENT`, `COMPONENT_CLIENT`, `COMPONENT_G8EO`).
4. **L3 Transport Proof Binding** — Extracts the SHA256 certificate fingerprint of the client mTLS certificate to satisfy L3 notary verification on the Gateway.
5. **Gateway Verification and Receipt Ingestion** — Posts the canonical JSON envelope to `POST /api/v1/governance/envelopes` (`GatewayAPIPaths.GOVERNANCE_ENVELOPES`). The Gateway verifies L1 doctrine, L2 consensus, and L3 notary authorization before dispatching to the operator. On success, the Gateway returns a signed `ActionReceipt` proving execution status.

## Connection Model and Workload Identity

g8ee authenticates to the Gateway using mTLS:

- **App Workload Identity** — In the unified deployment stack, g8ee enrolls at startup via the owner-approved platform enrollment protocol (`app/services/infra/app_enrollment_service.py`). It submits an enrollment request with a P-256 CSR to the Gateway's plain-HTTP discovery surface (port 8080), awaits owner approval, signs the completion transcript, and receives a dedicated app certificate (`spiffe://g8e.local/app/g8ee`) and trust bundle. This identity authenticates standard Gateway traffic (health checks, SSE event push, settings).
- **Governance Transport Credentials** — Because the Gateway's privileged route registry restricts `POST /api/v1/governance/envelopes` to authorized operator sessions, g8ee mounts the operator's enrolled mTLS credentials read-only (`G8E_GOVERNANCE_OPERATOR_CERT` and `G8E_GOVERNANCE_OPERATOR_KEY`). `GovernanceClient` lazily extracts the operator SPIFFE URI (`spiffe://g8e.local/operator/<org_id>/<operator_id>/<operator_session_id>`) from the certificate to bind the envelope's `operator_id` and `operator_session_id`, satisfying the Gateway's `verifyEnvelopeIdentityBinding` check.
- **Gateway-Authoritative Request Context** — g8ee receives typed `operator_session_id`, `cli_session_id`, and `user_id` values, then its internal Gateway client posts that exact tuple to `POST /api/v1/operators/validate` over the app mTLS channel. Only a `valid` response with matching `operator_id` and `user_id` becomes an authenticated request context. Local `operator_sessions` records support application state but do not authenticate callers; stale, inactive, mismatched, or duplicate active Gateway bindings fail closed.

## Related

- [Governance](governance.md) — Five-layer verification pipeline and envelope validation
- [Agents](agents.md) — Agent hierarchy, personas, and Tribunal consensus
- [Protocol](protocol.md) — Protocol reference for Gateway integration
- [Constants](constants.md) — Sourced protocol constants and application definitions
- [Prompts](prompts.md) — Prompt architecture and templating
- [Thinking](thinking.md) — L2 consensus, provider reasoning, and thought signatures
- [PKI & Trust](pki.md) — Public Key Infrastructure, trust bundles, and workload enrollment
- [Storage](storage.md) — Storage tiers and data sovereignty principles
- [LLM Providers](llm-providers.md) — Provider implementations and capacity tiers
- [Server-Sent Events](sse.md) — Real-time event streaming pipeline and Gateway push delivery
- [Development](devs.md) — Developer setup, guidelines, and coding standards
- [Testing](tests.md) — Testing framework, test tiers, and practices
- [Evals](evals.md) — Benchmark evaluation suite and Judge scoring rubrics
