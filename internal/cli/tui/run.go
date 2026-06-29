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

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the Tactical Governance TUI. It blocks until the user presses
// q or ctrl+c. The adapter runs in a goroutine and feeds events from the
// SSE stream into the program.
func Run(ctx context.Context, opts Options) error {
	m := NewModel(opts)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	adapter := NewAdapter(opts.SSEURL, opts.Token, p, opts.HTTPClient)
	go adapter.Run(ctx)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: run: %w", err)
	}
	return nil
}

// EmitPipeline sends a PipelineMsg to the program from outside the bubbletea
// loop. Used by demo scenario code to drive TUI state.
func EmitPipeline(p *tea.Program, stage PipelineStage, status PipelineStatus, txID, detail string) {
	p.Send(PipelineMsg{
		Stage:  stage,
		Status: status,
		TxID:   txID,
		Detail: detail,
	})
}

// EmitLedger sends a LedgerMsg to the program from outside the bubbletea loop.
func EmitLedger(p *tea.Program, level LedgerLevel, message string) {
	p.Send(LedgerMsg{
		Level:   level,
		Message: message,
		Time:    timeNow(),
	})
}

// EmitConsensus sends a ConsensusMsg to the program from outside the bubbletea
// loop.
func EmitConsensus(p *tea.Program, member string, decision, signed bool, quorum, total int, result ConsensusResult, hash string) {
	p.Send(ConsensusMsg{
		Member:   member,
		Decision: decision,
		Signed:   signed,
		Quorum:   quorum,
		Total:    total,
		Result:   result,
		Hash:     hash,
	})
}
