// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Frozen local public view contract for the OpenDevOps.ai evaluation explorer.
//
// This is the single typed source shared by fixture generation, projector
// output, replay events, bridge output, and frontend validators. Worker 0
// (Integration lead) owns this module; no consumer hand-defines enums or
// record shapes. Enum values are reconciled against canonical protocol vectors
// and checked-in explorer fixtures. See src/contract/CONTRACT.md for the
// ownership map and integration decisions.
//
// FROZEN at schema_version 1.5.0 on 2026-09-22. A change to any enum value
// or required field is a contract revision: bump VIEW_SCHEMA_VERSION and
// update descriptor.json and validators.

export const VIEW_SCHEMA_VERSION = '1.5.0' as const;
export const SUPPORTED_VIEW_SCHEMA_VERSIONS = ['1.0.0', '1.1.0', '1.2.0', '1.3.0', '1.4.0', '1.5.0'] as const;
export type ViewSchemaVersion = (typeof SUPPORTED_VIEW_SCHEMA_VERSIONS)[number];

export const ACTIVITY_AVAILABILITIES = ['observed', 'unavailable', 'not_applicable'] as const;
export type ActivityAvailability = (typeof ACTIVITY_AVAILABILITIES)[number];

export const PUBLIC_UNAVAILABLE_REASONS = [
  'historical_not_captured',
  'source_not_captured',
  'source_unavailable',
  'scenario_not_applicable',
  'incomplete_contributor_evidence',
  'no_scored_calls',
] as const;
export type PublicUnavailableReason = (typeof PUBLIC_UNAVAILABLE_REASONS)[number];

export const PUBLIC_GRADE_EXPLANATION_CODES = [
  'criterion_passed',
  'criterion_failed',
  'evidence_unavailable',
  'unsupported',
  'invalid_evidence',
  'grader_unavailable',
] as const;
export type PublicGradeExplanationCode = (typeof PUBLIC_GRADE_EXPLANATION_CODES)[number];

export const PUBLIC_FINISH_STATES = ['stop', 'length', 'tool_call', 'error', 'unavailable'] as const;
export type PublicFinishState = (typeof PUBLIC_FINISH_STATES)[number];

export const PUBLIC_LOAD_STATES = ['cold', 'warm', 'unavailable'] as const;
export type PublicLoadState = (typeof PUBLIC_LOAD_STATES)[number];

export const PUBLIC_EVIDENCE_KINDS = [
  'campaign_profile',
  'model_registry',
  'verification_report',
  'evaluation_projection',
  'comparison_row',
  'efficiency_observation',
  'statistical_analysis',
  'source_manifest',
] as const;
export type PublicEvidenceKind = (typeof PUBLIC_EVIDENCE_KINDS)[number];

export const PUBLIC_ACTIVITY_EVIDENCE_SOURCES = ['application_reported', 'bound_public_proof'] as const;
export type PublicActivityEvidenceSource = (typeof PUBLIC_ACTIVITY_EVIDENCE_SOURCES)[number];

export const PUBLIC_USAGE_AVAILABILITIES = ['reported', 'unavailable'] as const;
export type PublicUsageAvailability = (typeof PUBLIC_USAGE_AVAILABILITIES)[number];

export const PUBLIC_TOOL_OUTCOMES = ['allow', 'deny', 'refused'] as const;
export type PublicToolOutcome = (typeof PUBLIC_TOOL_OUTCOMES)[number];

export const PUBLIC_TOOL_EXECUTION_OUTCOMES = ['pass', 'fail', 'unavailable', 'unsupported', 'invalid_evidence'] as const;
export type PublicToolExecutionOutcome = (typeof PUBLIC_TOOL_EXECUTION_OUTCOMES)[number];

export const PUBLIC_SEMANTIC_OUTCOMES = ['pass', 'fail', 'unavailable', 'unsupported', 'invalid_evidence'] as const;
export type PublicSemanticOutcome = (typeof PUBLIC_SEMANTIC_OUTCOMES)[number];

export const PUBLIC_REPORTED_POLICY_OUTCOMES = ['allow', 'deny', 'refused'] as const;
export type PublicReportedPolicyOutcome = (typeof PUBLIC_REPORTED_POLICY_OUTCOMES)[number];

export const PUBLIC_RECEIPT_STATUSES = ['unavailable', 'reported'] as const;
export type PublicReceiptStatus = (typeof PUBLIC_RECEIPT_STATUSES)[number];

/** Quality state for every dataset, run, metric, and task shown in the site. */
export const QUALITY_STATES = [
  'verified_public',
  'exploratory_verified',
  'exploratory_partial',
  'legacy_unverified',
  'live_in_progress',
  'terminal_failed',
  'dead_evidence',
  'not_evaluated',
  'unavailable',
] as const;
export type QualityState = (typeof QUALITY_STATES)[number];

/** Dataset selector values. The site never averages across datasets. */
export const DATASET_KINDS = [
  'exploratory_baseline',
  'verified_public_snapshot',
  'live_run',
] as const;
export type DatasetKind = (typeof DATASET_KINDS)[number];

/** Model role buckets used across the exploratory baseline. */
export const MODEL_ROLES = ['primary', 'assistant', 'lite'] as const;
export type ModelRole = (typeof MODEL_ROLES)[number];

export const SCENARIO_CATEGORIES = [
  'instruction_adherence',
  'tool_selection',
  'tool_arguments',
  'technical_analysis',
  'routing_delegation',
  'verification',
  'security_policy',
  'recovery',
  'final_response',
] as const;
export type ScenarioCategory = (typeof SCENARIO_CATEGORIES)[number];

export const EVALUATION_UNITS = ['model', 'system'] as const;
export type EvaluationUnit = (typeof EVALUATION_UNITS)[number];

/** Provenance of a provider-host environment description. `declared` means
 *  the owner asserted the labels; `observed` means the producer measured them
 *  on the host. */
export const ENVIRONMENT_SOURCES = ['declared', 'observed'] as const;
export type EnvironmentSource = (typeof ENVIRONMENT_SOURCES)[number];

export const ESCALATION_DISPOSITIONS = [
  'correct_autonomous_completion',
  'correct_escalation',
  'false_escalation',
  'missed_escalation',
] as const;
export type EscalationDisposition = (typeof ESCALATION_DISPOSITIONS)[number];

export const TOOL_SCORE_DIMENSIONS = [
  'tool_recognition',
  'tool_selection',
  'argument_schema',
  'argument_semantics',
  'permission_compliance',
  'result_interpretation',
  'follow_up_decision',
  'unnecessary_tool_calls',
  'looping',
  'recovery',
] as const;
export type ToolScoreDimension = (typeof TOOL_SCORE_DIMENSIONS)[number];

export const SECURITY_PRIVACY_EVENTS = [
  'sensitive_data_present',
  'sensitive_data_required',
  'sensitive_data_sent_externally',
  'unnecessary_data_sent_externally',
  'policy_prevented_disclosure',
  'model_attempted_unauthorized_access',
  'tool_attempted_unauthorized_operation',
  'authorization_correctly_enforced',
  'audit_record_complete',
  'audit_record_tampered',
  'secret_redaction_successful',
] as const;
export type SecurityPrivacyEvent = (typeof SECURITY_PRIVACY_EVENTS)[number];

/** Terminal outcome classification for an assignment. Reconciled against the
 *  exploratory baseline source data, which emits `completed` and
 *  `model_failed` in `terminal_statuses`. `grader_failed` and
 *  `invalid_evidence` cover the remaining typed failure modes the projector
 *  must represent; `stopped` covers a run stopped before a natural terminal. */
export const TERMINAL_STATUSES = [
  'completed',
  'model_failed',
  'grader_failed',
  'invalid_evidence',
  'stopped',
] as const;
export type TerminalStatus = (typeof TERMINAL_STATUSES)[number];

/** Run or assignment lifecycle state. `invalid_evidence` is a terminal outcome
 *  (see TerminalStatus), not a lifecycle state, so it is not listed here. */
export const LIFECYCLE_STATUSES = [
  'queued',
  'running',
  'completed',
  'failed',
  'stopped',
] as const;
export type LifecycleStatus = (typeof LIFECYCLE_STATUSES)[number];

/** Repeatability composition classes for a model across repetitions. */
export const REPEATABILITY_CLASSES = [
  'consistently_correct',
  'consistently_wrong',
  'inconsistent',
  'insufficient',
] as const;
export type RepeatabilityClass = (typeof REPEATABILITY_CLASSES)[number];

/** Canonical verifier disposition for a suite or run. `not_applicable`
 *  covers ensemble_ungoverned runs where receipt coverage does not apply. */
export const VERIFIER_STATES = [
  'not_run',
  'passed',
  'failed',
  'not_applicable',
] as const;
export type VerifierState = (typeof VERIFIER_STATES)[number];

export const RUN_METRIC_UNITS = ['ratio', 'milliseconds', 'tokens_per_second'] as const;
export type RunMetricUnit = (typeof RUN_METRIC_UNITS)[number];

export const NATIVE_POSTURES = ['doctrine', 'consensus', 'ratify', 'notary'] as const;
export type NativePosture = (typeof NATIVE_POSTURES)[number];

export const NATIVE_LANES = ['platform'] as const;
export type NativeLane = (typeof NATIVE_LANES)[number];

export const NATIVE_RESULT_STATUSES = ['pass', 'fail', 'unavailable', 'unsupported', 'invalid_evidence'] as const;
export type NativeResultStatus = (typeof NATIVE_RESULT_STATUSES)[number];

export const NATIVE_SCENARIO_STATUSES = ['completed', 'rejected', 'failed', 'unavailable', 'unsupported', 'invalid_evidence'] as const;
export type NativeScenarioStatus = (typeof NATIVE_SCENARIO_STATUSES)[number];

export const NATIVE_METRIC_UNITS = ['count', 'ratio'] as const;
export type NativeMetricUnit = (typeof NATIVE_METRIC_UNITS)[number];

/** Snapshot record kinds (durable view records). */
export const SNAPSHOT_KINDS = [
  'catalog_snapshot',
  'model_summary',
  'suite_summary',
  'evaluation_summary',
  'assignment_result',
  'methodology_snapshot',
] as const;
export type SnapshotKind = (typeof SNAPSHOT_KINDS)[number];

/** Live event kinds (incremental lifecycle events). */
export const LIVE_EVENT_KINDS = [
  'evaluation_queued',
  'evaluation_started',
  'assignment_started',
  'stage_updated',
  'assignment_completed',
  'assignment_failed',
  'metric_updated',
  'evaluation_completed',
  'evaluation_failed',
  'evaluation_stopped',
] as const;
export type LiveEventKind = (typeof LIVE_EVENT_KINDS)[number];

/** Public feed record types carried by the mirror transport. */
export const FEED_RECORD_TYPES = [
  'projection',
  'event',
  'proof_manifest',
  'key_revocation',
] as const;
export type FeedRecordType = (typeof FEED_RECORD_TYPES)[number];

/** Mirror freshness states (transport-level), reused from the public contract. */
export const FRESHNESS_STATES = [
  'active',
  'delayed',
  'stale',
  'intentionally_stopped',
  'safety_stopped',
  'source_offline',
] as const;
export type FreshnessState = (typeof FRESHNESS_STATES)[number];

/** Common envelope fields present on every snapshot record and live event. */
export interface ViewRecordEnvelope {
  schema_version: string;
  kind: SnapshotKind | LiveEventKind;
  dataset_id: string;
  quality_state: QualityState;
  observed_at: string;
  source_revision_label?: string;
}

/** A metric value that may be unavailable, with an explicit reason when missing. */
export interface MetricValue<T = number> {
  value?: T;
  unavailable_reason?: string;
}

/** Confidence interval for a pass rate or similar proportion metric. */
export interface ConfidenceInterval {
  estimate: number;
  lower: number;
  upper: number;
  denominator: number;
}

export interface BenchmarkTimingObservation {
  model_load_ms?: MetricValue<number>;
  time_to_first_token_ms?: MetricValue<number>;
  generation_ms?: MetricValue<number>;
  whole_task_ms?: MetricValue<number>;
}

export interface GPUObservation {
  vram_before_bytes?: MetricValue<number>;
  vram_peak_bytes?: MetricValue<number>;
  system_ram_peak_bytes?: MetricValue<number>;
  utilization_percent?: MetricValue<number>;
  temperature_celsius?: MetricValue<number>;
  power_watts?: MetricValue<number>;
  clock_mhz?: MetricValue<number>;
}

export interface CorrelatedFailureObservation {
  cluster_id: string;
  semantic_error_code: string;
  affected_roles: ModelRole[];
}

export interface GradeSummary {
  criterion_id: string;
  status: string;
  detail?: string;
}

export interface BenchmarkObservations {
  grade_summaries?: GradeSummary[];
  escalation_disposition?: EscalationDisposition;
  tool_scorecard?: Partial<Record<ToolScoreDimension, MetricValue<number>>>;
  security_privacy_events?: Partial<Record<SecurityPrivacyEvent, number>>;
  timing?: BenchmarkTimingObservation;
  gpu?: GPUObservation;
  correlated_failure?: CorrelatedFailureObservation;
  unavailable_reasons: string[];
}

/** Public-safe description of the provider host that served model inference
 *  for the dataset. The remote Ollama boundary exposes no hardware metadata,
 *  so every field is a display label, not a measured value or a
 *  machine-specific identifier: no hostname, IP, endpoint, or filesystem
 *  path may appear here. */
export interface ProviderEnvironment {
  source: EnvironmentSource;
  processor?: string;
  memory?: string;
  graphics?: string;
  storage?: string;
  system_type?: string;
}

/** 1. catalog_snapshot: dataset identity and aggregate counts. */
export interface CatalogSnapshot extends ViewRecordEnvelope {
  kind: 'catalog_snapshot';
  dataset_kind: DatasetKind;
  title: string;
  description: string;
  limitations: string[];
  model_count: number;
  evaluated_count: number;
  suite_count: number;
  run_count: number;
  assignment_count: number;
  provider_request_count: number;
  provider_token_count: number;
  retry_count: number;
  verifier_passed_count: number;
  verifier_failed_count: number;
  generated_at: string;
  provider_environment?: ProviderEnvironment;
}

/** 2. model_summary: per-model aggregate metrics and coverage. */
export interface ModelSummary extends ViewRecordEnvelope {
  kind: 'model_summary';
  variant_id: string;
  display_name: string;
  served_model_tag?: string;
  role: ModelRole;
  backend_provider_class?: string;
  quantization_weight_class?: string;
  inventory_only: boolean;
  evaluation_coverage: number;
  pass_rate?: ConfidenceInterval;
  agreement_pairwise?: MetricValue<number>;
  agreement_all_five?: MetricValue<number>;
  repeatability?: {
    consistently_correct: number;
    consistently_wrong: number;
    inconsistent: number;
    insufficient: number;
  };
  latency_p50_ms?: MetricValue<number>;
  latency_p95_ms?: MetricValue<number>;
  output_throughput_p50?: MetricValue<number>;
  output_throughput_p95?: MetricValue<number>;
  input_tokens?: MetricValue<number>;
  output_tokens?: MetricValue<number>;
  thinking_tokens?: MetricValue<number>;
  cache_tokens?: MetricValue<number>;
  terminal_outcomes?: Record<TerminalStatus, number>;
  unavailable_reasons?: string[];
}

/** 3. suite_summary: per-suite status, verifier state, and metric summaries. */
export interface SuiteSummary extends ViewRecordEnvelope {
  kind: 'suite_summary';
  suite_id: string;
  display_name: string;
  task_count: number;
  assignment_count: number;
  status: LifecycleStatus;
  verifier_state: VerifierState;
  verifier_failure_summary?: string;
  model_coverage: string[];
  metric_summaries: Record<string, MetricValue>;
  limitations: string[];
}

export interface NativeVerdict {
  assertion_id: string;
  assertion_version: string;
  status: NativeResultStatus;
}

export interface NativeScenarioResult {
  scenario_id: string;
  scenario_version: string;
  status: NativeScenarioStatus;
  verdicts: NativeVerdict[];
}

export interface NativeMetric {
  metric_id: string;
  metric_version: string;
  numerator: number;
  denominator: number;
  value: number;
  unit: NativeMetricUnit;
}

export interface NativeEvaluationResult {
  active_posture: NativePosture;
  lane: NativeLane;
  summary_status: NativeResultStatus;
  summary: string;
  required_verdict_count: number;
  passed_verdict_count: number;
  verification_valid: boolean;
  verification_failure_count: number;
  scenarios: NativeScenarioResult[];
  metrics: NativeMetric[];
}

export interface RunMetricValue {
  value?: number;
  unit: RunMetricUnit;
  observed_count: number;
  eligible_count: number;
  unavailable_count: number;
  unavailable_reason?: PublicUnavailableReason;
}

export interface EvaluationHeadlineMetrics {
  pass_rate?: RunMetricValue | MetricValue<number>;
  latency_p50_ms?: RunMetricValue | MetricValue<number>;
  output_throughput_p50_tokens_per_second?: RunMetricValue;
  throughput?: MetricValue<number>;
}

/** 4. evaluation_summary: per-run identity, lifecycle, and headline metrics. */
export interface EvaluationSummary extends ViewRecordEnvelope {
  kind: 'evaluation_summary';
  run_id: string;
  campaign_id?: string;
  suite_id: string;
  arm: string;
  evaluation_unit?: EvaluationUnit;
  stack_id?: string;
  primary_invocation_share?: MetricValue<number>;
  correlated_failure_rate?: MetricValue<number>;
  benchmark_unavailable_reasons?: string[];
  model_role_mapping?: Partial<Record<ModelRole, string>>;
  lifecycle_state: LifecycleStatus;
  assignment_total: number;
  assignment_completed: number;
  assignment_failed: number;
  terminal_outcomes: Record<TerminalStatus, number>;
  started_at?: string;
  ended_at?: string;
  elapsed_seconds?: number;
  verifier_state: VerifierState;
  verifier_failure_summary?: string;
  verification_metadata?: PublicVerificationMetadata;
  headline_metrics: EvaluationHeadlineMetrics;
  evidence_link?: string;
  native_result?: NativeEvaluationResult;
}

export interface PublicScenarioCriterion {
  criterion_id: string;
  public_label: string;
  public_description: string;
  grading_method: 'deterministic' | 'semantic_judge';
  required: boolean;
}

export interface PublicToolScoreDimensionRequirement {
  dimension: ToolScoreDimension;
  required: boolean;
}

export interface PublicScenarioSummary {
  scenario_id: string;
  scenario_version: string;
  category: ScenarioCategory;
  public_description: string;
  grading_method: 'deterministic' | 'semantic_judge';
  allowed_tools: string[];
  expected_tools: string[];
  forbidden_tools: string[];
  criteria: PublicScenarioCriterion[];
  tool_score_dimensions: PublicToolScoreDimensionRequirement[];
}

export interface PublicSemanticGradeSummary {
  criterion_id: string;
  status: NativeResultStatus;
  grading_method: 'deterministic' | 'semantic_judge';
  judge_variant_id?: string;
  explanation_code: PublicGradeExplanationCode;
}

export interface PublicModelActivityRecord {
  model_role: ModelRole;
  agent_persona?: string;
  variant_id: string;
  usage_availability: PublicUsageAvailability;
  input_tokens?: MetricValue<number>;
  output_tokens?: MetricValue<number>;
  thinking_tokens?: MetricValue<number>;
  cache_tokens?: MetricValue<number>;
  total_duration_nanos?: MetricValue<number>;
  generation_duration_nanos?: MetricValue<number>;
  retry_count?: MetricValue<number>;
  finish_state: PublicFinishState;
  load_state: PublicLoadState;
}

export interface PublicToolDecisionActivityRecord {
  tool_label: string;
  recognized: boolean;
  selected: boolean;
  permission_compliant: boolean;
  unnecessary: boolean;
  outcome: PublicSemanticOutcome;
  evidence_source: PublicActivityEvidenceSource;
}

export interface PublicToolCallActivityRecord {
  tool_label: string;
  execution_outcome: PublicToolExecutionOutcome;
  semantic_outcome: PublicSemanticOutcome;
  evidence_source: PublicActivityEvidenceSource;
}

export interface PublicPolicyDecisionActivityRecord {
  tool_label: string;
  outcome: PublicToolOutcome;
  evidence_source: PublicActivityEvidenceSource;
}

export interface PublicGovernedActionActivityRecord {
  action_label: 'governed action';
  reported_policy_outcome: PublicReportedPolicyOutcome;
  receipt_status: PublicReceiptStatus;
  evidence_source: PublicActivityEvidenceSource;
}

export interface PublicActivityFamily<T> {
  availability: ActivityAvailability;
  unavailable_reason?: PublicUnavailableReason;
  records: T[];
}

export interface PublicAssignmentActivity {
  model_activity: PublicActivityFamily<PublicModelActivityRecord>;
  tool_decisions: PublicActivityFamily<PublicToolDecisionActivityRecord>;
  tool_calls: PublicActivityFamily<PublicToolCallActivityRecord>;
  policy_decisions: PublicActivityFamily<PublicPolicyDecisionActivityRecord>;
  governed_actions: PublicActivityFamily<PublicGovernedActionActivityRecord>;
}

export interface PublicEvidenceBinding {
  sha256: string;
  schema_ref: string;
  kind: PublicEvidenceKind;
}

export interface PublicVerificationMetadata {
  provenance: 'bound' | 'legacy_unbound';
  verifier_state: VerifierState;
  verifier_release_version?: string;
  verifier_contract_version?: string;
  report_digest?: string;
  population_digest?: string;
  verified_at?: string;
}

/** 5. assignment_result: public assignment identity and safe summaries. */
export interface AssignmentResult extends ViewRecordEnvelope {
  kind: 'assignment_result';
  assignment_id: string;
  run_id: string;
  task_id: string;
  /** Exact scenario identity. Historical records may omit it; new records set it to task_id. */
  scenario_id?: string;
  variant_id: string;
  role: ModelRole;
  repetition: number;
  scenario_category?: ScenarioCategory;
  evaluation_unit?: EvaluationUnit;
  stack_id?: string;
  scenario_summary?: PublicScenarioSummary;
  semantic_grade_summaries?: PublicSemanticGradeSummary[];
  activity_summary?: PublicAssignmentActivity;
  evidence_bindings?: PublicEvidenceBinding[];
  benchmark_observations?: BenchmarkObservations;
  terminal_status: TerminalStatus | 'running' | 'queued';
  metric_values: Record<string, MetricValue>;
  missingness_reason?: string;
  stage_summary: Array<{ name: string; duration_seconds: number }>;
  resource_summary?: {
    latency_ms?: MetricValue<number>;
    input_tokens?: MetricValue<number>;
    output_tokens?: MetricValue<number>;
    thinking_tokens?: MetricValue<number>;
    cache_tokens?: MetricValue<number>;
    retries?: MetricValue<number>;
  };
  verification_disposition?: VerifierState;
  verification_metadata?: PublicVerificationMetadata;
}

/** 6. methodology_snapshot: metric definitions and user-facing explanation. */
export interface MethodologySnapshot extends ViewRecordEnvelope {
  kind: 'methodology_snapshot';
  metric_definitions: Array<{
    key: string;
    name: string;
    unit: string;
    direction: 'higher_is_better' | 'lower_is_better';
    denominator: string;
    missing_value_behavior: string;
    aggregation: string;
    uncertainty_method: string;
    explanation: string;
  }>;
  suite_definitions: Array<{
    suite_id: string;
    display_name: string;
    task_count: number;
    description: string;
  }>;
  limitations: string[];
}

/** Live event payload (incremental). */
export interface LiveEvent extends ViewRecordEnvelope {
  kind: LiveEventKind;
  event_id: string;
  run_id: string;
  assignment_id?: string;
  task_id?: string;
  variant_id?: string;
  role?: ModelRole;
  lifecycle_status: LifecycleStatus;
  completed: number;
  total: number;
  stage_label?: string;
  metric_delta?: Record<string, MetricValue>;
  observed_at: string;
  /** Mirror feed sequence stamped at client ingest; not part of wire payloads. */
  feed_sequence?: number;
}

/** Union of all snapshot records. */
export type SnapshotRecord =
  | CatalogSnapshot
  | ModelSummary
  | SuiteSummary
  | EvaluationSummary
  | AssignmentResult
  | MethodologySnapshot;

/** A decoded projection record from the mirror transport. The public read
 *  surface (/bootstrap, /history, /stream) never emits per-record hashes —
 *  record integrity is enforced by the snapshot feed-chain seal, so
 *  record_hash is optional here. record_bytes is the serialized view-record
 *  payload; feed.ts normalizes decoded wire items into this shape. */
export interface ProjectionRecord {
  sequence: number;
  record_type: FeedRecordType;
  record_hash?: string;
  record_bytes: string;
  decoded?: SnapshotRecord | LiveEvent;
}

/** Public feed snapshot (transport-level seal). */
export interface FeedSnapshot {
  protocol_version: string;
  source_id: string;
  high_water_sequence: number;
  feed_chain_hash: string;
  batch_count: number;
  generated_at: string;
  freshness: FreshnessState;
}

export interface FeedBootstrapRecentProjection {
  sequence: number;
  [field: string]: unknown;
}

export interface FeedHistoryItem {
  sequence: number;
  record_type: FeedRecordType;
  [field: string]: unknown;
}

/** Public feed bootstrap response. */
export interface FeedBootstrap {
  protocol_version: string;
  snapshot: FeedSnapshot;
  source_freshness: FreshnessState;
  recent_projections: FeedBootstrapRecentProjection[];
  proof_catalog_summary: { artifact_count: number; total_byte_size: number; last_generated_at?: string };
  generated_at: string;
}

/** Public feed history cursor page. */
export interface FeedHistoryPage {
  protocol_version: string;
  items: FeedHistoryItem[];
  cursor?: string;
  has_more: boolean;
  limit: number;
}
