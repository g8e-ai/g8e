# Protocol Conformance Tests

Cross-language conformance tests that validate protocol constants, models, and
transaction hashes are identical across the Python package and Go implementation.

## Architecture

The JSON files in `protocol/constants/` and `protocol/models/` are the
**single source of truth** (SSOT). Both the Go code (`internal/constants/`)
and the Python package (`g8e/constants.py`) load from these files.

The Go constants are manually maintained to mirror the JSON values, verified
by existing contract tests in `internal/constants/*_test.go`. The Python
constants load the JSON directly at runtime.

The shared `hash_vectors.json` file in this directory is a second SSOT,
specifying expected SHA-256 digests for transaction hash computation. It is
consumed by both `test_hash_parity.py` (Python's `compute_transaction_hash`)
and `internal/governance/envelope_hash_parity_test.go` (Go's
`GenerateMessageID`), ensuring both implementations produce identical hashes
for the same envelope fields.

## Test Files

### `test_constants.py`

Validates that the 22 protocol constant JSON files in `protocol/constants/`
have the required metadata fields for Go and Python code generation.

- **JSON structural integrity**: All 22 protocol constant files (19
  single-wrapper-key files, `status.json`, `api_paths.json`, and `auth.json`)
  load successfully and have the expected wrapper key structure with
  non-empty entries.
- **`_go_const` presence**: Every entry in every constant file (except
  `exit_codes.json`, which uses a different schema) has a `_go_const` field,
  ensuring Go code can mirror it. `status.json` entries are verified via
  dedicated nested-category tests, and `auth.json` is verified via dedicated
  multi-wrapper tests.
- **`_python_const` presence**: Every entry in `status.json` and in the
  applicable wrapper keys of `channels.json`, `intents.json`, `prompts.json`,
  `collections.json`, `kv_keys.json` (including `session_types`), `agents.json`
  (including `agent_binaries` and the four `triage_*` categories), and
  `field_paths.json` has a `_python_const` field, ensuring Python enum
  generation works.
- **`_python_const` naming conventions**: All `_python_const` values in
  `status.json` and the applicable files above are valid
  `SCREAMING_SNAKE_CASE` identifiers.
- **Value uniqueness**: No duplicate wire-format values within any single
  file (except `events.json`, which allows aliases). For `status.json`,
  uniqueness is enforced per category except the `ai_task_id` category, which
  contains intentional alias entries.
- **Value field presence**: Every entry has a non-empty `value` field (except
  `field_paths.json`, which uses `allowed_paths`/`forbidden_paths`).
- **Event namespace convention**: All event values follow `g8e.v1.*` and
  contain dots.
- **Go naming conventions**: `_go_const` values follow PascalCase (with
  dot notation for nested constants like `Ports.OperatorHttp`). Event
  constants are prefixed with `Event`, header constants with `Header`, and
  collection constants with `Collection`.
- **Python-JSON parity**: Python-loaded constants (`EVENTS`, `STATUS`,
  `HEADERS`, `COLLECTIONS`) match the raw JSON values exactly (count, keys,
  and values).
- **Mutation flag types**: `_mutation` flags in the `action_type` category of
  `status.json` are boolean.
- **`auth.json` multi-wrapper structure**: All seven wrapper keys
  (`passkey_purposes`, `webauthn_types`, `webauthn_algorithms`,
  `webauthn_attestation`, `webauthn_requirements`, `pki_leaf_types`,
  `context_keys`) exist, are non-empty, and their entries have `_go_const`
  and non-empty `value` fields. Value uniqueness is enforced per wrapper
  except `webauthn_requirements`, where `resident_key_required` and
  `user_verification_required` intentionally share the value `required`.

### `test_models.py`

Validates that Python Pydantic models in the `g8e` package produce JSON that
is structurally compatible with the canonical model schemas in
`protocol/models/`.

- **Model schema integrity**: All 56 model JSON schemas (50 in
  `protocol/models/` plus 6 agent definition files in `agents/`) load and
  have expected structure.
- **PlatformSettings field parity**: Python `PlatformSettings` model fields
  match the `platform_settings.json` schema definition (excluding
  `created_at`/`updated_at` metadata).
- **RequestContext validation**: Client component requires session and user
  IDs; non-client does not. Serialization round-trip and schema field parity
  with `request_context.json` are verified.
- **BoundOperator conformance**: Serialization round-trip and schema field
  parity with `request_context.json`.
- **Serialization round-trip**: Models serialize to JSON and parse back with
  correct field values.
- **Canonical serialization**: `G8eBaseModel` excludes `None` fields and
  ignores extra fields by default.
- **UserSettings conformance**: LLM, search, command validation, and batch
  execution settings fields match the `user_settings.json` schema (LLM and
  search account for Pydantic field aliases).
- **ChatMessageRequest conformance**: Round-trip serialization with and
  without resource creation, `ChatStartedResponse` round-trip, and schema
  field parity with `chat_message.json`.
- **Event payload round-trip**: All SSE event payload models
  (`AiProcessingStoppedPayload`, `AIToolLifecyclePayload`,
  `ChatErrorPayload`, `ChatProcessingStartedPayload`,
  `ChatResponseChunkPayload`, `ChatResponseCompletePayload`,
  `ChatRetryPayload`, `ChatThinkingPayload`, `ChatTurnCompletePayload`,
  `TriageClarificationQuestionsPayload`, `SessionEventWire`,
  `BackgroundEventWire`) serialize and parse back correctly.
- **Zero-PII verification**: `user.json` schema contains no `email`, `name`,
  or `password_hash` fields.
- **Schema cross-reference**: All `_ref` pointers in model schemas resolve
  to defined model sections (both local `#section` and cross-file
  `file.json#section` references).
- **Governance conformance**: `GovernanceL1`, `GovernanceL2Vote`,
  `GovernanceL2`, `GovernanceL3`/`GovernanceL3Proof`, `GovernanceMetadata`,
  and `GovernanceEnvelope` Python models match `governance.json`. Verified
  via round-trip serialization, default-value checks, boolean/string-list
  type conformance, `protocol_version` default handling, and schema field
  parity for each model section.

### `test_hash_parity.py`

Validates that Python's `compute_transaction_hash` produces identical
SHA-256 digests to Go's `GenerateMessageID` for the same governance envelope
fields. Test vectors are loaded from the shared `hash_vectors.json` file,
which is the single source of truth for expected hash outputs.

- **Vector parity**: Each of the 8 vectors in `hash_vectors.json` (standard,
  nested_intent, unicode, empty_payload, empty_intent, optional_omitted,
  timestamp_no_fractional, timestamp_with_fractional) produces the expected
  SHA-256 hex digest.
- **Timestamp normalization**: Timestamps with and without fractional
  seconds produce the same hash.
- **Optional field handling**: `None` and empty string for optional fields
  (`requestor_user_id`, `acting_app_id`) produce the same hash (both
  omitted from the canonical form).
- **Determinism**: Same inputs always produce the same 64-character hex
  digest.
- **Type safety**: `_canonicalize_value` rejects unsupported types with
  `TypeError` rather than silently producing mismatched output.

## Running

```bash
# From the repository root
cd protocol/python && pip install -e ".[dev]" && cd ../..
python -m pytest protocol/conformance/ -v
```

## CI Integration

A `conformance` job in `.github/workflows/build-and-test.yml` runs these
tests on every PR using Python 3.14. The job installs the `g8e` package in
editable mode with dev extras, then runs `python -m pytest protocol/conformance/ -v`.
