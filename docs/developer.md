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

### Data Sovereignty
The platform is a stateless relay. Raw command output and file contents stay on the Operator host, encrypted, and never touch the platform side in persistent form.

---

## Service Dependency Graph

The g8e platform follows a strict hierarchical communication flow:

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

## Service Hierarchy (Domain vs. Data)

All component services must adhere to a two-tier hierarchy to ensure strict separation of concerns:

1.  **Domain Layer (Orchestration):** High-level services (e.g., `InvestigationService`) hosting business logic, coordinating multiple data services, and managing complex state.
2.  **Data Layer (CRUD):** Low-level services (e.g., `InvestigationDataService`) providing pure CRUD operations. These must not contain business logic or orchestration.

**Dependency Management:** Services must interact via **Protocols** rather than concrete classes to prevent circular dependencies and enable clean mocking.

---

## Shared Source of Truth

The `shared/` directory is the canonical source for all wire-protocol values and cross-component document schemas.

- **Constants:** All event types, status strings, and channel patterns must be defined in `shared/constants/*.json`.
- **Models:** All cross-component document schemas must be defined in `shared/models/*.json`.
- **Enforcement:** Components must mirror or load these values at runtime/compile-time. Contract tests enforce that component-specific constants match the shared definitions.

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
