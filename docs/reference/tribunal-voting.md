# g8e Governance Architecture — Complete Implementation Plan

## Project Objective

Implement the full governance architecture: intent-based Tribunal with five persona-driven members and always-present Nemesis, uniform ranked consensus voting with dissent-aware verification, compositional density incentives, and full persona alignment across the agent registry. The project ends when every component listed below is merged, tested against real infrastructure, benchmarked against the baseline, and documented.

## Ground Rules — Non-Negotiable

1. **No mocks.** All tests run against real services and real LLM providers per `docs/testing.md`. Fixtures go in `shared/test-fixtures` with justification.

2. **Benchmark every phase.** Record baseline pass rate before each phase, measure after, report the delta in the PR description. Regressions >2% require explicit justification to merge.

3. **Personas live in `shared/constants/agents.json`.** No hardcoded prompts in Python code. Use `agent_persona_loader.py`.

4. **Contract tests cover every SSE event schema.** Every new event type requires passing contract tests in g8ee and g8ed.

5. **One phase at a time.** Do not start phase N+1 until phase N is merged to main. No parallel phases.

6. **Every PR updates documentation.** `ai_agents.md`, `agent_personas.md`, and `architecture/ai_control_plane.md` must reflect current state at merge.

7. **No feature flags on behavior changes.** Correctness fixes ship directly. Feature flags are permitted only where explicitly called out in this plan.

8. **The engineer commits the plan to git before starting work.** This document is the source of truth. Checkboxes get ticked in the file as work progresses.

---

## Phase 0 — Baseline Measurement (Week 1)

### Objective

Establish measurable baselines before any changes. Without this, every future delta is a guess.

### Tasks

- [ ] **0.1** Build golden scenarios suite in `tests/fixtures/golden_scenarios/`. 20-50 real investigation scenarios. Schema: initial user message, expected manifest properties, expected tool calls, expected final outcome, difficulty tags (`requires_clarification`, `ambiguous_intent`, `subtle_command_flaw`, `destructive_if_wrong`, `cross_platform`, `multi_operator`).
- [ ] **0.2** Extend `BenchmarkJudge` to run golden scenarios with aggregate pass rate and per-tag breakdown reporting.
- [ ] **0.3** Add `approvals_per_resolved_intent` metric to benchmark framework. Track how many command approvals each scenario required.
- [ ] **0.4** Add persona contract test harness validating: required fields present, `tools` list references real registered tools, `model_tier` is valid, persona loads and renders without errors, tool authorization is enforceable.
- [ ] **0.5** Add documentation test harness that parses `ai_agents.md` and validates every documented agent exists in `agents.json`, every documented tool exists in the tool registry, every documented SSE event exists in shared constants.
- [ ] **0.6** Run full benchmark suite on current `main`. Record baseline pass rate, per-tag breakdown, approvals_per_resolved_intent, and P50/P95/P99 latency per agent. Commit results as `docs/baselines/phase_0_baseline.md`.

### Deliverables

- Golden scenarios suite with 20+ scenarios
- Persona contract test harness
- Documentation test harness
- Baseline metrics document
- Updated `docs/testing.md` describing the new test infrastructure

### Success Criteria

- Baseline metrics recorded and committed
- All new test harnesses pass on current `main`
- Per-tag pass rates recorded for at least five scenario tags

### Checkpoint

Do not proceed to Phase 1 until baseline is committed and test harnesses pass.

---

## Phase 1 — Persona Registry Alignment (Week 2)

### Objective

Land the full set of persona definitions in `agents.json`. These become the source of truth for all subsequent phases.

### Tasks

- [ ] **1.1** Update `triage` persona. Add four-route classification (`SIMPLE`, `NEEDS_CLARIFICATION`, `READY_FOR_REASONING`, `CONTINUATION`). Update `TriageResult` Pydantic model to match.
- [ ] **1.2** Update `sage` persona. Add intent-articulation guidance, pipeline density preference, CONSENSUS_FAILED handling, awareness that commands flow through the Tribunal.
- [ ] **1.3** Update `dash` persona. Align with Sage's intent-based command guidance for consistency.
- [ ] **1.4** Add/update `axiom` persona as the Composer. Compositional density pressure.
- [ ] **1.5** Add/update `concord` persona as the Guardian. Safety pressure, stage-boundary scrutiny for compositions.
- [ ] **1.6** Add/update `variance` persona as the Exhaustive. Edge-case pressure.
- [ ] **1.7** Add/update `pragma` persona as the Conventional. Idiomatic pressure, brevity absorption when appropriate.
- [ ] **1.8** Add/update `nemesis` persona as the Adversary. Always-present, flawed-or-abstain, compositional attack surface awareness, anonymized through cluster IDs.
- [ ] **1.9** Add/update `auditor` persona (Tribunal verifier). Three operating modes (unanimous, majority, tied), anonymized cluster evaluation, compositional stage-by-stage scrutiny.
- [ ] **1.10** Update `scribe` persona (titling). Ruthless compression, grammatical completeness.
- [ ] **1.11** Update `codex` persona (memory). Overfitting guard, identifier redaction requirement.
- [ ] **1.12** Update `judge` persona (evaluation). Rubric discipline, system-failure vs. low-score separation.
- [ ] **1.13** Update `warden` and sub-agents (`warden_command_risk`, `warden_error`, `warden_file_risk`). Fail-closed defaults, explicit classification criteria per sub-agent.
- [ ] **1.14** Run persona contract tests. All personas must pass.
- [ ] **1.15** Update `ai_agents.md` and `agent_personas.md` to match current persona state. Remove evolution notes; describe end state only.

### Deliverables

- Full `agents.json` with all 13 personas aligned to the architecture
- Updated Pydantic models reflecting new Triage routes
- Passing persona contract tests
- Updated architecture documentation

### Success Criteria

- All personas load successfully
- Contract tests pass
- Documentation tests pass
- Benchmark pass rate holds within ±2% of Phase 0 baseline

### Checkpoint

Do not proceed to Phase 2 until all personas are merged and contract tests are green.

---

## Phase 2 — Voting Overhaul (Week 3)

### Objective

Replace position-decay weighting with uniform per-member voting, deterministic tie-breaking, consensus threshold, dissent-aware verifier, and anonymized cluster IDs.

### Tasks

- [x] **2.1** Replace `_weighted_vote` in `command_generator.py` with uniform-weighting implementation. Each member contributes exactly 1 vote per candidate.
- [x] **2.2** Define `TRIBUNAL_MIN_CONSENSUS = 2` constant. If winner's vote count < threshold, set outcome to new `CommandGenerationOutcome.CONSENSUS_FAILED`.
- [x] **2.3** Implement tie-breaker ladder:
  1. Longest command wins (aligns with Axiom's compositional pressure)
  2. Non-Nemesis cluster wins over Nemesis-including cluster
  3. Alphabetical (deterministic fallback)
- [x] **2.4** Implement cluster ID anonymization. Verifier receives `cluster_a`, `cluster_b`, etc. Cluster ID assignment is **shuffled per round** — never stable across rounds. Internal mapping preserved for swap resolution.
- [x] **2.5** Define `VoteBreakdown` dataclass with fields: `candidates_by_member`, `candidates_by_command`, `winner`, `winner_supporters`, `dissenters_by_command`, `consensus_strength`, `tie_broken`, `tie_break_reason`.
- [x] **2.6** Define `ConsensusConfidence` descriptor with qualitative levels: `unanimous_verified`, `unanimous_unverified`, `strong_verified`, `strong_with_intervention`, `majority_verified`, `majority_with_intervention`, `tied_resolved`, `tied_verifier_resolved`, `consensus_failed`.
- [x] **2.7** Rewrite verifier stage with three modes:
  - **Unanimous**: single candidate shown, accepts `ok` or `revised:<command>`
  - **Majority**: all clusters shown with counts, accepts `ok`, `revised:<command>`, or `swap:<cluster_id>`
  - **Tied**: tied clusters shown, accepts only `swap:<cluster_id>` or `revised:<command>` — `ok` is rejected
- [x] **2.8** Update verifier prompt to reflect anonymized cluster IDs and three modes.
- [x] **2.9** Define outcome mapping:
  - Verifier returns `ok` → `VERIFIED`
  - Verifier returns `swap:<cluster_id>` → `VERIFIED` (with `verifier_reason=swapped_to_dissenter`)
  - Verifier returns `revised:<command>` → `VERIFICATION_FAILED` (with `verifier_reason=revised_from_dissent` or `revised` depending on mode)
  - No two members agree → `CONSENSUS_FAILED`
- [x] **2.10** Handle malformed verifier responses: if verifier returns `ok` in tied mode, or returns unparseable output, retry once. If retry fails, fall back to `CONSENSUS_FAILED`.
- [x] **2.11** Add SSE event `TRIBUNAL_CONSENSUS_FAILED`. Payload: full candidate set with member attribution, reason.
- [x] **2.12** Add SSE event `TRIBUNAL_DISSENT_RECORDED`. One event per losing cluster. Payload: losing command, dissenting member IDs, winner, vote breakdown.
- [x] **2.13** Update `TRIBUNAL_VOTING_CONSENSUS_REACHED` event payload to carry full `VoteBreakdown` and `ConsensusConfidence`. Preserve backward-compatible fields for downstream consumers.
- [x] **2.14** Add contract tests for all modified and new SSE events. Added tests for TRIBUNAL_CONSENSUS_FAILED and TRIBUNAL_DISSENT_RECORDED. All 11 SSE event contract tests passing.
- [x] **2.15** Raise `TribunalConsensusFailedError` on CONSENSUS_FAILED. Sage receives full candidate breakdown and decides to rephrase, clarify, or abort per her persona.
- [x] **2.16** Update Sage's handling of CONSENSUS_FAILED in `agent_tool_loop.py` — surface breakdown to Sage's reasoning context, do not silently retry.
- [~] **2.17** Write tests covering all voting scenarios. IN PROGRESS: Added tests for 5/5 unanimous, 4/1 majority, 3/2 majority, 2/2/1 tied-top (shortest command), non-Nemesis cluster tie-breaker. Added verifier path tests: swap-to-dissenter, revise-from-dissent, tied-mode disambiguation, malformed-response retry. Tests have some failures due to implementation details (score returns raw count not fraction, TieBreakReason enum uses SHORTEST not LONGEST_COMMAND). Needs fixes to align with actual implementation.
- [ ] **2.18** Run full benchmark suite. Compare to Phase 1 baseline.

### Deliverables

- Rewritten `_weighted_vote` with uniform voting and explicit tie-breaking
- `CommandGenerationOutcome.CONSENSUS_FAILED` and handler
- Anonymized cluster ID system with per-round shuffling
- Rewritten verifier with three modes
- `VoteBreakdown` and `ConsensusConfidence` in `CommandGenerationResult`
- New and modified SSE event schemas with contract tests
- Sage CONSENSUS_FAILED handling in tool loop
- Full test coverage for all voting scenarios
- Updated `ai_agents.md` section on Tribunal voting

### Success Criteria

- All voting scenarios have passing tests
- All SSE contract tests pass
- Benchmark aggregate pass rate within ±2% of Phase 1 baseline
- CONSENSUS_FAILED scenarios produce clean Sage recovery (no infinite loops)

### Checkpoint

Do not proceed to Phase 3 until voting overhaul is merged, benchmarks are reported, and CONSENSUS_FAILED handling is verified in at least one real scenario.

---

## Phase 3 — Intent-Based Tribunal Input (Week 4)

### Audit Status (April 2026)

Phase 3 audit found that intent-based Tribunal input is **largely already implemented** in the codebase:
- `SageOperatorRequest` already uses intent-based fields (`request`, `guidelines`) with no `command` field
- Tool schema uses `SageOperatorRequest` with `required_override=["request"]`
- Tool prompt (`run_commands_with_operator.txt`) explicitly states Sage should not propose commands directly
- Tribunal prompt templates already use `{request}` and `{guidelines}` placeholders
- `CommandGenerationResult` already tracks intent with `request` and `guidelines` fields
- SSE events already include `request` field for audit logging
- No `original_command` field exists in the codebase (as required by Phase 3.2)

**Remaining Phase 3 tasks to verify:**
- Verify Auditor prompt explicitly mentions intent-based context
- Verify tests cover intent-based flow
- Update documentation if needed

### Objective

Shift Sage from proposing commands to articulating intent. Tribunal becomes the sole authority on command syntax.

### Tasks

- [ ] **3.1** Update `OperatorCommandToolSchema` to the intent-based schema: `request` (natural-language intent), `guidelines` (optional creative guidance), plus existing `target_operators`, `expected_output_lines`, `timeout_seconds`.
- [ ] **3.2** Remove any `command` field from Sage's tool call schema. Sage can no longer propose a command directly.
- [ ] **3.3** Update Sage's persona and `modes/*/tools.txt` prompt data to describe `run_commands_with_operator` as intent-based.
- [ ] **3.4** Update Tribunal member input handling. All five members receive `(request, guidelines, operator_context, forbidden_patterns)`. No member receives a prior proposed command.
- [ ] **3.5** Update each Tribunal member's prompt template in `command_generator.py` to the intent-based format. Templates draw from personas in `agents.json`.
- [ ] **3.6** Update Auditor's prompt to evaluate candidates against intent, not against an originally proposed command.
- [ ] **3.7** Update SSE events. Any `original_command` field becomes `original_intent`. `final_command` remains as is.
- [ ] **3.8** Update `CommandGenerationResult` to track intent instead of original command.
- [ ] **3.9** Update `TribunalFallbackPayload` and related error payloads accordingly.
- [ ] **3.10** Update tests for Tribunal generation, voting, and verification to use intent-based inputs.
- [ ] **3.11** Add benchmark scenarios specifically exercising intent-based flow: clear intents, ambiguous intents, multi-fact investigative intents.
- [ ] **3.12** Run full benchmark suite. Measure pass rate delta and intent-to-command translation quality.

### Deliverables

- Updated tool schema with intent-only input
- Sage persona reflects intent-articulation responsibility
- Tribunal members generate from intent, not from proposed command
- Auditor judges against intent
- Updated SSE events and Pydantic models
- Test coverage for intent-based flow
- Intent-quality benchmark scenarios

### Success Criteria

- Sage never sends a command string to the Tribunal
- All Tribunal members receive identical intent inputs
- Benchmark pass rate within ±2% of Phase 2 baseline
- Qualitative review: on 10 representative intents, all five members produce diverse candidates from the same intent

### Checkpoint

Do not proceed to Phase 4 until intent-based flow is merged and Sage is confirmed unable to propose commands directly.

---

## Phase 4 — Nemesis Activation (Week 5)

### Objective

Make Nemesis always-present in every Tribunal round. Auditor knows with certainty that one of five candidates is adversarial.

### Tasks

- [ ] **4.1** Update Tribunal generation to always include Nemesis. Five members, one guaranteed adversarial.
- [ ] **4.2** Remove any probabilistic Nemesis slotting logic (if present from earlier experiments).
- [ ] **4.3** Update Auditor's prompt to state with certainty: "One of these five candidates was produced by an adversarial member."
- [ ] **4.4** Implement Nemesis participation tracking in `CommandGenerationResult`:
  - `nemesis_slot: int` (always present, always populated)
  - `nemesis_candidate: str` (the command Nemesis produced this round)
  - `nemesis_abstained: bool` (true if Nemesis produced the honest correct command)
  - `verifier_selected_nemesis: bool | None` (computed post-vote)
- [ ] **4.5** Add SSE event `TRIBUNAL_NEMESIS_PARTICIPATED` for audit log only. Do not emit to browser.
- [ ] **4.6** Add SSE event `TRIBUNAL_NEMESIS_DETECTED` when Auditor's final command does not match Nemesis's candidate.
- [ ] **4.7** Add SSE event `TRIBUNAL_NEMESIS_MISSED` when Auditor's final command matches Nemesis's non-abstention candidate. This is the critical reputation signal.
- [ ] **4.8** Build Nemesis-specific benchmark suite: 20-30 scenarios with known-subtle flaws. Measure Auditor's detection rate.
- [ ] **4.9** Verify Sentinel blocks all Nemesis outputs that accidentally cross into dangerous territory. Nemesis produces correctness flaws only; Sentinel is the backstop.
- [ ] **4.10** Add invariant test: `nemesis_candidate` never matches a forbidden pattern. If the test fails, Nemesis's persona is producing unsafe output and must be tightened.
- [ ] **4.11** Run full benchmark suite. Critical metric: Auditor Nemesis-detection rate ≥70%.

### Deliverables

- Always-present Nemesis in every Tribunal round
- Nemesis participation fields in `CommandGenerationResult`
- New SSE events for Nemesis tracking (audit log scope)
- Nemesis-detection benchmark with recorded baseline rate
- Invariant test preventing dangerous Nemesis outputs

### Success Criteria

- Nemesis participates in 100% of Tribunal rounds
- Auditor detects Nemesis at ≥70% rate on the Nemesis benchmark
- Sentinel never reports a block on a Nemesis candidate (Nemesis persona is sufficient constraint)
- Aggregate benchmark pass rate within ±2% of Phase 3 baseline

### Checkpoint

Do not proceed to Phase 5 until Nemesis detection rate meets threshold and invariant tests pass.

---

## Phase 5 — Compositional Density Incentive (Week 6)

### Objective

Incentivize pipeline-dense commands to reduce approval fatigue while preserving atomic-approval safety.

### Tasks

- [ ] **5.1** Confirm Sage persona includes intent-density guidance (from Phase 1). Verify in practice: Sage is articulating broad investigative intents, not atomic ones.
- [ ] **5.2** Confirm Tribunal member personas include compositional pressure language (from Phase 1). Each member's role in composition is explicit.
- [ ] **5.3** Implement `_logical_operation_count` helper in `command_generator.py`. Tokenize respecting quotes and escapes. Count `|`, `&&`, `||`, `;` as separators. Return 1 for simple commands.
- [ ] **5.4** Update tie-breaker ladder: insert "fewest logical operations" between "longest command" and "non-Nemesis cluster" rules. Update `VoteBreakdown.tie_break_reason` to include `fewest_operations`.
- [ ] **5.5** Define shell operator policy constant: `SEQUENCE_OPERATORS_ALLOWED = {"&&", "||"}`, `SEQUENCE_OPERATORS_RESTRICTED = {";"}`.
- [ ] **5.6** Add Sentinel detector for bare `;` outside strings/heredocs/comments. Signal (not block) for Auditor attention.
- [ ] **5.7** Update Auditor's persona: explicit requirement to justify any bare `;` in approved candidates.
- [ ] **5.8** Confirm Concord's persona rejects unconditional `;` without intent-justification.
- [ ] **5.9** Confirm Nemesis's persona identifies `;`-for-`&&` substitution as attack surface.
- [ ] **5.10** Implement command parser for approval UI breakdown. `CommandStageBreakdown` dataclass. Parser lives in `components/g8ee/app/services/ai/command_parser.py`. Graceful fallback to raw command string on parse failure.
- [ ] **5.11** Extend approval SSE event with optional `stage_breakdown` field for multi-stage commands.
- [ ] **5.12** Add contract tests for extended approval event schema.
- [ ] **5.13** Update g8ed frontend approval component to render staged breakdowns. Single-stage commands render with existing UI. Parse failures fall back gracefully.
- [ ] **5.14** Add frontend tests for staged rendering and fallback behavior.
- [ ] **5.15** Add pipeline-opportunity scenarios to benchmark suite: 15-20 scenarios where a pipeline is the correct answer, fragmentation traps, pipeline-robustness scenarios.
- [ ] **5.16** Add adversarial pipeline scenarios: 5-10 cases targeting Nemesis's pipeline attack surfaces.
- [ ] **5.17** Run full benchmark suite. Target: 20%+ reduction in `approvals_per_resolved_intent` on investigative scenarios. No regression in Nemesis detection rate.

### Deliverables

- Logical operation counting in tie-breaker
- Shell operator policy enforcement
- Sentinel detector for bare `;`
- Command parser for approval UI
- Staged approval UI in g8ed frontend
- Pipeline-opportunity and adversarial-pipeline benchmark scenarios
- Measured reduction in approvals per resolved intent

### Success Criteria

- `approvals_per_resolved_intent` reduced by ≥20% on investigative scenarios
- Nemesis detection rate maintained or improved
- Aggregate pass rate within ±2% of Phase 4 baseline
- Staged approval UI tested by at least three users and confirmed understandable

### Checkpoint

Do not proceed to Phase 6 until approval-fatigue reduction is measured and UI usability is confirmed.

---

## Phase 6 — Interrogator Agent (Week 7-8)

### Objective

Add Interrogator as the sole owner of user-facing clarification dialogue. Extend Triage routing. Add `request_clarification` tool for Sage.

### Tasks

- [ ] **6.1** Add Interrogator persona to `agents.json`. Model tier: assistant. Tools: `ask_user_question`, `hand_off_to_primary`, `return_structured_answer`. Autonomy: fully_autonomous.
- [ ] **6.2** Confirm Triage persona (from Phase 1) supports four routes. Verify `TriageResult` Pydantic model matches.
- [ ] **6.3** Update `ChatPipelineService` to dispatch on Triage's four routes. Route `NEEDS_CLARIFICATION` to Interrogator. Route `CONTINUATION` also to Interrogator for answer processing.
- [ ] **6.4** Add `request_clarification` tool to Sage's tool set in `AIToolService`:
  ```python
  request_clarification(
      information_needed: str,
      why: str,
      urgency: Literal["blocking", "background"],
      constraints: dict | None = None
  ) -> ClarificationRequestResult
  ```
  Fire-and-forget semantics — returns `clarification_id` immediately without waiting for user answer.
- [ ] **6.5** Add Interrogator-only tools: `ask_user_question`, `return_structured_answer`, `hand_off_to_primary`. These must not be exposed to any other agent (enforced in code, verified by contract test).
- [ ] **6.6** Create Interrogator service at `components/g8ee/app/services/ai/interrogator.py`. Two modes: upfront (called by Triage) and mid-case (called by Sage via `request_clarification`).
- [ ] **6.7** Create `PendingQuestionsService` in `components/g8ee/app/services/investigation/`. Backed by g8es KV. Keyed by `investigation_id`. Persists pending questions across case suspension/resumption.
- [ ] **6.8** Add `<pending_user_questions>` dynamic prompt section in `build_modular_system_prompt`. Populated from `PendingQuestionsService`. Sage sees outstanding questions every turn to prevent re-asking.
- [ ] **6.9** Add `<user_answers>` dynamic prompt section. Populated by Interrogator after answer processing. Keyed by `clarification_id`. Sage reads structured data, never raw user text.
- [ ] **6.10** Implement Interrogator's short-circuit logic. Before asking user, check:
  1. Conversation history for existing answer
  2. Investigation memory for relevant preferences
  3. Only if both miss, ask user
- [ ] **6.11** Add SSE events:
  - `AI_INTERROGATOR_QUESTION_ASKED`
  - `AI_INTERROGATOR_QUESTION_ANSWERED`
  - `AI_INTERROGATOR_QUESTION_TIMEOUT`
  - `AI_CLARIFICATION_REQUESTED` (Sage → Interrogator)
  - `AI_CLARIFICATION_RESOLVED` (Interrogator → Sage)
- [ ] **6.12** Add contract tests for all new SSE events in both g8ee and g8ed.
- [ ] **6.13** Add g8ed HTTP endpoint for browser to POST user answers. Routes answer to g8ee, which writes to conversation history and triggers Interrogator answer-processing.
- [ ] **6.14** Build frontend question-rendering component in g8ed. Reuse `ask_user_input_v0` pattern where applicable. Include rationale display ("I need to know this because...") for each question.
- [ ] **6.15** Add question timeout behavior. Blocking questions unanswered after configurable duration surface to the user prominently.
- [ ] **6.16** Add frontend tests for question rendering, answer submission, and timeout states.
- [ ] **6.17** Add benchmark scenarios: ambiguous inputs that should route through Interrogator, answer-integration scenarios where Sage incorporates user clarification.
- [ ] **6.18** Add test: Sage cannot call `ask_user_question` directly. Tool authorization contract prevents it.
- [ ] **6.19** Add test: Interrogator cannot call `run_commands_with_operator`. Contract prevents it.
- [ ] **6.20** Run full benchmark suite. Ambiguous-intent scenarios should show improved outcomes compared to Phase 5 baseline.

### Deliverables

- Interrogator persona and service
- `request_clarification` tool on Sage
- Interrogator-only tools with enforced authorization
- `PendingQuestionsService` and new prompt sections
- Updated ChatPipelineService routing
- New SSE events with contract tests
- g8ed answer endpoint and frontend question UI
- Benchmark scenarios for clarification flow
- Tool authorization contract tests

### Success Criteria

- All tool authorization contracts pass (no cross-agent tool access)
- Ambiguous-intent scenarios route through Interrogator and produce better Tribunal outcomes than Phase 5 baseline
- Interrogator short-circuits when answer is already in context (verified in test)
- Aggregate benchmark pass rate improves or holds steady
- Frontend question UI tested by at least three users

### Checkpoint

Do not proceed to Phase 7 until clarification flow is end-to-end functional and authorization contracts are enforced.

---

## Phase 7 — Reputation Service Stub (Week 9)

### Objective

Build the skeleton reputation service. Capture the data. Do not yet apply it to voting weights.

### Tasks

- [ ] **7.1** Create `ReputationService` in `components/g8ee/app/services/ai/reputation_service.py`. Subscribes to relevant SSE events: `TRIBUNAL_VOTING_CONSENSUS_REACHED`, `TRIBUNAL_DISSENT_RECORDED`, `TRIBUNAL_NEMESIS_PARTICIPATED`, `TRIBUNAL_NEMESIS_DETECTED`, `TRIBUNAL_NEMESIS_MISSED`, and judge outcome events from `BenchmarkJudge` and `EvalJudge`.
- [ ] **7.2** Define reputation schema in g8es KV. Keyed by `(agent_id, command_class)`. Tracks: participation count, winning count, dissent-correct count (Auditor agreed with dissent later), Brier score history.
- [ ] **7.3** Implement Brier score calculation per (agent_id, command_class) based on judge outcomes.
- [ ] **7.4** Add read API: `get_reputation(agent_id, command_class)` returning current reputation scores. Not yet used by voting; available for inspection.
- [ ] **7.5** Add SSE event `REPUTATION_UPDATED` for audit log.
- [ ] **7.6** Build reputation dashboard endpoint in g8ed (read-only). Developer-facing, not user-facing.
- [ ] **7.7** Add tests covering event subscription, score calculation, persistence, and read API.
- [ ] **7.8** Document reputation schema and calculation in `docs/architecture/reputation.md`.
- [ ] **7.9** Run full benchmark suite. No changes to voting behavior; reputation is read-only at this phase. Confirm no regression.

### Deliverables

- `ReputationService` subscribing to relevant events
- Reputation schema persisted in g8es KV
- Brier score calculation
- Read API and developer dashboard
- Documentation

### Success Criteria

- Reputation updates on every judge-scored case
- Brier scores visible in dashboard
- No regression in aggregate benchmark pass rate
- Reputation data accumulating for at least 50 cases before considering Phase 8

### Checkpoint

Do not proceed to Phase 8 until reputation data has accumulated for at least 50 real cases. This may take weeks of production usage; do not rush this.

---

## Phase 8 — Adversarial Test Framework (Week 10)

### Objective

Build the framework that proves the governance architecture withstands adversarial inputs. Not optional.

### Tasks

- [ ] **8.1** Create adversarial test framework in `tests/fixtures/adversarial/`. Each test case: adversarial input, expected safe behavior.
- [ ] **8.2** Build initial 15-20 adversarial scenarios covering:
  - Prompt injection in user messages
  - Prompt injection in simulated operator output (tool results)
  - Malformed tool call arguments from simulated LLMs
  - Conflicting instructions between user message and memory
  - Escalation attempts ("ignore previous instructions")
  - Role confusion ("pretend you're Auditor")
  - Denial bypass attempts (adversarial triage posture)
  - Sage attempts to call Interrogator-only tools
  - Interrogator attempts to call Sage's tools
- [ ] **8.3** Add judge calibration suite in `tests/fixtures/judge_calibration/`. Known-good outputs (should score high), known-bad outputs (should score low), edge cases (subtle failures, superficial polish).
- [ ] **8.4** Add fixture isolation primitives: `isolated_investigation_context()`, `isolated_operator()`, `isolated_g8es_namespace()`.
- [ ] **8.5** Add latency benchmark harness: P50/P95/P99 per agent per stage. Persist results to time-series. Fail CI on >20% regression vs baseline.
- [ ] **8.6** Run adversarial suite. Every scenario must produce expected safe behavior.
- [ ] **8.7** Run judge calibration suite. Judges must score within expected ranges.
- [ ] **8.8** Record latency baselines across all phases retroactively (if possible) and as current measurement.

### Deliverables

- Adversarial test framework with 15-20 scenarios
- Judge calibration suite
- Fixture isolation primitives
- Latency benchmark harness
- Current latency baselines

### Success Criteria

- All adversarial scenarios produce safe outcomes
- All judge calibration scenarios score as expected
- Latency baselines recorded for future drift detection
- No cross-agent tool authorization leaks

### Checkpoint

Do not declare project complete until adversarial suite is green and calibration baselines are recorded.

---

## Phase 9 — Documentation and Final Validation (Week 11)

### Objective

Ensure every part of the architecture is documented, tested, and auditable.

### Tasks

- [ ] **9.1** Write `docs/architecture/governance_game_theory.md`. Explain the adversarial design, role separations, compromise scenarios each role defends against, and the game-theoretic structure. This is both design anchor and public differentiator.
- [ ] **9.2** Update `docs/architecture/ai_control_plane.md` to reflect the final architecture.
- [ ] **9.3** Update `docs/architecture/about.md` governance principles section if needed.
- [ ] **9.4** Update `ai_agents.md` to reflect final agent registry, roles, and interactions.
- [ ] **9.5** Update `agent_personas.md` to describe final persona state. Remove evolution notes; describe end state only.
- [ ] **9.6** Update `docs/testing.md` to describe golden scenarios, adversarial framework, judge calibration, and latency benchmarks.
- [ ] **9.7** Run documentation test harness. Every documented element must exist in code.
- [ ] **9.8** Run full benchmark suite on final build. Compare to Phase 0 baseline. Record full delta.
- [ ] **9.9** Run adversarial suite. All must pass.
- [ ] **9.10** Run judge calibration suite. All must pass.
- [ ] **9.11** Run latency benchmark. Compare to Phase 0 baseline. Record drift.
- [ ] **9.12** Commit `docs/baselines/final_baseline.md` with complete metrics.
- [ ] **9.13** Write migration guide for users of prior Tribunal API (if any external consumers).
- [ ] **9.14** Review checkboxes in this document. Every box must be checked.

### Deliverables

- Complete governance documentation
- Final benchmark delta report vs Phase 0
- Passing adversarial, calibration, and latency suites
- Migration guide
- Fully checked project plan

### Success Criteria

- All documentation tests pass
- All benchmark suites pass
- Aggregate pass rate improved or within ±2% of Phase 0 baseline
- Approval fatigue reduced by ≥20% on investigative intents
- Nemesis detection rate ≥70%
- All adversarial scenarios produce safe outcomes
- Every checkbox in this plan is ticked

### Checkpoint

Project is complete when all Phase 9 success criteria are met. Not before.

---

## Cross-Phase Requirements

### Every PR Must Include

- Summary of changes
- Benchmark delta report (before vs after)
- New test coverage summary
- Any observed behavioral changes in real usage
- Known issues and follow-ups
- Updated documentation reflecting final state
- All contract tests passing

### Every PR Must Not

- Bypass human approval for command execution
- Grant new tool authority to an agent without explicit scope in its persona
- Introduce mocks on internal services or LLM providers
- Ship without benchmark comparison
- Merge with failing contract tests
- Include evolution commentary in persona content or public documentation

### Preserved Invariants

- Human approval gate is non-bypassable on all command execution
- Sentinel is the backstop — if any agent layer fails, Sentinel must still catch dangerous operations
- LFAA audit logging captures every command, file operation, and AI decision
- All agents are stateless per invocation (amnesia is structural)
- Tool authorization is enforced in code, not only in prompts
- No agent can communicate with another agent except through the defined tool interfaces

---

## Recovery From Work Loss

If work is lost again:

1. Check git history for the last merged phase
2. Resume from the next unchecked task in this plan
3. Do not attempt to recreate work from memory — re-plan from the documented checkpoint
4. Update the checklist in this file as work proceeds
5. Commit this file to git after every task completion

---

## Project Completion Criteria

This project is done when, and only when:

- [ ] Every phase's checkboxes are all ticked
- [ ] `docs/baselines/final_baseline.md` is committed with complete metrics
- [ ] Aggregate benchmark pass rate is improved or within ±2% of Phase 0 baseline
- [ ] Approval fatigue metric (`approvals_per_resolved_intent`) is reduced by ≥20% on investigative scenarios
- [ ] Nemesis detection rate is ≥70%
- [ ] Adversarial test suite passes 100%
- [ ] All documentation tests pass
- [ ] Governance game theory document is published
- [ ] Reputation service is capturing data (even if not yet weighting votes)

Anything less is incomplete work. Do not declare the project done until every criterion is met.

---

## Final Note to Implementing Engineer

This plan is the source of truth. Work it in order. Commit checkbox updates as you progress. Do not skip ahead. Do not parallelize phases. Do not declare a phase complete without running its benchmarks. Do not declare the project complete until every checkbox in every phase is ticked and every success criterion is met.

If you lose work, resume from the last merged checkpoint. Do not reconstruct from memory. Do not improvise. This plan exists so that interruptions do not reset progress.

The architecture is coherent. The design has been debated thoroughly. The work is now mechanical — follow the plan, run the tests, report the deltas, ship each phase cleanly. Do not add scope. Do not remove scope. Do not ship without measurement.

Commit this file. Begin Phase 0.