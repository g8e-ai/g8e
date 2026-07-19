# Protocol Conformance Tests

Cross-language conformance tests that validate protocol constants and models
are identical across the Python package and Go implementation.

## Architecture

The JSON files in `protocol/constants/` and `protocol/models/` are the
**single source of truth** (SSOT). Both the Go code (`internal/constants/`)
and the Python package (`g8e/constants.py`) load from these files.

The Go constants are manually maintained to mirror the JSON values, verified
by existing contract tests in `internal/constants/*_test.go`. The Python
constants load the JSON directly at runtime.

## What These Tests Verify

### `test_constants.py`

- **JSON structural integrity**: All 22 protocol constant files load
  successfully and have the expected wrapper key structure.
- **`_go_const` presence**: Every entry in every constant file (except
  `exit_codes.json`, which uses a different schema) has a `_go_const`
  field, ensuring Go code can mirror it. `auth.json` is verified via
  dedicated multi-wrapper tests.
- **`_python_const` presence**: Every entry in `status.json` has a
  `_python_const` field, ensuring Python enum generation works.
- **Value uniqueness**: No duplicate wire-format values within any single
  file (except `events.json`, which allows aliases).
- **Value field presence**: Every entry has a non-empty `value` field
  (except `field_paths.json`, which uses `allowed_paths`/`forbidden_paths`).
- **Event namespace convention**: All event values follow `g8e.v1.*`.
- **Go naming conventions**: `_go_const` values follow PascalCase with
  consistent prefixes (`Event*`, `Header*`, `Collection*`).
- **Python-JSON parity**: Python-loaded constants match the raw JSON values
  exactly (count, keys, and values).
- **Mutation flag types**: `_mutation` flags in `status.json` are boolean.

### `test_models.py`

- **Model schema integrity**: All 55 model JSON schemas (including 6
  agent definition files in `agents/`) load and have expected structure.
- **PlatformSettings field parity**: Python `PlatformSettings` model fields
  match the JSON schema definition.
- **RequestContext validation**: Client component requires session and user
  IDs; non-client does not.
- **BoundOperator conformance**: Serialization round-trip and schema field
  parity with `request_context.json`.
- **Serialization round-trip**: Models serialize to JSON and parse back with
  correct field values.
- **Canonical serialization**: `G8eBaseModel` excludes `None` fields and
  ignores extra fields by default.
- **UserSettings conformance**: LLM, search, command validation, and batch
  execution settings fields match the JSON schema.
- **ChatMessageRequest conformance**: Round-trip serialization with and
  without resource creation, and schema field parity.
- **Event payload round-trip**: All SSE event payload models serialize and
  parse back correctly.
- **Zero-PII verification**: `user.json` schema contains no `email`, `name`,
  or `password_hash` fields.
- **Schema cross-reference**: All `_ref` pointers in model schemas resolve
  to defined model sections.

## Running

```bash
# From the repository root
cd protocol/python && pip install -e ".[dev]" && cd ../..
python -m pytest protocol/conformance/ -v

# Or via the conformance CI job
```

## CI Integration

A `conformance` job in `.github/workflows/build-and-test.yml` runs these
tests on every PR using Python 3.14.
