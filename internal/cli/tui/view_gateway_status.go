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

	cligateway "github.com/g8e-ai/g8e/v2/internal/cli/gateway"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func (m Model) renderGatewayStatusView(width, height int) string {
	leftWidth, rightWidth := splitWidth(width)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		box(borderPane, leftWidth, height, m.gatewayStatusLines()),
		box(borderPane, rightWidth, height, m.gatewayWorkloadLines()),
	)
}

func (m Model) gatewayStatusLines() []string {
	lines := []string{paneHeaderStyle.Render("GATEWAY STATUS"), ""}
	if m.health.Status == "" && m.gatewayVersion == "" {
		lines = append(lines, detailStyle.Render("(loading...)"))
	} else {
		status := string(m.health.Status)
		if status == "" {
			status = "unknown"
		}
		mode := string(m.health.Mode)
		if mode == "" {
			mode = "unknown"
		}
		ready := "no"
		if m.health.GovernanceReady {
			ready = "yes"
		}
		lines = append(lines,
			detailRow("STATE", status),
			detailRow("MODE", mode),
			detailRow("VERSION", m.health.Version),
			detailRow("PID", gatewayPID(m.health.PID)),
			detailRow("GOVERNANCE", ready),
			detailRow("POSTURE", m.health.Posture),
		)
		if m.health.StateMerkleRoot != "" {
			lines = append(lines, detailRow("STATE ROOT", shortHash(m.health.StateMerkleRoot)))
		}
	}
	lines = append(lines, "", paneHeaderStyle.Render("CLI IDENTITY"),
		detailRow("USER", m.identity.UserID),
		detailRow("CLI SESSION", m.identity.CLISessionID),
		detailRow("OPERATOR", m.identity.OperatorID),
		"",
		paneHeaderStyle.Render("CONNECTED OPERATORS"),
		detailRow("COUNT", fmt.Sprintf("%d / %d", len(m.operators), m.operatorsTotal)),
	)
	if m.enrollmentsErr != "" {
		lines = append(lines, ledgerWarnStyle.Render("enrollments unavailable: "+m.enrollmentsErr))
	}
	return lines
}

func (m Model) gatewayWorkloadLines() []string {
	workloads := m.gatewayStatusWorkloads()
	lines := []string{paneHeaderStyle.Render(fmt.Sprintf("ENROLLED WORKLOADS (%d)", len(workloads))), ""}
	if m.enrollmentsErr != "" {
		return append(lines, ledgerWarnStyle.Render("unavailable: "+m.enrollmentsErr))
	}
	if !m.enrollmentsLoaded {
		return append(lines, detailStyle.Render("(loading...)"))
	}
	if len(workloads) == 0 {
		return append(lines, detailStyle.Render("(no completed enrollments)"))
	}
	for i, workload := range workloads {
		row := fmt.Sprintf("%s %s %s", workload.ComponentKind, workload.Hostname, shortHash(cligatewayIdentity(workload)))
		lines = append(lines, selectableRow(i == m.gatewayStatusSelected, row))
	}
	if workload, ok := m.selectedGatewayStatusWorkload(); ok {
		lines = append(lines, "", paneHeaderStyle.Render("SELECTED WORKLOAD"),
			detailRow("KIND", string(workload.ComponentKind)),
			detailRow("NAME", workload.ComponentName),
			detailRow("HOSTNAME", workload.Hostname),
			detailRow("INSTANCE", workload.InstanceID),
			detailRow("IDENTITY", cligatewayIdentity(workload)),
			detailRow("COMPLETED", formatOptionalTime(workload.CompletedAt)),
		)
	}
	return lines
}

func (m Model) gatewayStatusWorkloads() []models.PlatformEnrollmentEnrolledRequest {
	return cligateway.CompletedEnrollments(m.enrollEnrolled)
}

func (m Model) selectedGatewayStatusWorkload() (models.PlatformEnrollmentEnrolledRequest, bool) {
	workloads := m.gatewayStatusWorkloads()
	if m.gatewayStatusSelected < 0 || m.gatewayStatusSelected >= len(workloads) {
		return models.PlatformEnrollmentEnrolledRequest{}, false
	}
	return workloads[m.gatewayStatusSelected], true
}

func gatewayPID(pid int) string {
	if pid <= 0 {
		return "-"
	}
	return fmt.Sprint(pid)
}

func cligatewayIdentity(enrollment models.PlatformEnrollmentEnrolledRequest) string {
	if enrollment.OperatorID != "" {
		return enrollment.OperatorID
	}
	if enrollment.OperatorSessionID != "" {
		return enrollment.OperatorSessionID
	}
	if enrollment.CLISessionID != "" {
		return enrollment.CLISessionID
	}
	if enrollment.PolicyID != "" {
		return enrollment.PolicyID
	}
	return enrollment.RequestID
}
