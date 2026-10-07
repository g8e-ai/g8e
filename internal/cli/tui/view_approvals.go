// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// detailTimeFormat is how detail panes show timestamps, as 'g8e auth enroll
// pending' does.
const detailTimeFormat = "2006-01-02 15:04:05 MST"

// renderApprovalsView renders every pending L3 transaction beside the
// selected one's details.
func (m Model) renderApprovalsView(width, height int) string {
	leftWidth, rightWidth := splitWidth(width)

	rows := height - borderFocused.GetVerticalFrameSize() - 1
	list := []string{paneHeaderStyle.Render(fmt.Sprintf("PENDING L3 APPROVALS (%d)", len(m.pending)))}
	if len(m.pending) == 0 {
		list = append(list, detailStyle.Render("(no pending approvals)"))
	}
	start, end := windowFor(len(m.pending), m.pendingSelected, rows)
	for i := start; i < end; i++ {
		tx := m.pending[i]
		row := fmt.Sprintf("%s tx %s", toolLabel(tx.ToolName), shortHash(tx.TransactionHash))
		if _, ok := m.awaitingApproval[tx.TransactionHash]; ok {
			row += " [awaiting browser]"
		}
		list = append(list, selectableRow(i == m.pendingSelected, row))
	}

	detail := []string{paneHeaderStyle.Render("TRANSACTION"), ""}
	if m.pendingSelected < len(m.pending) {
		tx := m.pending[m.pendingSelected]
		status := "pending"
		if _, ok := m.awaitingApproval[tx.TransactionHash]; ok {
			status = "awaiting browser approval"
		}
		detail = append(detail,
			detailRow("Tool", toolLabel(tx.ToolName)),
			detailRow("Status", status),
			detailRow("Operator", tx.OperatorID),
			detailRow("User", tx.UserID),
			detailRow("Created", tx.CreatedAt.Format(detailTimeFormat)),
			detailRow("Expires", tx.ExpiresAt.Format(detailTimeFormat)+" ("+formatRemaining(tx.ExpiresAt.Sub(timeNow()))+")"),
			"",
			detailStyle.Render("Transaction hash:"),
		)
		detail = append(detail, wrapText(tx.TransactionHash, innerWidth(rightWidth))...)
		detail = append(detail, "",
			detailStyle.Render("a / enter: approve in the browser (WebAuthn), as 'g8e auth approve'"))
	} else {
		detail = append(detail, detailStyle.Render("(nothing selected)"))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top,
		box(borderFocused, leftWidth, height, list),
		box(borderPane, rightWidth, height, detail),
	)
}

// wrapText splits s into lines of at most width cells, for values such as
// hashes that have no spaces to wrap at.
func wrapText(s string, width int) []string {
	width = max(width, 1)
	var lines []string
	runes := []rune(s)
	for len(runes) > width {
		lines = append(lines, ledgerInfoStyle.Render(string(runes[:width])))
		runes = runes[width:]
	}
	return append(lines, ledgerInfoStyle.Render(string(runes)))
}
