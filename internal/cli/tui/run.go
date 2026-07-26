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
		adapter := NewAdapter(opts.SSEURL, opts.Token, p, opts.HTTPClient)
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

