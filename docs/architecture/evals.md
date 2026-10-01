---
doc_id: evals
title: Evaluation Programs
audience: maintainers and coding agents
status: current
last_updated: 2026-09-30
version: v2.2.4
owners:
  - internal/services/evaluation/
  - internal/cli/cmd/eval/
  - eval/
related:
  - gateway.md
  - operator.md
  - model-provenance.md
  - public_spectator.md
  - ensemble.md
  - governance.md
  - ../guides/unified_stack.md
  - ../guides/sovereignty_gauntlet.md
  - ../core/position_paper.md
when_to_read: Understanding g8e's evaluation programs, native execution-boundary test suites, model campaign scoring through real inference paths, provider-boundary observation, model provenance attestation, evidence persistence and verification, and the topology of witness operators separate from execution.
do_not_use_for:
  - Governance five-layer interlock and policy enforcement (governance.md)
  - Gateway HTTP routes and session routing (gateway.md)
  - Operator execution boundary and native tool catalog (operator.md)
  - Model weight zero-trust attestation (model-provenance.md)
  - Public evaluation projections and explorer contract (public_spectator.md)
---

# Evaluation Programs

## Purpose

Defines g8e's platform evaluation programs: the native execution-boundary suite and model campaign scoring. These programs prove governance posture, isolated inference dispatch, real model invocations through production paths, provider-boundary hardware telemetry, and storage-side model weight attestation. Both programs combine read-only witness collection with governed execution through the Gateway and Operator architecture, and persist canonical content-addressed evidence for deterministic verification independent of execution.

## Quick index

- [Purpose](#purpose)
- [Quick index](#quick-index)
- [Invariants](#invariants)
- [Owned surfaces](#owned-surfaces)
- [Procedures](#procedures)
  - [Topology: Confidential Edge Execution with Governed State Mutation](#topology-confidential-edge-execution-with-governed-state-mutation)
  - [Evaluation programs](#evaluation-programs)
  - [CLI surface](#cli-surface)
  - [Native execution-boundary suite](#native-execution-boundary-suite)
  - [Model campaign evaluations](#model-campaign-evaluations)
  - [Witness operator roles](#witness-operator-roles)
  - [Evidence and verification](#evidence-and-verification)
- [Anti-patterns](#anti-patterns)
- [Links out](#links-out)

Key invariant groups: [Program Definitions](#program-definitions-inv-eval-prog), [Campaign Structure](#campaign-structure-inv-eval-camp), [Witness Separation](#witness-separation-inv-eval-wit), [Evidence Storage](#evidence-storage-inv-eval-evid), [Verification Posture](#verification-posture-inv-eval-verif).

## Invariants

Ids are stable. Append the next free number within each group; do not renumber.

### Program Definitions (`INV-EVAL-PROG`)

| ID | Rule |
| --- | --- |
| INV-EVAL-PROG-01 | The native execution-boundary suite (`core-execution-boundary@1.0.0`) MUST NOT use g8ee, external model providers, model judges, campaigns, scheduling, or synthetic simulators. |
| INV-EVAL-PROG-02 | Model campaigns MUST score inference through the production g8ee `POST /api/v1/chat` path with real models and real tool dispatch to governed Data Operator sessions. |
| INV-EVAL-PROG-03 | All evaluation run evidence MUST be persisted as content-addressed digests in `.g8e/data/eval/runs/<run-id>/` with canonical protobuf JSON, separate from campaign definitions in `.g8e/data/eval/campaigns/<campaign-id>/`. |
| INV-EVAL-PROG-04 | Campaign run verification through `g8e eval runs verify` and native boundary verification through `g8e eval boundary verify` MUST recompute all bindings and signatures without executing new scored actions. |

### Campaign Structure (`INV-EVAL-CAMP`)

| ID | Rule |
| --- | --- |
| INV-EVAL-CAMP-01 | Model campaigns bind scored inference to frozen `served_model_tag` and `model_digest` pairs registered in the campaign's frozen model inventory. Placeholder digests lacking provenance authority MUST be re-frozen from the provider before scored runs. |
| INV-EVAL-CAMP-02 | The default campaign lane is `model_role`, which schedules each frozen model variant against catalog scenarios and records role-specific results. The `system` lane supports heterogeneous multi-model formations with deterministic per-formation binding. |
| INV-EVAL-CAMP-03 | On `g8e eval rollout run`, strict witness verification MUST default true and MUST fail closed when coverage is missing and `--require-witness` is active. On `g8e eval runs start`, witness requirements remain opt-in via `--require-observation`, `--require-provenance`, or `--require-witness`. |
| INV-EVAL-CAMP-04 | Homogeneous campaigns MUST apply authentic production agent personas (Sage, Dash) bound to real ReAct agentic loops in the Data Operator, not synthetic harnesses or mock loops. |
| INV-EVAL-CAMP-05 | Each catalog scenario MUST declare `eligible_roles`: the model roles that perform that task in g8ee (`ensemble/app/constants/chat_model_call_sites.py`). The homogeneous scheduler MUST assign a scenario only to its eligible roles and MUST fail closed (`ErrEvaluationScenarioRolesUnassigned`) on a scenario with none. `ValidateHomogeneousAssignmentMatrix` MUST reject an assignment for a role the scenario does not declare (`ErrEvaluationRoleNotEligible`). The frozen catalog digest binds the eligible-role sets. |
| INV-EVAL-CAMP-06 | Homogeneous assignments MUST execute model by model: every scenario, role, and repetition of one model runs before the next model loads, so the provider keeps that model resident. The scheduler and `Store.ListAssignments` share one order (`assignmentExecutionLess`: served model tag, variant ID, scenario ID, role, repetition, then identity). Execution order MUST NOT be derived from assignment identity hashes alone. |

### Witness Separation (`INV-EVAL-WIT`)

| ID | Rule |
| --- | --- |
| INV-EVAL-WIT-01 | The Observer Operator (`--provider-boundary-observer-enabled`) and Provenance Operator (`--provenance-operator-enabled`) MUST enroll as separate governed sessions from the Inference Operator with distinct capability flags. MUST NOT pass `--inference-enabled` on witness-only sessions. |
| INV-EVAL-WIT-02 | The Observer Operator MUST run on the machine where Ollama and GPU hardware execute. The Provenance Operator MUST run where model storage lives (for example `~/.ollama/models`). Neither MUST have generic command authority or access to Inference Operator attempt files. |
| INV-EVAL-WIT-03 | When scored inference begins and ends, the Gateway MUST fan out fire-and-forget BEGIN/FINALIZE commands in parallel with inference dispatch to separate Observer and Provenance Operator sessions. Witness coverage MUST be independent of inference completion. |
| INV-EVAL-WIT-04 | The Observer Operator MUST report provider-boundary GPU VRAM, utilization, temperature, power, clocks, and system RAM samples between BEGIN and FINALIZE, bound to the same `provider_attempt_id` as inference. The Provenance Operator MUST attest model manifest and blob SHA-256 hashes. |

### Evidence Storage (`INV-EVAL-EVID`)

| ID | Rule |
| --- | --- |
| INV-EVAL-EVID-01 | Native run evidence (`report.json`, `verification.json`, digest-named artifacts) MUST be persisted under `.g8e/data/eval/runs/<run-id>/` and owned by `g8e eval boundary run` and `g8e eval boundary verify`. Campaign evidence MUST be persisted under the same `.g8e/data/eval/runs/<run-id>/` path and owned by `g8e eval runs …`. |
| INV-EVAL-EVID-02 | Provider-boundary observation windows (ingested from Observer Operator results) MUST be stored under the Gateway volume at `data/inference/provider-observer/windows/`. Model provenance attestation windows (ingested from Provenance Operator results) MUST be stored under `data/inference/model-provenance/windows/`. |
| INV-EVAL-EVID-03 | Campaign run state is run-scoped: definitions, frozen scenario artifacts, lifecycle, assignment, trace, and aggregate records MUST all persist in `.g8e/data/eval/campaigns/<campaign-id>/` and `.g8e/data/eval/runs/<run-id>/`. |
| INV-EVAL-EVID-04 | Public-safe campaign projections MUST omit principal, Operator, session, credential, endpoint, path, raw target, envelope, receipt, audit, and evidence body fields. Private prompts, traces other than bounded `model_response`, `failure_output`, and `role_transcripts`, execution identifiers, and artifact locations MUST NOT cross the public boundary. |
| INV-EVAL-EVID-05 | `g8e eval backup` MUST write only to a destination outside the runtime directory, copy only `.g8e/data/eval/` and `.g8e/eval/` (excluding run `lease.json` and `active-run.json`), and record a SHA-256 manifest. Automatic post-run backup MUST NOT alter a run's result: its failure is a warning, never a run failure. `g8e eval restore` MUST verify every file against that manifest, and confine every write to those two trees, before writing; it MUST NOT overwrite differing evidence without `--overwrite`. |

### Verification Posture (`INV-EVAL-VERIF`)

| ID | Rule |
| --- | --- |
| INV-EVAL-VERIF-01 | Campaign assignment results use projection envelope `1.1.0` for new records with named public extensions: `scenario_summary`, `semantic_grade_summaries`, `activity_summary`, `benchmark_observations`, `resource_summary`, `verification_metadata`, and `evidence_bindings`. Historical envelopes (`1.0.0`) remain readable. |
| INV-EVAL-VERIF-02 | Model quality state `exploratory_verified` MUST be published only for exact run-derived datasets and eligible `(variant_id, role)` aggregates covered by an applicable passing verification report. Other datasets and roles remain at their existing state (`exploratory_partial` or `unavailable`). |
| INV-EVAL-VERIF-03 | A passing campaign verification report is applicable only when its run, campaign, catalog, model registry, completed population, and verified population match persisted evidence. Applicable verification MUST republish verified assignment results and emit report-scoped model-summary revisions. |
| INV-EVAL-VERIF-04 | Observation unavailability MUST use the closed vocabulary: `historical_not_captured`, `source_not_captured`, `source_unavailable`, `scenario_not_applicable`, `incomplete_contributor_evidence`, and `no_scored_calls`. Observed zero MUST be preserved separately from unavailable values. |

## Owned surfaces

| Claim | Path | Verify |
| --- | --- | --- |
| Native suite registry, grading, and target observer | `internal/services/evaluation/` | `g8e eval boundary run` and `g8e eval boundary verify` |
| Campaign controller and publication coordinator | `internal/services/evaluation/` | `g8e eval runs start`, `resume`, `verify`, `publish` |
| Provider-boundary observation | `internal/services/operatorcapability/provider_boundary_observer.go`, `internal/services/inference/provider_observer/` | Observer Operator enrollment and telemetry sampling |
| Model provenance attestation | `internal/services/operatorcapability/provenance_operator.go`, `internal/services/inference/model_provenance/` | Provenance Operator enrollment and weight hashing |
| Data Operator selection | `internal/services/operatorcapability/data_operator.go` | `SelectDataOperator` resolves the stack's `data-operator` |
| CLI command tree and subcommands | `internal/cli/cmd/eval/` | `./g8e eval --help` and per-subcommand help |
| Campaign definitions and frozen artifacts | `eval/` (checked-in program data), `examples/eval/` (templates), `.g8e/data/eval/campaigns/` (runtime) | `g8e eval campaigns create` and `g8e eval campaigns show` |
| Model inventory and rollout intake | `eval/base-model-inventory.json`, `eval/rollout-intake-hf.json` | `g8e eval models list` and registry inspection |
| Protocol contracts and envelopes | `protocol/proto/g8e/eval/v1/` | `make proto` generates Go, Python, TypeScript bindings |

## Procedures

### Topology: Confidential Edge Execution with Governed State Mutation

Evaluation programs implement one instance of a broader architecture: **Confidential Edge Execution with Governed State Mutation**. The same Gateway (Policy Decision Point) and Operator (Policy Execution Point) topology combines local artifact provenance, runtime telemetry observation, central policy enforcement, and outbound-only state-mutation operators at the data owner's boundary.

Evaluations prove that allowed governed mutations succeed through the real Gateway and remote Operator; doctrine-prohibited equivalents fail closed without side effects; independent observers attest terminal state without network access or write authority; model-role and system-lane inference remain bound to the selected Inference Operator; and Provenance and Observer Operators witness model weights and provider-boundary hardware separately from the inference executor.

When AI actively alters system state from local inference, the value of zero-trust provenance and outbound-only telemetry scales with each mutation's consequence. The evaluation topology proves this separation; the topology mirrors in the [Position Paper](../core/position_paper.md) show where the same pattern applies in DevSecOps, OT/SCADA, healthcare EHR, and financial trading workflows.

Every mirror deployment separates four roles:

| Role | Evaluation instance | General responsibility |
| --- | --- | --- |
| **Provenance Operator** | Storage-side model weight attestation | Hashes local artifacts inference depends on (code, firmware, scans, feeds, model weights) |
| **Observer** | Provider-boundary GPU/RAM sampling; networkless fixture reader | Captures runtime telemetry at the execution boundary without mutating target state |
| **Data Operator** | Campaign host tool/filesystem/process boundary | Mutates authoritative state at the owner's boundary (PRs, actuators, EHR, trades) |
| **Gateway (PDP)** | Campaign and boundary-suite policy admission | Enforces central policy from provenance proofs, observer telemetry, and posture-required authorization |

All Operators connect outbound-only over mTLS. The Gateway admits work; each Operator independently verifies envelopes before any local side effect or witness publication.

### Evaluation programs

Two programs exercise the evaluation topology:

| Program | Suite / catalog | Proves | Does not use |
| --- | --- | --- |
| **Execution-boundary** | `core-execution-boundary@1.0.0` | One allowed governed mutation and one doctrine-prohibited equivalent through the real Gateway and remote Operator | g8ee, model providers, model judges, campaigns, synthetic simulators |
| **Model campaign** | `north-star-25@1.0.0` (standard scenario catalog) | Governed model-role scoring through production inference, tool scenarios, heterogeneous system-lane formations, provider-boundary hardware telemetry, and storage-side model weight attestation | Direct Ollama calls from campaign CLI or g8ee |

Both programs persist canonical, content-addressed run evidence beneath `.g8e/data/eval/runs/` with campaign definitions and frozen artifacts stored separately beneath `.g8e/data/eval/campaigns/<campaign-id>/`. Verification is independent of execution: `g8e eval boundary verify` and `g8e eval runs verify` recompute bindings and signatures without executing new scored actions.

### CLI surface

The `g8e eval` command tree (alias `g8e evals`) groups platform evaluation commands across ten top-level subcommands:

| Subcommand | Purpose |
| --- | --- |
| `g8e eval boundary …` | Run, list, verify, and show native execution-boundary test suites |
| `g8e eval models …` | Catalog and registry management (list, show, add, remove, import, freeze, pull, diff) |
| `g8e eval campaigns …` | Campaign definitions (list, show, create, archive, unarchive) |
| `g8e eval runs …` | Campaign execution and lifecycle (list, show, start, resume, cancel, logs, verify, publish, export, repair, compare, archive, unarchive) |
| `g8e eval rollout …` | Rollout qualification queue (list, add, remove, next, retry, skip, run) |
| `g8e eval formations …` | Heterogeneous multi-model stacks (list, show, add, remove, smoke) |
| `g8e eval gates …` | Pre-campaign acceptance gates (chat, inference, probe) |
| `g8e eval observer …` | Provider-boundary hardware observer (run, verify) |
| `g8e eval backup` | Copy evaluation evidence to a directory outside `.g8e/` (default `eval/backups`) |
| `g8e eval restore [snapshot-dir]` | Verify a backup snapshot and restore it into `.g8e/` (default: newest in `eval/backups`) |

Use `./g8e eval --help` as the command-surface reference. On `g8e eval runs start`, verification and witness flags are optional by default; use `--require-observation`, `--require-provenance`, or the `--require-witness` preset when witness requirements are part of acceptance scope. On `g8e eval rollout run`, strict witness verification defaults true; `--gate-smoke` and `--promote-on-pass` provide fast candidate screening.

### Native execution-boundary suite

The Phase 1 suite is `core-execution-boundary@1.0.0`. It requires doctrine posture and does not use g8ee, external model providers, model judges, campaigns, scheduling, or synthetic simulators.

**Run:** Start and enroll the unified stack, then execute:

```bash
./g8e eval boundary run
```

The suite targets the stack's `data-operator` and ignores every other enrolled Operator. It takes no session flag.

The suite performs two attempts: the allowed attempt writes one run-specific marker through the authenticated Gateway command ingress and the `data-operator`; the prohibited equivalent traverses the same ingress and must be rejected by L1 without side effect. Required verdicts cover independent effect counts, target identity, terminal receipt status, receipt durability, deterministic protocol-chain validity, rejection, absence of completed alternative execution, and Gateway L1 attribution.

**Verify and inspect:** Each run persists `report.json`, `verification.json`, and digest-named evidence files under `.g8e/data/eval/runs/<run-id>/`. Re-run verification without executing new mutations:

```bash
./g8e eval boundary verify <run-id>
./g8e eval boundary show <run-id>
```

Add `--json` to emit canonical protojson. Verification resolves every declared artifact, recomputes content addresses, validates report and attempt bindings, verifies receipt and persistence signatures, validates deterministic stage evidence, reconstructs typed observations, recomputes verdicts and metrics, and rejects missing, substituted, contradictory, misbound, or undeclared evidence.

**Trust boundaries:** The Gateway is the Policy Decision Point and owns ingress authentication, envelope construction, L1-L3 decisions, and coordination state. The selected remote Operator is the Policy Execution Point and owns L4-L5 execution and authoritative local evidence. The Gateway receipt query is a verified mirror of Operator-authored evidence, not an independent read.

The native suite reads terminal fixture state through a short-lived `g8e-native-target-reader` process defined only in `eval/native-boundary-compose.yml`. It is not an Observer Operator and not a unified-stack service. The evaluator invokes it explicitly with `docker compose run --rm`; it has no network, workload identity, runtime volume, credentials, or writeable target mount. It mounts only the shared controlled fixture volume read-only with all Linux capabilities dropped so it can read root-owned fixture state without mutations or network crossings.

### Model campaign evaluations

Evaluation model campaigns score real models through the production g8ee `POST /api/v1/chat` path, governed inference dispatch, and a frozen scenario catalog. They use one campaign Gateway with two core Operator sessions on the campaign host (Data and Inference) plus optional provider-side witness sessions for hardware observation and model provenance:

| Session | Capability flag | Host | Role |
| --- | --- | --- |
| **Data Operator** | role `data`, hostname `data-operator` | Campaign host (Docker) | Governed tool/filesystem/process boundary for model-originated host actions. The only data Operator evaluations consider; other enrolled data Operators are ignored. |
| **Inference Operator** | `inference_enabled=true` | Campaign host (Docker) | Governed L4/L5 inference Policy Execution Point; sole scored path to the approved Ollama provider |
| **Observer Operator** | `provider_boundary_observer_enabled=true` | Provider host (where Ollama/GPU runs) | Read-only GPU and system RAM sampling at the provider execution boundary |
| **Provenance Operator** | `provenance_operator_enabled=true` | Model storage site (where weight blobs live) | Independent SHA-256 attestation of model manifests and weight blobs |

Scored inference and provider maintenance never call Ollama directly from g8ee or the campaign CLI. The Gateway routes inference envelopes to the exact Inference Operator session. Tool intents route to the exact Data Operator session, observation commands to the exact Observer Operator session (when enabled), and provenance commands to the exact Provenance Operator session (when enabled). Campaign-host `--ollama-endpoint` and `G8E_OLLAMA_ENDPOINT` overrides are not accepted; the Inference Operator's enrolled `runtime_config` determines Ollama access.

**Data Operator binding:** A CLI session can be bound to many Operators (`g8e operator bind`), and the Gateway accepts any bound Operator session as a request's operator identity, not only the primary one. Scored runs and `gates chat` send tool intents as the `data-operator`, so it must be one of the CLI session's bound sessions. When it is not, they issue one bind call for the still-active bound sessions plus the `data-operator`; `--no-auto-bind` turns that off and fails with `ErrDataOperatorNotBound` instead. No eval command takes an Operator session flag: the `data-operator` is identified by its hostname, and the Inference Operator by its `inference_enabled` capability.

**Model inventory and rollout intake:** Model campaigns bind scored inference to frozen `served_model_tag` and `model_digest` pairs in the campaign registry. The checked-in genesis inventory (`eval/base-model-inventory.json`) is a reference snapshot; live provider runs should re-freeze digests before execution.

`g8e eval models pull --all` reads the rollout intake catalog (`eval/rollout-intake-hf.json`) and dispatches governed pull and copy commands to the exact Inference Operator. The Operator-owned Ollama client pulls GGUF models (for example `huggingface.co/unsloth/Qwen3.8-27B-GGUF:UD-Q4_K_M`) and applies canonical served-model aliases (for example `qwen3.8:27b`). Use `--formations` to pull the sovereign ExecutionTopologies library tags through the same governed path.

After pulling, `g8e eval models freeze` discovers live provider inventory through a governed Inference Operator command and writes the frozen registry to `.g8e/eval/model-inventory.json`. Optional `--probe-capabilities` requests bounded, unscored inference through the same session. `g8e eval campaigns create` and `g8e eval rollout add` then bind campaigns and the rollout queue with provider-accurate digests.

`g8e eval models add` inserts or replaces one variant and recomputes the registry digest. When `--digest` is omitted, `add` derives a placeholder digest that lacks attestation authority; re-freeze from the provider before scored runs. `g8e eval models remove` deletes one variant and recomputes the same values. `g8e eval models import` copies selected variants from the catalog into the runtime registry.

**Two-tier rollout qualification:** Standard homogeneous campaigns evaluate each candidate model on all 26 catalog scenarios, each under only the roles that perform that task in g8ee: 10 bounded classification, verification, and output-analysis scenarios run under `lite`; 13 tool-loop, policy, and recovery scenarios run under `primary` and `assistant`; `route-primary-ownership` and `final-response-diagnosis` run under `primary`; and `route-handoff-assistant` runs under `assistant`. This yields 39 scored assignments per model.

To accelerate high-throughput qualification, `g8e eval rollout run` supports a two-tier screening pipeline:

1. **Tier 1 — Fast Smoke Gate** (`--gate-smoke`): Executes 5 high-discriminative scenarios under their eligible roles (8 assignments per model). Scenarios exercise syntax and tool execution, investigation and diagnostic reasoning, dissent and safety compliance, multi-step remediation, and fast-path direct instruction response. Requires 100% pass status on witness and verification gates.

2. **Tier 2 — Comprehensive Qualification** (`--promote-on-pass`): Automatically promotes Tier 1 candidates into the full 39-assignment matrix. Discards non-viable Tier 1 failures early, saving 45+ minutes GPU residency per candidate.

```bash
./g8e eval rollout run --gate-smoke --promote-on-pass
```

**Production agentic loop and persona binding:** Homogeneous campaigns score assignments by posting directly to `POST /api/v1/chat` and drive the real ReAct loop with real tool dispatch to the bound Data Operator. Role control activates authentic production agent personas:

- `primary`: Activates `ReasoningAgent.SAGE` bound to `SagePersona`
- `assistant`: Activates `ReasoningAgent.DASH` bound to `DashPersona`
- `lite`: Activates `ReasoningAgent.DASH` with lite-tier operational constraints

Each persona wraps in the production modular system prompt stack: Core Safety, Core Loyalty, Core Dissent, Mode Execution/Capabilities, Tool Schemas, Response Constraints, System Context, and Sentinel Mode.

**Simulated operator files:** A tool-selection or tool-argument scenario can pin a `ScenarioInputFixture.SimulatedFiles` entry to a synthetic operator path (e.g. `/synthetic/eval/network-summary.txt`) that the model is expected to reach with `file_read_on_operator` or `run_commands_with_operator`. Before the scenario's chat request is sent, `CampaignChatExecutor` dispatches a governed file write through the same bound Data Operator session, so the tool call resolves against real content rather than a missing path. This content is never sent to the model directly; only a real tool call can surface it.

**Campaign lanes and formations:** The default campaign lane is `model_role`, which schedules each frozen model variant against catalog scenarios and records role-specific results. Campaigns can also use the `system` lane. The CLI creates deterministic, persisted heterogeneous stacks with `g8e eval campaigns create <id> --formations <id>... --seed <seed>` and starts execution with `g8e eval runs start <id>`.

A formation contains primary, assistant, and lite model bindings, executed in `lite → assistant → primary` order. `g8e eval formations add` writes a formation (all three roles required) to the checked-in overlay `eval/formation-catalog-overlay.json`, replacing any entry with the same ID; `formations remove` deletes an overlay entry or records a checked-in default as removed, and the catalog can never be left empty. The effective catalog is the built-in execution topologies merged with that overlay. `runs start` and `runs resume` select the formation runner with `--formation-runner`:

| Runner | Execution | Grading | Telemetry |
| --- | --- | --- | --- |
| `g8ee` (default) | Each role is one g8ee `POST /api/v1/chat` call, the same pipeline homogeneous assignments use, with real tool calls against the bound Data Operator and simulated files materialized first. The assignment ID is constant; each role's evaluation attempt ID is role-qualified (`<attempt>:lite`, `:assistant`, `:primary`). Each role's message carries every prior role's output. Executes under the governed `FormationRunner` with storage-side provenance attestation and co-resident allocation/release. | `GradeHeterogeneousScenario`: every catalog criterion per role (grade IDs role-qualified), plus one pipeline-scoped `heterogeneous-pipeline` grade; `decomposed_scores` populated. | Full formation witness evidence: peak VRAM (max across all per-attempt observer windows), storage attestation digest. Tokens, generation duration, TTFT, and provider attempt IDs from each role's trace. Per-attempt witness windows apply exactly as for homogeneous assignments. |
| `direct` | Roles dispatch straight to the Inference Operator after storage-side provenance attestation, each bracketed by provider-boundary observation, with mutation candidates routed through the governed policy gate. | Single completion grade; no `decomposed_scores`. | Full formation witness evidence: peak VRAM, observer and provenance digests. |

Each g8ee-routed role trace is persisted inside the assignment's formation run evidence (`FormationRunEvidence.Result.Roles[].Trace`), not the per-assignment trace store.

### Witness operator roles

g8e uses distinct witness mechanisms:

| Name | Where it runs | Purpose |
| --- | --- | --- |
| **Native target reader** | Ephemeral campaign-host process from `eval/native-boundary-compose.yml` | Networkless target-state read for native `core-execution-boundary@1.0.0` suite only; not a unified-stack service or Operator |
| **Observer Operator** | Provider host (`g8e operator start --provider-boundary-observer-enabled`) | Enrolled read-only remote Operator for provider-boundary GPU/RAM telemetry during scored model campaigns |
| **Provenance Operator** | Model storage site (`g8e operator start --provenance-operator-enabled --model-storage-root <path>`) | Enrolled remote Operator for storage-side model weight hashing and digest attestation during scored model campaigns |

Only the remote Observer Operator supplies provider-boundary observation for model campaigns. The native target reader cannot satisfy hardware-efficiency requirements; the Provenance Operator attests model files rather than GPU state.

**Provider-boundary Observer Operator:** Deploy on the machine that runs Ollama (for example a remote Windows GPU host), not on the Linux campaign host. When scored inference starts and ends, the Observer samples GPU VRAM, utilization, temperature, power, clocks, and system RAM. The Gateway sends `ProviderBoundaryObservationCommand` (BEGIN/FINALIZE) on the observer's pub/sub channel; the Observer publishes `ProviderBoundaryObservationCompleted` on its results channel. Gateway ingests windows for `g8e eval runs verify --require-observation`.

The Observer has no Ollama management capability. Consecutive scored assignments keep the daemon resident; after a completed queue, the controller dispatches model-release commands through the exact Inference Operator. That governed command sends an empty `/api/generate` request with `keep_alive: 0` to the Operator's approved endpoint; the controller confirms campaign-owned tags are absent.

**Timing rule:** Assignments that reached a terminal state before the Observer Operator was enrolled and pub/sub-connected will fail `--require-provider-observation`. Enroll the observer before `execute`, or accept that early assignments lack hardware windows.

**Storage-side Provenance Operator:** Deploy at the model storage site where Ollama manifests and content-addressed weight blobs live (for example `~/.ollama/models`). On FINALIZE, the operator resolves the served tag with Ollama's canonical name parser and reads the matching manifest. The operator hashes every referenced blob and compares the manifest digest to the expected campaign model digest. The operator publishes `ModelProvenanceObservationCompleted` on its results channel.

**Fail closed:** If the served tag cannot be resolved, a referenced blob is missing, or observed and expected model digests do not match, FINALIZE fails and the attestation window is not published. This is independent of the Inference Operator's own digest checks — the Provenance Operator is a storage-side witness, not self-report from the inference executor.

**Timing rule:** Assignments that reached a terminal state before the Provenance Operator was enrolled and pub/sub-connected will lack attestation windows. Enroll the provenance operator before `execute` when chain-of-custody claims are required.

### Evidence and verification

**Storage layout:** Native run evidence (`report.json`, `verification.json`, digest-named artifacts) persists under `.g8e/data/eval/runs/<run-id>/` owned by `g8e eval boundary run` and `g8e eval boundary verify`. Campaign definitions and frozen scenario artifacts persist under `.g8e/data/eval/campaigns/<campaign-id>/`. Campaign run state and results persist under `.g8e/data/eval/runs/<run-id>/` (campaign-scoped lifecycle, assignment, trace, and aggregate records). Provider observation windows ingested from Observer Operator persist under the Gateway volume at `data/inference/provider-observer/windows/`. Model provenance attestation windows ingested from Provenance Operator persist under `data/inference/model-provenance/windows/`.

**Backup and restore:** Host evidence survives `docker compose down -v` and `./g8e docker clean`, which destroy only the Docker volumes, but `.g8e/` is still the single copy. `g8e eval backup` writes a new `eval-backup-<UTC timestamp>/` snapshot beneath `--output-dir`, which defaults to `eval/backups/` under the project root (git-ignored). Any directory outside `.g8e/` is accepted; the command rejects destinations inside it, including through symlinks. The snapshot mirrors `data/eval/` and `eval/` and carries an `eval-backup.json` manifest with a SHA-256 per file, written last so an interrupted backup is never restorable. Transient process state (`lease.json`, `active-run.json`) is not copied. The destructive commands `docker clean`, `docker reset`, `docker init --clean`, `gw clean`, and `gw reset` offer this same snapshot before wiping (`--skip-backup` opts out).

**Automatic backup:** `g8e eval runs start` (and therefore `g8e eval rollout run`) and `g8e eval runs resume` snapshot into `eval/backups/` when they finish, after verification and whether the run succeeded, failed, or was cancelled. A snapshot identical to the newest complete one is discarded rather than kept, so repeated resumes of an unchanged run do not accumulate copies. A backup failure is reported as a warning on stderr and never changes the run's result or exit status. Pass `--no-backup` to skip it. Snapshots are never pruned automatically.

```bash
./g8e eval backup                                       # eval/backups/
./g8e eval backup --output-dir ~/g8e-eval-backups
./g8e eval restore                                      # newest snapshot in eval/backups/
./g8e eval restore ~/g8e-eval-backups/eval-backup-<timestamp>
```

`g8e eval restore` verifies every file against the manifest, rejects manifest paths outside the two evidence trees, and only then writes. Files already identical are skipped; if any existing file differs, nothing is written unless `--overwrite` is passed. Restore repopulates host evidence only. It does not back up or recreate the Gateway volume (PKI, owner and Operator identities, mirror, observation and provenance windows); after enrolling a fresh stack, run `./g8e public restore --queue` to rebuild the Gateway mirror from the restored host evidence.

Native verification is owned by `g8e eval boundary verify`. Campaign verification is owned by `g8e eval runs verify`, with `--require-observation` enforcing hardware-window coverage through the Gateway read API when local evidence is missing.

Formation verification branches on the persisted evidence, not on a flag. When every role in the formation run evidence carries a trace, the verifier checks each role trace's digest and regrades from those traces with `GradeHeterogeneousScenario`; stored grades must match by grade ID and criterion ID, so a tampered grade for one role cannot hide behind another role's identical criterion. Direct-dispatch evidence undergoes the completion-grade recompute and full formation witness checks (`VerifyFormationWitnessEvidence`). Both paths run `VerifyFormationWitnessEvidence` for all witness evidence, plus the per-attempt observation and provenance checks over the result's model inference records.

**Public spectator projection:** The checked-in evaluation explorer reads canonical native and campaign projections from persisted runs. A public-safe projector omits principal, Operator, session, credential, endpoint, path, raw target, envelope, receipt, audit, and evidence body fields before records enter the signed public feed. Native verification remains on the owner path; mirror availability is not verification evidence. See [Public Spectator Architecture](./public_spectator.md).

Campaign assignment results use the enriched campaign projection envelope (`1.1.0`) for new records with named public extensions: scenario context, deterministic and semantic grade summaries, typed activity families, bounded resource metrics, verification metadata, and lowercase SHA-256 evidence bindings. Scenario descriptions and criterion labels come from the exact persisted campaign/catalog bindings; private prompts, gold answers, trace text other than the bounded `model_response`, `failure_output`, and `role_transcripts` extensions, execution identifiers, receipt bodies, and artifact locations do not cross the boundary. See [Public Spectator Architecture](./public_spectator.md) for exactly what `role_transcripts` carries.

Assignment activity preserves the distinction between an observed empty list, unavailable source capture, and scenario-not-applicable. Resource metrics preserve an observed zero and identify unavailable token, retry, or latency values explicitly. The closed public unavailable-reason vocabulary is `historical_not_captured`, `source_not_captured`, `source_unavailable`, `scenario_not_applicable`, `incomplete_contributor_evidence`, and `no_scored_calls`.

Current campaign aggregate records use Evaluation Explorer view schema `1.5.0`. The `evaluation_summary` projection emits typed pass-rate, scored-inference latency p50, and output-throughput p50 metrics with units and observed, eligible, and unavailable contributor counts. A passing campaign verification report publishes `quality_state: exploratory_verified` only for the exact run-derived dataset and eligible `(variant_id, role)` aggregate covered by the verified population. Other datasets, roles, incomplete populations, failed reports, and mismatched bindings remain at their existing quality state, normally `exploratory_partial` or `unavailable`. `exploratory_verified` means the named run evidence passed the verifier's scope; it does not mean universal model quality, complete optional telemetry, or `verified_public`.

## Anti-patterns

- Deploying Observer Operator on the campaign host instead of the provider host where Ollama and GPU hardware run.
- Passing `--inference-enabled` on witness-only Operator sessions; witness sessions must enroll with only capability flags for observation or provenance.
- Hand-editing frozen campaign digests or model inventories instead of running `g8e eval models freeze` to re-attest provider state.
- Running scored campaigns without witness operators enrolled when `--require-witness` or rollout defaults are active, resulting in failed verification.
- Updating placeholder model digests without re-freezing from the provider; placeholder digests lack provenance authority for attestation.
- Omitting pre-campaign `g8e eval models freeze` after adding new models, resulting in stale or unverified digests.
- Assuming the Inference Operator's digest checks alone prove model integrity; the Provenance Operator provides independent storage-side attestation.

## Links out

- [Position Paper](../core/position_paper.md) — Research framing for confidential edge execution and governed state mutation
- [Unified Docker Stack Guide](../guides/unified_stack.md) — Compose profiles, enrollment order, campaign workflows, and troubleshooting
- [Sovereignty Gauntlet](../guides/sovereignty_gauntlet.md) — Evidence-oriented demonstration and claim-scoping workflow
- [Ensemble Evaluations](../ensemble/evals.md) — How g8ee uses g8e evals through the production chat path
- [Ensemble (g8ee)](./ensemble.md) — g8ee's role in the platform and trust boundaries
- [Model Provenance](./model-provenance.md) — Zero-trust weight attestation, Provenance Operator enrollment, and chain of custody
- [Gateway Architecture](./gateway.md) — Inference dispatch, provider-boundary coordination, and pub/sub
- [Operator Architecture](./operator.md) — L4 Warden, L5 Actuator, and capability flags
- [Governance Architecture](./governance.md) — Five-layer verification pipeline and fail-closed enforcement
- [Public Spectator Architecture](./public_spectator.md) — Public-safe evaluation projections and explorer contract
