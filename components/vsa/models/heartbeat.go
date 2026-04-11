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
	"github.com/g8e-ai/g8e/components/vsa/constants"
)

type HeartbeatType = constants.HeartbeatType

const (
	HeartbeatTypeAutomatic = constants.HeartbeatTypeAutomatic
	HeartbeatTypeBootstrap = constants.HeartbeatTypeBootstrap
	HeartbeatTypeRequested = constants.HeartbeatTypeRequested
)

type HeartbeatSystemIdentity struct {
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	PWD          string `json:"pwd"`
	CurrentUser  string `json:"current_user"`
	CPUCount     int    `json:"cpu_count"`
	MemoryMB     int    `json:"memory_mb"`
}

type HeartbeatNetworkInterface struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	MTU  int    `json:"mtu"`
}

type HeartbeatNetworkInfo struct {
	PublicIP           string                      `json:"public_ip"`
	Interfaces         []string                    `json:"interfaces"`
	ConnectivityStatus []HeartbeatNetworkInterface `json:"connectivity_status"`
}

type HeartbeatCapabilityFlags struct {
	LocalStorageEnabled bool `json:"local_storage_enabled"`
	GitAvailable        bool `json:"git_available"`
	LedgerMirrorEnabled bool `json:"ledger_enabled"`
}

type HeartbeatVersionInfo struct {
	OperatorVersion string `json:"operator_version"`
	Status          string `json:"status"`
}

type HeartbeatUptimeInfo struct {
	Uptime        string `json:"uptime"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type HeartbeatPerformanceMetrics struct {
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryPercent  float64 `json:"memory_percent"`
	DiskPercent    float64 `json:"disk_percent"`
	NetworkLatency float64 `json:"network_latency"`
	MemoryUsedMB   int     `json:"memory_used_mb"`
	MemoryTotalMB  int     `json:"memory_total_mb"`
	DiskUsedGB     float64 `json:"disk_used_gb"`
	DiskTotalGB    float64 `json:"disk_total_gb"`
}

type HeartbeatOSDetails struct {
	Kernel  string `json:"kernel"`
	Distro  string `json:"distro"`
	Version string `json:"version"`
}

type HeartbeatUserDetails struct {
	Username string `json:"username"`
	UID      string `json:"uid"`
	GID      string `json:"gid"`
	Home     string `json:"home"`
	Name     string `json:"name"`
	Shell    string `json:"shell"`
}

type HeartbeatDiskDetails struct {
	TotalGB float64 `json:"total_gb"`
	UsedGB  float64 `json:"used_gb"`
	FreeGB  float64 `json:"free_gb"`
	Percent float64 `json:"percent"`
}

type HeartbeatMemoryDetails struct {
	TotalMB     int64   `json:"total_mb"`
	AvailableMB int64   `json:"available_mb"`
	UsedMB      int64   `json:"used_mb"`
	Percent     float64 `json:"percent"`
}

type HeartbeatEnvironment struct {
	PWD              string   `json:"pwd"`
	Lang             string   `json:"lang"`
	Timezone         string   `json:"timezone"`
	Term             string   `json:"term"`
	IsContainer      bool     `json:"is_container"`
	ContainerRuntime string   `json:"container_runtime"`
	ContainerSignals []string `json:"container_signals"`
	InitSystem       string   `json:"init_system"`
}

type Heartbeat struct {
	EventType         string        `json:"event_type"`
	SourceComponent   string        `json:"source_component"`
	OperatorID        string        `json:"operator_id"`
	OperatorSessionID string        `json:"operator_session_id"`
	CaseID            string        `json:"case_id"`
	InvestigationID   string        `json:"investigation_id"`
	Timestamp         string        `json:"timestamp"`
	HeartbeatType     HeartbeatType `json:"heartbeat_type"`

	SystemIdentity HeartbeatSystemIdentity `json:"system_identity"`
	NetworkInfo    HeartbeatNetworkInfo    `json:"network_info"`
	VersionInfo    HeartbeatVersionInfo    `json:"version_info"`
	UptimeInfo     HeartbeatUptimeInfo     `json:"uptime_info"`

	PerformanceMetrics HeartbeatPerformanceMetrics `json:"performance_metrics"`

	OSDetails     HeartbeatOSDetails     `json:"os_details"`
	UserDetails   HeartbeatUserDetails   `json:"user_details"`
	DiskDetails   HeartbeatDiskDetails   `json:"disk_details"`
	MemoryDetails HeartbeatMemoryDetails `json:"memory_details"`
	Environment   HeartbeatEnvironment   `json:"environment"`

	CapabilityFlags HeartbeatCapabilityFlags `json:"capability_flags"`

	APIKey string `json:"api_key,omitempty"`
}
