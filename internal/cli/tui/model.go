// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"net/http"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Options configures the TUI at launch.
type Options struct {
	Version      string
	NodeName     string
	NetLabel     string
	Quorum       int
	Total        int
	SSEURL       string
	Token        string
	CLISessionID string
	HTTPClient   *http.Client

	// ProgramOptions are appended to the default bubbletea program options
	// (AltScreen, MouseCellMotion). Tests use this to inject headless options.
	ProgramOptions []tea.ProgramOption
}

// Model is the bubbletea state container for the Tactical Governance Console.
type Model struct {
	width  int
	height int

	// Configuration
	version  string
	nodeName string
	netLabel string

	// Pipeline state — 5 entries: L1-L5
	pipeline []pipelineStageState
	activeTx string

	// Ledger state
	ledger       []ledgerEntry
	ledgerScroll int // 0 = auto-scroll to bottom; >0 = manual offset from bottom

	// Consensus state
	consensus     []consensusMemberState
	quorum        int
	total         int
	result        ConsensusResult
	consensusHash string

	// Animation
	blinkOn bool

	// Connection state (SSE adapter)
	connStatus ConnStatus
	connDetail string

	// Status
	quitting bool
}

// NewModel constructs a Model with all pipeline stages idle and consensus
// members in pending state.
func NewModel(opts Options) Model {
	members := []consensusMemberState{
		{name: constants.ConsensusMemberAxiom},
		{name: constants.ConsensusMemberConcord},
		{name: constants.ConsensusMemberVariance},
		{name: constants.ConsensusMemberPragma},
		{name: constants.ConsensusMemberNemesis},
	}

	quorum := opts.Quorum
	if quorum == 0 {
		quorum = 3
	}
	total := opts.Total
	if total == 0 {
		total = 5
	}

	return Model{
		version:  opts.Version,
		nodeName: opts.NodeName,
		netLabel: opts.NetLabel,
		pipeline: []pipelineStageState{
			{status: StatusIdle},
			{status: StatusIdle},
			{status: StatusIdle},
			{status: StatusIdle},
			{status: StatusIdle},
		},
		ledger:    make([]ledgerEntry, 0, 64),
		consensus: members,
		quorum:    quorum,
		total:     total,
		result:    ConsensusPending,
	}
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
