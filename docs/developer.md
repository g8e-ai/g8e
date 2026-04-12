# g8e Developer Guide

> **© 2026 Lateralus Labs, LLC.**  
> g8e is licensed under the [Apache License, Version 2.0](LICENSE).

---

## Origins and Architecture

g8e is a fully self-hosted, air-gapped capable AI governance platform with zero cloud dependencies. The architecture is built around the Operator with LFAA (Local Function Access & Audit), which serves as the backend for the entire platform.

When something is wrong, the right move is to fix it correctly and leave no trace. There is no legacy cloud compatibility layer to maintain.

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
- **Flag AI-Generated Smell:** Nearly all of this codebase was AI-authored. Some of it works but makes no structural sense — unnecessary wrappers, inverted abstractions, redundant indirection, over-engineered helpers. When you encounter code like this during an investigation or feature, fix it. Do not route around it with another wrapper. Cleaning up AI-written code is a permanent part of the SDLC, not a one-time event.

### Data Sovereignty
The platform is a stateless relay. Raw command output and file contents stay on the Operator host, encrypted, and never touch the platform side in persistent form.

---

## Platform Components

1.  **Browser:** The user interface for interacting with the platform.
2.  **g8ed (Node.js/Express):** Web frontend and Gateway Protocol. Handles user authentication, session management, and routes requests to g8ee.
3.  **g8ee (Python/FastAPI):** The AI Engine. Manages chat pipelines, agent logic, tool orchestration, and coordinates with g8es for persistence.
4.  **g8eo (Go):** The Operator binary. Executes commands on target systems, enforces LFAA, and reports results via pub/sub.
5.  **g8es (Go/SQLite):** The persistence and pub/sub broker. Provides the document store and real-time message bus for all components.

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
└── CacheAsideService ──> G8esDocumentClient, KVCacheClient

Auth Domain
├── WebSessionService ──────────> CacheAsideService, BootstrapService
├── OperatorSessionService ─────> CacheAsideService, BootstrapService
├── ApiKeyDataService ──────────> CacheAsideService
├── ApiKeyService ──────────────> ApiKeyDataService
├── UserService ────────────────> CacheAsideService, OrganizationModel, ApiKeyService
├── PasskeyAuthService ─────────> UserService, CacheAsideService, SettingsService
├── LoginSecurityService ───────> CacheAsideService
├── DownloadAuthService ────────> CacheAsideService, UserService, ApiKeyService
├── PostLoginService ───────────> WebSessionService, ApiKeyService, UserService,
│                                 OperatorService, G8ENodeOperatorService
├── DeviceRegistrationService ──> OperatorService, OperatorSessionService, UserService,
│                                 SSEService, InternalHttpClient, SessionAuthListener
├── DeviceLinkService ──────────> CacheAsideService, OperatorService,
│                                 WebSessionService, DeviceRegistrationService
└── SessionAuthListener ────────> G8esPubSubClient, OperatorSessionService, OperatorService

Operator Domain
├── OperatorDataService ────────> CacheAsideService
├── OperatorService ────────────> OperatorDataService, UserService, ApiKeyService,
│                                 OperatorSessionService, WebSessionService,
│                                 CertificateService, SSEService
├── OperatorDownloadService
├── OperatorAuthService ────────> ApiKeyService, UserService, OperatorService,
│                                 OperatorSessionService, BoundSessionsService,
│                                 WebSessionService
├── BindOperatorsService ───────> OperatorService, BoundSessionsService,
│                                 OperatorSessionService, WebSessionService
└── BoundSessionsService ──────> CacheAsideService, OperatorService

Platform Domain
├── SettingsService ────────────> CacheAsideService, BootstrapService
├── SSEService ─────────────────> SettingsService, InternalHttpClient, BoundSessionsService
├── AttachmentService ──────────> CacheAsideService, G8esBlobClient
├── CertificateService ─────────> BootstrapService
├── ConsoleMetricsService ──────> CacheAsideService, InternalHttpClient
├── G8ENodeOperatorService ─────> SettingsService, OperatorService
├── HealthCheckService ─────────> CacheAsideService, WebSessionService
├── SetupService ───────────────> UserService, SettingsService
├── AuditService
└── InternalHttpClient ─────────> BootstrapService, SettingsService
```

### g8ee (Python/FastAPI) Services

All services are constructed by `ServiceFactory.create_all_services()` in `services/service_factory.py` and bound to `app.state` at startup.

```
g8es Transport Layer
├── DBService ──────────────────> G8esDocumentClient
├── KVService ──────────────────> KVCacheClient
├── BlobService ────────────────> BlobClient
└── PubSubClient

Cache Layer
└── CacheAsideService ──────────> KVService, DBService

Infra Services
├── BootstrapService
├── SettingsService ────────────> CacheAsideService, BootstrapService
├── HTTPService
├── InternalHttpClient ─────────> G8eePlatformSettings
└── EventService (g8ed) ────────> InternalHttpClient

Data Layer (CRUD)
├── CaseDataService ────────────> CacheAsideService, EventService
├── InvestigationDataService ───> CacheAsideService
├── OperatorDataService ────────> CacheAsideService, InternalHttpClient
├── MemoryDataService ──────────> CacheAsideService
└── AttachmentStoreService ─────> BlobService

Domain Layer (Orchestration)
├── InvestigationService ───────> InvestigationDataService, OperatorDataService,
│                                 MemoryDataService
└── MemoryGenerationService ────> MemoryDataService

Operator Services
├── OperatorHeartbeatService ───> OperatorDataService, EventService, PubSubClient
├── ExecutionRegistryService
├── OperatorApprovalService ────> EventService, OperatorDataService,
│                                 InvestigationDataService
├── OperatorCommandService ─────> CacheAsideService, OperatorDataService,
│                                 InvestigationService, EventService,
│                                 ExecutionRegistryService, ApprovalService,
│                                 InternalHttpClient, PubSubClient
├── LFAAService
├── FileService
├── FilesystemService
├── IntentService
├── CloudCommandValidator
├── IAMCommandBuilder
└── PortService

AI Pipeline
├── AIToolService ──────────────> OperatorCommandService, InvestigationService,
│                                 WebSearchProvider
├── GroundingService
│   ├── AttachmentProvider
│   └── WebSearchProvider
├── AIRequestBuilder ───────────> AIToolService
├── AIResponseAnalyzer
├── g8eEngine (Agent) ──────────> AIToolService, GroundingService
├── ChatPipelineService ────────> EventService, InvestigationService,
│                                 AIRequestBuilder, g8eEngine,
│                                 MemoryDataService, MemoryGenerationService
├── ChatTaskManager
├── TitleGenerator
├── CommandGenerator
├── GenerationConfigBuilder
└── EvalJudge

MCP Services
├── MCPGatewayService ──────────> AIToolService, InvestigationService,
│                                 OperatorDataService
└── MCPAdapter
```

---

## Service Hierarchy (Domain vs. Data)

All component services must adhere to a two-tier hierarchy to ensure strict separation of concerns:

1.  **Domain Layer (Orchestration):** High-level services (e.g., `InvestigationService`) hosting business logic, coordinating multiple data services, and managing complex state.
2.  **Data Layer (CRUD):** Low-level services (e.g., `InvestigationDataService`) providing pure CRUD operations. These must not contain business logic or orchestration.

**Dependency Management:** Services must interact via **Protocols** rather than concrete classes to prevent circular dependencies and enable clean mocking.

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

| Boundary | g8ed method | g8ee method |
|---|---|---|
| Database write | `model.forDB()` | `model.flatten_for_db()` |
| Outbound HTTP / pub-sub | `model.forWire()` | `model.flatten_for_wire()` |
| Browser response | `model.forClient()` | `model.model_dump()` |
| LLM tool response | — | `model.flatten_for_llm()` |

Passing raw dicts between services, storing unvalidated JSON in the database, or constructing ad-hoc objects at call sites are all prohibited. If a shape crosses a wire boundary, it must have a corresponding entry in `shared/models/` and a typed model class in the consuming component.

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
