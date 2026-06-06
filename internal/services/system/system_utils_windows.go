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

//go:build windows
// +build windows

package system

import (
	"fmt"
	"math"
	"net"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"time"
	"unsafe"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"golang.org/x/sys/windows"
)

// Windows API structures and constants
var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

const (
	PROCESSOR_ARCHITECTURE_AMD64 = 9
	PROCESSOR_ARCHITECTURE_ARM64 = 12
	PROCESSOR_ARCHITECTURE_INTEL = 0
)

type MEMORYSTATUSEX struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

func GetUptime() string {
	uptimeSeconds := GetUptimeSeconds()
	duration := time.Duration(uptimeSeconds) * time.Second
	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%d days, %02d:%02d:%02d", days, hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func GetUptimeSeconds() int64 {
	ret, _, _ := procGetTickCount64.Call()
	if ret == 0 {
		return 0
	}
	return int64(ret / 1000) // Convert milliseconds to seconds
}

func GetCPUPercent() float64 {
	// Windows CPU monitoring requires more complex API calls
	// For now, return 0 - full implementation would use GetSystemTimes
	return 0.0
}

func GetMemoryPercent() float64 {
	var memStatus MEMORYSTATUSEX
	memStatus.dwLength = uint32(unsafe.Sizeof(memStatus))

	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if ret == 0 {
		return 0.0
	}

	if memStatus.ullTotalPhys == 0 {
		return 0.0
	}

	used := memStatus.ullTotalPhys - memStatus.ullAvailPhys
	memoryPercent := float64(used) / float64(memStatus.ullTotalPhys) * 100.0
	return float64(int(memoryPercent*100+0.5)) / 100
}

func GetNetworkLatency() float64 {
	start := time.Now().UTC()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:22", 1*time.Second)
	if err != nil {
		start = time.Now().UTC()
		conn, err = net.DialTimeout("tcp", "127.0.0.1:80", 1*time.Second)
		if err != nil {
			return 1.0
		}
	}
	defer conn.Close()

	latency := time.Since(start).Seconds() * 1000
	return math.Round(latency*100) / 100
}

func GetDiskPercent() float64 {
	var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	procGetDiskFreeSpaceExW := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytes, totalBytes, totalFreeBytes uint64

	ret, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("C:\\"))),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	if ret == 0 || totalBytes == 0 {
		return 0.0
	}

	used := totalBytes - freeBytes
	diskPercent := float64(used) / float64(totalBytes) * 100.0
	return float64(int(diskPercent*100+0.5)) / 100
}

func GetOSDetails() models.HeartbeatOSDetails {
	return models.HeartbeatOSDetails{
		Kernel:  getWindowsKernelVersion(),
		Distro:  "Windows",
		Version: getWindowsVersion(),
	}
}

func getWindowsKernelVersion() string {
	version := getWindowsVersion()
	return fmt.Sprintf("Windows %s", version)
}

func getWindowsVersion() string {
	version := windows.RtlGetVersion()
	if version == nil {
		return "Unknown"
	}
	return fmt.Sprintf("%d.%d.%d", version.MajorVersion, version.MinorVersion, version.BuildNumber)
}

func getDistroName() string {
	return "Windows"
}

func getDistroVersion() string {
	return getWindowsVersion()
}

func GetUserDetails(shell string) models.HeartbeatUserDetails {
	if shell == "" {
		shell = "cmd.exe"
	}
	currentUser, err := user.Current()
	if err != nil {
		return models.HeartbeatUserDetails{
			Username: string(constants.SystemHealthUnknown),
			Shell:    shell,
		}
	}
	return models.HeartbeatUserDetails{
		Username: currentUser.Username,
		UID:      parseUserID(currentUser.Uid),
		GID:      parseUserID(currentUser.Gid),
		Home:     currentUser.HomeDir,
		Name:     currentUser.Name,
		Shell:    shell,
	}
}

const maxInt32 = int64(1<<31 - 1)

func parseUserID(value string) int32 {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 0 || id > maxInt32 {
		return 0
	}
	return int32(id)
}

type ContainerInfo struct {
	IsContainer bool     `json:"is_container"`
	Runtime     string   `json:"container_runtime"`
	Signals     []string `json:"container_signals"`
}

func GetDiskDetails() models.HeartbeatDiskDetails {
	var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	procGetDiskFreeSpaceExW := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytes, totalBytes, totalFreeBytes uint64

	ret, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("C:\\"))),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	if ret == 0 {
		return models.HeartbeatDiskDetails{}
	}

	used := totalBytes - freeBytes

	totalGB := float64(totalBytes) / (1024 * 1024 * 1024)
	usedGB := float64(used) / (1024 * 1024 * 1024)
	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
	percent := 0.0
	if totalBytes > 0 {
		percent = float64(used) / float64(totalBytes) * 100.0
	}

	return models.HeartbeatDiskDetails{
		TotalGB: math.Round(totalGB*10) / 10,
		UsedGB:  math.Round(usedGB*10) / 10,
		FreeGB:  math.Round(freeGB*10) / 10,
		Percent: math.Round(percent*10) / 10,
	}
}

func GetDiskUsedGB() float64 {
	details := GetDiskDetails()
	return details.UsedGB
}

func GetDiskTotalGB() float64 {
	details := GetDiskDetails()
	return details.TotalGB
}

func GetMemoryDetails() models.HeartbeatMemoryDetails {
	var memStatus MEMORYSTATUSEX
	memStatus.dwLength = uint32(unsafe.Sizeof(memStatus))

	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if ret == 0 {
		return models.HeartbeatMemoryDetails{}
	}

	totalMB := memStatus.ullTotalPhys / (1024 * 1024)
	availableMB := memStatus.ullAvailPhys / (1024 * 1024)
	usedMB := totalMB - availableMB
	percent := 0.0
	if memStatus.ullTotalPhys > 0 {
		percent = float64(memStatus.ullTotalPhys-memStatus.ullAvailPhys) / float64(memStatus.ullTotalPhys) * 100.0
	}

	return models.HeartbeatMemoryDetails{
		TotalMB:     int64(totalMB),
		AvailableMB: int64(availableMB),
		UsedMB:      int64(usedMB),
		Percent:     math.Round(percent*10) / 10,
	}
}

func GetEnvironmentDetails(lang, term, tz string) models.HeartbeatEnvironment {
	pwd, _ := os.Getwd()

	initSystem := "Windows Service Manager"

	return models.HeartbeatEnvironment{
		PWD:              pwd,
		Lang:             lang,
		Timezone:         getTimezone(tz),
		Term:             term,
		IsContainer:      false,
		ContainerRuntime: "none",
		ContainerSignals: []string{},
		InitSystem:       initSystem,
	}
}

func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return string(constants.SystemHealthUnknown)
	}
	return hostname
}

func GetOSName() string {
	return runtime.GOOS
}

func GetArchitecture() string {
	return runtime.GOARCH
}

func GetNumCPU() int {
	return runtime.NumCPU()
}

func GetMemoryMB() int {
	var memStatus MEMORYSTATUSEX
	memStatus.dwLength = uint32(unsafe.Sizeof(memStatus))

	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if ret == 0 {
		return 0
	}

	return int(memStatus.ullTotalPhys / (1024 * 1024))
}

func GetCurrentUser() string {
	currentUser, err := user.Current()
	if err != nil {
		return string(constants.SystemHealthUnknown)
	}
	return currentUser.Username
}

func GetLocalIP(ipResolver string) string {
	// Use the cross-platform implementation from the main file
	return "127.0.0.1" // Placeholder
}

func GetNetworkInterfaces() []string {
	// Windows network interface enumeration requires Win32 API
	// For now, return empty
	return []string{}
}

func GetConnectivityStatus() []models.HeartbeatNetworkInterface {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []models.HeartbeatNetworkInterface{}
	}

	var activeInterfaces []models.HeartbeatNetworkInterface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					activeInterfaces = append(activeInterfaces, models.HeartbeatNetworkInterface{
						Name: iface.Name,
						IP:   ipnet.IP.String(),
						MTU:  iface.MTU,
					})
				}
			}
		}
	}
	return activeInterfaces
}

func getTimezone(tz string) string {
	if tz != "" {
		return tz
	}
	// Windows timezone detection
	// For now, return UTC as fallback
	return "UTC"
}
