// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Update implements tea.Model. It routes messages to state transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.ledgerScroll < len(m.ledger)-1 {
				m.ledgerScroll++
			}
		case "down", "j":
			if m.ledgerScroll > 0 {
				m.ledgerScroll--
			}
		case "g":
			m.ledgerScroll = len(m.ledger) - 1
			if m.ledgerScroll < 0 {
				m.ledgerScroll = 0
			}
		case "G":
			m.ledgerScroll = 0
		}

	case PipelineMsg:
		m = m.applyPipelineMsg(msg)
		if m.hasBlinkingState() {
			return m, tick()
		}

	case LedgerMsg:
		m = m.applyLedgerMsg(msg)

	case ConsensusMsg:
		m = m.applyConsensusMsg(msg)

	case ConnStatusMsg:
		m.connStatus = msg.Status
		m.connDetail = msg.Detail

	case PendingApprovalsMsg:
		m = m.applyPendingApprovalsMsg(msg)
		if m.hasBlinkingState() {
			return m, tick()
		}

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

// applyPendingApprovalsMsg reconciles the pending L3 approval set: each newly
// pending transaction is announced in the ledger with the CLI command that
// approves it, and the L3 stage reflects whether anything is still waiting.
func (m Model) applyPendingApprovalsMsg(msg PendingApprovalsMsg) Model {
	if msg.Err != nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Pending approvals refresh failed: " + msg.Err.Error()})
	}

	next := make(map[string]struct{}, len(msg.Transactions))
	for _, tx := range msg.Transactions {
		next[tx.TransactionHash] = struct{}{}
		if _, seen := m.pending[tx.TransactionHash]; seen {
			continue
		}
		tool := tx.ToolName
		if tool == "" {
			tool = "transaction"
		}
		m = m.applyLedgerMsg(LedgerMsg{
			Level:   LevelWarn,
			Message: fmt.Sprintf("APPROVAL REQUIRED: %s (tx %s) — run 'g8e auth approve %s'", tool, shortHash(tx.TransactionHash), tx.TransactionHash),
		})
	}
	m.pending = next

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

// applyConsensusMsg updates a consensus member's vote state and the overall
// consensus result.
func (m Model) applyConsensusMsg(msg ConsensusMsg) Model {
	for i, member := range m.consensus {
		if member.name == msg.Member {
			m.consensus[i].decision = msg.Decision
			m.consensus[i].signed = msg.Signed
			break
		}
	}
	if msg.Quorum > 0 {
		m.quorum = msg.Quorum
	}
	if msg.Total > 0 {
		m.total = msg.Total
	}
	if msg.Result != ConsensusPending {
		m.result = msg.Result
	}
	if msg.Hash != "" {
		m.consensusHash = msg.Hash
	}
	return m
}
