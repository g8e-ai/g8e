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

//go:build darwin
// +build darwin

package system

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"golang.org/x/sys/unix"
)

func GetUptime() string {
	uptimeSeconds := GetUptimeSeconds()
	if uptimeSeconds <= 0 {
		return string(constants.SystemHealthUnknown)
	}

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
	raw, err := unix.SysctlRaw("kern.boottime")
	if err != nil || len(raw) < 8 {
		return 0
	}

	// kern.boottime returns a Timeval struct; first 8 bytes are Sec (int64)
	sec := *(*int64)(unsafe.Pointer(&raw[0]))
	bootTime := time.Unix(sec, 0)
	return int64(time.Since(bootTime).Seconds())
}

func GetCPUPercent() float64 {
	// macOS CPU monitoring requires host_processor_info Mach traps
	return 0.0
}

func GetMemoryPercent() float64 {
	// macOS memory monitoring requires host_statistics64 Mach traps
	return 0.0
}

func GetDiskPercent() float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(constants.PathRoot, &stat); err != nil {
		return 0.0
	}

	total := stat.Blocks * uint64(stat.Bsize) //nolint:gosec // stat.Blocks bounded by filesystem
	free := stat.Bfree * uint64(stat.Bsize)   //nolint:gosec // stat.Bfree bounded by filesystem
	if total == 0 {
		return 0.0
	}

	used := total - free
	diskPercent := float64(used) / float64(total) * 100.0
	return float64(int(diskPercent*100+0.5)) / 100
}

func GetOSDetails() models.HeartbeatOSDetails {
	return models.HeartbeatOSDetails{
		Kernel:  getKernelVersion(),
		Distro:  getDistroName(),
		Version: getDistroVersion(),
	}
}

func getKernelVersion() string {
	ver, err := unix.Sysctl("kern.osrelease")
	if err != nil {
		return string(constants.SystemHealthUnknown)
	}
	return ver
}

func getDistroName() string {
	return "macOS"
}

func getDistroVersion() string {
	ver, err := unix.Sysctl("kern.osproductversion")
	if err != nil || ver == "" {
		return string(constants.SystemHealthUnknown)
	}
	return ver
}

func readOSReleaseField(field string) string {
	return string(constants.SystemHealthUnknown)
}

func GetMemoryDetails() models.HeartbeatMemoryDetails {
	totalBytes, err := unix.SysctlUint64("hw.memsize")
	if err != nil || totalBytes == 0 {
		return models.HeartbeatMemoryDetails{}
	}

	totalMB := int64(totalBytes / (1024 * 1024))

	return models.HeartbeatMemoryDetails{
		TotalMB:     totalMB,
		AvailableMB: 0, // Requires host_statistics64 Mach trap
		UsedMB:      0,
		Percent:     0.0,
	}
}

func getInitProcessName() string {
	return "launchd"
}

func detectInitSystem() string {
	return "launchd"
}

func GetMemoryMB() int {
	totalBytes, err := unix.SysctlUint64("hw.memsize")
	if err != nil || totalBytes == 0 {
		return 0
	}
	return int(totalBytes / (1024 * 1024))
}

func getTimezone(tz string) string {
	if tz != "" {
		return tz
	}

	// Try /etc/localtime symlink (same pattern as Linux)
	link, err := os.Readlink(constants.PathEtcLocaltime)
	if err == nil {
		parts := strings.Split(link, "/zoneinfo/")
		if len(parts) == 2 {
			return parts[1]
		}
	}

	// Fall back to Go's time package
	name, _ := time.Now().Zone()
	if name != "" {
		return name
	}
	return "UTC"
}
