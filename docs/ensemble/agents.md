# Agents

## Scope and boundary

g8ee is an optional application-layer client. Its personas, model reasoning, Tribunal agreement, Marshal analysis, memories, reputation, and application approvals express intent or telemetry; they do not authorize a host or platform mutation. Host-command requests are relayed as typed `CommandIntent` messages to the selected Operator, while designated application-record writes use the Gateway governance endpoint. The Gateway and executing Operator enforce the active five-layer protocol posture. See [AI Agents and the g8e Governance Boundary](../architecture/agents.md) and [Ensemble Architecture](../architecture/ensemble.md).

This page documents the registered persona models and the application pipeline that uses them. A registered persona is not necessarily invoked on every chat turn.

## Persona models and registry

Personas are immutable Pydantic models implemented under `ensemble/app/models/personas/` and derived from `AgentPersonaModel` in `ensemble/app/models/personas/base.py`. `ensemble/app/models/personas/__init__.py` constructs the process-local `PERSONA_REGISTRY`, and `ensemble/app/utils/agent_persona_loader.py` exposes validated `AgentPersona` views through `get_agent_persona()`, `get_tribunal_member()`, and `list_all_agents()`. An unknown ID raises `KeyError`.

The base model contains these fields:

- **`id`** — Registry key and stable persona identifier.
- **`display_name`**, **`icon`**, and **`description`** — Presentation metadata used by application surfaces and agent-state projections.
- **`role`** — Functional classification such as `classifier`, `reasoner`, `responder`, `tribunal_member`, `arbitrator`, `auditor`, `defender`, `summarizer`, `analyzer`, or `evaluator`.
- **`model_tier`** — Logical model role such as `primary`, `assistant`, or `lite`. The concrete provider and model are resolved from user and platform settings; the tier is not itself a provider identity.
- **`tools`** — Declared tool names. Triage and the collective personas have no tool list; Sage and Dash declare the operator and investigation tools available to their reasoning prompts.
- **`capabilities`** — Declared persona capabilities. The five Tribunal members declare `local_syntax_check`; a declaration is an upper bound that the pipeline intersects with settings and resolved-model support before enabling behavior.
- **`identity`**, **`purpose`**, **`autonomy`**, and optional **`output_contract`** — Prompt content and output constraints. `get_system_prompt()` emits the canonical sequence `<role>`, optional `<output_contract>`, `<identity>`, `<purpose>`, and `<autonomy>`.

The current registry contains these IDs: `triage`, `sage`, `dash`, `tribunal`, `axiom`, `concord`, `variance`, `pragma`, `nemesis`, `auditor`, `marshal`, `marshal_command`, `marshal_error`, `marshal_file`, `scribe`, `codex`, and `judge`. The registry is the implementation source for this roster; prompt scaffolding and structured response schemas are assembled by the owning services rather than stored entirely in persona models.

## Chat routing personas

- **Triage (`triage`)** — A `lite` classifier that returns `TriageResult`: complexity, confidence, intent, intent confidence, intent summary, request posture, and posture confidence. Security-sensitive requests involving authentication, credentials, permissions, account access, password resets, user management, or security configuration are forced to `complex`. Attachments and empty messages are also escalated by the triage service; an empty message receives `unknown` intent. Triage does not ask questions or call tools. The chat pipeline selects Sage for `complex` turns and Dash for other turns, unless evaluation role control supplies a controlled assignment.
- **Sage (`sage`)** — The `primary` reasoning persona for complex turns. It plans investigations, interprets tool results, synthesizes evidence, and composes the user-facing response. Its `SageOperatorRequest` describes the intended result and command-shape constraints in natural language; it has no `command` field. The Tribunal creates the command later. Sage owns the interrogation protocol for complex turns and must emit an `<interrogation>` block containing exactly three binary YES/NO questions when required context is missing.
- **Dash (`dash`)** — The `assistant` fast-path persona for non-complex turns. It answers from available context or makes a targeted tool call and escalates conceptually to Sage when the request needs deeper, multi-step reasoning. Dash also owns interrogation for turns routed to the fast path and uses the same exactly-three binary-question block; the tool loop suppresses execution while the application waits for answers.

Sage and Dash currently declare the same tool surface, including `run_commands_with_operator`, file operations, detailed file listing, port checks, intent permission changes, file history and diff, web search, and investigation-context queries. Their distinction is routing, model tier, and prompt behavior, not an empty-versus-full tool list.

## Tribunal command generation

The Tribunal is represented by the `tribunal` persona (`role="arbitrator"`, `model_tier="lite"`) and is implemented by `ensemble/app/services/ai/generator.py` and `ensemble/app/services/ai/tribunal/`. The collective persona is documentation and state-projection metadata; the five generation seats are the actual independent model passes:

- **Axiom (`axiom`)** — Composition. Produces a coherent command or pipeline that fulfills the complete intent in one invocation.
- **Concord (`concord`)** — Safety. Favors bounded, read-only, defensive, explicitly scoped commands, safe quoting, `&&`, and failure propagation where appropriate.
- **Variance (`variance`)** — Edge cases. Accounts for spaces, null-delimited filenames, symlinks, missing directories, binary data, locales, and other plausible environmental hazards.
- **Pragma (`pragma`)** — Convention. Uses idiomatic tools and flags for the target operating system, shell, and ecosystem.
- **Nemesis (`nemesis`)** — Calibrated adversary. Produces a plausible semantic flaw when one can be introduced without becoming dangerous, and otherwise emits the honest command. It does not produce destructive commands.

The default setting requests five passes, one for each seat (`llm_command_gen_passes`, default `5`). The setting can change the number of passes; seats repeat cyclically when more than five passes are requested. Each pass receives the same intent, guidelines, operator context, constraints, and its own member system prompt. Generation runs in parallel. Responses are normalized, structurally parsed when the model supports structured output, and rejected when command safety validation fails. A successful seat emits one command candidate without commentary.

Voting is uniform: each successful candidate contributes one vote. A command needs at least two votes (`TRIBUNAL_MIN_CONSENSUS = 2`). A unique top candidate wins; ties use deterministic shortest-command and non-Nemesis tie breakers, then trigger another anonymized generation and voting round if unresolved. If the final round cannot reach the threshold, the Tribunal returns a consensus failure and no command is executed.

The Tribunal's model agreement is application-level reasoning. It is not protocol L2 consensus, does not produce Ed25519 votes, and does not authorize execution.

## Marshal and Auditor stages

After a voting winner exists, the pipeline invokes Marshal risk analysis before the Auditor. Marshal is application-layer analysis and can be unavailable when no analyzer is configured. A `HIGH` command-risk result blocks the command. The first block records investigation feedback and asks the reasoning loop to propose a safer alternative; a second consecutive block emits an agent-conflict event and requires human intervention. A successful command resets the investigation's Marshal block count.

The registered Marshal personas are:

- **Marshal (`marshal`)** — Coordinates the consolidated pre-execution risk signal and emits risk and error-handling classifications.
- **Command Risk Analyzer (`marshal_command`)** — Classifies command blast radius, reversibility, and failure consequence as `LOW`, `MEDIUM`, or `HIGH`, failing closed to `HIGH` when analysis is inconclusive.
- **Error Analyzer (`marshal_error`)** — Classifies command failures as `AUTO_FIXABLE`, `ESCALATE`, or `RETRY_LIMIT`, with ambiguous failures escalating and a default retry budget of two.
- **File Operation Risk Analyzer (`marshal_file`)** — Classifies file-operation risk as `LOW`, `MEDIUM`, or `HIGH` using path sensitivity, reversibility, Git state, and backup availability. File-operation services use this analysis when evaluating operator file mutations.

The Auditor is controlled by `llm_command_gen_auditor`, enabled by default. When disabled, the voting winner passes through the Auditor stage without an Auditor model call. When enabled, a `primary`-tier model reviews anonymized candidate clusters against the original intent and command constraints. Unanimous mode permits `ok` or `revised`; majority mode permits `ok`, `revised`, or `swap`; tied mode forbids `ok` and requires `revised` or `swap`. Revisions and swaps are normalized and run through command safety validation again. When the enabled Auditor approves a result, the pipeline also creates the application reputation commitment used by the later stake-resolution path; the disabled-Auditor pass-through does not perform an Auditor model call or create that commitment.

## Support and evaluation personas

- **Scribe (`scribe`)** — A `lite` summarizer that generates a specific three-to-seven-word case title from the initial user message. It emits only the title and does not approve actions.
- **Codex (`codex`)** — A `lite` analyzer used by `MemoryGenerationService` to extract durable user preferences and scrubbed investigation summaries. It must redact hostnames, IP addresses, credentials, and other identifiers before the resulting `InvestigationMemory` is used in later prompts.
- **Judge (`judge`)** — A `primary`-tier evaluator whose authority is reputational, not operational. Evaluation services use Judge behavior to score agent responses against rubric dimensions; it does not gate a production command or create protocol authorization.

These support personas operate on application records, memory, evaluation, and telemetry. Those records remain outside the Gateway and Operator execution boundary even when g8ee persists selected results through governed application-record writes.

## End-to-end command flow

For a host-command tool call, the current pipeline is:

1. Triage classifies the user turn. The chat pipeline selects Dash or Sage and builds the reasoning prompt with investigation context and memories.
2. The selected reasoning persona may request `run_commands_with_operator` using `SageOperatorRequest`, which contains intent and constraints but no shell command.
3. `TribunalInvoker` resolves the selected Operator context and command-validation settings, then runs Tribunal generation, voting, and any second round.
4. Marshal analyzes the voting winner. A high-risk block stops this attempt before Auditor review.
5. Auditor review is performed when enabled. The final command is normalized and revalidated before it is placed in `ExecutorCommandArgs`.
6. The operator tool executor sends the typed internal request through the g8ee command path. At the pub/sub boundary, g8ee serializes `CommandIntent` for the exact Operator and session; the Gateway constructs the canonical envelope and the target Operator independently performs L1-L4 and L5 execution.
7. The result returns to the sequential ReAct loop. The model can request another tool turn until it stops or reaches `AGENT_MAX_TOOL_TURNS` (currently `25`); continuing after the limit requires a separate g8ee application approval. That approval is not protocol L3.

Application SSE events expose progress, candidate, risk, approval, and result telemetry. They do not authorize execution or replace the Operator's authoritative receipt and audit evidence. Application approvals and reputation outcomes have the same limitation.

## Related

- [Platform Agents](../architecture/agents.md) — Platform agent concepts, ingress paths, and governance boundary.
- [Governance Pipeline](../architecture/governance.md) — Canonical five-layer verification and posture behavior.
- [Architecture](architecture.md) — g8ee runtime, services, and model hierarchy.
- [Governance](governance.md) — g8ee integration limits and envelope paths.
- [Prompts](prompts.md) — System prompt assembly and persona templating.
- [Thinking](thinking.md) — Provider reasoning tokens and thought signatures.
- [Evals](evals.md) — Benchmark and evaluation behavior.
- [Documentation Guide](../devs/docs.md) — Documentation audit and ownership rules.
