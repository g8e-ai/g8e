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
	"sync"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/config"
	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	"github.com/g8e-ai/g8e/components/vsa/services/system"
)

// HeartbeatService owns all heartbeat logic for the VSA operator:
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

	return &models.Heartbeat{
		EventType:         constants.Event.Operator.Heartbeat,
		SourceComponent:   constants.Status.ComponentName.VSA,
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

		APIKey: hs.config.APIKey,
	}
}

// HandleRequest processes an inbound heartbeat request message.
func (hs *HeartbeatService) HandleRequest(ctx context.Context, msg PubSubCommandMessage) {
	hs.logger.Info("Heartbeat requested")
	heartbeat := hs.Build(models.HeartbeatTypeRequested)
	heartbeat.CaseID = msg.CaseID
	heartbeat.InvestigationID = msg.InvestigationID
	if hs.results != nil {
		if err := hs.results.PublishHeartbeat(ctx, heartbeat); err != nil {
			hs.logger.Error("Failed to send heartbeat", "error", err)
		}
	}
}

// SendAutomatic builds and publishes an automatic heartbeat immediately.
func (hs *HeartbeatService) SendAutomatic() {
	heartbeat := hs.Build(models.HeartbeatTypeAutomatic)
	if hs.results != nil {
		if err := hs.results.PublishHeartbeat(hs.ctx, heartbeat); err != nil {
			hs.logger.Error("Failed to send automatic heartbeat", "error", err)
		}
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
		return
	}

	hs.ticker = time.NewTicker(hs.config.HeartbeatInterval)
	hs.done = make(chan bool)
	ticker := hs.ticker
	done := hs.done

	hs.wg.Add(1)
	go func() {
		defer hs.wg.Done()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				hs.SendAutomatic()
			case <-hs.ctx.Done():
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
	}
	if hs.done != nil {
		close(hs.done)
		hs.done = nil
	}
}
