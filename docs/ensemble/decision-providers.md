---
doc_id: decision-providers
title: Decision Providers
audience: developers integrating structured reasoning into agent workloads
status: current
last_updated: 2026-09-30
version: v2.2.5
owners:
  - ensemble/app/decision/
  - ensemble/app/services/ai/triage.py
  - ensemble/app/services/ai/eval_judge.py
related:
  - llm-providers.md
  - evals.md
  - agents.md
  - docs/devs/devs.md
  - docs/devs/tests.md
when_to_read: Building triage or semantic eval features, integrating System One classification, testing decision provider configuration and coexistence constraints.
do_not_use_for:
  - Generative text LLM selection (docs/ensemble/llm-providers.md)
  - Eval rubrics and grading specifications (docs/ensemble/evals.md)
  - Agent persona and instruction design (docs/ensemble/agents.md)
---

# Decision Providers

## Purpose

g8ee separates **text generation** (generative LLM providers) from **structured decision** workloads (decision providers). Decision providers evaluate a `state` plus typed `questions` and return structured answers with probabilities. They do not emit chat prose.

The first decision provider is **System One** via Ollama, a local model service for native classification and scoring. Jev (available via Ollama 0.35+) powers lite-role decision workloads when explicitly configured; other roles require generative LLM providers.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Overview](#overview)
- [When to use Jev](#when-to-use-jev)
- [Configuration](#configuration)
- [API contract](#api-contract)
- [Question mappings](#question-mappings-as-implemented)
- [Guardrails](#guardrails)
- [Testing](#testing)
- [Links out](#links-out)

Invariant groups: [Coexistence constraints](#coexistence-constraints-inv-dec-coex).

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Decision provider architecture | [ensemble/app/decision/](ensemble/app/decision/) | `DecisionProvider` ABC, `JevProvider` implementation, factory cache |
| Triage question mapping | [ensemble/app/services/ai/triage.py:_build_jev_questions()](ensemble/app/services/ai/triage.py) | Choice question names and criteria; confidence threshold constant |
| Eval judge question mapping | [ensemble/app/services/ai/eval_judge.py:_build_jev_questions()](ensemble/app/services/ai/eval_judge.py) | Score and noul question names; passing threshold constant |
| Coexistence validation | [ensemble/app/decision/validation.py](ensemble/app/decision/validation.py) | `validate_jev_lite_coexistence()` rejects Tribunal + Jev lite |
| Configuration defaults | [ensemble/app/constants/config.py](ensemble/app/constants/config.py) | `JEV_DEFAULT_MODEL`, Ollama settings (`G8E_LLM_OLLAMA_ENDPOINT`) |

## Procedures

### Verifying Jev provider configuration

1. Confirm the lite provider is `jev` in the platform LLM settings (Console Settings).
2. Verify Ollama is running with System One model pulled (e.g., `ollama pull nimble` or `ollama pull tev1`).
3. Verify `G8E_LLM_OLLAMA_ENDPOINT` is set (default `localhost:11434`). If running Ollama behind a reverse proxy with API key, set `G8E_LLM_OLLAMA_API_KEY`.
4. Check that Tribunal command generation is disabled in the platform LLM settings (Tribunal must be off).
5. Verify primary and assistant roles use generative providers (e.g., `ollama`, `anthropic`, `openai`).
6. Run a test triage or eval request; logs should show Jev calls, not fallback generative triage.

### Adding a new decision workload

1. Create a new question type or use existing `ChoiceQuestion`, `ScoreQuestion`, or `NoulQuestion`.
2. Implement a call site builder method (e.g., `_build_jev_questions()`) returning a dict of question objects.
3. Call `get_decision_provider(settings.llm)` and invoke `await provider.evaluate(model=…, state=…, questions=…)`.
4. Parse the `EvaluateResponse` into a domain-specific result struct.
5. Add unit tests mocking the HTTP response in `ensemble/tests/unit/decision/`.
6. Add integration tests in `ensemble/tests/integration/` with the `requires_system_one` marker.

### Updating Jev endpoint or model defaults

1. For model changes: edit [ensemble/app/constants/config.py](ensemble/app/constants/config.py), update `JEV_DEFAULT_MODEL`.
2. For Ollama endpoint changes: set `G8E_LLM_OLLAMA_ENDPOINT` environment variable (default `localhost:11434`).
3. Run unit tests to verify factory cache key generation and provider instantiation still work.
4. Run integration tests against the running Ollama instance to confirm reachability and response format.

## Overview

Implementation entry points:

| Layer | Path |
| --- | --- |
| Abstract base class | [ensemble/app/decision/provider.py](ensemble/app/decision/provider.py) |
| Types and schema | [ensemble/app/decision/types.py](ensemble/app/decision/types.py) |
| Factory and cache | [ensemble/app/decision/factory.py](ensemble/app/decision/factory.py) |
| Jev HTTP adapter | [ensemble/app/decision/providers/jev.py](ensemble/app/decision/providers/jev.py) |
| Coexistence validation | [ensemble/app/decision/validation.py](ensemble/app/decision/validation.py) |

Call sites:

| Workload | Module | Entry point | Fallback |
| --- | --- | --- | --- |
| Triage (complexity, intent, posture) | [ensemble/app/services/ai/triage.py](ensemble/app/services/ai/triage.py) | `get_decision_provider()` + batched `choice` questions | `_classify_generative()` when lite provider is not `jev` |
| Semantic eval judge | [ensemble/app/services/ai/eval_judge.py](ensemble/app/services/ai/eval_judge.py) | `EvalJudge(decision_provider=…)` + `score` / `noul` questions | Generative LLM JSON judge when lite provider is not `jev` |

## When to use System One (Ollama)

Select System One models via Ollama for the **lite role** to enable native classification and scoring without prompt-and-JSON simulation. System One models (e.g., Nimble 9B or Tev1 4B/0.8B) excel at structured decision tasks but do not support text generation.

System One supports only **decision** and **triage** workloads via `get_decision_provider()`. Generative lite features automatically fall back to the assistant provider when lite is System One.

### Supported paths

- **Triage** (complexity, intent, posture classification): `get_decision_provider()` returns a cached System One provider instance; call `provider.evaluate()` with `choice` questions.
- **Semantic eval judge** (rubric scoring): Pass `decision_provider=get_decision_provider()` to `EvalJudge`; judge uses `score` and `noul` questions for grading.

### Unsupported paths and fallbacks

Generative lite features use `get_generative_lite_provider()`, which automatically selects the **assistant provider** when `lite_provider=jev` (System One):

- Case title generation
- Memory extraction
- Marshal response analysis

These call sites detect System One on the lite role and defer to the assistant provider without additional configuration.

### Coexistence constraints (`INV-DEC-COEX`)

| Constraint | Error | Resolution |
| --- | --- | --- |
| Tribunal command generation + Jev lite | Rejected at `validate_llm_config` | Disable command generation in the platform LLM settings or use a generative lite provider |
| Direct `get_llm_provider(..., is_lite=True)` with Jev | `ConfigurationError` at call time | Use `get_decision_provider()` for System One workloads or `get_generative_lite_provider()` for text generation |
| Jev on primary or assistant role | Rejected at `validate_llm_config` | Jev is valid only on the lite role |

At startup, when `lite_provider=jev`, ensemble logs a warning stating which generative lite call sites fall back to the assistant provider. This is expected behavior, not a configuration error.

## Configuration

Jev is selected through the same lite-role settings as other LLM providers. The constant `LLMProvider.JEV` (`"jev"` string value) is valid **only** for the lite role.

### Platform settings and environment variables

Provider and model selection, and whether Tribunal command generation is enabled, are Gateway-backed platform settings (set in the Console Settings page). They are not read from the environment (INV-ENV-04). Only endpoints and API keys come from the environment:

| Setting | Source | Type | Default |
| --- | --- | --- | --- |
| Lite provider | Platform settings (`jev`) | string | Unset (optional) |
| Lite model | Platform settings | string | `nimble` when lite provider is `jev` |
| Tribunal command generation | Platform settings (must be disabled with Jev lite) | bool | See platform settings |
| Ollama endpoint | `G8E_LLM_OLLAMA_ENDPOINT` | URL | `localhost:11434` |
| Ollama API key | `G8E_LLM_OLLAMA_API_KEY` | string | None (optional; for reverse-proxy authentication only) |

Jev resolves to the Ollama System One endpoint (`<G8E_LLM_OLLAMA_ENDPOINT>/v1/systemone`) using the existing Ollama settings path. No separate Jev endpoint or API key is required.

Primary and assistant roles must be configured with generative LLM providers; Jev is rejected on those roles during validation.

### Example configuration

```bash
# Ollama System One endpoint (ensure nimble is pulled: ollama pull nimble)
G8E_LLM_OLLAMA_ENDPOINT=localhost:11434
# G8E_LLM_OLLAMA_API_KEY=your_key  # Only if Ollama is behind a reverse proxy with auth
```

In the Console Settings page, set the lite provider to `jev` with model `nimble`, set the primary and assistant roles to generative providers (for example `ollama` with `qwen3.5:2b` and `llama3.2:3b`), and disable Tribunal command generation.

## API contract

The `JevProvider` class in [ensemble/app/decision/providers/jev.py](ensemble/app/decision/providers/jev.py) implements a thin HTTP adapter over the Ollama System One API (available in Ollama 0.35+).

| Field | Value |
| --- | --- |
| Endpoint | `POST <G8E_LLM_OLLAMA_ENDPOINT>/v1/systemone` (default `localhost:11434/v1/systemone`) |
| Authentication | Optional `Authorization: Bearer <api_key>` header (only if `G8E_LLM_OLLAMA_API_KEY` is set, for reverse-proxy scenarios) |
| Request body | JSON object with `model` (string), `state` (string, dict, or list), `questions` (object mapping names to question objects) |
| Question types | `choice` (enum classification), `score` (numeric scale 1–5), `noul` (yes/no/unknown) |
| Response | JSON object with `model` (string), `answers` (object mapping question names to answer objects), `usage` (token counts) |
| Limits | Request body ≤ 64 KiB (HTTP 413 if exceeded); 404 when model not pulled; 400 for prompt exceeding context |

Each question object includes `type`, `instructions` (string), and type-specific fields (`criteria` for `choice` and `score`). Each answer includes `type`, the resolved value (e.g., `choice` or `score`), `confidence` (0–1), and `probabilities` (per-choice or per-score-level breakdown).

**Implementation notes:**
- `JevProvider` uses `httpx.AsyncClient` with a default timeout.
- HTTP errors map to `ExternalServiceError` or `RateLimitError` for distinction.
- Requests exceeding 64 KiB raise `ExternalServiceError` (413 payload too large).
- Callers own retry logic; for example, `EvalJudge` applies exponential backoff on rate limits.
- Model boundary attestation records input artifact hashes for privacy audit trails.
- Reference: [Ollama System One API docs](https://docs.ollama.com/api/systemone)

## Question mappings (as implemented)

### Triage questions

Triage sends three `ChoiceQuestion` objects to Jev; see [ensemble/app/services/ai/triage.py:_build_jev_questions()](ensemble/app/services/ai/triage.py) for full criteria.

| Question key | Type | Choices | Maps to result field |
| --- | --- | --- | --- |
| `complexity` | `choice` | `simple`, `complex` | `complexity` enum + `complexity_confidence` (HIGH if confidence ≥ 0.85, else LOW) |
| `intent` | `choice` | `information`, `action`, `unknown` | `intent` enum + `intent_confidence` |
| `request_posture` | `choice` | `normal`, `escalated`, `adversarial`, `confused` | `request_posture` enum + `posture_confidence` |

Confidence mapping uses the constant `JEV_TRIAGE_HIGH_CONFIDENCE_THRESHOLD = 0.85`. `intent_summary` is synthesized from the choice result and context.

### Eval judge questions

Eval judge sends one `ScoreQuestion` and one `NoulQuestion` to the System One model via Ollama; see [ensemble/app/services/ai/eval_judge.py:_build_jev_questions()](ensemble/app/services/ai/eval_judge.py) for full instructions.

| Question key | Type | Criteria | Maps to result field |
| --- | --- | --- | --- |
| `rubric_score` | `score` | `["1", "2", "3", "4", "5"]` | `EvalGrade.score` (1–5 integer) |
| `meets_passing_threshold` | `noul` | N/A (yes/no/unknown) | Included in `EvalGrade.reasoning` as probability summary |

Pass/fail is deterministic: `score >= 3` passes. System One models do not emit prose reasoning; the grade stores a formatted probability summary instead of natural-language justification.

## Invariants

### Coexistence constraints (`INV-DEC-COEX`)

| Constraint | Location | Enforcement |
| --- | --- | --- |
| `get_llm_provider(..., is_lite=True)` with Jev | [ensemble/app/llm/factory.py:get_llm_provider()](ensemble/app/llm/factory.py) | Raises `ConfigurationError` before LLM initialization |
| `get_generative_lite_provider()` with Jev lite | [ensemble/app/llm/factory.py:get_generative_lite_provider()](ensemble/app/llm/factory.py) | Transparently falls back to assistant provider; no error |
| Jev on primary or assistant role | [ensemble/app/services/ai/chat_pipeline.py:validate_llm_config()](ensemble/app/services/ai/chat_pipeline.py) | Rejected during config validation in routers before request handling |
| Tribunal command generation + Jev lite | [ensemble/app/decision/validation.py:validate_jev_lite_coexistence()](ensemble/app/decision/validation.py) | Rejected during config validation; call `validate_jev_lite_coexistence()` after `resolve()` |
| Missing Ollama endpoint | [ensemble/app/decision/providers/jev.py:JevProvider.validate_config()](ensemble/app/decision/providers/jev.py) | Raises `ConfigurationError` when `JevProvider` is instantiated or when chat validation calls `provider_classes[LLMProvider.JEV.value].validate_config()` |

All guardrails trigger at startup or request time, never at runtime during classification or grading. Configuration errors are fatal; invalid Jev state is never silently degraded to a fallback.

## Testing

### Unit tests

Tier 1 unit tests mock HTTP at the `JevProvider` boundary:

- [ensemble/tests/unit/decision/test_jev_provider.py](ensemble/tests/unit/decision/test_jev_provider.py) — `JevProvider` HTTP request/response handling and error translation
- [ensemble/tests/unit/decision/test_factory.py](ensemble/tests/unit/decision/test_factory.py) — Provider cache key generation and factory logic
- [ensemble/tests/unit/decision/test_validation.py](ensemble/tests/unit/decision/test_validation.py) — Coexistence constraint validation
- [ensemble/tests/unit/decision/test_types.py](ensemble/tests/unit/decision/test_types.py) — Question and answer type schemas

Run from repository root:
```bash
make test-unit
```

### Integration tests

Tier 4 live tests call the actual Ollama System One API and require Ollama 0.35+ running with a System One model pulled (e.g., `nimble`). These tests use the `requires_system_one` pytest marker:

- [ensemble/tests/integration/test_jev_triage_integration.py](ensemble/tests/integration/test_jev_triage_integration.py) — End-to-end triage classification via System One
- [ensemble/tests/integration/test_jev_eval_judge_integration.py](ensemble/tests/integration/test_jev_eval_judge_integration.py) — End-to-end eval judge grading via System One

Run external tests from repository root:
```bash
make test-external   # Runs all tests marked requires_system_one
```

For details on test markers and CI scope, see [Testing Guide](docs/devs/tests.md).

## Anti-patterns

- Attempting to use System One for text generation (e.g., calling `get_llm_provider(..., is_lite=True)` with System One). Decision providers are not LLM providers; use `get_decision_provider()` for System One classification only.
- Enabling Tribunal command generation alongside System One lite. This configuration is rejected at validation; disable command generation or choose a generative lite provider.
- Configuring System One on primary or assistant roles. System One is valid **only** on the lite role for decisions; primary and assistant must use generative providers.
- Relying on fallback text generation when a generative lite call site runs against System One. Callers of `get_generative_lite_provider()` automatically fall back to the assistant provider; this is correct behavior, not a degradation.

## Links out

- [LLM Providers](llm-providers.md): Generative provider roles, adapters, and configuration
- [Evals](evals.md): Semantic judge rubrics, grading semantics, and evidence recording
- [Agents](agents.md): Triage and Judge agent personas and system prompts
- [Developer Guidelines](../devs/devs.md): Coding invariants and repository standards
- [Testing Guide](../devs/tests.md): Test execution tiers, markers, and CI scope
