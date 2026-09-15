// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import "encoding/json"

// EvalEngineRequestSchemaVersion is the versioned schema version the Go
// facade and the internal Python engine exchange. Bumped in lockstep with
// a contract test that asserts Go/Python parity.
const EvalEngineRequestSchemaVersion = "1.0.0"

// EvalOperation enumerates the engine-backed operations the Go facade
// dispatches to the internal Python engine. Read-only facade-only
// operations (setup, doctor, draft, plan, check, lease lifecycle) are not
// engine operations and are absent from this enum. The string values are
// the stable wire identifiers shared with Python through a contract-tested
// registry; do not hand-copy them without a parity test.
type EvalOperation string

const (
	EvalOperationDiagnosticStart             EvalOperation = "diagnostic_start"
	EvalOperationDiagnosticStatus            EvalOperation = "diagnostic_status"
	EvalOperationDiagnosticStop              EvalOperation = "diagnostic_stop"
	EvalOperationDiagnosticVerify            EvalOperation = "diagnostic_verify"
	EvalOperationCampaignStart               EvalOperation = "campaign_start"
	EvalOperationCampaignStatus              EvalOperation = "campaign_status"
	EvalOperationCampaignStop                EvalOperation = "campaign_stop"
	EvalOperationCampaignVerify              EvalOperation = "campaign_verify"
	EvalOperationCampaignPublish             EvalOperation = "campaign_publish"
	EvalOperationCampaignSetPlan             EvalOperation = "campaign_set_plan"
	EvalOperationCampaignSetValidate         EvalOperation = "campaign_set_validate"
	EvalOperationCampaignSetVerify           EvalOperation = "campaign_set_verify"
	EvalOperationControllerRun               EvalOperation = "controller_run"
	EvalOperationControllerStatus            EvalOperation = "controller_status"
	EvalOperationControllerStop              EvalOperation = "controller_stop"
	EvalOperationControllerRecover           EvalOperation = "controller_recover"
	EvalOperationBundle                      EvalOperation = "bundle"
	EvalOperationVerify                      EvalOperation = "verify"
	EvalOperationVerifyReceipts              EvalOperation = "verify_receipts"
	EvalOperationPublish                     EvalOperation = "publish"
	EvalOperationQualificationHashSource     EvalOperation = "qualification_hash_source"
	EvalOperationQualificationCandidate      EvalOperation = "qualification_candidate"
	EvalOperationQualificationCollectRuntime EvalOperation = "qualification_collect_runtime"
	EvalOperationQualificationRunGate        EvalOperation = "qualification_run_gate"
	EvalOperationQualificationBuild          EvalOperation = "qualification_build"
	EvalOperationBenchSynthetic              EvalOperation = "bench_synthetic"
)

// EvalEngineStatus is the terminal status the Python engine reports back.
type EvalEngineStatus string

const (
	EvalEngineStatusSucceeded   EvalEngineStatus = "succeeded"
	EvalEngineStatusFailed      EvalEngineStatus = "failed"
	EvalEngineStatusInterrupted EvalEngineStatus = "interrupted"
	EvalEngineStatusStopped     EvalEngineStatus = "stopped"
)

// EvalErrorCode is the stable error code registry shared between Go and
// Python. The Go facade maps a Python code to a Go sentinel in one
// adapter. EvalErrorCodeNone is the zero value for the success case.
type EvalErrorCode string

const (
	EvalErrorCodeNone                        EvalErrorCode = ""
	EvalErrorCodeEngineNotSetUp              EvalErrorCode = "engine_not_set_up"
	EvalErrorCodeEngineProtocolMismatch      EvalErrorCode = "engine_protocol_mismatch"
	EvalErrorCodeConfigInvalid               EvalErrorCode = "config_invalid"
	EvalErrorCodeAuthorityInvalid            EvalErrorCode = "authority_invalid"
	EvalErrorCodeLeaseMissing                EvalErrorCode = "lease_missing"
	EvalErrorCodeLeaseInactive               EvalErrorCode = "lease_inactive"
	EvalErrorCodeLeaseExpired                EvalErrorCode = "lease_expired"
	EvalErrorCodeLeaseMismatched             EvalErrorCode = "lease_mismatched"
	EvalErrorCodeLeaseConsumed               EvalErrorCode = "lease_consumed"
	EvalErrorCodeCandidateDrift              EvalErrorCode = "candidate_drift"
	EvalErrorCodeInventoryDrift              EvalErrorCode = "inventory_drift"
	EvalErrorCodeReportRootReused            EvalErrorCode = "report_root_reused"
	EvalErrorCodeEvidenceKeyInvalid          EvalErrorCode = "evidence_key_invalid"
	EvalErrorCodePlatformIdentityUnavailable EvalErrorCode = "platform_identity_unavailable"
	EvalErrorCodePlatformUnhealthy           EvalErrorCode = "platform_unhealthy"
	EvalErrorCodeProviderUnreachable         EvalErrorCode = "provider_unreachable"
	EvalErrorCodeBudgetPreflightFailed       EvalErrorCode = "budget_preflight_failed"
	EvalErrorCodeChildStartFailed            EvalErrorCode = "child_start_failed"
	EvalErrorCodeChildExitNonZero            EvalErrorCode = "child_exit_non_zero"
	EvalErrorCodeChildInterrupted            EvalErrorCode = "child_interrupted"
	EvalErrorCodeStatusReconciliationFailed  EvalErrorCode = "status_reconciliation_failed"
)

// EvalPlatformContext carries platform-owned identity and paths the facade
// injects from the selected repository/runtime context. Configs never
// carry these fields; the facade is their only source. The CLI auth
// identity fields are populated from the canonical local credentials for
// provider-backed starts so the engine child never reads G8E_* auth
// variables from the process environment; they are empty for read-only
// lifecycle operations that never authenticate to the platform.
type EvalPlatformContext struct {
	RepositoryRoot      string `json:"repository_root"`
	EvalProject         string `json:"eval_project"`
	G8EBinaryPath       string `json:"g8e_binary_path"`
	G8EBinarySHA256     string `json:"g8e_binary_sha256"`
	PlatformVersion     string `json:"platform_version"`
	AuthProjectRoot     string `json:"auth_project_root"`
	RuntimeDir          string `json:"runtime_dir"`
	TrustBundlePath     string `json:"trust_bundle_path"`
	GatewayHTTPURL      string `json:"gateway_http_url"`
	GatewayHTTPSURL     string `json:"gateway_https_url"`
	EnsembleURL         string `json:"ensemble_url"`
	CLICertPath         string `json:"cli_cert_path"`
	CLIKeyPath          string `json:"cli_key_path"`
	OperatorSessionID   string `json:"operator_session_id"`
	CLISessionID        string `json:"cli_session_id"`
	UserID              string `json:"user_id"`
	OperatorID          string `json:"operator_id"`
	SourceRevision      string `json:"source_revision"`
	SourceTreeStateHash string `json:"source_tree_state_hash"`
}

// EvalEngineFlags carries non-secret engine flags. Secret values
// (provider API keys, evidence-key bytes) are never placed here; they
// remain in the supported environment or OS secret mechanism and are
// inherited by the child process environment.
type EvalEngineFlags struct {
	JSONOutput    bool    `json:"json_output,omitempty"`
	Verbose       bool    `json:"verbose,omitempty"`
	IdleTimeoutS  float64 `json:"idle_timeout_s,omitempty"`
	ImmediateStop bool    `json:"immediate_stop,omitempty"`
}

// EvalEngineRequest is the typed contract the Go facade sends to the
// internal Python engine. It is the only engine entry shape; the facade
// never reconstructs a giant shell vector. The request is passed as a
// typed JSON file path argument, not positional reconstruction.
type EvalEngineRequest struct {
	SchemaVersion string              `json:"schema_version"`
	Operation     EvalOperation       `json:"operation"`
	OperationID   string              `json:"operation_id"`
	Revision      string              `json:"revision"`
	ConfigPath    string              `json:"config_path"`
	LeasePath     string              `json:"lease_path"`
	ReportRoot    string              `json:"report_root"`
	Platform      EvalPlatformContext `json:"platform"`
	Flags         EvalEngineFlags     `json:"flags,omitempty"`
}

// EvalEngineResult is the typed contract the Python engine returns. The
// facade translates ErrorCode/Stage into consistent g8e errors without
// parsing human prose. Payload is an operation-specific typed JSON object
// selected by Operation.
type EvalEngineResult struct {
	SchemaVersion string           `json:"schema_version"`
	Operation     EvalOperation    `json:"operation"`
	OperationID   string           `json:"operation_id"`
	Status        EvalEngineStatus `json:"status"`
	ErrorCode     EvalErrorCode    `json:"error_code,omitempty"`
	ErrorStage    string           `json:"error_stage,omitempty"`
	SafeDetail    string           `json:"safe_detail,omitempty"`
	Payload       json.RawMessage  `json:"payload,omitempty"`
}
