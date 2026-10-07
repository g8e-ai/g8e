// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

func (m Model) renderRecoveryView(width, height int) string {
	lines := []string{
		paneHeaderStyle.Render("CLI RECOVERY APPROVAL"),
		"",
		detailStyle.Render("Approve or deny a pending headless CLI enrollment recovery request."),
		"",
		detailRow("DECISION", recoveryDecisionLabel(m.recoveryApprove)),
		"",
		paneHeaderStyle.Render("ONE-TIME TOKEN"),
		m.recoveryTokenInput.View(),
		"",
		detailStyle.Render("tab toggles approve/deny; enter submits after confirmation"),
	}
	if m.recoveryState != "" {
		lines = append(lines, "", detailRow("LAST STATE", string(m.recoveryState)))
	}
	if m.recoveryErr != "" {
		lines = append(lines, "", ledgerWarnStyle.Render("unavailable: "+m.recoveryErr))
	}
	return box(borderFocused, width, height, lines)
}

func recoveryDecisionLabel(approve bool) string {
	if approve {
		return "APPROVE"
	}
	return "DENY"
}
