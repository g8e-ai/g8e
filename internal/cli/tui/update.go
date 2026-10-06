// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/v2/internal/cli/api"
	clioperator "github.com/g8e-ai/g8e/v2/internal/cli/operator"
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

// handleKey applies a key press. Movement keys act on the focused pane.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
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
	case "r":
		return m, m.refreshCmd()
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

// approveSelected starts the browser WebAuthn approval of the selected
// pending transaction: the same page 'g8e auth approve' opens. Completion
// arrives as approval.completed on the TUI's existing stream.
func (m Model) approveSelected() (tea.Model, tea.Cmd) {
	if len(m.pending) == 0 {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "No pending approvals to approve"}), nil
	}
	if m.approvalURL == nil || m.openBrowser == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Approving from the TUI is unavailable; run 'g8e auth approve " + m.pending[m.pendingSelected].TransactionHash + "'"}), nil
	}
	txHash := m.pending[m.pendingSelected].TransactionHash
	url := m.approvalURL(txHash)
	open := m.openBrowser
	return m, func() tea.Msg {
		return ApprovalOpenedMsg{TxHash: txHash, URL: url, Err: open(url)}
	}
}

// refreshCmd re-fetches pending approvals, Operators, and Gateway health.
func (m Model) refreshCmd() tea.Cmd {
	if m.gw == nil {
		return nil
	}
	gw := m.gw
	return tea.Batch(
		withTimeout(gw.fetchPendingApprovals),
		withTimeout(gw.fetchOperators),
		withTimeout(gw.fetchHealth),
	)
}

// verifyApprovalCmd confirms a completed approval over mTLS.
func (m Model) verifyApprovalCmd(txHash string) tea.Cmd {
	gw := m.gw
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return gw.verifyApproval(ctx, txHash)
	}
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

// applyPendingApprovalsMsg reconciles the pending L3 approval queue: each
// newly pending transaction is announced in the ledger with the CLI command
// that approves it, the selection follows the previously selected
// transaction, and the L3 stage reflects whether anything is still waiting.
func (m Model) applyPendingApprovalsMsg(msg PendingApprovalsMsg) Model {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Pending approvals refresh failed: " + msg.Err.Error()})
	}

	known := make(map[string]struct{}, len(m.pending))
	for _, tx := range m.pending {
		known[tx.TransactionHash] = struct{}{}
	}
	selectedHash := ""
	if m.pendingSelected < len(m.pending) {
		selectedHash = m.pending[m.pendingSelected].TransactionHash
	}

	m.pendingSelected = 0
	for i, tx := range msg.Transactions {
		if tx.TransactionHash == selectedHash {
			m.pendingSelected = i
		}
		if _, seen := known[tx.TransactionHash]; seen {
			continue
		}
		m = m.applyLedgerMsg(LedgerMsg{
			Level:   LevelWarn,
			Message: fmt.Sprintf("APPROVAL REQUIRED: %s (tx %s) — press 'a' or run 'g8e auth approve %s'", toolLabel(tx.ToolName), shortHash(tx.TransactionHash), tx.TransactionHash),
		})
	}
	m.pending = msg.Transactions

	l3 := &m.pipeline[StageL3]
	switch {
	case len(msg.Transactions) > 0:
		l3.status = StatusWaiting
		l3.detail = fmt.Sprintf("%d pending approval(s)", len(msg.Transactions))
		m.activeTx = msg.Transactions[0].TransactionHash
	case l3.status == StatusWaiting:
		l3.status = StatusIdle
		l3.detail = ""
	}
	return m
}

// applyApprovalOpenedMsg records a browser approval started from the TUI.
func (m Model) applyApprovalOpenedMsg(msg ApprovalOpenedMsg) Model {
	m.awaitingApproval[msg.TxHash] = struct{}{}
	if msg.Err != nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("Could not open a browser (%s); approve tx %s at %s", msg.Err, shortHash(msg.TxHash), msg.URL)})
	}
	return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: fmt.Sprintf("Opened browser for WebAuthn approval of tx %s: %s", shortHash(msg.TxHash), msg.URL)})
}

// applyApprovalVerifiedMsg reports the verified outcome and re-lists pending
// approvals.
func (m Model) applyApprovalVerifiedMsg(msg ApprovalVerifiedMsg) (tea.Model, tea.Cmd) {
	delete(m.awaitingApproval, msg.TxHash)
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Approval verification failed: " + msg.Err.Error()})
	} else {
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: fmt.Sprintf("✓ Transaction %s approved (%s)", shortHash(msg.TxHash), toolLabel(msg.Status.ToolName))})
	}
	if m.gw == nil {
		return m, nil
	}
	return m, withTimeout(m.gw.fetchPendingApprovals)
}

// applyOperatorsMsg keeps the connected Operators, using the same
// connectivity rule as 'g8e gw status'.
func (m Model) applyOperatorsMsg(msg OperatorsMsg) Model {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m.operatorsErr = msg.Err.Error()
		return m
	}
	m.operatorsErr = ""
	m.operatorsLoaded = true
	m.operatorsTotal = len(msg.Operators)
	m.operators = m.operators[:0:0]
	for _, op := range msg.Operators {
		if clioperator.IsConnected(op) {
			m.operators = append(m.operators, op)
		}
	}
	m.operatorsSelected = clamp(m.operatorsSelected, max(len(m.operators)-1, 0))
	return m
}

// applyHealthMsg records the Gateway's governance posture.
func (m Model) applyHealthMsg(msg HealthMsg) Model {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Gateway health unavailable: " + msg.Err.Error()})
	}
	m.gatewayVersion = msg.Health.Version
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
