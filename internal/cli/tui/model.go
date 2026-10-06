// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
)

// Identity is the enrolled CLI identity the TUI runs as: the same fields
// 'g8e auth' reports.
type Identity struct {
	UserID       string
	CLISessionID string
	OperatorID   string // bound Operator, empty when the CLI session is unbound
}

// Options configures the TUI at launch.
type Options struct {
	Version  string
	Identity Identity

	// Session is the CLI's authenticated Gateway session (*api.Client). When
	// nil the TUI runs without a live event source.
	Session Session

	// ApprovalURL returns the browser WebAuthn approval page for a pending
	// transaction (auth.ApprovalPageURL), and OpenBrowser opens it
	// (platform.OpenBrowser) — the same flow as 'g8e auth approve'. When
	// either is nil, approving from the TUI is disabled.
	ApprovalURL func(txHash string) string
	OpenBrowser func(url string) error

	// ProgramOptions are appended to the default bubbletea program options
	// (AltScreen, MouseCellMotion). Tests use this to inject headless options.
	ProgramOptions []tea.ProgramOption
}

// pane identifies a focusable pane.
type pane int

const (
	paneLedger pane = iota
	paneApprovals
	paneOperators
	paneCount
)

// Model is the bubbletea state container for the Tactical Governance Console.
type Model struct {
	width  int
	height int

	// Configuration
	version  string
	identity Identity

	// Gateway access and the approve flow. gw is nil without a session.
	gw          *gateway
	approvalURL func(string) string
	openBrowser func(string) error

	focus pane

	// Pipeline state — 5 entries: L1-L5
	pipeline []pipelineStageState
	activeTx string

	// Ledger state
	ledger       []ledgerEntry
	ledgerScroll int // 0 = auto-scroll to bottom; >0 = manual offset from bottom

	// Pending L3 approvals in Gateway order from the last successful refresh,
	// the selected row, and the transactions whose browser approval page was
	// opened from the TUI and are awaiting approval.completed.
	pending          []models.SuspendedTxResponse
	pendingSelected  int
	awaitingApproval map[string]struct{}

	// Operators from the last successful refresh: connected ones only, plus
	// the total the Gateway listed.
	operators         []models.OperatorDocumentGo
	operatorsTotal    int
	operatorsLoaded   bool
	operatorsErr      string
	operatorsSelected int

	// Gateway health: posture is nil until health is fetched.
	posture        governance.GovernancePosture
	gatewayVersion string

	// Animation
	blinkOn bool

	// Connection state (SSE adapter)
	connStatus ConnStatus
	connDetail string

	// Status
	quitting bool
}

// NewModel constructs a Model with all pipeline stages idle.
func NewModel(opts Options) Model {
	m := Model{
		version:     opts.Version,
		identity:    opts.Identity,
		approvalURL: opts.ApprovalURL,
		openBrowser: opts.OpenBrowser,
		pipeline: []pipelineStageState{
			{status: StatusIdle},
			{status: StatusIdle},
			{status: StatusIdle},
			{status: StatusIdle},
			{status: StatusIdle},
		},
		ledger:           make([]ledgerEntry, 0, 64),
		awaitingApproval: make(map[string]struct{}),
	}
	if opts.Session != nil {
		m.gw = &gateway{session: opts.Session, userID: opts.Identity.UserID}
	}
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tick()
}

// hasBlinkingState returns true if any pipeline stage is in Waiting or Failed
// status, which means the blink animation should be active.
func (m Model) hasBlinkingState() bool {
	for _, stage := range m.pipeline {
		if stage.status == StatusWaiting || stage.status == StatusFailed {
			return true
		}
	}
	return false
}

// tick returns a command that waits 500ms then sends a TickMsg.
func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}
