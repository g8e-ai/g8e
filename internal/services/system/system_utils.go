// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux
// +build linux

package system

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

func GetUptime() string {
	data, err := os.ReadFile(constants.PathProcUptime)
	if err != nil {
		return string(constants.SystemHealthUnknown)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return string(constants.SystemHealthUnknown)
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return string(constants.SystemHealthUnknown)
	}

	duration := time.Duration(uptime) * time.Second
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
	data, err := os.ReadFile(constants.PathProcUptime)
	if err != nil {
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}

	return int64(uptime)
}

func GetCPUPercent() float64 {
	stat1, err := readCPUStat()
	if err != nil {
		return 0.0
	}

	time.Sleep(100 * time.Millisecond)

	stat2, err := readCPUStat()
	if err != nil {
		return 0.0
	}

	total1 := stat1.user + stat1.nice + stat1.system + stat1.idle + stat1.iowait + stat1.irq + stat1.softirq
	total2 := stat2.user + stat2.nice + stat2.system + stat2.idle + stat2.iowait + stat2.irq + stat2.softirq

	totalDiff := total2 - total1
	idleDiff := stat2.idle - stat1.idle

	if totalDiff == 0 {
		return 0.0
	}

	cpuUsage := float64(totalDiff-idleDiff) / float64(totalDiff) * 100.0
	return math.Round(cpuUsage*100) / 100
}

type cpuStat struct {
	user, nice, system, idle, iowait, irq, softirq int64
}

func readCPUStat() (*cpuStat, error) {
	data, err := os.ReadFile(constants.PathProcStat)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 1 {
		return nil, constants.ErrSystemUtilsInvalidProcStatFormat
	}

	fields := strings.Fields(lines[0])
	if len(fields) < 8 || fields[0] != "cpu" {
		return nil, constants.ErrSystemUtilsInvalidCPULineFormat
	}

	stat := &cpuStat{}
	stat.user, _ = strconv.ParseInt(fields[1], 10, 64)
	stat.nice, _ = strconv.ParseInt(fields[2], 10, 64)
	stat.system, _ = strconv.ParseInt(fields[3], 10, 64)
	stat.idle, _ = strconv.ParseInt(fields[4], 10, 64)
	stat.iowait, _ = strconv.ParseInt(fields[5], 10, 64)
	stat.irq, _ = strconv.ParseInt(fields[6], 10, 64)
	stat.softirq, _ = strconv.ParseInt(fields[7], 10, 64)

	return stat, nil
}

func GetMemoryPercent() float64 {
	data, err := os.ReadFile(constants.PathProcMemInfo)
	if err != nil {
		return 0.0
	}

	lines := strings.Split(string(data), "\n")
	var memTotal, memAvailable int64

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			switch fields[0] {
			case "MemTotal:":
				memTotal, _ = strconv.ParseInt(fields[1], 10, 64)
			case "MemAvailable:":
				memAvailable, _ = strconv.ParseInt(fields[1], 10, 64)
			}
		}
	}

	if memTotal == 0 {
		return 0.0
	}

	memUsed := memTotal - memAvailable
	memoryPercent := float64(memUsed) / float64(memTotal) * 100.0
	return float64(int(memoryPercent*100+0.5)) / 100
}

func GetDiskPercent() float64 {
	data, err := os.ReadFile(constants.PathProcMounts)
	if err != nil {
		return 0.0
	}

	lines := strings.Split(string(data), "\n")
	var rootDevice string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "/" {
			rootDevice = fields[0]
			break
		}
	}

	if rootDevice == "" {
		return 0.0
	}

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
	data, err := os.ReadFile(constants.PathProcVersion)
	if err != nil {
		return string(constants.SystemHealthUnknown)
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		return fields[2]
	}
	return strings.TrimSpace(string(data))
}

func getDistroName() string {
	return readOSReleaseField("NAME")
}

func getDistroVersion() string {
	return readOSReleaseField("VERSION_ID")
}

func readOSReleaseField(field string) string {
	data, err := os.ReadFile(constants.PathEtcOSRelease)
	if err != nil {
		return string(constants.SystemHealthUnknown)
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, field+"=") {
			value := strings.TrimPrefix(line, field+"=")
			return strings.Trim(value, "\"")
		}
	}
	return string(constants.SystemHealthUnknown)
}

func GetMemoryDetails() models.HeartbeatMemoryDetails {
	data, err := os.ReadFile(constants.PathProcMemInfo)
	if err != nil {
		return models.HeartbeatMemoryDetails{}
	}

	lines := strings.Split(string(data), "\n")
	var memTotal, memAvailable, memFree, buffers, cached int64

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			switch fields[0] {
			case "MemTotal:":
				memTotal, _ = strconv.ParseInt(fields[1], 10, 64)
			case "MemAvailable:":
				memAvailable, _ = strconv.ParseInt(fields[1], 10, 64)
			case "MemFree:":
				memFree, _ = strconv.ParseInt(fields[1], 10, 64)
			case "Buffers:":
				buffers, _ = strconv.ParseInt(fields[1], 10, 64)
			case "Cached:":
				cached, _ = strconv.ParseInt(fields[1], 10, 64)
			}
		}
	}

	if memAvailable == 0 {
		memAvailable = memFree + buffers + cached
	}

	totalMB := memTotal / 1024
	availableMB := memAvailable / 1024
	usedMB := totalMB - availableMB
	percent := 0.0
	if memTotal > 0 {
		percent = float64(memTotal-memAvailable) / float64(memTotal) * 100.0
	}

	return models.HeartbeatMemoryDetails{
		TotalMB:     totalMB,
		AvailableMB: availableMB,
		UsedMB:      usedMB,
		Percent:     math.Round(percent*10) / 10,
	}
}

func getInitProcessName() string {
	data, err := os.ReadFile(constants.PathProcOneCmdline)
	if err != nil {
		return ""
	}
	parts := bytes.SplitN(data, []byte{0}, 2)
	if len(parts) == 0 || len(parts[0]) == 0 {
		return ""
	}
	return filepath.Base(string(parts[0]))
}

func detectInitSystem() string {
	initName := getInitProcessName()
	if initName == "" {
		return string(constants.SystemHealthUnknown)
	}
	return initName
}

func GetMemoryMB() int {
	data, err := os.ReadFile(constants.PathProcMemInfo)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			if memKB, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				return int(memKB / 1024)
			}
		}
	}
	return 0
}

func getTimezone(tz string) string {
	if tz != "" {
		return tz
	}
	data, err := os.ReadFile(constants.PathEtcTimezone)
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	link, err := os.Readlink(constants.PathEtcLocaltime)
	if err == nil {
		parts := strings.Split(link, "/zoneinfo/")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return "UTC"
}
