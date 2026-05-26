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

package models

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
)

func TestHeartbeatSystemIdentity(t *testing.T) {
	t.Run("creates valid system identity", func(t *testing.T) {
		identity := &HeartbeatSystemIdentity{
			Hostname:     "test-host",
			OS:           constants.PlatformLinux,
			Architecture: "amd64",
			PWD:          "/home/user",
			CurrentUser:  "testuser",
			CPUCount:     4,
			MemoryMB:     8192,
		}

		assert.Equal(t, "test-host", identity.Hostname)
		assert.Equal(t, constants.PlatformLinux, identity.OS)
		assert.Equal(t, 4, identity.CPUCount)
		assert.Equal(t, 8192, identity.MemoryMB)
	})
}

func TestHeartbeatNetworkInterface(t *testing.T) {
	t.Run("creates valid network interface", func(t *testing.T) {
		iface := &HeartbeatNetworkInterface{
			Name: "eth0",
			IP:   "192.168.1.1",
			MTU:  1500,
		}

		assert.Equal(t, "eth0", iface.Name)
		assert.Equal(t, "192.168.1.1", iface.IP)
		assert.Equal(t, 1500, iface.MTU)
	})
}

func TestHeartbeatNetworkInfo(t *testing.T) {
	t.Run("creates valid network info", func(t *testing.T) {
		info := &HeartbeatNetworkInfo{
			PublicIP:   "1.2.3.4",
			InternalIP: "192.168.1.1",
			Interfaces: []string{"eth0", "wlan0"},
			ConnectivityStatus: []HeartbeatNetworkInterface{
				{Name: "eth0", IP: "192.168.1.1", MTU: 1500},
			},
		}

		assert.Equal(t, "1.2.3.4", info.PublicIP)
		assert.Len(t, info.Interfaces, 2)
	})
}

func TestHeartbeatCapabilityFlags(t *testing.T) {
	t.Run("creates valid capability flags", func(t *testing.T) {
		flags := &HeartbeatCapabilityFlags{
			LocalStorageEnabled: true,
			GitAvailable:        true,
			LedgerMirrorEnabled: false,
		}

		assert.True(t, flags.LocalStorageEnabled)
		assert.True(t, flags.GitAvailable)
		assert.False(t, flags.LedgerMirrorEnabled)
	})
}

func TestHeartbeatVersionInfo(t *testing.T) {
	t.Run("creates valid version info", func(t *testing.T) {
		info := &HeartbeatVersionInfo{
			OperatorVersion: "v1.0.0",
			Status:          constants.VersionStabilityStable,
		}

		assert.Equal(t, "v1.0.0", info.OperatorVersion)
		assert.Equal(t, constants.VersionStabilityStable, info.Status)
	})
}

func TestHeartbeatUptimeInfo(t *testing.T) {
	t.Run("creates valid uptime info", func(t *testing.T) {
		info := &HeartbeatUptimeInfo{
			Uptime:        "2h30m",
			UptimeSeconds: 9000,
		}

		assert.Equal(t, "2h30m", info.Uptime)
		assert.Equal(t, int64(9000), info.UptimeSeconds)
	})
}

func TestHeartbeatPerformanceMetrics(t *testing.T) {
	t.Run("creates valid performance metrics", func(t *testing.T) {
		metrics := &HeartbeatPerformanceMetrics{
			CPUPercent:     25.5,
			MemoryPercent:  60.0,
			DiskPercent:    40.0,
			NetworkLatency: 10.5,
			MemoryUsedMB:   4915,
			MemoryTotalMB:  8192,
			DiskUsedGB:     100.0,
			DiskTotalGB:    250.0,
		}

		assert.Equal(t, 25.5, metrics.CPUPercent)
		assert.Equal(t, 60.0, metrics.MemoryPercent)
		assert.Equal(t, 4915, metrics.MemoryUsedMB)
	})
}

func TestHeartbeatOSDetails(t *testing.T) {
	t.Run("creates valid OS details", func(t *testing.T) {
		details := &HeartbeatOSDetails{
			Kernel:  "5.15.0",
			Distro:  "Ubuntu",
			Version: "22.04",
		}

		assert.Equal(t, "5.15.0", details.Kernel)
		assert.Equal(t, "Ubuntu", details.Distro)
	})
}

func TestHeartbeatUserDetails(t *testing.T) {
	t.Run("creates valid user details", func(t *testing.T) {
		details := &HeartbeatUserDetails{
			Username: "testuser",
			UID:      1000,
			GID:      1000,
			Home:     "/home/testuser",
			Name:     "Test User",
			Shell:    "/bin/bash",
		}

		assert.Equal(t, "testuser", details.Username)
		assert.Equal(t, int32(1000), details.UID)
		assert.Equal(t, "/home/testuser", details.Home)
	})
}

func TestHeartbeatDiskDetails(t *testing.T) {
	t.Run("creates valid disk details", func(t *testing.T) {
		details := &HeartbeatDiskDetails{
			TotalGB: 250.0,
			UsedGB:  100.0,
			FreeGB:  150.0,
			Percent: 40.0,
		}

		assert.Equal(t, 250.0, details.TotalGB)
		assert.Equal(t, 40.0, details.Percent)
	})
}

func TestHeartbeatMemoryDetails(t *testing.T) {
	t.Run("creates valid memory details", func(t *testing.T) {
		details := &HeartbeatMemoryDetails{
			TotalMB:     8192,
			AvailableMB: 3277,
			UsedMB:      4915,
			Percent:     60.0,
		}

		assert.Equal(t, int64(8192), details.TotalMB)
		assert.Equal(t, 60.0, details.Percent)
	})
}

func TestHeartbeatEnvironment(t *testing.T) {
	t.Run("creates valid environment", func(t *testing.T) {
		env := &HeartbeatEnvironment{
			PWD:              "/home/user",
			Lang:             "en_US.UTF-8",
			Timezone:         "UTC",
			Term:             "xterm-256color",
			IsContainer:      true,
			ContainerRuntime: "docker",
			ContainerSignals: []string{"SIGTERM", "SIGINT"},
			InitSystem:       "systemd",
		}

		assert.Equal(t, "/home/user", env.PWD)
		assert.True(t, env.IsContainer)
		assert.Equal(t, "docker", env.ContainerRuntime)
	})
}

func TestHeartbeatFingerprintDetails(t *testing.T) {
	t.Run("creates valid fingerprint details", func(t *testing.T) {
		details := &HeartbeatFingerprintDetails{
			OS:           constants.PlatformLinux,
			Architecture: "amd64",
			CPUCount:     4,
			MachineID:    "machine-123",
		}

		assert.Equal(t, constants.PlatformLinux, details.OS)
		assert.Equal(t, 4, details.CPUCount)
		assert.Equal(t, "machine-123", details.MachineID)
	})
}

func TestHeartbeat(t *testing.T) {
	t.Run("creates valid heartbeat", func(t *testing.T) {
		heartbeat := &Heartbeat{
			EventType:         constants.Event.Operator.Heartbeat,
			SourceComponent:   constants.ComponentNameG8EO,
			OperatorID:        "operator-123",
			OperatorSessionID: "session-123",
			CaseID:            "case-123",
			InvestigationID:   "inv-123",
			Timestamp:         "2026-01-01T00:00:00Z",
			HeartbeatType:     HeartbeatTypeAutomatic,
			SystemIdentity: HeartbeatSystemIdentity{
				Hostname: "test-host",
				OS:       constants.PlatformLinux,
			},
			NetworkInfo: HeartbeatNetworkInfo{
				PublicIP:   "1.2.3.4",
				InternalIP: "192.168.1.1",
			},
			VersionInfo: HeartbeatVersionInfo{
				OperatorVersion: "v1.0.0",
				Status:          constants.VersionStabilityStable,
			},
			UptimeInfo: HeartbeatUptimeInfo{
				Uptime:        "2h30m",
				UptimeSeconds: 9000,
			},
			PerformanceMetrics: HeartbeatPerformanceMetrics{
				CPUPercent: 25.5,
			},
			OSDetails: HeartbeatOSDetails{
				Kernel: "5.15.0",
			},
			UserDetails: HeartbeatUserDetails{
				Username: "testuser",
			},
			DiskDetails: HeartbeatDiskDetails{
				TotalGB: 250.0,
			},
			MemoryDetails: HeartbeatMemoryDetails{
				TotalMB: 8192,
			},
			Environment: HeartbeatEnvironment{
				PWD: "/home/user",
			},
			CapabilityFlags: HeartbeatCapabilityFlags{
				LocalStorageEnabled: true,
			},
			FingerprintDetails: &HeartbeatFingerprintDetails{
				OS:           constants.PlatformLinux,
				Architecture: "amd64",
			},
			SystemFingerprint: "fp-123",
			APIKey:            "api-key-123",
		}

		assert.Equal(t, constants.Event.Operator.Heartbeat, heartbeat.EventType)
		assert.Equal(t, constants.ComponentNameG8EO, heartbeat.SourceComponent)
		assert.Equal(t, HeartbeatTypeAutomatic, heartbeat.HeartbeatType)
		assert.NotNil(t, heartbeat.FingerprintDetails)
	})
}
