---
doc_id: ensemble-evals
title: Ensemble Evaluations
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.0
owners:
  - ensemble/app/services/evaluation/
  - protocol/python/g8e/models/internal_api.py
related:
  - ../architecture/evals.md
  - ../architecture/agents.md
  - tests.md
  - agents.md
when_to_read: Understanding g8ee evaluation pipeline, campaign request handling, trace persistence, role control, and semantic grading implementation.
do_not_use_for:
  - Platform evaluation programs and campaign orchestration (../architecture/evals.md)
  - AI Agents and the governance boundary (../architecture/agents.md)
  - Release process and evidence artifacts (../devs/release_process.md)
---

# Ensemble Evaluations

## Purpose

Describes the g8ee evaluation pipeline: how campaign controllers submit scored chat assignments, how g8ee records assignment traces, applies homogeneous role control, and invokes semantic judges. Establishes the boundary between g8ee's application-owned trace evidence and the platform's campaign verification and proof retrieval.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

## Invariants

### Evaluation ownership and scope (`INV-EVAL-SCOPE`)

| ID | Rule |
| --- | --- |
| INV-EVAL-SCOPE-01 | g8ee runs the normal `ChatPipelineService` for scored chat evaluations; it does not implement a separate evaluation-only inference path. Production triage, model selection, tool-loop, and governed-Operator behavior remain unchanged. |
| INV-EVAL-SCOPE-02 | g8ee does not own campaign definitions, scheduling, assignment lifecycle aggregates, platform evidence binding, or final verification. The Go evaluation services retrieve the trace and compute the campaign result. |
| INV-EVAL-SCOPE-03 | Model output, Tribunal agreement, semantic judge results, and application approvals remain outside the platform authorization boundary. See [AI Agents and the g8e Governance Boundary](../architecture/agents.md). |

### Evaluation context and request contract (`INV-EVAL-CONTEXT`)

| ID | Rule |
| --- | --- |
| INV-EVAL-CONTEXT-01 | Campaign controllers send a typed `EvaluationInferenceContext` object in the `POST /api/v1/chat` request body. The protocol model in [protocol/python/g8e/models/internal_api.py](../../protocol/python/g8e/models/internal_api.py) is the contract source. |
| INV-EVAL-CONTEXT-02 | The `campaign_id`, `run_id`, `assignment_id`, and `evaluation_attempt_id` fields identify the campaign and assignment. The `scenario_id` identifies the frozen scenario. |
| INV-EVAL-CONTEXT-03 | The `model_registry_digest` and non-empty `model_registry` bind the request to the campaign's frozen model registry. Each registry variant contains a model tag (unique within the registry) and a 64-character lowercase SHA-256 digest. |
| INV-EVAL-CONTEXT-04 | The `target_operator_session_id` identifies the intended governed-Operator execution session. The `evaluation_lane` is `model_role` or `system` (defaults to `system`). |
| INV-EVAL-CONTEXT-05 | For the `model_role` lane, `designated_model_role` is required and MUST be `primary`, `assistant`, or `lite`. For the `system` lane, `designated_model_role` MUST NOT be present. |
| INV-EVAL-CONTEXT-06 | The `grading_method` is `deterministic` or `semantic_judge` (defaults to `deterministic`). The `gold_summary` is required for `semantic_judge` and carries the user prompt, expected behavior, required and forbidden concepts, and expected and forbidden tools. |

### Trace schema and persistence (`INV-EVAL-TRACE`)

| ID | Rule |
| --- | --- |
| INV-EVAL-TRACE-01 | `EvaluationTraceService` persists one immutable JSON trace per assignment and evaluation attempt at `$G8E_RUNTIME_DIR/data/evaluation/traces/<assignment-id>/<evaluation-attempt-id>.json`. When `G8E_RUNTIME_DIR` is unset, g8ee uses `.g8e` in the project root. |
| INV-EVAL-TRACE-02 | Assignment and attempt IDs are validated as safe filenames before filesystem access. Trace writes use canonical JSON and atomic temporary-file replacement (write to `.json.tmp`, then replace). |
| INV-EVAL-TRACE-03 | Trace schema version is `2`. Digest is computed with the shared `g8e.eval.v1` chat-probe trace-digest implementation over the trace with its own `trace_digest` field cleared. Loading validates the typed trace and rejects a digest mismatch. |
| INV-EVAL-TRACE-04 | The authenticated, read-only lookup is `GET /api/v1/evaluation/trace/{assignment_id}/{evaluation_attempt_id}`. The response is `{ "trace": <typed trace> }`. Missing traces return not-found. Unsafe path parameters are rejected. |

### Role control and model selection (`INV-EVAL-ROLE`)

| ID | Rule |
| --- | --- |
| INV-EVAL-ROLE-01 | For the `model_role` lane, `apply_homogeneous_role_control` runs after triage and records the designated role, the natural role from triage, whether they agree, and the triage complexity. The designated role overrides normal triage routing. |
| INV-EVAL-ROLE-02 | The `primary` role activates `ReasoningAgent.SAGE` with `SagePersona`. The `assistant` and `lite` roles activate `ReasoningAgent.DASH` with `DashPersona`, with model resolution taken from the designated tier and request overrides. |
| INV-EVAL-ROLE-03 | A `lite` assignment always resolves the lite tier; the normal simple/complex routing rule does not override that assignment. The `system` lane does not apply homogeneous role control and follows normal triage routing. |

### Semantic grading (`INV-EVAL-GRADE`)

| ID | Rule |
| --- | --- |
| INV-EVAL-GRADE-01 | For `grading_method: "semantic_judge"`, g8ee invokes `EvalJudge` after the interaction completes, before finalizing the trace. Deterministic assignments do not invoke the judge. |
| INV-EVAL-GRADE-02 | The judge model comes from `eval_judge.model` when configured and otherwise falls back to the resolved lite model. Judge calls are retried up to 3 times with exponential backoff (2s initial, 2x multiplier) on transient failures. |
| INV-EVAL-GRADE-03 | The judge must return JSON with an integer score from 1 through 5 and non-empty reasoning. Scores of 3 or higher pass. Invalid or empty responses and failures after all retries produce an `unavailable` semantic grade rather than a fabricated score. |
| INV-EVAL-GRADE-04 | The semantic judge is a grader, not a policy gate. Its score is persisted for the Go campaign verifier and aggregate projections; it cannot approve a tool call or substitute for required governance proof. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Evaluation request model | [protocol/python/g8e/models/internal_api.py](../../protocol/python/g8e/models/internal_api.py) | `EvaluationInferenceContext`, `EvaluationLane`, `DesignatedModelRole`, `EvaluationGoldSummary` |
| Trace service implementation | [ensemble/app/services/evaluation/trace_service.py](../../ensemble/app/services/evaluation/trace_service.py) | `EvaluationTraceService.begin()`, `finalize()`, `load()` |
| Role control implementation | [ensemble/app/services/evaluation/role_control.py](../../ensemble/app/services/evaluation/role_control.py) | `apply_homogeneous_role_control()`, `resolve_role_outcome()` |
| Semantic grader implementation | [ensemble/app/services/evaluation/semantic_grader.py](../../ensemble/app/services/evaluation/semantic_grader.py) | `grade_campaign_assignment_semantically()` |
| Chat pipeline integration | [ensemble/app/services/ai/chat_pipeline.py](../../ensemble/app/services/ai/chat_pipeline.py) | Trace begin/finalize in `_finalize_evaluation_assignment()` |
| Evaluation trace tests | [ensemble/tests/integration/test_evaluation_trace_digest_integration.py](../../ensemble/tests/integration/test_evaluation_trace_digest_integration.py) | Trace digest validation |

## Procedures

## Campaign request contract

The campaign controller sends the normal g8ee chat request with an `evaluation_context` object. The protocol model requires these fields:

- `campaign_id`, `run_id`, `assignment_id`, and `evaluation_attempt_id` identify the campaign and assignment attempt.
- `scenario_id` identifies the frozen scenario.
- `model_registry_digest` and the non-empty `model_registry` bind the request to the campaign's model registry. Each registry variant contains a model tag and a 64-character lowercase SHA-256 digest; model tags must be unique.
- `target_operator_session_id` identifies the intended execution session.
- `evaluation_lane` is `model_role` or `system`. It defaults to `system`.
- `designated_model_role` is `primary`, `assistant`, or `lite` for the `model_role` lane and is required in that lane. It is not allowed in the `system` lane.
- `grading_method` is `deterministic` or `semantic_judge`. It defaults to `deterministic`.
- `gold_summary` is required for `semantic_judge` and carries the private user prompt, expected behavior, required concepts, expected tools, and forbidden tools used by the judge.

The request also carries the usual typed `RequestContext` and optional LLM overrides. The Go harness mirrors this contract in `internal/tools/agent_harness/client/ensemble.go`; the protocol model in `protocol/python/g8e/models/internal_api.py` is the contract source for the Python service.

The chat endpoint returns after starting the background chat task. The campaign controller polls the authenticated trace endpoint until the trace reaches `completed` or `failed`; it does not treat the initial chat response as the assignment result.

## Evaluation execution path

A scored request uses the normal `ChatPipelineService` rather than an evaluation-only inference shortcut:

1. g8ee performs triage and records the triage model telemetry in the assignment trace.
2. For the `model_role` lane, `apply_homogeneous_role_control` runs after triage. It records the designated role, the natural role implied by triage, whether they agree, and the triage complexity. `primary` dynamically activates `ReasoningAgent.SAGE` with `SagePersona`; `assistant` and `lite` activate `ReasoningAgent.DASH` with `DashPersona`, with model resolution taken from the designated tier and its request overrides.
3. The designated role controls the scored model selection even when triage would normally select another tier. A `lite` assignment always resolves the lite tier; the normal simple/complex routing rule does not override that assignment.
4. The system lane does not apply homogeneous role control and follows normal triage routing: complex turns use the primary Sage path, and other turns use the assistant Dash path.
5. The sequential ReAct loop (`G8eAgent._stream_with_tool_loop`) records model calls and tool activity against the bound remote Data Operator without synthetic mocks. Both Tier 1 fast smoke gate assignments (`--gate-smoke`) and full qualification assignments drive this identical production ReAct loop. Governed operator tool calls retain the bound Operator ID and session ID, execution binding, and receipt status in the trace. Failed tool results are classified as policy `deny` for known policy or validation blocks, or `refused` for other failures.
6. Before finalizing the trace, g8ee waits for the background memory-generation task, subject to its evaluation barrier timeout, and includes its model telemetry when available.
7. For `semantic_judge`, g8ee invokes the evaluation judge after the interaction completes, then finalizes the trace. Deterministic assignments do not invoke the semantic judge.

The Tribunal, Marshal, and Auditor remain application-layer behavior in the normal agent path. Tribunal agreement is not protocol L2 consensus and does not authorize a campaign action. Governed tool execution and its authoritative receipt remain owned by the Gateway and target Operator.

## Trace persistence and retrieval

`EvaluationTraceService` stores one JSON trace per assignment and evaluation attempt at:

```text
$G8E_RUNTIME_DIR/data/evaluation/traces/<assignment-id>/<evaluation-attempt-id>.json
```

When `G8E_RUNTIME_DIR` is unset, g8ee uses the project runtime `.g8e` directory. Assignment and attempt IDs are validated as safe filenames before filesystem access. Trace writes use canonical JSON and an atomic temporary-file replacement. The digest is computed with the shared `g8e.eval.v1` chat-probe trace-digest implementation over the trace with its own `trace_digest` field cleared. Loading validates the typed trace and rejects a digest mismatch.

The authenticated, read-only lookup is:

```text
GET /api/v1/evaluation/trace/{assignment_id}/{evaluation_attempt_id}
```

The response is `{ "trace": <typed trace> }`. Missing traces return a not-found response, and unsafe path parameters are rejected. The Go campaign client polls this endpoint after submitting the chat request and imports the trace into the campaign's run-scoped evidence; g8ee does not write the Go campaign run store.

A trace has schema version `2` and can contain:

- evaluation context and the g8ee chat execution ID;
- triage and model-call telemetry;
- controlled-role assignment and `invoked` or `role_not_invoked` outcome;
- the designated-role output;
- model tool decisions and executed tool calls, including hashed arguments;
- governed-action bindings and policy decisions;
- semantic grades and judge-call telemetry;
- finish reason, terminal status, completion timestamp, and trace digest.

The trace is campaign evidence, not an independent authorization record. It does not replace the Operator's local audit evidence, signed receipt, Gateway verification, or campaign verifier.

## Tool and governed-action evidence

Evaluation tool evidence is collected only when `g8e_context.evaluation_context` is present. A started tool call records its name and decision ID. A completed call records a deterministic SHA-256 hash of the command payload or typed result, success, execution ID, operator-tool status, and error type.

For a failed `CommandExecutionResult`, g8ee records a typed policy decision with the outcome `deny` or `refused` and the available error or denial detail. For a successful operator tool, it records the first bound Operator's ID and session ID, the execution binding, a completed receipt status, and policy outcome `allow`. These records describe what g8ee observed; they do not independently prove protocol authorization or receipt validity.

## Semantic grading

A semantic assignment includes `grading_method: "semantic_judge"` and a `gold_summary`. At finalization, `grade_campaign_assignment_semantically` builds an interaction trace from the designated-role output and tool calls, then invokes `EvalJudge` through the configured lite provider. The judge receives the user prompt, expected behavior, required and forbidden concepts/tools, and the recorded interaction summary.

The judge must return JSON with an integer score from 1 through 5 and non-empty reasoning. Scores of 3 or higher pass. Transient provider failures are retried up to three attempts with exponential backoff; invalid or empty responses and failures after retry produce an `unavailable` semantic grade rather than a fabricated score. The judge model comes from `eval_judge.model` when configured and otherwise falls back to the resolved lite model. Each judge call contributes model-call telemetry, including provider-attempt identifiers when available.

The semantic judge is a grader, not a policy gate. Its score is persisted in the g8ee trace for the Go campaign verifier and aggregate projections; it cannot approve a tool call or substitute for a required governance proof.

## Operational boundary

Campaign execution must use the enrolled Operator topology described in [Evaluations](../architecture/evals.md). g8ee routes inference and tool intents through the production governed path; it does not call Ollama directly for campaign scoring. Provider-boundary observation and storage-side model provenance are separate Go-coordinated witness paths, not g8ee trace fields or g8ee-owned verification.

For campaign startup, enrollment, witness operators, smoke workflows, and recovery, use the [Unified Docker Stack Guide](../guides/unified_stack.md). For model-role and system-lane semantics, use the canonical [Evaluations](../architecture/evals.md) architecture document.

## Testing

The g8ee evaluation implementation has focused unit coverage under `ensemble/tests/unit/services/evaluation/` and related chat-pipeline tests. The integration suite includes protocol trace-digest compatibility and trace persistence checks under `ensemble/tests/integration/test_evaluation_trace_digest_integration.py`. These tests validate g8ee trace construction, role control, tool evidence, semantic grading, and persistence; they do not replace Go campaign verification or prove platform-level evaluation results.

See [Ensemble Tests](tests.md) for component test tiers and commands.

## Related documentation

- [Evaluations](../architecture/evals.md) — Platform evaluation programs, campaign orchestration, witness roles, evidence, and verification
- [AI Agents and the g8e Governance Boundary](../architecture/agents.md) — Trust boundaries and the limits of application-layer reasoning
- [Ensemble Architecture](architecture.md) — g8ee runtime and service wiring
- [Agents](agents.md) — Persona routing, Tribunal, Marshal, Auditor, and model-loop behavior
- [LLM Providers](llm-providers.md) — Provider implementations and the governed g8e inference path
- [Unified Docker Stack](../guides/unified_stack.md) — Campaign deployment and operations
- [Documentation Guide](../devs/docs.md) — Documentation audit and ownership rules
