// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package wizard

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Config holds only the fields the wizard owns and can edit.
// The cmd package owns conversion to/from GatewayFlags and merging
// according to configuration precedence.
type Config struct {
	PublicBaseURL      string
	CertIdentityMode   string
	AllowedOrigins     []string
	Posture            string
	ConsensusID        string
	ConsensusURL       string
	ConsensusBootstrap string
	PasskeyRpID        string
	PasskeyRpName      string
	PasskeyRpOrigins   []string
	MCPDownstreamURL   string
	A2ADownstreamURL   string
}

// Options configures the wizard at launch.
type Options struct {
	// InitialConfig is the already-resolved starting state. It includes CLI
	// values, environment values, and defaults according to cmd precedence.
	InitialConfig Config

	// ProgramOptions supports headless Bubble Tea tests.
	ProgramOptions []tea.ProgramOption
}

// Result is the output of a completed wizard run.
type Result struct {
	Config Config
	Cancel bool // true if the user cancelled before confirmation
}

// Run starts the wizard TUI and blocks until completion or cancellation.
func Run(opts Options) (Result, error) {
	m := NewModel(opts)
	programOpts := append(
		[]tea.ProgramOption{tea.WithAltScreen()},
		opts.ProgramOptions...,
	)
	p := tea.NewProgram(m, programOpts...)

	finalModel, err := p.Run()
	if err != nil {
		return Result{}, fmt.Errorf("wizard: run: %w", err)
	}

	final, ok := finalModel.(Model)
	if !ok {
		return Result{}, fmt.Errorf("wizard: unexpected final model type %T", finalModel)
	}
	return final.result(), nil
}
