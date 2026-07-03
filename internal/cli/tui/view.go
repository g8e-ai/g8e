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
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultTerminalWidth  = 100
	defaultTerminalHeight = 30
	leftPaneWidthRatio    = 2 // numerator; left pane = width * leftPaneWidthRatio / leftPaneWidthDivisor
	leftPaneWidthDivisor  = 5
	reservedBottomLines   = 7 // tribunal pane + status bar
	minTopHeight          = 10
	ledgerReservedLines   = 4 // header + border padding
	hashDisplayLen        = 8
)

// View implements tea.Model. It renders the three-pane Tactical Governance Console.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	width := m.width
	if width == 0 {
		width = defaultTerminalWidth
	}
	height := m.height
	if height == 0 {
		height = defaultTerminalHeight
	}

	// Calculate pane widths: left 40%, right 60%
	leftWidth := width * leftPaneWidthRatio / leftPaneWidthDivisor
	rightWidth := width - leftWidth

	// Top section: pipeline + ledger side by side
	topHeight := height - reservedBottomLines
	if topHeight < minTopHeight {
		topHeight = minTopHeight
	}

	leftPane := m.renderPipeline(leftWidth, topHeight)
	rightPane := m.renderLedger(rightWidth, topHeight)
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	// Bottom section: tribunal consensus
	tribunalPane := m.renderTribunal(width)

	// Status bar
	statusBar := m.renderStatusBar(width)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, tribunalPane, statusBar)
}

// renderPipeline renders the left pane: the L1-L5 execution pipeline.
func (m Model) renderPipeline(width, height int) string {
	header := pipelineHeaderStyle.Render("EXECUTION PIPELINE (L1-L5)")

	var lines []string
	lines = append(lines, header, "")

	for i, stage := range m.pipeline {
		icon := statusIcon(stage.status)
		label := PipelineStage(i).String()

		var styled string
		switch stage.status {
		case StatusPassed:
			styled = stagePassedStyle.Render(fmt.Sprintf("%s %s", icon, label))
		case StatusWaiting:
			if m.blinkOn {
				styled = stageWaitingStyle.Render(fmt.Sprintf("%s %s", icon, label))
			} else {
				styled = stageIdleStyle.Render(fmt.Sprintf("%s %s", icon, label))
			}
		case StatusFailed:
			if m.blinkOn {
				styled = stageFailedStyle.Render(fmt.Sprintf("%s %s", icon, label))
			} else {
				styled = stageIdleStyle.Render(fmt.Sprintf("%s %s", icon, label))
			}
		case StatusActive:
			styled = stageActiveStyle.Render(fmt.Sprintf("%s %s", icon, label))
		default:
			styled = stageIdleStyle.Render(fmt.Sprintf("%s %s", icon, label))
		}
		lines = append(lines, styled)

		detail := stage.detail
		if detail == "" {
			detail = stage.status.String()
		}
		lines = append(lines, "    "+detailStyle.Render(detail), "")
	}

	content := strings.Join(lines, "\n")
	return borderPipeline.Width(width).Height(height).Render(content)
}

// renderLedger renders the right pane: the Sovereign Audit Ledger.
func (m Model) renderLedger(width, height int) string {
	header := ledgerHeaderStyle.Render("SOVEREIGN AUDIT LEDGER")

	var lines []string
	lines = append(lines, header, "")

	visibleEntries := m.ledger
	scrollOffset := 0
	if m.ledgerScroll > 0 && m.ledgerScroll < len(m.ledger) {
		scrollOffset = len(m.ledger) - m.ledgerScroll
		visibleEntries = m.ledger[:scrollOffset]
	}

	maxLines := height - ledgerReservedLines
	start := 0
	if len(visibleEntries) > maxLines {
		start = len(visibleEntries) - maxLines
	}
	visibleEntries = visibleEntries[start:]

	for _, entry := range visibleEntries {
		ts := entry.time.Format("15:04:05")
		var line string
		switch entry.level {
		case LevelCritical:
			line = ledgerCritStyle.Render(fmt.Sprintf("%s %s %s", ts, entry.level.Tag(), entry.message))
		case LevelWarn:
			line = ledgerWarnStyle.Render(fmt.Sprintf("%s %s %s", ts, entry.level.Tag(), entry.message))
		default:
			line = ledgerInfoStyle.Render(fmt.Sprintf("%s %s %s", ts, entry.level.Tag(), entry.message))
		}
		lines = append(lines, line)
	}

	if len(m.ledger) == 0 {
		lines = append(lines, detailStyle.Render("(awaiting events...)"))
	}

	content := strings.Join(lines, "\n")
	return borderLedger.Width(width).Height(height).Render(content)
}

// renderTribunal renders the bottom pane: the L2 Tribunal Consensus status.
func (m Model) renderTribunal(width int) string {
	header := tribunalHeaderStyle.Render(
		fmt.Sprintf("L2 TRIBUNAL CONSENSUS (k-of-n: %d/%d required)", m.quorum, m.total),
	)

	var memberBlocks []string
	for _, member := range m.tribunal {
		icon := voteIcon(member)
		name := strings.ToUpper(string(member.name))

		var styled string
		switch {
		case !member.signed:
			styled = tribunalPendingStyle.Render(fmt.Sprintf("%s %s", icon, name))
		case member.decision:
			styled = tribunalApproveStyle.Render(fmt.Sprintf("%s %s", icon, name))
		default:
			styled = tribunalVetoStyle.Render(fmt.Sprintf("%s %s", icon, name))
		}
		memberBlocks = append(memberBlocks, styled)
	}

	membersLine := lipgloss.JoinHorizontal(lipgloss.Center, memberBlocks...)

	var statusLine string
	switch m.result {
	case ConsensusReached:
		statusLine = tribunalApproveStatusStyle.Render(
			fmt.Sprintf("STATUS: %s. HASH: %s", m.result, shortHash(m.consensusHash)),
		)
	case ConsensusRejected:
		statusLine = tribunalRejectStyle.Render(
			fmt.Sprintf("STATUS: %s. HASH: %s", m.result, shortHash(m.consensusHash)),
		)
	default:
		affirmative := m.countAffirmative()
		statusLine = tribunalStatusStyle.Render(
			fmt.Sprintf("STATUS: %s (%d/%d signed). %s", m.result, affirmative, m.total, "AWAITING VOTES..."),
		)
	}

	content := header + "\n\n" + membersLine + "\n\n" + statusLine
	return borderTribunal.Width(width).Render(content)
}

// renderStatusBar renders the bottom status bar with version, node, network, and connection info.
func (m Model) renderStatusBar(width int) string {
	left := fmt.Sprintf(" g8e OPERATOR CONSOLE %s | NODE: %s | NET: %s",
		m.version, m.nodeName, m.netLabel)

	var connPart string
	switch m.connStatus {
	case ConnConnected:
		connPart = "SSE: CONNECTED"
	case ConnConnecting:
		connPart = "SSE: CONNECTING..."
	case ConnReconnecting:
		connPart = "SSE: RECONNECTING..."
	case ConnFailed:
		connPart = "SSE: DISCONNECTED"
	default:
		connPart = "SSE: IDLE"
	}
	if m.connDetail != "" {
		connPart += " (" + m.connDetail + ")"
	}

	right := connPart + " | q: quit | j/k: scroll"

	gap := width - len(left) - len(right)
	if gap < 1 {
		gap = 1
	}

	return statusBarStyle.Render(left + strings.Repeat(" ", gap) + right)
}

// countAffirmative returns the number of tribunal members who voted yes.
func (m Model) countAffirmative() int {
	count := 0
	for _, member := range m.tribunal {
		if member.signed && member.decision {
			count++
		}
	}
	return count
}

// shortHash truncates a hash to hashDisplayLen characters with ellipsis for display.
func shortHash(hash string) string {
	if len(hash) <= hashDisplayLen {
		return hash
	}
	return hash[:hashDisplayLen] + "..."
}

// timeNow returns the current time. It is a package-level variable so tests
// can override it for deterministic behavior.
var timeNow = func() time.Time { return time.Now() }

// Ensure Model satisfies tea.Model at compile time.
var _ tea.Model = (*Model)(nil)
