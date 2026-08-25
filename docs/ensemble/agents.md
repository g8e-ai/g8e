# Agents

## Overview

The g8e Agentic Ensemble (`g8ee`) uses a structured multi-agent architecture where specialized agents collaborate across defined operational stages. Each agent operates under a concrete persona model that defines its role, model tier, tool availability, autonomy boundary, and output contract. The architecture enforces separation of concerns: reasoning agents formulate high-level intent without raw shell syntax, a five-member Tribunal derives and validates exact commands under information isolation, defensive filters assess execution risk, and support agents manage lifecycle metadata, memory, and performance evaluation.

## Persona Architecture

Agent personas are implemented as typed Python classes inheriting from `AgentPersonaModel` (`app.models.personas.base.AgentPersonaModel`), replacing unstructured JSON configuration with validated code models.

Each persona defines:

- **`id`** — Unique identifier registered in the central `PERSONA_REGISTRY` (e.g., `triage`, `sage`, `dash`, `auditor`).
- **`display_name`** — Human-readable display label for user interfaces and logs.
- **`icon`** — Material icon identifier representing the persona.
- **`description`** — Summary of the agent's responsibilities.
- **`role`** — Functional classification within the ensemble (`classifier`, `reasoner`, `responder`, `tribunal_member`, `arbitrator`, `auditor`, `defender`, `summarizer`, `analyzer`, `evaluator`).
- **`model_tier`** — Assigned LLM capacity tier (`primary`, `assistant`, `lite`).
- **`tools`** — Whitelist of tool names available to the persona during execution.
- **`identity`** — System prompt guidelines specifying behavioral principles, voice, and discipline, structured via canonical XML tags (`<role>`, `<identity>`, `<purpose>`, `<autonomy>`, `<output_contract>`).
- **`purpose`** — Operational charter defining what the agent accomplishes in the workflow pipeline.
- **`autonomy`** — Authority boundary governing whether the agent commits decisions directly or delegates to downstream verification.
- **`output_contract`** — Schema or format constraints enforced on the agent's generation output.

## Ensemble Roster and Hierarchy

The ensemble organizes agents into functional tiers corresponding to the lifecycle of an investigation turn.

### Gatekeeper / Triage

- **Triage (`triage`)** — Operates on the `lite` tier as the initial classifier (`role="classifier"`). Triage performs the first read of the user's message and emits a structured `TriageResult` containing `complexity` (`simple` or `complex`), `intent` (`information`, `action`, or `unknown`), `request_posture` (`normal`, `escalated`, `adversarial`, or `confused`), and associated confidence ratings. Triage enforces a mandatory security override: any request involving authentication, credentials, permissions, accounts, or security configuration is classified as `complex` regardless of phrasing. Triage does not generate clarifying questions or call tools; it routes simple turns to Dash and complex turns to Sage.

### Reasoning Agents

- **Sage (`sage`)** — The senior reasoning authority (`model_tier="primary"`, `role="reasoner"`). Sage plans multi-step investigations, articulates intent to the Tribunal, interprets tool outputs, synthesizes evidence, and drafts final user responses. Sage articulates investigative intent using `SageOperatorRequest` without proposing raw shell syntax. If an investigation stalls due to missing context or ambiguity, Sage invokes the interrogation protocol by emitting three binary YES/NO questions.
- **Dash (`dash`)** — The fast-path responder (`model_tier="assistant"`, `role="responder"`). Dash resolves straightforward requests with minimal latency. It answers knowledge-based inquiries directly or executes a single surgical tool call when required. If a simple turn lacks necessary context, Dash emits an interrogation block before executing state-changing tools. Requests requiring multi-step chains or deep hypothesis testing escalate to Sage.

### Tribunal Collective and Members

The Tribunal (`tribunal`, `role="arbitrator"`) is a five-member consensus panel that translates Sage's natural-language intent into executable commands on the target host. To prevent groupthink and preserve Information Isolation, all five members evaluate the intent independently in parallel:

- **Axiom (`axiom`)** — Composition lens (`role="tribunal_member"`, `lite` tier). Focuses on clean, coherent multi-stage pipelines that fulfill multi-fact intents in a single invocation.
- **Concord (`concord`)** — Safety lens (`role="tribunal_member"`, `lite` tier). Focuses on defensive flags, read-only discipline, explicit paths, and safe pipeline chaining (`&&`, `pipefail`, `xargs -r`).
- **Variance (`variance`)** — Edge-case lens (`role="tribunal_member"`, `lite` tier). Focuses on environmental hazards, handling spaces in filenames, null-delimited processing (`-print0 | xargs -0`), locales, and missing directories.
- **Pragma (`pragma`)** — Convention lens (`role="tribunal_member"`, `lite` tier). Focuses on idiomatic community patterns and native OS/shell tools (`journalctl` on systemd, `ss` over `netstat`, `kubectl get`).
- **Nemesis (`nemesis`)** — Calibrated adversary lens (`role="tribunal_member"`, `lite` tier). Proposes subtle, plausible-but-flawed commands to stress-test verification, or honestly abstains by emitting the correct command when no realistic flaw exists.

Every Tribunal member emits strictly a shell command string without commentary or markdown formatting.

### Machine-Domain Auditor

- **Auditor (`auditor`)** — The Tribunal judge and quality gate (`model_tier="primary"`, `role="auditor"`). The Auditor inspects anonymized candidate command clusters produced by the Tribunal members against Sage's intent. Operating across unanimous (5/5), majority (3-4), or tied modes, the Auditor emits one of three structured verdicts: `ok` (approves the top candidate), `revised:<command>` (corrects syntax or whitelist violations), or `swap:<cluster_id>` (selects a superior dissenting candidate).

### Pre-Generation Risk Filter (Warden)

- **LLM Risk Filter / Warden (`warden`)** — Pre-generation risk coordinator (`model_tier="lite"`, `role="defender"`). Consolidates risk assessments before a `GovernanceEnvelope` is constructed:
  - **Command Risk Analyzer (`warden_command_risk`)** — Evaluates shell command blast radius, reversibility, and failure impact (`LOW`, `MEDIUM`, `HIGH`). Stakes reputation on assessment accuracy.
  - **Error Analyzer (`warden_error`)** — Analyzes command execution failures and classifies recovery as `AUTO_FIXABLE`, `ESCALATE`, or `RETRY_LIMIT`.
  - **File Operation Risk Analyzer (`warden_file_risk`)** — Assesses file mutation risks based on path sensitivity, reversibility, and Git repository state (`LOW`, `MEDIUM`, `HIGH`). Stakes reputation on assessment accuracy.

### Support and Evaluation Agents

- **Scribe (`scribe`)** — Case titler (`model_tier="lite"`, `role="summarizer"`). Generates concise, 3-7 word titles summarizing new cases based on initial user prompts.
- **Codex (`codex`)** — Memory builder (`model_tier="lite"`, `role="analyzer"`). Extracts durable user preferences and redacted investigation summaries into `InvestigationMemory` records for cross-session personalization.
- **Judge (`judge`)** — Benchmark evaluator (`model_tier="primary"`, `role="evaluator"`). Evaluates agent outputs against gold-standard rubric criteria, generating quantitative scores and qualitative justifications for eval runs and reputation tracking.

## Operational Flow and Invariants

The interaction between agents follows strict architectural invariants:

1. **Intent vs. Command Separation** — The caller-facing model `SageOperatorRequest` does not contain a `command` field; reasoning agents articulate what to accomplish rather than shell commands. The Tribunal derives the exact syntax, which is injected into `ExecutorCommandArgs` after consensus and auditor verification.
2. **Information Isolation** — Tribunal members run without knowledge of each other's candidate outputs or identities, preventing premature convergence.
3. **Fail-Closed Risk Analysis** — Warden sub-agents fail closed to `HIGH` risk on ambiguous or inconclusive data.
4. **Interrogation Gate** — When Sage or Dash encounters ambiguity, it emits an `<interrogation>` block with exactly three binary YES/NO questions. Tool execution pauses until the user provides answers.
5. **Reputation Staking** — Risk analyzer agents stake reputation on classification decisions, penalizing unwarranted blocks of benign operations while rewarding accurate detection of hazardous operations.

## Related

- [Architecture](architecture.md) — System architecture, protocol surfaces, and model hierarchy
- [Governance](governance.md) — Five-layer verification pipeline and envelope validation
- [Prompts](prompts.md) — System prompt assembly and persona templating
- [Thinking](thinking.md) — Provider reasoning tokens and cryptographic thought signatures
- [Evals](evals.md) — Benchmark evaluation suite and Judge scoring rubrics
- [Constants](constants.md) — Application constants and agent identifiers

