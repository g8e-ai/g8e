---
doc_id: ensemble-evals
title: Ensemble Evaluations
audience: maintainers and coding agents
status: current
last_updated: 2026-10-01
version: v2.2.6
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
| INV-EVAL-CONTEXT-07 | An optional `seed` (`EvaluationInvestigationSeed`) describes the investigation a scored turn runs in: `case_title`, optional `case_description`, `turns` (sender `user`, `primary`, or `assistant`), `history_events`, and optional `case_memory`. The model bounds it to 16 turns, 16 history events, and 8000 characters per text field. A seed is accepted only when `resource_creation.create_case` is true; otherwise the request fails with HTTP 400 before anything is written, so a seed can never write into an existing investigation. |
| INV-EVAL-CONTEXT-08 | An optional `workspace` (`EvaluationWorkspace`) carries the attempt-scoped fixture `root` and the Data Operator's `operator_working_directory`; the root MUST be strictly under the working directory with no `..` segment. g8ee only echoes it into the trace; the Go campaign executor owns writing the fixtures. |

### Trace schema and persistence (`INV-EVAL-TRACE`)

| ID | Rule |
| --- | --- |
| INV-EVAL-TRACE-01 | `EvaluationTraceService` persists one immutable JSON trace per assignment and evaluation attempt at `<runtime-dir>/data/evaluation/traces/<assignment-id>/<evaluation-attempt-id>.json`. `<runtime-dir>` is the `--runtime-dir` launch argument; when it is not given, g8ee uses `.g8e` in the project root. |
| INV-EVAL-TRACE-02 | Assignment and attempt IDs are validated as safe filenames before filesystem access. Trace writes use canonical JSON and atomic temporary-file replacement (write to `.json.tmp`, then replace). |
| INV-EVAL-TRACE-03 | Trace schema version is `7` (`3` added `arguments_json`, `command`, and `result_json` to tool calls; `4` added the terminal `error` field; `5` added `tool_gate`, `provider_tool_rejection`, per-call `tools_declared`, and the guidance fields; `6` added `seed_application`, `user_memories_suppressed`, and `tool_turn_limit_reached`; `7` added `player_steps`). Digest is computed with the shared `g8e.eval.v1` chat-probe trace-digest implementation over the trace with its own `trace_digest` field cleared. Loading validates the typed trace and rejects a digest mismatch. |
| INV-EVAL-TRACE-04 | The authenticated, read-only lookup is `GET /api/v1/evaluation/trace/{assignment_id}/{evaluation_attempt_id}`. The response is `{ "trace": <typed trace> }`. Missing traces return not-found. Unsafe path parameters are rejected. |
| INV-EVAL-TRACE-05 | `player_steps` is the chain of the scored turn: one typed `EvaluationPlayerStep` per player that did its job (`triage`, `sage` or `dash`, the Tribunal seats `axiom` `concord` `variance` `pragma` `nemesis`, the deterministic `tribunal` vote, `marshal_command`, `marshal_error`, `auditor`, `codex`), each carrying the tier it resolved from (`model_role`), the model, and at most one typed output (triage classification, candidate command, vote, risk, audit verdict, error analysis, or text). A step is recorded where the player finishes, from the objects g8ee itself produced: Tribunal-chain steps through the `TribunalObserver` on `TribunalEmitter` (so a failed seat, a second round, and a failed Auditor are recorded as well as successes), the others from `TriageResult`, the agent stream state, and the memory update. Recording never changes how the chain runs, and a production request has no observer. The wire shape is pinned by `protocol/vectors/eval/player_steps.json`, which `test_player_steps.py` and the Go grader both test. |

### Role control and model selection (`INV-EVAL-ROLE`)

| ID | Rule |
| --- | --- |
| INV-EVAL-ROLE-01 | For the `model_role` lane, `apply_homogeneous_role_control` runs after triage and records the designated role, the natural role from triage, whether they agree, and the triage complexity. The designated role overrides normal triage routing. |
| INV-EVAL-ROLE-02 | The `primary` role activates `ReasoningAgent.SAGE` with `SagePersona`. The `assistant` and `lite` roles activate `ReasoningAgent.DASH` with `DashPersona`, with model resolution taken from the designated tier and request overrides. Resolution uses `resolve_model_for_designated_role`, which reads only that tier's own override and settings value and never falls back to another tier; a missing model raises a `ValidationError`. |
| INV-EVAL-ROLE-03 | A `lite` assignment always resolves the lite tier; the normal simple/complex routing rule does not override that assignment. The `system` lane does not apply homogeneous role control and follows normal triage routing. |

### Seeded investigations and eval-only divergences (`INV-EVAL-SEED`)

| ID | Rule |
| --- | --- |
| INV-EVAL-SEED-01 | `InvestigationSeedService.apply` writes a seed through the same investigation, case, and memory services the live chat path uses, in this order: case and investigation title and description, conversation turns, history events, then case memory. It returns the counts it wrote (`EvaluationSeedApplication`). It never writes user-wide memories. |
| INV-EVAL-SEED-02 | A history event's `event_type` MUST be one of eight operator events (`OPERATOR_COMMAND_EXECUTION`, `OPERATOR_COMMAND_FAILED`, `OPERATOR_COMMAND_APPROVAL_REJECTED`, `OPERATOR_FILESYSTEM_GREP_COMPLETED`, `OPERATOR_FILESYSTEM_GREP_FAILED`, `OPERATOR_FILESYSTEM_READ_COMPLETED`, `OPERATOR_FILESYSTEM_READ_FAILED`, `OPERATOR_FILE_EDIT_FAILED`). Any other type is refused before any write, and a write failure surfaces rather than leaving a partial seed. |
| INV-EVAL-SEED-03 | The router applies the seed synchronously after inline case creation and before the chat task is scheduled, and does not schedule AI title generation for a seeded request, so the realistic seeded title stands. If `apply` raises, the already-begun trace is closed through `EvaluationTraceService.finalize_crashed` and the error is re-raised, so the Go side sees an HTTP error rather than a stuck `running` trace. |
| INV-EVAL-SEED-04 | Every eval-only divergence from production chat is keyed on `evaluation_context` and recorded in the trace: `tool_gate: bypassed_for_eval`; `user_memories_suppressed: true` (user-wide memories, which are artifacts of other assignments, are not read; case memories still are); and `tool_turn_limit_reached: true` (at `AGENT_MAX_TOOL_TURNS` the continue-approval is denied immediately and the run ends with finish reason `tool_turn_limit`, because no human is present to answer). A production request takes none of these branches. |
| INV-EVAL-SEED-05 | A pipeline crash on a scored request still finalizes a terminal trace (`failed`, keeping any triage already recorded and never overwriting a terminal trace), so the Go waiter gets a result instead of timing out. |

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
| Investigation seed application | [ensemble/app/services/evaluation/investigation_seed.py](../../ensemble/app/services/evaluation/investigation_seed.py) | `InvestigationSeedService.apply()`, `SEEDABLE_HISTORY_EVENTS` |
| Player step recording | [ensemble/app/services/evaluation/player_steps.py](../../ensemble/app/services/evaluation/player_steps.py) | `PlayerStepRecorder`, `assemble_player_steps()`; wire vector [protocol/vectors/eval/player_steps.json](../../protocol/vectors/eval/player_steps.json) |
| Agent tool registry export | [ensemble/app/services/evaluation/agent_tool_registry_export.py](../../ensemble/app/services/evaluation/agent_tool_registry_export.py) | `make agent-tool-registry-check`; see [Ensemble Development Guide](devs.md) |
| Eval tool-gate decision | [ensemble/app/services/evaluation/tool_gate.py](../../ensemble/app/services/evaluation/tool_gate.py) | `resolve_tool_gate()`; applied by `AIToolService.get_tools()` |
| Role control implementation | [ensemble/app/services/evaluation/role_control.py](../../ensemble/app/services/evaluation/role_control.py) | `apply_homogeneous_role_control()`, `resolve_role_outcome()` |
| Semantic grader implementation | [ensemble/app/services/evaluation/semantic_grader.py](../../ensemble/app/services/evaluation/semantic_grader.py) | `grade_campaign_assignment_semantically()` |
| Chat pipeline integration | [ensemble/app/services/ai/chat_pipeline.py](../../ensemble/app/services/ai/chat_pipeline.py) | Trace begin/finalize in `_finalize_evaluation_assignment()` |
| Evaluation trace tests | [ensemble/tests/integration/test_evaluation_trace_digest_integration.py](../../ensemble/tests/integration/test_evaluation_trace_digest_integration.py) | Trace digest validation |
| Seeded-fact integration test | [ensemble/tests/integration/test_investigation_seed_integration.py](../../ensemble/tests/integration/test_investigation_seed_integration.py) | Seeded history is returned by the real `query_investigation_context` handler |

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
- `seed` (optional) is the investigation the turn runs in; see [Seeded investigations](#seeded-investigations).
- `workspace` (optional) is the attempt-scoped fixture workspace the Go executor prepared on the Data Operator. It is echoed in the trace and not otherwise used by g8ee.

The request also carries the usual typed `RequestContext` and optional LLM overrides. The Go harness mirrors this contract in `internal/tools/agent_harness/client/ensemble.go`; the protocol model in `protocol/python/g8e/models/internal_api.py` is the contract source for the Python service.

The chat endpoint returns after starting the background chat task. The campaign controller polls the authenticated trace endpoint until the trace reaches `completed` or `failed`; it does not treat the initial chat response as the assignment result.

## Evaluation execution path

A scored request uses the normal `ChatPipelineService` rather than an evaluation-only inference shortcut. When it carries a seed, the router has already written the seeded investigation (see [Seeded investigations](#seeded-investigations)) before the chat task starts:

1. g8ee performs triage and records the triage model telemetry in the assignment trace.
2. For the `model_role` lane, `apply_homogeneous_role_control` runs after triage. It records the designated role, the natural role implied by triage, whether they agree, and the triage complexity. `primary` dynamically activates `ReasoningAgent.SAGE` with `SagePersona`; `assistant` and `lite` activate `ReasoningAgent.DASH` with `DashPersona`, with model resolution taken from the designated tier and its request overrides, with no cross-tier fallback.
3. The designated role controls the scored model selection even when triage would normally select another tier. A `lite` assignment always resolves the lite tier; the normal simple/complex routing rule does not override that assignment.
4. The system lane does not apply homogeneous role control and follows normal triage routing: complex turns use the primary Sage path, and other turns use the assistant Dash path.
5. The full production tool set for the agent mode is declared to the scored model, for any model tag, registered or not. The request's `evaluation_context` lifts the static `supports_tools` gate that production chat applies (`tool_gate: bypassed_for_eval`); no capability probe, table, or label withholds tools or alters a verdict. The tool names actually sent to the provider are recorded on each model call as `tools_declared`. If the provider itself rejects the declaration, g8ee records a `provider_tool_rejection` and finalizes the trace as `failed`; that is an explicit scored reason, not an infrastructure error.
6. The sequential ReAct loop (`G8eAgent._stream_with_tool_loop`) records model calls and tool activity against the bound remote Data Operator without synthetic mocks. Both Tier 1 fast smoke gate assignments (`--gate-smoke`) and full qualification assignments drive this identical production ReAct loop. Governed operator tool calls retain the bound Operator ID and session ID, execution binding, and receipt status in the trace. Failed tool results are classified as policy `deny` for known policy or validation blocks, or `refused` for other failures. Every tool result, including a failure, returns to the model, and the trace keeps the guidance the model was shown (see [Tool and governed-action evidence](#tool-and-governed-action-evidence)).
   A scored run does not read user-wide memories (`user_memories_suppressed`), and when the tool loop reaches `AGENT_MAX_TOOL_TURNS` it ends immediately (`tool_turn_limit_reached`) instead of waiting for a human to approve continuing (INV-EVAL-SEED-04).
7. Before finalizing the trace, g8ee waits for the background memory-generation task, subject to its evaluation barrier timeout, and includes its model telemetry when available. The memory update reports `agent_role: codex`; the Go importer excludes it from the scored inference span and from latency and token aggregates. A memory-task failure or timeout still finalizes the trace as `completed`.
8. For `semantic_judge`, g8ee invokes the evaluation judge after the interaction completes, then finalizes the trace. Deterministic assignments do not invoke the semantic judge.

The Tribunal, Marshal, and Auditor remain application-layer behavior in the normal agent path. Tribunal agreement is not protocol L2 consensus and does not authorize a campaign action. Governed tool execution and its authoritative receipt remain owned by the Gateway and target Operator.

## Trace persistence and retrieval

`EvaluationTraceService` stores one JSON trace per assignment and evaluation attempt at:

```text
<runtime-dir>/data/evaluation/traces/<assignment-id>/<evaluation-attempt-id>.json
```

`<runtime-dir>` is the `--runtime-dir` launch argument (`/root/.g8e` in the unified Compose stack); when it is not given, g8ee uses the project runtime `.g8e` directory. Assignment and attempt IDs are validated as safe filenames before filesystem access. Trace writes use canonical JSON and an atomic temporary-file replacement. The digest is computed with the shared `g8e.eval.v1` chat-probe trace-digest implementation over the trace with its own `trace_digest` field cleared. Loading validates the typed trace and rejects a digest mismatch.

The authenticated, read-only lookup is:

```text
GET /api/v1/evaluation/trace/{assignment_id}/{evaluation_attempt_id}
```

The response is `{ "trace": <typed trace> }`. Missing traces return a not-found response, and unsafe path parameters are rejected. The Go campaign client polls this endpoint after submitting the chat request and imports the trace into the campaign's run-scoped evidence; g8ee does not write the Go campaign run store.

A trace has schema version `7` and can contain:

- evaluation context (including the echoed `seed` and `workspace`) and the g8ee chat execution ID;
- `player_steps`, the chain: what each player produced for its own job, in the order the chain ran (INV-EVAL-TRACE-05). The Auditor and Marshal steps carry the tier their call resolved from; the Auditor runs on the primary-tier model and is attributed `primary`, or `lite` when the Tribunal had to fall back to the lite provider;
- `seed_application`, the counts of what the seed actually wrote (`turns`, `history_events`, `case_memory`), which proves the investigation was seeded and not merely requested;
- `user_memories_suppressed` and `tool_turn_limit_reached`, the two eval-only divergences besides `tool_gate`;
- triage and model-call telemetry;
- controlled-role assignment and `invoked` or `role_not_invoked` outcome;
- the designated-role output;
- per model call, `tools_declared`: the tool names as actually sent to the provider on that call (captured at the provider boundary, never recomputed from the registry; absent when the provider reports nothing, `[]` when the call declared none), including a call the provider refused; and `tool_gate`, which is `bypassed_for_eval` for every scored request and `registry` only where the production table decided the set;
- `provider_tool_rejection` (the rejected model and the Gateway's public-safe reason) when the provider itself refused the tool declaration;
- model tool decisions and executed tool calls, each with the model's exact canonical-JSON `arguments_json`, `arguments_hash` (`sha256(arguments_json)`), the resolved `command`, and the canonical-JSON `result_json`, plus the guidance the model was shown for the result (`loop_turn`, `error`, `error_type`, `suggestion`, and an `error_analysis` summary);
- governed-action bindings and policy decisions;
- semantic grades and judge-call telemetry;
- finish reason, terminal status, the terminal stream `error` message for a failed assignment, completion timestamp, and trace digest.

The trace is campaign evidence, not an independent authorization record. It does not replace the Operator's local audit evidence, signed receipt, Gateway verification, or campaign verifier.

## Seeded investigations

A scored turn runs in an investigation that already has history, the way a real g8ee turn does. `InvestigationSeedService` ([ensemble/app/services/evaluation/investigation_seed.py](../../ensemble/app/services/evaluation/investigation_seed.py)) is constructed from the investigation, case, and memory services (no globals) and writes the seed in the order of INV-EVAL-SEED-01. The internal chat router calls it between inline case creation and scheduling the chat task. What the model can see follows from where each part is written:

- Conversation turns become chat contents and are shown to the model inline. `SYSTEM`-sender messages are never shown to the model, so seeds do not use them.
- History events go to the history trail. They are not inline; the model reaches them only through the real `query_investigation_context` handler (`history_trail`, `operator_actions`). A fact the model must look up therefore belongs in a history event. The summary line carries `tool_name` and `arguments_json` when present, because the history metadata has no field for them.
- Case memory is written as the case's memory record; user-wide memories are never written.

The seeded case title is the realistic scenario title; AI title generation is skipped for seeded requests so the title is never a harness label. Seed contracts and bounds are in INV-EVAL-CONTEXT-07; the Go side that builds and grades seeds is described in [Evaluations](../architecture/evals.md#scenario-fixtures-trajectories-and-grading).

## Tool and governed-action evidence

Evaluation tool evidence is collected only when `g8e_context.evaluation_context` is present. A started tool call records its name and decision ID. A completed call records the model's arguments as canonical JSON with their SHA-256 hash, the resolved command, the canonical-JSON typed result, success, execution ID, operator-tool status, and error type. These values are the source of the public `role_transcripts` extension described in [Public Spectator Architecture](../architecture/public_spectator.md).

Each completed call also records the guidance its result carried back to the model, because g8ee is a guided loop: argument-validation failures return the exception text, blocked commands return a `SECURITY VIOLATION` or `RISK_ANALYSIS_BLOCKED` result, and a failed operator command can carry an LLM error analysis. `loop_turn` is the tool-loop turn whose model response issued the call (it restarts at 1 only if an operator approves continuing past the turn limit), so the ordered `tool_calls` list shows what the model did after each correction. `error` and `suggestion` are the result's own text, `error_type` is the typed error class, and `error_analysis` is a bounded summary (`error_category`, `root_cause`, `suggested_fix`, `suggested_command`, `should_escalate`). These fields are recorded; the trace does not judge whether the model followed the guidance.

For a failed `CommandExecutionResult`, g8ee records a typed policy decision with the outcome `deny` or `refused` and the available error or denial detail. For a successful operator tool, it records the first bound Operator's ID and session ID, the execution binding, a completed receipt status, and policy outcome `allow`. These records describe what g8ee observed; they do not independently prove protocol authorization or receipt validity.

## Semantic grading

A semantic assignment includes `grading_method: "semantic_judge"` and a `gold_summary`. At finalization, `grade_campaign_assignment_semantically` builds an interaction trace from the designated-role output and tool calls, then invokes `EvalJudge` through the configured lite provider. The judge receives the user prompt, expected behavior, required and forbidden concepts/tools, and the recorded interaction summary.

The judge must return JSON with an integer score from 1 through 5 and non-empty reasoning. Scores of 3 or higher pass. Transient provider failures are retried up to three attempts with exponential backoff; invalid or empty responses and failures after retry produce an `unavailable` semantic grade rather than a fabricated score. The judge model comes from `eval_judge.model` when configured and otherwise falls back to the resolved lite model. Each judge call contributes model-call telemetry, including provider-attempt identifiers when available.

The semantic judge is a grader, not a policy gate. Its score is persisted in the g8ee trace for the Go campaign verifier and aggregate projections; it cannot approve a tool call or substitute for a required governance proof.

## Operational boundary

Campaign execution must use the enrolled Operator topology described in [Evaluations](../architecture/evals.md). g8ee routes inference and tool intents through the production governed path; it does not call Ollama directly for campaign scoring. Provider-boundary observation and storage-side model provenance are separate Go-coordinated witness paths, not g8ee trace fields or g8ee-owned verification.

For campaign startup, enrollment, witness operators, smoke workflows, and recovery, use the [Unified Docker Stack Guide](../guides/unified_stack.md). For model-role and system-lane semantics, use the canonical [Evaluations](../architecture/evals.md) architecture document.

## Testing

The g8ee evaluation implementation has focused unit coverage under `ensemble/tests/unit/services/evaluation/` and related chat-pipeline tests. The integration suite includes protocol trace-digest compatibility and trace persistence checks under `ensemble/tests/integration/test_evaluation_trace_digest_integration.py`, and the seeded-fact check under `ensemble/tests/integration/test_investigation_seed_integration.py`. Unit tests under `ensemble/tests/unit/routers/test_internal_router_evaluation_seed.py` pin the router rules (HTTP 400 without `create_case`, ordering, no title generation, production requests never touching the seed service), and `ensemble/tests/unit/services/ai/` characterizes that production chat still reads user memories and still asks for continue-approval. These tests validate g8ee trace construction, role control, tool evidence, semantic grading, and persistence; they do not replace Go campaign verification or prove platform-level evaluation results.

See [Ensemble Tests](tests.md) for component test tiers and commands.

## Related documentation

- [Evaluations](../architecture/evals.md) — Platform evaluation programs, campaign orchestration, witness roles, evidence, and verification
- [AI Agents and the g8e Governance Boundary](../architecture/agents.md) — Trust boundaries and the limits of application-layer reasoning
- [Ensemble Architecture](architecture.md) — g8ee runtime and service wiring
- [Agents](agents.md) — Persona routing, Tribunal, Marshal, Auditor, and model-loop behavior
- [LLM Providers](llm-providers.md) — Provider implementations and the governed g8e inference path
- [Unified Docker Stack](../guides/unified_stack.md) — Campaign deployment and operations
- [Documentation Guide](../devs/docs.md) — Documentation audit and ownership rules
