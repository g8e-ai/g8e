// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
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
	return operatorShowCmdWithConfig(shared.LoadConfig, authcmd.DefaultAPIClientFactory, shared.NewFileSvc)
}

func operatorShowCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
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
	if view != nil && view.SystemIdentity.Hostname != "" {
		return view.SystemIdentity.Hostname
	}
	return op.CurrentHostname
}

func operatorHostnameDisplay(op models.OperatorDocumentGo) string {
	if hostname := operatorHostnameValue(op); hostname != "" {
		return hostname
	}
	return "-"
}

type operatorShowOutput struct {
	OperatorID        string                   `json:"operator_id"`
	OperatorSessionID string                   `json:"operator_session_id"`
	OperatorType      constants.OperatorType   `json:"operator_type"`
	Status            constants.OperatorStatus `json:"status"`
	Component         constants.ComponentName  `json:"component"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
	Name              string                   `json:"name,omitempty"`
	SystemFingerprint string                   `json:"system_fingerprint,omitempty"`
	RuntimeConfig     *models.RuntimeConfig    `json:"runtime_config,omitempty"`
	Heartbeat         *operatorHeartbeatOutput `json:"heartbeat,omitempty"`
}

type operatorHeartbeatOutput struct {
	Timestamp          string                              `json:"timestamp,omitempty"`
	HeartbeatType      string                              `json:"heartbeat_type,omitempty"`
	SystemIdentity     *models.HeartbeatSystemIdentity     `json:"system_identity,omitempty"`
	PerformanceMetrics *models.HeartbeatPerformanceMetrics `json:"performance_metrics,omitempty"`
	NetworkInfo        *models.HeartbeatNetworkInfo        `json:"network_info,omitempty"`
	UptimeInfo         *models.HeartbeatUptimeInfo         `json:"uptime_info,omitempty"`
	OSDetails          *models.HeartbeatOSDetails          `json:"os_details,omitempty"`
	UserDetails        *models.HeartbeatUserDetails        `json:"user_details,omitempty"`
	DiskDetails        *models.HeartbeatDiskDetails        `json:"disk_details,omitempty"`
	MemoryDetails      *models.HeartbeatMemoryDetails      `json:"memory_details,omitempty"`
	Environment        *models.HeartbeatEnvironment        `json:"environment,omitempty"`
	VersionInfo        *models.HeartbeatVersionInfo        `json:"version_info,omitempty"`
	CapabilityFlags    *models.HeartbeatCapabilityFlags    `json:"capability_flags,omitempty"`
	SystemFingerprint  string                              `json:"system_fingerprint,omitempty"`
}

func operatorShowPayload(op models.OperatorDocumentGo) operatorShowOutput {
	payload := operatorShowOutput{
		OperatorID:        op.ID,
		OperatorSessionID: op.OperatorSessionID,
		OperatorType:      op.OperatorType,
		Status:            op.Status,
		Component:         op.Component,
		CreatedAt:         op.CreatedAt,
		UpdatedAt:         op.UpdatedAt,
		Name:              op.Name,
		SystemFingerprint: op.SystemFingerprint,
		RuntimeConfig:     op.RuntimeConfig,
	}
	if view := parseOperatorHeartbeatView(op.LatestHeartbeat); view != nil {
		payload.Heartbeat = heartbeatViewOutput(view)
	}
	return payload
}

func heartbeatViewOutput(view *operatorHeartbeatView) *operatorHeartbeatOutput {
	result := &operatorHeartbeatOutput{Timestamp: view.Timestamp, HeartbeatType: view.HeartbeatType, SystemFingerprint: view.SystemFingerprint}
	if view.SystemIdentity.Hostname != "" || view.SystemIdentity.OS != "" {
		result.SystemIdentity = &view.SystemIdentity
	}
	if view.PerformanceMetrics.CPUPercent != 0 || view.PerformanceMetrics.MemoryPercent != 0 || view.PerformanceMetrics.DiskPercent != 0 {
		result.PerformanceMetrics = &view.PerformanceMetrics
	}
	if len(view.NetworkInfo.Interfaces) > 0 || len(view.NetworkInfo.ConnectivityStatus) > 0 || view.NetworkInfo.HTTPPort != 0 || view.NetworkInfo.HTTPSPort != 0 {
		result.NetworkInfo = &view.NetworkInfo
	}
	if view.UptimeInfo.Uptime != "" || view.UptimeInfo.UptimeSeconds != 0 {
		result.UptimeInfo = &view.UptimeInfo
	}
	if view.OSDetails.Kernel != "" || view.OSDetails.Distro != "" || view.OSDetails.Version != "" {
		result.OSDetails = &view.OSDetails
	}
	if view.UserDetails.Username != "" {
		result.UserDetails = &view.UserDetails
	}
	if view.DiskDetails.TotalGB != 0 || view.DiskDetails.UsedGB != 0 {
		result.DiskDetails = &view.DiskDetails
	}
	if view.MemoryDetails.TotalMB != 0 || view.MemoryDetails.UsedMB != 0 {
		result.MemoryDetails = &view.MemoryDetails
	}
	if view.Environment.PWD != "" || view.Environment.Timezone != "" || view.Environment.IsContainer {
		result.Environment = &view.Environment
	}
	if view.VersionInfo.OperatorVersion != "" {
		result.VersionInfo = &view.VersionInfo
	}
	if view.CapabilityFlags.ExecutionVaultEnabled || view.CapabilityFlags.GitAvailable || view.CapabilityFlags.LedgerMirrorEnabled {
		result.CapabilityFlags = &view.CapabilityFlags
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
