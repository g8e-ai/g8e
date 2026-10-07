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

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	clioperator "github.com/g8e-ai/g8e/v2/internal/cli/operator"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
)

// Layout sizes are outer sizes: a pane's height and width include its border.
const (
	defaultTerminalWidth  = 100
	defaultTerminalHeight = 30
	leftPaneWidthRatio    = 2 // numerator; left pane = width * leftPaneWidthRatio / leftPaneWidthDivisor
	leftPaneWidthDivisor  = 5
	headerLines           = 2
	tabLines              = 1
	statusBarLines        = 1
	chromeLines           = headerLines + tabLines + statusBarLines
	listPaneRows          = 6 // content rows in the overview's approvals and Operators panes, including the pane header
	listPaneHeight        = listPaneRows + 2
	minTopHeight          = 10
	minBodyHeight         = minTopHeight + listPaneHeight
	paneTitleLines        = 2 // a pane's title and the blank line under it
	hashDisplayLen        = 8
)

// View implements tea.Model. It renders the header, the view tabs, the
// active view (or the help or confirmation overlay), and the status bar.
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
	bodyHeight := max(height-chromeLines, minBodyHeight)

	var body string
	switch {
	case m.confirm != nil || m.showHelp:
		body = m.renderOverlay(width, bodyHeight)
	case m.view == viewApprovals:
		body = m.renderApprovalsView(width, bodyHeight)
	case m.view == viewOperators:
		body = m.renderOperatorsView(width, bodyHeight)
	case m.view == viewOperatorDetails:
		body = m.renderOperatorDetailsView(width, bodyHeight)
	case m.view == viewEnrollments:
		body = m.renderEnrollmentsView(width, bodyHeight)
	case m.view == viewAudit:
		body = m.renderAuditView(width, bodyHeight)
	case m.view == viewGatewayStatus:
		body = m.renderGatewayStatusView(width, bodyHeight)
	case m.view == viewRecovery:
		body = m.renderRecoveryView(width, bodyHeight)
	default:
		body = m.renderOverview(width, bodyHeight)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(width),
		m.renderTabs(width),
		body,
		m.renderStatusBar(width),
	)
}

// renderOverview renders the overview: the pipeline and ledger over the
// pending-approval and Operator panes.
func (m Model) renderOverview(width, height int) string {
	leftWidth := width * leftPaneWidthRatio / leftPaneWidthDivisor
	topHeight := max(height-listPaneHeight, minTopHeight)
	topRow := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderPipeline(leftWidth, topHeight),
		m.renderLedger(width-leftWidth, topHeight),
	)
	halfWidth := width / 2
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderApprovals(halfWidth),
		m.renderOperators(width-halfWidth),
	)
	return lipgloss.JoinVertical(lipgloss.Left, topRow, bottomRow)
}

// renderHeader renders two lines: the console title and the Gateway posture,
// then the same identity 'g8e auth' reports. Each line is cut to width.
func (m Model) renderHeader(width int) string {
	title := " g8e TACTICAL GOVERNANCE CONSOLE " + m.version
	posture := "POSTURE: " + m.postureLabel() + " "
	gap := max(width-lipgloss.Width(title)-lipgloss.Width(posture), 1)

	operator := m.identity.OperatorID
	if operator == "" {
		operator = "unbound"
	}
	identity := fmt.Sprintf(" USER: %s | CLI SESSION: %s | OPERATOR: %s",
		m.identity.UserID, shortHash(m.identity.CLISessionID), operator)
	if m.gatewayVersion != "" {
		identity += " | GATEWAY: " + m.gatewayVersion
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		fitLine(headerStyle.Render(title+strings.Repeat(" ", gap)+posture), width),
		fitLine(detailStyle.Render(identity), width),
	)
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

// box renders lines in style's bordered pane at exactly width x height
// (outer size). Lines are cut to the inner width and the inner height, so
// content never wraps or pushes the layout past the terminal.
func box(style lipgloss.Style, width, height int, lines []string) string {
	innerWidth := max(width-style.GetHorizontalFrameSize(), 1)
	innerHeight := max(height-style.GetVerticalFrameSize(), 1)
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}
	fitted := make([]string, len(lines))
	for i, line := range lines {
		fitted[i] = fitLine(line, innerWidth)
	}
	return style.
		Width(max(width-style.GetHorizontalBorderSize(), 1)).
		Height(max(height-style.GetVerticalBorderSize(), 1)).
		Render(strings.Join(fitted, "\n"))
}

// fitLine cuts a single (possibly styled) line to width cells.
func fitLine(line string, width int) string {
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// innerWidth is the content width of a pane of outer width.
func innerWidth(width int) int {
	return max(width-borderPane.GetHorizontalFrameSize(), 1)
}

// renderPipeline renders the left pane: the L1-L5 execution pipeline.
func (m Model) renderPipeline(width, height int) string {
	header := pipelineHeaderStyle.Render("EXECUTION PIPELINE (L1-L5)")

	// Drop the spacer lines when the pane is too short to show every stage
	// with them: a title, then a label, a detail, and a spacer per stage.
	spaced := height-borderPane.GetVerticalFrameSize() >= paneTitleLines+3*len(m.pipeline)

	lines := []string{header}
	if spaced {
		lines = append(lines, "")
	}

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
		lines = append(lines, "    "+detailStyle.Render(detail))
		if spaced {
			lines = append(lines, "")
		}
	}

	return box(borderPane, width, height, lines)
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

// renderLedger renders the right pane: the Sovereign Audit Ledger. Entries
// wrap to the pane width so long lines (such as an approval URL to visit)
// stay readable; the newest lines that fit are shown.
func (m Model) renderLedger(width, height int) string {
	lines := []string{ledgerHeaderStyle.Render("SOVEREIGN AUDIT LEDGER"), ""}

	visibleEntries := m.ledger
	if m.ledgerScroll > 0 && m.ledgerScroll < len(m.ledger) {
		visibleEntries = m.ledger[:len(m.ledger)-m.ledgerScroll]
	}

	wrap := lipgloss.NewStyle().Width(innerWidth(width))
	maxLines := max(height-borderPane.GetVerticalFrameSize()-paneTitleLines, 1)
	var body []string
	// Walk back from the newest entry until the pane is full.
	for i := len(visibleEntries) - 1; i >= 0 && len(body) < maxLines; i-- {
		entry := visibleEntries[i]
		text := fmt.Sprintf("%s %s %s", entry.time.Format("15:04:05"), entry.level.Tag(), entry.message)
		wrapped := strings.Split(wrap.Render(text), "\n")
		for j := range wrapped {
			wrapped[j] = ledgerLevelStyle(entry.level).Render(strings.TrimRight(wrapped[j], " "))
		}
		body = append(wrapped, body...)
	}
	if len(body) > maxLines {
		body = body[len(body)-maxLines:]
	}
	lines = append(lines, body...)

	if len(m.ledger) == 0 {
		lines = append(lines, detailStyle.Render("(awaiting events...)"))
	}

	return box(m.paneBorder(paneLedger), width, height, lines)
}

// ledgerLevelStyle returns the text style for a ledger level.
func ledgerLevelStyle(level LedgerLevel) lipgloss.Style {
	switch level {
	case LevelCritical:
		return ledgerCritStyle
	case LevelWarn:
		return ledgerWarnStyle
	default:
		return ledgerInfoStyle
	}
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
	return box(m.paneBorder(paneApprovals), width, listPaneHeight, lines)
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
		row := fmt.Sprintf("%s %s %s %s", operatorHostname(op), operatorcapability.GetOperatorRoles(op), op.Status, shortHash(op.Id))
		if op.Id == m.identity.OperatorID {
			row += " [bound]"
		}
		lines = append(lines, m.listRow(paneOperators, i == m.operatorsSelected, row))
	}
	return box(m.paneBorder(paneOperators), width, listPaneHeight, lines)
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
	return windowFor(n, selected, listPaneRows-1) // the header takes one row
}

// operatorHostname names an Operator for display, as 'g8e gw status' does.
func operatorHostname(op *operatorv1.OperatorDocument) string {
	if view := clioperator.HeartbeatViewFromResult(op.LatestHeartbeatSnapshot); view != nil && view.SystemIdentity.Hostname != "" {
		return view.SystemIdentity.Hostname
	}
	if op.CurrentHostname != "" {
		return op.CurrentHostname
	}
	if op.GetName() != "" {
		return op.GetName()
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

	right := m.statusHints()

	gap := max(width-lipgloss.Width(connPart)-lipgloss.Width(right), 1)
	return fitLine(statusBarStyle.Render(connPart+strings.Repeat(" ", gap)+right), width)
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
