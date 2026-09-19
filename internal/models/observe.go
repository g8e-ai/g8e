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

// MeasurementScope is the typed scope for an ObservedMeasurement from the S6
// typed observer. Process scope measures the eval process, system scope
// measures the whole host, and accelerator scope measures the GPU/NPU.
type MeasurementScope string

const (
	MeasurementScopeProcess     MeasurementScope = "process"
	MeasurementScopeSystem      MeasurementScope = "system"
	MeasurementScopeAccelerator MeasurementScope = "accelerator"
)

// CampaignFreshness is the freshness status for campaign source projections.
// It uses a closed vocabulary that distinguishes active, delayed, stale,
// intentionally stopped, safety stopped, and source offline states.
type CampaignFreshness string

const (
	CampaignFreshnessActive               CampaignFreshness = "active"
	CampaignFreshnessDelayed              CampaignFreshness = "delayed"
	CampaignFreshnessStale                CampaignFreshness = "stale"
	CampaignFreshnessIntentionallyStopped CampaignFreshness = "intentionally_stopped"
	CampaignFreshnessSafetyStopped        CampaignFreshness = "safety_stopped"
	CampaignFreshnessSourceOffline        CampaignFreshness = "source_offline"
)

// CampaignCycleStatus is the lifecycle state of a single campaign cycle.
type CampaignCycleStatus string

const (
	CampaignCycleStatusQueued     CampaignCycleStatus = "queued"
	CampaignCycleStatusRunning    CampaignCycleStatus = "running"
	CampaignCycleStatusCompleted  CampaignCycleStatus = "completed"
	CampaignCycleStatusFailed     CampaignCycleStatus = "failed"
	CampaignCycleStatusSuperseded CampaignCycleStatus = "superseded"
)

// AssignmentProgressStatus is the lifecycle state of a single campaign
// assignment.
type AssignmentProgressStatus string

const (
	AssignmentProgressStatusQueued      AssignmentProgressStatus = "queued"
	AssignmentProgressStatusRunning     AssignmentProgressStatus = "running"
	AssignmentProgressStatusCompleted   AssignmentProgressStatus = "completed"
	AssignmentProgressStatusFailed      AssignmentProgressStatus = "failed"
	AssignmentProgressStatusSuperseded  AssignmentProgressStatus = "superseded"
	AssignmentProgressStatusUnavailable AssignmentProgressStatus = "unavailable"
)

// TerminalOutcomeStatus is the terminal outcome of a campaign assignment.
type TerminalOutcomeStatus string

const (
	TerminalOutcomeStatusCompleted     TerminalOutcomeStatus = "completed"
	TerminalOutcomeStatusFailed        TerminalOutcomeStatus = "failed"
	TerminalOutcomeStatusSuperseded    TerminalOutcomeStatus = "superseded"
	TerminalOutcomeStatusUnavailable   TerminalOutcomeStatus = "unavailable"
	TerminalOutcomeStatusQualification TerminalOutcomeStatus = "qualification"
)

// SupervisorStatus is the lifecycle state of the continuous campaign
// supervisor.
type SupervisorStatus string

const (
	SupervisorStatusIdle          SupervisorStatus = "idle"
	SupervisorStatusRunning       SupervisorStatus = "running"
	SupervisorStatusStopped       SupervisorStatus = "stopped"
	SupervisorStatusSafetyStopped SupervisorStatus = "safety_stopped"
)

// StopReason is the typed stop reason from a closed vocabulary distinguishing
// graceful owner stops from safety stops.
type StopReason string

const (
	StopReasonGraceful            StopReason = "graceful"
	StopReasonSafetyBudget        StopReason = "safety_budget"
	StopReasonSafetyDisk          StopReason = "safety_disk"
	StopReasonSafetyHardwareDrift StopReason = "safety_hardware_drift"
	StopReasonSafetyCredential    StopReason = "safety_credential"
	StopReasonSafetyVerifier      StopReason = "safety_verifier"
	StopReasonSafetyDisclosure    StopReason = "safety_disclosure"
	StopReasonSafetyPublication   StopReason = "safety_publication"
)

// StopScope is the scope of a stop request: cycle or supervisor.
type StopScope string

const (
	StopScopeCycle      StopScope = "cycle"
	StopScopeSupervisor StopScope = "supervisor"
)

// ModelRole is the model role exercised in a campaign assignment.
type ModelRole string

const (
	ModelRolePrimary   ModelRole = "primary"
	ModelRoleAssistant ModelRole = "assistant"
	ModelRoleLite      ModelRole = "lite"
)

// PublicationStatus is the lifecycle state of the publication path for a
// single cycle.
type PublicationStatus string

const (
	PublicationStatusPending   PublicationStatus = "pending"
	PublicationStatusPublished PublicationStatus = "published"
	PublicationStatusFailed    PublicationStatus = "failed"
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
// campaign dimensions, terminal aggregate counts, and the verification
// boundary of a completed eval run. Carries arm_ids (list) and campaign_id
// so a multi-arm, multi-cohort campaign renders as one event.
type EvalRunCompletedPayload struct {
	SchemaVersion             string                 `json:"schema_version"`
	RunID                     string                 `json:"run_id"`
	SuiteID                   string                 `json:"suite_id"`
	SuiteVersion              string                 `json:"suite_version"`
	CampaignID                string                 `json:"campaign_id"`
	ArmIDs                    []string               `json:"arm_ids"`
	TerminalAttempts          int                    `json:"terminal_attempts"`
	AssignedTasks             int                    `json:"assigned_tasks"`
	ReceiptCount              int                    `json:"receipt_count"`
	VerificationStatus        EvalVerificationStatus `json:"verification_status"`
	PublishedProjectionSHA256 string                 `json:"published_projection_sha256"`
	CompletedAt               time.Time              `json:"completed_at"`
}

// EvalMetricRecordedPayload is the payload for ai.eval.metric.recorded events.
// It reports a single registered metric value with its cohort and arm stratum,
// population, denominator, and verification status.
type EvalMetricRecordedPayload struct {
	SchemaVersion      string                 `json:"schema_version"`
	RunID              string                 `json:"run_id"`
	MetricID           string                 `json:"metric_id"`
	MetricVersion      string                 `json:"metric_version"`
	ModelCohortID      string                 `json:"model_cohort_id"`
	ArmID              string                 `json:"arm_id"`
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
	SchemaVersion    string            `json:"schema_version"`
	MetricID         string            `json:"metric_id"`
	Value            float64           `json:"value"`
	Unit             string            `json:"unit"`
	SourceComponent  string            `json:"source_component"`
	ObservedAt       time.Time         `json:"observed_at"`
	WindowSeconds    *float64          `json:"window_seconds,omitempty"`
	Status           MeasurementStatus `json:"status"`
	Scope            MeasurementScope  `json:"scope,omitempty"`
	CollectorVersion string            `json:"collector_version,omitempty"`
	EvidenceHash     string            `json:"evidence_hash,omitempty"`
}

// ObserveEventPayloadForType returns the schema version constant for the given
// observe dashboard event type. It returns an empty string for event types
// that are not part of the observability dashboard family.
func ObserveEventPayloadSchemaVersion(eventType constants.EventType) string {
	switch eventType {
	case constants.EventAppAgentStatusUpdated,
		constants.EventAppRunStatusUpdated,
		constants.EventAiEvalRunCompleted,
		constants.EventAiEvalMetricRecorded,
		constants.EventAiEvalCycleStarted,
		constants.EventAiEvalCycleCompleted,
		constants.EventAiEvalAssignmentStarted,
		constants.EventAiEvalAssignmentCompleted,
		constants.EventAiEvalModelRoleInvoked,
		constants.EventAiEvalMetricAvailable,
		constants.EventAiEvalVerifierCompleted,
		constants.EventAiEvalProofAvailable,
		constants.EventAiEvalPublicationCompleted,
		constants.EventAiEvalHeartbeat,
		constants.EventAiEvalStopRequested:
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

// EvalSummary is the paginated eval run projection. It carries campaign
// dimensions (campaign_id, arm_ids, model_cohort_ids) so a multi-arm,
// multi-cohort campaign renders as one projection. Uses registered metrics
// with honest verification labels.
type EvalSummary struct {
	SchemaVersion             string                 `json:"schema_version"`
	RunID                     string                 `json:"run_id"`
	SuiteID                   string                 `json:"suite_id"`
	SuiteVersion              string                 `json:"suite_version"`
	CampaignID                string                 `json:"campaign_id"`
	ArmIDs                    []string               `json:"arm_ids"`
	ModelCohortIDs            []string               `json:"model_cohort_ids"`
	Status                    RunLifecycleStatus     `json:"status"`
	VerificationStatus        EvalVerificationStatus `json:"verification_status"`
	ReceiptCount              int                    `json:"receipt_count"`
	MetricCount               int                    `json:"metric_count"`
	PublishedProjectionSHA256 string                 `json:"published_projection_sha256,omitempty"`
	CompletedAt               *time.Time             `json:"completed_at,omitempty"`
	ObservedAt                time.Time              `json:"observed_at"`
}

// EvalMetricSummary is a single registered metric in an eval detail projection,
// stratified by model cohort and arm.
type EvalMetricSummary struct {
	SchemaVersion      string                 `json:"schema_version"`
	MetricID           string                 `json:"metric_id"`
	MetricVersion      string                 `json:"metric_version"`
	ModelCohortID      string                 `json:"model_cohort_id"`
	ArmID              string                 `json:"arm_id"`
	Value              *float64               `json:"value,omitempty"`
	Unit               string                 `json:"unit"`
	Eligible           int                    `json:"eligible"`
	Denominator        int                    `json:"denominator"`
	VerificationStatus EvalVerificationStatus `json:"verification_status"`
	RecordedAt         *time.Time             `json:"recorded_at,omitempty"`
}

// EvalDetail is the typed eval detail projection with campaign dimensions,
// cohort-stratified aggregate metrics, status, and verification boundary.
// Model identity is per-cohort (carried via model_cohort_ids), not a single
// scalar.
type EvalDetail struct {
	SchemaVersion             string                 `json:"schema_version"`
	RunID                     string                 `json:"run_id"`
	SuiteID                   string                 `json:"suite_id"`
	SuiteVersion              string                 `json:"suite_version"`
	CampaignID                string                 `json:"campaign_id"`
	ArmIDs                    []string               `json:"arm_ids"`
	ModelCohortIDs            []string               `json:"model_cohort_ids"`
	Status                    RunLifecycleStatus     `json:"status"`
	VerificationStatus        EvalVerificationStatus `json:"verification_status"`
	ReceiptCount              int                    `json:"receipt_count"`
	AssignedTasks             int                    `json:"assigned_tasks"`
	TerminalAttempts          int                    `json:"terminal_attempts"`
	Metrics                   []EvalMetricSummary    `json:"metrics"`
	RoleCombinationID         string                 `json:"role_combination_id,omitempty"`
	PrimaryVariantID          string                 `json:"primary_variant_id,omitempty"`
	AssistantVariantID        string                 `json:"assistant_variant_id,omitempty"`
	LiteVariantID             string                 `json:"lite_variant_id,omitempty"`
	BenchmarkPopulation       int                    `json:"benchmark_population,omitempty"`
	RepetitionCount           int                    `json:"repetition_count,omitempty"`
	SupersededCount           int                    `json:"superseded_count,omitempty"`
	QualificationCount        int                    `json:"qualification_count,omitempty"`
	UnavailableCount          int                    `json:"unavailable_count,omitempty"`
	EnvironmentClass          string                 `json:"environment_class,omitempty"`
	BackendName               string                 `json:"backend_name,omitempty"`
	ArtifactDigest            string                 `json:"artifact_digest,omitempty"`
	Quantization              string                 `json:"quantization,omitempty"`
	ProofLinks                []EvidenceSafeLink     `json:"proof_links,omitempty"`
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

// ObserveProducerResponse is the typed response body for the mTLS producer
// endpoints. It carries a single accepted flag indicating the gateway
// accepted and persisted the projection. The response contains no record
// identifiers, ownership fields, or echo of the request payload.
type ObserveProducerResponse struct {
	Accepted bool `json:"accepted"`
}

// ---------------------------------------------------------------------------
// Live campaign event payloads (g8e.v1.ai.eval.* family)
//
// These payloads carry source_sequence (monotonic ordering), event_id
// (duplicate suppression), and observed_at on every event. Producers persist
// the corresponding projection before emitting the SSE event. See
// protocol/models/observe_event_payloads.json for the canonical wire shapes.
// ---------------------------------------------------------------------------

// EvalCycleStartedPayload is the payload for ai.eval.cycle.started events.
type EvalCycleStartedPayload struct {
	SchemaVersion       string    `json:"schema_version"`
	SourceSequence      int64     `json:"source_sequence"`
	EventID             string    `json:"event_id"`
	CycleID             string    `json:"cycle_id"`
	CampaignID          string    `json:"campaign_id"`
	CampaignRevision    string    `json:"campaign_revision"`
	RoleCombinationID   string    `json:"role_combination_id"`
	PrimaryVariantID    string    `json:"primary_variant_id"`
	AssistantVariantID  string    `json:"assistant_variant_id,omitempty"`
	LiteVariantID       string    `json:"lite_variant_id,omitempty"`
	BenchmarkPopulation int       `json:"benchmark_population"`
	RepetitionCount     int       `json:"repetition_count"`
	StartedAt           time.Time `json:"started_at"`
	ObservedAt          time.Time `json:"observed_at"`
}

// EvalCycleCompletedPayload is the payload for ai.eval.cycle.completed events.
type EvalCycleCompletedPayload struct {
	SchemaVersion      string                 `json:"schema_version"`
	SourceSequence     int64                  `json:"source_sequence"`
	EventID            string                 `json:"event_id"`
	CycleID            string                 `json:"cycle_id"`
	CampaignID         string                 `json:"campaign_id"`
	CampaignRevision   string                 `json:"campaign_revision"`
	RoleCombinationID  string                 `json:"role_combination_id"`
	VerificationStatus EvalVerificationStatus `json:"verification_status"`
	TerminalAttempts   int                    `json:"terminal_attempts"`
	AssignedTasks      int                    `json:"assigned_tasks"`
	CompletedAt        time.Time              `json:"completed_at"`
	ObservedAt         time.Time              `json:"observed_at"`
}

// EvalAssignmentStartedPayload is the payload for
// ai.eval.assignment.started events.
type EvalAssignmentStartedPayload struct {
	SchemaVersion  string    `json:"schema_version"`
	SourceSequence int64     `json:"source_sequence"`
	EventID        string    `json:"event_id"`
	CycleID        string    `json:"cycle_id"`
	CampaignID     string    `json:"campaign_id"`
	AssignmentID   string    `json:"assignment_id"`
	VariantID      string    `json:"variant_id"`
	Role           ModelRole `json:"role"`
	TaskID         string    `json:"task_id"`
	ArmID          string    `json:"arm_id"`
	Repetition     int       `json:"repetition"`
	StartedAt      time.Time `json:"started_at"`
	ObservedAt     time.Time `json:"observed_at"`
}

// EvalAssignmentCompletedPayload is the payload for
// ai.eval.assignment.completed events.
type EvalAssignmentCompletedPayload struct {
	SchemaVersion  string                `json:"schema_version"`
	SourceSequence int64                 `json:"source_sequence"`
	EventID        string                `json:"event_id"`
	CycleID        string                `json:"cycle_id"`
	CampaignID     string                `json:"campaign_id"`
	AssignmentID   string                `json:"assignment_id"`
	VariantID      string                `json:"variant_id"`
	Role           ModelRole             `json:"role"`
	TaskID         string                `json:"task_id"`
	ArmID          string                `json:"arm_id"`
	Repetition     int                   `json:"repetition"`
	TerminalStatus TerminalOutcomeStatus `json:"terminal_status"`
	CompletedAt    time.Time             `json:"completed_at"`
	ObservedAt     time.Time             `json:"observed_at"`
}

// EvalModelRoleInvokedPayload is the payload for
// ai.eval.model_role.invoked events.
type EvalModelRoleInvokedPayload struct {
	SchemaVersion  string    `json:"schema_version"`
	SourceSequence int64     `json:"source_sequence"`
	EventID        string    `json:"event_id"`
	CycleID        string    `json:"cycle_id"`
	CampaignID     string    `json:"campaign_id"`
	AssignmentID   string    `json:"assignment_id"`
	VariantID      string    `json:"variant_id"`
	Role           ModelRole `json:"role"`
	ServedModelTag string    `json:"served_model_tag"`
	BackendName    string    `json:"backend_name"`
	Quantization   string    `json:"quantization,omitempty"`
	InvokedAt      time.Time `json:"invoked_at"`
	ObservedAt     time.Time `json:"observed_at"`
}

// EvalMetricAvailablePayload is the payload for ai.eval.metric.available
// events.
type EvalMetricAvailablePayload struct {
	SchemaVersion      string                 `json:"schema_version"`
	SourceSequence     int64                  `json:"source_sequence"`
	EventID            string                 `json:"event_id"`
	CycleID            string                 `json:"cycle_id"`
	CampaignID         string                 `json:"campaign_id"`
	AssignmentID       string                 `json:"assignment_id"`
	VariantID          string                 `json:"variant_id"`
	MetricID           string                 `json:"metric_id"`
	MetricVersion      string                 `json:"metric_version"`
	Numerator          int                    `json:"numerator"`
	Denominator        int                    `json:"denominator"`
	Rate               *float64               `json:"rate,omitempty"`
	Unit               string                 `json:"unit"`
	VerificationStatus EvalVerificationStatus `json:"verification_status"`
	AvailableAt        time.Time              `json:"available_at"`
	ObservedAt         time.Time              `json:"observed_at"`
}

// EvalVerifierCompletedPayload is the payload for
// ai.eval.verifier.completed events.
type EvalVerifierCompletedPayload struct {
	SchemaVersion               string                 `json:"schema_version"`
	SourceSequence              int64                  `json:"source_sequence"`
	EventID                     string                 `json:"event_id"`
	CycleID                     string                 `json:"cycle_id"`
	CampaignID                  string                 `json:"campaign_id"`
	VerificationStatus          EvalVerificationStatus `json:"verification_status"`
	VerifiedIndexGenerationHash string                 `json:"verified_index_generation_hash"`
	LayerCount                  int                    `json:"layer_count"`
	FailureCount                int                    `json:"failure_count"`
	CompletedAt                 time.Time              `json:"completed_at"`
	ObservedAt                  time.Time              `json:"observed_at"`
}

// EvalProofAvailablePayload is the payload for ai.eval.proof.available events.
type EvalProofAvailablePayload struct {
	SchemaVersion   string    `json:"schema_version"`
	SourceSequence  int64     `json:"source_sequence"`
	EventID         string    `json:"event_id"`
	CycleID         string    `json:"cycle_id"`
	CampaignID      string    `json:"campaign_id"`
	ProofRootSHA256 string    `json:"proof_root_sha256"`
	ArtifactCount   int       `json:"artifact_count"`
	AvailableAt     time.Time `json:"available_at"`
	ObservedAt      time.Time `json:"observed_at"`
}

// EvalPublicationCompletedPayload is the payload for
// ai.eval.publication.completed events.
type EvalPublicationCompletedPayload struct {
	SchemaVersion             string    `json:"schema_version"`
	SourceSequence            int64     `json:"source_sequence"`
	EventID                   string    `json:"event_id"`
	CycleID                   string    `json:"cycle_id"`
	CampaignID                string    `json:"campaign_id"`
	PublicationSchemaVersion  string    `json:"publication_schema_version"`
	PublishedProjectionSHA256 string    `json:"published_projection_sha256"`
	CompletedAt               time.Time `json:"completed_at"`
	ObservedAt                time.Time `json:"observed_at"`
}

// EvalHeartbeatPayload is the payload for ai.eval.heartbeat events. A
// heartbeat proves source liveness only, not that any specific work is
// progressing.
type EvalHeartbeatPayload struct {
	SchemaVersion  string    `json:"schema_version"`
	SourceSequence int64     `json:"source_sequence"`
	EventID        string    `json:"event_id"`
	SourceID       string    `json:"source_id"`
	ObservedAt     time.Time `json:"observed_at"`
}

// EvalStopRequestedPayload is the payload for ai.eval.stop.requested events.
type EvalStopRequestedPayload struct {
	SchemaVersion  string     `json:"schema_version"`
	SourceSequence int64      `json:"source_sequence"`
	EventID        string     `json:"event_id"`
	CampaignID     string     `json:"campaign_id"`
	StopReason     StopReason `json:"stop_reason"`
	StopScope      StopScope  `json:"stop_scope"`
	RequestedAt    time.Time  `json:"requested_at"`
	ObservedAt     time.Time  `json:"observed_at"`
}

// ---------------------------------------------------------------------------
// Live campaign projections (observe_api.json)
//
// These projections carry a freshness field using the closed vocabulary
// active, delayed, stale, intentionally_stopped, safety_stopped,
// source_offline. See protocol/models/observe_api.json for the canonical
// wire shapes.
// ---------------------------------------------------------------------------

// SupervisorStateProjection is the current lifecycle state of the continuous
// campaign supervisor. One supervisor per campaign.
type SupervisorStateProjection struct {
	SchemaVersion    string            `json:"schema_version"`
	SupervisorID     string            `json:"supervisor_id"`
	CampaignID       string            `json:"campaign_id"`
	CampaignRevision string            `json:"campaign_revision"`
	Status           SupervisorStatus  `json:"status"`
	CurrentCycleID   string            `json:"current_cycle_id,omitempty"`
	LastCycleID      string            `json:"last_cycle_id,omitempty"`
	StopReason       StopReason        `json:"stop_reason,omitempty"`
	Freshness        CampaignFreshness `json:"freshness"`
	ObservedAt       time.Time         `json:"observed_at"`
}

// CycleStateProjection is the current lifecycle state of a single campaign
// cycle. One cycle at a time per supervisor.
type CycleStateProjection struct {
	SchemaVersion      string                 `json:"schema_version"`
	CycleID            string                 `json:"cycle_id"`
	CampaignID         string                 `json:"campaign_id"`
	CampaignRevision   string                 `json:"campaign_revision"`
	RoleCombinationID  string                 `json:"role_combination_id"`
	Status             CampaignCycleStatus    `json:"status"`
	VerificationStatus EvalVerificationStatus `json:"verification_status,omitempty"`
	Freshness          CampaignFreshness      `json:"freshness"`
	StartedAt          *time.Time             `json:"started_at,omitempty"`
	CompletedAt        *time.Time             `json:"completed_at,omitempty"`
	ObservedAt         time.Time              `json:"observed_at"`
}

// AssignmentProgressProjection is the current progress of a single campaign
// assignment.
type AssignmentProgressProjection struct {
	SchemaVersion  string                   `json:"schema_version"`
	AssignmentID   string                   `json:"assignment_id"`
	CycleID        string                   `json:"cycle_id"`
	CampaignID     string                   `json:"campaign_id"`
	VariantID      string                   `json:"variant_id"`
	Role           ModelRole                `json:"role"`
	TaskID         string                   `json:"task_id"`
	ArmID          string                   `json:"arm_id"`
	Repetition     int                      `json:"repetition"`
	Status         AssignmentProgressStatus `json:"status"`
	TerminalStatus TerminalOutcomeStatus    `json:"terminal_status,omitempty"`
	Freshness      CampaignFreshness        `json:"freshness"`
	StartedAt      *time.Time               `json:"started_at,omitempty"`
	CompletedAt    *time.Time               `json:"completed_at,omitempty"`
	ObservedAt     time.Time                `json:"observed_at"`
}

// RoleCombinationProjection is the exact Primary/Assistant/Lite
// role-combination identity. Binds all three exact ModelVariant identities
// with served model tags, backends, and quantization from the provider
// boundary.
type RoleCombinationProjection struct {
	SchemaVersion         string    `json:"schema_version"`
	RoleCombinationID     string    `json:"role_combination_id"`
	CampaignID            string    `json:"campaign_id"`
	PrimaryVariantID      string    `json:"primary_variant_id"`
	AssistantVariantID    string    `json:"assistant_variant_id,omitempty"`
	LiteVariantID         string    `json:"lite_variant_id,omitempty"`
	PrimaryModelTag       string    `json:"primary_model_tag"`
	AssistantModelTag     string    `json:"assistant_model_tag,omitempty"`
	LiteModelTag          string    `json:"lite_model_tag,omitempty"`
	PrimaryBackend        string    `json:"primary_backend"`
	AssistantBackend      string    `json:"assistant_backend,omitempty"`
	LiteBackend           string    `json:"lite_backend,omitempty"`
	PrimaryQuantization   string    `json:"primary_quantization,omitempty"`
	AssistantQuantization string    `json:"assistant_quantization,omitempty"`
	LiteQuantization      string    `json:"lite_quantization,omitempty"`
	ObservedAt            time.Time `json:"observed_at"`
}

// VerificationProgressProjection is the current progress of the campaign
// verifier for a single cycle.
type VerificationProgressProjection struct {
	SchemaVersion               string                 `json:"schema_version"`
	CycleID                     string                 `json:"cycle_id"`
	CampaignID                  string                 `json:"campaign_id"`
	VerificationStatus          EvalVerificationStatus `json:"verification_status"`
	VerifiedIndexGenerationHash string                 `json:"verified_index_generation_hash,omitempty"`
	LayerCount                  int                    `json:"layer_count"`
	FailureCount                int                    `json:"failure_count"`
	Freshness                   CampaignFreshness      `json:"freshness"`
	CompletedAt                 *time.Time             `json:"completed_at,omitempty"`
	ObservedAt                  time.Time              `json:"observed_at"`
}

// PublicationProgressProjection is the current progress of the publication
// path for a single cycle.
type PublicationProgressProjection struct {
	SchemaVersion             string            `json:"schema_version"`
	CycleID                   string            `json:"cycle_id"`
	CampaignID                string            `json:"campaign_id"`
	PublicationStatus         PublicationStatus `json:"publication_status"`
	PublicationSchemaVersion  string            `json:"publication_schema_version,omitempty"`
	PublishedProjectionSHA256 string            `json:"published_projection_sha256,omitempty"`
	Freshness                 CampaignFreshness `json:"freshness"`
	CompletedAt               *time.Time        `json:"completed_at,omitempty"`
	ObservedAt                time.Time         `json:"observed_at"`
}

// SourceFreshnessProjection is the current freshness state of the campaign
// source. A heartbeat proves source liveness only, not that any specific
// work is progressing.
type SourceFreshnessProjection struct {
	SchemaVersion   string            `json:"schema_version"`
	SourceID        string            `json:"source_id"`
	CampaignID      string            `json:"campaign_id,omitempty"`
	Freshness       CampaignFreshness `json:"freshness"`
	LastHeartbeatAt time.Time         `json:"last_heartbeat_at"`
	ObservedAt      time.Time         `json:"observed_at"`
}
