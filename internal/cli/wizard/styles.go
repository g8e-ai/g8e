// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package wizard

import "github.com/charmbracelet/lipgloss"

// Color palette — matches the TUI package for visual consistency.
var (
	colorBorder   = lipgloss.Color("63")  // tech blue
	colorMuted    = lipgloss.Color("245") // gray
	colorNormal   = lipgloss.Color("250") // light gray
	colorPassed   = lipgloss.Color("34")  // muted green
	colorCritical = lipgloss.Color("196") // bright red
	colorHeader   = lipgloss.Color("39")  // bright blue
	colorSelected = lipgloss.Color("226") // bright yellow
)

var (
	borderStyle = lipgloss.NewStyle().
			BorderForeground(colorBorder).
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader).
			MarginBottom(1)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorCritical).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(colorSelected).
			Bold(true)

	passedStyle = lipgloss.NewStyle().
			Foreground(colorPassed)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(22)

	valueStyle = lipgloss.NewStyle().
			Foreground(colorNormal)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)
)
