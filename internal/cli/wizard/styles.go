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

	normalStyle = lipgloss.NewStyle().
			Foreground(colorNormal)

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
