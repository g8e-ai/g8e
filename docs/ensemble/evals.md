# Evals

## Overview

g8ee includes a standalone evidence-grade evaluation package at `ensemble/evals/`. It compares raw provider inference, ensemble orchestration, and the canonical doctrine, consensus, and notary experiment arms while preserving the configuration, model identity, stage telemetry, receipts, and immutable inputs needed to reproduce and audit a run. Ratify is a supported gateway posture but does not have a standalone arm in this five-arm experiment design.

The package is separate from the g8ee application package and has its own locked environment, CLI, type checking, linting, tests, and CI job. Python 3.12 or later is required.

## Experiment Arms

The CLI exposes five typed arms:

| Arm | Execution path | Requested posture | Receipt binding |
| --- | --- | --- | --- |
| `direct` | Provider API only | none | no |
| `ensemble_ungoverned` | Real g8ee chat pipeline without gateway governance | none | no |
| `doctrine` | g8ee, gateway, and operator | L1 enforced; L2/L3 audited | when the turn emits an `ActionReceipt` |
| `consensus` | g8ee, gateway, and operator | L1/L2 enforced; L3 audited | when the turn emits an `ActionReceipt` |
| `notary` | g8ee, gateway, and operator | L1/L2/L3 enforced | when the turn emits an `ActionReceipt` |

The posture-only comparisons are `consensus - doctrine` and `notary - consensus`. The ensemble orchestration comparison is `ensemble_ungoverned - direct`; a direct-versus-governed comparison combines orchestration and governance effects.

## Evidence Model

Each run writes an immutable manifest before execution. The manifest records the source revision and tree hash, benchmark and grader hashes, exact role-to-model mapping, sampling settings, stack environment, selected arm, requested posture, and redacted configuration. The runner independently observes the gateway's effective posture instead of treating the requested CLI argument as evidence.

Attempt records normalize the full execution into typed stages, including model inference, Tribunal generation and auditing, deterministic doctrine, protocol L2, L3 ceremony, L4 verification, L5 execution, scrubbing, receipt persistence, commitment append, and grading. Provider telemetry records monotonic timing, token usage, retries, finish reasons, hashes of model-boundary inputs and outputs, and privacy attestations containing only scanner identity, the input hash, sensitive-occurrence counts, and detected types. Usage reconciliation reports missing provider usage instead of inventing estimates.

Governed arms collect canonical `ActionReceipt` protobuf messages and verify the receipt signature and final persistence attestation with protocol-owned helpers. Receipt stage evidence carries transaction and identity bindings, state roots, doctrine metadata, signature digests, commitment hashes, audit record IDs, and parent-child relationships. Unknown, duplicated, cyclic, incomplete, or unverifiable evidence fails the attempt closed as invalid evidence.

The runner grades authoritative evidence with versioned deterministic graders. Receipt-integrity grading requires exactly one verified primary receipt matching the expected action class and its verified final-persistence stage. Protocol-chain grading verifies stage identity and transaction bindings, ordering, posture-specific L2/L3 statuses, parent relationships, execution status, and state roots. Policy-outcome grading compares the signed L4 allow/block result and rejection layer with typed task expectations. Receipt-bound final-state grading evaluates typed `state_root_changed` or `state_root_unchanged` assertions. Canary grading binds expected sensitive values by hash to scrubbing-stage input and output hashes, and model-boundary grading measures raw sensitive occurrences without retaining the values.

The harness also defines injected observer contracts for evidence collected outside the system under test. A configured state observer records typed file, document, workload-side-effect, or ledger-consistency values and binds them to fixture hashes and source evidence. Configured secret-detection and rehydration observers record confusion-matrix counts or exact token restoration at the local runtime boundary. The corresponding deterministic graders reject missing, duplicated, unbound, unsupported, or internally inconsistent observations. The standard `g8e-evals run` command does not construct these observers; integrations that use these task assertions supply them through `SUTConfig`.

Raw prompts, model outputs, and observer source material are restricted evidence. The runner requires an owner-only key file and writes captured prompt and model-output artifacts as AES-256-GCM envelopes with authenticated index metadata, plaintext and ciphertext hashes, byte lengths, key IDs, and named-key-holder access policy. Analytical records contain hashes, counts, types, and references rather than raw sensitive values.

## Benchmark Support

The current CLI supports the curated `ifeval_subset` suite. Its dataset has a provenance manifest, deterministic instruction verifiers, and content hashes included in each run manifest. An optional secondary LLM judge records its model calls and scores as separate grading stages rather than replacing deterministic grading.

## Running Evals

Create an owner-only evidence key file containing a random 32-byte key:

```json
{"version":1,"key_id":"eval-owner-1","key_b64":"<32-byte-base64>"}
```

Run the CLI from `ensemble/evals/` through the locked environment. Governed and ensemble arms load the canonical local CLI identity through `g8e auth context`; `--auth-project-root` identifies the project runtime containing that identity, and `--g8e-cli` identifies the repository binary. Refresh the CLI identity after Operator enrollment so its session is bound to the active Operator session. The command returns `operator_session_id`, `cli_session_id`, `user_id`, `operator_id`, `client_cert`, and `client_key`. The certificate and key values are filesystem paths, not credential contents. g8ee validates the session tuple with the Gateway before admitting the request context. Set both trust-bundle variables explicitly when the eval working directory does not contain the project runtime; transport fails closed if either trust bundle is unavailable.

```bash
REPO_ROOT="$PWD"
./g8e auth refresh
export G8E_APP_TRUST_BUNDLE="${REPO_ROOT}/.g8e/pki/trust/g8eg-ca-bundle.pem"
export G8E_GATEWAY_TRUST_BUNDLE="${REPO_ROOT}/.g8e/pki/trust/g8eg-ca-bundle.pem"
cd ensemble/evals
uv sync --locked --extra test
uv run g8e-evals run \
  --suite ifeval_subset \
  --arm doctrine \
  --g8ee-url http://localhost:8000 \
  --g8e-cli "${REPO_ROOT}/g8e" \
  --auth-project-root "${REPO_ROOT}" \
  --evidence-key-file /path/to/owner-only-evidence-key.json
```

Provider, model, endpoint, API-key, judge, output, task-limit, and headless approval options are available through `g8e-evals run --help`. Local providers and the deterministic fake provider do not require API keys.

### Stage 1 release diagnostic

The Stage 1 README profile is a bounded real-agent diagnostic, not a broad benchmark or governance proof. It runs task IDs `1001`, `1019`, `1051`, `1072`, and `1075` exactly once through the `doctrine` arm with no `--limit`, no judge, primary/assistant/lite roles configured to real providers, and a 180-second idle timeout. Every assigned task remains in the denominator whether it completes, fails, errors, or times out. A publishable run contains one terminal attempt and one bound deterministic `ifeval_subset_verifier` metric for each task plus real provider-boundary model telemetry for every attempt.

The public snapshot separates configured roles from observed calls. `manifest.json` declares all three role mappings, while `stages.jsonl` establishes observation only when provider-boundary telemetry matches the configured provider and model. A configured role without matching telemetry is `Configured but unobserved`; configuration alone never proves use.

Raw report directories remain private because `tasks.jsonl`, encrypted evidence metadata, local paths, and runtime configuration can expose information that is not eligible for public release. `scripts/project_readme_evidence.py` consumes one immutable completed report and creates a new checksum-bound candidate using an explicit allowlist. The candidate contains safe projections of `manifest.json`, `tasks.jsonl`, `attempts.jsonl`, `stages.jsonl`, `metrics.jsonl`, `summary.json`, `evidence-index.jsonl`, a portable `reproduction-manifest.json`, an empty `receipts.jsonl` for the answer-only profile, and `index.json`. It excludes raw prompts, raw outputs, encryption keys, encrypted payloads and their storage locations, credentials, exact provider endpoints, machine-specific paths, and private-network topology.

`summary.json` is a deterministic orientation view derived from typed attempts and metrics, not an independent source of truth. The safe evidence index publishes only attempt-bound artifact identity, media type, plaintext hash, byte length, and the fact that restricted evidence remains private. The reproduction manifest records the fixed suite, arm, task population, timeout, endpoint class, role-model mapping, environment class, CLI version, and a command expressed with replaceable endpoint and model variables. The offline README reader recomputes population, outcome, metric, role, and checksum consistency and rejects mismatches or undeclared artifacts.

The release-owner v2.1.5 profile uses Ollama with `gemma4:12b` for primary, `gemma4:e4b` for assistant, and `gemma4:e2b` for lite. Reproducing operators provide their own reachable `OLLAMA_ENDPOINT` and may substitute `PRIMARY_MODEL`, `ASSISTANT_MODEL`, and `LITE_MODEL`; they record substitutions and do not represent different tags or model revisions as byte-identical reproduction. The attended command sequence and candidate approval boundary are in [Release Process](../devs/release_process.md#stage-1-real-agent-readme-evidence).

A zero-receipt answer-only run is valid Stage 1 diagnostic evidence but provides no receipt, mutation, persistence, independently observed state, governance, or compliance evidence. Receipt and governance sections render unavailable rather than passed. Stages 2 through 4 add stronger provenance, governed actions, complete verification, statistical depth, and compliance analysis in later releases; they do not change the meaning of a published Stage 1 claim.

## Evidence Bundle Contract

The persisted record schema version is defined as `SCHEMA_VERSION` in `ensemble/evals/g8e_evals/schema.py` and is recorded in every run `manifest.json`; consumers must read the schema version from the manifest and reject unsupported versions rather than copying a prose value. Every typed record rejects unknown fields and carries stable run, task, attempt, receipt, state, rehydration, secret-detection, stage, metric, or evidence identifiers used to link the bundle without relying on file order. Task definitions use typed receipt-bound final-state assertions, independently observed state fixtures, canary annotations, rehydration assertions, secret-detection assertions, allow/block outcomes, and rejection layers. Validation rejects duplicate assertion IDs, malformed hashes, inconsistent counts and types, and inconsistent policy expectations. Readers must reject unsupported schema versions, missing references, duplicate identities, invalid parent-stage graphs, hash mismatches, and incomplete evidence rather than partially accepting a report.

A report directory contains:

- `manifest.json` — immutable run configuration and input hashes, written before execution begins.
- `tasks.jsonl` — immutable benchmark task definitions and provenance.
- `attempts.jsonl` — terminal attempt outcomes and record linkage.
- `receipts.jsonl` — typed receipt observations containing canonical `ActionReceipt` messages.
- `final-state-observations.jsonl` — receipt-bound state-root observations linked to typed task assertions.
- `state-observations.jsonl` — independently collected typed state observations linked to fixture assertions and source evidence.
- `rehydration-observations.jsonl` — local-runtime token restoration observations supplied by a configured observer.
- `secret-detection-observations.jsonl` — typed scanner confusion-matrix observations supplied by a configured observer.
- `stages.jsonl` — normalized model, governance, privacy, persistence, commitment, and grading stages.
- `metrics.jsonl` — typed measurements linked to attempts and evidence.
- `evidence-index.jsonl` — authenticated metadata, hashes, locations, classifications, and access policy for encrypted restricted evidence.
- Encrypted evidence files — AES-256-GCM envelopes containing captured raw prompts and model outputs.
- `results.jsonl` and `summary.json` — compatibility reporting views derived from the typed records.

The evidence key file is versioned JSON containing a key ID and exactly 32 bytes of base64-encoded key material. It must be a regular, non-symlink file with no group or other permission bits. The runner never embeds the key in the report. The `named_key_holders` policy means only principals explicitly given the corresponding key out of band can decrypt restricted evidence; deleting that external key makes the ciphertext unrecoverable, while retaining it requires the owner to protect and rotate it outside the bundle lifecycle.

Verify receipts in an existing report bundle with a directory containing every producing actuator's `*Actuator_pub.pem` file:

```bash
uv run g8e-evals verify-receipts <report-directory> --pki-dir <verifier-pki-directory>
```

`verify-receipts` derives a key ID from each discovered actuator public key, selects the key matching each receipt's `signer_key_id`, and verifies both the canonical receipt signature and final persistence attestation. It exits non-zero when no public keys are available, a receipt has no matching signer key, or a parsed receipt fails verification. A report with no bound receipts produces a zero-receipt result, not a receipt-verification pass; require a non-zero total, no missing keys, zero failures, and equal verified and total counts before making a receipt-verification claim. The command does not validate the manifest and dataset hashes, decrypt restricted evidence, reconcile all stage or metric links, verify the commitment ledger, or establish trust in the supplied public keys; complete offline report validation must perform those checks separately and fail closed on any missing or inconsistent record.

The public README evidence snapshot under `docs/evidence/readme/current/` contains only reviewed synthetic or hash-safe projections. `scripts/generate_readme.py` validates the snapshot checksums and shapes, aggregates eligible metric rows, projects receipt and demo verification results, and renders `README.md` from `docs/templates/README.md.tmpl`. It does not run live evals, start the platform, decrypt restricted evidence, or recompute cryptographic signatures; evidence refresh is a separate reviewed operation.

## Tests and Lint

From the repository root:

```bash
make evals-test
make evals-test-unit
make evals-test-integration
make evals-lint
```

Tier 1 tests have no filesystem, process, network, database, or provider dependencies. Tier 2 tests may use local files, subprocesses, or in-process dependencies. Live stack or provider evaluations are Tier 3 and are not part of the offline test targets.

## Related

- [Testing](tests.md) — g8ee and standalone eval test boundaries
- [Architecture](../architecture/ensemble.md) — g8ee's role in the platform
- [Protocol Library](../architecture/protocol.md) — canonical receipt parsing and verification
- [Headless UX Smoke Test](../guides/ux_smoke_test.md) — bringing up a governed local stack
