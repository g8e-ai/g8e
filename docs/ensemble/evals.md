# Evals

## Overview

g8ee includes a standalone evaluation package in `ensemble/evals/`. It supports live model evaluation across raw inference, ensemble orchestration, and governed execution, plus deterministic synthetic benchmarks for privacy, governance, utility, reliability, and economics. The package has its own Python 3.12 environment, locked dependencies, CLI, tests, linting, type checking, and CI job.

The CLI writes typed, versioned report bundles that bind tasks, attempts, metrics, receipts, stage telemetry, and indexed evidence by stable identifiers. Report consumers must validate those links and treat missing or inconsistent evidence as invalid rather than partially accepting a run.

## Evaluation modes

### Live model evaluation

`g8e-evals run` currently runs the curated `ifeval_subset` benchmark. The suite contains five tasks selected from a pinned upstream IFEval revision, preserves the selected source lines, and records dataset, prompt-bundle, and grader-bundle hashes in the run manifest. An optional LLM judge adds separate model-call and grading records; it does not replace deterministic IFEval grading.

The command exposes five experiment arms:

| Arm | Execution path | Requested posture | Receipt expectation |
| --- | --- | --- | --- |
| `direct` | Provider API only | None | None |
| `ensemble_ungoverned` | g8ee chat pipeline without the Gateway or Operator | None | None |
| `doctrine` | g8ee, Gateway, and Operator | L1 enforced; L2 and L3 audited | Collected when the turn produces an `ActionReceipt` |
| `consensus` | g8ee, Gateway, and Operator | L1 and L2 enforced; L3 audited | Collected when the turn produces an `ActionReceipt` |
| `notary` | g8ee, Gateway, and Operator | L1 and L2 enforced; L3 enforced for mutation-classified actions | Collected when the turn produces an `ActionReceipt` |

The Gateway, not the requested arm, is the posture authority. Governed attempts record the posture observed from the Gateway health surface, including an unobserved result when posture discovery fails. `ratify` remains a supported Gateway posture but is not a standalone arm in this experiment design.

Compare `ensemble_ungoverned` with `direct` to isolate the ensemble orchestration difference. Compare `consensus` with `doctrine`, or `notary` with `consensus`, to isolate adjacent requested-posture differences. A direct-versus-governed comparison combines orchestration and governance effects.

Current reports label L2 in the `consensus` and `notary` arms as `deterministic_replicated_doctrine`. This label distinguishes the current replicated deterministic doctrine evaluation from heterogeneous model reasoning.

### Deterministic synthetic benchmarks

`g8e-evals bench-synthetic` runs local production-shaped simulators and deterministic observers without an LLM provider, g8ee, the Gateway, the Operator, or an authentication context. Synthetic reports use the `direct` analytical arm because they do not traverse the live governance stack. They validate observer, evidence-linking, and grader behavior; they do not demonstrate live platform enforcement.

The command supports these suites:

| Area | Suites | Scope |
| --- | --- | --- |
| Privacy | `privacy_token_lifecycle`, `privacy_boundary_leakage` | Encrypted token persistence, expiry and failure behavior, rehydration, exfiltration resistance, and report artifact leakage |
| Governance and security | `governance_adversarial`, `policy_attack`, `benign_overblock` | Replay, signed-field and payload tampering, stale state roots, identity and nonce failures, signer defects, proof transplant, revoked credentials, evidence preservation, policy attacks, and benign overblocking |
| Utility | `tool_sequence`, `factual_qa`, `citation_backed`, `partial_milestone`, `final_state`, `ledger_consistency` | Tool order, factual answers, citation support, partial completion, receipt-bound final state, and independently observed ledger state |
| Reliability | `reliability` | Deterministic reliability scenarios and their declared evidence |
| Economics and performance | `economics_performance` | Deterministic cost and performance scenarios and their declared evidence |

Each synthetic task must declare at least one typed assertion with a registered deterministic grader. The command rejects tasks with no applicable grader or attempts that produce no metrics. It persists content-addressed source evidence, validates observation hashes and lengths, and scans the completed report for raw synthetic canaries and the per-run encryption key.

## Evidence and grading

### Live run evidence

The live runner completes authentication and provider preflight before creating a report directory. It then writes `manifest.json` before task execution and records the suite identity, eval package version, selected arm, requested posture, role-to-model mapping, runtime environment, content hashes, source/build provenance, provider budget, and stack environment. Preflight fails before any task executes when a required identity, hash, capability, or provenance field is unavailable.

Attempts use typed terminal states for completion, model failure, governance rejection, human denial, timeout, infrastructure failure, and invalid evidence. Normalized stages cover model inference, Tribunal generation and auditing, L1 doctrine, protocol L2, L3 ceremony, L4 verification, L5 execution, scrubbing, rehydration, receipt persistence, commitment append, and grading. Model stages record available timing, usage, retry, finish-reason, boundary-hash, and privacy-attestation data; missing provider usage remains missing rather than being estimated.

Governed attempts collect canonical `ActionReceipt` protobuf messages produced and signed by L5 Actuators. Receipt grading verifies the signature, final persistence attestation, expected action class, transaction and identity bindings, stage ordering, posture-specific L2 and L3 status, execution result, state roots, and parent relationships. Policy, final-state, canary, rehydration, secret-detection, mutation, tampering, replay, identity, and evidence-preservation graders run only when the task declares the corresponding typed assertion and supplies the required evidence.

Integrations can inject boundary-specific observers through the evaluation harness. The standard live CLI does not configure those observers, so an assertion that depends on an absent observer cannot become verified evidence merely from task configuration.

Raw prompts, model outputs, and agent trails are restricted evidence. The live runner requires an owner-only key file and stores these artifacts as AES-256-GCM envelopes with authenticated index metadata. Analytical records retain hashes, lengths, counts, types, and evidence references instead of raw restricted values.

### Metric contract

Every metric is registered by metric ID and version with its unit, direction, eligible population, denominator semantics, missing-value policy, aggregation method, uncertainty method, evidence requirements, grader class, and any release threshold. The runner rejects unregistered metrics and rows whose unit or grader class does not match the registry. Consumers aggregate only eligible rows according to each metric definition and keep unsupported exclusions out of the denominator.

### Preflight and source/build provenance

`g8e_evals.preflight` runs typed validation before any task executes and fails closed with a stable `PreflightFailureCode` when a required identity, hash, capability, or provenance field is unavailable. The runner never runs ad hoc Git commands to populate source/build provenance; all provenance comes from environment variables set by the trusted build system or CI pipeline. Each of the 15 ordered checks is single-purpose: provider/model presence, credential presence (keyless providers exempt), endpoint validation, sampling parameter ranges, seed support, stack image digests, network mode, OS metadata, runtime version, hardware metadata, redacted configuration leak detection, content hash presence and validity, preregistration hash, provider budget, and source/build provenance.

Source/build provenance is a typed `SourceBuildProvenance` model binding the source revision, source tree state hash (64-char hex), and optional build ID, build system, CI run ID, CI URL, binary sha256, and source-tree-modified flag. Two channels supply it. When `G8E_EVALS_SOURCE_REVISION` or `G8E_EVALS_SOURCE_TREE_STATE_HASH` is set, the environment wins and must fully validate; the full CI contract is `G8E_EVALS_SOURCE_REVISION`, `G8E_EVALS_SOURCE_TREE_STATE_HASH`, `G8E_EVALS_BINARY_SHA256`, `G8E_EVALS_BUILD_ID`, `G8E_EVALS_BUILD_SYSTEM`, `G8E_EVALS_CI_RUN_ID`, and `G8E_EVALS_CI_URL`. When no provenance env var is set, the runner reads the stamp embedded in the g8e CLI binary: `g8e_evals.provenance_bridge.load_cli_build_provenance` calls `g8e version --json`, which emits the `source_revision` and `source_tree_state_hash` the Makefile linked into the binary at build time (plus the toolchain's VCS revision and modified flag), and hashes the resolved binary file into `binary_sha256` so the evidence record names the exact artifact that ran. The Makefile computes the tree-state hash over `git ls-files` — tracked plus untracked, non-ignored files — so the digest covers the actual working source without including `.venv`, `node_modules`, `__pycache__`, or other generated noise; archive and Docker builds without `.git` fall back to a manifest walk with component excludes (`internal/tools/treehash`). The `compute_source_tree_state_hash` function produces the same digest format over a single directory tree when the build system has not already supplied a hash; it rejects symlinks and requires a directory root, and `internal/buildinfo` produces identical digests for identical trees.

Provider budget is a typed `ProviderBudget` model with `max_usd` (required, non-negative) and optional `max_tokens` and `max_requests` (non-negative). The build system supplies these through `G8E_EVALS_PROVIDER_BUDGET_MAX_USD`, `G8E_EVALS_PROVIDER_BUDGET_MAX_TOKENS`, and `G8E_EVALS_PROVIDER_BUDGET_MAX_REQUESTS`.

Production-posture runs (the live `g8e-evals run` path) require source/build provenance and fail before execution when it is unavailable. Non-production runs (the synthetic `g8e-evals bench-synthetic` path) may omit provenance entirely; the check skips the env-var fallback when `is_production_posture=False` and no provenance is supplied on the request. When provenance is supplied on the request, it is always validated regardless of posture.

Bundle verification layer 12 (`SOURCE_BUILD_PROVENANCE`) binds source/build provenance into the signed bundle and verification report. It checks that `source_build_provenance` is present for production-posture runs, validates `source_revision` is non-empty, and validates `source_tree_state_hash` is a 64-char hex string. Non-production runs may omit provenance.

## Run live evaluations

1. From the repository root, set `REPO_ROOT="$PWD"` and set `EVIDENCE_KEY_FILE` to the evidence key file location before changing directories. Create the key file as a JSON object with `version` set to `1`, a non-empty `key_id`, and `key_b64` set to exactly 32 bytes of base64-encoded random key material. Make the file owner-only with `chmod 600`; symlinks and files with group or other permission bits are rejected.
2. Start the required local stack and refresh the canonical CLI identity with `./g8e auth refresh` after Operator enrollment.
3. Set `G8E_APP_TRUST_BUNDLE` and `G8E_GATEWAY_TRUST_BUNDLE` to the canonical `.g8e/pki/trust/g8eg-ca-bundle.pem` when the eval process cannot resolve that runtime-relative path from its working directory.
4. Enter `ensemble/evals/` and install the locked environment with `uv sync --locked --extra test`.
5. Run the suite, for example: `uv run --locked g8e-evals run --suite ifeval_subset --arm doctrine --auth-project-root "$REPO_ROOT" --evidence-key-file "$EVIDENCE_KEY_FILE"`.

The `--g8e-cli` option (env var `G8E_CLI_BIN`) selects the platform binary used for the canonical authentication context and build provenance. When omitted, resolution tries the repo-root `./g8e` produced by `make build`, then `bin/g8e-<os>-<arch>`, then `g8e` on `PATH` — the flags above work from `ensemble/evals/` without setting `G8E_CLI_BIN`. An explicit path must exist and be executable. The value `auto` fetches the host platform binary from the running Gateway's unauthenticated `/.well-known/g8e/bin/` endpoint (the binaries baked by `make build-all` inside the compose image, or `bin/` beside a natively run gateway), caches it under `~/.cache/g8e-evals/bin/` with a conditional `If-Modified-Since` revalidation, and guarantees the CLI is byte-identical to the platform under test; the fetch targets `G8E_GATEWAY_HTTP_URL`, defaulting to `http://localhost:8080`. `make evals-bin` verifies the repo-root binary's stamped `source_tree_state_hash` against the current source tree and rebuilds it when missing or stale.

Ensemble and governed arms require `--auth-project-root`, a valid result from `g8e auth context`, and an evidence key. `--g8ee-url`/`G8E_G8EE_URL` is optional: the evals client posts to the g8ee app directly and defaults to `http://localhost:8000`, the port docker-compose publishes and the Dockerfile binds — set it only for a non-local deployment. The `direct` arm bypasses g8ee, the Gateway, and the Operator entirely and needs none of the stack options.

The direct arm requires an explicit primary provider and model. g8ee arms can obtain role settings from the running application or from CLI options and `G8E_TEST_LLM_*` environment variables. OpenAI, Anthropic, and Gemini require applicable credentials; Ollama, llama.cpp, and the deterministic fake provider are keyless. Use `uv run --locked g8e-evals run --help` for role, endpoint, judge, timeout, output, task-limit, and headless approval options.

## Run synthetic benchmarks

Install the locked environment as described above, then run a suite without stack credentials or an external evidence key. For example: `uv run --locked g8e-evals bench-synthetic --suite governance_adversarial --output-dir reports`.

Use `--gold-set` to replace the suite dataset and `--limit` to run only the leading tasks. A custom task still requires typed assertions recognized by the selected suite and its registered graders.

## Run finite attended controllers

`g8e-evals controller` runs one finite, owner-approved cycle manifest. It is an attended entry point, not a daemon: it executes the manifest's ordered child commands through a closed command map, verifies every completed report, validates the exact candidate-tree digest, writes a durable publication outbox, invokes `g8e public push`, and exits in a typed terminal state. The controller never generates or changes an authority.

A cycle manifest binds the cycle identity and revision, ordered content-addressed child commands, report root, controller outbox root, stop conditions, aggregate-verification requirement, strict-validation requirement, and approved candidate digest. `controller run` requires aggregate verification, strict validation, and an approved digest. Runtime paths are normalized relative paths; absolute paths and traversal fail closed. A replacement cycle uses a fresh cycle identity, report root, outbox root, and rule-derived child identity. The interrupted report remains dead evidence.

Run and inspect a cycle from `ensemble/evals/`:

```bash
uv run --locked g8e-evals controller run --manifest <cycle-manifest.json> --work-dir <controller-work-dir> --controller-id <fresh-controller-id> --g8e-cli <g8e-binary>
uv run --locked g8e-evals controller status --work-dir <controller-work-dir>
```

`controller stop` writes an identity-bound, content-addressed stop request. Without `--immediate`, the controller finishes the active child and stops before starting another child. With `--immediate`, the attended subprocess is terminated and the in-flight report is retained as dead evidence. A malformed request or a request bound to another controller or cycle fails closed.

```bash
uv run --locked g8e-evals controller stop --work-dir <controller-work-dir>
uv run --locked g8e-evals controller stop --work-dir <controller-work-dir> --immediate
```

`controller recover` has two distinct modes. Execution recovery seals an interrupted active child as dead evidence and transitions the controller to `safety_stopped`; it does not resume that report. Publication recovery applies only to a terminal `mirror_outage`. It reloads and validates the frozen manifest, state hash, transition chain, completed child set, original report directories, approved candidate digest, and ordered durable outbox, then invokes only `g8e public push`. Each retry attempt and success is durable. The controller reaches `completed` only after every outbox entry is published, and publication recovery never constructs or invokes a child handler.

```bash
uv run --locked g8e-evals controller recover --work-dir <controller-work-dir>
uv run --locked g8e-evals controller recover --work-dir <controller-work-dir> --publication --g8e-cli <g8e-binary>
```

Provider cost comes only from eligible `provider_cost_usd` observations in completed child `metrics.jsonl` files. The controller accumulates observed cost and enforces the manifest's aggregate USD ceiling. It also enforces maximum used disk and minimum free-disk reserve before each child. Verifier, disclosure, trust, integrity, authority, digest, budget, disk, child, and immediate-stop failures stop the finite cycle according to their typed reason. Publication retry reads immutable reports and the durable outbox; it never reruns inference.

## Report bundle contract

The schema version comes from `manifest.json`. Readers reject unsupported versions, unknown fields, duplicate identities, missing references, invalid parent-stage graphs, digest mismatches, and incomplete evidence.

Both live and synthetic reports contain:

- `manifest.json`: Run identity, selected analytical arm, environment metadata, and content hashes.
- `tasks.jsonl`: Typed task definitions, assertions, compatible arms, and grader references.
- `attempts.jsonl`: Terminal outcomes and references to receipts, observations, stages, metrics, and evidence.
- `receipts.jsonl`: Typed receipt observations; this file can be empty.
- `stages.jsonl`: Normalized stage records; synthetic suites currently write an empty file.
- `metrics.jsonl`: Versioned metric observations linked to attempts and evidence.
- `evidence-index.jsonl`: Content hashes, lengths, classifications, storage locations, and access metadata for indexed artifacts.
- Assertion-specific `*-observations.jsonl` files: Typed observations for the boundaries supported by that command. Supported but unused observation files are present and empty.

Both live and synthetic reports additionally contain the canonical analysis artifacts, which are the authoritative release-facing output derived from the complete immutable input record:

- `analysis-input.json`: The complete validated `AnalysisInputRecord` containing every immutable input that can influence analysis, with a content hash over the canonical bytes.
- `analysis.json`: Canonical analysis JSON produced by `compute_canonical_analysis_from_record`, including metric results, confusion matrices, paired comparisons, and preregistration metadata.
- `analysis.md`: Markdown rendering of the canonical analysis.
- `analysis.html`: HTML rendering of the canonical analysis.
- `analysis.txt`: Plain-text CLI rendering of the canonical analysis.

Live reports additionally contain encrypted evidence envelopes and `diagnostic-results.jsonl`, a non-authoritative per-attempt diagnostic derived from typed records. It is not an independent evidence source and must not be presented as release evidence. Synthetic reports instead contain content-addressed files under `evidence/` and any local simulator state needed by the selected suite.

The external live evidence key is never embedded in the report. Retaining it allows named key holders to decrypt restricted evidence out of band; deleting it makes those encrypted artifacts unrecoverable.

## Report lifecycle

A report progresses through distinct lifecycle states, each with specific evidence and validation properties.

### Partial reports

A report directory is partial when one or more required artifacts are missing. The `is_report_complete` function in `g8e_evals/report/completeness.py` checks for the presence of `manifest.json`, `attempts.jsonl`, `metrics.jsonl`, and `analysis.json`. A partial report returns `False`; it is never treated as complete. A corrupted artifact (invalid JSON in `manifest.json` or `analysis.json`) raises `ReportCompletenessError` rather than returning `False`, distinguishing corruption from incompleteness.

A process-killed partial directory remains immutable evidence of interruption but is never treated as complete. Resume creates a new report directory for a missing or replacement assignment and writes a new index generation.

### Complete reports

A report is complete only when all required artifacts exist and are internally consistent. The standalone report validator (`validate_standalone_report` in `g8e_evals/report/validate.py`) checks manifest, expected terminal attempts, metrics, evidence index, analysis summary, and report checksum. It returns a typed `StandaloneReportResult` with `ok`, `checked_layers`, and `failures`.

### Finalized reports

A report is finalized only after its manifest, expected terminal attempts, metrics, evidence index, summary, and report checksum validate. The campaign runner writes a `FINALIZATION` index generation after all assignments are complete and the campaign is finalized. The finalization generation carries the complete report checksums and assignment dispositions.

### Index generations and supersession

The campaign runner writes append-only index generations to `campaign-index.jsonl`. Each `IndexGeneration` carries a parent-generation hash, creation reason (`INITIAL`, `RESUME`, `SUPERSESSION`, `FINALIZATION`), complete report checksums, and assignment dispositions. The first generation has a zero parent hash (`"0" * 64`); each subsequent generation's parent hash must match the previous generation's content hash. Generation numbers are contiguous starting from zero.

Each assignment has exactly one disposition per generation: `EFFECTIVE` (exactly one effective valid report), `SUPERSEDED` (replaced due to interruption or infrastructure failure), `QUALIFICATION` (typed qualification outcome: unavailable, license-blocked, incompatible, out-of-memory, backend-unsupported), or `UNAVAILABLE` (no report). Exactly one effective valid report exists per publishable assignment.

Supersession is permitted only for interrupted or infrastructure-invalid assignments. A completed valid assignment cannot be superseded. Model, governance, human, timeout, and invalid-evidence failures are assignment-scoped and cannot be superseded. Result-aware discretionary reruns create a new campaign revision and cannot replace an unfavorable valid result.

Resume reads the persisted schedule and existing attempts without rerandomizing or discarding failures. Resume creates a new `RESUME` index generation rather than mutating existing ones.

## Evidence semantics

Every artifact in a report directory has defined evidence semantics: what it proves, what it does not prove, and how it is bound.

### Manifest (`manifest.json`)

The `RunManifest` binds the run identity, suite identity, eval package version, selected arm, requested posture, role-to-model mapping, runtime environment, content hashes, source/build provenance, provider budget, and stack environment. It is written before task execution. Content hashes bind the task bundle, prompt bundle, grader bundle, and dataset to immutable SHA-256 digests. The manifest is the root evidence record; all other artifacts reference it.

### Attempts (`attempts.jsonl`)

Each `AttemptRecord` records a terminal outcome (completed, model failure, governance rejection, human denial, timeout, infrastructure failure, or invalid evidence) and references receipts, observations, stages, metrics, and evidence by stable identifier. Every effective attempt must be terminal. A completed attempt must have at least one bound metric.

### Metrics (`metrics.jsonl`)

Each `MetricObservation` is a versioned metric observation linked to an attempt and evidence. Metrics are registered by metric ID and version with unit, direction, eligible population, denominator semantics, missing-value policy, aggregation method, uncertainty method, evidence requirements, grader class, and any release threshold. The runner rejects unregistered metrics and rows whose unit or grader class does not match the registry. Consumers aggregate only eligible rows according to each metric definition and keep unsupported exclusions out of the denominator.

### Evidence index (`evidence-index.jsonl`)

Each evidence index entry records content hash, byte length, classification (public or restricted), storage location, and access metadata for an indexed artifact. Raw prompts, model outputs, and agent trails are restricted evidence stored as AES-256-GCM envelopes with authenticated index metadata. Analytical records retain hashes, lengths, counts, types, and evidence references instead of raw restricted values.

### Analysis (`analysis.json`, `analysis-input.json`)

The canonical analysis is the authoritative release-facing output. `analysis-input.json` is the complete validated `AnalysisInputRecord` containing every immutable input that can influence analysis, with a content hash over the canonical bytes. `analysis.json` is produced by `compute_canonical_analysis_from_record` and includes metric results, confusion matrices, paired comparisons, and preregistration metadata. The analysis is reproducible: the same input record always produces the same analysis output.

### Report checksum (`report-checksum.json`)

The optional report checksum file records a SHA-256 over the report's canonical content. The standalone report validator checks the checksum if present; a mismatch fails closed.

### Campaign index (`campaign-index.jsonl`)

The append-only campaign index records every index generation with its parent-generation hash, creation reason, report checksums, and assignment dispositions. The index chain is the campaign-level evidence record: it proves that the campaign progressed through valid states, that exactly one effective valid report exists per publishable assignment, and that supersession followed the frozen policy.

### Statistical analysis (`StatisticalAnalysisRecord`)

The optional `StatisticalAnalysisRecord` on `CanonicalEvalAnalysis` records the statistical method, independent unit, population, correction family, estimates, intervals, practical thresholds, and claim status. Descriptive-only evidence produces no winner, superiority, production-suitability, or broad-quality language.

## Security requirements

The evals package enforces security through fail-closed checks at every boundary.

### Fail-closed validation

Preflight, bundle verification, campaign verification, standalone report validation, source provenance verification, and public projection all fail closed on any discrepancy. A missing, corrupted, or inconsistent artifact is an error, not a warning. Unknown fields are rejected by `extra="forbid"` on every typed model. Path traversal, symlinks, duplicate identities, missing references, invalid parent-stage graphs, digest mismatches, and incomplete evidence are rejected.

### Source provenance

Source provenance is checksum-bound from an explicit reviewed inclusion manifest. The verifier checks for missing files, escaping files (on disk but not in the manifest), duplicate paths, symlinks, path traversal, checksum mismatch, byte-length mismatch, and manifest-hash mismatch. All checks fail closed. The runner never runs ad hoc Git commands to populate source/build provenance; all provenance comes from environment variables set by the trusted build system or CI pipeline.

### Public projection safety

Public projections contain only allowlisted fields: campaign identity, variant identity, task identity, metric identity, numerator, denominator, rate, unit, verification status, and evidence link. No raw prompts, outputs, keys, credentials, private endpoints, machine-specific paths, evidence-key metadata, encrypted artifact locations, private download locations, or fields outside the explicit schema allowlist cross the projection boundary. The projection function fails closed on any prohibited field, unknown field, non-finite value, or path traversal in the evidence link.

### File safety

The campaign verifier checks that every required artifact is a regular file (not a symlink). The source provenance verifier rejects symlinks anywhere in the source tree. The bundle verifier validates rooted inventory, rejects path traversal, and enforces file size and count limits. Evidence keys and encrypted artifacts are stored with owner-only permissions; symlinks and group/other access are rejected.

### Restricted evidence

Raw prompts, model outputs, and agent trails are restricted evidence. The live runner requires an owner-only key file and stores these artifacts as AES-256-GCM envelopes. The external evidence key is never embedded in the report. Analytical records retain hashes, lengths, counts, and types instead of raw restricted values. The disclosure policy excludes prompts, outputs, chain-of-thought, raw trails, user email, session IDs, host paths, credentials, evidence keys, encrypted-evidence key-discovery metadata, and private host data from public projections.

## Verify receipts

`verify-receipts` is a diagnostic primitive, not complete eval verification. Reverify receipt signatures with `uv run --locked g8e-evals verify-receipts <report-directory> --pki-dir <verifier-pki-directory>`. The PKI directory must contain each producing signer's `*Actuator_pub.pem` file. Add `--json` for a machine-readable result bound to the run ID in a valid manifest.

The verifier derives each key ID, matches it to `signer_key_id`, and verifies the canonical receipt signature and final persistence attestation. A receipt-verification claim requires a nonzero receipt count, no missing keys, no failures, and verified counts equal to the total. A zero-receipt report is not evidence that receipt verification passed.

This command does not validate the complete report graph, dataset hashes, encrypted evidence, commitment ledger, or trustworthiness of supplied public keys. It is a receipt-signature diagnostic primitive, not a complete offline bundle verifier. Complete eval verification requires the immutable bundle contract, signing identity, and assessed-trust policy defined in later phases.

## Eval bundle signing identity and assessed trust

The `g8e_evals.bundle.signing` module defines the dedicated Ed25519 eval-run signing identity and the external assessed-trust input. The signing identity is not an actuator receipt identity and is never represented as an `ActionReceipt`. `EvalSigningKey` wraps an Ed25519 key pair; its `key_id` is the hexadecimal encoding of the raw public key, matching the platform receipt verifier convention. `sign_bundle` signs the canonical manifest root and checksum root bytes, binding the signature algorithm, key ID, bundle ID, run ID, release version, and SHA-256 signed digest to each signature.

The verifier never trusts a key merely because the bundle contains it. `EvalTrustStore` is the external assessed-trust input supplied by the verifier out of band from protocol-owned public-key metadata. `EvalTrustedKey` declares each key's algorithm, scope, validity window, revocation state, and provenance source. `verify_bundle_signature` assesses each signature against the trust store and fails closed for unknown, revoked, expired, wrong-scope, wrong-algorithm, malformed, and substituted keys and signatures. A signature is accepted only when both the manifest and checksum-root signatures are `TRUSTED`.

The signing identity, trust models, and complete offline verifier are implemented and wired into the `g8e-evals verify <bundle>` command. The verifier reads a bundle directory and an externally supplied trust store through twelve ordered layers, each producing typed failures with stable failure codes. The verifier validates rooted inventory, schemas/canonical, file hashes, signatures/trust, record bindings, envelope/receipt, chain links, metric producers, analysis reproduction, renderer equality, privacy separation, and source-build provenance.

## Published README evidence

The reviewed snapshot under `docs/evidence/readme/current/` contains hash-safe projections rather than private report directories. The current publication preserves the v2.1.5 five-task Stage 1 diagnostic and adds a separately invoked v2.1.6 Stage 2 reproduction with campaign, source, component, image, and environment provenance. Both runs used the same local operator environment and produced no receipts, so the snapshot supports deterministic instruction-following comparison and independent execution state, not receipt, mutation, persistence, governance, compliance, causal, statistical, or external-audit claims.

The projection, provenance collection, exact-digest approval, and promotion procedures are documented in [Release Process](../devs/release_process.md#stage-1-real-agent-readme-evidence). `scripts/generate_readme.py` validates the promoted snapshot and renders `README.md`; it does not run evaluations, decrypt evidence, or verify signatures.

The generated README distinguishes measured models from planned candidates. The only measured cohort is `ollama/gemma4:12b`, `ollama/gemma4:e4b`, and `ollama/gemma4:e2b` in the two existing five-task executions. The `## Planned Model Evaluation` section lists candidate model families labeled `candidate; not yet compared by g8e` with no ranks, scores, or performance claims. The candidate roster expands during the generative campaign Phase 0 as canonical identities and backend artifacts are resolved against primary upstream sources.

## Tests and lint

The standalone eval package uses a three-tier test model. Every test must declare exactly one tier marker (`unit`, `integration`, or `e2e`); tests without a marker are rejected at collection time.

| Tier | Name | Marker | External dependencies | Execution time |
| --- | --- | --- | --- | --- |
| 1 | Unit | `@pytest.mark.unit` | None (no filesystem, process, network, database, or provider) | < 10ms per test |
| 2 | Integration | `@pytest.mark.integration` | Local filesystem, subprocess, or in-process dependencies | < 2s per suite |
| 3 | E2E | `@pytest.mark.e2e` | Live g8e stack or provider | < 30s per suite |

Tier 1 tests are pure: they use stubs and mocks for external dependencies only, never touch the filesystem, spawn processes, open network connections, access databases, or call providers. Tier 2 tests use real local filesystem operations, in-process pub/sub, and local PKI certificates. Tier 3 tests require a live g8e stack or remote provider and are not part of the offline test targets.

From the repository root, run:

- `make evals-test`: Tier 1 and Tier 2 tests.
- `make evals-test-unit`: Tier 1 tests with no filesystem, process, network, database, or provider dependencies.
- `make evals-test-integration`: Tier 2 tests with local files, subprocesses, or in-process dependencies.
- `make evals-lint`: Ruff and Pyright checks for the standalone package.

Live stack and provider evaluations are Tier 3 and are not part of the offline test targets. Test filenames describe their scope (e.g., `test_source_provenance.py`, `test_campaign_verifier.py`); generic names like `edge_test.py` or `coverage_test.py` are not used. Test function names describe the specific behavior being verified, not generic categories.

## Model registry and campaign profile

The 46-model comparison campaign requires a typed, versioned private model registry and a frozen campaign profile. These contracts extend the existing `CampaignSpec`/`CampaignManifest`/`ModelCohort` contracts in `campaign.py` rather than replacing them.

### Model registry

`g8e_evals/registry.py` defines the typed model registry. Each `ModelVariant` records its stable campaign model-variant ID, corrected canonical display name, source-list alias, upstream Hugging Face repository and immutable revision SHA, retrieval date, SPDX license identifier, license-text hash, gated status, publication eligibility, parameter count and architecture, backend and artifact identity (backend name/version, served model tag, artifact digest, artifact bytes, quantization, tensor format), tokenizer and chat-template identity, reasoning mode, hidden reasoning tokens, and weight class. A changed checkpoint revision, quantization, chat template, reasoning mode, or backend becomes a distinct variant; results from distinct variants are never silently merged.

`QualificationRecord` records unavailable, license-blocked, incompatible, out-of-memory, or backend-unsupported outcomes with an evidence-backed reason. A variant with a non-runnable qualification outcome remains in the registry but is excluded from measured campaign cells.

`ModelRegistry` binds all variants and qualification records with a content-addressed hash. It rejects duplicate variant IDs, qualification records that reference unknown variant IDs, and content-hash mismatches. The `runnable_variant_ids()` method returns sorted variant IDs that have no non-runnable qualification record.

### Campaign profile

`g8e_evals/profile.py` defines the frozen campaign profile. `CampaignProfile` binds the campaign ID, revision, schema version, purpose, lifecycle status, generative variant IDs, benchmark and dataset identities, task population, repetitions, track-to-arm assignments (`TrackArmAssignment`), model-to-tier assignments (`ModelTierAssignment`), baseline tier mappings, routing policy, effective sampling/context/timeout/retry settings, warm-up and concurrency policies, hardware identity, environment stratum, primary metrics, unit of analysis, claim boundary (`ClaimBoundary`: descriptive-only or confirmatory), and model registry hash. The profile is frozen and hashed before the first measured run; any material change creates a new campaign revision.

`validate_against_registry()` checks that the model registry hash matches, every generative variant ID exists in the registry, and every model-to-tier assignment references a variant in the registry.

### Campaign subcommands

The `g8e-evals campaign` command is a Click group with `run`, `validate`, and `plan` subcommands. `run` executes a campaign with a frozen specification (the existing v2.1.8 pipeline-integrity campaign). `validate` performs side-effect-free validation of a campaign profile against a model registry: it loads both, checks variant ID consistency and hash matching, and reports the result without making provider calls, writing files, or starting network operations. `plan` produces a deterministic dry-run output showing exact run, task, warm-up, measured-call, disk, and declared remote-cost ceilings without making provider calls.

`campaign run` creates one campaign identity, one report directory, one assignment manifest, one randomized schedule, and one final canonical analysis. Resume reads the persisted schedule and existing attempts; it does not rerandomize or discard failures. The runner writes append-only index generations (`campaign-index.jsonl`) recording `INITIAL`, `RESUME`, and `FINALIZATION` generations, each carrying a parent-generation hash, creation reason, report checksums, and assignment dispositions.

`campaign validate` loads a campaign profile and model registry, checks that every generative variant ID exists in the registry, every model-to-tier assignment references a variant in the registry, and the profile's model registry hash matches the registry's content hash. It prints the campaign ID, revision, variant count, benchmark count, task count, repetition count, track count, and registry identity.

`campaign plan` computes the deterministic Fisher-Yates schedule from the frozen profile seed and prints the exact run order, schedule seed, total assignments, warm-up calls, measured calls, and disk ceiling without executing any assignments. The same inputs always produce the same output.

### Campaign verification

The offline campaign verifier (`g8e_evals.campaign_verify.verify_campaign`) reads a campaign report directory and checks the complete matrix without network access, provider calls, or external services. It emits a typed `CampaignVerificationReport` with verification status, campaign identity, verified index generation hash, checked layers, and typed failures. A campaign with missing or inconsistent cells cannot produce a passing publication candidate.

The verifier checks nine ordered layers:

1. **file_safety**: Required artifacts exist as regular files (no symlinks). Required artifacts: `manifest.json`, `attempts.jsonl`, `metrics.jsonl`, `analysis.json`, `campaign-manifest.json`, `campaign-assignments.jsonl`, `campaign-index.jsonl`.
2. **standalone_report**: The report passes standalone report validation (manifest, attempts, metrics, evidence index, analysis, report checksum).
3. **campaign_manifest**: `campaign-manifest.json` is a valid `CampaignManifest`.
4. **index_chain**: Index generations form a valid append-only parent-hash chain with contiguous generation numbers starting from zero.
5. **duplicate_effective**: No duplicate effective assignments in any generation. Exactly one effective valid report per publishable assignment.
6. **cell_coverage**: Every campaign assignment has a disposition in the final index generation.
7. **terminal_attempts**: Every effective attempt is terminal (completed, model failure, governance rejection, human denial, timeout, infrastructure failure, or invalid evidence).
8. **metric_binding**: Every completed attempt has at least one bound metric.
9. **manifest_identity**: Run manifest content hashes are present and consistent with the campaign manifest.

### Source provenance

Source provenance is collected from an explicit reviewed inclusion manifest (`g8e_evals/provenance.py`) without relying on repository history. The manifest is a frozen, `extra="forbid"` typed model listing every expected source file with its relative path, SHA-256, and byte length. A manifest hash binds the entire manifest content so any tampering is detected.

The verifier (`verify_source_provenance`) checks the manifest against the on-disk source tree in seven ordered layers: manifest hash, duplicate paths, path traversal, symlink scan, missing files, escaping files, and checksum verification (SHA-256 and byte length). All layers run regardless of earlier failures so the result reports every discrepancy. Missing, escaping, duplicate, symlinked, or changed source entries fail closed.

## Browser Publication and Observe Projections

The observe frontend exposes eval summaries, eval details, and downloads through the browser-scoped observe read API. The eval publication path and the `g8e-evals verify <bundle>` command are implemented in the current working tree. The `verify` command performs twelve-layer offline verification with external assessed trust; the existing `verify-receipts` command remains a receipt-signature diagnostic primitive and must not be presented as complete eval verification. The publication path is campaign-aware: `build_publication_request` derives `campaign_id`, `arm_ids`, `model_cohort_ids`, and `assignment_count` from the canonical analysis and campaign records, and the observe projection carries these campaign dimensions. Single-arm diagnostic runs (no campaign manifest) produce `campaign_id=""`, `arm_ids=[single]`, `model_cohort_ids=[]`, `assignment_count=0`.

### Current status: campaign-aware publication implemented

The `g8e-evals verify <bundle>` command and the campaign-aware eval publication path are implemented. The publication protocol, Python models, Go models, Gateway validator/producer/read service, Python publisher, and dashboard contract pack carry campaign dimensions (`campaign_id`, `arm_ids`, `model_cohort_ids`, `assignment_count`). The `g8e.v1.ai.eval` event family is classified as `produced_to_sse` with `dashboard_safe: true`. The observability implementation does not solve the broader 46-model eval campaign plan inside a frontend change; the v2.1.8 campaign is a bounded pipeline-integrity diagnostic (2 cohorts, 2 arms, 5 tasks, 2 replicates, 40 assignments, descriptive-only claims).

### Current prerequisite state

The `g8e-evals verify <bundle>` command produces a typed verified-bundle result containing run identity, suite and arm identity, assigned and terminal counts, registered metric rows, receipt count, source schema/version, canonical analysis hash, verification report, and public/restricted artifact classification. Verification statuses map to `projection_validated`, `receipt_verification_not_applicable`, and `verified`. The `verified` status requires the complete eval-native verifier; receipt-only verification cannot produce it. Several semantic verification layers are incomplete; the verifier exists but does not yet validate the full semantics named by every layer interface.

The governed ingress used by `g8e-evals publish` uses an enrolled workload identity or existing governed CLI ingress, never a browser session and never an unauthenticated route.

### Publication boundary

The implemented publication path is campaign-aware. The publication path:

1. Define a typed publication request and result with source run ID, source schema/version, canonical analysis hash, publication timestamp, verification status, and exact projection SHA-256.
2. Read and verify the completed bundle first. Build the browser projection only from verified typed records; never parse `summary.json` as authoritative.
3. Apply a typed disclosure policy that excludes prompts, outputs, chain-of-thought, raw trails, user email, session IDs, host paths, credentials, evidence keys, encrypted-evidence key-discovery metadata, and private host data.
4. Canonically serialize the projection, compute its SHA-256, and persist it through the governed mutation path (five-layer verification gauntlet).
5. Emit one `ai.eval.metric.recorded` event per eligible published metric and one `ai.eval.run.completed` event only after successful projection persistence. If persistence fails, emit neither.
6. Update or create the corresponding run projection with `run_kind="eval"` using the eval run ID. Do not collapse it into an investigation ID.
7. Build the download catalog only from deterministic public-safe artifacts with media type, byte size, SHA-256, privacy classification, source run ID, generation time, and authenticated URL.
8. Implement authenticated download streaming with ownership checks, fixed content type, fixed content length, digest verification, safe disposition, and rooted path handling.
9. Reject path traversal, symlinks escaping the publication root, duplicate IDs, hash mismatch, size mismatch, oversized files, unknown media types, restricted artifacts, and post-catalog file substitution.

### Verification labels

The observe frontend displays verification labels honestly:

- `verified` — requires the complete eval-native verifier; the verifier is implemented but several semantic layers are incomplete.
- `projection_validated` — the projection was validated against the verified bundle.
- `receipt_verification_not_applicable` — receipt verification is not applicable to this run.

The frontend displays `verified` only when the complete eval-native verifier passes every layer. Partial verification is shown as `projection_validated`, never as `verified`.

### Event classification

The `g8e.v1.ai.eval` event family is classified as `produced_to_sse` with `dashboard_safe: true` in `protocol/constants/event_dashboard_classification.json`. The two eval events (`ai.eval.run.completed`, `ai.eval.metric.recorded`) are registered with protocol-owned payloads carrying campaign dimensions (`campaign_id`, `arm_ids` for run-completed; `model_cohort_id`, `arm_id` for metric-recorded). The campaign-aware publication path is verified through the Gateway mTLS eval publication endpoint with real HTTP integration tests that assert persist-before-publish ordering, typed SSE envelopes, and cross-user isolation.

## Related

- [Testing](tests.md): g8ee and standalone eval test boundaries.
- [Governance](governance.md): the five-layer execution pipeline and posture behavior.
- [Architecture](../architecture/ensemble.md): g8ee's role in the platform.
- [Protocol Library](../architecture/protocol.md): canonical receipt parsing and verification.
- [Headless UX Smoke Test](../guides/ux_smoke_test.md): starting a governed local stack.
