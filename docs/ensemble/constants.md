# Constants

## Overview

g8ee uses a structured constants system for configuration, action type mappings, agent definitions, and protocol values. Constants are defined in `app/constants/` and serve as the single source of truth for configurable values.

Protocol constants and enums are sourced directly from the `g8e` package (`g8e>=1.5.6`) via `g8e.constants` accessors and `g8e.enums` re-exports. g8ee-specific constants and enums that have no g8e equivalent are defined locally.

## g8e-Sourced Constants and Enums

The following constant categories source their shared values from `g8e.constants` and `g8e.enums`:

- **DB Collections** (`app/constants/collections.py`) — 17 collection names via `g8e.constants.collection()`; 2 g8ee-specific collections (`api_keys`, `tribunal_commands`); document IDs via `g8e.constants.document_id()` (`platform_settings`, `user_settings_prefix`).
- **KV Keys** (`app/constants/kv_keys.py`) — 23 key patterns via `g8e.constants.kv_key()`; 2 g8ee-specific keys (`cli_session`, `operator_slot_counter`); static prefixes derived via `KVKeyPrefix`. Key patterns use `sessions` (plural) per protocol convention.
- **Channels** (`app/constants/channels.py`) — 9 channel values via `g8e.constants.channel()`; 7 g8ee-specific channel values (`cmd`, `results`, `heartbeat`, `g8eo_results`, `operator_heartbeats`, `sse_events`, `system_events`); pubsub enums (`PubSubChannel`, `PubSubAuthPrefix`, `PubSubAction`, `PubSubWireEventType`, `PubSubField`, `PubSubMessageType`).
- **Intents** (`app/constants/intents.py`) — All 52 `CloudIntent` values via `g8e.constants.intent()`, along with intent dependency mappings (`CLOUD_INTENT_DEPENDENCIES`), confirmation prompts (`CLOUD_INTENT_QUESTIONS`), and IAM verification actions (`CLOUD_INTENT_VERIFICATION_ACTIONS`).
- **Prompts** (`app/constants/prompts.py`) — 14 `PromptSection` values and 3 `AgentMode` values via `g8e.constants.prompt()`; 1 g8ee-specific section (`SENTINEL_MODE`); prompt file path mappings (`PromptFile`, `AGENT_MODE_PROMPT_FILES`); UI context labels (`InvestigationContextLabel`).
- **API Paths** (`app/constants/api_paths.py`) — `GatewayAPIPaths` wraps gateway route constants from `g8e.constants.API_PATHS`; `InternalAPIPaths` provides typed access for g8ee-internal and client routing defined in `api_paths.json`.
- **Protocol Enums** (`app/constants/generated_status.py`, `app/constants/config.py`, `app/constants/errors.py`, `app/constants/platform.py`) — Enums re-exported directly from `g8e.enums`, including `EventType` (297 members), `SessionType` (`WEB`, `OPERATOR`, `CLI`, `APP`), `ErrorCode`, `ErrorCategory`, `ErrorSeverity`, `AuthMethod`, `CloudSubtype`, `ConversationStatus`, `EscalationRisk`, `ExecutionStatus`, `FileOperation`, `HealthStatus`, `InfrastructureStatus`, `NetworkProtocol`, `AttachmentType`, `ToolDisplayCategory`, `ToolCallStatus`, `ThinkingActionType`, `ApprovalErrorType`, and `ApprovalType`.
- **OperatorToolName** (`app/constants/generated_status.py`) — Re-exports 19 protocol tool names from `g8e.enums.OperatorToolName` extended with 2 ensemble-specific tool identifiers (`GRANT_INTENT`, `REVOKE_INTENT`).
- **HTTP Headers** (`app/constants/__init__.py`) — 32 canonical headers sourced from `g8e.constants` (such as `AUTHORIZATION`, `CASE_ID`, `INVESTIGATION_ID`, `SOURCE_COMPONENT`, `SYSTEM_FINGERPRINT`); 3 ensemble-local reverse-proxy headers (`X_PROXY_USER_EMAIL`, `X_PROXY_CLI_SESSION_ID`, `X_PROXY_WEB_SESSION_ID`).
- **Component Attribution** (`app/constants/generated_status.py`, `app/constants/config.py`) — `ComponentName` imported from `g8e.constants` (`CLIENT`, `G8EO`, `G8EO_GATEWAY`); `G8EE_COMPONENT = "g8ee"` defined locally for outbound attribution.

## Key Constant Categories

- **Action Type Mappings** (`app/constants/action_type_mappings.py`) — Bidirectional mapping between protobuf event types and Unified Action Protocol (UAP) action types and result types (`map_event_type_to_action_type`, `map_action_type_to_event_type`, `map_event_type_to_result_action_type`).
- **Agent Definitions** (`app/models/personas/`) — Persona models and runtime registry inheriting from `AgentPersonaModel`. `app/constants/agents.py` is reserved for legacy compatibility.
- **Provider and Generation Configuration** (`app/constants/config.py`) — LLM provider settings (`LLMProvider`), thinking intensity levels (`ThinkingLevel`, `ThinkingDialect`), default model identifiers, timeouts, token limits, truncation thresholds, scrubber patterns (`ScrubType`), and security constraints.
- **Paths and Ports** (`app/constants/paths.py`, `app/constants/generated_paths.py`) — Canonical port constants (`PortConstants`), document directory paths (`PathConstants`), and dynamic runtime path resolution (`PATHS` proxy, `get_paths()`, `reload_paths()`, `get_app_cert_paths()`).
- **Environment Variables** (`app/constants/env_vars.py`) — Canonical environment variable names encapsulated in the `EnvVar` class.
- **Message Senders** (`app/constants/message_sender.py`) — Database message origin identifiers (`MessageSender`) for conversation history persistence and display.

## Usage

Constants are imported directly from `app.constants` and used throughout the codebase to ensure consistency and eliminate magic strings.

## Related

- [Protocol](protocol.md) — Protocol constants and action types
- [Agents](agents.md) — Agent definitions and persona models
- [LLM Providers](llm-providers.md) — Provider configuration constants
- [Prompts](prompts.md) — Prompt templates and assembly constants
