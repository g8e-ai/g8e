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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/system"
	"github.com/g8e-ai/g8e/internal/uuid"
	govpkg "github.com/g8e-ai/g8e/internal/governance"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/proto"
)

// HeartbeatService owns all heartbeat logic for the g8eo operator:
// building heartbeat payloads, handling inbound heartbeat requests,
// sending automatic heartbeats, and managing the periodic scheduler.
type HeartbeatService struct {
	config   *config.Config
	logger   *slog.Logger
	results  ResultsPublisher
	actuator *governance.L5Actuator

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

// SetActuator sets the L5Actuator for the HeartbeatService.
func (hs *HeartbeatService) SetActuator(actuator *governance.L5Actuator) {
	hs.actuator = actuator
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
	pwd, err := os.Getwd()
	if err != nil {
		hs.logger.Warn("[HEARTBEAT] Failed to get working directory", "error", err)
		pwd = ""
	}

	heartbeat := &models.Heartbeat{
		EventType:         constants.Event.Operator.Heartbeat,
		SourceComponent:   constants.ComponentNameG8EO,
		OperatorID:        hs.config.OperatorID,
		OperatorSessionID: hs.config.OperatorSessionId,
		CaseID:            "",
		InvestigationID:   "",
		Timestamp:         models.NowTimestamp(),
		HeartbeatType:     heartbeatType,
		SystemIdentity: models.HeartbeatSystemIdentity{
			Hostname:     system.GetHostname(),
			OS:           constants.Platform(system.GetOSName()),
			Architecture: system.GetArchitecture(),
			PWD:          pwd,
			CurrentUser:  system.GetCurrentUser(),
			CPUCount:     system.GetNumCPU(),
			MemoryMB:     system.GetMemoryMB(),
		},
		NetworkInfo: models.HeartbeatNetworkInfo{
			HTTPPort:           hs.config.Gateway.HTTPPort,
			HTTPSPort:          hs.config.Gateway.HTTPSPort,
			Interfaces:         system.GetNetworkInterfaces(),
			ConnectivityStatus: system.GetConnectivityStatus(),
		},
		VersionInfo: models.HeartbeatVersionInfo{
			OperatorVersion: hs.config.Version,
			Status:          constants.VersionStabilityStable,
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
			ExecutionVaultEnabled: hs.config.ExecutionVaultEnabled,
			GitAvailable:          hs.config.GitAvailable,
			LedgerMirrorEnabled:   hs.config.GitAvailable && !hs.config.NoGit,
		},
		FingerprintDetails: &models.HeartbeatFingerprintDetails{
			OS:           constants.Platform(runtime.GOOS),
			Architecture: runtime.GOARCH,
			CPUCount:     runtime.NumCPU(),
			MachineID:    hs.config.SystemFingerprint,
		},
		SystemFingerprint: hs.config.SystemFingerprint,
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
		"http_port", heartbeat.NetworkInfo.HTTPPort,
		"https_port", heartbeat.NetworkInfo.HTTPSPort)

	return heartbeat
}

// buildProtoHeartbeat converts the in-memory models.Heartbeat into the canonical
// operatorv1.HeartbeatResult protobuf carried on the pub/sub heartbeat channel.
func (hs *HeartbeatService) buildProtoHeartbeat(h *models.Heartbeat) *operatorv1.HeartbeatResult {
	p := &operatorv1.HeartbeatResult{
		OperatorId:        h.OperatorID,
		OperatorSessionId: h.OperatorSessionID,
		Timestamp:         h.Timestamp,
		Status:            string(h.HeartbeatType),
		EventType:         string(h.EventType),
		SourceComponent:   string(h.SourceComponent),
		CaseId:            h.CaseID,
		InvestigationId:   h.InvestigationID,
		SystemIdentity: &operatorv1.SystemIdentity{
			Hostname:     h.SystemIdentity.Hostname,
			Os:           string(h.SystemIdentity.OS),
			Architecture: h.SystemIdentity.Architecture,
			Pwd:          h.SystemIdentity.PWD,
			CurrentUser:  h.SystemIdentity.CurrentUser,
			CpuCount:     int32(h.SystemIdentity.CPUCount), //nolint:gosec // realistically < 1000
			MemoryMb:     int32(h.SystemIdentity.MemoryMB), //nolint:gosec // realistically < 1TB (1,000,000 MB)
		},
		NetworkInfo: &operatorv1.NetworkInfo{
			PublicIp:   "",
			InternalIp: "",
			Interfaces: h.NetworkInfo.Interfaces,
		},
		VersionInfo: &operatorv1.VersionInfo{
			OperatorVersion: h.VersionInfo.OperatorVersion,
			Status:          string(h.VersionInfo.Status),
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
			MemoryUsedMb:   int32(h.PerformanceMetrics.MemoryUsedMB),  //nolint:gosec // realistically < 1TB
			MemoryTotalMb:  int32(h.PerformanceMetrics.MemoryTotalMB), //nolint:gosec // realistically < 1TB
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
			LocalStorageEnabled: h.CapabilityFlags.ExecutionVaultEnabled,
			GitAvailable:        h.CapabilityFlags.GitAvailable,
			LedgerMirrorEnabled: h.CapabilityFlags.LedgerMirrorEnabled,
		},
		SystemFingerprint: h.SystemFingerprint,
	}

	if h.FingerprintDetails != nil {
		p.FingerprintDetails = &operatorv1.FingerprintDetails{
			Os:           string(h.FingerprintDetails.OS),
			Architecture: h.FingerprintDetails.Architecture,
			CpuCount:     int32(h.FingerprintDetails.CPUCount), //nolint:gosec // realistically < 1000
			MachineId:    h.FingerprintDetails.MachineID,
		}
	}

	for _, iface := range h.NetworkInfo.ConnectivityStatus {
		p.NetworkInfo.ConnectivityStatus = append(p.NetworkInfo.ConnectivityStatus, &operatorv1.NetworkInterface{
			Name: iface.Name,
			Ip:   iface.IP,
			Mtu:  int32(iface.MTU), //nolint:gosec // MTU is typically < 9000
		})
	}

	return p
}

// Publish publishes a heartbeat to the results publisher.
func (hs *HeartbeatService) Publish(ctx context.Context, heartbeat *operatorv1.HeartbeatResult) error {
	if hs.results == nil {
		return constants.ErrPubSubResultsPublisher
	}
	return hs.results.PublishHeartbeat(ctx, heartbeat)
}

// HandleRequest processes an inbound heartbeat request message.
func (hs *HeartbeatService) HandleRequest(ctx context.Context, msg *PubSubCommandMessage) {
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
		if hs.config.Gateway.Enabled {
			hs.logger.Debug("[HEARTBEAT] Results publisher not set, skipping heartbeat in gateway mode")
		} else {
			hs.logger.Warn("[HEARTBEAT] Results publisher not set, cannot send heartbeat")
		}
	}
}

// SendAutomatic builds and publishes an automatic heartbeat immediately.
func (hs *HeartbeatService) SendAutomatic() error {
	hs.logger.Info("[HEARTBEAT] Sending automatic heartbeat")
	heartbeat := hs.Build(models.HeartbeatTypeAutomatic)

	data, err := json.Marshal(heartbeat)
	if err != nil {
		return fmt.Errorf("heartbeat: failed to marshal heartbeat: %w", err)
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	env := &govpkg.GovernanceEnvelope{
		Id:              uuid.NewString(),
		TransactionHash: hashStr,
		ActionType:      string(constants.ActionTypeHeartbeat),
		Payload:         data,
	}

	vt := &governance.VerifiedTransaction{
		Envelope:   env,
		ActionType: constants.ActionTypeHeartbeat,
	}

	cmdMsg := &PubSubCommandMessage{
		ID:        env.Id,
		Payload:   data,
		EventType: constants.Event.Operator.Heartbeat,
	}

	if hs.actuator != nil {
		_, err := hs.actuator.Execute(hs.ctx, vt, cmdMsg)
		if err != nil {
			return fmt.Errorf("heartbeat: actuator execution failed: %w", err)
		}
	} else {
		hs.logger.Warn("[HEARTBEAT] Actuator service not set, skipping receipted heartbeat dispatch")
	}

	return nil
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
				if err := hs.SendAutomatic(); err != nil {
					hs.logger.Error("[HEARTBEAT] Failed to send automatic heartbeat", "error", err)
				}
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
