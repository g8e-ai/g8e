// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestNewModel(t *testing.T) {
	t.Run("defaults to 5 idle pipeline stages", func(t *testing.T) {
		m := NewModel(Options{})
		assert.Len(t, m.pipeline, 5)
		for i, stage := range m.pipeline {
			assert.Equal(t, StatusIdle, stage.status, "stage %d should be idle", i)
		}
	})

	t.Run("defaults quorum and total when zero", func(t *testing.T) {
		m := NewModel(Options{})
		assert.Equal(t, 3, m.quorum)
		assert.Equal(t, 5, m.total)
	})

	t.Run("uses provided quorum and total", func(t *testing.T) {
		m := NewModel(Options{Quorum: 4, Total: 7})
		assert.Equal(t, 4, m.quorum)
		assert.Equal(t, 7, m.total)
	})

	t.Run("initializes 5 consensus members", func(t *testing.T) {
		m := NewModel(Options{})
		assert.Len(t, m.consensus, 5)
		for _, member := range m.consensus {
			assert.False(t, member.signed, "member %s should be unsigned", member.name)
			assert.False(t, member.decision, "member %s should not have decision", member.name)
		}
	})

	t.Run("starts with empty ledger", func(t *testing.T) {
		m := NewModel(Options{})
		assert.Empty(t, m.ledger)
	})

	t.Run("consensus starts pending", func(t *testing.T) {
		m := NewModel(Options{})
		assert.Equal(t, ConsensusPending, m.result)
	})
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

func TestApplyConsensusMsg(t *testing.T) {
	t.Run("updates member vote to approve", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyConsensusMsg(ConsensusMsg{
			Member:   constants.ConsensusMemberAxiom,
			Decision: true,
			Signed:   true,
		})
		assert.Equal(t, constants.ConsensusMemberAxiom, m.consensus[0].name)
		assert.True(t, m.consensus[0].decision)
		assert.True(t, m.consensus[0].signed)
	})

	t.Run("updates member vote to veto", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyConsensusMsg(ConsensusMsg{
			Member:   constants.ConsensusMemberPragma,
			Decision: false,
			Signed:   true,
		})
		assert.Equal(t, constants.ConsensusMemberPragma, m.consensus[3].name)
		assert.False(t, m.consensus[3].decision)
		assert.True(t, m.consensus[3].signed)
	})

	t.Run("updates quorum and total", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyConsensusMsg(ConsensusMsg{
			Member: constants.ConsensusMemberAxiom,
			Quorum: 4,
			Total:  5,
		})
		assert.Equal(t, 4, m.quorum)
		assert.Equal(t, 5, m.total)
	})

	t.Run("updates result to rejected", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyConsensusMsg(ConsensusMsg{
			Member: constants.ConsensusMemberPragma,
			Result: ConsensusRejected,
			Hash:   "abcdef1234567890",
		})
		assert.Equal(t, ConsensusRejected, m.result)
		assert.Equal(t, "abcdef1234567890", m.consensusHash)
	})

	t.Run("does not overwrite result with pending", func(t *testing.T) {
		m := NewModel(Options{})
		m.result = ConsensusReached
		m = m.applyConsensusMsg(ConsensusMsg{
			Member: constants.ConsensusMemberAxiom,
			Result: ConsensusPending,
		})
		assert.Equal(t, ConsensusReached, m.result)
	})

	t.Run("ignores unknown member name", func(t *testing.T) {
		m := NewModel(Options{})
		m = m.applyConsensusMsg(ConsensusMsg{
			Member:   constants.ConsensusMember("unknown"),
			Decision: true,
			Signed:   true,
		})
		for _, member := range m.consensus {
			assert.False(t, member.signed, "member %s should remain unsigned", member.name)
		}
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

func TestWindowSizeMsg(t *testing.T) {
	m := NewModel(Options{})
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = model.(Model)
	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)
}

func TestCountAffirmative(t *testing.T) {
	m := NewModel(Options{})
	assert.Equal(t, 0, m.countAffirmative())

	m = m.applyConsensusMsg(ConsensusMsg{Member: constants.ConsensusMemberAxiom, Decision: true, Signed: true})
	m = m.applyConsensusMsg(ConsensusMsg{Member: constants.ConsensusMemberConcord, Decision: true, Signed: true})
	m = m.applyConsensusMsg(ConsensusMsg{Member: constants.ConsensusMemberVariance, Decision: false, Signed: true})
	assert.Equal(t, 2, m.countAffirmative())
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
