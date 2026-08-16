# g8e

Python protocol library for the g8e zero-trust execution platform. Provides protocol constants, dynamic enums, and Pydantic models for building g8e-compatible clients and services.

## Installation

Requires Python 3.10 or later. Dependencies are `pydantic>=2.0.0` and `protobuf>=4.0.0`.

```bash
pip install g8e
```

## Usage

### Constants

The `g8e.constants` module loads JSON protocol constants from `protocol/constants/` at import time. It exports dicts for events, status, collections, headers, channels, pubsub, intents, prompts, timestamps, document IDs, platform configuration, agents, network, API paths, key-value keys, and sender identifiers. The module also exports the `ComponentName` `StrEnum` and individual HTTP header string constants for session, context, and g8e-specific headers.

Set the `G8E_PROTOCOL_DIR` environment variable to override the default protocol directory resolution. The loader checks this variable first, then the bundled `g8e/_data/` directory included in PyPI installs, then the relative path from the package for dev mode, then `/app/protocol/constants` for containerized environments, and finally `./protocol/constants` as a last resort.

#### Accessor Functions

The module provides typed accessor functions that resolve constant keys to their wire values:

- **`collection(name)`**: collection wire value (e.g. `collection("cases")`)
- **`channel(name)`**: channel wire value
- **`intent(name)`**: intent wire value
- **`prompt(name)`**: prompt section wire value
- **`kv_key(name, **kwargs)`**: formatted KV key with placeholder substitution (e.g. `kv_key("SessionWeb", **{"session.type": "web", "session.id": "abc"})`)
- **`kv_session_type(name)`**: session type wire value

The `kv_key()` function uses regex substitution to handle dotted placeholder names like `{session.type}` in KV key templates.

### Enums

The `g8e.enums` module dynamically generates `StrEnum` and `IntEnum` classes from the `STATUS` and `EVENTS` protocol constants. Enum member names use SCREAMING_SNAKE_CASE; values preserve the raw protocol wire format. Integer-valued categories produce `IntEnum`; all others produce `StrEnum`. Access enums by PascalCase name via attribute lookup, for example `g8e.enums.OperatorToolName` or `g8e.enums.EventType`.

In addition to STATUS-derived enums, the module generates enums from other constant categories:

- **`g8e.enums.Channel`**: from `channels.json`
- **`g8e.enums.Intent`**: from `intents.json`
- **`g8e.enums.Prompt`**: from `prompts.json`
- **`g8e.enums.Collection`**: from `collections.json`
- **`g8e.enums.KVKey`**: from `kv_keys.json`

### Models

The `g8e.models` package provides Pydantic v2 models for protocol data structures. All models extend `G8eBaseModel`, which configures `populate_by_name` and `extra="ignore"`, and defaults `exclude_none` on serialization. The `UTCDatetime` annotated type serializes datetimes to ISO 8601 with a `Z` suffix.

- **`g8e/models/base.py`**: `G8eBaseModel`, `UTCDatetime`, and re-exports of Pydantic `Field`, `ConfigDict`, `field_validator`, `model_validator`, and `ValidationError`.
- **`g8e/models/context.py`**: `RequestContext` and `BoundOperator`. `RequestContext` validates session identity for `CLIENT` source components, requiring either `web_session_id` or `cli_session_id` and a `user_id`.
- **`g8e/models/events.py`**: SSE wire models (`SessionEventWire`, `BackgroundEventWire`) with factory methods `from_session_event()` and `from_background_event()` for construction from event type + data. AI event payload models for chat processing, processing stopped, response chunks, response completion, tool lifecycle, citations, errors, thinking, turn completion, retry, and triage clarification.
- **`g8e/models/internal_api.py`**: `ChatMessageRequest`, `ChatStartedResponse`, and `ResourceCreationRequest` for internal API interactions. `ChatMessageRequest` inherits from `LLMOverrides`, which provides 12 optional LLM provider/model/endpoint override fields.
- **`g8e/models/settings.py`**: `G8eeUserSettings`, `PlatformSettings`, and nested settings models for LLM providers, search, eval judge, command validation, and batch execution.
- **`g8e/models/governance.py`**: `GovernanceEnvelope`, `GovernanceMetadata`, `GovernanceL1`, `GovernanceL2`, `GovernanceL2Vote`, `GovernanceL3`, `GovernanceL3Proof`, and `compute_transaction_hash()`. The `GovernanceEnvelope` model mirrors the canonical wire format for all mutations. `compute_transaction_hash()` produces a deterministic SHA-256 over pipe-delimited canonical fields.

### Examples

Working examples are in `protocol/python/examples/`. Run `constants_example.py` for constants and headers usage, or `models_example.py` for model instantiation, serialization, and validation.

## Components

- **`g8e/constants.py`**: Runtime loader for JSON protocol constants from `protocol/constants/`. Exports dict constants, `ComponentName` enum, HTTP header string constants, and accessor functions (`collection()`, `channel()`, `intent()`, `prompt()`, `kv_key()`, `kv_session_type()`).
- **`g8e/enums.py`**: Dynamic `StrEnum` and `IntEnum` generation from `STATUS`, `EVENTS`, and other constant categories (channels, intents, prompts, collections, kv_keys).
- **`g8e/models/`**: Pydantic v2 models for protocol data structures, SSE events, internal API requests, user settings, and governance envelopes.

## Protocol Versioning

This package follows semantic versioning. Major version changes indicate breaking protocol changes. Minor version changes add new protocol features. Patch version changes include bug fixes and non-breaking enhancements.

## License

Apache License 2.0. See `protocol/LICENSE` for details.

## Contributing

Protocol changes require coordination across all g8e components. Submit protocol change proposals via GitHub issues with clear justification and impact analysis.

## Support

For protocol questions and support, open a GitHub issue or visit https://github.com/g8e-ai/g8e
