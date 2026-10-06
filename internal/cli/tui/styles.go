// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
	colorHeader   = lipgloss.Color("39")  // bright blue
	colorWarn     = lipgloss.Color("208") // orange
	colorFocus    = lipgloss.Color("226") // bright yellow
)

// Border styles for the panes; the focused pane is highlighted.
var (
	borderPane = lipgloss.NewStyle().
			BorderForeground(colorBorder).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder())

	borderFocused = borderPane.BorderForeground(colorFocus)
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

	paneHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader)

	selectedRowStyle = lipgloss.NewStyle().
				Foreground(colorWaiting).
				Bold(true)

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

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	tabStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	tabActiveStyle = lipgloss.NewStyle().
			Foreground(colorFocus).
			Bold(true)
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
