---
doc_id: ensemble-agents
title: Agent Personas
audience: maintainers and coding agents
status: current
last_updated: 2026-09-28
version: v2.2.3
owners:
  - ensemble/app/models/personas/
  - ensemble/app/services/ai/
  - ensemble/app/utils/agent_persona_loader.py
related:
  - ../architecture/agents.md
  - ../architecture/ensemble.md
  - ../architecture/governance.md
  - architecture.md
  - governance.md
  - prompts.md
  - thinking.md
  - evals.md
  - ../devs/docs.md
when_to_read: Understanding agent personas, the Tribunal consensus mechanism, command generation pipeline, risk analysis stages, or agent-layer reasoning and auditing.
do_not_use_for:
  - Protocol-layer governance (docs/architecture/governance.md)
  - Platform-level agent concepts (docs/architecture/agents.md)
  - System prompt assembly details (ensemble/prompts.md)
  - Evaluation and scoring (ensemble/evals.md)
---

# Agent Personas

## Purpose

Documents the registered agent personas, the Tribunal consensus mechanism for command generation, Marshal risk analysis, Auditor review, and the end-to-end command flow within the ensemble application layer. These personas and processes express reasoning intent and telemetry; they do not authorize host or platform mutations. Authorization flows through the Gateway and Operator's five-layer protocol posture, which operates independently of ensemble's application-layer models and voting.

## Quick index

- [Purpose](#purpose)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Persona models and registry](#persona-models-and-registry)
- [Chat routing personas](#chat-routing-personas)
- [Tribunal command generation](#tribunal-command-generation)
- [Marshal and Auditor stages](#marshal-and-auditor-stages)
- [Support and evaluation personas](#support-and-evaluation-personas)
- [End-to-end command flow](#end-to-end-command-flow)
- [Procedures](#procedures)
- [Links out](#links-out)

## Invariants

Persona models are immutable Pydantic classes. Each persona receives a stable registry key at construction. New personas are added to `PERSONA_REGISTRY` in [ensemble/app/models/personas/__init__.py](ensemble/app/models/personas/__init__.py) and must include all required base fields. Application logic never constructs personas at runtime; all personas are loaded from the registry through the validated loader in [ensemble/app/utils/agent_persona_loader.py](ensemble/app/utils/agent_persona_loader.py).

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Persona model definitions | `ensemble/app/models/personas/` | Import and registry construction |
| Base persona class | `ensemble/app/models/personas/base.py` | `AgentPersonaModel` fields and `get_system_prompt()` |
| Registry and loader | `ensemble/app/models/personas/__init__.py`, `ensemble/app/utils/agent_persona_loader.py` | `PERSONA_REGISTRY`, `get_agent_persona()`, `list_all_agents()` |
| Tribunal generation and voting | `ensemble/app/services/ai/generator.py`, `ensemble/app/services/ai/voter.py` | `TRIBUNAL_MIN_CONSENSUS = 2`, `weighted_vote()` |
| Marshal and Auditor stages | `ensemble/app/services/ai/tribunal/stages/` | Generation, voting, marshal, and auditor implementations |

## Scope and boundary

Ensemble exposes agent personas, model reasoning, Tribunal consensus, Marshal risk analysis, and application-layer memories and approvals. These express intent or telemetry; they do not authorize a host or platform mutation. Host-command requests dispatch through `GatewayOperatorClient` to `POST /api/v1/operators/commands` with a registered request `event_type`. The Gateway and executing Operator enforce the five-layer protocol posture independently.

This page documents the registered personas and the application pipeline that uses them. A registered persona is not necessarily invoked on every chat turn.

## Persona models and registry

Personas are immutable Pydantic models defined under [ensemble/app/models/personas/](ensemble/app/models/personas/) and derived from `AgentPersonaModel` in [ensemble/app/models/personas/base.py](ensemble/app/models/personas/base.py). The process-local `PERSONA_REGISTRY` in [ensemble/app/models/personas/__init__.py](ensemble/app/models/personas/__init__.py) constructs and stores all personas. The loader [ensemble/app/utils/agent_persona_loader.py](ensemble/app/utils/agent_persona_loader.py) exposes validated `AgentPersona` views through `get_agent_persona()`, `get_tribunal_member()`, and `list_all_agents()`. An unknown ID raises `KeyError`.

The base model contains these fields:

- **`id`** — Registry key and stable persona identifier.
- **`display_name`**, **`icon`**, and **`description`** — Presentation metadata used by application surfaces and agent-state projections.
- **`role`** — Functional classification: `classifier`, `reasoner`, `responder`, `arbitrator`, `auditor`, `analyzer`, `summarizer`, or `evaluator`.
- **`model_tier`** — Logical model tier: `primary`, `assistant`, or `lite`. The concrete provider and model are resolved from user and platform settings; the tier is not itself a provider identity.
- **`tools`** — Declared tool names. Triage and Tribunal members have an empty list; Sage and Dash declare the operator and investigation tools available to their reasoning.
- **`capabilities`** — Declared persona capabilities as a frozenset of `PersonaCapability` enums. A declaration is an upper bound that the pipeline intersects with settings and resolved-model support before enabling behavior.
- **`identity`**, **`purpose`**, **`autonomy`**, and optional **`output_contract`** — Prompt content and output constraints. `get_system_prompt()` emits the canonical XML sequence: `<role>`, optional `<output_contract>`, `<identity>`, `<purpose>`, and `<autonomy>`.

The registry contains these IDs: `triage`, `sage`, `dash`, `tribunal`, `axiom`, `concord`, `variance`, `pragma`, `nemesis`, `auditor`, `marshal`, `marshal_command`, `marshal_error`, `marshal_file`, `scribe`, `codex`, and `judge`. The registry is the implementation source for this roster. Prompt scaffolding and structured response schemas are assembled by the owning services rather than stored entirely in persona models.

## Chat routing personas

- **Triage (`triage`)** — A `lite` classifier that classifies user turns and returns a `TriageResult` with: complexity classification and confidence, intent classification and confidence, intent summary, request posture, and posture confidence. Security-sensitive requests involving authentication, credentials, permissions, account access, password resets, user management, or security configuration are forced to `complex`. Attachments and empty messages are also escalated; empty messages receive `unknown` intent. Triage does not ask questions or call tools. The chat pipeline selects Sage for `complex` turns and Dash for other turns, unless evaluation control supplies a controlled assignment.

- **Sage (`sage`)** — The `primary` reasoning persona for complex turns. Sage plans investigations, interprets tool results, synthesizes evidence, and composes user-facing responses. When Sage needs a shell command, it emits a `SageOperatorRequest` that describes the intended result and command-shape constraints in natural language; the request has no `command` field. The Tribunal generates the final command from Sage's intent. Sage owns the interrogation protocol and emits an `<interrogation>` block containing exactly three binary YES/NO questions when required context is missing.

- **Dash (`dash`)** — The `assistant` fast-path persona for non-complex turns. Dash answers from available context or makes a targeted tool call and escalates to Sage when the request needs deeper, multi-step reasoning. Dash owns interrogation for turns routed to the fast path, using the same exactly-three binary-question block; the tool loop suppresses execution while the application waits for answers.

Sage and Dash declare the same tool surface: `run_commands_with_operator`, file operations, file listing, port checks, permission changes, file history and diffs, web search, and investigation-context queries. Their distinction is routing tier (complex vs. non-complex), model tier, and prompt behavior, not tool availability.

## Tribunal command generation

The Tribunal is represented by the `tribunal` persona and is implemented by [ensemble/app/services/ai/generator.py](ensemble/app/services/ai/generator.py) and [ensemble/app/services/ai/tribunal/](ensemble/app/services/ai/tribunal/). The collective persona is metadata for state projections; the five generation seats are the actual independent model passes:

- **Axiom (`axiom`)** — Composition. Produces a coherent command or pipeline that fulfills the complete intent in one invocation.
- **Concord (`concord`)** — Safety. Favors bounded, read-only, defensive, explicitly scoped commands, safe quoting, failure-safe operators like `&&`, and appropriate failure propagation.
- **Variance (`variance`)** — Edge cases. Accounts for spaces, null-delimited filenames, symlinks, missing directories, binary data, locales, and other plausible environmental hazards.
- **Pragma (`pragma`)** — Convention. Uses idiomatic tools and flags for the target operating system, shell, and ecosystem.
- **Nemesis (`nemesis`)** — Calibrated adversary. Produces a plausible semantic flaw when one can be introduced without becoming dangerous. It emits the honest command otherwise. Nemesis does not produce destructive commands.

The `llm_command_gen_passes` setting (default `5`) controls the number of generation passes. The setting can request more than five passes; seats repeat cyclically. Each pass receives the same intent, guidelines, operator context, and constraints, plus its own member system prompt. Generation runs in parallel. Responses are normalized and structurally parsed when the model supports structured output. Command safety validation rejects unsafe responses. A successful seat emits one command candidate without commentary.

Voting is uniform: each successful candidate contributes one vote to the pool. A command requires at least `TRIBUNAL_MIN_CONSENSUS = 2` votes. A unique top candidate wins; ties use deterministic tie-breaking by shortest command, then non-Nemesis over Nemesis membership, then alphabetical order. Unresolved ties trigger another anonymized generation and voting round. If the final round cannot reach consensus, the Tribunal returns a consensus failure and does not execute a command.

Tribunal consensus is application-level reasoning. It is not protocol L2 consensus, does not produce signed votes, and does not authorize execution.

## Marshal and Auditor stages

After a voting winner emerges, the pipeline invokes Marshal risk analysis before Auditor review. Marshal is application-layer analysis and can be unavailable when no analyzer is configured. A `HIGH` command-risk result blocks the command. The first block records investigation feedback and asks the reasoning loop to propose a safer alternative. A second consecutive block emits an agent-conflict event and requires human intervention. A successful command execution resets the investigation's Marshal block count.

The registered Marshal personas are:

- **Marshal (`marshal`)** — Coordinates the consolidated pre-execution risk signal and emits risk and error-handling classifications.
- **Command Risk Analyzer (`marshal_command`)** — Classifies command blast radius, reversibility, and failure consequence as `LOW`, `MEDIUM`, or `HIGH`. Ambiguous analysis defaults closed to `HIGH`.
- **Error Analyzer (`marshal_error`)** — Classifies command failures as `AUTO_FIXABLE`, `ESCALATE`, or `RETRY_LIMIT`. Ambiguous failures escalate. The default retry budget is two.
- **File Operation Risk Analyzer (`marshal_file`)** — Classifies file-operation risk as `LOW`, `MEDIUM`, or `HIGH` using path sensitivity, reversibility, Git state, and backup availability. File-operation services use this analysis when evaluating operator file mutations.

The Auditor stage is controlled by `llm_command_gen_auditor` (enabled by default). When disabled, the voting winner passes through without an Auditor model call. When enabled, a `primary`-tier model reviews anonymized candidate clusters against the original intent and command constraints. Unanimous mode permits `ok` or `revised`; majority mode permits `ok`, `revised`, or `swap`; tied mode requires `revised` or `swap`. Revisions and swaps are normalized and revalidated through command safety validation. When the Auditor approves a result, the pipeline creates the application reputation commitment for later stake-resolution. The disabled-Auditor pass-through does not perform an Auditor model call or create that commitment.

## Support and evaluation personas

- **Scribe (`scribe`)** — A `lite` summarizer that generates a three-to-seven-word case title from the initial user message. Scribe emits only the title and does not approve actions.
- **Codex (`codex`)** — A `lite` analyzer used by `MemoryGenerationService` to extract durable user preferences and scrubbed investigation summaries. Codex must redact hostnames, IP addresses, credentials, and other identifiers before the resulting `InvestigationMemory` is used in later prompts.
- **Judge (`judge`)** — A `primary`-tier evaluator whose authority is reputational, not operational. Evaluation services use Judge to score agent responses against rubric dimensions. Judge does not gate production commands or create protocol authorization.

These support personas operate on application records, memory, evaluation, and telemetry. Those records remain outside the Gateway and Operator execution boundary, even when the application persists selected results through governed application-record writes.

## End-to-end command flow

For a host-command tool call, the pipeline is:

1. Triage classifies the user turn. The chat pipeline selects Dash or Sage and builds the reasoning prompt with investigation context and memories.
2. The selected reasoning persona requests `run_commands_with_operator` using `SageOperatorRequest`, which contains intent and constraints but no shell command.
3. The Tribunal generation phase resolves the selected Operator context and command-validation settings, then runs Tribunal member passes, voting, and any second round if consensus is not reached.
4. Marshal analyzes the voting winner. A high-risk result blocks the attempt before Auditor review.
5. Auditor review occurs when enabled. The final command is normalized and revalidated before placement in `ExecutorCommandArgs`.
6. The operator tool executor sends the typed internal request through `OperatorExecutionService`, which calls `GatewayOperatorClient.dispatch()` with the registered request `event_type` and serialized protobuf payload. The Gateway validates the event, derives `action_type` from the registry, constructs the canonical envelope, and the target Operator independently performs L1-L4 and L5 execution. The HTTP response carries the correlated result envelope.
7. The result returns to the sequential ReAct loop. The model requests additional tool turns until it stops or reaches `AGENT_MAX_TOOL_TURNS` (set to `25`). Continuing after the limit requires a separate application approval, which is not protocol L3 authorization.

Application SSE events expose progress, candidate, risk, approval, and result telemetry. They do not authorize execution or replace the Operator's authoritative receipt and audit evidence. Application approvals and reputation outcomes have the same limitation.

## Procedures

### Adding a new persona

1. Create a new persona class in [ensemble/app/models/personas/](ensemble/app/models/personas/) inheriting from `AgentPersonaModel`.
2. Implement `__init__` with all required base fields: `id`, `display_name`, `icon`, `description`, `role`, `model_tier`, `tools`, `capabilities`, `identity`, `purpose`, `autonomy`, and optional `output_contract`.
3. Add an import and registry entry in [ensemble/app/models/personas/__init__.py](ensemble/app/models/personas/__init__.py).
4. Verify the persona loads through `get_agent_persona()` and appears in `list_all_agents()`.

### Verifying Tribunal consensus behavior

Run `ensemble/app/services/ai/voter.py` tests to verify tie-breaking and consensus mechanisms. The tie-breaker ladder is: shortest command, non-Nemesis over Nemesis, then alphabetical order. Consensus requires at least `TRIBUNAL_MIN_CONSENSUS = 2` votes.

### Auditing prompt assembly

System prompts are assembled by `get_system_prompt()` in canonical order: `<role>`, optional `<output_contract>`, `<identity>`, `<purpose>`, `<autonomy>`. Verify the layout through test assertions in `ensemble/tests/`.

## Links out

- [Platform Agents](../architecture/agents.md) — Platform agent concepts, ingress paths, and governance boundary.
- [Governance Pipeline](../architecture/governance.md) — Canonical five-layer verification and posture behavior.
- [Architecture](architecture.md) — Ensemble runtime, services, and model hierarchy.
- [Governance](governance.md) — Ensemble integration limits and envelope paths.
- [Prompts](prompts.md) — System prompt assembly and persona templating.
- [Thinking](thinking.md) — Provider reasoning tokens and thought signatures.
- [Evals](evals.md) — Benchmark and evaluation behavior.
- [Documentation Guide](../devs/docs.md) — Documentation audit and ownership rules.
