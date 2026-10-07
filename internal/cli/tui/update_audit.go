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

// handleAuditKey applies movement and read-only actions on the Audit view.
func (m Model) handleAuditKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.auditSelected = clamp(m.auditSelected-1, max(len(m.auditEvents)-1, 0))
	case "down", "j":
		m.auditSelected = clamp(m.auditSelected+1, max(len(m.auditEvents)-1, 0))
	case "n":
		if m.auditEventsCount < auditPageSize {
			return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "Already at the newest audit events"}), nil
		}
		m.auditOffset += auditPageSize
		m.auditSelected = 0
		return m, m.auditEventsCmd()
	case "p":
		if m.auditOffset == 0 {
			return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "Already at the oldest audit events"}), nil
		}
		m.auditOffset = max(m.auditOffset-auditPageSize, 0)
		m.auditSelected = 0
		return m, m.auditEventsCmd()
	case "v":
		if m.gw == nil {
			return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Not connected to a Gateway"}), nil
		}
		gw := m.gw
		return m, withTimeout(func(ctx context.Context) AuditVerifyMsg {
			return gw.verifyAudit(ctx, 0)
		})
	}
	return m, nil
}

func (m Model) auditRefreshCmd() tea.Cmd {
	cmds := m.auditRefreshCmds()
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m Model) auditRefreshCmds() []tea.Cmd {
	if m.gw == nil {
		return nil
	}
	return []tea.Cmd{m.auditEventsCmd(), withTimeout(m.gw.fetchAuditSummary)}
}

func (m Model) auditEventsCmd() tea.Cmd {
	gw := m.gw
	if gw == nil {
		return nil
	}
	offset := m.auditOffset
	return withTimeout(func(ctx context.Context) AuditEventsMsg {
		return gw.fetchAuditEvents(ctx, offset)
	})
}

func (m Model) applyAuditEventsMsg(msg AuditEventsMsg) Model {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m.auditEventsErr = msg.Err.Error()
		return m
	}
	m.auditEventsErr = ""
	m.auditEventsLoaded = true
	m.auditEvents = msg.Events
	m.auditEventsCount = msg.Count
	m.auditOffset = max(msg.Offset, 0)
	m.auditSelected = clamp(m.auditSelected, max(len(m.auditEvents)-1, 0))
	return m
}

func (m Model) applyAuditSummaryMsg(msg AuditSummaryMsg) Model {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m.auditSummaryErr = msg.Err.Error()
		return m
	}
	m.auditSummaryErr = ""
	m.auditSummaryLoaded = true
	m.auditSummary = msg.Summary
	return m
}

func (m Model) applyAuditVerifyMsg(msg AuditVerifyMsg) Model {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m.auditVerifyErr = msg.Err.Error()
		return m
	}
	m.auditVerifyErr = ""
	m.auditVerify = &msg.Verify
	if msg.Verify.OK {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: fmt.Sprintf("Audit chain verified through seq %d", msg.Verify.HeadSeq)})
	}
	return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Audit chain verification failed: " + msg.Verify.Error})
}
