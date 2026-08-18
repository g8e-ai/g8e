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
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the Tactical Governance TUI. It blocks until the user presses
// q or ctrl+c. When an SSE URL is configured, the adapter runs in a
// goroutine and feeds events from the SSE stream into the program.
func Run(ctx context.Context, opts Options) error {
	m := NewModel(opts)
	programOpts := append(
		[]tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion()},
		opts.ProgramOptions...,
	)
	p := tea.NewProgram(m, programOpts...)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	if opts.SSEURL != "" {
		adapter := NewAdapter(opts.SSEURL, opts.Token, opts.CLISessionID, p, opts.HTTPClient)
		wg.Add(1)
		go func() {
			defer wg.Done()
			adapter.Run(ctx)
		}()
	}

	_, err := p.Run()
	cancel()
	wg.Wait()

	if err != nil {
		return fmt.Errorf("tui: run: %w", err)
	}
	return nil
}
