# Decision Providers

Last Updated: 2026-09-25  
Version: v2.2.x (ensemble MVP)

## Overview

g8ee separates **text generation** (`LLMProvider` in `ensemble/app/llm/`) from **structured decision** workloads (`DecisionProvider` in `ensemble/app/decision/`). Decision providers evaluate a `state` plus typed `questions` and return structured answers with probabilities. They do not emit chat prose.

The first decision provider is **Jev** from [TypeSafe AI](https://typesafe.ai/blog/introducing-system-one-models-and-jev), a System One model exposed at `POST https://api.typesafe.ai/v1/systemone`.

Implementation entry points:

| Layer | Path |
| --- | --- |
| ABC and types | `ensemble/app/decision/provider.py`, `ensemble/app/decision/types.py` |
| Factory | `ensemble/app/decision/factory.py` |
| Jev HTTP adapter | `ensemble/app/decision/providers/jev.py` |
| Coexistence validation | `ensemble/app/decision/validation.py` |

## When to use Jev

Select Jev for the **lite role** when you want native System One classification or scoring instead of prompt-and-JSON simulation:

| Call site | Jev path | LLM fallback |
| --- | --- | --- |
| Triage (complexity, intent, posture) | `get_decision_provider()` + batched `choice` questions | `generate_content_lite` when lite provider is not `jev` |
| Semantic eval judge | `EvalJudge(decision_provider=…)` + `score` / `noul` questions | LLM JSON judge when lite provider is not `jev` |

Jev is **not** a drop-in replacement for every lite call site. Features that still call `get_llm_provider(..., is_lite=True)` for text generation require a generative lite provider:

- Case title generation
- Memory extraction
- Marshal response analysis
- Tribunal command generation (blocked at validation when `G8E_LLM_COMMAND_GEN_ENABLED=true`)

At startup, ensemble logs a warning when `lite_provider=jev` summarizing these limitations. Chat validation fails closed when Tribunal is enabled alongside Jev on the lite role.

## Configuration

Jev is selected through the same lite-role settings as other providers. `LLMProvider.JEV` (`"jev"`) is valid **only** for the lite role; primary and assistant reject it during chat validation.

### Environment variables

| Setting | Env var | Default / notes |
| --- | --- | --- |
| Lite provider | `G8E_LLM_LITE_PROVIDER` | Set to `jev` |
| Lite model | `G8E_LLM_LITE_MODEL` | `jev-latest` (pin `jev-<version>` when thresholds matter) |
| Jev model | `G8E_LLM_JEV_MODEL` | `jev-latest` |
| Jev API key | `G8E_LLM_JEV_API_KEY` | Required; `TYPESAFE_API_KEY` is accepted as a fallback |
| Jev endpoint | `G8E_LLM_JEV_ENDPOINT` | `https://api.typesafe.ai/v1/systemone` |

Primary and assistant roles must remain generative LLM providers.

### Example

```bash
# Lite role: Jev for triage + eval judge
G8E_LLM_LITE_PROVIDER=jev
G8E_LLM_LITE_MODEL=jev-latest
G8E_LLM_JEV_API_KEY=ts_...

# Tribunal off when lite is Jev (required for chat validation)
G8E_LLM_COMMAND_GEN_ENABLED=false

# Primary/assistant remain generative
G8E_LLM_PRIMARY_PROVIDER=ollama
G8E_LLM_PRIMARY_MODEL=qwen3.5:2b
G8E_LLM_ASSISTANT_PROVIDER=ollama
G8E_LLM_ASSISTANT_MODEL=llama3.2:3b
```

## API contract

Reference: [Jev how-to](https://www.jevtypesafeai.com/how-to-use)

| Field | Value |
| --- | --- |
| Endpoint | `POST https://api.typesafe.ai/v1/systemone` |
| Auth | `Authorization: Bearer <api_key>` |
| Request body | `{ "model", "state", "questions" }` |
| Question types | `choice`, `score`, `noul` |
| Response | `{ "model", "answers", "usage" }` with per-question typed payloads and probabilities |

`JevProvider` uses a thin `httpx` client (no `typesafe-sdk` dependency). SDK-level retries are disabled; ensemble services own retry policy where needed (for example eval judge exponential backoff on rate limits).

## Question mappings (as implemented)

### Triage

| Question key | Type | Maps to |
| --- | --- | --- |
| `complexity` | `choice` | `TriageComplexityClassification` |
| `intent` | `choice` | `TriageIntentClassification` |
| `request_posture` | `choice` | `TriageRequestPosture` |

`intent_summary` is synthesized from the three choices. Choice confidence maps to `TriageConfidence` (HIGH when confidence ≥ 0.85).

### Eval judge

| Question key | Type | Maps to |
| --- | --- | --- |
| `rubric_score` | `score` (criteria `["1"…"5"]`) | `EvalGrade.score` |
| `meets_passing_threshold` | `noul` | Included in `EvalGrade.reasoning` probability summary |

Pass/fail is deterministic from `score >= 3`. Jev does not emit prose reasoning; semantic grade `detail` stores the formatted probability summary.

## Guardrails

| Check | Behavior |
| --- | --- |
| `get_llm_provider(..., is_lite=True)` with `jev` | Raises `ConfigurationError` — no fake text stream |
| Primary or assistant `jev` | Rejected in `validate_llm_config` |
| Jev lite + Tribunal enabled | Rejected in `validate_llm_config` and at startup |
| Missing Jev API key | `JevProvider.validate_config` error on lite tier |

## Testing

Tier 1 unit tests mock HTTP at the `JevProvider` boundary (`ensemble/tests/unit/decision/`). Tier 4 live tests use the `requires_typesafe` marker and are gated on `G8E_LLM_JEV_API_KEY` or `TYPESAFE_API_KEY`:

- `tests/integration/test_jev_triage_integration.py`
- `tests/integration/test_jev_eval_judge_integration.py`

Run external tests from the repository root:

```bash
make test-external   # includes requires_typesafe
```

See [Testing](tests.md) for marker details.

## Related

- [LLM Providers](llm-providers.md): Generative provider roles and adapters
- [Evals](evals.md): Semantic judge grading and evidence
- [Agents](agents.md): Triage and Judge personas
- Plan: `.local.dev/docs/plans/in-progress/jev-decision-provider.md`
