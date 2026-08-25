# Changelog

All notable changes to the g8ee ensemble are documented in this file. Platform-wide changes are documented in the repo-root [release notes](../docs/release_notes/).

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Modern Python project structure setup
- Environment variable template (.env.example)
- Contribution guidelines (CONTRIBUTING.md)
- Pre-commit hooks configuration
- Development dependencies as extras in pyproject.toml
- `make proto` target for regenerating protobuf stubs from g8e protocol definitions
- `app/proto/` directory with generated protobuf stubs (`common_pb2.py`, `operator_pb2.py`, `pubsub_pb2.py`)
- `GatewayAPIPaths` class wrapping `g8e.constants.API_PATHS` for Gateway-side API path constants
- `PromptSection.VAULT_MODE` section from g8e protocol
- Phase 5 regression tests (`test_internal_api_g8e_base.py` — 35 tests)
- Phase 14 regression tests (`test_governance_alignment.py` — 19 tests)
- 218 regression tests across 6 files for Phases 7–11 and 15 (all passing)

### Changed
- Migrated to use the in-tree `protocol/python/` g8e package as single source of truth for protocol constants, enums, and models (path dependency via `[tool.uv.sources]` in `pyproject.toml`; the Dockerfile installs `protocol/python` before the ensemble so pip resolves `g8e` from the local package)
- `G8eBaseModel` now imported from `g8e.models.base` (re-exported via `app.models.base`)
- `RequestContext` subclasses `g8e.models.context.RequestContext`, adds `operator_id` and `operator_session_id`
- `BoundOperator` directly re-exported from `g8e.models.context`
- `ChatMessageRequest` uses multiple inheritance: g8e base + `RequestOverrides` mixin
- `ResourceCreationRequest` and `ChatStartedResponse` re-exported from `g8e.models.internal_api`
- Settings models subclassed from `g8e.models.settings`
- SSE wire models (`SessionEventWire`, `BackgroundEventWire`) subclass `g8e.models.events`; all 11 payload classes re-exported from g8e
- DB collection names sourced from `g8e.constants.collection()` (17 shared collections)
- KV key patterns sourced from `g8e.constants.kv_key()` (23 shared key patterns)
- Channel values aligned with `g8e.constants.channel()` (9 shared channels)
- Intent enum values sourced from `g8e.constants.intent()` (all 52 `CloudIntent` values)
- Prompt section/mode values sourced from `g8e.constants.prompt()` (14 `PromptSection` + 3 `AgentMode`)
- API paths aligned with `g8e.constants.API_PATHS` via `GatewayAPIPaths` class
- All `from pydantic import` in `app/` routed through `app.models.base` re-exports (30 files)
- 12 duplicate enum definitions in `config.py` replaced with re-exports from `g8e.enums`
- Replaced hardcoded `/api/v1/governance/envelopes` URL in `governance_client.py` with `GatewayAPIPaths.GOVERNANCE_ENVELOPES` constant
- Added 4 missing client paths to `api_paths.json` (`sse_push`, `grant_intent`, `revoke_intent`, `create_operator_link`)
- `PubSubGovernanceClient.submit_envelope()` fixed to serialize pre-built envelope dict directly (was incorrectly calling `build_uap_envelope_json` on a dict)

### Removed
- Vendored `vendor/g8e` git submodule (now installed from the in-tree `protocol/python/` package)
- 4 redundant `model_config` overrides that duplicated g8e base config

### Security
- All business-critical DB writes (cases, investigations, memories, reputation) go through `GovernanceClient` governance envelopes with mTLS
- Transaction hash uses SHA256 of canonical JSON (sorted keys, no whitespace) for replay defense

## [0.2.6] - 2026-05-25

### Added
- Initial g8ee implementation
- FastAPI-based HTTP server
- LLM provider integrations (OpenAI, Anthropic, Gemini, Ollama)
- Operator service clients (DB, KV, PubSub, Blob, HTTP)
- Command validation with whitelist/blacklist support
- Auto-approval system for trusted commands
- Reputation staking system
- Evaluation suite and benchmarks
- TLS/mTLS support
- Session management
- User authentication with passkeys

### Changed
- Migrated to pyproject.toml for project configuration
- Added Ruff for linting and formatting
- Added Pyright for type checking

### Security
- Added session encryption
- Added auditor HMAC key for reputation commitments
- TLS certificate validation for operator connections

---

## Version Format

- **Added**: New features
- **Changed**: Changes in existing functionality
- **Deprecated**: Soon-to-be removed features
- **Removed**: Removed features
- **Fixed**: Bug fixes
- **Security**: Security vulnerabilities or fixes
