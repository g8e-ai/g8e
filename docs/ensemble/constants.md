# Constants

## Overview

g8ee uses a structured constants system for configuration, action type mappings, agent definitions, and protocol values. Constants are defined in `app/constants/` and are the single source of truth for configurable values.

Protocol constants are sourced from the `g8e` package (`g8e>=1.5.6`) via `g8e.constants` accessors. g8ee-specific constants that have no g8e equivalent are defined locally.

## g8e-Sourced Constants

The following constant categories source their shared values from `g8e.constants`:

- **DB Collections** (`app/constants/collections.py`) — 17 collection names via `g8e.constants.collection()`; 2 g8ee-specific (`api_keys`, `tribunal_commands`)
- **KV Keys** (`app/constants/kv_keys.py`) — 23 key patterns via `g8e.constants.kv_key()`; 2 g8ee-specific (`cli_session`, `operator_slot_counter`). **Note:** KV keys use `sessions` (plural) per g8e protocol, replacing the old `session` (singular) convention.
- **Channels** (`app/constants/channels.py`) — 9 channel values via `g8e.constants.channel()`; 7 g8ee-specific
- **Intents** (`app/constants/intents.py`) — All 52 `CloudIntent` values via `g8e.constants.intent()`
- **Prompts** (`app/constants/prompts.py`) — 14 `PromptSection` values and 3 `AgentMode` values via `g8e.constants.prompt()`; `SENTINEL_MODE` is g8ee-specific
- **API Paths** (`app/constants/api_paths.py`) — `GatewayAPIPaths` class wraps `g8e.constants.API_PATHS`; `InternalAPIPaths` for g8ee-internal routing via `api_paths.json`
- **Enums** (`app/constants/config.py`) — 12 enums re-exported from `g8e.enums` (`CloudSubtype`, `ConversationStatus`, `EscalationRisk`, `ExecutionStatus`, `FileOperation`, `HealthStatus`, `NetworkProtocol`, `AttachmentType`, `ToolDisplayCategory`, `ToolCallStatus`, `ThinkingActionType`, `ApprovalErrorType`); 3 g8ee-specific (`ApprovalType`, `InfrastructureStatus`, `AuthMethod`)
- **SessionType** (`app/constants/generated_status.py`) — Local `StrEnum` with `WEB`, `OPERATOR`, `CLI`; `CLI` is g8ee-specific (not in g8e)
- **EventType** (`app/constants/generated_status.py`) — Local `StrEnum` combining 288 g8e members + 8 g8ee-specific intent members using g8e's dotted naming convention

## Key Constant Categories

- **Action Type Mappings** — Maps action categories to protocol-recognized types
- **Agents** — Agent persona definitions and configurations
- **Protocol Constants** — Protocol version, envelope types, signature schemes
- **Provider Configuration** — LLM provider settings and feature flags
- **Governance Rules** — L1/L2/L3 rule definitions and thresholds

## Usage

Constants are imported directly from `app/constants/` and used throughout the codebase to ensure consistency and avoid magic values.

## Related

- [Protocol](protocol.md) — Protocol constants and action types
- [Agents](agents.md) — Agent definitions
- [LLM Providers](llm-providers.md) — Provider configuration constants
