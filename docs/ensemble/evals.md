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
| `doctrine` | g8ee, gateway, and operator | L1 enforced; L2/L3 audited | yes |
| `consensus` | g8ee, gateway, and operator | L1/L2 enforced; L3 audited | yes |
| `notary` | g8ee, gateway, and operator | L1/L2/L3 enforced | yes |

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

Run the CLI from `ensemble/evals/` through the locked environment. Governed and ensemble arms load the canonical local CLI identity through `./g8e auth context`; `--auth-project-root` identifies the project runtime containing that identity. The command returns `operator_session_id`, `cli_session_id`, `user_id`, `operator_id`, `client_cert`, and `client_key`. The certificate and key values are filesystem paths, not credential contents. g8ee validates the session tuple with the Gateway before admitting the request context.

```bash
cd ensemble/evals
uv sync --locked --extra test
uv run g8e-evals run \
  --suite ifeval_subset \
  --arm doctrine \
  --g8ee-url http://localhost:8000 \
  --auth-project-root ../.. \
  --evidence-key-file /path/to/owner-only-evidence-key.json
```

Provider, model, endpoint, API-key, judge, output, task-limit, and headless approval options are available through `g8e-evals run --help`. Local providers and the deterministic fake provider do not require API keys.

## Evidence Bundle Contract

The persisted record schema version is `1.17.0`. Every typed record rejects unknown fields and carries stable run, task, attempt, receipt, state, rehydration, secret-detection, stage, metric, or evidence identifiers used to link the bundle without relying on file order. Task definitions use typed receipt-bound final-state assertions, independently observed state fixtures, canary annotations, rehydration assertions, secret-detection assertions, allow/block outcomes, and rejection layers. Validation rejects duplicate assertion IDs, malformed hashes, inconsistent counts and types, and inconsistent policy expectations. Readers must reject unsupported schema versions, missing references, duplicate identities, invalid parent-stage graphs, hash mismatches, and incomplete evidence rather than partially accepting a report.

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

Verify receipts in an existing report bundle with:

```bash
uv run g8e-evals verify-receipts <report-directory> --pki-dir <operator-pki-directory>
```

`verify-receipts` parses every `ReceiptObservation` in `receipts.jsonl` and verifies both the canonical receipt signature and final persistence attestation against the supplied Warden public key. It exits non-zero when a parsed receipt fails verification. It does not validate the manifest and dataset hashes, decrypt restricted evidence, reconcile all stage or metric links, verify the commitment ledger, or establish trust in the supplied public key; complete offline report validation must perform those checks separately and fail closed on any missing or inconsistent record.

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
