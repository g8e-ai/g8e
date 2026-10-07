// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tui

import (
	"fmt"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/cli/operator"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
)

// renderOperatorDetailsView renders the selected Operator's complete
// metadata and latest heartbeat snapshot. The list remains the source of the
// selected document; this view only changes how much of it is shown.
func (m Model) renderOperatorDetailsView(width, height int) string {
	if m.operatorsSelected >= len(m.operators) {
		return box(borderFocused, width, height, []string{
			paneHeaderStyle.Render("OPERATOR DETAILS"),
			"",
			detailStyle.Render("(nothing selected — esc returns to Operators)"),
		})
	}

	lines := operatorDetailsLines(m.operators[m.operatorsSelected], m.identity)
	visibleRows := max(height-borderFocused.GetVerticalFrameSize(), 1)
	start := clamp(m.operatorDetailScroll, max(len(lines)-visibleRows, 0))
	end := min(len(lines), start+visibleRows)
	return box(borderFocused, width, height, lines[start:end])
}

// operatorDetailsLines mirrors the data shown by 'g8e operator show': the
// identity and runtime fields followed by the normalized heartbeat sections.
func operatorDetailsLines(op operatorv1.OperatorDocument, identity Identity) []string {
	bound := op.BoundWebSessionID
	if op.ID == identity.OperatorID {
		bound = "yes (this CLI session)"
	}
	lines := []string{paneHeaderStyle.Render("OPERATOR DETAILS"), "",
		detailRow("ID", op.ID),
		detailRow("Hostname", operatorHostname(op)),
		detailRow("Roles", operatorcapability.GetOperatorRoles(op).String()),
		detailRow("Status", string(op.Status)),
		detailRow("Type", string(op.OperatorType)),
		detailRow("Session", op.OperatorSessionID),
		detailRow("Bound", bound),
		detailRow("Started", formatOptionalTime(op.StartedAt)),
		detailRow("Heartbeat", formatHeartbeat(op.LastHeartbeatAt)),
		detailRow("Fingerprint", op.SystemFingerprint),
	}
	if op.LocalDir != "" {
		lines = append(lines, detailRow("Directory", op.LocalDir))
	}
	if op.RuntimeConfig != nil {
		lines = append(lines, "", paneHeaderStyle.Render("RUNTIME"),
			detailRow("Inference", fmt.Sprintf("%t", op.RuntimeConfig.InferenceEnabled)),
			detailRow("Ollama", op.RuntimeConfig.InferenceOllamaEndpoint),
			detailRow("Observer", fmt.Sprintf("%t", op.RuntimeConfig.ProviderBoundaryObserverEnabled)),
			detailRow("Provenance", fmt.Sprintf("%t", op.RuntimeConfig.ProvenanceOperatorEnabled)),
		)
	}

	view := operator.ParseHeartbeatView(op.LatestHeartbeat)
	if view == nil {
		return append(lines, "", paneHeaderStyle.Render("HEARTBEAT"), "", detailStyle.Render("(no heartbeat snapshot available)"))
	}

	lines = append(lines, "", paneHeaderStyle.Render("HOST"),
		detailRow("Hostname", view.SystemIdentity.Hostname),
		detailRow("OS", string(view.SystemIdentity.OS)),
		detailRow("Architecture", view.SystemIdentity.Architecture),
		detailRow("User", view.SystemIdentity.CurrentUser),
		detailRow("Working dir", view.SystemIdentity.PWD),
	)
	if view.SystemIdentity.CPUCount > 0 {
		lines = append(lines, detailRow("CPUs", fmt.Sprintf("%d", view.SystemIdentity.CPUCount)))
	}
	if view.SystemIdentity.MemoryMB > 0 {
		lines = append(lines, detailRow("Memory", fmt.Sprintf("%d MB", view.SystemIdentity.MemoryMB)))
	}
	if view.OSDetails.Distro != "" || view.OSDetails.Kernel != "" || view.OSDetails.Version != "" {
		lines = append(lines, "", paneHeaderStyle.Render("OS DETAILS"),
			detailRow("Distro", view.OSDetails.Distro),
			detailRow("Version", view.OSDetails.Version),
			detailRow("Kernel", view.OSDetails.Kernel),
		)
	}
	if view.PerformanceMetrics.CPUPercent != 0 || view.PerformanceMetrics.MemoryPercent != 0 || view.PerformanceMetrics.DiskPercent != 0 || view.PerformanceMetrics.NetworkLatency != 0 {
		lines = append(lines, "", paneHeaderStyle.Render("PERFORMANCE"))
		if view.PerformanceMetrics.CPUPercent != 0 {
			lines = append(lines, detailRow("CPU", fmt.Sprintf("%.1f%%", view.PerformanceMetrics.CPUPercent)))
		}
		if view.PerformanceMetrics.MemoryPercent != 0 {
			memory := fmt.Sprintf("%.1f%%", view.PerformanceMetrics.MemoryPercent)
			if view.PerformanceMetrics.MemoryUsedMB > 0 && view.PerformanceMetrics.MemoryTotalMB > 0 {
				memory += fmt.Sprintf(" (%d / %d MB)", view.PerformanceMetrics.MemoryUsedMB, view.PerformanceMetrics.MemoryTotalMB)
			}
			lines = append(lines, detailRow("Memory", memory))
		}
		if view.PerformanceMetrics.DiskPercent != 0 {
			lines = append(lines, detailRow("Disk", fmt.Sprintf("%.1f%%", view.PerformanceMetrics.DiskPercent)))
		}
		if view.PerformanceMetrics.NetworkLatency != 0 {
			lines = append(lines, detailRow("Network", fmt.Sprintf("%.1f ms", view.PerformanceMetrics.NetworkLatency)))
		}
	}
	if view.UptimeInfo.Uptime != "" || view.UptimeInfo.UptimeSeconds != 0 {
		uptime := view.UptimeInfo.Uptime
		if uptime == "" {
			uptime = fmt.Sprintf("%ds", view.UptimeInfo.UptimeSeconds)
		} else if view.UptimeInfo.UptimeSeconds > 0 {
			uptime += fmt.Sprintf(" (%ds)", view.UptimeInfo.UptimeSeconds)
		}
		lines = append(lines, "", paneHeaderStyle.Render("UPTIME"), detailRow("Duration", uptime))
	}
	if view.VersionInfo.OperatorVersion != "" || view.VersionInfo.Status != "" {
		lines = append(lines, "", paneHeaderStyle.Render("VERSION"),
			detailRow("Operator", view.VersionInfo.OperatorVersion),
			detailRow("Status", string(view.VersionInfo.Status)),
		)
	}
	if view.Timestamp != "" || view.HeartbeatType != "" {
		lines = append(lines, "", paneHeaderStyle.Render("HEARTBEAT"),
			detailRow("Timestamp", view.Timestamp),
			detailRow("Type", view.HeartbeatType),
		)
	}
	if len(view.NetworkInfo.ConnectivityStatus) > 0 || len(view.NetworkInfo.Interfaces) > 0 || view.NetworkInfo.HTTPPort > 0 || view.NetworkInfo.HTTPSPort > 0 {
		lines = append(lines, "", paneHeaderStyle.Render("NETWORK"))
		if view.NetworkInfo.HTTPPort > 0 || view.NetworkInfo.HTTPSPort > 0 {
			lines = append(lines, detailRow("Ports", fmt.Sprintf("HTTP %d, HTTPS %d", view.NetworkInfo.HTTPPort, view.NetworkInfo.HTTPSPort)))
		}
		for _, iface := range view.NetworkInfo.ConnectivityStatus {
			if iface.Name != "" || iface.IP != "" {
				lines = append(lines, detailRow(iface.Name, iface.IP))
			}
		}
		if len(view.NetworkInfo.ConnectivityStatus) == 0 {
			for _, iface := range view.NetworkInfo.Interfaces {
				lines = append(lines, detailRow("Interface", iface))
			}
		}
	}
	if view.Environment.PWD != "" || view.Environment.Timezone != "" || view.Environment.IsContainer {
		lines = append(lines, "", paneHeaderStyle.Render("ENVIRONMENT"),
			detailRow("PWD", view.Environment.PWD),
			detailRow("Timezone", view.Environment.Timezone),
			detailRow("Terminal", view.Environment.Term),
		)
		if view.Environment.IsContainer {
			lines = append(lines, detailRow("Container", view.Environment.ContainerRuntime))
		}
	}
	if view.CapabilityFlags.ExecutionVaultEnabled || view.CapabilityFlags.GitAvailable || view.CapabilityFlags.LedgerMirrorEnabled {
		lines = append(lines, "", paneHeaderStyle.Render("CAPABILITIES"),
			detailRow("Execution vault", fmt.Sprintf("%t", view.CapabilityFlags.ExecutionVaultEnabled)),
			detailRow("Git", fmt.Sprintf("%t", view.CapabilityFlags.GitAvailable)),
			detailRow("Ledger mirror", fmt.Sprintf("%t", view.CapabilityFlags.LedgerMirrorEnabled)),
		)
	}
	return append(lines, "", detailStyle.Render("j/k: scroll | esc: back"))
}
