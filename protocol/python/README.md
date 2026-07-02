# g8e-protocol

Python protocol library for the g8e zero-trust execution platform. Provides protocol constants, dynamic enums, and Pydantic models for building g8e-compatible clients and services.

## Installation

Requires Python 3.10 or later. Dependencies are `pydantic>=2.0.0` and `protobuf>=4.0.0`.

```bash
pip install g8e-protocol
```

## Usage

### Constants

The `g8e.constants` module loads JSON protocol constants from `protocol/constants/` at import time. It exports dicts for events, status, collections, headers, channels, pubsub, intents, prompts, timestamps, document IDs, platform configuration, agents, network, API paths, key-value keys, and sender identifiers. The module also exports the `ComponentName` `StrEnum` and individual HTTP header string constants for session, context, and g8e-specific headers.

Set the `G8E_PROTOCOL_DIR` environment variable to override the default protocol directory resolution. The loader checks this variable first, then falls back to the relative path from the package, then to `/app/protocol/constants` for containerized environments.

### Enums

The `g8e.enums` module dynamically generates `StrEnum` and `IntEnum` classes from the `STATUS` and `EVENTS` protocol constants. Enum member names use SCREAMING_SNAKE_CASE; values preserve the raw protocol wire format. Integer-valued categories produce `IntEnum`; all others produce `StrEnum`. Access enums by PascalCase name via attribute lookup, for example `g8e.enums.OperatorToolName` or `g8e.enums.EventType`.

### Models

The `g8e.models` package provides Pydantic v2 models for protocol data structures. All models extend `G8eBaseModel`, which configures `populate_by_name` and `extra="ignore"`, and defaults `exclude_none` on serialization. The `UTCDatetime` annotated type serializes datetimes to ISO 8601 with a `Z` suffix.

- **`g8e/models/base.py`**: `G8eBaseModel`, `UTCDatetime`, and re-exports of Pydantic `Field`, `ConfigDict`, `field_validator`, `model_validator`.
- **`g8e/models/context.py`**: `RequestContext` and `BoundOperator`. `RequestContext` validates session identity for `CLIENT` source components, requiring either `web_session_id` or `cli_session_id` and a `user_id`.
- **`g8e/models/events.py`**: SSE wire models (`SessionEventWire`, `BackgroundEventWire`) and AI event payload models for chat processing, tool lifecycle, citations, errors, thinking, turn completion, retry, and triage clarification.
- **`g8e/models/internal_api.py`**: `ChatMessageRequest`, `ChatStartedResponse`, and `ResourceCreationRequest` for internal API interactions.
- **`g8e/models/settings.py`**: `G8eeUserSettings`, `PlatformSettings`, and nested settings models for LLM providers, search, eval judge, command validation, and batch execution.

### Examples

Working examples are in `protocol/python/examples/`. Run `constants_example.py` for constants and headers usage, or `models_example.py` for model instantiation, serialization, and validation.

## Components

- **`g8e/constants.py`**: Runtime loader for JSON protocol constants from `protocol/constants/`. Exports dict constants, `ComponentName` enum, and HTTP header string constants.
- **`g8e/enums.py`**: Dynamic `StrEnum` and `IntEnum` generation from `STATUS` and `EVENTS` protocol constants.
- **`g8e/models/`**: Pydantic v2 models for protocol data structures, SSE events, internal API requests, and user settings.

## Protocol Versioning

This package follows semantic versioning. Major version changes indicate breaking protocol changes. Minor version changes add new protocol features. Patch version changes include bug fixes and non-breaking enhancements.

## License

Apache License 2.0. See `protocol/LICENSE` for details.

## Contributing

Protocol changes require coordination across all g8e components. Submit protocol change proposals via GitHub issues with clear justification and impact analysis.

## Support

For protocol questions and support, open a GitHub issue or visit https://github.com/g8e-ai/g8e
