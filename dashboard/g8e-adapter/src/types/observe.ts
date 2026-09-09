// Typed observe read models and event payloads mirroring the Go contracts in
// internal/models/observe.go. Field names and JSON tags match the Go wire
// shapes byte-for-byte. Optional Go fields (omitempty with pointer or zero
// value) are typed as optional here.

import type {
  AgentLifecycleStatus,
  DownloadPrivacyClassification,
  EvalVerificationStatus,
  MeasurementStatus,
  RunKind,
  RunLifecycleStatus,
  SnapshotFreshness,
} from './enums';

// An ISO-8601 UTC timestamp string. The Go side serializes time.Time as RFC3339.
export type UTCDatetime = string;

// ObservedMeasurement is the shared typed measurement shape for displayed
// resource and throughput values. Resource and throughput cards remain
// unavailable until real instrumentation exists.
export interface ObservedMeasurement {
  schema_version: string;
  metric_id: string;
  value: number;
  unit: string;
  source_component: string;
  observed_at: UTCDatetime;
  window_seconds?: number;
  status: MeasurementStatus;
}

// AgentStateProjection is the current lifecycle state of a single agent persona
// in the roster.
export interface AgentStateProjection {
  schema_version: string;
  agent_id: string;
  display_name: string;
  role: string;
  status: AgentLifecycleStatus;
  run_id?: string;
  task_id?: string;
  model?: string;
  throughput?: ObservedMeasurement;
  freshness: SnapshotFreshness;
  observed_at: UTCDatetime;
}

// SuccessRateMeasurement is the aggregate success-rate measurement.
export interface SuccessRateMeasurement {
  metric_id: string;
  metric_version: string;
  value: number;
  unit: string;
  denominator: number;
  verification_status: EvalVerificationStatus;
}

// OverviewCounters is the bounded overview counters for the dashboard header.
export interface OverviewCounters {
  schema_version: string;
  agents_running: number;
  agents_running_freshness: SnapshotFreshness;
  tasks_in_queue: number;
  tasks_in_queue_freshness: SnapshotFreshness;
  success_rate?: SuccessRateMeasurement;
  generated_at: UTCDatetime;
}

// OverviewMeasurements contains resource and throughput measurements. Cards
// remain unavailable until a real host telemetry collector exists.
export interface OverviewMeasurements {
  schema_version: string;
  total_throughput?: ObservedMeasurement;
  cpu?: ObservedMeasurement;
  ram?: ObservedMeasurement;
  vram?: ObservedMeasurement;
  disk?: ObservedMeasurement;
}

// RunSummary is the paginated read-only run summary owned by the authenticated
// user.
export interface RunSummary {
  schema_version: string;
  run_id: string;
  run_kind: RunKind;
  display_name: string;
  status: RunLifecycleStatus;
  active_task_id?: string;
  completed_tasks: number;
  total_tasks: number;
  started_at?: UTCDatetime;
  ended_at?: UTCDatetime;
  has_receipts: boolean;
  evidence_count: number;
  observed_at: UTCDatetime;
}

// RunTask is a single task within a run detail projection.
export interface RunTask {
  schema_version: string;
  task_id: string;
  display_name?: string;
  status: RunLifecycleStatus;
  owning_agent?: string;
  model?: string;
  started_at?: UTCDatetime;
  ended_at?: UTCDatetime;
}

// EvidenceSafeLink is an evidence-safe link in a run detail projection.
export interface EvidenceSafeLink {
  schema_version: string;
  artifact_id: string;
  label: string;
  media_type: string;
}

// RunDetail is the typed run detail projection with tasks and evidence-safe
// links.
export interface RunDetail {
  schema_version: string;
  run_id: string;
  run_kind: RunKind;
  display_name: string;
  status: RunLifecycleStatus;
  active_task_id?: string;
  completed_tasks: number;
  total_tasks: number;
  tasks: RunTask[];
  evidence_safe_links: EvidenceSafeLink[];
  started_at?: UTCDatetime;
  ended_at?: UTCDatetime;
  observed_at: UTCDatetime;
}

// EvalSummary is the paginated eval run projection.
export interface EvalSummary {
  schema_version: string;
  run_id: string;
  suite_id: string;
  suite_version: string;
  arm_id: string;
  status: RunLifecycleStatus;
  verification_status: EvalVerificationStatus;
  receipt_count: number;
  metric_count: number;
  published_projection_sha256?: string;
  completed_at?: UTCDatetime;
  observed_at: UTCDatetime;
}

// EvalMetricSummary is a single registered metric in an eval detail projection.
export interface EvalMetricSummary {
  schema_version: string;
  metric_id: string;
  metric_version: string;
  value?: number;
  unit: string;
  eligible: number;
  denominator: number;
  verification_status: EvalVerificationStatus;
  recorded_at?: UTCDatetime;
}

// EvalDetail is the typed eval detail projection.
export interface EvalDetail {
  schema_version: string;
  run_id: string;
  suite_id: string;
  suite_version: string;
  arm_id: string;
  model_id?: string;
  model_provider?: string;
  status: RunLifecycleStatus;
  verification_status: EvalVerificationStatus;
  receipt_count: number;
  assigned_tasks: number;
  terminal_attempts: number;
  metrics: EvalMetricSummary[];
  published_projection_sha256?: string;
  completed_at?: UTCDatetime;
  observed_at: UTCDatetime;
}

// DownloadArtifact is an allowlisted generated public-safe artifact in the
// download catalog.
export interface DownloadArtifact {
  schema_version: string;
  artifact_id: string;
  filename: string;
  media_type: string;
  byte_size: number;
  sha256: string;
  privacy_classification: DownloadPrivacyClassification;
  source_run_id?: string;
  download_url: string;
  generated_at: UTCDatetime;
}

// ObservePage is the pagination wrapper for paginated observe API responses.
// The items field is a JSON array of the endpoint-specific summary type.
export interface ObservePage<T> {
  schema_version: string;
  items: T[];
  cursor?: string;
  has_more: boolean;
  limit: number;
}

// ObserveBootstrapSnapshot is one bounded initial snapshot for first paint.
export interface ObserveBootstrapSnapshot {
  schema_version: string;
  agents: AgentStateProjection[];
  active_run?: RunSummary;
  overview: OverviewCounters;
  measurements: OverviewMeasurements;
  recent_runs: RunSummary[];
  latest_evals: EvalSummary[];
  downloads: DownloadArtifact[];
  generated_at: UTCDatetime;
}

// Event payloads carried inside SSE envelopes. These mirror the Go
// *Payload structs in internal/models/observe.go.

export interface AgentStatusUpdatedPayload {
  schema_version: string;
  agent_id: string;
  display_name: string;
  role: string;
  status: AgentLifecycleStatus;
  run_id?: string;
  task_id?: string;
  model?: string;
  observed_at: UTCDatetime;
}

export interface RunStatusUpdatedPayload {
  schema_version: string;
  run_id: string;
  run_kind: RunKind;
  display_name: string;
  status: RunLifecycleStatus;
  active_task_id?: string;
  completed_tasks: number;
  total_tasks: number;
  started_at?: UTCDatetime;
  ended_at?: UTCDatetime;
  observed_at: UTCDatetime;
}

export interface EvalRunCompletedPayload {
  schema_version: string;
  run_id: string;
  suite_id: string;
  suite_version: string;
  arm_id: string;
  terminal_attempts: number;
  assigned_tasks: number;
  receipt_count: number;
  verification_status: EvalVerificationStatus;
  published_projection_sha256: string;
  completed_at: UTCDatetime;
}

export interface EvalMetricRecordedPayload {
  schema_version: string;
  run_id: string;
  metric_id: string;
  metric_version: string;
  value?: number;
  unit: string;
  eligible: number;
  denominator: number;
  verification_status: EvalVerificationStatus;
  recorded_at: UTCDatetime;
}
