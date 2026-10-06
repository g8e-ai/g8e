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
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
)

const (
	defaultTerminalWidth  = 100
	defaultTerminalHeight = 30
	leftPaneWidthRatio    = 2 // numerator; left pane = width * leftPaneWidthRatio / leftPaneWidthDivisor
	leftPaneWidthDivisor  = 5
	listPaneRows          = 6 // visible rows in the approvals and Operators panes
	listPaneHeight        = listPaneRows + 2
	reservedLines         = 2 + listPaneHeight + 2 // header + status bar + list pane and its border
	minTopHeight          = 10
	ledgerReservedLines   = 4 // header + border padding
	hashDisplayLen        = 8
)

// View implements tea.Model. It renders the header, the pipeline and ledger,
// the pending-approval and Operator panes, and the status bar.
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

	leftWidth := width * leftPaneWidthRatio / leftPaneWidthDivisor
	rightWidth := width - leftWidth

	topHeight := max(height-reservedLines, minTopHeight)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderPipeline(leftWidth, topHeight),
		m.renderLedger(rightWidth, topHeight),
	)
	halfWidth := width / 2
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderApprovals(halfWidth),
		m.renderOperators(width-halfWidth),
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(width),
		topRow,
		bottomRow,
		m.renderStatusBar(width),
	)
}

// renderHeader renders the identity and governance posture line: the same
// identity 'g8e auth' reports and the posture the Gateway runs.
func (m Model) renderHeader(width int) string {
	operator := m.identity.OperatorID
	if operator == "" {
		operator = "unbound"
	}
	left := fmt.Sprintf(" g8e TACTICAL GOVERNANCE CONSOLE %s | USER: %s | CLI SESSION: %s | OPERATOR: %s",
		m.version, m.identity.UserID, shortHash(m.identity.CLISessionID), operator)
	right := "POSTURE: " + m.postureLabel() + " "
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return headerStyle.Render(left + strings.Repeat(" ", gap) + right)
}

// postureLabel names the Gateway posture and which of L2/L3 it enforces.
func (m Model) postureLabel() string {
	if m.posture == nil {
		return "UNKNOWN"
	}
	return fmt.Sprintf("%s (L2 %s, L3 %s)", strings.ToUpper(m.posture.Name()),
		enforcement(m.posture.RequiresL2Signature()), enforcement(m.posture.RequiresL3Proof()))
}

func enforcement(enforced bool) string {
	if enforced {
		return "enforced"
	}
	return "audited"
}

// paneBorder returns the border style for p, highlighted when it has focus.
func (m Model) paneBorder(p pane) lipgloss.Style {
	if m.focus == p {
		return borderFocused
	}
	return borderPane
}

// renderPipeline renders the left pane: the L1-L5 execution pipeline.
func (m Model) renderPipeline(width, height int) string {
	header := pipelineHeaderStyle.Render("EXECUTION PIPELINE (L1-L5)")

	var lines []string
	lines = append(lines, header, "")

	for i, stage := range m.pipeline {
		icon := statusIcon(stage.status)
		label := PipelineStage(i).String() + m.stageAnnotation(PipelineStage(i))

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
	return borderPane.Width(width).Height(height).Render(content)
}

// stageAnnotation marks L2 and L3 as audited when the posture does not
// enforce them, so a passing stage is not mistaken for an enforced gate.
func (m Model) stageAnnotation(stage PipelineStage) string {
	if m.posture == nil {
		return ""
	}
	switch {
	case stage == StageL2 && !m.posture.RequiresL2Signature(),
		stage == StageL3 && !m.posture.RequiresL3Proof():
		return " (audited)"
	default:
		return ""
	}
}

// renderLedger renders the right pane: the Sovereign Audit Ledger.
func (m Model) renderLedger(width, height int) string {
	header := ledgerHeaderStyle.Render("SOVEREIGN AUDIT LEDGER")

	var lines []string
	lines = append(lines, header, "")

	visibleEntries := m.ledger
	if m.ledgerScroll > 0 && m.ledgerScroll < len(m.ledger) {
		scrollOffset := len(m.ledger) - m.ledgerScroll
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
	return m.paneBorder(paneLedger).Width(width).Height(height).Render(content)
}

// renderApprovals renders the pending L3 approval queue with the selection.
func (m Model) renderApprovals(width int) string {
	lines := []string{paneHeaderStyle.Render(fmt.Sprintf("PENDING APPROVALS (%d)", len(m.pending)))}
	if len(m.pending) == 0 {
		lines = append(lines, detailStyle.Render("(no pending approvals)"))
	}
	start, end := listWindow(len(m.pending), m.pendingSelected)
	now := timeNow()
	for i := start; i < end; i++ {
		tx := m.pending[i]
		row := fmt.Sprintf("%s tx %s", toolLabel(tx.ToolName), shortHash(tx.TransactionHash))
		if !tx.ExpiresAt.IsZero() {
			row += " expires " + formatRemaining(tx.ExpiresAt.Sub(now))
		}
		if _, ok := m.awaitingApproval[tx.TransactionHash]; ok {
			row += " [awaiting browser]"
		}
		lines = append(lines, m.listRow(paneApprovals, i == m.pendingSelected, row))
	}
	return m.paneBorder(paneApprovals).Width(width).Height(listPaneHeight).Render(strings.Join(lines, "\n"))
}

// renderOperators renders the connected Operators with the selection.
func (m Model) renderOperators(width int) string {
	header := "OPERATORS"
	if m.operatorsLoaded {
		header = fmt.Sprintf("OPERATORS (%d connected / %d)", len(m.operators), m.operatorsTotal)
	}
	lines := []string{paneHeaderStyle.Render(header)}
	switch {
	case m.operatorsErr != "":
		lines = append(lines, ledgerWarnStyle.Render("unavailable: "+m.operatorsErr))
	case !m.operatorsLoaded:
		lines = append(lines, detailStyle.Render("(loading...)"))
	case len(m.operators) == 0:
		lines = append(lines, detailStyle.Render("(no connected operators)"))
	}
	start, end := listWindow(len(m.operators), m.operatorsSelected)
	for i := start; i < end; i++ {
		op := m.operators[i]
		row := fmt.Sprintf("%s %s %s %s", operatorHostname(op), operatorcapability.GetOperatorRoles(op), op.Status, shortHash(op.ID))
		if op.ID == m.identity.OperatorID {
			row += " [bound]"
		}
		lines = append(lines, m.listRow(paneOperators, i == m.operatorsSelected, row))
	}
	return m.paneBorder(paneOperators).Width(width).Height(listPaneHeight).Render(strings.Join(lines, "\n"))
}

// listRow renders one list row, marking the selection when its pane has focus.
func (m Model) listRow(p pane, selected bool, row string) string {
	if selected && m.focus == p {
		return selectedRowStyle.Render("> " + row)
	}
	return ledgerInfoStyle.Render("  " + row)
}

// listWindow returns the [start, end) rows of an n-row list to show so the
// selected row stays visible.
func listWindow(n, selected int) (int, int) {
	rows := listPaneRows - 1 // the header takes one row
	start := 0
	if selected >= rows {
		start = selected - rows + 1
	}
	return start, min(n, start+rows)
}

// operatorHostname names an Operator for display, as 'g8e gw status' does.
func operatorHostname(op models.OperatorDocumentGo) string {
	if op.CurrentHostname != "" {
		return op.CurrentHostname
	}
	if op.Name != "" {
		return op.Name
	}
	return "-"
}

// formatRemaining renders a time-to-expiry, or "expired".
func formatRemaining(d time.Duration) string {
	if d <= 0 {
		return "expired"
	}
	return d.Truncate(time.Second).String()
}

// renderStatusBar renders the connection state and the key hints.
func (m Model) renderStatusBar(width int) string {
	var connPart string
	switch m.connStatus {
	case ConnConnected:
		connPart = " SSE: CONNECTED"
	case ConnConnecting:
		connPart = " SSE: CONNECTING..."
	case ConnReconnecting:
		connPart = " SSE: RECONNECTING..."
	case ConnFailed:
		connPart = " SSE: DISCONNECTED"
	default:
		connPart = " SSE: IDLE"
	}
	if m.connDetail != "" {
		connPart += " (" + m.connDetail + ")"
	}

	right := "tab: focus | j/k: move | a: approve | r: refresh | q: quit "

	gap := max(width-lipgloss.Width(connPart)-lipgloss.Width(right), 1)
	return statusBarStyle.Render(connPart + strings.Repeat(" ", gap) + right)
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
