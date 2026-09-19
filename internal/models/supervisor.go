// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software
// is released under the Apache License, Version 2.0.

package models

import "time"

// SupervisorSchemaVersion is the schema version for supervisor-domain models.
const SupervisorSchemaVersion = "1.0.0"

// ContinuousRunSpec is the typed specification for a continuous campaign run.
// It references immutable campaign-profile and model-registry hashes and
// declares exact Primary/Assistant/Lite combinations, benchmarks, arms,
// repetitions, cycle budget, global budget ceiling, cooldown/backoff,
// publication policy, and safety-stop policy. A stable role-combination ID
// binds all three exact ModelVariant identities. The spec is frozen at Start
// time; subsequent resume operations reload the same spec without
// rerandomizing the schedule.
type ContinuousRunSpec struct {
	SchemaVersion          string                `json:"schema_version"`
	SupervisorID           string                `json:"supervisor_id"`
	CampaignID             string                `json:"campaign_id"`
	CampaignRevision       string                `json:"campaign_revision"`
	CampaignProfileHash    string                `json:"campaign_profile_hash"`
	ModelRegistryHash      string                `json:"model_registry_hash"`
	RoleCombinations       []RoleCombinationSpec `json:"role_combinations"`
	CycleBudget            int                   `json:"cycle_budget"`
	GlobalBudgetUSD        float64               `json:"global_budget_usd"`
	CooldownSeconds        int                   `json:"cooldown_seconds"`
	BackoffPolicy          BackoffPolicy         `json:"backoff_policy"`
	PublicationPolicy      PublicationPolicySpec `json:"publication_policy"`
	SafetyStopPolicy       SafetyStopPolicy      `json:"safety_stop_policy"`
	DiskReserveBytes       int64                 `json:"disk_reserve_bytes"`
	ProviderBudgetPerCycle float64               `json:"provider_budget_per_cycle"`
	ScheduleSeed           string                `json:"schedule_seed"`
}

// RoleCombinationSpec is one exact Primary/Assistant/Lite role combination
// in a continuous run specification. Each variant ID references an immutable
// ModelVariant in the frozen model registry.
type RoleCombinationSpec struct {
	RoleCombinationID  string `json:"role_combination_id"`
	PrimaryVariantID   string `json:"primary_variant_id"`
	AssistantVariantID string `json:"assistant_variant_id,omitempty"`
	LiteVariantID      string `json:"lite_variant_id,omitempty"`
}

// BackoffPolicy defines the bounded retry backoff for provider and mirror
// retries. InitialSeconds is the first backoff, Multiplier scales each
// subsequent retry, and MaxSeconds caps the backoff.
type BackoffPolicy struct {
	InitialSeconds int     `json:"initial_seconds"`
	MaxSeconds     int     `json:"max_seconds"`
	Multiplier     float64 `json:"multiplier"`
}

// PublicationPolicySpec defines the publication requirements for each cycle.
// A cycle rolls over to the next only after publication completes
// successfully when RequiredVerification is true.
type PublicationPolicySpec struct {
	RequiredVerification bool `json:"required_verification"`
	RequiredDisclosure   bool `json:"required_disclosure"`
	MaxRetries           int  `json:"max_retries"`
}

// SafetyStopPolicy enables or disables each safety-stop class. All classes
// default to enabled; a spec may disable a class only with explicit owner
// intent. Disabled classes are still recorded as diagnostics but do not stop
// the supervisor.
type SafetyStopPolicy struct {
	BudgetEnabled              bool `json:"budget_enabled"`
	DiskEnabled                bool `json:"disk_enabled"`
	HardwareDriftEnabled       bool `json:"hardware_drift_enabled"`
	CredentialEnabled          bool `json:"credential_enabled"`
	ProfileMismatchEnabled     bool `json:"profile_mismatch_enabled"`
	IneffectiveSettingsEnabled bool `json:"ineffective_settings_enabled"`
	VerifierEnabled            bool `json:"verifier_enabled"`
	DisclosureEnabled          bool `json:"disclosure_enabled"`
	MirrorAuthEnabled          bool `json:"mirror_auth_enabled"`
	PublicationEnabled         bool `json:"publication_enabled"`
}

// CycleLedgerEntry is one entry in the hash-linked cycle ledger. Each entry
// carries a cycle_hash computed from the entry's canonical content and a
// parent_cycle_hash linking to the previous entry, forming an append-only
// chain. The ledger is persisted to disk and read on resume to reconstruct
// the cycle history without rerunning completed cycles.
type CycleLedgerEntry struct {
	SchemaVersion      string                 `json:"schema_version"`
	CycleID            string                 `json:"cycle_id"`
	CampaignID         string                 `json:"campaign_id"`
	CampaignRevision   string                 `json:"campaign_revision"`
	RoleCombinationID  string                 `json:"role_combination_id"`
	CycleNumber        int                    `json:"cycle_number"`
	Status             CampaignCycleStatus    `json:"status"`
	ParentCycleHash    string                 `json:"parent_cycle_hash,omitempty"`
	CycleHash          string                 `json:"cycle_hash"`
	StartedAt          time.Time              `json:"started_at"`
	CompletedAt        *time.Time             `json:"completed_at,omitempty"`
	VerificationStatus EvalVerificationStatus `json:"verification_status,omitempty"`
	PublicationStatus  PublicationStatus      `json:"publication_status,omitempty"`
	TerminalAttempts   int                    `json:"terminal_attempts,omitempty"`
	AssignedTasks      int                    `json:"assigned_tasks,omitempty"`
	StopReason         StopReason             `json:"stop_reason,omitempty"`
}

// CycleResult is the result of executing a single campaign cycle through the
// CampaignRunner interface. The supervisor uses this to determine whether to
// proceed to verification, publication, or emit a safety stop.
type CycleResult struct {
	CycleID            string
	Status             CampaignCycleStatus
	VerificationStatus EvalVerificationStatus
	PublicationStatus  PublicationStatus
	TerminalAttempts   int
	AssignedTasks      int
	ReportPath         string
	Err                error
}

// SafetyStopResult is the result of a safety stop check. If Triggered is true,
// the supervisor transitions to SafetyStopped with the given reason.
type SafetyStopResult struct {
	Triggered bool
	Reason    StopReason
	Message   string
}

// SupervisorSnapshot is the read-only status response returned by the
// supervisor's Status operation. It carries the current supervisor state, the
// persisted spec, and the cycle ledger.
type SupervisorSnapshot struct {
	Spec           ContinuousRunSpec  `json:"spec"`
	Status         SupervisorStatus   `json:"status"`
	CurrentCycleID string             `json:"current_cycle_id,omitempty"`
	LastCycleID    string             `json:"last_cycle_id,omitempty"`
	StopReason     StopReason         `json:"stop_reason,omitempty"`
	CycleCount     int                `json:"cycle_count"`
	LastCycleHash  string             `json:"last_cycle_hash,omitempty"`
	Ledger         []CycleLedgerEntry `json:"ledger"`
	ObservedAt     time.Time          `json:"observed_at"`
}
