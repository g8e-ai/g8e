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
	"github.com/charmbracelet/lipgloss"
)

// Color palette: muted for normal ops, violent for threats.
var (
	colorBorder   = lipgloss.Color("63")  // tech blue
	colorMuted    = lipgloss.Color("245") // gray
	colorNormal   = lipgloss.Color("250") // light gray
	colorPassed   = lipgloss.Color("34")  // muted green
	colorWaiting  = lipgloss.Color("226") // bright yellow
	colorCritical = lipgloss.Color("196") // bright red
	colorApprove  = lipgloss.Color("34")  // green
	colorVeto     = lipgloss.Color("196") // red
	colorPending  = lipgloss.Color("245") // gray
	colorHeader   = lipgloss.Color("39")  // bright blue
	colorWarn     = lipgloss.Color("208") // orange
)

// Border styles for the three panes.
var (
	baseBorder = lipgloss.NewStyle().
			BorderForeground(colorBorder).
			Padding(0, 1)

	borderPipeline  = baseBorder.Border(lipgloss.RoundedBorder())
	borderLedger    = baseBorder.Border(lipgloss.RoundedBorder())
	borderConsensus = baseBorder.Border(lipgloss.RoundedBorder(), false, true, true, true)
)

// Text styles.
var (
	pipelineHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorHeader).
				MarginBottom(0)

	ledgerHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorHeader).
				MarginBottom(0)

	consensusHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorHeader)

	stagePassedStyle = lipgloss.NewStyle().
				Foreground(colorPassed)

	stageWaitingStyle = lipgloss.NewStyle().
				Foreground(colorWaiting).
				Bold(true)

	stageFailedStyle = lipgloss.NewStyle().
				Foreground(colorCritical).
				Bold(true)

	stageIdleStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	stageActiveStyle = lipgloss.NewStyle().
				Foreground(colorNormal).
				Bold(true)

	detailStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	ledgerInfoStyle = lipgloss.NewStyle().
			Foreground(colorNormal)

	ledgerWarnStyle = lipgloss.NewStyle().
			Foreground(colorWarn)

	ledgerCritStyle = lipgloss.NewStyle().
			Foreground(colorCritical).
			Bold(true)

	consensusApproveStyle = lipgloss.NewStyle().
				Foreground(colorApprove).
				Bold(true)

	consensusVetoStyle = lipgloss.NewStyle().
				Foreground(colorVeto).
				Bold(true)

	consensusPendingStyle = lipgloss.NewStyle().
				Foreground(colorPending)

	consensusStatusStyle = lipgloss.NewStyle().
				Foreground(colorNormal)

	consensusRejectStyle = lipgloss.NewStyle().
				Foreground(colorCritical).
				Bold(true)

	consensusApproveStatusStyle = lipgloss.NewStyle().
					Foreground(colorPassed).
					Bold(true)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted)
)

// statusIcon returns the display icon for a pipeline stage status.
func statusIcon(s PipelineStatus) string {
	switch s {
	case StatusPassed:
		return "[x]"
	case StatusWaiting:
		return "[!]"
	case StatusFailed:
		return "[X]"
	case StatusActive:
		return "[>]"
	default:
		return "[ ]"
	}
}

// voteIcon returns the display icon for a consensus member vote.
func voteIcon(m consensusMemberState) string {
	if !m.signed {
		return "[...]"
	}
	if m.decision {
		return "[YES]"
	}
	return "[NO ]"
}
