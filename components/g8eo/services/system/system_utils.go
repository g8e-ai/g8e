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
	"bytes"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/models"
)

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

func GetUptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "unknown"
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return "unknown"
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "unknown"
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
	data, err := os.ReadFile("/proc/uptime")
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
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 1 {
		return nil, fmt.Errorf("invalid /proc/stat format")
	}

	fields := strings.Fields(lines[0])
	if len(fields) < 8 || fields[0] != "cpu" {
		return nil, fmt.Errorf("invalid CPU line format")
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
	data, err := os.ReadFile("/proc/meminfo")
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
	data, err := os.ReadFile("/proc/mounts")
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
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0.0
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
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
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
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
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, field+"=") {
			value := strings.TrimPrefix(line, field+"=")
			return strings.Trim(value, "\"")
		}
	}
	return "unknown"
}

func GetUserDetails(shell string) models.HeartbeatUserDetails {
	if shell == "" {
		shell = "/bin/sh"
	}
	currentUser, err := user.Current()
	if err != nil {
		return models.HeartbeatUserDetails{
			Username: "unknown",
			Shell:    shell,
		}
	}
	// os/user returns UID/GID as decimal strings; the wire format carries them as
	// POSIX ints. Fall back to 0 on a malformed string (never expected on real systems).
	uid, _ := strconv.Atoi(currentUser.Uid)
	gid, _ := strconv.Atoi(currentUser.Gid)
	return models.HeartbeatUserDetails{
		Username: currentUser.Username,
		UID:      uid,
		GID:      gid,
		Home:     currentUser.HomeDir,
		Name:     currentUser.Name,
		Shell:    shell,
	}
}

func GetDiskDetails() models.HeartbeatDiskDetails {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return models.HeartbeatDiskDetails{}
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	totalGB := float64(total) / (1024 * 1024 * 1024)
	usedGB := float64(used) / (1024 * 1024 * 1024)
	freeGB := float64(free) / (1024 * 1024 * 1024)
	percent := 0.0
	if total > 0 {
		percent = float64(used) / float64(total) * 100.0
	}

	return models.HeartbeatDiskDetails{
		TotalGB: math.Round(totalGB*10) / 10,
		UsedGB:  math.Round(usedGB*10) / 10,
		FreeGB:  math.Round(freeGB*10) / 10,
		Percent: math.Round(percent*10) / 10,
	}
}

func GetDiskUsedGB() float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free
	return math.Round(float64(used)/(1024*1024*1024)*10) / 10
}

func GetDiskTotalGB() float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0
	}
	total := stat.Blocks * uint64(stat.Bsize)
	return math.Round(float64(total)/(1024*1024*1024)*10) / 10
}

func GetMemoryDetails() models.HeartbeatMemoryDetails {
	data, err := os.ReadFile("/proc/meminfo")
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

func GetEnvironmentDetails(lang, term, tz string) models.HeartbeatEnvironment {
	pwd, _ := os.Getwd()

	containerInfo := detectContainerEnvironment()
	initSystem := detectInitSystem()

	return models.HeartbeatEnvironment{
		PWD:              pwd,
		Lang:             lang,
		Timezone:         getTimezone(tz),
		Term:             term,
		IsContainer:      containerInfo.IsContainer,
		ContainerRuntime: containerInfo.Runtime,
		ContainerSignals: containerInfo.Signals,
		InitSystem:       initSystem,
	}
}

type ContainerInfo struct {
	IsContainer bool     `json:"is_container"`
	Runtime     string   `json:"container_runtime"`
	Signals     []string `json:"container_signals"`
}

func detectContainerEnvironment() ContainerInfo {
	info := ContainerInfo{
		Runtime: "none",
	}

	if _, err := os.Stat("/.dockerenv"); err == nil {
		info.IsContainer = true
		info.Runtime = "docker"
		info.Signals = append(info.Signals, "dockerenv_file")
	}

	if _, err := os.Stat("/run/.containerenv"); err == nil {
		info.IsContainer = true
		if info.Runtime == "none" {
			info.Runtime = "podman"
		}
		info.Signals = append(info.Signals, "containerenv_file")
	}

	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		cgroupContent := strings.ToLower(string(data))
		cgroupRuntimes := map[string]string{
			"docker":     "docker",
			"kubepods":   "kubernetes",
			"containerd": "containerd",
			"lxc":        "lxc",
		}
		for marker, runtime := range cgroupRuntimes {
			if strings.Contains(cgroupContent, marker) {
				info.IsContainer = true
				info.Signals = append(info.Signals, "cgroup_"+marker)
				if info.Runtime == "none" {
					info.Runtime = runtime
				}
			}
		}
	}

	if data, err := os.ReadFile("/proc/1/mountinfo"); err == nil {
		mountContent := strings.ToLower(string(data))
		if strings.Contains(mountContent, "overlay") || strings.Contains(mountContent, "aufs") {
			info.Signals = append(info.Signals, "overlay_filesystem")
			if !info.IsContainer {
				info.IsContainer = true
				if info.Runtime == "none" {
					info.Runtime = "unknown"
				}
			}
		}
	}

	initName := getInitProcessName()
	standardInits := map[string]bool{
		"systemd": true, "init": true, "launchd": true, "upstart": true,
	}
	if initName != "" && !standardInits[initName] {
		info.Signals = append(info.Signals, "non_standard_init_"+initName)
		if !info.IsContainer {
			info.IsContainer = true
			if info.Runtime == "none" {
				info.Runtime = "unknown"
			}
		}
	}

	if info.Signals == nil {
		info.Signals = []string{}
	}

	return info
}

func getInitProcessName() string {
	data, err := os.ReadFile("/proc/1/cmdline")
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
		return "unknown"
	}
	return initName
}

func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
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
	data, err := os.ReadFile("/proc/meminfo")
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

func GetCurrentUser() string {
	currentUser, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return currentUser.Username
}

func GetPublicIP(ipService string) string {
	if ipService == "" {
		ipService = "https://api.ipify.org?format=text"
	}
	resp, err := http.Get(ipService)
	if err != nil {
		return GetLocalIP("")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return GetLocalIP("")
	}
	return strings.TrimSpace(string(body))
}

func GetLocalIP(ipResolver string) string {
	if ipResolver == "" {
		ipResolver = "8.8.8.8:80"
	}
	conn, err := net.Dial("udp", ipResolver)
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func GetNetworkInterfaces() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{}
	}
	var interfaceNames []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 {
			interfaceNames = append(interfaceNames, iface.Name)
		}
	}
	return interfaceNames
}

func getTimezone(tz string) string {
	if tz != "" {
		return tz
	}
	data, err := os.ReadFile("/etc/timezone")
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	link, err := os.Readlink("/etc/localtime")
	if err == nil {
		parts := strings.Split(link, "/zoneinfo/")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return "UTC"
}
