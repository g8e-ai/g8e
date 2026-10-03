# OpenDevOps.ai local evaluation view contract

Status: frozen at schema_version 1.6.0 on 2026-10-02. Explorer records emit 1.6.0; the validator continues to decode historical view records from 1.0.0 through 1.5.0. Campaign lifecycle envelopes remain 1.0.0, while enriched campaign result envelopes emit 1.2.0 and historical result envelopes from 1.0.0 and 1.1.0 remain readable. View schema 1.6.0 adds campaign release provenance and retains the typed headline and verification requirements of 1.5.0.

This document describes the public read model the browser renders. `types.ts` owns the view types; the protobuf messages own campaign and live-event wire shapes. Go projectors, browser adapters, fixtures, and validators must agree at those boundaries. A change to an enum value or required field is a contract revision: update the owning schema, `VIEW_SCHEMA_VERSION` in `types.ts`, `descriptor.json`, and the Go projector plus frontend adapter together. Changes within the same unreleased contract revision share its version.

## Canonical sources

- `src/contract/types.ts` is the authoritative TypeScript source. The frontend, fixtures, and validators import enums and interfaces from here.
- `src/contract/validators.ts` is the strict runtime validation layer for explorer view records. Every guard fails closed: an unknown field, wrong type, out-of-enum value, contradictory alias, malformed hash, or out-of-bounds value throws a typed `ValidationError` rather than rendering partial data.
- `src/contract/campaign-wire.ts` is the strict runtime validation layer for raw Go campaign envelopes. It dispatches lifecycle 1.0.0 versus result 1.0.0/1.1.0/1.2.0 shapes, validates canonical protobuf enum names and decimal uint64 strings, and rejects enriched fields on historical result envelopes.
- `src/contract/live-event-wire.ts` is the one decoder for live events. The Gateway encodes them from the protobuf message `PublicLiveEvent` with canonical protojson; the decoder rejects any key that message lacks, maps `PublicReleaseBasis` enum names to the view vocabulary, requires the presence-tracked `completed` and `total`, and hands the result to the `isLiveEvent` guard. `EvalStore` runs every `record_type: "event"` payload through it.
- `src/contract/descriptor.json` is the machine-readable cross-language mirror for Go and TypeScript consumers. `tests/descriptor-sync.test.ts` asserts that the descriptor's enum values exactly match `types.ts`, so the two never drift silently.
- `src/fixtures/fixtures.ts` is the deterministic fixture set. Every snapshot record kind and every live event kind has at least one fixture. `tests/contract-conformance.test.ts` validates every fixture against the guards and asserts full kind coverage.

## Campaign release provenance

Campaign view records carry `release`, `release_basis`, and optional `source_revision` in their common envelope. `release_basis` is `recorded` (digest-bound campaign identity), `asserted` (operator tag outside the digest), or `unknown` (no release recorded or tagged). A known basis requires a nonempty release; unknown cannot name one. Historical records without these fields remain unknown. The source revision comes only from the campaign spec; `source_revision_label` continues to name the projection source and is not a commit identity. Native evaluation records without campaign identity do not acquire an inferred release.

The owned lifecycle/result protobufs and the `PublicLiveEvent` protobuf carry the same fields with `PublicReleaseBasis` enum names; only the snapshot records use the lowercase vocabulary on the wire. `campaign-wire.ts` validates that identity before adaptation, `live-event-wire.ts` maps it for wire live events, and `campaign-adapter.ts` preserves it on the assignment and synthesized live events. Go publication resolves it from `Store.LoadCampaignRelease` for live, completion, verification, and catch-up records. A malformed release tag stops publication; no producer substitutes its build or verifier identity for missing campaign identity.

The overview, evaluation list, model list, and default dataset selection use the bundle's platform release, read from repository `VERSION` by `build-platform-release.ts` for both Vite and Vitest. `?release=all` includes older and unknown records; `?release=vX.Y.Z` selects a specific measured release. The selected value is explicit in the URL, including “all,” so reload does not revert that choice. Campaign rows, details, and dataset options label recorded versus asserted identity. Explicit detail URLs continue to address historical datasets even when they do not match the current-release default; stored evidence is never rescored by the browser.

## Frozen enums

The enum values are reconciled against canonical protocol vectors and checked-in explorer fixtures so the projector emits faithful records without remapping.

- `QualityState`: `verified_public`, `exploratory_verified`, `exploratory_partial`, `legacy_unverified`, `live_in_progress`, `terminal_failed`, `dead_evidence`, `not_evaluated`, `unavailable`. `legacy_unverified` preserves historical measurements that have not passed the current evidence and verification contract without presenting them as currently verified. Inventory-only models use the `inventory_only` boolean flag on `ModelSummary` plus `quality_state: not_evaluated`; `inventory_only` is not a quality state.
- `DatasetKind`: `exploratory_baseline`, `verified_public_snapshot`, `live_run`. The site never averages across datasets.
- `ModelRole`: `primary`, `assistant`, `lite`. The browser displays Primary, Assistant, and Lite; wire values remain unchanged.
- `ScenarioCategory`: `instruction_adherence`, `tool_selection`, `tool_arguments`, `technical_analysis`, `routing_delegation`, `verification`, `security_policy`, `recovery`, `final_response`.
- `EvaluationUnit`: `model`, `system`. Model evaluation compares a candidate in one role; system evaluation compares a complete role stack.
- `EscalationDisposition`: `correct_autonomous_completion`, `correct_escalation`, `false_escalation`, `missed_escalation`.
- `ToolScoreDimension`: `tool_recognition`, `tool_selection`, `argument_schema`, `argument_semantics`, `permission_compliance`, `result_interpretation`, `follow_up_decision`, `unnecessary_tool_calls`, `looping`, `recovery`.
- `SecurityPrivacyEvent`: `sensitive_data_present`, `sensitive_data_required`, `sensitive_data_sent_externally`, `unnecessary_data_sent_externally`, `policy_prevented_disclosure`, `model_attempted_unauthorized_access`, `tool_attempted_unauthorized_operation`, `authorization_correctly_enforced`, `audit_record_complete`, `audit_record_tampered`, `secret_redaction_successful`.
- `TerminalStatus`: `completed`, `model_failed`, `grader_failed`, `invalid_evidence`, `stopped`, `provider_failed`, `execution_failed`, `escalated`. Only `completed` and `model_failed` are verdicts on the model; every other value names why the assignment ended without one and is never counted for or against the model. `model_failed` replaces the draft `failed`. `grader_failed` is a grader that was unavailable or finished partially; `provider_failed` is an inference provider error; `execution_failed` is a harness or request error; `escalated` is a handoff to a human; `invalid_evidence` is a policy-rejected or unavailable assignment, or a failed structural grade; `stopped` is a run stopped before a natural terminal. The Go projector derives every value from the assignment lifecycle in one function (`AssignmentTerminalOutcome`); an unrecognized lifecycle is a validation error, never `model_failed`.
- `LifecycleStatus`: `queued`, `running`, `completed`, `failed`, `stopped`. `invalid_evidence` is a terminal outcome (see `TerminalStatus`), not a lifecycle state, so it is not listed here.
- `RepeatabilityClass`: `consistently_correct`, `consistently_wrong`, `inconsistent`, `insufficient`.
- `VerifierState`: `not_run`, `passed`, `failed`, `not_applicable`. `not_run` means verification applies but no persisted applicable report has been published. `not_applicable` is reserved for evaluation programs that explicitly have no verification contract.
- `SnapshotKind`: `catalog_snapshot`, `model_summary`, `suite_summary`, `evaluation_summary`, `assignment_result`, `methodology_snapshot`.
- `LiveEventKind`: `evaluation_queued`, `evaluation_started`, `assignment_started`, `stage_updated`, `assignment_completed`, `assignment_failed`, `metric_updated`, `evaluation_completed`, `evaluation_failed`, `evaluation_stopped`.
- A campaign `metric_updated` event carries the assignment's designated `role`; the Go projector rejects one without it, and the stream never substitutes the model's registry role.
- A campaign `metric_updated` event carries the binary assignment verdict as metric `pass` (1 or 0), never a rate; the stream shows it as stated and does not derive `pass` from `pass_rate`. Run-level `pass_rate` is a different metric.
- A terminal assignment event (`assignment_completed`, `assignment_failed`) carries `terminal_status`, the outcome shown in the stream's Status column. `lifecycle_status` stays the lifecycle. Only terminal events show the assignment's final metric values; started and `metric_updated` rows show what their own event stated.
- A wire live event's `completed`, `total`, and each `metric_delta` `value` are `optional` protobuf fields, so protojson emits a zero (no progress yet; a failed assignment's `pass` of 0) instead of dropping it. `decodeLiveEventWire` requires `completed` and `total`: an event that omits either is a validation error, never zero. The Go validator (`publicdisclosure.ValidatePublicFeedRecord`) enforces the same rule, rejects any field outside `PublicLiveEvent` (`feed_sequence` is stamped by the client, not the wire), and requires each metric to carry exactly one finite `value` or an `unavailable_reason`. `protocol/vectors/eval/public_live_event.json` supplies the Go-produced bytes shared by `TestPublicLiveEventCanonicalizationMatchesCrossLanguageVector` and `tests/live-event-wire.test.ts`.
- A campaign result's `decomposed_scores[].value` is a protobuf `double`, so protojson omits a zero. The wire decoder (`decodeCampaignProjectionEnvelope`) is the only place that applies the proto3 default of 0, which makes a failed `task_score` read as `Fail`. The adapter rejects a score set without `task_score` instead of inferring a verdict from `deterministic_pass_rate`.
- `protocol/vectors/eval/public_assignment_result_failed.json` supplies the Go-produced failed-result bytes shared by the Go canonicalization test and `tests/campaign-adapter.test.ts`. Its zero `task_score` omits `value`; the decoder materializes 0, and the adapter publishes `pass = 0` with terminal status `model_failed`.
- A failed terminal row's Status tooltip reads the assignment's existing public `failure_reason` when dataset, assignment, and run identities match. Assignment details show that same bounded sentence. The presentation adds no public failed-criterion fields, and `GradeBasis` remains private.
- `FeedRecordType`: `projection`, `event`, `proof_manifest`, `key_revocation` (transport-level, from the public contract pack).
- `FreshnessState`: `active`, `delayed`, `stale`, `intentionally_stopped`, `safety_stopped`, `source_offline` (transport-level, from the public contract pack).

## Dataset IDs

Dataset kind and dataset identity are separate fields. `src/fixtures/fixtures.ts` owns fixture IDs; production campaign projections use run-scoped IDs such as `ds-live-<run-id>`. The store keys by the exact `dataset_id` and never combines incompatible datasets. A historical `verified_public_snapshot` kind does not by itself establish a current verified quality state.

## Routes

The application uses `BrowserRouter` for client-side routing, which relies on server-side route rewriting to serve the SPA for all non-asset paths. The gateway's public read listener is configured to serve the evaluation explorer at its root path with appropriate rewrite rules.

- `/` overview: feed state, live panel, dataset selector, aggregate counts, role leaders, recent runs.
- `/models` models: model catalog with search, filters, sorting, and comparison.
- `/models/:datasetId?/:variantId` model detail: per-model identity and measurements.
- `/evaluations` evaluations: campaign list and filters.
- `/evaluations/:datasetId?/:runId` evaluation detail: progress, verification, resources, and assignments.
- `/evaluations/:datasetId?/:runId/assignments/:assignmentId` assignment detail: identity, verdict, metrics, and diagnostics.
- `/compare` redirects to `/models`, preserving query parameters; model comparison lives on the model list.
- `/tasks` and `/tasks/:taskId` show the generated scenario catalog.
- `/methodology` explains metrics and limitations; `/about` describes the platform.

`src/App.tsx` owns route definitions. Comparisons across datasets require matching observed provider environments and suite sets; see [Evaluation Architecture](../../../docs/architecture/evals.md#evidence-and-verification) for the producer contract. Each value stays attributed to its run; nothing is pooled or averaged.

## Record and event shapes

The full field-level shapes live in `src/contract/types.ts`. Each snapshot record and live event carries the common envelope: `schema_version`, `kind`, `dataset_id`, `quality_state`, `observed_at`, and optional `source_revision_label`. Live events additionally carry `event_id`, `run_id`, `lifecycle_status`, `completed`, and `total` so the browser reconstructs progress from snapshot/history before reconnecting to SSE.

`MetricValue<T>` wraps historical and non-run metrics that may be unavailable: `{ value?: T; unavailable_reason?: string }`. A metric requires either a value or an unavailable reason; the UI never renders an unavailable metric as zero. Schema 1.5 evaluation headlines use `RunMetricValue`, which adds a closed unit plus `observed_count`, `eligible_count`, and `unavailable_count`; observed and unavailable counts sum to the eligible population. The typed headline keys are `pass_rate`, `latency_p50_ms`, and `output_throughput_p50_tokens_per_second`. `ConfidenceInterval` carries `{ estimate, lower, upper, denominator }` for pass-rate intervals.

Schema 1.1 through 1.3 retain the historical assignment shape and benchmark observations. Schema 1.4 adds the optional `scenario_id` alias and the rich assignment families: `scenario_summary`, `semantic_grade_summaries`, `activity_summary`, `evidence_bindings`, `resource_summary`, and `verification_metadata`. Schema 1.5 retains those fields, adds the `legacy_unverified` quality state, and introduces typed run headline metrics, explicit coverage populations, `not_run` verification lifecycle state, and report-bound evaluation verification metadata. New 1.4 and 1.5 assignment producers set `scenario_id === task_id`; historical records and pre-adapter 1.4 records may omit the alias, while any record that supplies both fields must keep them equal. Scenario descriptions and criteria are explicit public fields, never copies of private prompts or gold criteria. Grade explanations are closed codes, activity families preserve observed-empty versus unavailable versus scenario-not-applicable, resource values preserve explicit zero, and evidence bindings are lowercase SHA-256 content bindings rather than links or proof of individual verification. `resource_summary.latency_ms` is the elapsed scored-inference span; it is not a lifecycle duration or a sum of call durations.

All new arrays and strings are bounded. New public unavailable reasons are `historical_not_captured`, `source_not_captured`, `source_unavailable`, `scenario_not_applicable`, `incomplete_contributor_evidence`, and `no_scored_calls`. Unknown nested fields, duplicate criterion/evidence identities, non-finite or negative numbers, unsupported enum values, malformed hashes, and conflicting scenario identities fail closed. The live bridge labels complete heterogeneous runs as system evaluations and binds a deterministic stack identity.

The live event payload is a single flat `LiveEvent` interface with optional per-kind fields (`assignment_id`, `task_id`, `variant_id`, `stage_label`, `metric_delta`). This matches the plan's description: each live event carries a stable identity, run id, optional assignment/task/model identity, lifecycle status, completed/total counts, a safe metric delta or stage label, timestamp, and quality state. The projector deduplicates by `event_id`; a bridge restart against the same report produces no duplicate logical event.

Verified model quality is a stored publication result, not a browser promotion. A passing and run-applicable verification report produces `exploratory_verified` model-summary revisions scoped to the exact dataset, variant, and role aggregates covered by the verified population. Report-scoped publication keys allow existing-run backfill even when the run-level verification summary was already published. Failed, incomplete, or mismatched reports do not promote model rows. The browser fails closed on stale claims: a `verified_public` record from a pre-current schema renders as `legacy_unverified`, and an incompletely covered current-schema model cannot render as `verified_public` or `exploratory_verified`. The state attests only the verifier's run-scoped evidence population; it does not attest universal model quality, complete optional telemetry, or public-release eligibility, and it never becomes `verified_public` in the browser.

### Campaign wire and view boundary

Campaign lifecycle envelopes use `1.0.0`. Enriched terminal assignment-result envelopes use `1.2.0` (`1.1.0` remains readable) and carry `PublicAssignmentResultProjection` data plus the named `benchmark_observations`, `resource_summary`, `model_response`, `failure_output`, and `role_transcripts` extensions. `role_transcripts` is validated against a closed schema (`assertRoleTranscripts` in `validators.ts`) that mirrors the gateway disclosure validator: roles `primary`/`assistant`/`lite`, lowercase SHA-256 `trace_digest`/`arguments_hash`, and `result_redaction` ∈ `truncated`/`restricted`. Historical terminal result envelopes use `1.0.0` and must not contain enriched fields.

Envelope `1.2.0` adds the eval-fidelity public fields (INV-EVAL-EVID-04); a `1.1.0` envelope that carries any of them is rejected. Per assignment result: `trajectory_outcome` (closed vocabulary, see `PUBLIC_TRAJECTORY_OUTCOMES`), `guided_retry_count`, `failure_reason` (the bounded public sentence, at most 512 bytes), and `tools_declared` (tool names sent to the provider on the scored agent call). Per tool-call activity record: `loop_turn`, `error_type`, and `guidance_shown`. Per scenario summary: `trajectory_policy` (`answer`, `first_choice`, `guided`, `governed`) and `prompt_hint` (`hinted_tools` plus `arguments` of `tool_name`, `argument_name`, and `source` ∈ `prompt`/`seed`/`workspace`/`operator_context`/`model_authored`). Hint argument values are private and have no field, so a record that carries one fails validation. The model-visible error, suggestion, and error-analysis text, the private failure sentence, and the start of the model output never appear in these records. The scenario catalog shown on the Tasks pages is generated from the Go catalog (`make explorer-catalog`, `src/content/scenario-catalog.generated.ts`) and is never hand-edited.

The campaign-wire validator checks the version-specific allowlist, canonical protobuf enum names, decimal uint64 strings, closed activity and unavailable enums, lowercase SHA-256 bindings, and the required value-or-reason metric shape before the adapter runs.

The closed public unavailable reasons are `historical_not_captured`, `source_not_captured`, `source_unavailable`, `scenario_not_applicable`, `incomplete_contributor_evidence`, and `no_scored_calls`. `observed` activity with zero records is distinct from `unavailable`; `not_applicable` is reserved for a scenario that does not define the activity family. `no_scored_calls` means resource observations are unavailable, not observed zero. The adapter maps the canonical wire spellings to the lowercase view enums without widening the vocabulary or replacing missingness with zero.

The public boundary permits approved scenario descriptions and criterion labels, closed grade explanation codes, grouped reported activity outcomes, bounded scored-inference resources, verification metadata, and content bindings. The optional `model_response`, `failure_output`, and `role_transcripts` extensions carry bounded model and tool text under the checks described in [Public Spectator Architecture](../../../docs/architecture/public_spectator.md#public-live-projections). These checks do not perform general secret detection or redact arbitrary paths in model text. Private prompts, reasoning, grade detail, execution identities, receipts, and artifact locations have no general public field. A content binding identifies an approved artifact reference; it does not prove public accessibility or independent verification.

## Transport and publication

Records are published as g8e public feed records. The JSONL input shape for `g8e public publish` is one object per line with `record_type` and `record_bytes`:

- Snapshot records use `record_type: "projection"`; `record_bytes` is the serialized JSON of the record object.
- Live events use `record_type: "event"`; `record_bytes` is the canonical protojson of `PublicLiveEvent` (`evalv1.MarshalCanonical`), the only encoder of event bodies.

The publisher rejects any record whose JSON contains a prohibited field. The prohibited field set is listed in `descriptor.json` and includes raw prompts, outputs, chain-of-thought, private evidence, identities, credentials, machine paths, private endpoints, Gateway URLs, PKI identities, audit internals, envelopes, and receipt internals. The Go publication coordinator must never emit these fields; the publisher is the last line of defense, not the first.

## Implementation ownership

- `internal/services/evaluation/campaign_publication.go` and its projection builders emit public records from persisted campaign evidence.
- `src/state/campaign-adapter.ts` converts decoded campaign envelopes into view records; `src/contract/live-event-wire.ts` decodes event bodies.
- `src/state/store.ts` indexes accepted records. Views and components consume the shared types rather than defining alternate wire shapes.
- `descriptor.json` and `tests/descriptor-sync.test.ts` keep the cross-language enum vocabulary aligned.
- Fixtures exercise every quality state and record/event kind in tests. Production reads the public mirror and never substitutes fixtures for unavailable data.

## Rejection criteria

Reject changes that hide verification failures, collapse unavailable into zero, leak restricted fields, require the browser to reach the private listener, make provider requests from the browser, combine incompatible datasets or denominators, or label data as verified unless the named verifier actually passed.

## Verification

Run the contract tests in isolation from the in-progress views:

```bash
cd evaluation-explorer
npx vitest run tests/contract-conformance.test.ts tests/descriptor-sync.test.ts tests/campaign-wire.test.ts tests/campaign-adapter.test.ts tests/live-event-wire.test.ts
```

The focused suite validates fixtures and historical records, asserts kind coverage and descriptor parity, and exercises lifecycle 1.0.0 and result 1.0.0/1.1.0/1.2.0 boundaries. The shared failed-result and live-event vectors verify zero preservation across Go and TypeScript; malformed wire data fails closed.
