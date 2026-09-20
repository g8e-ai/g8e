# Ensemble Evaluations

## Overview

g8ee participates in g8e evaluation programs but does not own platform evidence, verification, or campaign orchestration. The Go-native evaluator, campaign controller, evidence store, and verifier live in the `g8e` binary. This page documents how g8ee uses those programs.

For the platform evaluation model — execution-boundary suite, model campaign topology, Observer and Provenance Operator roles, evidence layout, and verification — see [Evaluations](../architecture/evals.md).

## Relationship to g8e evals

| Concern | Owner | g8ee role |
| --- | --- | --- |
| Execution-boundary suite (`core-execution-boundary@1.0.0`) | `g8e eval boundary run` | Not involved. The suite proves the remote Operator boundary without Python, g8ee, or a model provider. |
| Model campaign scoring | `g8e eval campaign …` | Production inference path. Campaign execute dispatches scored assignments through governed `POST /api/v1/chat`. |
| Chat-path acceptance | `g8e eval gate chat run` | Direct vertical acceptance of the production chat API before or alongside campaign work. |
| Evidence and verification | `g8e eval boundary verify`, `g8e eval campaign verify` | g8ee emits trace and telemetry records consumed by the campaign controller; verification is always platform-owned. |

g8ee remains outside the trusted execution boundary. Model output, Tribunal agreement, and application approval do not authorize host mutation. The Gateway and target Operators apply the active governance posture before any operation executes.

## Production chat path for model campaigns

Model evaluation campaigns score real models through the same `POST /api/v1/chat` path used in production. The campaign CLI does not call Ollama or model APIs directly. Instead, `g8e eval campaign execute` drives assignments that:

1. Bind campaign authority, model registry digest, and exact Operator sessions on each governed dispatch.
2. Route inference through the enrolled Inference Operator to the approved remote Ollama provider.
3. Route tool intents through the enrolled Data Operator.
4. Trigger provider-boundary BEGIN/FINALIZE observation on the enrolled Observer Operator (provider host) for GPU/RAM witness telemetry.
5. Trigger model provenance BEGIN/FINALIZE attestation on the enrolled Provenance Operator (model storage site) when provenance is enabled, binding `served_model_tag` and `expected_model_digest` to each `provider_attempt_id`.
6. When the Observer was started with `--ollama`, reset the Ollama provider on the provider host before each assignment through governed commands to that exact Observer session. The platform-specific sequence stops Ollama, restarts its daemon, waits for readiness, and confirms quiescence with `ollama ps`; see [Evaluations](../architecture/evals.md#provider-boundary-observer-operator) for the Unix and Windows command sequences.

g8ee's `ChatPipelineService` handles triage, model selection, tool loops, Tribunal command generation, and governed relay to the bound Operators. Campaign scoring depends on this production path rather than a separate eval-only shortcut.

Operational campaign workflows (Compose profiles, enrollment order, smoke runs, Observer and Provenance Operator checklists) live in the [Unified Docker Stack Guide](../guides/unified_stack.md).

## g8ee evaluation services

g8ee records evaluation-specific telemetry and trace data under `ensemble/app/services/evaluation/`:

| Module | Purpose |
| --- | --- |
| `trace_service.py` | Persists assignment traces, governed action records, tool calls, grader calls, and semantic grade bindings with canonical digest computation |
| `role_control.py` | Controlled-role assignment and outcome tracking for scored chat scenarios |
| `semantic_grader.py` | Semantic grading hooks invoked during campaign assignments |
| `tool_evidence.py` | Tool-call evidence collection for governed scenario grading |

Trace records align with protocol types in `g8e.eval.v1` and are written beneath the g8ee runtime tree. The campaign controller on the Go side binds these traces to assignment lifecycle records under `.g8e/data/eval/runs/<run-id>/`.

## Chat-path acceptance

Before or alongside full campaign execution, run the Phase 1A chat-path vertical acceptance matrix:

```bash
./g8e eval gate chat run \
  --model <ollama-model> \
  --campaign-id <campaign-id> \
  --registry-file <registry-freeze.json>
```

This exercises production `POST /api/v1/chat` through g8ee with governed Operator binding, waits for trace completion, and validates the acceptance cases without bypassing the ensemble stack.

## Testing

Evaluation behavior in g8ee is covered by unit tests under `ensemble/tests/unit/services/evaluation/` and chat-pipeline integration tests that exercise controlled-role assignments, trace barriers, semantic grading, and tool evidence. Platform verification and campaign evidence checks remain Go-owned; Python tests validate g8ee's contribution to the trace and grading surface, not the final campaign verdict.

See [Ensemble Tests](tests.md) for the full test tier model and commands.

## Related documentation

- [Evaluations](../architecture/evals.md) — Primary platform evaluation architecture
- [Model Provenance](../architecture/model-provenance.md) — Storage-side weight attestation and chain of custody
- [Ensemble Architecture](architecture.md) — g8ee system design and component overview
- [LLM Providers](llm-providers.md) — Provider implementations and the governed `g8e` inference path
- [Agents](agents.md) — Persona roster, Tribunal, and tool-loop behavior during scored assignments
- [Unified Docker Stack](../guides/unified_stack.md) — Deployment and campaign operations
