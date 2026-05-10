---
title: Developer
---

# g8e Developer Guide

Last Updated: 2026-05-07
Version: v0.2.0

This document defines the deterministic execution constraints for all code generated for the g8e platform. The platform is an open-source, self-hosted AI governance layer designed for offline operation. The architecture is built around the Operator with Local Function Access & Audit (LFAA), which serves as the backend for the entire platform.

I. Core Architectural Invariants
Human Agency is Absolute: The human is always the one making state-changing decisions. Every state-changing operation must surface its own approval prompt, ensuring a permanent governance trail. Automatic Function Calling is permanently disabled.

Data Sovereignty: The platform operates as a stateless relay. Raw command output and file contents must stay on the Operator host, encrypted, and never touch the platform side in persistent form.

Security by Structure: Functionality is built strictly inside the security boundary. All changes must adhere to the Security Review Checklist.

II. Anti-Tech-Debt Directives
AI agents are prone to wrapping poorly understood code in new abstractions. This is strictly forbidden.

Rip and Replace: When existing code violates contracts or is structurally unsound, you must replace it correctly. Do not route around it with a wrapper.

Prohibited Patterns: The implementation of ensure*(), getOrCreate*(), Any in type signatures, and map[string]interface{} for known shapes are hard stops. Functions must do exactly one thing: reads read, and writes write.

No Defensive Guards: Never add defensive code to handle unexpected values at the call site. You must hunt down the root cause of why the unexpected value is being received and fix it at the source.

III. Application Boundary and State Management
Single Source of Truth: The shared/ directory is the canonical source for all wire-protocol values and cross-component document schemas. Components must mirror or load these values at runtime/compile-time.

Strict Typing: Inside the application boundary, data lives exclusively as typed model instances. Raw dicts, untyped maps, and ad-hoc JSON are prohibited. Models are only flattened to plain objects when crossing a wire boundary (database, KV cache, HTTP, pub-sub).

No Type Coercion Fallbacks: Fix the model or the caller. Never use JSON.stringify or String() to hide contract bugs.

Data Access Layering: All document operations must use the CacheAsideService. The database is the authoritative source of truth for writes. The KV store is the primary read path. Writes must explicitly invalidate or update the cache to ensure consistency.

IV. Component and Language-Specific Rules
1. g8ed (Node.js/Express)
Service Hierarchy: Adhere strictly to the two-tier hierarchy: Domain Layer (Orchestration) and Data Layer (CRUD). Data services must not contain business logic. Services must interact via Protocols rather than concrete classes.

Model Validation: Subclasses of G8eBaseModel must validate, coerce, and strip unknown fields at every inbound boundary using ModelClass.parse(raw).

2. g8ee (Python/FastAPI)
Async Safety: Avoid state-modifying finally blocks in async generators.

Pydantic Enforcement: Domain objects must extend G8eBaseModel (Pydantic). Pydantic enforces type checking, default handling, and extra-field rejection at construction time. Use model.model_dump(mode="json") exclusively for wire boundaries.

3. g8eo / operator (Go)
LFAA Payload Stamping: All Local Function Access & Audit (LFAA) result payloads published by g8eo must include an execution_id field. The payload struct must implement the models.ExecutionIDSetter interface.

Concurrency: Goroutines must have explicit cancellation contexts. Do not leave dangling goroutines. Channels must have clear ownership for closure.

Wire Alignment: Structs must strictly align with shared/proto/*.proto for all wire-protocol payloads. The g8e platform uses Protobuf as the canonical wire format for operator commands, results, and heartbeats.

V. Testing Invariants
Reproduce First: Always reproduce a bug with a failing test before generating the fix.

Contract Tests: Enforce alignment between components and shared constants/models using strict contract tests.