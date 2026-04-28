---
title: Developer
---

# g8e Developer Guide

g8e is an open-source, self-hosted AI governance platform designed for offline operation: with a local LLM provider (Ollama or any OpenAI-compatible endpoint) it runs with zero cloud dependencies at runtime. Building the container images currently still requires outbound access to Docker Hub, PyPI, npmjs, and the Alpine/Debian package mirrors — see [`docs/architecture/air_gap.md`](architecture/air_gap.md) for the deployment path and current vendoring status. The architecture is built around the Operator with LFAA (Local Function Access & Audit), which serves as the backend for the entire platform.

---

## Engineering Commitments

### User Intent is the Guiding Principle
The human is always the one making state-changing decisions. Human control is a first-class architectural property and the core of our governance model. Every contribution must be evaluated through this lens.

### Human Agency is the Governance Model
- **Propose/Approve:** The AI proposes, the human approves. Nothing executes without explicit user consent.
- **No Autonomy:** Automatic Function Calling is permanently disabled to ensure human oversight.
- **Auditability:** Every state-changing operation surfaces its own approval prompt, ensuring a permanent governance trail.

### Security and Privacy as Bedrock
Security is the first constraint of governance. Functionality is built inside it. All changes must pass the Security Review Checklist in `docs/architecture/security.md`.

### No Tech Debt
- **Rip and Replace:** When code is wrong, replace it correctly.
- **Prohibited Patterns:** `ensure*()`, `getOrCreate*()`, `JSON.stringify` as type coercion, `Any` in signatures, and `map[string]interface{}` for known shapes are hard stops.
- **Refactor AI-Generated Tech Debt:** We use AI extensively to build this codebase. If you encounter code that works but makes no structural sense—unnecessary wrappers, inverted abstractions, redundant indirection—fix it. Do not route around it with another wrapper. Refining abstractions is a permanent part of the SDLC.

### Data Sovereignty
The platform is a stateless relay. Raw command output and file contents stay on the Operator host, encrypted, and never touch the platform side in persistent form.

---

## Platform Components

1.  **Browser:** The user interface for interacting with the platform.
2.  **g8ed (Node.js/Express):** Web frontend and Gateway Protocol. Handles user authentication, session management, and routes requests to g8ee.
3.  **g8ee (Python/FastAPI):** The AI Engine. Manages chat pipelines, AI reasoning, tool orchestration, and coordinates with g8es for persistence.
4.  **g8eo (Go):** The Operator reference implementation. Executes commands on target systems, enforces LFAA, and reports results via pub/sub. Any client following the g8e events protocol can act as an Operator.
5.  **g8es (Go/SQLite):** A standalone Go service providing persistence (SQLite document store), KV cache, and pub/sub messaging. The g8eo binary can run in multiple modes: standard operator mode (executes commands on target systems), listen mode (acts as the platform's central persistence and pub/sub broker), or OpenClaw node host mode (connects to an OpenClaw Gateway).
6.  **g8el (llama.cpp):** Optional local LLM inference server. Provides OpenAI-compatible API for running quantized models (e.g., Gemma 4 E2B) using the llama.cpp library. Integrates with g8ee as the LLAMACPP provider.

### Communication Flow
- **g8ed ↔ g8ee:** Synchronous HTTP for orchestration and state management.
- **g8ee ↔ g8eo:** Asynchronous Pub/Sub via g8es for command execution and result streaming.
- **Pub/Sub:** Real-time event routing for heartbeats, status updates, and execution results.

---

## Service Dependency Graphs

### g8ed (Node.js) Services

All services are constructed in `services/initialization.js` in strict phase order. Arrows point from consumer to dependency.

```
g8es Transport Layer
├── G8esDocumentClient
├── G8esPubSubClient
├── KVCacheClient
└── G8esBlobClient

Cache Layer
└── CacheAsideService ──────────> G8esDocumentClient, KVCacheClient

Auth Domain
├── WebSessionService ──────────> CacheAsideService, BootstrapService
├── CliSessionService ──────────> CacheAsideService, BootstrapService, AuditService
├── ApiKeyDataService ──────────> CacheAsideService
├── ApiKeyService ──────────────> ApiKeyDataService
├── UserService ────────────────> CacheAsideService, OrganizationModel, ApiKeyService
├── PasskeyAuthService ─────────> UserService, CacheAsideService, SettingsService
├── LoginSecurityService ───────> CacheAsideService
├── DownloadAuthService ────────> CacheAsideService, UserService, ApiKeyService
├── PostLoginService ───────────> WebSessionService, ApiKeyService, UserService,
│                                 OperatorService, G8ENodeOperatorService,
│                                 SSEService, ConsoleMetricsService
├── DeviceRegistrationService ──> OperatorService, UserService, SSEService,
│                                 InternalHttpClient
├── DeviceLinkService ──────────> CacheAsideService, OperatorService,
│                                 WebSessionService, DeviceRegistrationService
└── BindOperatorsService ───────> OperatorService, BoundSessionsService,
                                  WebSessionService, SSEService

Operator Domain
├── OperatorDataService ────────> CacheAsideService
├── OperatorService ────────────> OperatorDataService, UserService, ApiKeyService,
│                                 WebSessionService, CertificateService,
│                                 SSEService, InternalHttpClient
├── OperatorDownloadService
├── BoundSessionsService ───────> CacheAsideService, OperatorService
├── OperatorRelayService ───────> G8esPubSubClient, OperatorService
└── OperatorNotificationService ─> SSEService, OperatorService

Platform Domain
├── SettingsService ────────────> CacheAsideService, BootstrapService
├── SSEService ─────────────────> SettingsService, InternalHttpClient,
│                                 BoundSessionsService, InvestigationService
├── AttachmentService ──────────> CacheAsideService, G8esBlobClient
├── CertificateService ─────────> BootstrapService, InternalHttpClient
├── ConsoleMetricsService ──────> CacheAsideService, InternalHttpClient
├── G8ENodeOperatorService ─────> SettingsService, OperatorService
├── HealthCheckService ─────────> CacheAsideService, WebSessionService
├── InvestigationService ───────> CacheAsideService
├── SetupService ───────────────> UserService, SettingsService
├── AuditService
└── InternalHttpClient ─────────> BootstrapService, SettingsService
```

### g8ee (Python/FastAPI) Services

All services are constructed by `ServiceFactory.create_all_services()` in `services/service_factory.py` and bound to `app.state` at startup.

```
g8es Transport Layer
├── DBService (g8es) ────────────> G8esDocumentClient
├── KVService (g8es) ────────────> KVCacheClient
├── BlobService (g8es) ──────────> BlobClient
└── PubSubClient (g8es)

Cache Layer
└── CacheAsideService ──────────> KVService, DBService

Infra Services
├── HTTPService (Aiohttp)
├── InternalHttpClient ─────────> G8eePlatformSettings
└── EventService (g8ed) ────────> InternalHttpClient

Data Layer (CRUD)
├── CaseDataService ────────────> CacheAsideService, EventService
├── InvestigationDataService ───> CacheAsideService
├── OperatorDataService ────────> CacheAsideService, InternalHttpClient
├── MemoryDataService ──────────> CacheAsideService
├── AgentActivityDataService ───> CacheAsideService
├── ReputationDataService ──────> CacheAsideService
├── StakeResolutionDataService ─> CacheAsideService
└── AttachmentStoreService ─────> BlobService

Domain Layer (Orchestration)
├── InvestigationService ───────> InvestigationDataService, OperatorDataService,
│                                 MemoryDataService
├── MemoryGenerationService ────> MemoryDataService
├── ReputationService ──────────> ReputationDataService, StakeResolutionDataService
└── SshInventoryService

Operator Services
├── OperatorHeartbeatService ───> OperatorDataService, EventService, PubSubClient
├── HeartbeatStaleMonitorService > OperatorDataService, EventService
├── OperatorCommandService ─────> PubSubService, ApprovalService, ExecutionService,
│                                 FilesystemService, PortService, FileService,
│                                 IntentService, LFAAService, OperatorDataService
├── OperatorApprovalService ────> EventService, OperatorDataService,
│                                 InvestigationDataService
├── OperatorExecutionService ───> PubSubService, OperatorApprovalService,
│                                 EventService, AIResponseAnalyzer
├── OperatorStreamExecutor ─────> OperatorApprovalService, InternalHttpClient
└── OperatorAuthService ────────> ApiKeyService, OperatorSessionService,
                                  OperatorDataService, CertificateService

AI Pipeline
├── AIToolService ──────────────> OperatorCommandService, InvestigationService,
│                                 ReputationService, SshInventoryService,
│                                 OperatorStreamExecutor, WebSearchProvider
├── GroundingService
│   ├── AttachmentProvider
│   └── WebSearchProvider
├── AIRequestBuilder ───────────> AIToolService
├── AIResponseAnalyzer
├── g8eEngine ──────────────────> AIToolService, GroundingService, OperatorApprovalService
├── ChatPipelineService ────────> EventService, InvestigationService, AIRequestBuilder,
│                                 g8eEngine, MemoryDataService, TriageAgent,
│                                 AgentActivityDataService
├── BackgroundTaskManager
└── TriageAgent (instantiated by ChatPipelineService)

```

---

## Service Hierarchy (Domain vs. Data)

All component services must adhere to a two-tier hierarchy to ensure strict separation of concerns:

1.  **Domain Layer (Orchestration):** High-level services (e.g., `InvestigationService`) hosting business logic, coordinating multiple data services, and managing complex state.
2.  **Data Layer (CRUD):** Low-level services (e.g., `InvestigationDataService`) providing pure CRUD operations. These must not contain business logic or orchestration.

**Dependency Management:** Services must interact via **Protocols** rather than concrete classes to prevent circular dependencies and enable clean mocking.

---

## AI Pipeline Services

The AI pipeline in g8ee consists of several specialized services that orchestrate LLM interactions, tool execution, and result evaluation:

### TriageAgent
- **Location:** `app/services/ai/triage.py`
- **Purpose:** Classifies incoming user messages as 'simple' or 'complex' using the `triage` persona. This determines the model tier (Assistant/Dash vs Primary/Sage) used for the response.
- **Integration:** Instantiated directly by `ChatPipelineService`.
- **Key Behaviors:**
  - **Short-circuits:** COMPLEX if attachments are present; COMPLEX for empty messages.
  - **Metadata:** Provides complexity, intent (INFORMATION, ACTION, UNKNOWN), and request posture (NORMAL, ESCALATED, ADVERSARIAL, CONFUSED).
  - **Safe Default:** Falls back to COMPLEX on any error.

### g8eEngine
- **Location:** `app/services/ai/agent.py`
- **Purpose:** The core ReAct loop orchestrator. Manages the lifecycle of an AI turn, including tool execution and streaming delivery.
- **Key Features:**
  - **Retry Loop:** Implements AGENT_MAX_RETRIES with backoff.
  - **ReAct Loop:** Coordinates provider turn -> tool calls -> next turn.
  - **Streaming:** Handles SSE delivery through `agent_sse.py`.

### Tribunal Generator
- **Location:** `app/services/ai/generator.py`
- **Purpose:** Orchestrates the five-member heterogeneous AI panel (The Tribunal) to convert Sage's intent into executable shell commands.
- **Key Features:**
  - **Multi-Round Consensus:** Supports parallel generation passes (Axiom, Concord, Variance, Pragma, Nemesis) and weighted voting.
  - **Auditor Integration:** The `auditor` persona verifies the winner against the original intent.
  - **Reputation Binding:** Cryptographically binds Tribunal outcomes to the reputation scoreboard.

### Tool Execution
- **Location:** `app/services/ai/tool_service.py` and `app/services/operator/command_service.py`.
- **Purpose:** Dispatches tool calls to specific handlers and manages operator interactions.
- **Key Features:**
  - **Orchestration:** `OperatorCommandService` coordinates PubSub, Approval, Execution, and LFAA.
  - **Sequential Loop:** `agent_tool_loop.py` executes multiple tool calls from a single turn.
  - **LFAA Integration:** Every command is audited via Local Function Access & Audit.

### Utility AI Services
- **TitleGenerator (`scribe`):** Generates concise case titles from initial user messages.
- **MemoryGenerationService (`codex`):** Extracts user preferences and investigation summaries into durable memories.
- **ReputationService:** Tracks the performance and reliability of Tribunal members and primary agents.
- **BenchmarkJudge:** Deterministic judge for grading agent tool call payloads.

---

## Shared Source of Truth

The `shared/` directory is the canonical source for all wire-protocol values and cross-component document schemas.

- **Constants:** All event types, status strings, and channel patterns must be defined in `shared/constants/*.json`.
- **Models:** All cross-component document schemas must be defined in `shared/models/*.json`. Wire-specific shapes live in `shared/models/wire/*.json`; persistent document shapes live in `shared/models/*.json`.
- **Enforcement:** Components must mirror or load these values at runtime/compile-time. Contract tests enforce that component-specific constants match the shared definitions.

### Typed Objects Derived from Shared Schemas

Every component derives its own strongly-typed model classes from the canonical schemas in `shared/`. Raw dicts, untyped maps, and ad-hoc JSON are prohibited inside application code.

- **g8ed (Node.js):** Domain objects extend `G8eBaseModel` in `components/g8ed/models/base.js`. Each subclass declares a static `fields` object whose shape is aligned with the corresponding `shared/models/` schema. Construction goes through `ModelClass.parse(raw)`, which validates, coerces, and strips unknown fields at every inbound boundary.
- **g8ee (Python):** Domain objects extend `G8eBaseModel` (Pydantic) in `components/g8ee/app/models/base.py`. Each subclass declares typed Pydantic fields aligned with the corresponding `shared/models/` schema. Pydantic enforces type checking, default handling, and extra-field rejection at construction time.
- **g8eo (Go):** Structs in `components/g8eo/` are aligned with `shared/models/wire/*.json` for all wire-protocol payloads.

### Application Boundary Rule

Inside the application boundary, data lives as typed model instances — never as raw dicts or unstructured JSON. Models are only flattened to plain objects when crossing a boundary:

| Boundary | g8ee method |
|---|---|
| Database write | `model.model_dump(mode="json")` |
| KV cache write | `model.model_dump(mode="json")` |
| Outbound HTTP / pub-sub | `model.model_dump(mode="json")` |
| Browser response | `model.model_dump()` |
| LLM tool response | `model.model_dump(mode="json")` |

All datetime fields use the `UTCDatetime` type which serializes to ISO 8601 with `Z` suffix (e.g., `2026-01-15T10:30:00.123456Z`).

Passing raw dicts between services, storing unvalidated JSON in the database, or constructing ad-hoc objects at call sites are all prohibited. If a shape crosses a wire boundary, it must have a corresponding entry in `shared/models/` and a typed model class in the consuming component.

### LFAA Result Payload Requirements

All LFAA (Local Function Access & Audit) result payloads published by g8eo MUST include an `execution_id` field for request-response correlation. This field is automatically stamped by `setExecutionIDOnPayload()` in `components/g8eo/services/pubsub/publish_helpers.go` before serialization, via the `models.ExecutionIDSetter` interface defined in `components/g8eo/models/execution_id_setter.go`.

Participating payloads implement:

```go
type ExecutionIDSetter interface {
    SetExecutionID(string)
}
```

When adding a new result payload type, do the following:
1. Declare an `ExecutionID string` field with JSON tag `execution_id`.
2. Implement `SetExecutionID(id string)` by adding a one-line method in `components/g8eo/models/execution_id_setter.go`.

No test registration is required. The AST-based guardrails in `components/g8eo/models/execution_id_setter_test.go` auto-discover every struct whose name ends in `ResultPayload`, `StatusPayload`, or `ErrorPayload` and carries an `ExecutionID` field, and fail the build if its `SetExecutionID` method is missing or non-trivial. The publisher does not need changes — it dispatches through the interface, not a concrete type switch.

All result publishers — both the consolidated `publishResultEnvelope` helper in `components/g8eo/services/pubsub/pubsub_results.go` (command/cancellation/file-edit/fs-list) and the `publishLFAA*` helpers in `components/g8eo/services/pubsub/publish_helpers.go` (file read, port check, fetch logs/history, restore file) — MUST stamp `msg.APIKey = cfg.APIKey` on the outbound `G8eMessage` for operator identity continuity across pub/sub. Regression coverage lives in `TestPublishLFAA_StampsAPIKeyFromConfig` (LFAA path) and the `api_key` assertions in `components/g8eo/services/pubsub/pubsub_results_test.go` (envelope path).

---

## Code Quality Standards

### Universal Rules
- **No Emojis:** Prohibited in code, comments, logs, and runtime strings.
- **No Inline Styles:** Use proper CSS definitions.
- **No .env for Secrets:** All runtime configuration flows through the `platform_settings` pipeline.
- **No Unnecessary Comments:** Only include comments that are essential for understanding complex logic.
- **Async Safety:** Avoid state-modifying `finally` blocks in async generators.

### Prohibited Implementation Patterns
- **No `ensure*()` / `getOrCreate*()`:** Every function does exactly one thing. Reads read. Writes write.
- **Explicit Ownership:** Every document has a single authoritative writer component.
- **Boundary Validation:** Every value crossing a wire boundary must be parsed and validated through a model factory (`.parse()`).
- **No Type Coercion Fallbacks:** Fix the model or the caller; never use `JSON.stringify` or `String()` to hide contract bugs.
- **No Deduplicators, Arbitrary Guards, or Unions:** Never add defensive code to handle unexpected values at the call site. Hunt down the root cause of why the unexpected value is being received and fix it at the source.

### Data Access
All document operations must use the `CacheAsideService`.
- **Authoritative DB:** The database is the source of truth for all writes.
- **Read Cache:** The KV store is the primary read path.
- **Invalidation:** Writes must explicitly invalidate or update the cache to ensure consistency.

---

## Testing Philosophy

Tests are a non-negotiable requirement for all contributions.
- **Reproduce First:** Always reproduce a bug with a test before fixing it.
- **Contract Tests:** Enforce alignment between components and shared constants/models.
- **Isolation:** Use DI and protocols to test services in isolation with mocks.
- **Quality:** High-quality tests must cover edge cases and error paths, not just the happy path.

Refer to `docs/testing.md` for detailed component-specific testing guides.
