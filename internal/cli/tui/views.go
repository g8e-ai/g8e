// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// viewID identifies a main-area view. The header, the view tabs, and the
// status bar stay put while number keys switch the main area.
type viewID int

const (
	viewOverview viewID = iota
	viewApprovals
	viewOperators
	viewEnrollments
)

// viewSpec binds a view to its number key. Keys follow the TUI roadmap so a
// view keeps its key as others are added (4 chat, 5 audit, 7 account are
// reserved).
type viewSpec struct {
	key   string
	id    viewID
	title string
}

var viewSpecs = []viewSpec{
	{key: "1", id: viewOverview, title: "Overview"},
	{key: "2", id: viewApprovals, title: "Approvals"},
	{key: "3", id: viewOperators, title: "Operators"},
	{key: "6", id: viewEnrollments, title: "Enrollments"},
}

// viewForKey returns the view bound to a number key.
func viewForKey(key string) (viewID, bool) {
	for _, spec := range viewSpecs {
		if spec.key == key {
			return spec.id, true
		}
	}
	return 0, false
}

// confirmation is a pending y/N prompt guarding a mutating action. run is
// issued only on 'y'.
type confirmation struct {
	prompt string
	run    tea.Cmd
}

// handleConfirmKey answers the open confirmation: 'y' runs the action, 'n'
// or esc cancels it, and any other key is ignored.
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		run := m.confirm.run
		m.confirm = nil
		return m, run
	case "n", "N", "esc":
		m = m.applyLedgerMsg(LedgerMsg{Level: LevelInfo, Message: "Cancelled: " + m.confirm.prompt})
		m.confirm = nil
	}
	return m, nil
}

// renderTabs renders the view tabs with live counts, highlighting the
// active view.
func (m Model) renderTabs(width int) string {
	parts := make([]string, 0, len(viewSpecs))
	for _, spec := range viewSpecs {
		label := spec.key + " " + spec.title + m.viewCount(spec.id)
		if spec.id == m.view {
			parts = append(parts, tabActiveStyle.Render("["+label+"]"))
		} else {
			parts = append(parts, tabStyle.Render(" "+label+" "))
		}
	}
	return fitLine(" "+strings.Join(parts, " ")+tabStyle.Render("  ?: help"), width)
}

// viewCount is the count shown on a view's tab, or "" when there is none.
func (m Model) viewCount(id viewID) string {
	switch id {
	case viewApprovals:
		return fmt.Sprintf(" (%d)", len(m.pending))
	case viewOperators:
		if m.operatorsLoaded {
			return fmt.Sprintf(" (%d)", len(m.operators))
		}
	case viewEnrollments:
		if m.enrollmentsLoaded {
			return fmt.Sprintf(" (%d)", len(m.enrollPending))
		}
	}
	return ""
}

// helpLines lists the keys; the active view's keys come first.
func (m Model) helpLines() []string {
	lines := []string{paneHeaderStyle.Render("KEYS"), ""}
	switch m.view {
	case viewOverview:
		lines = append(lines,
			"tab / shift+tab  focus the ledger, approvals, or Operators pane",
			"j/k, ↓/↑         move in the focused pane (ledger: newer / older)",
			"G / g            ledger bottom (newest) / top (oldest)",
			"a, enter         approve the selected L3 transaction (browser WebAuthn)")
	case viewApprovals:
		lines = append(lines,
			"j/k, ↓/↑         select a pending L3 transaction",
			"a, enter         approve it (browser WebAuthn, as 'g8e auth approve')")
	case viewOperators:
		lines = append(lines,
			"j/k, ↓/↑         select a connected Operator",
			"s                stop the selected remote Operator (governed shutdown)")
	case viewEnrollments:
		lines = append(lines,
			"tab              switch between pending requests and enrollments",
			"j/k, ↓/↑         select",
			"a / d            approve / deny the selected pending request",
			"x                revoke the selected enrollment")
	}
	lines = append(lines, "",
		"1 2 3 6          switch view: overview, approvals, Operators, enrollments",
		"r                refresh approvals, Operators, enrollments, and posture",
		"?, esc           close this help",
		"q, ctrl+c        quit",
		"",
		detailStyle.Render("Every change asks y/N first."))
	return lines
}

// renderOverlay renders the help or the open confirmation over the main area.
func (m Model) renderOverlay(width, height int) string {
	if m.confirm != nil {
		lines := []string{paneHeaderStyle.Render("CONFIRM"), ""}
		lines = append(lines, strings.Split(lipgloss.NewStyle().Width(innerWidth(width)).Render(m.confirm.prompt), "\n")...)
		lines = append(lines, "", selectedRowStyle.Render("y: yes")+"   "+detailStyle.Render("n / esc: no"))
		return box(borderFocused, width, height, lines)
	}
	return box(borderFocused, width, height, m.helpLines())
}

// statusHints are the status bar's key hints for the active view.
func (m Model) statusHints() string {
	switch {
	case m.confirm != nil:
		return "y: confirm | n/esc: cancel "
	case m.showHelp:
		return "?/esc: close help | q: quit "
	}
	switch m.view {
	case viewApprovals:
		return "j/k: move | a: approve | r: refresh | ?: help | q: quit "
	case viewOperators:
		return "j/k: move | s: stop | r: refresh | ?: help | q: quit "
	case viewEnrollments:
		return "tab: section | j/k: move | a: approve | d: deny | x: revoke | ?: help | q: quit "
	default:
		return "tab: focus | j/k: move | a: approve | r: refresh | ?: help | q: quit "
	}
}

// detailRow renders a "label: value" line for a detail pane.
func detailRow(label, value string) string {
	if value == "" {
		value = "-"
	}
	return detailStyle.Render(fmt.Sprintf("%-14s", label+":")) + " " + ledgerInfoStyle.Render(value)
}

// windowFor returns the [start, end) rows of an n-row list to show in rows
// lines so the selected row stays visible.
func windowFor(n, selected, rows int) (int, int) {
	rows = max(rows, 1)
	start := 0
	if selected >= rows {
		start = selected - rows + 1
	}
	return start, min(n, start+rows)
}

// selectableRow renders a list row, marking it when selected.
func selectableRow(selected bool, row string) string {
	if selected {
		return selectedRowStyle.Render("> " + row)
	}
	return ledgerInfoStyle.Render("  " + row)
}

// splitWidth divides width into a left and right pane.
func splitWidth(width int) (int, int) {
	left := width / 2
	return left, width - left
}
