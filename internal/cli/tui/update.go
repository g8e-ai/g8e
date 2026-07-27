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
	tea "github.com/charmbracelet/bubbletea"
)

// Update implements tea.Model. It routes messages to state transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.ledgerScroll < len(m.ledger)-1 {
				m.ledgerScroll++
			}
		case "down", "j":
			if m.ledgerScroll > 0 {
				m.ledgerScroll--
			}
		case "g":
			m.ledgerScroll = len(m.ledger) - 1
			if m.ledgerScroll < 0 {
				m.ledgerScroll = 0
			}
		case "G":
			m.ledgerScroll = 0
		}

	case PipelineMsg:
		m = m.applyPipelineMsg(msg)
		if m.hasBlinkingState() {
			return m, tick()
		}

	case LedgerMsg:
		m = m.applyLedgerMsg(msg)

	case ConsensusMsg:
		m = m.applyConsensusMsg(msg)

	case ConnStatusMsg:
		m.connStatus = msg.Status
		m.connDetail = msg.Detail

	case TickMsg:
		m.blinkOn = !m.blinkOn
		if m.hasBlinkingState() {
			return m, tick()
		}
		return m, nil

	case QuitMsg:
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

// applyPipelineMsg updates a pipeline stage's status and detail.
func (m Model) applyPipelineMsg(msg PipelineMsg) Model {
	idx := int(msg.Stage)
	if idx < 0 || idx >= len(m.pipeline) {
		return m
	}
	m.pipeline[idx].status = msg.Status
	m.pipeline[idx].detail = msg.Detail
	if msg.TxID != "" {
		m.activeTx = msg.TxID
	}
	return m
}

// applyLedgerMsg appends a ledger entry to the buffer.
func (m Model) applyLedgerMsg(msg LedgerMsg) Model {
	entry := ledgerEntry{
		level:   msg.Level,
		message: msg.Message,
		time:    msg.Time,
	}
	if entry.time.IsZero() {
		entry.time = timeNow()
	}
	m.ledger = append(m.ledger, entry)
	return m
}

// applyConsensusMsg updates a consensus member's vote state and the overall
// consensus result.
func (m Model) applyConsensusMsg(msg ConsensusMsg) Model {
	for i, member := range m.consensus {
		if member.name == msg.Member {
			m.consensus[i].decision = msg.Decision
			m.consensus[i].signed = msg.Signed
			break
		}
	}
	if msg.Quorum > 0 {
		m.quorum = msg.Quorum
	}
	if msg.Total > 0 {
		m.total = msg.Total
	}
	if msg.Result != ConsensusPending {
		m.result = msg.Result
	}
	if msg.Hash != "" {
		m.consensusHash = msg.Hash
	}
	return m
}
