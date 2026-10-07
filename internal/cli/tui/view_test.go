// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"strings"
	"testing"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
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
	assert.Contains(t, out, "PENDING APPROVALS")
	assert.Contains(t, out, "OPERATORS")
	assert.Contains(t, out, "g8e TACTICAL GOVERNANCE CONSOLE")
}

func TestView_ContainsAllPipelineStages(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	out := m.View()
	assert.Contains(t, out, "L1: Doctrine")
	assert.Contains(t, out, "L2: Consensus")
	assert.Contains(t, out, "L3: Notary")
	assert.Contains(t, out, "L4: Warden")
	assert.Contains(t, out, "L5: Actuator")
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
	assert.Contains(t, out, "L1: Doctrine")
	assert.Contains(t, out, "L2: Consensus")
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

func TestRenderStatusBar_QuitAndScrollHints(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderStatusBar(120)
	assert.Contains(t, out, "q: quit")
	assert.Contains(t, out, "j/k: move")
	assert.Contains(t, out, "a: approve")
	assert.Contains(t, out, "r: refresh")
	assert.Contains(t, out, "tab: focus")
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
		Version: "v1.0.0",
		Identity: Identity{
			UserID:       "user@example.com",
			CLISessionID: "session123456789",
		},
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
	m := NewModel(Options{
		Version: "v9.9.9",
		Identity: Identity{
			UserID:       "test@example.com",
			CLISessionID: "test-session",
		},
	})
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
		Version: "v1.2.3",
		Identity: Identity{
			UserID:       "user@example.com",
			CLISessionID: "session123456789",
		},
	})
	m.width = 120
	m.height = 40
	m.pipeline[0].status = StatusPassed
	m.pipeline[0].detail = "doctrine verified"
	m.pipeline[1].status = StatusActive
	m.pipeline[1].detail = "consensus deliberating"
	m.ledger = append(m.ledger, ledgerEntry{
		level:   LevelWarn,
		message: "rate limit approaching",
		time:    fixedTime,
	})
	m.connStatus = ConnConnected

	out := m.View()
	require.NotEmpty(t, out)
	assert.Contains(t, out, "EXECUTION PIPELINE")
	assert.Contains(t, out, "doctrine verified")
	assert.Contains(t, out, "consensus deliberating")
	assert.Contains(t, out, "SOVEREIGN AUDIT LEDGER")
	assert.Contains(t, out, "rate limit approaching")
	assert.Contains(t, out, "PENDING APPROVALS")
	assert.Contains(t, out, "OPERATORS")
	assert.Contains(t, out, "v1.2.3")
	assert.Contains(t, out, "SSE: CONNECTED")
}

func TestRenderHeader_ContainsIdentity(t *testing.T) {
	m := NewModel(Options{
		Version: "v1.0.0",
		Identity: Identity{
			UserID:       "alice@example.com",
			CLISessionID: "abcdef1234567890",
			OperatorID:   "",
		},
	})
	m.width = 120
	out := m.renderHeader(120)
	assert.Contains(t, out, "alice@example.com")
	assert.Contains(t, out, "abcdef12...")
	assert.Contains(t, out, "OPERATOR: unbound")
	assert.Contains(t, out, "v1.0.0")
}

func TestRenderHeader_BoundOperatorShown(t *testing.T) {
	m := NewModel(Options{
		Version: "v1.0.0",
		Identity: Identity{
			UserID:       "alice@example.com",
			CLISessionID: "abcdef1234567890",
			OperatorID:   "op-12345",
		},
	})
	m.width = 120
	out := m.renderHeader(120)
	assert.Contains(t, out, "OPERATOR: op-12345")
	assert.NotContains(t, out, "unbound")
}

func TestRenderHeader_PostureUnknownBeforeHealth(t *testing.T) {
	m := NewModel(Options{
		Version: "v1.0.0",
		Identity: Identity{
			UserID:       "alice@example.com",
			CLISessionID: "test123",
		},
	})
	m.width = 120
	m.posture = nil
	out := m.renderHeader(120)
	assert.Contains(t, out, "POSTURE: UNKNOWN")
}

func TestRenderApprovals_EmptyState(t *testing.T) {
	m := NewModel(Options{})
	m.pending = []models.SuspendedTxResponse{}
	out := m.renderApprovals(60)
	assert.Contains(t, out, "PENDING APPROVALS (0)")
	assert.Contains(t, out, "(no pending approvals)")
}

func TestRenderApprovals_ShowsToolAndHash(t *testing.T) {
	m := NewModel(Options{})
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	m.pending = []models.SuspendedTxResponse{
		{
			ToolName:        "file_write",
			TransactionHash: "abcdef1234567890",
			ExpiresAt:       fixedTime.Add(5 * time.Minute),
		},
	}
	timeNow = func() time.Time { return fixedTime }
	defer func() { timeNow = time.Now }()

	out := m.renderApprovals(60)
	assert.Contains(t, out, "PENDING APPROVALS (1)")
	assert.Contains(t, out, "file_write")
	assert.Contains(t, out, "abcdef12...")
	assert.Contains(t, out, "expires")
}

func TestRenderApprovals_AwaitingBrowserMarked(t *testing.T) {
	m := NewModel(Options{})
	m.pending = []models.SuspendedTxResponse{
		{
			ToolName:        "file_write",
			TransactionHash: "abcdef1234567890",
		},
	}
	m.awaitingApproval["abcdef1234567890"] = struct{}{}
	out := m.renderApprovals(60)
	assert.Contains(t, out, "[awaiting browser]")
}

func TestRenderApprovals_SelectionMarkerWhenFocused(t *testing.T) {
	m := NewModel(Options{})
	m.pending = []models.SuspendedTxResponse{
		{ToolName: "file_write", TransactionHash: "abc123"},
	}
	m.focus = paneApprovals
	m.pendingSelected = 0
	out := m.renderApprovals(60)
	assert.Contains(t, out, "> ")
}

func TestRenderOperators_LoadingState(t *testing.T) {
	m := NewModel(Options{})
	m.operatorsLoaded = false
	out := m.renderOperators(60)
	assert.Contains(t, out, "OPERATORS")
	assert.Contains(t, out, "(loading...)")
}

func TestRenderOperators_ConnectedCount(t *testing.T) {
	m := NewModel(Options{})
	m.operators = []*operatorv1.OperatorDocument{
		{Id: "op1", Status: "healthy"},
		{Id: "op2", Status: "healthy"},
	}
	m.operatorsTotal = 5
	m.operatorsLoaded = true
	out := m.renderOperators(60)
	assert.Contains(t, out, "OPERATORS (2 connected / 5)")
}

func TestRenderOperators_ErrorState(t *testing.T) {
	m := NewModel(Options{})
	m.operatorsErr = "connection timeout"
	m.operatorsLoaded = true
	out := m.renderOperators(60)
	assert.Contains(t, out, "unavailable: connection timeout")
}

func TestRenderOperators_BoundMarked(t *testing.T) {
	m := NewModel(Options{})
	m.identity.OperatorID = "bound-op"
	m.operators = []*operatorv1.OperatorDocument{
		{Id: "bound-op", Status: "healthy"},
	}
	m.operatorsLoaded = true
	out := m.renderOperators(60)
	assert.Contains(t, out, "[bound]")
}

func TestListWindow_SelectionVisibleAtStart(t *testing.T) {
	start, end := listWindow(10, 0)
	assert.Equal(t, 0, start)
	assert.True(t, end > 0)
	assert.True(t, 0 < end)
}

func TestListWindow_SelectionVisibleWhenScrolled(t *testing.T) {
	start, end := listWindow(10, 8)
	assert.True(t, start <= 8)
	assert.True(t, 8 < end)
}

func TestListWindow_BoundsListLength(t *testing.T) {
	start, end := listWindow(5, 0)
	assert.Equal(t, 0, start)
	assert.LessOrEqual(t, end, 5)
}

func TestStageAnnotation_NoAuditWhenPostureNil(t *testing.T) {
	m := NewModel(Options{})
	m.posture = nil
	result := m.stageAnnotation(StageL2)
	assert.Equal(t, "", result)
}

func TestStageAnnotation_AuditedWhenNotEnforced(t *testing.T) {
	m := NewModel(Options{})
	// Create a mock posture that doesn't enforce L2
	m.posture = &mockPosture{requiresL2: false, requiresL3: true}
	result := m.stageAnnotation(StageL2)
	assert.Equal(t, " (audited)", result)
}

func TestStageAnnotation_NoMarkingWhenEnforced(t *testing.T) {
	m := NewModel(Options{})
	// Create a mock posture that enforces L2
	m.posture = &mockPosture{requiresL2: true, requiresL3: true}
	result := m.stageAnnotation(StageL2)
	assert.Equal(t, "", result)
}

type mockPosture struct {
	requiresL2 bool
	requiresL3 bool
}

func (mp *mockPosture) Name() string {
	return "DOCTRINE"
}

func (mp *mockPosture) Description() string {
	return "mock posture"
}

func (mp *mockPosture) RequiresL2Signature() bool {
	return mp.requiresL2
}

func (mp *mockPosture) RequiresL3Proof() bool {
	return mp.requiresL3
}

// TestView_FitsTerminalExactly guards the layout math: borders count toward
// each pane's size, so the frame is exactly the terminal and the header is
// never scrolled off the top.
func TestView_FitsTerminalExactly(t *testing.T) {
	fixedTime := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	for _, size := range []struct{ width, height int }{{160, 40}, {120, 40}, {100, 30}, {80, 24}, {200, 60}} {
		m := NewModel(Options{
			Version: "v2.3.1",
			Identity: Identity{
				UserID:       "d0816a38-86b4-478c-a80a-5ddfd0f87541",
				CLISessionID: "0506b69e-6753-453a-bf70-782e76ca0542",
				OperatorID:   "embedded-operator",
			},
		})
		m.width, m.height = size.width, size.height
		posture, err := governance.ParseGovernancePosture("notary")
		require.NoError(t, err)
		m.posture = posture
		for i := 0; i < 40; i++ {
			m.ledger = append(m.ledger, ledgerEntry{
				level:   LevelWarn,
				message: "Could not open a browser (exec: \"xdg-open\": not found); approve tx abcdef12... at https://localhost:8443/console/approve?tx=" + strings.Repeat("ab", 32),
				time:    fixedTime,
			})
		}
		for i := 0; i < 9; i++ {
			m.operators = append(m.operators, &operatorv1.OperatorDocument{Id: strings.Repeat("f", 36), CurrentHostname: strings.Repeat("host", 20), Status: "active"})
		}
		m.operatorsLoaded = true

		out := m.View()
		lines := strings.Split(out, "\n")
		assert.Len(t, lines, size.height, "%dx%d: line count", size.width, size.height)
		for i, line := range lines {
			assert.LessOrEqual(t, lipgloss.Width(line), size.width, "%dx%d: line %d too wide", size.width, size.height, i)
		}
		assert.Contains(t, lines[0], "g8e TACTICAL GOVERNANCE CONSOLE", "%dx%d: header on the first line", size.width, size.height)
		assert.Contains(t, lines[len(lines)-1], "SSE:", "%dx%d: status bar on the last line", size.width, size.height)
	}
}

func TestRenderLedger_WrapsLongEntries(t *testing.T) {
	m := NewModel(Options{})
	m.ledger = append(m.ledger, ledgerEntry{level: LevelInfo, message: "visit https://localhost:8443/console/approve?tx=0123456789abcdef-END", time: time.Now()})
	out := m.renderLedger(40, 20)
	assert.Contains(t, out, "-END", "the tail of a long entry wraps instead of being cut")
}

func TestRenderPipeline_ShortPaneShowsEveryStage(t *testing.T) {
	m := NewModel(Options{})
	out := m.renderPipeline(40, 13)
	assert.Contains(t, out, "L5: Actuator", "a short pane drops spacer lines instead of the last stages")
	assert.Len(t, strings.Split(out, "\n"), 13)
}
