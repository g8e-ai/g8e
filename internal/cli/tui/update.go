// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/v2/internal/cli/api"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
)

// requestTimeout bounds each Gateway request issued from a key press.
const requestTimeout = 10 * time.Second

// Update implements tea.Model. It routes messages to state transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case PipelineMsg:
		m = m.applyPipelineMsg(msg)
		if m.hasBlinkingState() {
			return m, tick()
		}

	case LedgerMsg:
		m = m.applyLedgerMsg(msg)

	case ConnStatusMsg:
		m.connStatus = msg.Status
		m.connDetail = msg.Detail

	case PendingApprovalsMsg:
		m = m.applyPendingApprovalsMsg(msg)
		if m.hasBlinkingState() {
			return m, tick()
		}

	case OperatorsMsg:
		m = m.applyOperatorsMsg(msg)

	case HealthMsg:
		m = m.applyHealthMsg(msg)

	case EnrollmentsMsg:
		m = m.applyEnrollmentsMsg(msg)

	case EnrollmentDecidedMsg:
		return m.applyEnrollmentDecidedMsg(msg)

	case EnrollmentRevokedMsg:
		return m.applyEnrollmentRevokedMsg(msg)

	case OperatorStopMsg:
		return m.applyOperatorStopMsg(msg)

	case OperatorBindMsg:
		return m.applyOperatorBindMsg(msg)

	case OperatorUnbindMsg:
		return m.applyOperatorUnbindMsg(msg)

	case RecoveryApprovedMsg:
		return m.applyRecoveryApprovedMsg(msg)

	case SessionRotatedMsg:
		return m.applySessionRotatedMsg(msg)

	case AuditEventsMsg:
		m = m.applyAuditEventsMsg(msg)

	case AuditSummaryMsg:
		m = m.applyAuditSummaryMsg(msg)

	case AuditVerifyMsg:
		m = m.applyAuditVerifyMsg(msg)

	case ApprovalOpenedMsg:
		m = m.applyApprovalOpenedMsg(msg)

	case ApprovalCompletedMsg:
		m = m.applyPipelineMsg(PipelineMsg{Stage: StageL3, Status: StatusPassed, TxID: msg.TxHash, Detail: "Approval completed"})
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "Approval completed for tx " + msg.TxHash + " — verifying"})
		if m.gw != nil && msg.TxHash != "" {
			return m, m.verifyApprovalCmd(msg.TxHash)
		}

	case ApprovalVerifiedMsg:
		return m.applyApprovalVerifiedMsg(msg)

	case ScenarioCompleteMsg:
		level := LevelCritical
		checkpoint := "scenario-status-unknown"
		switch msg.Status {
		case ScenarioSucceeded:
			level = LevelInfo
			checkpoint = "scenario-succeeded"
		case ScenarioFailed:
			checkpoint = "scenario-failed"
		case ScenarioCancelled:
			level = LevelWarn
			checkpoint = "scenario-cancelled"
		}
		m = m.applyLedgerMsg(LedgerMsg{Level: level, Message: "PRESENTATION CHECKPOINT: " + checkpoint})

	case TickMsg:
		m.blinkOn = !m.blinkOn
		if m.hasBlinkingState() {
			return m, tick()
		}
		return m, nil

	case QuitMsg:
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

// handleKey applies a key press. An open confirmation takes every key, the
// help overlay closes on ?/esc, and global keys (quit, view switch, refresh)
// apply before the active view's keys.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}
	if m.confirm != nil {
		return m.handleConfirmKey(msg)
	}
	if m.showHelp {
		switch key {
		case "?", "esc":
			m.showHelp = false
		case "q":
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	}
	if m.view == viewRecovery {
		return m.handleRecoveryKey(msg)
	}
	switch key {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "r":
		return m, m.refreshCmd()
	case "esc":
		if m.view == viewOperatorDetails {
			m.view = viewOperators
			m.operatorDetailScroll = 0
		}
		return m, nil
	}
	if id, ok := viewForKey(key); ok {
		m.view = id
		if id == viewAudit {
			return m, m.auditRefreshCmd()
		}
		if id == viewGatewayStatus {
			return m, m.refreshCmd()
		}
		if id == viewRecovery {
			cmd := m.recoveryTokenInput.Focus()
			return m, cmd
		}
		return m, nil
	}
	switch m.view {
	case viewApprovals:
		return m.handleApprovalsKey(key)
	case viewOperators, viewOperatorDetails:
		return m.handleOperatorsKey(key)
	case viewEnrollments:
		return m.handleEnrollmentsKey(key)
	case viewAudit:
		return m.handleAuditKey(key)
	case viewGatewayStatus:
		return m.handleGatewayStatusKey(key)
	default:
		return m.handleOverviewKey(key)
	}
}

// handleOverviewKey applies a key on the overview. Movement keys act on the
// focused pane.
func (m Model) handleOverviewKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		m.focus = (m.focus + 1) % paneCount
	case "shift+tab":
		m.focus = (m.focus + paneCount - 1) % paneCount
	case "up", "k":
		m = m.moveSelection(-1)
	case "down", "j":
		m = m.moveSelection(1)
	case "g":
		if m.focus == paneLedger {
			m.ledgerScroll = max(len(m.ledger)-1, 0)
		}
	case "G":
		if m.focus == paneLedger {
			m.ledgerScroll = 0
		}
	case "a":
		return m.approveSelected()
	case "enter":
		if m.focus == paneApprovals {
			return m.approveSelected()
		}
	}
	return m, nil
}

// moveSelection moves within the focused pane: older/newer ledger lines, or
// the previous/next pending approval or Operator.
func (m Model) moveSelection(delta int) Model {
	switch m.focus {
	case paneLedger:
		// Up (delta -1) scrolls toward older entries.
		m.ledgerScroll = clamp(m.ledgerScroll-delta, max(len(m.ledger)-1, 0))
	case paneApprovals:
		m.pendingSelected = clamp(m.pendingSelected+delta, max(len(m.pending)-1, 0))
	case paneOperators:
		m.operatorsSelected = clamp(m.operatorsSelected+delta, max(len(m.operators)-1, 0))
	}
	return m
}

// refreshCmd re-fetches the active session data, Gateway health, and audit
// data when the Audit view is active.
func (m Model) refreshCmd() tea.Cmd {
	if m.gw == nil {
		return nil
	}
	gw := m.gw
	cmds := []tea.Cmd{
		withTimeout(gw.fetchPendingApprovals),
		withTimeout(gw.fetchOperators),
		withTimeout(gw.fetchHealth),
		withTimeout(gw.fetchEnrollments),
	}
	if m.view == viewAudit {
		cmds = append(cmds, m.auditRefreshCmds()...)
	}
	return tea.Batch(cmds...)
}

// withTimeout adapts a Gateway fetch to a tea.Cmd bounded by requestTimeout.
func withTimeout[T tea.Msg](fetch func(context.Context) T) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return fetch(ctx)
	}
}

// noteSessionError marks the connection failed when err is a Gateway 401, so
// an expired CLI session shows the same recovery as the rest of the CLI.
func (m Model) noteSessionError(err error) Model {
	if api.IsUnauthorized(err) {
		m.connStatus = ConnFailed
		m.connDetail = sessionExpiredDetail
	}
	return m
}

// applySessionRotatedMsg replaces the request gateway and tells the adapter
// to restart its SSE stream on the same fresh session.
func (m Model) applySessionRotatedMsg(msg SessionRotatedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "CLI session rebuild failed: " + msg.Err.Error()}), nil
	}
	if msg.Session == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "CLI session rebuild returned no session"}), nil
	}
	m.identity = msg.Identity
	m.gw = &gateway{session: msg.Session, userID: msg.Identity.UserID}
	if m.sessionManager != nil {
		m.sessionManager.replace(msg.Session, msg.Identity.UserID)
	}
	m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "CLI session rotated; reconnecting SSE"})
	return m, m.refreshCmd()
}

func (m Model) rebuildSessionCmd(identity Identity) tea.Cmd {
	if m.rebuildSession == nil {
		return nil
	}
	rebuild := m.rebuildSession
	return withTimeout(func(ctx context.Context) SessionRotatedMsg {
		session, err := rebuild(ctx, identity)
		return SessionRotatedMsg{Session: session, Identity: identity, Err: err}
	})
}

// applyPipelineMsg updates a pipeline stage's status and detail.
func (m Model) applyPipelineMsg(msg PipelineMsg) Model {
	idx := int(msg.Stage)
	if idx < 0 || idx >= len(m.pipeline) {
		return m
	}
	m.pipeline[idx].status = msg.Status
	m.pipeline[idx].detail = msg.Detail
	if msg.TxID != "" {
		m.activeTx = msg.TxID
	}
	return m
}

// applyHealthMsg records the Gateway's governance posture.
func (m Model) applyHealthMsg(msg HealthMsg) Model {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Gateway health unavailable: " + msg.Err.Error()})
	}
	m.gatewayVersion = msg.Health.Version
	m.health = msg.Health
	posture, err := governance.ParseGovernancePosture(msg.Health.Posture)
	if err != nil {
		m.posture = nil
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Gateway reported an unknown posture: " + err.Error()})
	}
	m.posture = posture
	return m
}

// applyLedgerMsg appends a ledger entry to the buffer.
func (m Model) applyLedgerMsg(msg LedgerMsg) Model {
	entry := ledgerEntry{
		level:   msg.Level,
		message: msg.Message,
		time:    msg.Time,
	}
	if entry.time.IsZero() {
		entry.time = timeNow()
	}
	m.ledger = append(m.ledger, entry)
	return m
}

// toolLabel names a suspended transaction's tool for display.
func toolLabel(tool string) string {
	if tool == "" {
		return "transaction"
	}
	return tool
}

// clamp bounds v to [0, hi].
func clamp(v, hi int) int {
	return min(max(v, 0), hi)
}
