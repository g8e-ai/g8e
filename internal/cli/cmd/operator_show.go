// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/spf13/cobra"
)

type operatorHeartbeatView struct {
	Timestamp          string
	HeartbeatType      string
	SystemIdentity     models.HeartbeatSystemIdentity
	PerformanceMetrics models.HeartbeatPerformanceMetrics
	NetworkInfo        models.HeartbeatNetworkInfo
	UptimeInfo         models.HeartbeatUptimeInfo
	OSDetails          models.HeartbeatOSDetails
	UserDetails        models.HeartbeatUserDetails
	DiskDetails        models.HeartbeatDiskDetails
	MemoryDetails      models.HeartbeatMemoryDetails
	Environment        models.HeartbeatEnvironment
	VersionInfo        models.HeartbeatVersionInfo
	CapabilityFlags    models.HeartbeatCapabilityFlags
	SystemFingerprint  string
}

func operatorShowCmd() *cobra.Command {
	return operatorShowCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func operatorShowCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory apiClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <operator-id-or-session-id>",
		Short: "Show operator details and host heartbeat metrics",
		Long: `Show details for a single Operator instance, including the latest
heartbeat snapshot gathered from the operator host.

The argument may be an operator ID (from the ID column) or an operator
session ID (from the Session ID column in './g8e operator list').`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			creds, err := auth.LoadCredentials(fileSvc, cfg)
			if err != nil || creds == nil {
				return fmt.Errorf("%w: Please run './g8e auth enroll user' first", constants.ErrNotAuthenticated)
			}

			client, err := clientFactory(fileSvc, cfg)
			if err != nil {
				return err
			}

			operators, err := listUserOperators(client, creds.UserID)
			if err != nil {
				return err
			}

			target := findOperatorByIDOrSession(operators, args[0])
			if target == nil {
				return fmt.Errorf("%w: no operator matches %q for the authenticated user", constants.ErrRegistrationOperatorNotFound, args[0])
			}

			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), operatorShowPayload(*target))
			}

			printOperatorShow(cmd, *target)
			return nil
		},
	}
	return cmd
}

func findOperatorByIDOrSession(operators []models.OperatorDocumentGo, idOrSession string) *models.OperatorDocumentGo {
	for i := range operators {
		if operators[i].ID == idOrSession || operators[i].OperatorSessionID == idOrSession {
			return &operators[i]
		}
	}
	return nil
}

func operatorHostnameValue(op models.OperatorDocumentGo) string {
	view := parseOperatorHeartbeatView(op.LatestHeartbeat)
	if view == nil {
		return ""
	}
	return view.SystemIdentity.Hostname
}

func operatorHostnameDisplay(op models.OperatorDocumentGo) string {
	if hostname := operatorHostnameValue(op); hostname != "" {
		return hostname
	}
	return "-"
}

func parseOperatorHeartbeatView(raw json.RawMessage) *operatorHeartbeatView {
	if len(raw) == 0 {
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}

	view := &operatorHeartbeatView{}
	unmarshalHeartbeatField(fields, "timestamp", &view.Timestamp)
	unmarshalHeartbeatField(fields, "heartbeat_type", &view.HeartbeatType)
	unmarshalHeartbeatField(fields, "system_identity", &view.SystemIdentity)
	unmarshalHeartbeatField(fields, "os_details", &view.OSDetails)
	unmarshalHeartbeatField(fields, "user_details", &view.UserDetails)
	unmarshalHeartbeatField(fields, "disk_details", &view.DiskDetails)
	unmarshalHeartbeatField(fields, "memory_details", &view.MemoryDetails)
	unmarshalHeartbeatField(fields, "environment", &view.Environment)
	unmarshalHeartbeatField(fields, "version_info", &view.VersionInfo)
	unmarshalHeartbeatField(fields, "capability_flags", &view.CapabilityFlags)
	unmarshalHeartbeatField(fields, "system_fingerprint", &view.SystemFingerprint)

	if !unmarshalHeartbeatField(fields, "performance_metrics", &view.PerformanceMetrics) {
		unmarshalHeartbeatField(fields, "performance", &view.PerformanceMetrics)
	}
	if !unmarshalHeartbeatField(fields, "network_info", &view.NetworkInfo) {
		unmarshalHeartbeatField(fields, "network", &view.NetworkInfo)
	}
	if !unmarshalHeartbeatField(fields, "uptime_info", &view.UptimeInfo) {
		unmarshalHeartbeatField(fields, "uptime", &view.UptimeInfo)
	}

	if view.SystemIdentity.Hostname == "" &&
		view.PerformanceMetrics.CPUPercent == 0 &&
		view.Timestamp == "" {
		return nil
	}
	return view
}

func unmarshalHeartbeatField(fields map[string]json.RawMessage, key string, dest any) bool {
	raw, ok := fields[key]
	if !ok || len(raw) == 0 {
		return false
	}
	return json.Unmarshal(raw, dest) == nil
}

func operatorShowPayload(op models.OperatorDocumentGo) map[string]any {
	payload := map[string]any{
		"operator_id":         op.ID,
		"operator_session_id": op.OperatorSessionID,
		"operator_type":       op.OperatorType,
		"status":              op.Status,
		"component":           op.Component,
		"created_at":          op.CreatedAt,
		"updated_at":          op.UpdatedAt,
	}
	if op.Name != "" {
		payload["name"] = op.Name
	}
	if op.SystemFingerprint != "" {
		payload["system_fingerprint"] = op.SystemFingerprint
	}
	if op.RuntimeConfig != nil {
		payload["runtime_config"] = op.RuntimeConfig
	}
	if view := parseOperatorHeartbeatView(op.LatestHeartbeat); view != nil {
		payload["heartbeat"] = heartbeatViewMap(view)
	}
	return payload
}

func heartbeatViewMap(view *operatorHeartbeatView) map[string]any {
	result := map[string]any{}
	if view.Timestamp != "" {
		result["timestamp"] = view.Timestamp
	}
	if view.HeartbeatType != "" {
		result["heartbeat_type"] = view.HeartbeatType
	}
	if view.SystemIdentity.Hostname != "" || view.SystemIdentity.OS != "" {
		result["system_identity"] = view.SystemIdentity
	}
	if view.PerformanceMetrics.CPUPercent != 0 || view.PerformanceMetrics.MemoryPercent != 0 || view.PerformanceMetrics.DiskPercent != 0 {
		result["performance_metrics"] = view.PerformanceMetrics
	}
	if len(view.NetworkInfo.Interfaces) > 0 || len(view.NetworkInfo.ConnectivityStatus) > 0 || view.NetworkInfo.HTTPPort != 0 || view.NetworkInfo.HTTPSPort != 0 {
		result["network_info"] = view.NetworkInfo
	}
	if view.UptimeInfo.Uptime != "" || view.UptimeInfo.UptimeSeconds != 0 {
		result["uptime_info"] = view.UptimeInfo
	}
	if view.OSDetails.Kernel != "" || view.OSDetails.Distro != "" || view.OSDetails.Version != "" {
		result["os_details"] = view.OSDetails
	}
	if view.UserDetails.Username != "" {
		result["user_details"] = view.UserDetails
	}
	if view.DiskDetails.TotalGB != 0 || view.DiskDetails.UsedGB != 0 {
		result["disk_details"] = view.DiskDetails
	}
	if view.MemoryDetails.TotalMB != 0 || view.MemoryDetails.UsedMB != 0 {
		result["memory_details"] = view.MemoryDetails
	}
	if view.Environment.PWD != "" || view.Environment.Timezone != "" || view.Environment.IsContainer {
		result["environment"] = view.Environment
	}
	if view.VersionInfo.OperatorVersion != "" {
		result["version_info"] = view.VersionInfo
	}
	if view.CapabilityFlags.ExecutionVaultEnabled || view.CapabilityFlags.GitAvailable || view.CapabilityFlags.LedgerMirrorEnabled {
		result["capability_flags"] = view.CapabilityFlags
	}
	if view.SystemFingerprint != "" {
		result["system_fingerprint"] = view.SystemFingerprint
	}
	return result
}

func printOperatorShow(cmd *cobra.Command, op models.OperatorDocumentGo) {
	view := parseOperatorHeartbeatView(op.LatestHeartbeat)

	cmd.Printf("Operator:  %s\n", op.ID)
	cmd.Printf("Session:   %s\n", op.OperatorSessionID)
	cmd.Printf("Type:      %s\n", op.OperatorType)
	cmd.Printf("Status:    %s\n", op.Status)
	if op.Name != "" {
		cmd.Printf("Name:      %s\n", op.Name)
	}
	if op.SystemFingerprint != "" {
		cmd.Printf("Fingerprint: %s\n", op.SystemFingerprint)
	}
	cmd.Printf("Updated:   %s\n", op.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"))

	if op.RuntimeConfig != nil {
		cmd.Println()
		cmd.Println("Runtime")
		cmd.Println(strings.Repeat("-", 40))
		cmd.Printf("  Inference enabled: %t\n", op.RuntimeConfig.InferenceEnabled)
		if op.RuntimeConfig.InferenceOllamaEndpoint != "" {
			cmd.Printf("  Ollama endpoint:   %s\n", op.RuntimeConfig.InferenceOllamaEndpoint)
		}
		cmd.Printf("  Provider boundary observer: %t\n", op.RuntimeConfig.ProviderBoundaryObserverEnabled)
		cmd.Printf("  Provenance operator:        %t\n", op.RuntimeConfig.ProvenanceOperatorEnabled)
	}

	if view == nil {
		cmd.Println()
		cmd.Println("No heartbeat snapshot available for this operator.")
		return
	}

	cmd.Println()
	cmd.Println("Host")
	cmd.Println(strings.Repeat("-", 40))
	printHeartbeatField(cmd, "Hostname", view.SystemIdentity.Hostname)
	printHeartbeatField(cmd, "OS", string(view.SystemIdentity.OS))
	printHeartbeatField(cmd, "Architecture", view.SystemIdentity.Architecture)
	printHeartbeatField(cmd, "User", view.SystemIdentity.CurrentUser)
	printHeartbeatField(cmd, "Working dir", view.SystemIdentity.PWD)
	if view.SystemIdentity.CPUCount > 0 {
		cmd.Printf("  CPUs:        %d\n", view.SystemIdentity.CPUCount)
	}
	if view.SystemIdentity.MemoryMB > 0 {
		cmd.Printf("  Memory:      %d MB\n", view.SystemIdentity.MemoryMB)
	}

	if view.OSDetails.Distro != "" || view.OSDetails.Kernel != "" || view.OSDetails.Version != "" {
		cmd.Println()
		cmd.Println("OS Details")
		cmd.Println(strings.Repeat("-", 40))
		printHeartbeatField(cmd, "Distro", view.OSDetails.Distro)
		printHeartbeatField(cmd, "Version", view.OSDetails.Version)
		printHeartbeatField(cmd, "Kernel", view.OSDetails.Kernel)
	}

	if view.PerformanceMetrics.CPUPercent != 0 || view.PerformanceMetrics.MemoryPercent != 0 || view.PerformanceMetrics.DiskPercent != 0 {
		cmd.Println()
		cmd.Println("Performance")
		cmd.Println(strings.Repeat("-", 40))
		if view.PerformanceMetrics.CPUPercent != 0 {
			cmd.Printf("  CPU:             %.1f%%\n", view.PerformanceMetrics.CPUPercent)
		}
		if view.PerformanceMetrics.MemoryPercent != 0 {
			cmd.Printf("  Memory:          %.1f%%", view.PerformanceMetrics.MemoryPercent)
			if view.PerformanceMetrics.MemoryUsedMB > 0 && view.PerformanceMetrics.MemoryTotalMB > 0 {
				cmd.Printf(" (%d / %d MB)", view.PerformanceMetrics.MemoryUsedMB, view.PerformanceMetrics.MemoryTotalMB)
			}
			cmd.Println()
		}
		if view.PerformanceMetrics.DiskPercent != 0 {
			cmd.Printf("  Disk:            %.1f%%", view.PerformanceMetrics.DiskPercent)
			if view.PerformanceMetrics.DiskUsedGB > 0 && view.PerformanceMetrics.DiskTotalGB > 0 {
				cmd.Printf(" (%.1f / %.1f GB)", view.PerformanceMetrics.DiskUsedGB, view.PerformanceMetrics.DiskTotalGB)
			}
			cmd.Println()
		}
		if view.PerformanceMetrics.NetworkLatency != 0 {
			cmd.Printf("  Network latency: %.1f ms\n", view.PerformanceMetrics.NetworkLatency)
		}
	}

	if view.UptimeInfo.Uptime != "" || view.UptimeInfo.UptimeSeconds != 0 {
		cmd.Println()
		cmd.Println("Uptime")
		cmd.Println(strings.Repeat("-", 40))
		if view.UptimeInfo.Uptime != "" {
			cmd.Printf("  %s", view.UptimeInfo.Uptime)
			if view.UptimeInfo.UptimeSeconds > 0 {
				cmd.Printf(" (%ds)", view.UptimeInfo.UptimeSeconds)
			}
			cmd.Println()
		} else {
			cmd.Printf("  %ds\n", view.UptimeInfo.UptimeSeconds)
		}
	}

	if view.VersionInfo.OperatorVersion != "" {
		cmd.Println()
		cmd.Println("Version")
		cmd.Println(strings.Repeat("-", 40))
		cmd.Printf("  Operator: %s\n", view.VersionInfo.OperatorVersion)
		if view.VersionInfo.Status != "" {
			cmd.Printf("  Status:   %s\n", view.VersionInfo.Status)
		}
	}

	if view.Timestamp != "" || view.HeartbeatType != "" {
		cmd.Println()
		cmd.Println("Heartbeat")
		cmd.Println(strings.Repeat("-", 40))
		if view.Timestamp != "" {
			cmd.Printf("  Timestamp: %s\n", view.Timestamp)
		}
		if view.HeartbeatType != "" {
			cmd.Printf("  Type:      %s\n", view.HeartbeatType)
		}
	}

	if len(view.NetworkInfo.ConnectivityStatus) > 0 || len(view.NetworkInfo.Interfaces) > 0 {
		cmd.Println()
		cmd.Println("Network")
		cmd.Println(strings.Repeat("-", 40))
		if view.NetworkInfo.HTTPPort > 0 || view.NetworkInfo.HTTPSPort > 0 {
			cmd.Printf("  Ports: HTTP %d, HTTPS %d\n", view.NetworkInfo.HTTPPort, view.NetworkInfo.HTTPSPort)
		}
		for _, iface := range view.NetworkInfo.ConnectivityStatus {
			if iface.Name == "" && iface.IP == "" {
				continue
			}
			cmd.Printf("  %s: %s\n", iface.Name, iface.IP)
		}
		if len(view.NetworkInfo.ConnectivityStatus) == 0 {
			for _, iface := range view.NetworkInfo.Interfaces {
				cmd.Printf("  %s\n", iface)
			}
		}
	}

	if view.Environment.PWD != "" || view.Environment.Timezone != "" || view.Environment.IsContainer {
		cmd.Println()
		cmd.Println("Environment")
		cmd.Println(strings.Repeat("-", 40))
		printHeartbeatField(cmd, "PWD", view.Environment.PWD)
		printHeartbeatField(cmd, "Timezone", view.Environment.Timezone)
		if view.Environment.IsContainer {
			cmd.Printf("  Container: %s\n", view.Environment.ContainerRuntime)
		}
	}
}

func printHeartbeatField(cmd *cobra.Command, label, value string) {
	if value == "" {
		return
	}
	cmd.Printf("  %-12s %s\n", label+":", value)
}
