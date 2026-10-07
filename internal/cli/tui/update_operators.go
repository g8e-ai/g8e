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

	clioperator "github.com/g8e-ai/g8e/v2/internal/cli/operator"
)

// stopReason is recorded with every shutdown the TUI requests.
const stopReason = "stopped from g8e tui"

// handleOperatorsKey applies a key on the Operators view: s requests a
// governed shutdown of the selected Operator, after y/N.
func (m Model) handleOperatorsKey(key string) (tea.Model, tea.Cmd) {
	if m.view == viewOperatorDetails {
		switch key {
		case "up", "k":
			m.operatorDetailScroll = max(m.operatorDetailScroll-1, 0)
		case "down", "j":
			m.operatorDetailScroll++
		case "s":
			return m.confirmOperatorStop(), nil
		case "b":
			return m.confirmOperatorBind(), nil
		case "u":
			return m.confirmOperatorUnbind(), nil
		}
		return m, nil
	}

	switch key {
	case "up", "k":
		m.operatorsSelected = clamp(m.operatorsSelected-1, max(len(m.operators)-1, 0))
	case "down", "j":
		m.operatorsSelected = clamp(m.operatorsSelected+1, max(len(m.operators)-1, 0))
	case "s":
		return m.confirmOperatorStop(), nil
	case "b":
		return m.confirmOperatorBind(), nil
	case "u":
		return m.confirmOperatorUnbind(), nil
	case "enter":
		if m.operatorsSelected < len(m.operators) {
			m.view = viewOperatorDetails
			m.operatorDetailScroll = 0
		}
	}
	return m, nil
}

// confirmOperatorStop asks to stop the selected Operator. Only a remote
// Operator can be stopped; the embedded one is refused here with the same
// error 'g8e operator stop' returns.
func (m Model) confirmOperatorStop() Model {
	if m.operatorsSelected >= len(m.operators) {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "Select an Operator first"})
	}
	op := m.operators[m.operatorsSelected]
	if err := clioperator.CheckStoppable(op); err != nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("Cannot stop %s: %s", operatorHostname(op), err)})
	}
	if m.gw == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Not connected to a Gateway"})
	}
	gw := m.gw
	sessionID := op.OperatorSessionId
	m.confirm = &confirmation{
		prompt: fmt.Sprintf("Stop Operator %s (%s, session %s)? It receives a governed shutdown request and disconnects.",
			operatorHostname(op), shortHash(op.Id), shortHash(sessionID)),
		run: func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			return gw.stopOperator(ctx, sessionID, stopReason)
		},
	}
	return m
}

// confirmOperatorBind asks to bind the CLI session to the selected Operator.
func (m Model) confirmOperatorBind() Model {
	if m.operatorsSelected >= len(m.operators) {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "Select an Operator first"})
	}
	op := m.operators[m.operatorsSelected]
	if op.OperatorSessionId == "" {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Selected Operator has no session ID"})
	}
	if m.gw == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Not connected to a Gateway"})
	}
	gw := m.gw
	sessionID := op.OperatorSessionId
	m.confirm = &confirmation{
		prompt: fmt.Sprintf("Bind this CLI session to Operator %s (session %s)? The Gateway issues a replacement CLI session.", operatorHostname(op), shortHash(sessionID)),
		run: func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			return gw.bindOperator(ctx, sessionID)
		},
	}
	return m
}

// confirmOperatorUnbind asks to clear the current CLI session's binding.
func (m Model) confirmOperatorUnbind() Model {
	if m.gw == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Not connected to a Gateway"})
	}
	gw := m.gw
	m.confirm = &confirmation{
		prompt: "Clear this CLI session's Operator binding? The Gateway issues a replacement CLI session.",
		run: func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			return gw.unbindOperator(ctx)
		},
	}
	return m
}

func (m Model) applyOperatorBindMsg(msg OperatorBindMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Operator bind failed: " + msg.Err.Error()}), nil
	}
	identity := m.identity
	identity.UserID = msg.Response.UserID
	identity.CLISessionID = msg.Response.CLISessionID
	identity.OperatorID = msg.Response.OperatorID
	identity.OperatorSessionID = msg.Response.OperatorSessionID
	m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: fmt.Sprintf("CLI session bound to Operator session %s; rotating session", shortHash(msg.Response.OperatorSessionID))})
	if m.rebuildSession == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Session rotation is unavailable; restart the TUI to use the new binding"}), nil
	}
	return m, m.rebuildSessionCmd(identity)
}

func (m Model) applyOperatorUnbindMsg(msg OperatorUnbindMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Operator unbind failed: " + msg.Err.Error()}), nil
	}
	identity := m.identity
	identity.UserID = msg.Response.UserID
	identity.CLISessionID = msg.Response.CLISessionID
	identity.OperatorID = ""
	identity.OperatorSessionID = ""
	m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "CLI session Operator binding cleared; rotating session"})
	if m.rebuildSession == nil {
		return m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "Session rotation is unavailable; restart the TUI to use the unbound session"}), nil
	}
	return m, m.rebuildSessionCmd(identity)
}

// applyOperatorStopMsg reports a shutdown request and re-lists Operators.
func (m Model) applyOperatorStopMsg(msg OperatorStopMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m = m.noteSessionError(msg.Err)
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: fmt.Sprintf("Stopping operator session %s failed: %s", shortHash(msg.OperatorSessionID), msg.Err)})
	} else {
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: fmt.Sprintf("Stop requested for operator session %s (operator %s).", shortHash(msg.Response.OperatorSessionID), shortHash(msg.Response.OperatorID))})
	}
	if m.gw == nil {
		return m, nil
	}
	return m, withTimeout(m.gw.fetchOperators)
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
