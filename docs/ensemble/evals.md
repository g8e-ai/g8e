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

The live runner completes authentication and provider preflight before creating a report directory. It then writes `manifest.json` before task execution and records the suite identity, eval package version, selected arm, requested posture, role-to-model mapping, runtime environment, and content hashes. The manifest schema also defines source revision, source tree state, sampling, context-limit, preregistration, and redacted-configuration fields, but the current CLI does not populate those fields. A report therefore does not establish source-state or sampling provenance unless a separate reviewed process binds it.

Attempts use typed terminal states for completion, model failure, governance rejection, human denial, timeout, infrastructure failure, and invalid evidence. Normalized stages cover model inference, Tribunal generation and auditing, L1 doctrine, protocol L2, L3 ceremony, L4 verification, L5 execution, scrubbing, rehydration, receipt persistence, commitment append, and grading. Model stages record available timing, usage, retry, finish-reason, boundary-hash, and privacy-attestation data; missing provider usage remains missing rather than being estimated.

Governed attempts collect canonical `ActionReceipt` protobuf messages produced and signed by L5 Actuators. Receipt grading verifies the signature, final persistence attestation, expected action class, transaction and identity bindings, stage ordering, posture-specific L2 and L3 status, execution result, state roots, and parent relationships. Policy, final-state, canary, rehydration, secret-detection, mutation, tampering, replay, identity, and evidence-preservation graders run only when the task declares the corresponding typed assertion and supplies the required evidence.

Integrations can inject boundary-specific observers through the evaluation harness. The standard live CLI does not configure those observers, so an assertion that depends on an absent observer cannot become verified evidence merely from task configuration.

Raw prompts, model outputs, and agent trails are restricted evidence. The live runner requires an owner-only key file and stores these artifacts as AES-256-GCM envelopes with authenticated index metadata. Analytical records retain hashes, lengths, counts, types, and evidence references instead of raw restricted values.

### Metric contract

Every metric is registered by metric ID and version with its unit, direction, eligible population, denominator semantics, missing-value policy, aggregation method, uncertainty method, evidence requirements, grader class, and any release threshold. The runner rejects unregistered metrics and rows whose unit or grader class does not match the registry. Consumers aggregate only eligible rows according to each metric definition and keep unsupported exclusions out of the denominator.

## Run live evaluations

1. From the repository root, set `REPO_ROOT="$PWD"` and set `EVIDENCE_KEY_FILE` to the evidence key file location before changing directories. Create the key file as a JSON object with `version` set to `1`, a non-empty `key_id`, and `key_b64` set to exactly 32 bytes of base64-encoded random key material. Make the file owner-only with `chmod 600`; symlinks and files with group or other permission bits are rejected.
2. Start the required local stack and refresh the canonical CLI identity with `./g8e auth refresh` after Operator enrollment.
3. Set `G8E_APP_TRUST_BUNDLE` and `G8E_GATEWAY_TRUST_BUNDLE` to the canonical `.g8e/pki/trust/g8eg-ca-bundle.pem` when the eval process cannot resolve that runtime-relative path from its working directory.
4. Enter `ensemble/evals/` and install the locked environment with `uv sync --locked --extra test`.
5. Run the suite, for example: `uv run --locked g8e-evals run --suite ifeval_subset --arm doctrine --g8ee-url http://localhost:8000 --g8e-cli "$REPO_ROOT/g8e" --auth-project-root "$REPO_ROOT" --evidence-key-file "$EVIDENCE_KEY_FILE"`.

Every `run` invocation currently requires `--g8ee-url`, `--auth-project-root`, a valid result from `g8e auth context`, and an evidence key, including the `direct` arm. The direct arm bypasses g8ee, the Gateway, and the Operator during task execution, but the current CLI still performs the common authentication setup before selecting that execution path.

The direct arm requires an explicit primary provider and model. g8ee arms can obtain role settings from the running application or from CLI options and `G8E_TEST_LLM_*` environment variables. OpenAI, Anthropic, and Gemini require applicable credentials; Ollama, llama.cpp, and the deterministic fake provider are keyless. Use `uv run --locked g8e-evals run --help` for role, endpoint, judge, timeout, output, task-limit, and headless approval options.

## Run synthetic benchmarks

Install the locked environment as described above, then run a suite without stack credentials or an external evidence key. For example: `uv run --locked g8e-evals bench-synthetic --suite governance_adversarial --output-dir reports`.

Use `--gold-set` to replace the suite dataset and `--limit` to run only the leading tasks. A custom task still requires typed assertions recognized by the selected suite and its registered graders.

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

Live reports additionally contain encrypted evidence envelopes, `results.jsonl`, and `summary.json`. The latter two are compatibility views derived from typed records and are not independent evidence sources. Synthetic reports instead contain content-addressed files under `evidence/` and any local simulator state needed by the selected suite; they do not write `results.jsonl` or `summary.json`.

The external live evidence key is never embedded in the report. Retaining it allows named key holders to decrypt restricted evidence out of band; deleting it makes those encrypted artifacts unrecoverable.

## Verify receipts

Reverify receipts with `uv run --locked g8e-evals verify-receipts <report-directory> --pki-dir <verifier-pki-directory>`. The PKI directory must contain each producing signer's `*Actuator_pub.pem` file. Add `--json` for a machine-readable result bound to the run ID in a valid manifest.

The verifier derives each key ID, matches it to `signer_key_id`, and verifies the canonical receipt signature and final persistence attestation. A receipt-verification claim requires a nonzero receipt count, no missing keys, no failures, and verified counts equal to the total. A zero-receipt report is not evidence that receipt verification passed.

This command does not validate the complete report graph, dataset hashes, encrypted evidence, commitment ledger, or trustworthiness of supplied public keys. It is a receipt verifier, not a complete offline bundle verifier.

## Published README evidence

The reviewed snapshot under `docs/evidence/readme/current/` contains hash-safe projections rather than private report directories. The current publication preserves the v2.1.5 five-task Stage 1 diagnostic and adds a separately invoked v2.1.6 Stage 2 reproduction with campaign, source, component, image, and environment provenance. Both runs used the same local operator environment and produced no receipts, so the snapshot supports deterministic instruction-following comparison and independent execution state, not receipt, mutation, persistence, governance, compliance, causal, statistical, or external-audit claims.

The projection, provenance collection, exact-digest approval, and promotion procedures are documented in [Release Process](../devs/release_process.md#stage-1-real-agent-readme-evidence). `scripts/generate_readme.py` validates the promoted snapshot and renders `README.md`; it does not run evaluations, decrypt evidence, or verify signatures.

## Tests and lint

From the repository root, run:

- `make evals-test`: Tier 1 and Tier 2 tests.
- `make evals-test-unit`: Tier 1 tests with no filesystem, process, network, database, or provider dependencies.
- `make evals-test-integration`: Tier 2 tests with local files, subprocesses, or in-process dependencies.
- `make evals-lint`: Ruff and Pyright checks for the standalone package.

Live stack and provider evaluations are Tier 3 and are not part of the offline test targets.

## Related

- [Testing](tests.md): g8ee and standalone eval test boundaries.
- [Governance](governance.md): the five-layer execution pipeline and posture behavior.
- [Architecture](../architecture/ensemble.md): g8ee's role in the platform.
- [Protocol Library](../architecture/protocol.md): canonical receipt parsing and verification.
- [Headless UX Smoke Test](../guides/ux_smoke_test.md): starting a governed local stack.
