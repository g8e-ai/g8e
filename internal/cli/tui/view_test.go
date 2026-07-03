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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestView_QuittingReturnsEmpty(t *testing.T) {
	m := NewModel(Options{})
	m.quitting = true
	assert.Equal(t, "", m.View())
}

func TestView_DefaultDimensionsWhenZero(t *testing.T) {
	m := NewModel(Options{})
	out := m.View()
	assert.NotEmpty(t, out)
}

func TestView_ExplicitDimensions(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	out := m.View()
	assert.NotEmpty(t, out)
}

func TestView_MinHeightClamp(t *testing.T) {
	m := NewModel(Options{})
	m.width = 80
	m.height = 5
	out := m.View()
	assert.NotEmpty(t, out)
}

func TestView_ContainsAllPaneHeaders(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	out := m.View()
	assert.Contains(t, out, "EXECUTION PIPELINE")
	assert.Contains(t, out, "SOVEREIGN AUDIT LEDGER")
	assert.Contains(t, out, "L2 TRIBUNAL CONSENSUS")
	assert.Contains(t, out, "g8e OPERATOR CONSOLE")
}

func TestView_ContainsAllPipelineStages(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	out := m.View()
	assert.Contains(t, out, "L1: Technical Bedrock")
	assert.Contains(t, out, "L2: Consensus Tribunal")
	assert.Contains(t, out, "L3: Notary")
	assert.Contains(t, out, "L4: Warden")
	assert.Contains(t, out, "L5: Actuator")
}

func TestView_ContainsAllTribunalMembers(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	out := m.View()
	upper := strings.ToUpper(out)
	assert.Contains(t, upper, "AXIOM")
	assert.Contains(t, upper, "CONCORD")
	assert.Contains(t, upper, "VARIANCE")
	assert.Contains(t, upper, "PRAGMA")
	assert.Contains(t, upper, "NEMESIS")
}

func TestRenderPipeline_AllStatusesRender(t *testing.T) {
	tests := []struct {
		name   string
		status PipelineStatus
		detail string
	}{
		{name: "idle", status: StatusIdle, detail: ""},
		{name: "active", status: StatusActive, detail: "processing"},
		{name: "waiting", status: StatusWaiting, detail: "FIDO2 touch"},
		{name: "passed", status: StatusPassed, detail: "committed"},
		{name: "failed", status: StatusFailed, detail: "PII blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(Options{})
			m.pipeline[0].status = tt.status
			m.pipeline[0].detail = tt.detail
			out := m.renderPipeline(60, 20)
			assert.NotEmpty(t, out)
			if tt.detail != "" {
				assert.Contains(t, out, tt.detail)
			}
		})
	}
}

func TestRenderPipeline_BlinkToggleWaiting(t *testing.T) {
	m := NewModel(Options{})
	m.pipeline[0].status = StatusWaiting
	m.pipeline[0].detail = "FIDO2"

	m.blinkOn = true
	outBlink := m.renderPipeline(60, 20)
	assert.NotEmpty(t, outBlink)

	m.blinkOn = false
	outIdle := m.renderPipeline(60, 20)
	assert.NotEmpty(t, outIdle)
}

func TestRenderPipeline_BlinkToggleFailed(t *testing.T) {
	m := NewModel(Options{})
	m.pipeline[0].status = StatusFailed
	m.pipeline[0].detail = "blocked"

	m.blinkOn = true
	outBlink := m.renderPipeline(60, 20)
	assert.NotEmpty(t, outBlink)

	m.blinkOn = false
	outIdle := m.renderPipeline(60, 20)
	assert.NotEmpty(t, outIdle)
}

func TestRenderPipeline_DetailFallbackToStatusString(t *testing.T) {
	m := NewModel(Options{})
	m.pipeline[0].status = StatusActive
	m.pipeline[0].detail = ""
	out := m.renderPipeline(60, 20)
	assert.Contains(t, out, StatusActive.String())
}

func TestRenderPipeline_ExplicitDetailShown(t *testing.T) {
	m := NewModel(Options{})
	m.pipeline[0].status = StatusPassed
	m.pipeline[0].detail = "actuator committed"
	out := m.renderPipeline(60, 20)
	assert.Contains(t, out, "actuator committed")
}

func TestRenderPipeline_AllFiveStagesPresent(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderPipeline(60, 30)
	assert.Contains(t, out, "L1: Technical Bedrock")
	assert.Contains(t, out, "L2: Consensus Tribunal")
	assert.Contains(t, out, "L3: Notary")
	assert.Contains(t, out, "L4: Warden")
	assert.Contains(t, out, "L5: Actuator")
}

func TestRenderLedger_EmptyShowsAwaitingMessage(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderLedger(60, 20)
	assert.Contains(t, out, "awaiting events")
}

func TestRenderLedger_EntriesRendered(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name    string
		level   LedgerLevel
		message string
	}{
		{name: "info", level: LevelInfo, message: "normal event"},
		{name: "warn", level: LevelWarn, message: "warning event"},
		{name: "critical", level: LevelCritical, message: "critical event"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(Options{})
			m.ledger = append(m.ledger, ledgerEntry{
				level:   tt.level,
				message: tt.message,
				time:    fixedTime,
			})
			out := m.renderLedger(60, 20)
			assert.Contains(t, out, tt.message)
			assert.Contains(t, out, fixedTime.Format("15:04:05"))
			assert.Contains(t, out, tt.level.Tag())
		})
	}
}

func TestRenderLedger_ScrollOffsetShowsOlderEntries(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	m := NewModel(Options{})
	for i := 0; i < 10; i++ {
		m.ledger = append(m.ledger, ledgerEntry{
			level:   LevelInfo,
			message: "entry",
			time:    fixedTime.Add(time.Duration(i) * time.Second),
		})
	}

	m.ledgerScroll = 3
	out := m.renderLedger(60, 20)
	assert.Contains(t, out, "entry")
}

func TestRenderLedger_ScrollOffsetZeroShowsAll(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	m := NewModel(Options{})
	for i := 0; i < 3; i++ {
		m.ledger = append(m.ledger, ledgerEntry{
			level:   LevelInfo,
			message: "entry",
			time:    fixedTime,
		})
	}

	m.ledgerScroll = 0
	out := m.renderLedger(60, 20)
	assert.Contains(t, out, "entry")
}

func TestRenderLedger_ScrollOffsetExceedsLedgerLength(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	m := NewModel(Options{})
	m.ledger = append(m.ledger, ledgerEntry{
		level:   LevelInfo,
		message: "only entry",
		time:    fixedTime,
	})
	m.ledgerScroll = 100

	out := m.renderLedger(60, 20)
	assert.Contains(t, out, "only entry")
}

func TestRenderLedger_TruncatesToMaxLines(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	m := NewModel(Options{})
	for i := 0; i < 50; i++ {
		m.ledger = append(m.ledger, ledgerEntry{
			level:   LevelInfo,
			message: "entry",
			time:    fixedTime,
		})
	}

	out := m.renderLedger(60, 10)
	assert.NotEmpty(t, out)
}

func TestRenderTribunal_PendingState(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderTribunal(120)
	assert.Contains(t, out, "PENDING")
	assert.Contains(t, out, "AWAITING VOTES")
	assert.Contains(t, out, "0/5 signed")
}

func TestRenderTribunal_ConsensusReached(t *testing.T) {
	m := NewModel(Options{})
	m.result = ConsensusReached
	m.consensusHash = "abcdef1234567890abcdef"
	out := m.renderTribunal(120)
	assert.Contains(t, out, "CONSENSUS REACHED")
	assert.Contains(t, out, "abcdef12...")
}

func TestRenderTribunal_ConsensusRejected(t *testing.T) {
	m := NewModel(Options{})
	m.result = ConsensusRejected
	m.consensusHash = "deadbeef12345678"
	out := m.renderTribunal(120)
	assert.Contains(t, out, "CONSENSUS REJECTED")
}

func TestRenderTribunal_MemberApproved(t *testing.T) {
	m := NewModel(Options{})
	m.tribunal[0].signed = true
	m.tribunal[0].decision = true
	out := m.renderTribunal(120)
	assert.Contains(t, out, "[YES]")
}

func TestRenderTribunal_MemberVetoed(t *testing.T) {
	m := NewModel(Options{})
	m.tribunal[0].signed = true
	m.tribunal[0].decision = false
	out := m.renderTribunal(120)
	assert.Contains(t, out, "[NO ")
}

func TestRenderTribunal_MemberPending(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderTribunal(120)
	assert.Contains(t, out, "[...]")
}

func TestRenderTribunal_QuorumDisplay(t *testing.T) {
	m := NewModel(Options{Quorum: 4, Total: 7})
	out := m.renderTribunal(120)
	assert.Contains(t, out, "4/7 required")
}

func TestRenderTribunal_AffirmativeCountInPending(t *testing.T) {
	m := NewModel(Options{})
	m.tribunal[0].signed = true
	m.tribunal[0].decision = true
	m.tribunal[1].signed = true
	m.tribunal[1].decision = true
	out := m.renderTribunal(120)
	assert.Contains(t, out, "2/5 signed")
}

func TestRenderStatusBar_ConnectionStates(t *testing.T) {
	tests := []struct {
		name   string
		status ConnStatus
		want   string
	}{
		{name: "idle", status: ConnIdle, want: "SSE: IDLE"},
		{name: "connecting", status: ConnConnecting, want: "SSE: CONNECTING..."},
		{name: "connected", status: ConnConnected, want: "SSE: CONNECTED"},
		{name: "reconnecting", status: ConnReconnecting, want: "SSE: RECONNECTING..."},
		{name: "failed", status: ConnFailed, want: "SSE: DISCONNECTED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(Options{})
			m.connStatus = tt.status
			out := m.renderStatusBar(120)
			assert.Contains(t, out, tt.want)
		})
	}
}

func TestRenderStatusBar_ConnDetailAppended(t *testing.T) {
	m := NewModel(Options{})
	m.connStatus = ConnConnected
	m.connDetail = "retry 3/5"
	out := m.renderStatusBar(120)
	assert.Contains(t, out, "(retry 3/5)")
}

func TestRenderStatusBar_NoConnDetailOmitsParens(t *testing.T) {
	m := NewModel(Options{})
	m.connStatus = ConnConnected
	m.connDetail = ""
	out := m.renderStatusBar(120)
	assert.NotContains(t, out, "()")
}

func TestRenderStatusBar_VersionNodeNetDisplayed(t *testing.T) {
	m := NewModel(Options{
		Version:  "v1.2.3",
		NodeName: "node-alpha",
		NetLabel: "mainnet",
	})
	out := m.renderStatusBar(120)
	assert.Contains(t, out, "v1.2.3")
	assert.Contains(t, out, "node-alpha")
	assert.Contains(t, out, "mainnet")
}

func TestRenderStatusBar_QuitAndScrollHints(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderStatusBar(120)
	assert.Contains(t, out, "q: quit")
	assert.Contains(t, out, "j/k: scroll")
}

func TestRenderStatusBar_NarrowWidthDoesNotPanic(t *testing.T) {
	m := NewModel(Options{})
	m.connStatus = ConnConnected
	m.connDetail = "very long detail string that exceeds width"
	out := m.renderStatusBar(20)
	assert.NotEmpty(t, out)
}

func TestRenderStatusBar_GapPadding(t *testing.T) {
	m := NewModel(Options{
		Version:  "v1.0.0",
		NodeName: "n",
		NetLabel: "net",
	})
	m.connStatus = ConnIdle
	out := m.renderStatusBar(200)
	assert.NotEmpty(t, out)
}

func TestView_ConnStatusDisplayedInStatusBar(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.connStatus = ConnFailed
	m.connDetail = "timeout"
	out := m.View()
	assert.Contains(t, out, "SSE: DISCONNECTED")
	assert.Contains(t, out, "(timeout)")
}

func TestView_LedgerEntriesAppearInOutput(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.ledger = append(m.ledger, ledgerEntry{
		level:   LevelCritical,
		message: "PII EGRESS BLOCKED",
		time:    fixedTime,
	})
	out := m.View()
	assert.Contains(t, out, "PII EGRESS BLOCKED")
}

func TestView_ConsensusResultDisplayedInTribunal(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.result = ConsensusReached
	m.consensusHash = "abcdef1234567890"
	out := m.View()
	assert.Contains(t, out, "CONSENSUS REACHED")
}

func TestView_PipelineStatusDetailAppearsInOutput(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.pipeline[0].status = StatusFailed
	m.pipeline[0].detail = "DOCTRINE VIOLATION"
	out := m.View()
	assert.Contains(t, out, "DOCTRINE VIOLATION")
}

func TestView_StatusBarPresentAtBottom(t *testing.T) {
	m := NewModel(Options{Version: "v9.9.9"})
	m.width = 120
	m.height = 40
	out := m.View()
	assert.Contains(t, out, "v9.9.9")
}

func TestRenderLedger_MultipleEntriesAllRendered(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	m := NewModel(Options{})
	for i := 0; i < 5; i++ {
		m.ledger = append(m.ledger, ledgerEntry{
			level:   LevelInfo,
			message: "msg",
			time:    fixedTime.Add(time.Duration(i) * time.Second),
		})
	}
	out := m.renderLedger(60, 20)
	count := strings.Count(out, "msg")
	assert.Equal(t, 5, count)
}

func TestRenderLedger_MixedLevelsRendered(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	m := NewModel(Options{})
	m.ledger = append(m.ledger,
		ledgerEntry{level: LevelInfo, message: "info msg", time: fixedTime},
		ledgerEntry{level: LevelWarn, message: "warn msg", time: fixedTime},
		ledgerEntry{level: LevelCritical, message: "crit msg", time: fixedTime},
	)
	out := m.renderLedger(60, 20)
	assert.Contains(t, out, "info msg")
	assert.Contains(t, out, "warn msg")
	assert.Contains(t, out, "crit msg")
	assert.Contains(t, out, "[INFO]")
	assert.Contains(t, out, "[WARN]")
	assert.Contains(t, out, "[CRIT]")
}

func TestRenderTribunal_AllMembersRendered(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderTribunal(120)
	upper := strings.ToUpper(out)
	assert.Contains(t, upper, "AXIOM")
	assert.Contains(t, upper, "CONCORD")
	assert.Contains(t, upper, "VARIANCE")
	assert.Contains(t, upper, "PRAGMA")
	assert.Contains(t, upper, "NEMESIS")
}

func TestRenderTribunal_MixedVoteStates(t *testing.T) {
	m := NewModel(Options{})
	m.tribunal[0].signed = true
	m.tribunal[0].decision = true
	m.tribunal[1].signed = true
	m.tribunal[1].decision = false
	m.tribunal[2].signed = false
	out := m.renderTribunal(120)
	assert.Contains(t, out, "[YES]")
	assert.Contains(t, out, "[NO ")
	assert.Contains(t, out, "[...]")
}

func TestRenderPipeline_HeaderPresent(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderPipeline(60, 20)
	assert.Contains(t, out, "EXECUTION PIPELINE (L1-L5)")
}

func TestRenderLedger_HeaderPresent(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderLedger(60, 20)
	assert.Contains(t, out, "SOVEREIGN AUDIT LEDGER")
}

func TestView_FullLayoutIntegration(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	m := NewModel(Options{
		Version:  "v1.2.3",
		NodeName: "node-1",
		NetLabel: "testnet",
	})
	m.width = 120
	m.height = 40
	m.pipeline[0].status = StatusPassed
	m.pipeline[0].detail = "doctrine verified"
	m.pipeline[1].status = StatusActive
	m.pipeline[1].detail = "tribunal deliberating"
	m.ledger = append(m.ledger, ledgerEntry{
		level:   LevelWarn,
		message: "rate limit approaching",
		time:    fixedTime,
	})
	m.tribunal[0].signed = true
	m.tribunal[0].decision = true
	m.connStatus = ConnConnected

	out := m.View()
	require.NotEmpty(t, out)
	assert.Contains(t, out, "EXECUTION PIPELINE")
	assert.Contains(t, out, "doctrine verified")
	assert.Contains(t, out, "tribunal deliberating")
	assert.Contains(t, out, "SOVEREIGN AUDIT LEDGER")
	assert.Contains(t, out, "rate limit approaching")
	assert.Contains(t, out, "L2 TRIBUNAL CONSENSUS")
	assert.Contains(t, out, "[YES]")
	assert.Contains(t, out, "v1.2.3")
	assert.Contains(t, out, "SSE: CONNECTED")
}
