# OpenDevOps.ai local evaluation view contract

Status: frozen at schema_version 1.4.0 on 2026-09-20. Explorer records emit 1.4.0; the validator continues to decode historical view records from 1.0.0 through 1.3.0. Campaign lifecycle envelopes remain 1.0.0, while enriched campaign result envelopes emit 1.1.0 and historical result envelopes from 1.0.0 remain readable.

This is the single typed source of truth for the public-safe read model the browser renders. The Go evaluation publication service, the frontend `campaign-adapter`, the mock replay producer, the deterministic fixtures, and the frontend validators all consume this contract. No consumer hand-defines enums or record shapes. A change to any enum value or required field is a contract revision: bump `VIEW_SCHEMA_VERSION` in `types.ts`, update `descriptor.json`, and update the Go projector plus frontend adapter together.

## Canonical sources

- `src/contract/types.ts` is the authoritative TypeScript source. The frontend, fixtures, and validators import enums and interfaces from here.
- `src/contract/validators.ts` is the strict runtime validation layer for explorer view records. Every guard fails closed: an unknown field, wrong type, out-of-enum value, contradictory alias, malformed hash, or out-of-bounds value throws a typed `ValidationError` rather than rendering partial data.
- `src/contract/campaign-wire.ts` is the strict runtime validation layer for raw Go campaign envelopes. It dispatches lifecycle 1.0.0 versus result 1.0.0/1.1.0 shapes, validates canonical protobuf enum names and decimal uint64 strings, and rejects enriched fields on historical result envelopes.
- `src/contract/descriptor.json` is the machine-readable cross-language mirror for Go and TypeScript consumers. `tests/descriptor-sync.test.ts` asserts that the descriptor's enum values exactly match `types.ts`, so the two never drift silently.
- `src/fixtures/fixtures.ts` is the deterministic fixture set. Every snapshot record kind and every live event kind has at least one fixture. `tests/contract-conformance.test.ts` validates every fixture against the guards and asserts full kind coverage.

## Frozen enums

The enum values are reconciled against canonical protocol vectors and checked-in explorer fixtures so the projector emits faithful records without remapping.

- `QualityState`: `verified_public`, `exploratory_verified`, `exploratory_partial`, `live_in_progress`, `terminal_failed`, `dead_evidence`, `not_evaluated`, `unavailable`. Matches the plan's data quality model table exactly. Inventory-only models use the `inventory_only` boolean flag on `ModelSummary` plus `quality_state: not_evaluated`; `inventory_only` is not a quality state.
- `DatasetKind`: `exploratory_baseline`, `verified_public_snapshot`, `live_run`. The site never averages across datasets.
- `ModelRole`: `primary`, `assistant`, `lite`. The browser displays Primary, Assistant, and Lite; wire values remain unchanged.
- `ScenarioCategory`: `instruction_adherence`, `tool_selection`, `tool_arguments`, `technical_analysis`, `routing_delegation`, `verification`, `security_policy`, `recovery`, `final_response`.
- `EvaluationUnit`: `model`, `system`. Model evaluation compares a candidate in one role; system evaluation compares a complete role stack.
- `EscalationDisposition`: `correct_autonomous_completion`, `correct_escalation`, `false_escalation`, `missed_escalation`.
- `ToolScoreDimension`: `tool_recognition`, `tool_selection`, `argument_schema`, `argument_semantics`, `permission_compliance`, `result_interpretation`, `follow_up_decision`, `unnecessary_tool_calls`, `looping`, `recovery`.
- `SecurityPrivacyEvent`: `sensitive_data_present`, `sensitive_data_required`, `sensitive_data_sent_externally`, `unnecessary_data_sent_externally`, `policy_prevented_disclosure`, `model_attempted_unauthorized_access`, `tool_attempted_unauthorized_operation`, `authorization_correctly_enforced`, `audit_record_complete`, `audit_record_tampered`, `secret_redaction_successful`.
- `TerminalStatus`: `completed`, `model_failed`, `grader_failed`, `invalid_evidence`, `stopped`. Reconciled: the exploratory source data emits `completed` and `model_failed` in `terminal_statuses`, so `model_failed` replaces the draft `failed`. `grader_failed` and `invalid_evidence` cover the remaining typed failure modes the projector must represent; `stopped` covers a run stopped before a natural terminal.
- `LifecycleStatus`: `queued`, `running`, `completed`, `failed`, `stopped`. `invalid_evidence` is a terminal outcome (see `TerminalStatus`), not a lifecycle state, so it is not listed here.
- `RepeatabilityClass`: `consistently_correct`, `consistently_wrong`, `inconsistent`, `insufficient`.
- `VerifierState`: `passed`, `failed`, `not_applicable`. Reconciled: `not_applicable` replaces the draft `not_run` and `unavailable` and covers `ensemble_ungoverned` runs where receipt coverage does not apply.
- `SnapshotKind`: `catalog_snapshot`, `model_summary`, `suite_summary`, `evaluation_summary`, `assignment_result`, `methodology_snapshot`.
- `LiveEventKind`: `evaluation_queued`, `evaluation_started`, `assignment_started`, `stage_updated`, `assignment_completed`, `assignment_failed`, `metric_updated`, `evaluation_completed`, `evaluation_failed`, `evaluation_stopped`.
- `FeedRecordType`: `projection`, `event`, `proof_manifest`, `key_revocation` (transport-level, from the public contract pack).
- `FreshnessState`: `active`, `delayed`, `stale`, `intentionally_stopped`, `safety_stopped`, `source_offline` (transport-level, from the public contract pack).

## Dataset IDs

- `exploratory_baseline` maps to dataset id `exploratory-baseline-r2` with quality state `exploratory_partial`.
- `verified_public_snapshot` maps to dataset id `verified-public-snapshot-current` with quality state `verified_public`.
- `live_run` maps to dataset id `live-run` with quality state `live_in_progress`. A real live run uses a fresh dataset id per run; `live-run` is the fixture and replay default.

The fixtures currently use `ds-exploratory-baseline-20260914-r2`, `ds-verified-public-20260914`, and `ds-live-demo-20260914` as concrete dataset id strings. Production campaign projections use run-scoped dataset IDs such as `ds-live-<run-id>`; the store keys by the exact `dataset_id` string and never combines incompatible datasets. Fixture IDs and producer IDs must agree within their respective replay or live source.

## Routes

The application uses a hash router so local static serving and the gateway-owned public origin require no server-side route rewriting.

- `#/` overview: feed state, live panel, dataset selector, aggregate counts, role leaders, recent runs.
- `#/models` models: model catalog with search, filters, sorting, and comparison.
- `#/models/:variantId` model-detail: per-model identity, metrics, suite results, repeatability, performance, tokens, outcomes, source runs.
- `#/evaluations` evaluations: evaluation list with dataset, suite, status, quality, model, role, and date filters.
- `#/evaluations/:runId` evaluation-detail: run status, progress, quality, model-role map, metrics, verification, resources, assignment table.
- `#/evaluations/:runId/assignments/:assignmentId` assignment-detail: assignment identity, status, metric values, missingness, stage and resource summaries.
- `#/compare` compare: two to four model comparison within one dataset and compatible metrics.
- `#/methodology` methodology: dataset definitions, metric direction and denominator, uncertainty, quality states, and limitations.

## Record and event shapes

The full field-level shapes live in `src/contract/types.ts`. Each snapshot record and live event carries the common envelope: `schema_version`, `kind`, `dataset_id`, `quality_state`, `observed_at`, and optional `source_revision_label`. Live events additionally carry `event_id`, `run_id`, `lifecycle_status`, `completed_count`, and `total_count` so the browser reconstructs progress from snapshot/history before reconnecting to SSE.

`MetricValue<T>` wraps any metric that may be unavailable: `{ value?: T; unavailable_reason?: string }`. A metric requires either a value or an unavailable reason; the UI never renders an unavailable metric as zero. `ConfidenceInterval` carries `{ estimate, lower, upper, denominator }` for pass-rate intervals.

Schema 1.1 through 1.3 retain the historical assignment shape and benchmark observations. Schema 1.4 adds the optional `scenario_id` alias and the rich assignment families: `scenario_summary`, `semantic_grade_summaries`, `activity_summary`, `evidence_bindings`, `resource_summary`, and `verification_metadata`. New 1.4 producers set `scenario_id === task_id`; historical records and pre-adapter 1.4 records may omit the alias, while any record that supplies both fields must keep them equal. Scenario descriptions and criteria are explicit public fields, never copies of private prompts or gold criteria. Grade explanations are closed codes, activity families preserve observed-empty versus unavailable versus scenario-not-applicable, resource values preserve explicit zero, and evidence bindings are lowercase SHA-256 content bindings rather than links or proof of individual verification. `resource_summary.latency_ms` is the elapsed scored-inference span; it is not a lifecycle duration or a sum of call durations.

All new arrays and strings are bounded. New public unavailable reasons are `historical_not_captured`, `source_not_captured`, `source_unavailable`, `scenario_not_applicable`, `incomplete_contributor_evidence`, and `no_scored_calls`. Unknown nested fields, duplicate criterion/evidence identities, non-finite or negative numbers, unsupported enum values, malformed hashes, and conflicting scenario identities fail closed. The live bridge labels complete heterogeneous runs as system evaluations and binds a deterministic stack identity.

The live event payload is a single flat `LiveEvent` interface with optional per-kind fields (`assignment_id`, `task_id`, `variant_id`, `stage_label`, `metric_delta`). This matches the plan's description: each live event carries a stable identity, run id, optional assignment/task/model identity, lifecycle status, completed/total counts, a safe metric delta or stage label, timestamp, and quality state. The projector deduplicates by `event_id`; a bridge restart against the same report produces no duplicate logical event.

Verified model quality is a stored publication result, not a browser inference. A passing and run-applicable verification report produces `exploratory_verified` model-summary revisions scoped to the exact dataset, variant, and role aggregates covered by the verified population. Report-scoped publication keys allow existing-run backfill even when the run-level verification summary was already published. Failed, incomplete, or mismatched reports do not promote model rows.

## Transport and publication

Records are published as g8e public feed records. The JSONL input shape for `g8e public publish` is one object per line with `record_type` and `record_bytes`:

- Snapshot records use `record_type: "projection"`; `record_bytes` is the serialized JSON of the record object.
- Live events use `record_type: "event"`; `record_bytes` is the serialized JSON of the event object.

The publisher rejects any record whose JSON contains a prohibited field. The prohibited field set is listed in `descriptor.json` and includes raw prompts, outputs, chain-of-thought, private evidence, identities, credentials, machine paths, private endpoints, Gateway URLs, PKI identities, audit internals, envelopes, and receipt internals. The Go publication coordinator must never emit these fields; the publisher is the last line of defense, not the first.

## File ownership

Workers do not edit the same files concurrently. Worker 0 assigns concrete file ownership here. Cross-worker changes are proposed through a fixture or contract change and integrated by Worker 0.

- Worker 1B (Explorer contracts): `src/contract/types.ts`, `src/contract/validators.ts`, `src/contract/campaign-wire.ts`, `src/contract/descriptor.json`, `src/contract/CONTRACT.md`, `src/fixtures/fixtures.ts`, `tests/validators.test.ts`, `tests/contract-conformance.test.ts`, `tests/descriptor-sync.test.ts`, and `tests/campaign-wire.test.ts`.
- Go evaluation publication (`internal/services/evaluation/campaign_publication.go` and related projection builders): emits public-safe envelopes from canonical campaign state.
- Worker 2 (Mirror and local runtime): `dev.mjs`, `scripts/seed.mjs`, `scripts/replay.mjs`, local mirror startup, and the runtime fixture. Worker 2 does not edit contract or fixture files.
- Worker 3 (UX shell and navigation): `src/App.tsx`, `src/main.tsx`, `src/state/*`, `src/components/*`, `src/utils/*`, global styles, and the route shell. Worker 3 imports enums and types from `src/contract/types.ts` and never redefines them.
- Worker 4 (Metrics and detail views): `src/views/*` (model, evaluation, assignment, comparison, methodology detail views). Worker 4 imports shared components from `src/components/*` and types from `src/contract/types.ts`.
- Frontend campaign adapter (`src/state/campaign-adapter.ts`): decodes Go publication envelopes from the mirror into frozen view records.
- Worker 6 (Runtime and live eval): runtime recovery and the bounded real eval. Worker 6 does not edit frontend or contract files.
- Worker 7 (UX QA and browser acceptance): `tests/*` (excluding the contract conformance and descriptor sync tests owned by Worker 0), `e2e/*`, and the accessibility harness. Worker 7 extends `tests/setup.ts` only through Worker 0.
- Worker 8 (Junior runbook and handoff): the runbook document and handoff captures. Worker 8 does not edit source files.

## Integration decisions

1. The contract is frozen before Worker 1 builds the projector and before Workers 4 and 5 implement against it. The enum reconciliation (especially `model_failed` and `not_applicable`) is applied now so the projector emits faithful records without remapping.
2. The descriptor.json plus sync test replaces a TS-to-JSON generator. A generator that parses TypeScript source text is fragile; a sync test that asserts equality is robust and runs in milliseconds. Worker 1 and Worker 5 generate Python enums from descriptor.json.
3. The fixtures are test data that exercises every quality state and every record/event kind. Tests inject them into the typed store; production code does not import or load them.
4. The read-only mirror is authoritative for browser-visible state. The browser never imports local generated JSON, substitutes fixture data, or falls back to another endpoint in production.
5. Integration order: static historical snapshot, model pages, evaluation pages, assignment pages, live fixture events, then real eval events.

## Rejection criteria

Worker 0 rejects any integration that hides verification failures, collapses unavailable into zero, leaks restricted fields, requires the browser to reach the private listener, makes a provider request from the browser, combines incompatible datasets or denominators, or labels exploratory, partial, failed-verification, synthetic, or live-in-progress data as verified unless the named verifier actually passed.

## Verification

Run the contract tests in isolation from the in-progress views:

```bash
cd /home/bob/g8e/dashboard/g8e-adapter/evaluation-explorer
npx vitest run tests/contract-conformance.test.ts tests/descriptor-sync.test.ts
```

The focused contract suite validates every fixture and generated historical record against the guards, asserts full kind coverage, and checks that every descriptor enum matches `types.ts`. Campaign wire tests separately cover lifecycle 1.0.0, historical result 1.0.0, enriched result 1.1.0, canonical enum conversion inputs, and fail-closed malformed nested data.
