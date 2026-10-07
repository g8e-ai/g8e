// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// PipelineStage identifies a layer in the 5-layer verification gauntlet.
type PipelineStage int

const (
	StageL1 PipelineStage = iota
	StageL2
	StageL3
	StageL4
	StageL5
)

func (s PipelineStage) String() string {
	switch s {
	case StageL1:
		return "L1: Doctrine"
	case StageL2:
		return "L2: Consensus"
	case StageL3:
		return "L3: Notary"
	case StageL4:
		return "L4: Warden"
	case StageL5:
		return "L5: Actuator"
	default:
		return "Unknown"
	}
}

// PipelineStatus represents the visual state of a pipeline stage.
type PipelineStatus int

const (
	StatusIdle PipelineStatus = iota
	StatusActive
	StatusWaiting
	StatusPassed
	StatusFailed
)

func (s PipelineStatus) String() string {
	switch s {
	case StatusIdle:
		return "Pending"
	case StatusActive:
		return "Processing"
	case StatusWaiting:
		return "AWAITING FIDO2 KEY..."
	case StatusPassed:
		return "Passed"
	case StatusFailed:
		return "BLOCKED"
	default:
		return "Unknown"
	}
}

// LedgerLevel controls the color and tag of a ledger entry.
type LedgerLevel int

const (
	LevelInfo LedgerLevel = iota
	LevelWarn
	LevelCritical
)

func (l LedgerLevel) Tag() string {
	switch l {
	case LevelInfo:
		return "[INFO]"
	case LevelWarn:
		return "[WARN]"
	case LevelCritical:
		return "[CRIT]"
	default:
		return "[UNKNOWN]"
	}
}

// PipelineMsg advances or updates a pipeline stage in the TUI.
type PipelineMsg struct {
	Stage  PipelineStage
	Status PipelineStatus
	TxID   string
	Detail string
}

// LedgerMsg appends a line to the Sovereign Audit Ledger pane.
type LedgerMsg struct {
	Level   LedgerLevel
	Message string
	Time    time.Time
}

// ConnStatus represents the SSE adapter connection state.
type ConnStatus int

const (
	ConnIdle ConnStatus = iota
	ConnConnecting
	ConnConnected
	ConnReconnecting
	ConnFailed
)

func (c ConnStatus) String() string {
	switch c {
	case ConnIdle:
		return "IDLE"
	case ConnConnecting:
		return "CONNECTING"
	case ConnConnected:
		return "CONNECTED"
	case ConnReconnecting:
		return "RECONNECTING"
	case ConnFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// ConnStatusMsg updates the TUI's connection status indicator.
type ConnStatusMsg struct {
	Status ConnStatus
	Detail string
}

// ApprovalCompletedMsg reports an approval.completed event for a suspended
// L3 transaction. The event alone is not proof: the model verifies the
// transaction's status over mTLS before reporting it approved.
type ApprovalCompletedMsg struct {
	TxHash string
}

// ApprovalOpenedMsg reports that the browser WebAuthn approval page for a
// pending transaction was opened (or could not be, in which case the user is
// pointed at URL).
type ApprovalOpenedMsg struct {
	TxHash string
	URL    string
	Err    error
}

// ApprovalVerifiedMsg carries the mTLS-verified status of a transaction after
// approval.completed, or the error that prevented or failed verification.
type ApprovalVerifiedMsg struct {
	TxHash string
	Status models.ApprovalStatusResponse
	Err    error
}

// OperatorsMsg carries the session user's Operators as listed by the Gateway,
// or the error that prevented listing them.
type OperatorsMsg struct {
	Operators []*operatorv1.OperatorDocument
	Err       error
}

// HealthMsg carries the Gateway's health report, including its immutable
// governance posture, or the error that prevented fetching it.
type HealthMsg struct {
	Health models.HealthResponse
	Err    error
}

// PendingApprovalsMsg carries the session user's pending L3 transactions as
// listed by the Gateway, or the error that prevented listing them.
type PendingApprovalsMsg struct {
	Transactions []models.SuspendedTxResponse
	Err          error
}

// EnrollmentsMsg carries the pending platform enrollment requests and the
// completed enrollments as listed by the Gateway, or the error that prevented
// listing them. Only the platform owner may list them.
type EnrollmentsMsg struct {
	Pending  []models.PlatformEnrollmentPendingRequest
	Enrolled []models.PlatformEnrollmentEnrolledRequest
	Err      error
}

// EnrollmentDecidedMsg carries the Gateway's response to an owner decision on
// a pending platform enrollment request, or the error that failed it.
type EnrollmentDecidedMsg struct {
	RequestID string
	Decision  models.PlatformEnrollmentDecision
	Response  *models.PlatformEnrollmentDecisionResponse
	Err       error
}

// EnrollmentRevokedMsg carries the Gateway's response to revoking a completed
// platform enrollment, or the error that failed it.
type EnrollmentRevokedMsg struct {
	RequestID string
	Response  *models.PlatformEnrollmentRevokeResponse
	Err       error
}

// OperatorStopMsg carries the Gateway's response to a governed shutdown
// request for a remote Operator, or the error that failed it.
type OperatorStopMsg struct {
	OperatorSessionID string
	Response          models.StopOperatorResponse
	Err               error
}

// OperatorBindMsg carries a replacement CLI session after binding to an
// Operator, or the error that prevented binding.
type OperatorBindMsg struct {
	Response auth.CLISessionBind
	Err      error
}

// OperatorUnbindMsg carries a replacement CLI session after clearing the
// Operator binding, or the error that prevented unbinding.
type OperatorUnbindMsg struct {
	Response auth.CLISessionUnbind
	Err      error
}

// RecoveryApprovedMsg carries the Gateway response to an approve-recovery
// action, or the error that prevented it.
type RecoveryApprovedMsg struct {
	Approve  bool
	Response models.CLIRecoveryApproveResponse
	Err      error
}

// SessionRotatedMsg carries a freshly rebuilt CLI session and authoritative
// identity after a Gateway action replaces the session credentials.
type SessionRotatedMsg struct {
	Session  Session
	Identity Identity
	Err      error
}

// AuditEventsMsg carries one page of Gateway audit events, or the error that
// prevented listing them.
type AuditEventsMsg struct {
	Events []models.AuditEventRow
	Count  int
	Offset int
	Err    error
}

// AuditSummaryMsg carries the aggregate Gateway audit summary, or the error
// that prevented fetching it.
type AuditSummaryMsg struct {
	Summary models.AuditSummaryResponse
	Err     error
}

// AuditVerifyMsg carries the result of verifying the Gateway audit chain, or
// the error that prevented the verification request.
type AuditVerifyMsg struct {
	Verify models.AuditVerifyResponse
	Err    error
}

// ScenarioStatus represents the terminal state of a demo scenario run.
type ScenarioStatus int

const (
	ScenarioUnknown ScenarioStatus = iota
	ScenarioSucceeded
	ScenarioFailed
	ScenarioCancelled
)

// ScenarioCompleteMsg reports a machine-observable demo scenario completion.
type ScenarioCompleteMsg struct {
	Status ScenarioStatus
	Detail string
}

// TickMsg drives the blink/pulse animation for waiting and failed states.
type TickMsg struct{}

// QuitMsg signals the TUI to exit cleanly.
type QuitMsg struct{}

// pipelineStageState is the per-stage state held in the Model.
type pipelineStageState struct {
	status PipelineStatus
	detail string
}

// ledgerEntry is a single line in the Sovereign Audit Ledger.
type ledgerEntry struct {
	level   LedgerLevel
	message string
	time    time.Time
}
