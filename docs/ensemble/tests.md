# Testing

## Overview

g8ee uses pytest with marker-based test categorization. Tests are organized into unit, integration, and e2e suites.

## Test Structure

```
tests/
├── unit/           # Unit tests (no external dependencies)
├── integration/    # Integration tests (may require services)
├── e2e/            # End-to-end tests (full system)
└── fakes/          # Fake implementations for testing
```

## Running Tests

```bash
# Unit tests only (no external dependencies)
pytest tests/ -v -m "not ai_integration and not requires_web_search and not requires_api and not e2e"

# Eval tests
pytest evals/tests/ -v

# Full test suite
pytest tests/ evals/tests/ -v
```

## Test Markers

- `ai_integration` — Tests requiring AI provider access
- `requires_web_search` — Tests requiring web search capability
- `requires_api` — Tests requiring external API access
- `e2e` — End-to-end tests

## Fakes

The `tests/fakes/` directory contains fake implementations of service clients, agents, and async helpers for use in unit and integration tests without external dependencies.

## Related

- [Development](devs.md) — Dev setup and coding standards
- [Evals](evals.md) — Evaluation suite and benchmarks
