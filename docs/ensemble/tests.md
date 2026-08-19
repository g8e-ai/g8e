# Testing

## Overview

g8ee uses pytest with marker-based test categorization. Tests are organized into four suites aligned with the g8e 4-tier test model: unit (Tier 1), integration (Tier 2, in-process), external (Tier 4, real LLM/API), and e2e.

## Test Structure

```
tests/
├── unit/           # Tier 1 unit tests (no external dependencies)
├── integration/    # Tier 2 in-process integration + Tier 4 external tests
│                   # (Tier 4 tests carry the ai_integration / requires_web_search /
│                   #  requires_api markers and are skipped when credentials are absent)
├── e2e/            # End-to-end tests (full system)
└── fakes/          # Fake implementations for testing
```

## Running Tests

```bash
# Tier 1 + Tier 2 (unit + in-process integration; no external deps)
make ensemble-test

# Tier 4 (external; real LLM/API calls, gated on credentials)
make test-external

# Unit tests only (no external dependencies)
pytest tests/ -v -m "not ai_integration and not requires_web_search and not requires_api and not e2e"

# Eval tests
pytest evals/tests/ -v

# Full test suite
pytest tests/ evals/tests/ -v
```

## Test Markers

Tier 4 (External) markers — tests that depend on resources outside the platform's own infrastructure. These tests are skipped when the relevant credentials are absent:

- `ai_integration` — Tests requiring AI provider access
- `requires_web_search` — Tests requiring web search capability
- `requires_api` — Tests requiring external API access
- `e2e` — End-to-end tests

## Fakes

The `tests/fakes/` directory contains fake implementations of service clients, agents, and async helpers for use in unit and integration tests without external dependencies.

## Related

- [Development](devs.md) — Dev setup and coding standards
- [Evals](evals.md) — Evaluation suite and benchmarks
