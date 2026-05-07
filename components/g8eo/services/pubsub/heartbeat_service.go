// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pubsub

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/services/system"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"google.golang.org/protobuf/proto"
)

// HeartbeatService owns all heartbeat logic for the g8eo operator:
// building heartbeat payloads, handling inbound heartbeat requests,
// sending automatic heartbeats, and managing the periodic scheduler.
type HeartbeatService struct {
	config  *config.Config
	logger  *slog.Logger
	results ResultsPublisher

	ctx    context.Context
	mu     sync.Mutex
	wg     *sync.WaitGroup
	ticker *time.Ticker
	done   chan bool
}

// NewHeartbeatService creates a new HeartbeatService.
func NewHeartbeatService(cfg *config.Config, logger *slog.Logger, wg *sync.WaitGroup) *HeartbeatService {
	return &HeartbeatService{
		config: cfg,
		logger: logger,
		wg:     wg,
	}
}

// SetResultsPublisher sets the results publisher for the HeartbeatService.
func (hs *HeartbeatService) SetResultsPublisher(results ResultsPublisher) {
	hs.results = results
}

// SetContext sets the context for the HeartbeatService.
func (hs *HeartbeatService) SetContext(ctx context.Context) {
	hs.ctx = ctx
}

// Build constructs a complete Heartbeat payload of the given type.
func (hs *HeartbeatService) Build(heartbeatType models.HeartbeatType) *models.Heartbeat {
	pwd, _ := os.Getwd()

	heartbeat := &models.Heartbeat{
		EventType:         constants.Event.Operator.Heartbeat,
		SourceComponent:   constants.Status.ComponentName.G8EO,
		OperatorID:        hs.config.OperatorID,
		OperatorSessionID: hs.config.OperatorSessionId,
		CaseID:            "",
		InvestigationID:   "",
		Timestamp:         models.NowTimestamp(),
		HeartbeatType:     heartbeatType,
		SystemIdentity: models.HeartbeatSystemIdentity{
			Hostname:     system.GetHostname(),
			OS:           system.GetOSName(),
			Architecture: system.GetArchitecture(),
			PWD:          pwd,
			CurrentUser:  system.GetCurrentUser(),
			CPUCount:     system.GetNumCPU(),
			MemoryMB:     system.GetMemoryMB(),
		},
		NetworkInfo: models.HeartbeatNetworkInfo{
			PublicIP:           system.GetPublicIP(hs.config.IPService),
			InternalIP:         system.GetLocalIP(""),
			Interfaces:         system.GetNetworkInterfaces(),
			ConnectivityStatus: system.GetConnectivityStatus(),
		},
		VersionInfo: models.HeartbeatVersionInfo{
			OperatorVersion: hs.config.Version,
			Status:          constants.Status.VersionStability.Stable,
		},
		UptimeInfo: models.HeartbeatUptimeInfo{
			Uptime:        system.GetUptime(),
			UptimeSeconds: system.GetUptimeSeconds(),
		},
		PerformanceMetrics: models.HeartbeatPerformanceMetrics{
			CPUPercent:     system.GetCPUPercent(),
			MemoryPercent:  system.GetMemoryPercent(),
			DiskPercent:    system.GetDiskPercent(),
			NetworkLatency: system.GetNetworkLatency(),
			MemoryUsedMB:   int(system.GetMemoryDetails().UsedMB),
			MemoryTotalMB:  system.GetMemoryMB(),
			DiskUsedGB:     system.GetDiskUsedGB(),
			DiskTotalGB:    system.GetDiskTotalGB(),
		},
		OSDetails:     system.GetOSDetails(),
		UserDetails:   system.GetUserDetails(hs.config.Shell),
		DiskDetails:   system.GetDiskDetails(),
		MemoryDetails: system.GetMemoryDetails(),
		Environment:   system.GetEnvironmentDetails(hs.config.Lang, hs.config.Term, hs.config.TZ),
		CapabilityFlags: models.HeartbeatCapabilityFlags{
			LocalStorageEnabled: hs.config.LocalStoreEnabled,
			GitAvailable:        hs.config.GitAvailable,
			LedgerMirrorEnabled: hs.config.GitAvailable && !hs.config.NoGit,
		},
		FingerprintDetails: &models.HeartbeatFingerprintDetails{
			OS:           runtime.GOOS,
			Architecture: runtime.GOARCH,
			CPUCount:     runtime.NumCPU(),
			MachineID:    hs.config.SystemFingerprint,
		},
		SystemFingerprint: hs.config.SystemFingerprint,

		APIKey: hs.config.APIKey,
	}

	hs.logger.Info("[HEARTBEAT] Built heartbeat payload",
		"heartbeat_type", heartbeat.HeartbeatType,
		"operator_id", heartbeat.OperatorID,
		"operator_session_id", heartbeat.OperatorSessionID,
		"hostname", heartbeat.SystemIdentity.Hostname,
		"os", heartbeat.SystemIdentity.OS,
		"architecture", heartbeat.SystemIdentity.Architecture,
		"cpu_percent", heartbeat.PerformanceMetrics.CPUPercent,
		"memory_percent", heartbeat.PerformanceMetrics.MemoryPercent,
		"disk_percent", heartbeat.PerformanceMetrics.DiskPercent,
		"network_latency", heartbeat.PerformanceMetrics.NetworkLatency,
		"uptime_seconds", heartbeat.UptimeInfo.UptimeSeconds,
		"public_ip", heartbeat.NetworkInfo.PublicIP,
		"internal_ip", heartbeat.NetworkInfo.InternalIP)

	return heartbeat
}

// buildProtoHeartbeat converts a legacy models.Heartbeat to a v0.2.0 operatorv1.HeartbeatResult.
func (hs *HeartbeatService) buildProtoHeartbeat(h *models.Heartbeat) *operatorv1.HeartbeatResult {
	p := &operatorv1.HeartbeatResult{
		OperatorId:        h.OperatorID,
		OperatorSessionId: h.OperatorSessionID,
		Timestamp:         h.Timestamp,
		Status:            string(h.HeartbeatType),
		EventType:         h.EventType,
		SourceComponent:   h.SourceComponent,
		CaseId:            h.CaseID,
		InvestigationId:   h.InvestigationID,
		SystemIdentity: &operatorv1.SystemIdentity{
			Hostname:     h.SystemIdentity.Hostname,
			Os:           h.SystemIdentity.OS,
			Architecture: h.SystemIdentity.Architecture,
			Pwd:          h.SystemIdentity.PWD,
			CurrentUser:  h.SystemIdentity.CurrentUser,
			CpuCount:     int32(h.SystemIdentity.CPUCount),
			MemoryMb:     int32(h.SystemIdentity.MemoryMB),
		},
		NetworkInfo: &operatorv1.NetworkInfo{
			PublicIp:   h.NetworkInfo.PublicIP,
			InternalIp: h.NetworkInfo.InternalIP,
			Interfaces: h.NetworkInfo.Interfaces,
		},
		VersionInfo: &operatorv1.VersionInfo{
			OperatorVersion: h.VersionInfo.OperatorVersion,
			Status:          h.VersionInfo.Status,
		},
		UptimeInfo: &operatorv1.UptimeInfo{
			Uptime:        h.UptimeInfo.Uptime,
			UptimeSeconds: h.UptimeInfo.UptimeSeconds,
		},
		PerformanceMetrics: &operatorv1.PerformanceMetrics{
			CpuPercent:     h.PerformanceMetrics.CPUPercent,
			MemoryPercent:  h.PerformanceMetrics.MemoryPercent,
			DiskPercent:    h.PerformanceMetrics.DiskPercent,
			NetworkLatency: h.PerformanceMetrics.NetworkLatency,
			MemoryUsedMb:   int32(h.PerformanceMetrics.MemoryUsedMB),
			MemoryTotalMb:  int32(h.PerformanceMetrics.MemoryTotalMB),
			DiskUsedGb:     h.PerformanceMetrics.DiskUsedGB,
			DiskTotalGb:    h.PerformanceMetrics.DiskTotalGB,
		},
		OsDetails: &operatorv1.OSDetails{
			Kernel:  h.OSDetails.Kernel,
			Distro:  h.OSDetails.Distro,
			Version: h.OSDetails.Version,
		},
		UserDetails: &operatorv1.UserDetails{
			Username: h.UserDetails.Username,
			Uid:      h.UserDetails.UID,
			Gid:      h.UserDetails.GID,
			Home:     h.UserDetails.Home,
			Name:     h.UserDetails.Name,
			Shell:    h.UserDetails.Shell,
		},
		DiskDetails: &operatorv1.DiskDetails{
			TotalGb: h.DiskDetails.TotalGB,
			UsedGb:  h.DiskDetails.UsedGB,
			FreeGb:  h.DiskDetails.FreeGB,
			Percent: h.DiskDetails.Percent,
		},
		MemoryDetails: &operatorv1.MemoryDetails{
			TotalMb:     h.MemoryDetails.TotalMB,
			AvailableMb: h.MemoryDetails.AvailableMB,
			UsedMb:      h.MemoryDetails.UsedMB,
			Percent:     h.MemoryDetails.Percent,
		},
		Environment: &operatorv1.EnvironmentDetails{
			Pwd:              h.Environment.PWD,
			Lang:             h.Environment.Lang,
			Timezone:         h.Environment.Timezone,
			Term:             h.Environment.Term,
			IsContainer:      h.Environment.IsContainer,
			ContainerRuntime: h.Environment.ContainerRuntime,
			ContainerSignals: h.Environment.ContainerSignals,
			InitSystem:       h.Environment.InitSystem,
		},
		CapabilityFlags: &operatorv1.CapabilityFlags{
			LocalStorageEnabled: h.CapabilityFlags.LocalStorageEnabled,
			GitAvailable:        h.CapabilityFlags.GitAvailable,
			LedgerMirrorEnabled: h.CapabilityFlags.LedgerMirrorEnabled,
		},
		SystemFingerprint: h.SystemFingerprint,
		ApiKey:            h.APIKey,
	}

	if h.FingerprintDetails != nil {
		p.FingerprintDetails = &operatorv1.FingerprintDetails{
			Os:           h.FingerprintDetails.OS,
			Architecture: h.FingerprintDetails.Architecture,
			CpuCount:     int32(h.FingerprintDetails.CPUCount),
			MachineId:    h.FingerprintDetails.MachineID,
		}
	}

	for _, iface := range h.NetworkInfo.ConnectivityStatus {
		p.NetworkInfo.ConnectivityStatus = append(p.NetworkInfo.ConnectivityStatus, &operatorv1.NetworkInterface{
			Name: iface.Name,
			Ip:   iface.IP,
			Mtu:  int32(iface.MTU),
		})
	}

	return p
}

// HandleRequest processes an inbound heartbeat request message.
func (hs *HeartbeatService) HandleRequest(ctx context.Context, msg PubSubCommandMessage) {
	var protoReq operatorv1.HeartbeatRequested
	if err := proto.Unmarshal(msg.Payload, &protoReq); err != nil {
		hs.logger.Error("[HEARTBEAT] Failed to decode heartbeat request payload as protobuf HeartbeatRequested", "error", err)
		return
	}

	hs.logger.Info("[HEARTBEAT] Heartbeat request received (via Protobuf)",
		"case_id", msg.CaseID,
		"investigation_id", msg.InvestigationID,
		"operator_session_id", msg.OperatorSessionID)
	heartbeat := hs.Build(models.HeartbeatTypeRequested)
	heartbeat.CaseID = msg.CaseID
	heartbeat.InvestigationID = msg.InvestigationID
	if hs.results != nil {
		protoHeartbeat := hs.buildProtoHeartbeat(heartbeat)
		if err := hs.results.PublishHeartbeat(ctx, protoHeartbeat); err != nil {
			hs.logger.Error("[HEARTBEAT] Failed to send requested heartbeat", "error", err)
		} else {
			hs.logger.Info("[HEARTBEAT] Requested heartbeat sent successfully")
		}
	} else {
		hs.logger.Warn("[HEARTBEAT] Results publisher not set, cannot send heartbeat")
	}
}

// SendAutomatic builds and publishes an automatic heartbeat immediately.
func (hs *HeartbeatService) SendAutomatic() {
	hs.logger.Info("[HEARTBEAT] Sending automatic heartbeat")
	heartbeat := hs.Build(models.HeartbeatTypeAutomatic)
	if hs.results != nil {
		protoHeartbeat := hs.buildProtoHeartbeat(heartbeat)
		if err := hs.results.PublishHeartbeat(hs.ctx, protoHeartbeat); err != nil {
			hs.logger.Error("[HEARTBEAT] Failed to send automatic heartbeat", "error", err)
		} else {
			hs.logger.Info("[HEARTBEAT] Automatic heartbeat sent successfully")
		}
	} else {
		hs.logger.Warn("[HEARTBEAT] Results publisher not set, cannot send automatic heartbeat")
	}
}

// StartScheduler starts the periodic heartbeat ticker goroutine.
// Must be called with hs.mu held by the caller (StartSchedulerUnlocked variant)
// or via StartScheduler (acquires lock itself).
func (hs *HeartbeatService) StartScheduler() {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.startSchedulerUnlocked()
}

// StartSchedulerUnlocked starts the scheduler without acquiring the lock.
// Caller must hold hs.mu.
func (hs *HeartbeatService) StartSchedulerUnlocked() {
	hs.startSchedulerUnlocked()
}

func (hs *HeartbeatService) startSchedulerUnlocked() {
	if hs.config.HeartbeatInterval <= 0 {
		hs.logger.Info("[HEARTBEAT] Heartbeat scheduler disabled (interval <= 0)")
		return
	}

	hs.ticker = time.NewTicker(hs.config.HeartbeatInterval)
	hs.done = make(chan bool)
	ticker := hs.ticker
	done := hs.done

	hs.logger.Info("[HEARTBEAT] Heartbeat scheduler started",
		"interval_seconds", hs.config.HeartbeatInterval.Seconds())

	hs.wg.Add(1)
	go func() {
		defer hs.wg.Done()
		for {
			select {
			case <-done:
				hs.logger.Info("[HEARTBEAT] Heartbeat scheduler stopped via done channel")
				return
			case <-ticker.C:
				hs.SendAutomatic()
			case <-hs.ctx.Done():
				hs.logger.Info("[HEARTBEAT] Heartbeat scheduler stopped via context cancellation")
				return
			}
		}
	}()
}

// StopScheduler stops the periodic heartbeat ticker.
func (hs *HeartbeatService) StopScheduler() {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.stopSchedulerUnlocked()
}

// StopSchedulerUnlocked stops the scheduler without acquiring the lock.
// Caller must hold hs.mu.
func (hs *HeartbeatService) StopSchedulerUnlocked() {
	hs.stopSchedulerUnlocked()
}

func (hs *HeartbeatService) stopSchedulerUnlocked() {
	if hs.ticker != nil {
		hs.ticker.Stop()
		hs.ticker = nil
		hs.logger.Info("[HEARTBEAT] Heartbeat ticker stopped")
	}
	if hs.done != nil {
		close(hs.done)
		hs.done = nil
	}
}
