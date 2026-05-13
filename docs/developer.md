---
title: Developer
---

# g8e Developer Guide

Last Updated: 2026-05-13
Version: v0.2.5

This document defines the deterministic execution constraints for all code generated for the g8e platform. The platform is an open-source, self-hosted AI governance layer designed for offline operation. The mandatory substrate is the Operator (g8eo) plus the shared Protobuf protocol; the Engine (g8ee) is an optional application-layer adapter.

I. Core Architectural Invariants
1. Human Agency is Absolute: The human is always the one making state-changing decisions. Every state-changing operation must surface its own approval prompt, ensuring a permanent governance trail. Automatic Function Calling is permanently disabled.

2. 3-Layer Governance Bedrock:
   - L1 (Technical Bedrock): Hard gates (Forbidden patterns like sudo, allowlists/denylists) enforced via Protobuf reflection.
   - L2 (Consensus): Multi-agent tribunal (Axiom, Concord, Variance, Pragma, Nemesis) for intent verification and reputation staking.
   - L3 (Authorization): Human-in-the-loop with auto-approval for benign diagnostics. State-changing actions require local passkey/WebAuthn proofs.

3. Data Sovereignty: The platform operates as a stateless relay. Raw command output and file contents must stay on the Operator host, encrypted, and never touch the platform side in persistent form. Platform state is stored host-natively in the `.g8e/` directory.

4. Security by Structure: Functionality is built strictly inside the security boundary. All changes must adhere to the Security Review Checklist. The Operator (g8eo) is the only component with execution authority.

II. Development Lifecycle
The platform runs host-natively. Do not use Docker for primary component development or testing.

1. Setup:
   - Go 1.22+ (for the Operator/protocol substrate)
   - Python 3.12+ (only when developing the optional g8ee adapter)

2. Commands:
   - `./g8e`: Launches the Interactive Platform Manager (Menu).
   - `./g8e platform start`: Boots the Operator/protocol substrate only.
   - `./g8e platform start --with-apps`: Boots the Operator plus optional bundled app adapters.
   - `./g8e apps start [g8ee|all]`: Starts optional application-layer adapters explicitly.
   - `./g8e platform status`: Checks substrate health first and optional app status separately.
   - `./g8e login`: Authenticates the local CLI to the running platform.
   - `./g8e test <component>`: Runs host-native tests (default: g8eo).

3. State & Logs:
   - `.g8e/logs/`: Component stdout/stderr.
   - `.g8e/pids/`: Process IDs for running components.
   - `.g8e/pki/`: Operator PKI hierarchy (CA, intermediate, workload certs).
   - `.g8e/secrets/`: Bootstrap secrets with tamper-evidence manifest.
   - `.g8e/data/`: SQLite databases and KV storage.

4. Four-Port Contract (Listen Mode):
   - **WSS (9001)**: Pub/Sub broker for operator connections (mTLS required)
   - **HTTP (9000)**: mTLS API for substrate operations (mTLS required)
   - **Bootstrap (8080)**: Device-link enrollment and CSR-based registration (plain TLS)
   - **Public (8081)**: Browser-based auth and BYO bootstrap (plain TLS)

III. Anti-Tech-Debt Directives
AI agents are prone to wrapping poorly understood code in new abstractions. This is strictly forbidden.

1. Rip and Replace: When existing code violates contracts or is structurally unsound, you must replace it correctly. Do not route around it with a wrapper. No backwards compatibility is maintained for broken data structures or legacy shims.

2. Prohibited Patterns: The implementation of ensure*(), getOrCreate*(), Any in type signatures, and map[string]interface{} for known shapes are hard stops. Functions must do exactly one thing: reads read, and writes write.

3. No Defensive Guards: Never add defensive code to handle unexpected values at the call site. You must hunt down the root cause of why the unexpected value is being received and fix it at the source.

IV. Application Boundary and State Management
1. Single Source of Truth: The `shared/` directory is the canonical source for all wire-protocol values and cross-component document schemas. Components must mirror or load these values at runtime/compile-time.

2. Strict Typing: Inside the application boundary, data lives exclusively as typed model instances. Raw dicts, untyped maps, and ad-hoc JSON are prohibited. Models are only flattened to plain objects when crossing a wire boundary (database, KV cache, HTTP, pub-sub).

3. UAP JSON First: The canonical mutation envelope is UAP JSON (aliased to `UniversalEnvelope` in `components/g8eo/pkg/uap/types.go`). All state-changing transactions use JSON bytes carrying a base64-encoded binary `payload` field. This binary payload is the ONLY authority for execution and MUST contain the typed Protobuf message (e.g., `CommandRequested`). `intent_data` (google.protobuf.Struct) provides a structured view for visibility and audit but is NEVER used as a fallback for execution.

4. Data Access Layering: All document operations must use the `CacheAsideService`. The database is the authoritative source of truth for writes. The KV store is the primary read path. Writes must explicitly invalidate or update the cache to ensure consistency.

V. Component Rules
1. g8eo / operator (Go)
   - LFAA Payload Stamping: All LFAA results must include an `execution_id`.
   - Concurrency: Goroutines must have explicit cancellation contexts and clear channel ownership.
   - Substrate Boundary: Any capability needed by bundled apps or BYO clients must be exposed through the public Operator protocol.
   - Execution Boundary: Warden is the sole circuit breaker before dispatch. Every accepted mutation must emit a signed `ActionReceipt`.

2. g8ee (Python/FastAPI, optional application-layer adapter)
   - Pydantic Enforcement: Domain objects must extend `G8eBaseModel` (Pydantic). Pydantic enforces type checking and extra-field rejection.
   - Async Safety: Avoid state-modifying `finally` blocks in async generators.
   - Adapter Boundary: Must produce protocol-verifiable proposals and proofs as an external signer would.

VI. Testing Invariants
1. Reproduce First: Always reproduce a bug with a failing test before generating the fix.
2. No Mocks: Tests must use real infrastructure (real database, real pub/sub, real LLM calls).
3. Contract Tests: Enforce alignment between the Operator, optional adapters, and `shared/` constants/models using typed Protobuf assertions.