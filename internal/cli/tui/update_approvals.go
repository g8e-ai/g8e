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

	tea "github.com/charmbracelet/bubbletea"
)

// handleApprovalsKey applies a key on the approvals view.
func (m Model) handleApprovalsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.pendingSelected = clamp(m.pendingSelected-1, max(len(m.pending)-1, 0))
	case "down", "j":
		m.pendingSelected = clamp(m.pendingSelected+1, max(len(m.pending)-1, 0))
	case "a", "enter":
		return m.approveSelected()
	}
	return m, nil
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

// verifyApprovalCmd confirms a completed approval over mTLS.
func (m Model) verifyApprovalCmd(txHash string) tea.Cmd {
	gw := m.gw
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return gw.verifyApproval(ctx, txHash)
	}
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
