// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
)

// renderOperatorsView renders the connected Operators beside the selected
// one's details.
func (m Model) renderOperatorsView(width, height int) string {
	leftWidth, rightWidth := splitWidth(width)

	header := "OPERATORS"
	if m.operatorsLoaded {
		header = fmt.Sprintf("OPERATORS (%d connected / %d)", len(m.operators), m.operatorsTotal)
	}
	list := []string{paneHeaderStyle.Render(header)}
	switch {
	case m.operatorsErr != "":
		list = append(list, ledgerWarnStyle.Render("unavailable: "+m.operatorsErr))
	case !m.operatorsLoaded:
		list = append(list, detailStyle.Render("(loading...)"))
	case len(m.operators) == 0:
		list = append(list, detailStyle.Render("(no connected operators)"))
	}
	rows := height - borderFocused.GetVerticalFrameSize() - 1
	start, end := windowFor(len(m.operators), m.operatorsSelected, rows)
	for i := start; i < end; i++ {
		op := m.operators[i]
		row := fmt.Sprintf("%s %s %s", operatorHostname(op), operatorcapability.GetOperatorRoles(op), shortHash(op.Id))
		if op.Id == m.identity.OperatorID {
			row += " [bound]"
		}
		list = append(list, selectableRow(i == m.operatorsSelected, row))
	}

	detail := []string{paneHeaderStyle.Render("OPERATOR"), ""}
	if m.operatorsSelected < len(m.operators) {
		op := m.operators[m.operatorsSelected]
		bound := "no"
		if op.Id == m.identity.OperatorID {
			bound = "yes (this CLI session)"
		}
		detail = append(detail,
			detailRow("ID", op.Id),
			detailRow("Hostname", operatorHostname(op)),
			detailRow("Roles", operatorcapability.GetOperatorRoles(op).String()),
			detailRow("Status", string(op.Status)),
			detailRow("Type", string(op.OperatorType)),
			detailRow("Session", op.OperatorSessionId),
			detailRow("Bound", bound),
			detailRow("Started", formatOptionalProtoTime(op.StartedAt)),
			detailRow("Heartbeat", formatProtoHeartbeat(op.LastHeartbeatAt)),
			detailRow("Fingerprint", shortHash(op.SystemFingerprint)),
		)
		if op.LocalDir != "" {
			detail = append(detail, detailRow("Directory", op.LocalDir))
		}
	} else {
		detail = append(detail, detailStyle.Render("(nothing selected)"))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top,
		box(borderFocused, leftWidth, height, list),
		box(borderPane, rightWidth, height, detail),
	)
}

// formatOptionalTime renders an optional timestamp, or "".
func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(detailTimeFormat)
}

// formatHeartbeat renders the last heartbeat time and its age.
func formatHeartbeat(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s (%s ago)", t.Format("15:04:05"), timeNow().Sub(*t).Truncate(time.Second))
}

// formatOptionalProtoTime renders an optional proto timestamp, or "".
func formatOptionalProtoTime(t *timestamppb.Timestamp) string {
	if t == nil {
		return ""
	}
	tm := t.AsTime()
	if tm.IsZero() {
		return ""
	}
	return tm.Format(detailTimeFormat)
}

// formatProtoHeartbeat renders the last heartbeat time and its age from a proto timestamp.
func formatProtoHeartbeat(t *timestamppb.Timestamp) string {
	if t == nil {
		return ""
	}
	tm := t.AsTime()
	if tm.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s (%s ago)", tm.Format("15:04:05"), timeNow().Sub(tm).Truncate(time.Second))
}
