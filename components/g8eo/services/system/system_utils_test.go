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

package system

import (
	"math"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConnectivityStatus(t *testing.T) {
	t.Parallel()

	status := GetConnectivityStatus()

	require.NotNil(t, status)

	for i, item := range status {
		assert.NotEmpty(t, item.Name, "interface %d Name should not be empty", i)
		assert.NotEmpty(t, item.IP, "interface %d IP should not be empty", i)
		assert.Positive(t, item.MTU, "interface %d MTU should be positive", i)
	}
}

func TestGetUptime(t *testing.T) {
	uptime := GetUptime()

	assert.NotEmpty(t, uptime)
	if uptime != "unknown" {
		assert.True(t, strings.Contains(uptime, ":") || strings.Contains(uptime, "days"),
			"GetUptime() = %q, expected time format with ':' or 'days'", uptime)
	}
}

func TestGetUptimeSeconds(t *testing.T) {
	uptime := GetUptimeSeconds()

	assert.GreaterOrEqual(t, uptime, int64(0))
	assert.LessOrEqual(t, uptime, int64(365*24*3600*10))
}

func TestReadCPUStat(t *testing.T) {
	stat, err := readCPUStat()

	require.NoError(t, err)
	require.NotNil(t, stat)

	assert.GreaterOrEqual(t, stat.user, int64(0))
	assert.GreaterOrEqual(t, stat.nice, int64(0))
	assert.GreaterOrEqual(t, stat.system, int64(0))
	assert.GreaterOrEqual(t, stat.idle, int64(0))
	assert.GreaterOrEqual(t, stat.iowait, int64(0))
	assert.GreaterOrEqual(t, stat.irq, int64(0))
	assert.GreaterOrEqual(t, stat.softirq, int64(0))

	total := stat.user + stat.nice + stat.system + stat.idle + stat.iowait + stat.irq + stat.softirq
	assert.Positive(t, total)
}

func TestGetCPUPercent(t *testing.T) {
	cpuPercent := GetCPUPercent()

	assert.GreaterOrEqual(t, cpuPercent, 0.0)
	assert.LessOrEqual(t, cpuPercent, 100.0)
	rounded := float64(int(cpuPercent*100+0.5)) / 100
	assert.InDelta(t, cpuPercent, rounded, 0.005, "GetCPUPercent() should be rounded to 2 decimal places")
}

func TestGetMemoryPercent(t *testing.T) {
	memPercent := GetMemoryPercent()

	assert.GreaterOrEqual(t, memPercent, 0.0)
	assert.LessOrEqual(t, memPercent, 100.0)
	rounded := float64(int(memPercent*100+0.5)) / 100
	assert.InDelta(t, memPercent, rounded, 0.005, "GetMemoryPercent() should be rounded to 2 decimal places")
}

func TestGetNetworkLatency(t *testing.T) {

	latency := GetNetworkLatency()

	assert.GreaterOrEqual(t, latency, 0.0)
	assert.LessOrEqual(t, latency, 1000.0)
	rounded := math.Round(latency*100) / 100
	assert.InDelta(t, latency, rounded, 0.001, "GetNetworkLatency() should be rounded to 2 decimal places")
}

func TestSystemUtilsIntegration(t *testing.T) {
	t.Parallel()

	t.Run("all functions execute without panic", func(t *testing.T) {
		require.NotNil(t, GetConnectivityStatus())
		assert.NotEmpty(t, GetUptime())
		assert.GreaterOrEqual(t, GetUptimeSeconds(), int64(0))

		cpuPercent := GetCPUPercent()
		assert.GreaterOrEqual(t, cpuPercent, 0.0)
		assert.LessOrEqual(t, cpuPercent, 100.0)

		memPercent := GetMemoryPercent()
		assert.GreaterOrEqual(t, memPercent, 0.0)
		assert.LessOrEqual(t, memPercent, 100.0)

		assert.GreaterOrEqual(t, GetNetworkLatency(), 0.0)
	})
}

func TestDetectContainerEnvironment(t *testing.T) {
	info := detectContainerEnvironment()

	assert.NotNil(t, info.Signals)
	assert.NotEmpty(t, info.Runtime)

	if _, err := os.Stat("/.dockerenv"); err == nil {
		assert.True(t, info.IsContainer)
		assert.Equal(t, "docker", info.Runtime)
		assert.Contains(t, info.Signals, "dockerenv_file")
	}

	if !info.IsContainer {
		assert.Equal(t, "none", info.Runtime)
	}
}

func TestGetInitProcessName(t *testing.T) {
	initName := getInitProcessName()

	assert.NotEmpty(t, initName)
	assert.NotContains(t, initName, "/")
	assert.NotContains(t, initName, "\x00")
}

func TestDetectInitSystem(t *testing.T) {
	assert.NotEmpty(t, detectInitSystem())
}

func TestGetEnvironmentDetails_ContainerFields(t *testing.T) {
	details := GetEnvironmentDetails("", "", "")

	assert.NotNil(t, details.ContainerSignals)
	assert.NotEmpty(t, details.ContainerRuntime)
	assert.NotEmpty(t, details.InitSystem)
	assert.NotEmpty(t, details.PWD)
}

func TestContainerInfo_EmptySignals(t *testing.T) {
	require.NotNil(t, detectContainerEnvironment().Signals)
}

func TestCPUStatConsistency(t *testing.T) {
	stat1, err := readCPUStat()
	require.NoError(t, err)

	stat2, err := readCPUStat()
	require.NoError(t, err)

	total1 := stat1.user + stat1.nice + stat1.system + stat1.idle + stat1.iowait + stat1.irq + stat1.softirq
	total2 := stat2.user + stat2.nice + stat2.system + stat2.idle + stat2.iowait + stat2.irq + stat2.softirq

	assert.GreaterOrEqual(t, total2, total1, "CPU time should be monotonically non-decreasing")
}

func TestGetDiskPercent(t *testing.T) {
	diskPercent := GetDiskPercent()

	assert.GreaterOrEqual(t, diskPercent, 0.0)
	assert.LessOrEqual(t, diskPercent, 100.0)
}

func TestGetDiskDetails(t *testing.T) {
	details := GetDiskDetails()

	assert.GreaterOrEqual(t, details.TotalGB, 0.0)
	assert.GreaterOrEqual(t, details.UsedGB, 0.0)
	assert.GreaterOrEqual(t, details.FreeGB, 0.0)
	assert.GreaterOrEqual(t, details.Percent, 0.0)
	assert.LessOrEqual(t, details.Percent, 100.0)

	if details.TotalGB > 0 {
		assert.LessOrEqual(t, details.UsedGB, details.TotalGB)
		assert.LessOrEqual(t, details.FreeGB, details.TotalGB)
	}
}

func TestGetDiskUsedGB(t *testing.T) {
	usedGB := GetDiskUsedGB()

	assert.GreaterOrEqual(t, usedGB, 0.0)
}

func TestGetDiskTotalGB(t *testing.T) {
	totalGB := GetDiskTotalGB()

	assert.GreaterOrEqual(t, totalGB, 0.0)
}

func TestGetDiskDetails_Consistency(t *testing.T) {
	details := GetDiskDetails()
	usedGB := GetDiskUsedGB()
	totalGB := GetDiskTotalGB()

	assert.InDelta(t, details.UsedGB, usedGB, 0.5, "GetDiskDetails.UsedGB and GetDiskUsedGB should be consistent")
	assert.InDelta(t, details.TotalGB, totalGB, 0.5, "GetDiskDetails.TotalGB and GetDiskTotalGB should be consistent")
}

func TestGetOSDetails(t *testing.T) {
	details := GetOSDetails()

	assert.NotEmpty(t, details.Kernel)
	assert.NotEmpty(t, details.Distro)
}

func TestReadOSReleaseField_KnownField(t *testing.T) {
	name := readOSReleaseField("NAME")

	assert.NotEmpty(t, name)
	assert.NotEqual(t, "unknown", name)
}

func TestReadOSReleaseField_MissingField(t *testing.T) {
	result := readOSReleaseField("NONEXISTENT_FIELD_XYZ")

	assert.Equal(t, "unknown", result)
}

func TestGetUserDetails(t *testing.T) {
	details := GetUserDetails("/bin/bash")

	assert.NotEmpty(t, details.Username)
	assert.Equal(t, "/bin/bash", details.Shell)
}

func TestGetUserDetails_ShellEmpty_DefaultsToSh(t *testing.T) {
	details := GetUserDetails("")

	assert.Equal(t, "/bin/sh", details.Shell)
}

func TestGetUserDetails_ShellInjected(t *testing.T) {
	details := GetUserDetails("/bin/zsh")

	assert.Equal(t, "/bin/zsh", details.Shell)
}

func TestGetMemoryDetails(t *testing.T) {
	details := GetMemoryDetails()

	assert.Greater(t, details.TotalMB, int64(0))
	assert.GreaterOrEqual(t, details.AvailableMB, int64(0))
	assert.GreaterOrEqual(t, details.UsedMB, int64(0))
	assert.GreaterOrEqual(t, details.Percent, 0.0)
	assert.LessOrEqual(t, details.Percent, 100.0)
	assert.LessOrEqual(t, details.UsedMB, details.TotalMB)
}

func TestGetMemoryMB(t *testing.T) {
	totalMB := GetMemoryMB()

	assert.Greater(t, totalMB, 0)
}

func TestGetMemoryDetails_Consistency(t *testing.T) {
	details := GetMemoryDetails()
	totalMB := GetMemoryMB()

	assert.InDelta(t, details.TotalMB, int64(totalMB), 10, "GetMemoryDetails.TotalMB and GetMemoryMB should be consistent")
}

func TestGetHostname(t *testing.T) {
	hostname := GetHostname()

	assert.NotEmpty(t, hostname)
	assert.NotEqual(t, "unknown", hostname)
}

func TestGetOSName(t *testing.T) {
	osName := GetOSName()

	assert.NotEmpty(t, osName)
	assert.Equal(t, runtime.GOOS, osName)
}

func TestGetArchitecture(t *testing.T) {
	arch := GetArchitecture()

	assert.NotEmpty(t, arch)
	assert.Equal(t, runtime.GOARCH, arch)
}

func TestGetNumCPU(t *testing.T) {
	numCPU := GetNumCPU()

	assert.Greater(t, numCPU, 0)
	assert.Equal(t, runtime.NumCPU(), numCPU)
}

func TestGetCurrentUser(t *testing.T) {
	currentUser := GetCurrentUser()

	assert.NotEmpty(t, currentUser)
	assert.NotEqual(t, "unknown", currentUser)
}

func TestGetNetworkInterfaces(t *testing.T) {
	interfaces := GetNetworkInterfaces()

	require.NotNil(t, interfaces)
	assert.Greater(t, len(interfaces), 0)

	for _, iface := range interfaces {
		assert.NotEmpty(t, iface)
	}
}

func TestGetLocalIP(t *testing.T) {
	ip := GetLocalIP("")

	assert.NotEmpty(t, ip)
	assert.NotEqual(t, "127.0.0.1", ip, "GetLocalIP should return the outbound IP, not loopback")
}

func TestGetLocalIP_CustomResolver(t *testing.T) {
	ip := GetLocalIP("8.8.8.8:80")

	assert.NotEmpty(t, ip)
}

func TestGetPublicIP(t *testing.T) {

	ip := GetPublicIP("")

	assert.NotEmpty(t, ip)
}

func TestGetTimezone_InjectedValue(t *testing.T) {
	tz := getTimezone("America/Los_Angeles")

	assert.Equal(t, "America/Los_Angeles", tz)
}

func TestGetTimezone_EmptyFallsBackToSystem(t *testing.T) {
	tz := getTimezone("")

	assert.NotEmpty(t, tz)
}

func TestGetEnvironmentDetails_AllFields(t *testing.T) {
	details := GetEnvironmentDetails("", "", "")

	assert.NotEmpty(t, details.PWD)
	assert.NotEmpty(t, details.ContainerRuntime)
	assert.NotEmpty(t, details.InitSystem)
	assert.NotNil(t, details.ContainerSignals)
}

func TestGetEnvironmentDetails_TimezoneInjected(t *testing.T) {
	details := GetEnvironmentDetails("", "", "America/New_York")

	assert.Equal(t, "America/New_York", details.Timezone)
}

func TestGetEnvironmentDetails_LangAndTermInjected(t *testing.T) {
	details := GetEnvironmentDetails("en_US.UTF-8", "xterm-256color", "")

	assert.Equal(t, "en_US.UTF-8", details.Lang)
	assert.Equal(t, "xterm-256color", details.Term)
}
