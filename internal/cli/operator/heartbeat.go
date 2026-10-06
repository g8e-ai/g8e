// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operator

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// HeartbeatView is the normalized host snapshot shared by operator show and
// the TUI. Heartbeat snapshots are canonical HeartbeatResult protojson.
type HeartbeatView struct {
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

// ParseHeartbeatView decodes a canonical Operator heartbeat snapshot. Legacy
// Python-shaped payloads are rejected because protojson rejects unknown fields.
func ParseHeartbeatView(raw json.RawMessage) *HeartbeatView {
	if len(raw) == 0 {
		return nil
	}

	heartbeat := &operatorv1.HeartbeatResult{}
	if err := protojson.Unmarshal(raw, heartbeat); err != nil {
		return nil
	}
	return HeartbeatViewFromResult(heartbeat)
}

// HeartbeatViewFromResult normalizes a typed heartbeat result for human-facing
// CLI and TUI rendering.
func HeartbeatViewFromResult(heartbeat *operatorv1.HeartbeatResult) *HeartbeatView {
	if heartbeat == nil {
		return nil
	}

	view := &HeartbeatView{
		Timestamp:         heartbeat.Timestamp,
		HeartbeatType:     heartbeat.Status,
		SystemFingerprint: heartbeat.SystemFingerprint,
	}

	if heartbeat.SystemIdentity != nil {
		view.SystemIdentity = models.HeartbeatSystemIdentity{
			Hostname:     heartbeat.SystemIdentity.Hostname,
			OS:           constants.Platform(heartbeat.SystemIdentity.Os),
			Architecture: heartbeat.SystemIdentity.Architecture,
			PWD:          heartbeat.SystemIdentity.Pwd,
			CurrentUser:  heartbeat.SystemIdentity.CurrentUser,
			CPUCount:     int(heartbeat.SystemIdentity.CpuCount),
			MemoryMB:     int(heartbeat.SystemIdentity.MemoryMb),
		}
	}
	if heartbeat.PerformanceMetrics != nil {
		view.PerformanceMetrics = models.HeartbeatPerformanceMetrics{
			CPUPercent:     heartbeat.PerformanceMetrics.CpuPercent,
			MemoryPercent:  heartbeat.PerformanceMetrics.MemoryPercent,
			DiskPercent:    heartbeat.PerformanceMetrics.DiskPercent,
			NetworkLatency: heartbeat.PerformanceMetrics.NetworkLatency,
			MemoryUsedMB:   int(heartbeat.PerformanceMetrics.MemoryUsedMb),
			MemoryTotalMB:  int(heartbeat.PerformanceMetrics.MemoryTotalMb),
			DiskUsedGB:     heartbeat.PerformanceMetrics.DiskUsedGb,
			DiskTotalGB:    heartbeat.PerformanceMetrics.DiskTotalGb,
		}
	}
	if heartbeat.NetworkInfo != nil {
		view.NetworkInfo = models.HeartbeatNetworkInfo{Interfaces: heartbeat.NetworkInfo.Interfaces}
		for _, iface := range heartbeat.NetworkInfo.ConnectivityStatus {
			view.NetworkInfo.ConnectivityStatus = append(view.NetworkInfo.ConnectivityStatus, models.HeartbeatNetworkInterface{
				Name: iface.Name,
				IP:   iface.Ip,
				MTU:  int(iface.Mtu),
			})
		}
	}
	if heartbeat.UptimeInfo != nil {
		view.UptimeInfo = models.HeartbeatUptimeInfo{
			Uptime:        heartbeat.UptimeInfo.Uptime,
			UptimeSeconds: heartbeat.UptimeInfo.UptimeSeconds,
		}
	}
	if heartbeat.OsDetails != nil {
		view.OSDetails = models.HeartbeatOSDetails{
			Kernel:  heartbeat.OsDetails.Kernel,
			Distro:  heartbeat.OsDetails.Distro,
			Version: heartbeat.OsDetails.Version,
		}
	}
	if heartbeat.UserDetails != nil {
		view.UserDetails = models.HeartbeatUserDetails{
			Username: heartbeat.UserDetails.Username,
			UID:      heartbeat.UserDetails.Uid,
			GID:      heartbeat.UserDetails.Gid,
			Home:     heartbeat.UserDetails.Home,
			Name:     heartbeat.UserDetails.Name,
			Shell:    heartbeat.UserDetails.Shell,
		}
	}
	if heartbeat.DiskDetails != nil {
		view.DiskDetails = models.HeartbeatDiskDetails{
			TotalGB: heartbeat.DiskDetails.TotalGb,
			UsedGB:  heartbeat.DiskDetails.UsedGb,
			FreeGB:  heartbeat.DiskDetails.FreeGb,
			Percent: heartbeat.DiskDetails.Percent,
		}
	}
	if heartbeat.MemoryDetails != nil {
		view.MemoryDetails = models.HeartbeatMemoryDetails{
			TotalMB:     heartbeat.MemoryDetails.TotalMb,
			AvailableMB: heartbeat.MemoryDetails.AvailableMb,
			UsedMB:      heartbeat.MemoryDetails.UsedMb,
			Percent:     heartbeat.MemoryDetails.Percent,
		}
	}
	if heartbeat.Environment != nil {
		view.Environment = models.HeartbeatEnvironment{
			PWD:              heartbeat.Environment.Pwd,
			Lang:             heartbeat.Environment.Lang,
			Timezone:         heartbeat.Environment.Timezone,
			Term:             heartbeat.Environment.Term,
			IsContainer:      heartbeat.Environment.IsContainer,
			ContainerRuntime: heartbeat.Environment.ContainerRuntime,
			ContainerSignals: heartbeat.Environment.ContainerSignals,
			InitSystem:       heartbeat.Environment.InitSystem,
		}
	}
	if heartbeat.VersionInfo != nil {
		view.VersionInfo = models.HeartbeatVersionInfo{
			OperatorVersion: heartbeat.VersionInfo.OperatorVersion,
			Status:          constants.VersionStability(heartbeat.VersionInfo.Status),
		}
	}
	if heartbeat.CapabilityFlags != nil {
		view.CapabilityFlags = models.HeartbeatCapabilityFlags{
			ExecutionVaultEnabled: heartbeat.CapabilityFlags.LocalStorageEnabled,
			GitAvailable:          heartbeat.CapabilityFlags.GitAvailable,
			LedgerMirrorEnabled:   heartbeat.CapabilityFlags.LedgerMirrorEnabled,
		}
	}

	if view.SystemIdentity.Hostname == "" &&
		view.PerformanceMetrics.CPUPercent == 0 &&
		view.Timestamp == "" {
		return nil
	}
	return view
}
