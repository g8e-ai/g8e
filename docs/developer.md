---
title: Developer
---

# g8e Developer Guide

Last Updated: 2026-05-10
Version: v0.2.2

This document defines the deterministic execution constraints for all code generated for the g8e platform. The platform is an open-source, self-hosted AI governance layer designed for offline operation. The architecture consists of three core components: the Operator (g8eo), the Dashboard (g8ed), and the Engine (g8ee).

I. Core Architectural Invariants
1. Human Agency is Absolute: The human is always the one making state-changing decisions. Every state-changing operation must surface its own approval prompt, ensuring a permanent governance trail. Automatic Function Calling is permanently disabled.

2. 3-Layer Governance:
   - L1 (Technical Bedrock): Hard gates (Forbidden patterns like sudo, allowlists/denylists).
   - L2 (Consensus): Multi-agent tribunal for intent verification and reputation staking.
   - L3 (Authorization): Human-in-the-loop with auto-approval for benign diagnostics.

3. Data Sovereignty: The platform operates as a stateless relay. Raw command output and file contents must stay on the Operator host, encrypted, and never touch the platform side in persistent form. Platform state is stored host-natively in the `.g8e/` directory.

4. Security by Structure: Functionality is built strictly inside the security boundary. All changes must adhere to the Security Review Checklist.

II. Development Lifecycle
The platform runs host-natively. Do not use Docker for primary component development or testing.

1. Setup:
   - Go 1.22+ (for g8eo)
   - Node.js 22+ (for g8ed)
   - Python 3.12+ (for g8ee)

2. Commands:
   - `./g8e`: Launches the Interactive Platform Manager (Menu).
   - `./g8e platform start`: Boots the platform (Operator listen mode, Engine, Dashboard).
   - `./g8e platform status`: Checks health of all host processes.
   - `./g8e login`: Authenticates the local CLI to the running platform.
   - `./g8e test <component>`: Runs host-native tests with managed toolchains.

3. State & Logs:
   - `.g8e/logs/`: Component stdout/stderr.
   - `.g8e/pids/`: Process IDs for running components.
   - `.g8e/ssl/`: CA certs and internal auth tokens.
   - `.g8e/data/`: SQLite databases and KV storage.

III. Anti-Tech-Debt Directives
AI agents are prone to wrapping poorly understood code in new abstractions. This is strictly forbidden.

1. Rip and Replace: When existing code violates contracts or is structurally unsound, you must replace it correctly. Do not route around it with a wrapper.

2. Prohibited Patterns: The implementation of ensure*(), getOrCreate*(), Any in type signatures, and map[string]interface{} for known shapes are hard stops. Functions must do exactly one thing: reads read, and writes write.

3. No Defensive Guards: Never add defensive code to handle unexpected values at the call site. You must hunt down the root cause of why the unexpected value is being received and fix it at the source.

IV. Application Boundary and State Management
1. Single Source of Truth: The `shared/` directory is the canonical source for all wire-protocol values and cross-component document schemas. Components must mirror or load these values at runtime/compile-time.

2. Strict Typing: Inside the application boundary, data lives exclusively as typed model instances. Raw dicts, untyped maps, and ad-hoc JSON are prohibited. Models are only flattened to plain objects when crossing a wire boundary (database, KV cache, HTTP, pub-sub).

3. Protobuf First: `shared/proto/*.proto` defines the canonical wire format. All inter-component communication uses the `UniversalEnvelope` carrying typed Protobuf payloads.

4. Data Access Layering: All document operations must use the `CacheAsideService`. The database is the authoritative source of truth for writes. The KV store is the primary read path. Writes must explicitly invalidate or update the cache to ensure consistency.

V. Component Rules
1. g8ed (Node.js/Express)
   - Service Hierarchy: Domain Layer (Orchestration) and Data Layer (CRUD). Data services must not contain business logic.
   - Model Validation: Subclasses of `G8eBaseModel` must validate, coerce, and strip unknown fields at every inbound boundary.

2. g8ee (Python/FastAPI)
   - Pydantic Enforcement: Domain objects must extend `G8eBaseModel` (Pydantic). Pydantic enforces type checking and extra-field rejection.
   - Async Safety: Avoid state-modifying `finally` blocks in async generators.

3. g8eo / operator (Go)
   - LFAA Payload Stamping: All LFAA results must include an `execution_id`.
   - Concurrency: Goroutines must have explicit cancellation contexts and clear channel ownership.

VI. Testing Invariants
1. Reproduce First: Always reproduce a bug with a failing test before generating the fix.
2. No Mocks: Tests must use real infrastructure (real database, real pub/sub, real LLM calls).
3. Contract Tests: Enforce alignment between components and `shared/` constants/models.