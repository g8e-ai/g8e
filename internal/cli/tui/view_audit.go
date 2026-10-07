// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func (m Model) renderAuditView(width, height int) string {
	leftWidth, rightWidth := splitWidth(width)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		box(borderPane, leftWidth, height, m.auditEventLines()),
		box(borderPane, rightWidth, height, m.auditDetailLines(rightWidth)),
	)
}

func (m Model) auditEventLines() []string {
	title := "AUDIT EVENTS"
	if m.auditEventsLoaded {
		title = fmt.Sprintf("AUDIT EVENTS (offset %d)", m.auditOffset)
	}
	lines := []string{paneHeaderStyle.Render(title), ""}
	switch {
	case m.auditEventsErr != "":
		lines = append(lines, ledgerWarnStyle.Render("unavailable: "+m.auditEventsErr))
	case !m.auditEventsLoaded:
		lines = append(lines, detailStyle.Render("(loading...)"))
	case len(m.auditEvents) == 0:
		lines = append(lines, detailStyle.Render("(no audit events)"))
	default:
		for i, event := range m.auditEvents {
			timestamp := event.Timestamp
			if len(timestamp) > 19 {
				timestamp = timestamp[:19]
			}
			row := fmt.Sprintf("%-6d %-19s", event.ID, timestamp)
			if event.Type != "" {
				row += " " + event.Type
			}
			lines = append(lines, selectableRow(i == m.auditSelected, row))
		}
		lines = append(lines, "", detailStyle.Render(fmt.Sprintf("page: %d event(s)", m.auditEventsCount)))
	}
	return lines
}

func (m Model) auditDetailLines(width int) []string {
	lines := []string{paneHeaderStyle.Render("AUDIT DETAILS"), ""}
	if event, ok := m.selectedAuditEvent(); ok {
		lines = append(lines,
			detailRow("ID", fmt.Sprint(event.ID)),
			detailRow("TYPE", event.Type),
			detailRow("TIMESTAMP", event.Timestamp),
			detailRow("TRANSACTION", event.TransactionID),
			detailRow("OPERATOR SESSION", event.OperatorSessionID),
			detailRow("EXIT CODE", auditExitCode(event.CommandExitCode)),
			"",
			paneHeaderStyle.Render("COMMAND"),
		)
		lines = append(lines, wrapDetail(event.CommandRaw, width)...)
	} else {
		lines = append(lines, detailStyle.Render("Select an event for details"))
	}

	lines = append(lines, "", paneHeaderStyle.Render("AUDIT SUMMARY"))
	switch {
	case m.auditSummaryErr != "":
		lines = append(lines, ledgerWarnStyle.Render("unavailable: "+m.auditSummaryErr))
	case !m.auditSummaryLoaded:
		lines = append(lines, detailStyle.Render("(loading...)"))
	default:
		lines = append(lines,
			detailRow("EVENTS", fmt.Sprint(m.auditSummary.EventsTotal)),
			detailRow("RECEIPTS", fmt.Sprint(m.auditSummary.ReceiptsTotal)),
			detailRow("TOTAL", fmt.Sprint(m.auditSummary.TotalRecords)),
		)
		for _, key := range sortedAuditKeys(m.auditSummary.EventsSummary) {
			lines = append(lines, detailRow(key, fmt.Sprint(m.auditSummary.EventsSummary[key])))
		}
	}

	lines = append(lines, "", paneHeaderStyle.Render("CHAIN VERIFICATION"))
	switch {
	case m.auditVerifyErr != "":
		lines = append(lines, ledgerWarnStyle.Render("unavailable: "+m.auditVerifyErr))
	case m.auditVerify == nil:
		lines = append(lines, detailStyle.Render("(press v to verify)"))
	case m.auditVerify.OK:
		lines = append(lines,
			ledgerInfoStyle.Render("OK"),
			detailRow("FROM SEQ", fmt.Sprint(m.auditVerify.VerifiedFromSeq)),
			detailRow("HEAD SEQ", fmt.Sprint(m.auditVerify.HeadSeq)),
			detailRow("HEAD HASH", m.auditVerify.HeadHash),
		)
	default:
		errorText := m.auditVerify.Error
		if errorText == "" {
			errorText = "verification failed"
		}
		lines = append(lines, ledgerWarnStyle.Render("FAILED: "+errorText))
	}
	return lines
}

func (m Model) selectedAuditEvent() (models.AuditEventRow, bool) {
	if m.auditSelected < 0 || m.auditSelected >= len(m.auditEvents) {
		return models.AuditEventRow{}, false
	}
	return m.auditEvents[m.auditSelected], true
}

func auditExitCode(code int) string {
	if code == constants.ExitCodeNone {
		return "-"
	}
	return fmt.Sprint(code)
}

func sortedAuditKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func wrapDetail(value string, width int) []string {
	if value == "" {
		return []string{detailStyle.Render("-")}
	}
	wrapped := lipgloss.NewStyle().Width(max(width-borderPane.GetHorizontalFrameSize(), 1)).Render(value)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		lines[i] = ledgerInfoStyle.Render(lines[i])
	}
	return lines
}
