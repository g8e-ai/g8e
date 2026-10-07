// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/api"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestNewModel(t *testing.T) {
	t.Run("defaults to 5 idle pipeline stages", func(t *testing.T) {
		m := NewModel(Options{})
		assert.Len(t, m.pipeline, 5)
		for i, stage := range m.pipeline {
			assert.Equal(t, StatusIdle, stage.status, "stage %d should be idle", i)
		}
	})

	t.Run("starts with empty ledger, no posture, and ledger focus", func(t *testing.T) {
		m := NewModel(Options{})
		assert.Empty(t, m.ledger)
		assert.Nil(t, m.posture)
		assert.Equal(t, paneLedger, m.focus)
	})

	t.Run("has gateway access only with a session", func(t *testing.T) {
		assert.Nil(t, NewModel(Options{}).gw)
		m := NewModel(Options{Session: &testSession{}, Identity: Identity{UserID: "user-1"}})
		require.NotNil(t, m.gw)
		assert.Equal(t, "user-1", m.gw.userID)
	})
}

func TestSessionRotationReplacesRequestsAndSSESession(t *testing.T) {
	oldSession := &testSession{}
	newSession := &testSession{}
	manager := newSessionManager(oldSession, "user-old")
	m := NewModel(Options{
		Session:        oldSession,
		Identity:       Identity{UserID: "user-old", CLISessionID: "cli-old"},
		sessionManager: manager,
	})

	model, _ := m.Update(SessionRotatedMsg{
		Session:  newSession,
		Identity: Identity{UserID: "user-new", CLISessionID: "cli-new"},
	})
	m = model.(Model)

	assert.Equal(t, "user-new", m.identity.UserID)
	assert.Same(t, newSession, m.gw.session)
	gw, _, _ := manager.snapshot()
	assert.Same(t, newSession, gw.session)
	assert.Contains(t, m.ledger[len(m.ledger)-1].message, "rotated")
}

func TestApplyPipelineMsg(t *testing.T) {
	tests := []struct {
		name       string
		stage      PipelineStage
		status     PipelineStatus
		txID       string
		detail     string
		wantStage  int
		wantStatus PipelineStatus
		wantDetail string
		wantTxID   string
	}{
		{
			name:       "L1 active with tx",
			stage:      StageL1,
			status:     StatusActive,
			txID:       "tx-abc123",
			detail:     "doctrine check",
			wantStage:  0,
			wantStatus: StatusActive,
			wantDetail: "doctrine check",
			wantTxID:   "tx-abc123",
		},
		{
			name:       "L3 waiting for FIDO2",
			stage:      StageL3,
			status:     StatusWaiting,
			txID:       "",
			detail:     "FIDO2 touch required",
			wantStage:  2,
			wantStatus: StatusWaiting,
			wantDetail: "FIDO2 touch required",
		},
		{
			name:       "L1 failed PII block",
			stage:      StageL1,
			status:     StatusFailed,
			txID:       "tx-bad",
			detail:     "PII EGRESS BLOCKED",
			wantStage:  0,
			wantStatus: StatusFailed,
			wantDetail: "PII EGRESS BLOCKED",
			wantTxID:   "tx-bad",
		},
		{
			name:       "L5 passed",
			stage:      StageL5,
			status:     StatusPassed,
			txID:       "tx-ok",
			detail:     "actuator committed",
			wantStage:  4,
			wantStatus: StatusPassed,
			wantDetail: "actuator committed",
			wantTxID:   "tx-ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(Options{})
			msg := PipelineMsg{
				Stage:  tt.stage,
				Status: tt.status,
				TxID:   tt.txID,
				Detail: tt.detail,
			}
			m = m.applyPipelineMsg(msg)

			assert.Equal(t, tt.wantStatus, m.pipeline[tt.wantStage].status)
			assert.Equal(t, tt.wantDetail, m.pipeline[tt.wantStage].detail)
			if tt.wantTxID != "" {
				assert.Equal(t, tt.wantTxID, m.activeTx)
			}
		})
	}

	t.Run("does not update activeTx when TxID is empty", func(t *testing.T) {
		m := NewModel(Options{})
		m.activeTx = "existing-tx"
		m = m.applyPipelineMsg(PipelineMsg{Stage: StageL2, Status: StatusActive, TxID: ""})
		assert.Equal(t, "existing-tx", m.activeTx)
	})

	t.Run("ignores out-of-range stage index", func(t *testing.T) {
		m := NewModel(Options{})
		msg := PipelineMsg{Stage: PipelineStage(99), Status: StatusActive}
		m = m.applyPipelineMsg(msg)
		for _, stage := range m.pipeline {
			assert.Equal(t, StatusIdle, stage.status)
		}
	})
}

func TestApplyLedgerMsg(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	originalTimeNow := timeNow
	t.Cleanup(func() { timeNow = originalTimeNow })
	timeNow = func() time.Time { return fixedTime }

	t.Run("appends entry with provided time", func(t *testing.T) {
		m := NewModel(Options{})
		msgTime := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "test entry", Time: msgTime})
		require.Len(t, m.ledger, 1)
		assert.Equal(t, "test entry", m.ledger[0].message)
		assert.Equal(t, LevelInfo, m.ledger[0].level)
		assert.Equal(t, msgTime, m.ledger[0].time)
	})

	t.Run("uses timeNow when time is zero", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelWarn, Message: "auto time", Time: time.Time{}})
		require.Len(t, m.ledger, 1)
		assert.Equal(t, fixedTime, m.ledger[0].time)
	})

	t.Run("retains all entries without trimming", func(t *testing.T) {
		m := NewModel(Options{})
		for i := 0; i < 505; i++ {
			m = m.applyLedgerMsg(LedgerMsg{
				Level:   LevelInfo,
				Message: "entry",
				Time:    fixedTime.Add(time.Duration(i) * time.Second),
			})
		}
		require.Len(t, m.ledger, 505)
		assert.Equal(t, fixedTime, m.ledger[0].time)
		assert.Equal(t, fixedTime.Add(504*time.Second), m.ledger[504].time)
	})
}

func TestApplyPendingApprovalsMsg(t *testing.T) {
	m := NewModel(Options{})
	pending := PendingApprovalsMsg{Transactions: []models.SuspendedTxResponse{{TransactionHash: "tx-aaaaaaaaaaaaaaaaaaaa", ToolName: "run_command"}}}

	m = m.applyPendingApprovalsMsg(pending)
	assert.Equal(t, StatusWaiting, m.pipeline[StageL3].status)
	assert.Equal(t, "tx-aaaaaaaaaaaaaaaaaaaa", m.activeTx)
	require.Len(t, m.ledger, 1)
	assert.Contains(t, m.ledger[0].message, "g8e auth approve tx-aaaaaaaaaaaaaaaaaaaa")

	m = m.applyPendingApprovalsMsg(pending)
	assert.Len(t, m.ledger, 1, "an already-announced approval must not be logged again")

	m = m.applyPendingApprovalsMsg(PendingApprovalsMsg{})
	assert.Equal(t, StatusIdle, m.pipeline[StageL3].status, "L3 returns to idle once nothing is pending")

	m = m.applyPendingApprovalsMsg(PendingApprovalsMsg{Err: constants.ErrHTTPStatusError})
	require.Len(t, m.ledger, 2)
	assert.Equal(t, LevelWarn, m.ledger[1].level)
}

func pendingTxs(hashes ...string) PendingApprovalsMsg {
	msg := PendingApprovalsMsg{}
	for _, h := range hashes {
		msg.Transactions = append(msg.Transactions, models.SuspendedTxResponse{TransactionHash: h, ToolName: "run_command"})
	}
	return msg
}

func press(t *testing.T, m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	model, cmd := m.Update(key)
	return model.(Model), cmd
}

var (
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyA     = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	keyR     = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}
)

func TestPendingApprovalSelection(t *testing.T) {
	t.Run("tab focuses approvals and j/k select within them", func(t *testing.T) {
		m := NewModel(Options{}).applyPendingApprovalsMsg(pendingTxs("tx-1", "tx-2", "tx-3"))
		m, _ = press(t, m, keyTab)
		assert.Equal(t, paneApprovals, m.focus)

		m, _ = press(t, m, keyDown)
		m, _ = press(t, m, keyDown)
		m, _ = press(t, m, keyDown)
		assert.Equal(t, 2, m.pendingSelected, "selection stops at the last row")
		assert.Equal(t, 0, m.ledgerScroll, "moving in approvals must not scroll the ledger")
	})

	t.Run("tab cycles through every pane", func(t *testing.T) {
		m := NewModel(Options{})
		for range paneCount {
			m, _ = press(t, m, keyTab)
		}
		assert.Equal(t, paneLedger, m.focus)
		m, _ = press(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
		assert.Equal(t, paneOperators, m.focus)
	})

	t.Run("selection follows the selected transaction across refreshes", func(t *testing.T) {
		m := NewModel(Options{}).applyPendingApprovalsMsg(pendingTxs("tx-1", "tx-2"))
		m.pendingSelected = 1
		m = m.applyPendingApprovalsMsg(pendingTxs("tx-0", "tx-1", "tx-2"))
		assert.Equal(t, 2, m.pendingSelected)
		m = m.applyPendingApprovalsMsg(pendingTxs("tx-0"))
		assert.Equal(t, 0, m.pendingSelected, "selection resets when the transaction is gone")
	})
}

func TestApproveFlow(t *testing.T) {
	t.Run("a opens the approval page for the selected transaction", func(t *testing.T) {
		var opened string
		m := NewModel(Options{
			Session:     &testSession{},
			ApprovalURL: func(tx string) string { return "https://gw/approve/" + tx },
			OpenBrowser: func(url string) error { opened = url; return nil },
		}).applyPendingApprovalsMsg(pendingTxs("tx-1", "tx-2"))
		m.pendingSelected = 1

		m, cmd := press(t, m, keyA)
		require.NotNil(t, cmd)
		msg := cmd()
		assert.Equal(t, ApprovalOpenedMsg{TxHash: "tx-2", URL: "https://gw/approve/tx-2"}, msg)
		assert.Equal(t, "https://gw/approve/tx-2", opened)

		model, _ := m.Update(msg)
		m = model.(Model)
		assert.Contains(t, m.awaitingApproval, "tx-2")
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "https://gw/approve/tx-2")
	})

	t.Run("enter approves only when the approvals pane has focus", func(t *testing.T) {
		m := NewModel(Options{
			ApprovalURL: func(tx string) string { return tx },
			OpenBrowser: func(string) error { return nil },
		}).applyPendingApprovalsMsg(pendingTxs("tx-1"))
		_, cmd := press(t, m, keyEnter)
		assert.Nil(t, cmd)
		m, _ = press(t, m, keyTab)
		_, cmd = press(t, m, keyEnter)
		assert.NotNil(t, cmd)
	})

	t.Run("a browser failure points the user at the URL", func(t *testing.T) {
		m := NewModel(Options{})
		model, _ := m.Update(ApprovalOpenedMsg{TxHash: "tx-1", URL: "https://gw/approve/tx-1", Err: constants.ErrNotFound})
		m = model.(Model)
		last := m.ledger[len(m.ledger)-1]
		assert.Equal(t, LevelWarn, last.level)
		assert.Contains(t, last.message, "https://gw/approve/tx-1")
	})

	t.Run("without the approve flow, a points at the CLI command", func(t *testing.T) {
		m := NewModel(Options{}).applyPendingApprovalsMsg(pendingTxs("tx-1"))
		m, cmd := press(t, m, keyA)
		assert.Nil(t, cmd)
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "g8e auth approve tx-1")
	})

	t.Run("with nothing pending, a does nothing but say so", func(t *testing.T) {
		m, cmd := press(t, NewModel(Options{}), keyA)
		assert.Nil(t, cmd)
		assert.Contains(t, m.ledger[0].message, "No pending approvals")
	})

	t.Run("approval.completed is verified over mTLS, then pending is re-listed", func(t *testing.T) {
		session := &testSession{statusJSON: `{"status":"approved","tool_name":"run_command"}`}
		m := NewModel(Options{Session: session})
		m.awaitingApproval["tx-1"] = struct{}{}

		model, cmd := m.Update(ApprovalCompletedMsg{TxHash: "tx-1"})
		m = model.(Model)
		assert.Equal(t, StatusPassed, m.pipeline[StageL3].status)
		require.NotNil(t, cmd)
		verified, ok := cmd().(ApprovalVerifiedMsg)
		require.True(t, ok)
		require.NoError(t, verified.Err)
		assert.Contains(t, session.requestedPaths(), constants.APIPaths.ApprovalsCLIStatus+"tx-1")

		model, cmd = m.Update(verified)
		m = model.(Model)
		assert.NotContains(t, m.awaitingApproval, "tx-1")
		assert.Contains(t, m.ledger[len(m.ledger)-1].message, "approved (run_command)")
		require.NotNil(t, cmd)
		_, ok = cmd().(PendingApprovalsMsg)
		assert.True(t, ok, "a verified approval re-lists pending approvals")
	})

	t.Run("a non-approved status is reported as a failure", func(t *testing.T) {
		m := NewModel(Options{Session: &testSession{statusJSON: `{"status":"expired_or_not_found"}`}})
		_, cmd := m.Update(ApprovalCompletedMsg{TxHash: "tx-1"})
		verified := cmd().(ApprovalVerifiedMsg)
		require.Error(t, verified.Err)

		model, _ := m.Update(verified)
		m = model.(Model)
		last := m.ledger[len(m.ledger)-1]
		assert.Equal(t, LevelWarn, last.level)
		assert.Contains(t, last.message, "expired or not found")
	})
}

func TestApplyOperatorsMsg(t *testing.T) {
	m := NewModel(Options{})
	m = m.applyOperatorsMsg(OperatorsMsg{Operators: []*operatorv1.OperatorDocument{
		{Id: "op-1", Status: string(constants.OperatorStatusActive)},
		{Id: "op-2", Status: string(constants.OperatorStatusOffline)},
		{Id: "slot", IsSlot: true, Status: string(constants.OperatorStatusAvailable)},
	}})
	require.Len(t, m.operators, 1, "only connected Operators are listed, as in 'g8e gw status'")
	assert.Equal(t, "op-1", m.operators[0].Id)
	assert.Equal(t, 3, m.operatorsTotal)
	assert.True(t, m.operatorsLoaded)

	m = m.applyOperatorsMsg(OperatorsMsg{Err: constants.ErrHTTPStatusError})
	assert.NotEmpty(t, m.operatorsErr)
	assert.Len(t, m.operators, 1, "a failed refresh keeps the last list")
}

func TestApplyHealthMsg(t *testing.T) {
	m := NewModel(Options{})
	m = m.applyHealthMsg(HealthMsg{Health: models.HealthResponse{Posture: constants.PostureRatify, Version: "v2.3.2"}})
	require.NotNil(t, m.posture)
	assert.Equal(t, constants.PostureRatify, m.posture.Name())
	assert.Equal(t, "v2.3.2", m.gatewayVersion)

	m = m.applyHealthMsg(HealthMsg{Health: models.HealthResponse{Posture: "bogus"}})
	assert.Nil(t, m.posture)
	assert.Equal(t, LevelWarn, m.ledger[len(m.ledger)-1].level)
}

func TestSessionExpiry(t *testing.T) {
	expired := &api.StatusError{StatusCode: http.StatusUnauthorized, Body: "CLI session expired"}
	for name, msg := range map[string]tea.Msg{
		"pending approvals": PendingApprovalsMsg{Err: expired},
		"operators":         OperatorsMsg{Err: expired},
		"health":            HealthMsg{Err: expired},
	} {
		t.Run(name, func(t *testing.T) {
			model, _ := NewModel(Options{}).Update(msg)
			m := model.(Model)
			assert.Equal(t, ConnFailed, m.connStatus)
			assert.Contains(t, m.connDetail, "g8e auth refresh")
		})
	}

	t.Run("other errors leave the connection state alone", func(t *testing.T) {
		m := NewModel(Options{})
		m.connStatus = ConnConnected
		model, _ := m.Update(PendingApprovalsMsg{Err: &api.StatusError{StatusCode: http.StatusInternalServerError}})
		assert.Equal(t, ConnConnected, model.(Model).connStatus)
	})
}

func TestRefreshKey(t *testing.T) {
	t.Run("r re-fetches approvals, operators, and health", func(t *testing.T) {
		session := &testSession{}
		_, cmd := press(t, NewModel(Options{Session: session}), keyR)
		require.NotNil(t, cmd)
		batch, ok := cmd().(tea.BatchMsg)
		require.True(t, ok)
		for _, c := range batch {
			c()
		}
		paths := session.requestedPaths()
		assert.Contains(t, paths, constants.APIPaths.ApprovalsCLIList)
		assert.Contains(t, paths, constants.APIPaths.Health)
		assert.Contains(t, strings.Join(paths, " "), constants.APIPaths.Operators)
	})

	t.Run("r without a session does nothing", func(t *testing.T) {
		_, cmd := press(t, NewModel(Options{}), keyR)
		assert.Nil(t, cmd)
	})
}

func TestTickMsgBlinkToggle(t *testing.T) {
	t.Run("toggles blink and reschedules when blinking state exists", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyPipelineMsg(PipelineMsg{Stage: StageL3, Status: StatusWaiting, Detail: "FIDO2"})
		assert.False(t, m.blinkOn)

		model, cmd := m.Update(TickMsg{})
		m = model.(Model)
		assert.True(t, m.blinkOn)
		assert.NotNil(t, cmd)

		model, _ = m.Update(TickMsg{})
		m = model.(Model)
		assert.False(t, m.blinkOn)
	})

	t.Run("toggles blink but does not reschedule when no blinking state", func(t *testing.T) {
		m := NewModel(Options{})
		assert.False(t, m.blinkOn)

		model, cmd := m.Update(TickMsg{})
		m = model.(Model)
		assert.True(t, m.blinkOn)
		assert.Nil(t, cmd)
	})
}

func TestKeyMsgScroll(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	originalTimeNow := timeNow
	t.Cleanup(func() { timeNow = originalTimeNow })
	timeNow = func() time.Time { return fixedTime }

	t.Run("up/k increases scroll offset", func(t *testing.T) {
		m := NewModel(Options{})
		for i := 0; i < 10; i++ {
			m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "entry", Time: fixedTime})
		}
		assert.Equal(t, 0, m.ledgerScroll)

		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = model.(Model)
		assert.Equal(t, 1, m.ledgerScroll)

		model, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = model.(Model)
		assert.Equal(t, 2, m.ledgerScroll)
	})

	t.Run("down/j decreases scroll offset", func(t *testing.T) {
		m := NewModel(Options{})
		for i := 0; i < 10; i++ {
			m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "entry", Time: fixedTime})
		}
		m.ledgerScroll = 3

		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = model.(Model)
		assert.Equal(t, 2, m.ledgerScroll)
	})

	t.Run("down does not go below zero", func(t *testing.T) {
		m := NewModel(Options{})
		m.ledgerScroll = 0
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = model.(Model)
		assert.Equal(t, 0, m.ledgerScroll)
	})

	t.Run("up does not exceed ledger length minus one", func(t *testing.T) {
		m := NewModel(Options{})
		for i := 0; i < 3; i++ {
			m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "entry", Time: fixedTime})
		}
		for i := 0; i < 10; i++ {
			model, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
			m = model.(Model)
		}
		assert.Equal(t, 2, m.ledgerScroll)
	})

	t.Run("g jumps to top (max scroll)", func(t *testing.T) {
		m := NewModel(Options{})
		for i := 0; i < 10; i++ {
			m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "entry", Time: fixedTime})
		}
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = model.(Model)
		assert.Equal(t, 1, m.ledgerScroll)

		model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
		m = model.(Model)
		assert.Equal(t, 9, m.ledgerScroll)
	})

	t.Run("G jumps to bottom (zero scroll)", func(t *testing.T) {
		m := NewModel(Options{})
		for i := 0; i < 10; i++ {
			m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "entry", Time: fixedTime})
		}
		m.ledgerScroll = 5
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
		m = model.(Model)
		assert.Equal(t, 0, m.ledgerScroll)
	})
}

func TestQuitKeyMsg(t *testing.T) {
	t.Run("q sets quitting and returns tea.Quit", func(t *testing.T) {
		m := NewModel(Options{})
		model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		m = model.(Model)
		assert.True(t, m.quitting)
		assert.NotNil(t, cmd)
	})

	t.Run("ctrl+c sets quitting and returns tea.Quit", func(t *testing.T) {
		m := NewModel(Options{})
		model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		m = model.(Model)
		assert.True(t, m.quitting)
		assert.NotNil(t, cmd)
	})
}

func TestQuitMsg(t *testing.T) {
	m := NewModel(Options{})
	model, cmd := m.Update(QuitMsg{})
	m = model.(Model)
	assert.True(t, m.quitting)
	assert.NotNil(t, cmd)
}

func TestScenarioCompleteMsg_AppendsTypedPresentationCheckpoint(t *testing.T) {
	tests := []struct {
		name    string
		status  ScenarioStatus
		level   LedgerLevel
		message string
	}{
		{name: "unknown status", status: ScenarioUnknown, level: LevelCritical, message: "PRESENTATION CHECKPOINT: scenario-status-unknown"},
		{name: "successful scenario", status: ScenarioSucceeded, level: LevelInfo, message: "PRESENTATION CHECKPOINT: scenario-succeeded"},
		{name: "failed scenario", status: ScenarioFailed, level: LevelCritical, message: "PRESENTATION CHECKPOINT: scenario-failed"},
		{name: "cancelled scenario", status: ScenarioCancelled, level: LevelWarn, message: "PRESENTATION CHECKPOINT: scenario-cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, _ := NewModel(Options{}).Update(ScenarioCompleteMsg{Status: tt.status})
			m := model.(Model)
			require.Len(t, m.ledger, 1)
			assert.Equal(t, tt.level, m.ledger[0].level)
			assert.Equal(t, tt.message, m.ledger[0].message)
		})
	}
}

func TestWindowSizeMsg(t *testing.T) {
	m := NewModel(Options{})
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = model.(Model)
	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)
}

func TestShortHash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "short hash", input: "abc123", want: "abc123"},
		{name: "exactly 8 chars", input: "12345678", want: "12345678"},
		{name: "long hash truncated", input: "abcdef1234567890abcdef", want: "abcdef12..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shortHash(tt.input))
		})
	}
}

func TestViewInit(t *testing.T) {
	m := NewModel(Options{})
	cmd := m.Init()
	assert.NotNil(t, cmd)
}

func TestHasBlinkingState(t *testing.T) {
	t.Run("false when all stages idle", func(t *testing.T) {
		m := NewModel(Options{})
		assert.False(t, m.hasBlinkingState())
	})

	t.Run("true when a stage is waiting", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyPipelineMsg(PipelineMsg{Stage: StageL3, Status: StatusWaiting})
		assert.True(t, m.hasBlinkingState())
	})

	t.Run("true when a stage is failed", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyPipelineMsg(PipelineMsg{Stage: StageL1, Status: StatusFailed})
		assert.True(t, m.hasBlinkingState())
	})

	t.Run("false when stages are passed or active", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyPipelineMsg(PipelineMsg{Stage: StageL1, Status: StatusPassed})
		m = m.applyPipelineMsg(PipelineMsg{Stage: StageL2, Status: StatusActive})
		assert.False(t, m.hasBlinkingState())
	})
}

func TestPipelineMsgSchedulesTickForBlinkingState(t *testing.T) {
	t.Run("schedules tick when waiting status introduced", func(t *testing.T) {
		m := NewModel(Options{})
		model, cmd := m.Update(PipelineMsg{Stage: StageL3, Status: StatusWaiting, Detail: "FIDO2"})
		m = model.(Model)
		assert.Equal(t, StatusWaiting, m.pipeline[2].status)
		assert.NotNil(t, cmd)
	})

	t.Run("does not schedule tick when no blinking status", func(t *testing.T) {
		m := NewModel(Options{})
		model, cmd := m.Update(PipelineMsg{Stage: StageL1, Status: StatusActive, Detail: "processing"})
		m = model.(Model)
		assert.Equal(t, StatusActive, m.pipeline[0].status)
		assert.Nil(t, cmd)
	})
}

func TestViewQuitting(t *testing.T) {
	m := NewModel(Options{})
	m.quitting = true
	assert.Equal(t, "", m.View())
}

func TestViewRenders(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	out := m.View()
	assert.NotEmpty(t, out)
}
