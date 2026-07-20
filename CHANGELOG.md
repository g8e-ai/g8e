# Changelog

All notable changes to this project are documented in per-release notes under [`docs/release_notes/`](docs/release_notes/).
This file serves as an index — each entry links to the full release notes for details.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## v1.5.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| 1.5.9 | 2026-07-19 | Documentation, protocol conformance, and generated protobuf API reference release. Expands encryption architecture with certificate management and TLS standards documentation, consolidates release workflow into a single `make release` target, adds troubleshooting coverage, generates protobuf Go stubs and API docs for common, operator, and pubsub packages, and strengthens Python conformance tests with `auth.json` multi-wrapper validation and improved model schema field detection. | [v1.5.9](docs/release_notes/v1.5.x/v1.5.9.md) |
| 1.5.8 | 2026-07-19 | Protocol constant additions, enum fixes, and wire-format documentation. Restores 8 intent EventType members, adds SessionOperator KV key template, expands InfrastructureStatus and AuthMethod enums, adds grant_intent/revoke_intent API paths, fixes SessionType.CLI Python enum generation, fixes SectionVaultMode JSON SSOT value mismatch, strengthens _SSEEventBody.type type safety, and documents wire-format changes for EventType, AgentMode, and SessionEventWire.user_id. | [v1.5.8](docs/release_notes/v1.5.x/v1.5.8.md) |
| 1.5.7 | 2026-07-18 | Test coverage improvement and governance interface refactoring. Raises aggregate test coverage to 75.9% (threshold: 75%). Adds approximately 60 new tests across 12 packages. Extracts governance interfaces from `l4_warden.go` into separate files, moves test fixtures to `governancetest` package, and introduces shared test doubles (`TestTokenStore`, `TestResultsPublisher`). Exports `SentinelKeyPrefix` constant. Removes dead `FilesystemSignerStore.logger` field. Refactors `getMachineID` for unsupported OS branch testability. | [v1.5.7](docs/release_notes/v1.5.x/v1.5.7.md) |
| 1.5.6 | 2026-07-18 | Lattice gRPC adapter, MCP tool interception verification, agent support consolidation, and Python protocol enhancements. Adds Anduril Lattice adapter with OAuth2 client credentials auth and gRPC retry. Introduces pre-launch tool interception verification (`--verify` flag) for `g8e mcp agent run`. Consolidates supported agents to Claude Code, Codex, Goose, and Gemini CLI. Adds `GovernanceEnvelope` model with `compute_transaction_hash()`, dynamic enums, and `_python_const` fields to the Python protocol package. Adds heartbeat sink registration for external components. Fixes L4 Warden raw-bytes passthrough for adapter-specific action types. | [v1.5.6](docs/release_notes/v1.5.x/v1.5.6.md) |
| 1.5.5 | 2026-07-15 | Python protocol package fix. Bundles JSON constants files into the PyPI package and updates the constants directory lookup logic to find bundled files first, fixing runtime failures when the package is installed from PyPI without the protocol source tree. | [v1.5.5](docs/release_notes/v1.5.x/v1.5.5.md) |
| 1.5.4 | 2026-07-15 | Release process and test cleanup. Adds `make lint` to the `make release` target, updates release process documentation to reflect lint inclusion, and fixes unreachable code after `t.Fatal` in DHS scenario test. | [v1.5.4](docs/release_notes/v1.5.x/v1.5.4.md) |
| 1.5.3 | 2026-07-15 | Hotfix for binary release CD pipeline. The `release-binary.yml` workflow used a broad `bin/g8e-*` glob that matched non-binary files (checksums, signatures), causing incorrect checksum generation and duplicate asset uploads. Replaced with explicit platform-specific patterns. | [v1.5.3](docs/release_notes/v1.5.x/v1.5.3.md) |
| 1.5.2 | 2026-07-15 | Onboarding wizard, HTTPS port split fix, and test mode cleanup. Adds `--interactive`/`-i` flag to `g8e gw start` launching a Bubble Tea TUI wizard with 4-step configuration flow. Splits `--endpoint`/`--port` into independent HTTP discovery and HTTPS/mTLS overrides to fix auth re-enroll in Docker demos. Removes `VaultRequireUnlock` config and `testMode` from gateway service builder. Moves `internal/netutil` to `internal/services/network`. Fixes vault and rate-limit settings propagation through `GatewayOptions`. Adds `gui_enrollment.md` guide. Fixes cosign binary signing CI permissions. | [v1.5.2](docs/release_notes/v1.5.x/v1.5.2.md) |
| 1.5.1 | 2026-07-14 | File service abstraction and internal refactoring. Introduces `RuntimeFileService` (`internal/services/fs`) to replace direct `os.*` calls across gateway, keystore, PKI, secret manager, and CLI. Refactors `SecretManager` and `Keystore` constructors to accept `RuntimeFileService`. Removes duplicate GitHub Release step from Python protocol workflow. Bumps `golang.org/x/crypto`, `golang.org/x/sys`, and `pkg/sftp` dependencies. | [v1.5.1](docs/release_notes/v1.5.x/v1.5.1.md) |
| 1.5.0 | 2026-07-13 | Go protocol module merge, CI modernization, and protocol updates. Merges the separate `github.com/g8e-ai/g8e/protocol` Go module into the root module. Renames `OperatorIntent*` events to `OperatorNotary*`, changes timestamp format to fixed microsecond precision, adds new CLI flags (`--tribunal-bootstrap`, `--public-base-url`, `--cors-origin`, `--passkey-rp-*`), changes MCP gateway tool discovery to proxied passthrough, adds cross-OS CI matrix (ubuntu/macOS/Windows) with cross-platform test compatibility fixes, Python pytest suite (94 tests), protocol conformance suite (151 tests), Go performance benchmarks (13 benchmarks), smoke test scripts, gitleaks secret scanning, go-licenses license compliance, cosign/sigstore artifact signing, fresh-install verification jobs, pip-audit dependency scanning, and py.typed PEP 561 marker. | [v1.5.0](docs/release_notes/v1.5.x/v1.5.0.md) |

## v1.4.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| 1.4.1 | 2026-07-12 | Release tooling and CI automation. Introduces `make release` / `make release-tag` workflow for unified version syncing, testing, building, and tagging. Adds binary release GitHub workflow and CI version sync verification. | [v1.4.1](docs/release_notes/v1.4.x/v1.4.1.md) |
| 1.4.0 | 2026-07-12 | Frontend enrollment, Cloudflare Tunnel, L5 Actuator refactor, and Swagger annotations. Adds `g8e gui` and `g8e tunnel` command suites, `operator start` rename, default TUI launch, consolidated web-cert trust scripts, OpenAPI annotations across gateway endpoints, and Windows build fixes. | [v1.4.0](docs/release_notes/v1.4.x/v1.4.0.md) |

## v1.3.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| 1.3.11 | 2026-07-10 | Test coverage, CLI testability, and Docker E2E reliability. Raises coverage threshold to 75%, adds dependency injection across CLI commands, shared Docker E2E fixture via TestMain, BuildKit cache mounts, g8e.local network alias, env var resolution for gateway flags, and removes demos/airgap.sh in favor of Go CLI commands. | [v1.3.11](docs/release_notes/v1.3.x/v1.3.11.md) |
| 1.3.10 | 2026-07-10 | CORS support, build hardening, and protocol cleanup. Adds configurable cross-origin browser access, unified build flags, UPX compression target, and WebAuthn assertion response fix. | [v1.3.10](docs/release_notes/v1.3.x/v1.3.10.md) |
| 1.3.9 | 2026-07-09 | Code quality, test coverage, and reliability. Renames entry point to `cmd/g8e`, adds `--public-base-url` flag, health-check-based process startup, and significantly expands test coverage. | [v1.3.9](docs/release_notes/v1.3.x/v1.3.9.md) |
| 1.3.8 | 2026-07-08 | Security and usability. One-time enrollment tokens, gateway foreground mode, live swarm demo, PATH setup in setup scripts, and test suite hardening. | [v1.3.8](docs/release_notes/v1.3.x/v1.3.8.md) |
| 1.3.7 | 2026-07-06 | L3 approval reliability, thread safety, and architectural refactoring. SSE-based approval notifications, atomic pointers for late-bound deps, two-phase MCP gateway dependency model. | [v1.3.7](docs/release_notes/v1.3.x/v1.3.7.md) |
| 1.3.6 | 2026-07-03 | Code quality, test coverage, and structural consolidation. Moves governance types to `internal/`, replaces `os.Exit` with error returns, context-based version propagation, 73% test coverage. | [v1.3.6](docs/release_notes/v1.3.x/v1.3.6.md) |
| 1.3.5 | 2026-07-02 | Air-gap readiness and demo integrity. Vendored Go modules for zero-network builds, pinned Docker image digests, dedicated demo Dockerfile, and air-gap CLI commands (`g8e demos export/import/images`). | [v1.3.5](docs/release_notes/v1.3.x/v1.3.5.md) |
| 1.3.4 | 2026-06-30 | TUI, onboarding UX, and demo integrity. Real-time TUI console, SSE-based passkey enrollment, typed route auth registry, and completion of demo integrity initiative. | [v1.3.4](docs/release_notes/v1.3.x/v1.3.4.md) |
| 1.3.3 | 2026-06-29 | Demos infrastructure and passkey configuration. Injectable WebAuthn RP origins for Docker, dedicated mTLS endpoint for pending approvals, reusable demo scenario pattern. | [v1.3.3](docs/release_notes/v1.3.x/v1.3.3.md) |
| 1.3.2 | 2026-06-28 | DHS Persistent Sovereign Capability demo with five real-governance scenarios, declarative tribunal bootstrap, and seed-based ensemble key generation. | [v1.3.2](docs/release_notes/v1.3.x/v1.3.2.md) |
| 1.3.1 | 2026-06-28 | Backward-compatibility cruft cleanup. Removes legacy code paths, DB migration helpers, CLI hidden aliases, MCP per-method REST routes, and WebAuthn fallbacks. | [v1.3.1](docs/release_notes/v1.3.x/v1.3.1.md) |
| 1.3.0 | 2026-06-27 | Layered authorization and architectural remediation. Two-layer L3 Notary model, three purpose-specific notary types, PasskeyService domain separation, and L5 Actuator cleanup. | [v1.3.0](docs/release_notes/v1.3.x/v1.3.0.md) |

## v1.2.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| 1.2.4 | 2026-06-26 | Console UX and passkey bootstrap refinement. Browser-facing SSE audit stream, automatic user creation during passkey bootstrap, Windows Hello hardening, and Swagger regeneration. | [v1.2.4](docs/release_notes/v1.2.x/v1.2.4.md) |
| 1.2.3 | 2026-06-26 | Passkey architecture consolidation and dependency reduction. Moves 15 passkey handlers to PasskeyService, eliminates `swaggo/swag` and `google/uuid` deps, adds token bucket rate limiter. | [v1.2.3](docs/release_notes/v1.2.x/v1.2.3.md) |
| 1.2.2 | 2026-06-25 | Security hardening, testability, and documentation consolidation. PrivilegedRouteRegistry, private IP allowlist for MCP probing, shell safety improvements, and CLI DI refactoring. | [v1.2.2](docs/release_notes/v1.2.x/v1.2.2.md) |
| 1.2.1 | 2026-06-25 | Maintenance and stability. Fixes routing, authentication, and UX bugs from the v1.2.0 Console release. | [v1.2.1](docs/release_notes/v1.2.x/v1.2.1.md) |
| 1.2.0 | 2026-06-24 | Major feature release introducing the g8e Console SPA. Zero-dependency single-page web app, dual-auth browser passkey endpoints, and L7 hybrid TLS/mTLS enforcement. | [v1.2.0](docs/release_notes/v1.2.x/v1.2.0.md) |

## v1.1.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| 1.1.9 | 2026-06-23 | Code review and cleanup. Completes partially implemented subsystems, fills architectural gaps in L5 Actuator zero standing privileges and state tiering, and fixes CLI L3 notary verification. | [v1.1.9](docs/release_notes/v1.1.x/v1.1.9.md) |
| 1.1.8 | 2026-06-23 | Maintenance and stability. Hardens Tribunal consensus infrastructure, reorganizes development tooling, and improves E2E integration test reliability. | [v1.1.8](docs/release_notes/v1.1.x/v1.1.8.md) |
| 1.1.7 | 2026-06-23 | Introduces the Tribunal system, replacing gateway L2 self-signing with a deliberation-based consensus model. Removes deprecated `gateway_signed` field from the protocol. | [v1.1.7](docs/release_notes/v1.1.x/v1.1.7.md) |
| 1.1.6 | 2026-06-22 | Reporting, compliance, and platform stability. Comprehensive reporting infrastructure, cross-platform process management, HTTPS enforcement, and SSH error handling improvements. | [v1.1.6](docs/release_notes/v1.1.x/v1.1.6.md) |
| 1.1.5 | 2026-06-20 | Code quality and test infrastructure. Error handling standardization, massive test coverage expansion, code simplification through interface dissolution, and test infrastructure cleanup. | [v1.1.5](docs/release_notes/v1.1.x/v1.1.5.md) |
| 1.1.4 | 2026-06-19 | Code quality and test coverage. MCP tool test refactoring, integration test improvements, database error handling cleanup, and cross-platform path handling. | [v1.1.4](docs/release_notes/v1.1.x/v1.1.4.md) |
| 1.1.3 | 2026-06-17 | Governed migration command suite, auth enrollment flow enhancements, and storage reliability improvements. | [v1.1.3](docs/release_notes/v1.1.x/v1.1.3.md) |
| 1.1.2 | 2026-06-16 | Maintenance release. Documentation cleanup, dependency updates, build infrastructure improvements, and removal of deprecated invitation service references. | [v1.1.2](docs/release_notes/v1.1.x/v1.1.2.md) |
| 1.1.1 | 2026-06-15 | Maintenance and stability. AuthController decomposition, MCP tool refactoring for testability, OpenAPI/Swagger docs, and critical data race fixes. | [v1.1.1](docs/release_notes/v1.1.x/v1.1.1.md) |
| 1.1.0 | 2026-06-12 | Major agentic tool expansion. 13 additional AI agent binaries, unified MCP CLI `agent` command, gateway service cleanup, and new position paper. | [v1.1.0](docs/release_notes/v1.1.x/v1.1.0.md) |

## v1.0.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| 1.0.12 | 2026-06-11 | Foundational refactoring. Security fix for governance bypass in `read_field`, CanonicalDBService extraction, auth caching, and documentation updates. | [v1.0.12](docs/release_notes/v1.0.x/v1.0.12.md) |
| 1.0.11 | 2026-06-10 | Multi-industry demo suite (Finance, Government, Healthcare), gateway/operator bootstrap hardening, secret manager, and expanded E2E testing. | [v1.0.11](docs/release_notes/v1.0.x/v1.0.11.md) |
| 1.0.10 | 2026-06-08 | Major release. Mandatory encryption at rest, consolidated 2-port gateway, WebAuthn passkey bootstrap, operator remote management commands, and native Windows compatibility. | [v1.0.10](docs/release_notes/v1.0.x/v1.0.10.md) |
| 1.0.9 | 2026-06-04 | Focused bug fix. Windows startup issues and gateway audit vault functionality fixes. | [v1.0.9](docs/release_notes/v1.0.x/v1.0.9.md) |
| 1.0.8 | 2026-06-02 | MCP native tool ecosystem expansion with 14 new tools, SSH streaming improvements, naming convention standardization, and security fixes. | [v1.0.8](docs/release_notes/v1.0.x/v1.0.8.md) |
| 1.0.7 | 2026-06-01 | Security hardening, performance optimizations, and 12 new MCP native tools for database, filesystem, network, process, and system monitoring. | [v1.0.7](docs/release_notes/v1.0.x/v1.0.7.md) |
| 1.0.6 | 2026-06-01 | Full Windows support, gateway network identity detection, foundational PKI architecture for gateway federation, and MCP-over-HTTP. | [v1.0.6](docs/release_notes/v1.0.x/v1.0.6.md) |
| 1.0.5 | 2026-05-31 | Version mismatch fix. VERSION file was not updated before the v1.0.4 release tag. | [v1.0.5](docs/release_notes/v1.0.x/v1.0.5.md) |
| 1.0.4 | 2026-05-31 | MCP stdio transport for local agent integration, complete Python protocol package, PKI/certificate hardening, and CLI reorganization. | [v1.0.4](docs/release_notes/v1.0.x/v1.0.4.md) |
| 1.0.3 | 2026-05-29 | Removes g8ee application-layer coupling. Dedicated controllers, CLI approval command for out-of-band L3 authorization, and security hardening. | [v1.0.3](docs/release_notes/v1.0.x/v1.0.3.md) |
| 1.0.2 | 2026-05-28 | TLS 1.3 enforcement, CSR-based enrollment, unified single-port multiplexing, and PKI revocation with fail-closed identity management. | [v1.0.2](docs/release_notes/v1.0.x/v1.0.2.md) |
| 1.0.1 | 2026-05-28 | JIT user provisioning by invitation, JWT-only authentication, API key deprecation and removal, and `insecure_mcp` rename. | [v1.0.1](docs/release_notes/v1.0.x/v1.0.1.md) |
| 1.0.0 | 2026-05-25 | Platform-first architecture. g8ee application layer excised, Sentinel dissolved into governance protocol, pure host-sovereign governance platform with fail-closed L1–L5 gates. | [v1.0.0](docs/release_notes/v1.0.x/v1.0.0.md) |

## v0.2.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| 0.2.7 | 2026-05-20 | Separates Gateway and Operator roles, introduces MCP & A2A protocol translator gateway, and removes external runtime dependencies (`git`, `jq`). | [v0.2.7](docs/release_notes/v0.2.x/v0.2.7.md) |
| 0.2.6 | 2026-05-19 | Intent classification as first-class architecture citizen and SPIFFE URI SAN refactoring for mTLS and workload identity. | [v0.2.6](docs/release_notes/v0.2.x/v0.2.6.md) |
| 0.2.5 | 2026-05-16 | CLI chat functionality, multi-ledger session-isolated Git audit ledgers, actuator execution boundary with signed receipts, and governance APIs. | [v0.2.5](docs/release_notes/v0.2.x/v0.2.5.md) |
| 0.2.4 | 2026-05-13 | Operator-owned PKI/TLS, CSR-based mTLS enrollment, BYO client support, and first-class CLI login. | [v0.2.4](docs/release_notes/v0.2.x/v0.2.4.md) |
| 0.2.3 | 2026-05-11 | Interactive platform manager menu for setup, environment configuration, and e2e testing. | [v0.2.3](docs/release_notes/v0.2.x/v0.2.3.md) |
| 0.2.2 | 2026-05-10 | Ollama model query during setup, runtime evals configuration, and host-native component testing without Docker. | [v0.2.2](docs/release_notes/v0.2.x/v0.2.2.md) |
| 0.2.1 | 2026-05-07 | Build system improvements for more reliable component container builds. | [v0.2.1](docs/release_notes/v0.2.x/v0.2.1.md) |
| 0.2.0 | 2026-05-07 | Protobuf-driven architecture, GovernanceEnvelope JSON transport, L1/L2/L3 governance hierarchy, and recursive grep tool. | [v0.2.0](docs/release_notes/v0.2.x/v0.2.0.md) |

## v0.1.x

| Version | Date | Description | Notes |
|---------|------|-------------|-------|
| 0.1.9 | 2026-05-05 | Acme Corp demo profile, blog post, and reorganized Nginx demo profile with regional deployments. | [v0.1.9](docs/release_notes/v0.1.x/v0.1.9.md) |
| 0.1.8 | 2026-05-04 | Batch tool execution with fan-out, improved evals suite, async sub-agents, and unified BatchRunner service. | [v0.1.8](docs/release_notes/v0.1.x/v0.1.8.md) |
| 0.1.7 | 2026-05-01 | Actuator reputation staking improvements and agent task cancellation with UI controls. | [v0.1.7](docs/release_notes/v0.1.x/v0.1.7.md) |
| 0.1.6 | 2026-04-29 | Information isolation round 2, multi-phase reputation commitment and stake resolution, and bug fixes. | [v0.1.6](docs/release_notes/v0.1.x/v0.1.6.md) |
| 0.1.5 | 2026-04-28 | Multi-stage reputation and staking system, SSH inventory streaming, enhanced test fixtures, and reputation CLI scripts. | [v0.1.5](docs/release_notes/v0.1.x/v0.1.5.md) |
| 0.1.4 | 2026-04-24 | Version bump to synchronize platform components after tagging conflict. | [v0.1.4](docs/release_notes/v0.1.x/v0.1.4.md) |
| 0.1.3 | 2026-04-24 | Global platform refactor for wire-contract stability, iteration-scoped AI message persistence, and background task tracking. | [v0.1.3](docs/release_notes/v0.1.x/v0.1.3.md) |
| 0.1.2 | 2026-04-20 | 5-member tribunal implementation, operator panel documentation, and operator panel test coverage. | [v0.1.2](docs/release_notes/v0.1.x/v0.1.2.md) |
| 0.1.1 | 2026-04-16 | UTCDatetime wire serialization, Pydantic native model_dump, and SSE event contract models. | [v0.1.1](docs/release_notes/v0.1.x/v0.1.1.md) |
| 0.1.0 | 2026-04-11 | Open-source release of the g8e platform. ReAct-based Python orchestration, dependency-free Go operator, and SQLite-backed persistence. | [v0.1.0](docs/release_notes/v0.1.x/v0.1.0.md) |
