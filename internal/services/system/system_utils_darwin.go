// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build darwin
// +build darwin

package system

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
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
	details := GetMemoryDetails()
	if details.TotalMB == 0 {
		return 0.0
	}
	return details.Percent
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

	freePages, inactivePages, err := parseVMStat()
	if err != nil {
		return models.HeartbeatMemoryDetails{
			TotalMB: totalMB,
		}
	}

	pageSize := int64(unix.Getpagesize())
	availableBytes := (freePages + inactivePages) * pageSize
	availableMB := availableBytes / (1024 * 1024)
	usedMB := totalMB - availableMB
	if usedMB < 0 {
		usedMB = 0
	}

	percent := 0.0
	if totalBytes > 0 {
		percent = float64(totalBytes-uint64(availableBytes)) / float64(totalBytes) * 100.0
	}

	return models.HeartbeatMemoryDetails{
		TotalMB:     totalMB,
		AvailableMB: availableMB,
		UsedMB:      usedMB,
		Percent:     math.Round(percent*10) / 10,
	}
}

// parseVMStat runs vm_stat and extracts free and inactive page counts.
func parseVMStat() (freePages, inactivePages int64, err error) {
	output, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0, 0, err
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Pages free:"):
			freePages = parseVMStatPages(line)
		case strings.HasPrefix(line, "Pages inactive:"):
			inactivePages = parseVMStatPages(line)
		}
	}

	if freePages == 0 && inactivePages == 0 {
		return 0, 0, fmt.Errorf("could not parse vm_stat output")
	}
	return freePages, inactivePages, nil
}

// parseVMStatPages extracts the page count from a vm_stat line like
// "Pages free:                             12345."
func parseVMStatPages(line string) int64 {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return 0
	}
	value := strings.TrimSpace(strings.TrimSuffix(line[colon+1:], "."))
	value = strings.ReplaceAll(value, ",", "")
	n, _ := strconv.ParseInt(value, 10, 64)
	return n
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
