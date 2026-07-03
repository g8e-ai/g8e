// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// newTestProgram builds a headless bubbletea program (no renderer, no real
// input/output) suitable for driving Model updates from tests.
func newTestProgram(t *testing.T) *tea.Program {
	t.Helper()
	m := NewModel(Options{})
	return tea.NewProgram(m, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(io.Discard))
}

// headlessProgramOptions returns program options that allow Run to execute
// without a real terminal. The provided input string is fed to the program;
// "q" triggers a clean quit via the key handler.
func headlessProgramOptions(input string) []tea.ProgramOption {
	return []tea.ProgramOption{
		tea.WithoutRenderer(),
		tea.WithInput(strings.NewReader(input)),
		tea.WithOutput(io.Discard),
	}
}

func TestEmitPipeline(t *testing.T) {
	tests := []struct {
		name   string
		stage  PipelineStage
		status PipelineStatus
		txID   string
		detail string
	}{
		{
			name:   "L1 active with txID and detail",
			stage:  StageL1,
			status: StatusActive,
			txID:   "tx-001",
			detail: "doctrine check",
		},
		{
			name:   "L2 waiting for fido2",
			stage:  StageL2,
			status: StatusWaiting,
			txID:   "tx-002",
			detail: "awaiting tribunal",
		},
		{
			name:   "L3 passed",
			stage:  StageL3,
			status: StatusPassed,
			txID:   "tx-003",
			detail: "notary approved",
		},
		{
			name:   "L4 failed",
			stage:  StageL4,
			status: StatusFailed,
			txID:   "tx-004",
			detail: "replay detected",
		},
		{
			name:   "L5 idle with empty txID",
			stage:  StageL5,
			status: StatusIdle,
			txID:   "",
			detail: "",
		},
		{
			name:   "L1 active with empty detail",
			stage:  StageL1,
			status: StatusActive,
			txID:   "tx-005",
			detail: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestProgram(t)

			go func() {
				EmitPipeline(p, tt.stage, tt.status, tt.txID, tt.detail)
				p.Quit()
			}()

			finalModel, err := p.Run()
			require.NoError(t, err)

			m, ok := finalModel.(Model)
			require.True(t, ok)
			assert.Equal(t, tt.status, m.pipeline[tt.stage].status)
			assert.Equal(t, tt.detail, m.pipeline[tt.stage].detail)
			if tt.txID != "" {
				assert.Equal(t, tt.txID, m.activeTx)
			}
		})
	}
}

func TestEmitPipeline_OverwritesPreviousStageState(t *testing.T) {
	p := newTestProgram(t)

	go func() {
		EmitPipeline(p, StageL1, StatusActive, "tx-001", "processing")
		EmitPipeline(p, StageL1, StatusPassed, "tx-001", "done")
		p.Quit()
	}()

	finalModel, err := p.Run()
	require.NoError(t, err)

	m, ok := finalModel.(Model)
	require.True(t, ok)
	assert.Equal(t, StatusPassed, m.pipeline[StageL1].status)
	assert.Equal(t, "done", m.pipeline[StageL1].detail)
	assert.Equal(t, "tx-001", m.activeTx)
}

func TestEmitLedger(t *testing.T) {
	fixedTime := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	originalTimeNow := timeNow
	t.Cleanup(func() { timeNow = originalTimeNow })
	timeNow = func() time.Time { return fixedTime }

	tests := []struct {
		name    string
		level   LedgerLevel
		message string
	}{
		{
			name:    "info level",
			level:   LevelInfo,
			message: "envelope received",
		},
		{
			name:    "warn level",
			level:   LevelWarn,
			message: "doctrine warning: pattern match",
		},
		{
			name:    "critical level",
			level:   LevelCritical,
			message: "PII EGRESS BLOCKED",
		},
		{
			name:    "empty message",
			level:   LevelInfo,
			message: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestProgram(t)

			go func() {
				EmitLedger(p, tt.level, tt.message)
				p.Quit()
			}()

			finalModel, err := p.Run()
			require.NoError(t, err)

			m, ok := finalModel.(Model)
			require.True(t, ok)
			require.Len(t, m.ledger, 1)
			assert.Equal(t, tt.level, m.ledger[0].level)
			assert.Equal(t, tt.message, m.ledger[0].message)
			assert.Equal(t, fixedTime, m.ledger[0].time)
		})
	}
}

func TestEmitLedger_AppendsMultipleEntries(t *testing.T) {
	p := newTestProgram(t)

	go func() {
		EmitLedger(p, LevelInfo, "first")
		EmitLedger(p, LevelWarn, "second")
		EmitLedger(p, LevelCritical, "third")
		p.Quit()
	}()

	finalModel, err := p.Run()
	require.NoError(t, err)

	m, ok := finalModel.(Model)
	require.True(t, ok)
	require.Len(t, m.ledger, 3)
	assert.Equal(t, "first", m.ledger[0].message)
	assert.Equal(t, "second", m.ledger[1].message)
	assert.Equal(t, "third", m.ledger[2].message)
	assert.Equal(t, LevelInfo, m.ledger[0].level)
	assert.Equal(t, LevelWarn, m.ledger[1].level)
	assert.Equal(t, LevelCritical, m.ledger[2].level)
}

func TestEmitConsensus(t *testing.T) {
	tests := []struct {
		name     string
		member   constants.TribunalMember
		decision bool
		signed   bool
		quorum   int
		total    int
		result   ConsensusResult
		hash     string
	}{
		{
			name:     "axiom signs consensus reached",
			member:   constants.TribunalMemberAxiom,
			decision: true,
			signed:   true,
			quorum:   4,
			total:    5,
			result:   ConsensusReached,
			hash:     "hash123",
		},
		{
			name:     "concord rejects unsigned",
			member:   constants.TribunalMemberConcord,
			decision: false,
			signed:   false,
			quorum:   3,
			total:    5,
			result:   ConsensusRejected,
			hash:     "hash456",
		},
		{
			name:     "variance pending with empty hash",
			member:   constants.TribunalMemberVariance,
			decision: true,
			signed:   false,
			quorum:   0,
			total:    0,
			result:   ConsensusPending,
			hash:     "",
		},
		{
			name:     "pragma signs with zero quorum",
			member:   constants.TribunalMemberPragma,
			decision: true,
			signed:   true,
			quorum:   0,
			total:    5,
			result:   ConsensusReached,
			hash:     "hash789",
		},
		{
			name:     "nemesis rejects with zero total",
			member:   constants.TribunalMemberNemesis,
			decision: false,
			signed:   true,
			quorum:   3,
			total:    0,
			result:   ConsensusRejected,
			hash:     "hashabc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestProgram(t)

			go func() {
				EmitConsensus(p, tt.member, tt.decision, tt.signed, tt.quorum, tt.total, tt.result, tt.hash)
				p.Quit()
			}()

			finalModel, err := p.Run()
			require.NoError(t, err)

			m, ok := finalModel.(Model)
			require.True(t, ok)

			found := false
			for _, member := range m.tribunal {
				if member.name == tt.member {
					found = true
					assert.Equal(t, tt.decision, member.decision)
					assert.Equal(t, tt.signed, member.signed)
				}
			}
			assert.True(t, found, "expected %s tribunal member to be present", tt.member)

			if tt.quorum > 0 {
				assert.Equal(t, tt.quorum, m.quorum)
			}
			if tt.total > 0 {
				assert.Equal(t, tt.total, m.total)
			}
			if tt.result != ConsensusPending {
				assert.Equal(t, tt.result, m.result)
			}
			if tt.hash != "" {
				assert.Equal(t, tt.hash, m.consensusHash)
			}
		})
	}
}

func TestEmitConsensus_UnknownMemberDoesNotCorruptTribunal(t *testing.T) {
	p := newTestProgram(t)

	go func() {
		EmitConsensus(p, constants.TribunalMember("unknown-member"), true, true, 4, 5, ConsensusReached, "hash")
		p.Quit()
	}()

	finalModel, err := p.Run()
	require.NoError(t, err)

	m, ok := finalModel.(Model)
	require.True(t, ok)

	for _, member := range m.tribunal {
		assert.False(t, member.decision, "no tribunal member should be updated for unknown member")
		assert.False(t, member.signed, "no tribunal member should be updated for unknown member")
	}
	assert.Equal(t, 4, m.quorum)
	assert.Equal(t, 5, m.total)
	assert.Equal(t, ConsensusReached, m.result)
	assert.Equal(t, "hash", m.consensusHash)
}

func TestEmitConsensus_MultipleMembersVote(t *testing.T) {
	p := newTestProgram(t)

	go func() {
		EmitConsensus(p, constants.TribunalMemberAxiom, true, true, 0, 0, ConsensusPending, "")
		EmitConsensus(p, constants.TribunalMemberConcord, true, true, 0, 0, ConsensusPending, "")
		EmitConsensus(p, constants.TribunalMemberVariance, false, false, 0, 0, ConsensusPending, "")
		EmitConsensus(p, constants.TribunalMemberPragma, true, true, 4, 5, ConsensusReached, "final-hash")
		p.Quit()
	}()

	finalModel, err := p.Run()
	require.NoError(t, err)

	m, ok := finalModel.(Model)
	require.True(t, ok)

	memberState := make(map[constants.TribunalMember]tribunalMemberState)
	for _, member := range m.tribunal {
		memberState[member.name] = member
	}

	assert.True(t, memberState[constants.TribunalMemberAxiom].decision)
	assert.True(t, memberState[constants.TribunalMemberAxiom].signed)
	assert.True(t, memberState[constants.TribunalMemberConcord].decision)
	assert.True(t, memberState[constants.TribunalMemberConcord].signed)
	assert.False(t, memberState[constants.TribunalMemberVariance].decision)
	assert.False(t, memberState[constants.TribunalMemberVariance].signed)
	assert.True(t, memberState[constants.TribunalMemberPragma].decision)
	assert.True(t, memberState[constants.TribunalMemberPragma].signed)
	assert.Equal(t, 4, m.quorum)
	assert.Equal(t, 5, m.total)
	assert.Equal(t, ConsensusReached, m.result)
	assert.Equal(t, "final-hash", m.consensusHash)
}

func TestRun_EmptySSEURLReturnsQuickly(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		done <- Run(t.Context(), Options{
			ProgramOptions: headlessProgramOptions("q"),
		})
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after simulated quit keypress")
	}
}

func TestRun_QuitViaCtrlC(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		done <- Run(t.Context(), Options{
			ProgramOptions: headlessProgramOptions("ctrl+c"),
		})
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after ctrl+c")
	}
}

func TestRun_PassesOptionsToModel(t *testing.T) {
	opts := Options{
		Version:        "v1.3.6",
		NodeName:       "node-alpha",
		NetLabel:       "mTLS",
		Quorum:         4,
		Total:          7,
		ProgramOptions: headlessProgramOptions("q"),
	}

	done := make(chan struct{}, 1)
	var capturedModel Model
	go func() {
		m := NewModel(opts)
		capturedModel = m
		done <- struct{}{}
	}()
	<-done

	assert.Equal(t, "v1.3.6", capturedModel.version)
	assert.Equal(t, "node-alpha", capturedModel.nodeName)
	assert.Equal(t, "mTLS", capturedModel.netLabel)
	assert.Equal(t, 4, capturedModel.quorum)
	assert.Equal(t, 7, capturedModel.total)
}

func TestRun_AdapterGoroutineCleanedUpOnExit(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		done <- Run(t.Context(), Options{
			SSEURL:         "http://127.0.0.1:1/nonexistent",
			HTTPClient:     nil,
			ProgramOptions: headlessProgramOptions("q"),
		})
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return; adapter goroutine may not be cleaned up")
	}
}

func TestRun_CancelledContextExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			ProgramOptions: headlessProgramOptions("q"),
		})
	}()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}
