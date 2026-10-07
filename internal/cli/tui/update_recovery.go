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
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleRecoveryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.recoveryTokenInput.Blur()
		m.view = viewOverview
		return m, nil
	case "tab":
		m.recoveryApprove = !m.recoveryApprove
		return m, nil
	case "enter":
		return m.confirmRecovery(), nil
	}
	var cmd tea.Cmd
	m.recoveryTokenInput, cmd = m.recoveryTokenInput.Update(msg)
	return m, cmd
}

func (m Model) confirmRecovery() Model {
	token := strings.TrimSpace(m.recoveryTokenInput.Value())
	if token == "" {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "Enter a CLI recovery token first"})
	}
	if m.gw == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Not connected to a Gateway"})
	}
	verb := "Approve"
	if !m.recoveryApprove {
		verb = "Deny"
	}
	gw := m.gw
	approve := m.recoveryApprove
	m.confirm = &confirmation{
		prompt: fmt.Sprintf("%s CLI recovery request %s? This sends the one-time token to the Gateway.", verb, shortHash(token)),
		run: func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			return gw.approveRecovery(ctx, token, approve)
		},
	}
	return m
}

func (m Model) applyRecoveryApprovedMsg(msg RecoveryApprovedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m.recoveryErr = msg.Err.Error()
		return m, nil
	}
	m.recoveryErr = ""
	m.recoveryState = msg.Response.State
	verb := "approved"
	level := LevelInfo
	if !msg.Approve {
		verb = "denied"
	}
	m = m.applyLedgerMsg(LedgerMsg{Level: level, Message: fmt.Sprintf("CLI recovery request %s.", verb)})
	return m, nil
}
