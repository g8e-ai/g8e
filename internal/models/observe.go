// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"encoding/json"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Observe event payload typed strings. These mirror the enums defined in
// protocol/models/observe_event_payloads.json and keep the Go wire shapes in
// sync with the Python models in protocol/python/g8e/models/events.py.

// AgentLifecycleStatus is the typed string for agent lifecycle states carried
// in AgentStatusUpdatedPayload.
type AgentLifecycleStatus string

const (
	AgentLifecycleStatusIdle      AgentLifecycleStatus = "idle"
	AgentLifecycleStatusQueued    AgentLifecycleStatus = "queued"
	AgentLifecycleStatusRunning   AgentLifecycleStatus = "running"
	AgentLifecycleStatusWaiting   AgentLifecycleStatus = "waiting"
	AgentLifecycleStatusCompleted AgentLifecycleStatus = "completed"
	AgentLifecycleStatusFailed    AgentLifecycleStatus = "failed"
	AgentLifecycleStatusOffline   AgentLifecycleStatus = "offline"
)

// RunLifecycleStatus is the typed string for run lifecycle states carried in
// RunStatusUpdatedPayload.
type RunLifecycleStatus string

const (
	RunLifecycleStatusQueued    RunLifecycleStatus = "queued"
	RunLifecycleStatusRunning   RunLifecycleStatus = "running"
	RunLifecycleStatusWaiting   RunLifecycleStatus = "waiting"
	RunLifecycleStatusCompleted RunLifecycleStatus = "completed"
	RunLifecycleStatusFailed    RunLifecycleStatus = "failed"
	RunLifecycleStatusCancelled RunLifecycleStatus = "cancelled"
)

// RunKind is the typed string for run categories carried in
// RunStatusUpdatedPayload.
type RunKind string

const (
	RunKindInvestigation RunKind = "investigation"
	RunKindEval          RunKind = "eval"
	RunKindDemo          RunKind = "demo"
	RunKindWorkflow      RunKind = "workflow"
)

// EvalVerificationStatus is the honest verification boundary label for eval
// projections. It is never "verified" until the complete eval-native bundle
// verifier exists.
type EvalVerificationStatus string

const (
	EvalVerificationProjectionValidated              EvalVerificationStatus = "projection_validated"
	EvalVerificationReceiptVerificationNotApplicable EvalVerificationStatus = "receipt_verification_not_applicable"
	EvalVerificationVerified                         EvalVerificationStatus = "verified"
)

// MeasurementStatus is the freshness status for an ObservedMeasurement.
type MeasurementStatus string

const (
	MeasurementStatusObserved    MeasurementStatus = "observed"
	MeasurementStatusStale       MeasurementStatus = "stale"
	MeasurementStatusUnavailable MeasurementStatus = "unavailable"
	MeasurementStatusUnsupported MeasurementStatus = "unsupported"
)

// AgentStatusUpdatedPayload is the payload for app.agent.status.updated events.
// It reports the current lifecycle state of a single agent persona. It is
// distinct from app.agent.activity.recorded, which is a governed document
// update for AgentActivityMetadata.
type AgentStatusUpdatedPayload struct {
	SchemaVersion string               `json:"schema_version"`
	AgentID       string               `json:"agent_id"`
	DisplayName   string               `json:"display_name"`
	Role          string               `json:"role"`
	Status        AgentLifecycleStatus `json:"status"`
	RunID         string               `json:"run_id,omitempty"`
	TaskID        string               `json:"task_id,omitempty"`
	Model         string               `json:"model,omitempty"`
	ObservedAt    time.Time            `json:"observed_at"`
}

// RunStatusUpdatedPayload is the payload for app.run.status.updated events.
// It reports the current lifecycle state of a run across investigations,
// evals, demos, and workflows.
type RunStatusUpdatedPayload struct {
	SchemaVersion  string             `json:"schema_version"`
	RunID          string             `json:"run_id"`
	RunKind        RunKind            `json:"run_kind"`
	DisplayName    string             `json:"display_name"`
	Status         RunLifecycleStatus `json:"status"`
	ActiveTaskID   string             `json:"active_task_id,omitempty"`
	CompletedTasks int                `json:"completed_tasks"`
	TotalTasks     int                `json:"total_tasks"`
	StartedAt      *time.Time         `json:"started_at,omitempty"`
	EndedAt        *time.Time         `json:"ended_at,omitempty"`
	ObservedAt     time.Time          `json:"observed_at"`
}

// EvalRunCompletedPayload is the payload for ai.eval.run.completed events.
// It is emitted only after the eval projection is persisted. It reports
// terminal aggregate counts and the verification boundary of a completed
// eval run.
type EvalRunCompletedPayload struct {
	SchemaVersion             string                 `json:"schema_version"`
	RunID                     string                 `json:"run_id"`
	SuiteID                   string                 `json:"suite_id"`
	SuiteVersion              string                 `json:"suite_version"`
	ArmID                     string                 `json:"arm_id"`
	TerminalAttempts          int                    `json:"terminal_attempts"`
	AssignedTasks             int                    `json:"assigned_tasks"`
	ReceiptCount              int                    `json:"receipt_count"`
	VerificationStatus        EvalVerificationStatus `json:"verification_status"`
	PublishedProjectionSHA256 string                 `json:"published_projection_sha256"`
	CompletedAt               time.Time              `json:"completed_at"`
}

// EvalMetricRecordedPayload is the payload for ai.eval.metric.recorded events.
// It reports a single registered metric value with its population,
// denominator, and verification status.
type EvalMetricRecordedPayload struct {
	SchemaVersion      string                 `json:"schema_version"`
	RunID              string                 `json:"run_id"`
	MetricID           string                 `json:"metric_id"`
	MetricVersion      string                 `json:"metric_version"`
	Value              *float64               `json:"value,omitempty"`
	Unit               string                 `json:"unit"`
	Eligible           int                    `json:"eligible"`
	Denominator        int                    `json:"denominator"`
	VerificationStatus EvalVerificationStatus `json:"verification_status"`
	RecordedAt         time.Time              `json:"recorded_at"`
}

// ObservedMeasurement is the shared typed measurement shape for displayed
// resource and throughput values. Every displayed measurement uses a typed
// value with source and observation time. Resource and throughput cards
// remain unavailable until real instrumentation exists.
type ObservedMeasurement struct {
	SchemaVersion   string            `json:"schema_version"`
	MetricID        string            `json:"metric_id"`
	Value           float64           `json:"value"`
	Unit            string            `json:"unit"`
	SourceComponent string            `json:"source_component"`
	ObservedAt      time.Time         `json:"observed_at"`
	WindowSeconds   *float64          `json:"window_seconds,omitempty"`
	Status          MeasurementStatus `json:"status"`
}

// ObserveEventPayloadForType returns the schema version constant for the given
// observe dashboard event type. It returns an empty string for event types
// that are not part of the observability dashboard family.
func ObserveEventPayloadSchemaVersion(eventType constants.EventType) string {
	switch eventType {
	case constants.EventAppAgentStatusUpdated,
		constants.EventAppRunStatusUpdated,
		constants.EventAiEvalRunCompleted,
		constants.EventAiEvalMetricRecorded:
		return constants.ObserveEventPayloadSchemaVersion
	default:
		return ""
	}
}

// SnapshotFreshness is the freshness status for a snapshot projection.
type SnapshotFreshness string

const (
	SnapshotFreshnessObserved    SnapshotFreshness = "observed"
	SnapshotFreshnessStale       SnapshotFreshness = "stale"
	SnapshotFreshnessUnavailable SnapshotFreshness = "unavailable"
)

// DownloadPrivacyClassification is the disclosure classification for a download
// artifact. Only public_safe artifacts appear in the browser catalog.
type DownloadPrivacyClassification string

const (
	DownloadPrivacyPublicSafe DownloadPrivacyClassification = "public_safe"
	DownloadPrivacyRestricted DownloadPrivacyClassification = "restricted"
)

// AgentStateProjection is the current lifecycle state of a single agent persona
// in the roster. It combines the static persona registry projection with the
// current agent-state snapshot.
type AgentStateProjection struct {
	SchemaVersion string               `json:"schema_version"`
	AgentID       string               `json:"agent_id"`
	DisplayName   string               `json:"display_name"`
	Role          string               `json:"role"`
	Status        AgentLifecycleStatus `json:"status"`
	RunID         string               `json:"run_id,omitempty"`
	TaskID        string               `json:"task_id,omitempty"`
	Model         string               `json:"model,omitempty"`
	Throughput    *ObservedMeasurement `json:"throughput,omitempty"`
	Freshness     SnapshotFreshness    `json:"freshness"`
	ObservedAt    time.Time            `json:"observed_at"`
}

// SuccessRateMeasurement is the aggregate success-rate measurement with
// metric ID, version, denominator, and verification status.
type SuccessRateMeasurement struct {
	MetricID           string                 `json:"metric_id"`
	MetricVersion      string                 `json:"metric_version"`
	Value              float64                `json:"value"`
	Unit               string                 `json:"unit"`
	Denominator        int                    `json:"denominator"`
	VerificationStatus EvalVerificationStatus `json:"verification_status"`
}

// OverviewCounters is the bounded overview counters for the dashboard header.
// Each counter carries a freshness label so the frontend can show stale or
// unavailable states honestly.
type OverviewCounters struct {
	SchemaVersion          string                  `json:"schema_version"`
	AgentsRunning          int                     `json:"agents_running"`
	AgentsRunningFreshness SnapshotFreshness       `json:"agents_running_freshness"`
	TasksInQueue           int                     `json:"tasks_in_queue"`
	TasksInQueueFreshness  SnapshotFreshness       `json:"tasks_in_queue_freshness"`
	SuccessRate            *SuccessRateMeasurement `json:"success_rate,omitempty"`
	GeneratedAt            time.Time               `json:"generated_at"`
}

// OverviewMeasurements contains resource and throughput measurements for the
// dashboard. Each card uses ObservedMeasurement with explicit status. Cards
// remain unavailable until a real host telemetry collector exists.
type OverviewMeasurements struct {
	SchemaVersion   string               `json:"schema_version"`
	TotalThroughput *ObservedMeasurement `json:"total_throughput,omitempty"`
	CPU             *ObservedMeasurement `json:"cpu,omitempty"`
	RAM             *ObservedMeasurement `json:"ram,omitempty"`
	VRAM            *ObservedMeasurement `json:"vram,omitempty"`
	Disk            *ObservedMeasurement `json:"disk,omitempty"`
}

// RunSummary is the paginated read-only run summary owned by the authenticated
// user. It uses stable run IDs, kinds, timestamps, terminal status, and
// evidence/receipt indicators.
type RunSummary struct {
	SchemaVersion  string             `json:"schema_version"`
	RunID          string             `json:"run_id"`
	RunKind        RunKind            `json:"run_kind"`
	DisplayName    string             `json:"display_name"`
	Status         RunLifecycleStatus `json:"status"`
	ActiveTaskID   string             `json:"active_task_id,omitempty"`
	CompletedTasks int                `json:"completed_tasks"`
	TotalTasks     int                `json:"total_tasks"`
	StartedAt      *time.Time         `json:"started_at,omitempty"`
	EndedAt        *time.Time         `json:"ended_at,omitempty"`
	HasReceipts    bool               `json:"has_receipts"`
	EvidenceCount  int                `json:"evidence_count"`
	ObservedAt     time.Time          `json:"observed_at"`
}

// RunTask is a single task within a run detail projection.
type RunTask struct {
	SchemaVersion string             `json:"schema_version"`
	TaskID        string             `json:"task_id"`
	DisplayName   string             `json:"display_name,omitempty"`
	Status        RunLifecycleStatus `json:"status"`
	OwningAgent   string             `json:"owning_agent,omitempty"`
	Model         string             `json:"model,omitempty"`
	StartedAt     *time.Time         `json:"started_at,omitempty"`
	EndedAt       *time.Time         `json:"ended_at,omitempty"`
}

// EvidenceSafeLink is an evidence-safe link in a run detail projection. It
// links to allowlisted download artifacts only; it never exposes raw prompts,
// outputs, or restricted evidence.
type EvidenceSafeLink struct {
	SchemaVersion string `json:"schema_version"`
	ArtifactID    string `json:"artifact_id"`
	Label         string `json:"label"`
	MediaType     string `json:"media_type"`
}

// RunDetail is the typed run detail projection with tasks and evidence-safe
// links. It is owned by the authenticated user.
type RunDetail struct {
	SchemaVersion     string             `json:"schema_version"`
	RunID             string             `json:"run_id"`
	RunKind           RunKind            `json:"run_kind"`
	DisplayName       string             `json:"display_name"`
	Status            RunLifecycleStatus `json:"status"`
	ActiveTaskID      string             `json:"active_task_id,omitempty"`
	CompletedTasks    int                `json:"completed_tasks"`
	TotalTasks        int                `json:"total_tasks"`
	Tasks             []RunTask          `json:"tasks"`
	EvidenceSafeLinks []EvidenceSafeLink `json:"evidence_safe_links"`
	StartedAt         *time.Time         `json:"started_at,omitempty"`
	EndedAt           *time.Time         `json:"ended_at,omitempty"`
	ObservedAt        time.Time          `json:"observed_at"`
}

// EvalSummary is the paginated eval run projection. It uses manifest
// role-to-model mappings and registered metrics with honest verification
// labels.
type EvalSummary struct {
	SchemaVersion             string                 `json:"schema_version"`
	RunID                     string                 `json:"run_id"`
	SuiteID                   string                 `json:"suite_id"`
	SuiteVersion              string                 `json:"suite_version"`
	ArmID                     string                 `json:"arm_id"`
	Status                    RunLifecycleStatus     `json:"status"`
	VerificationStatus        EvalVerificationStatus `json:"verification_status"`
	ReceiptCount              int                    `json:"receipt_count"`
	MetricCount               int                    `json:"metric_count"`
	PublishedProjectionSHA256 string                 `json:"published_projection_sha256,omitempty"`
	CompletedAt               *time.Time             `json:"completed_at,omitempty"`
	ObservedAt                time.Time              `json:"observed_at"`
}

// EvalMetricSummary is a single registered metric in an eval detail projection.
type EvalMetricSummary struct {
	SchemaVersion      string                 `json:"schema_version"`
	MetricID           string                 `json:"metric_id"`
	MetricVersion      string                 `json:"metric_version"`
	Value              *float64               `json:"value,omitempty"`
	Unit               string                 `json:"unit"`
	Eligible           int                    `json:"eligible"`
	Denominator        int                    `json:"denominator"`
	VerificationStatus EvalVerificationStatus `json:"verification_status"`
	RecordedAt         *time.Time             `json:"recorded_at,omitempty"`
}

// EvalDetail is the typed eval detail projection with manifest-safe identity,
// arm/model information, aggregate metrics, status, and verification boundary.
type EvalDetail struct {
	SchemaVersion             string                 `json:"schema_version"`
	RunID                     string                 `json:"run_id"`
	SuiteID                   string                 `json:"suite_id"`
	SuiteVersion              string                 `json:"suite_version"`
	ArmID                     string                 `json:"arm_id"`
	ModelID                   string                 `json:"model_id,omitempty"`
	ModelProvider             string                 `json:"model_provider,omitempty"`
	Status                    RunLifecycleStatus     `json:"status"`
	VerificationStatus        EvalVerificationStatus `json:"verification_status"`
	ReceiptCount              int                    `json:"receipt_count"`
	AssignedTasks             int                    `json:"assigned_tasks"`
	TerminalAttempts          int                    `json:"terminal_attempts"`
	Metrics                   []EvalMetricSummary    `json:"metrics"`
	PublishedProjectionSHA256 string                 `json:"published_projection_sha256,omitempty"`
	CompletedAt               *time.Time             `json:"completed_at,omitempty"`
	ObservedAt                time.Time              `json:"observed_at"`
}

// DownloadArtifact is an allowlisted generated public-safe artifact in the
// download catalog. It has fixed content type, byte size, SHA-256 hash,
// privacy classification, and authenticated download URL.
type DownloadArtifact struct {
	SchemaVersion         string                        `json:"schema_version"`
	ArtifactID            string                        `json:"artifact_id"`
	Filename              string                        `json:"filename"`
	MediaType             string                        `json:"media_type"`
	ByteSize              int64                         `json:"byte_size"`
	SHA256                string                        `json:"sha256"`
	PrivacyClassification DownloadPrivacyClassification `json:"privacy_classification"`
	SourceRunID           string                        `json:"source_run_id,omitempty"`
	DownloadURL           string                        `json:"download_url"`
	GeneratedAt           time.Time                     `json:"generated_at"`
}

// ObservePage is the pagination wrapper for paginated observe API responses.
// It uses stable cursors and bounded page sizes. The Items field is a slice of
// json.RawMessage so the same wrapper serves run_summary, eval_summary, and
// download_artifact endpoints.
type ObservePage struct {
	SchemaVersion string          `json:"schema_version"`
	Items         json.RawMessage `json:"items"`
	Cursor        string          `json:"cursor,omitempty"`
	HasMore       bool            `json:"has_more"`
	Limit         int             `json:"limit"`
}

// ObserveBootstrapSnapshot is one bounded initial snapshot for first paint.
// It contains agents, active run, overview counters, recent runs, latest eval
// summaries, measurements, and download metadata. SSE provides incremental
// change notifications after the snapshot.
type ObserveBootstrapSnapshot struct {
	SchemaVersion string                 `json:"schema_version"`
	Agents        []AgentStateProjection `json:"agents"`
	ActiveRun     *RunSummary            `json:"active_run,omitempty"`
	Overview      OverviewCounters       `json:"overview"`
	Measurements  OverviewMeasurements   `json:"measurements"`
	RecentRuns    []RunSummary           `json:"recent_runs"`
	LatestEvals   []EvalSummary          `json:"latest_evals"`
	Downloads     []DownloadArtifact     `json:"downloads"`
	GeneratedAt   time.Time              `json:"generated_at"`
}

// ObserveProducerAgentStateRequest is the typed request body for the mTLS
// producer endpoint POST /api/v1/observe/producer/agent-state. It carries the
// AgentStatusUpdatedPayload fields plus the SSE routing target (exactly one of
// web_session_id or cli_session_id). The gateway derives user_id from the mTLS
// peer certificate, never from the request body.
type ObserveProducerAgentStateRequest struct {
	SchemaVersion string               `json:"schema_version"`
	AgentID       string               `json:"agent_id"`
	DisplayName   string               `json:"display_name"`
	Role          string               `json:"role"`
	Status        AgentLifecycleStatus `json:"status"`
	RunID         string               `json:"run_id,omitempty"`
	TaskID        string               `json:"task_id,omitempty"`
	Model         string               `json:"model,omitempty"`
	ObservedAt    time.Time            `json:"observed_at"`
	WebSessionID  string               `json:"web_session_id,omitempty"`
	CLISessionID  string               `json:"cli_session_id,omitempty"`
}

// ObserveProducerRunStateRequest is the typed request body for the mTLS
// producer endpoint POST /api/v1/observe/producer/run-state. It carries the
// RunStatusUpdatedPayload fields plus the SSE routing target. The gateway
// derives user_id from the mTLS peer certificate, never from the request body.
type ObserveProducerRunStateRequest struct {
	SchemaVersion  string             `json:"schema_version"`
	RunID          string             `json:"run_id"`
	RunKind        RunKind            `json:"run_kind"`
	DisplayName    string             `json:"display_name"`
	Status         RunLifecycleStatus `json:"status"`
	ActiveTaskID   string             `json:"active_task_id,omitempty"`
	CompletedTasks int                `json:"completed_tasks"`
	TotalTasks     int                `json:"total_tasks"`
	StartedAt      *time.Time         `json:"started_at,omitempty"`
	EndedAt        *time.Time         `json:"ended_at,omitempty"`
	ObservedAt     time.Time          `json:"observed_at"`
	WebSessionID   string             `json:"web_session_id,omitempty"`
	CLISessionID   string             `json:"cli_session_id,omitempty"`
}
