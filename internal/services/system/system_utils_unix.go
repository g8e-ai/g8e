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

//go:build !windows
// +build !windows

package system

import (
	"math"
	"net"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
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

func GetNetworkLatency() float64 {
	start := time.Now().UTC()
	conn, err := net.DialTimeout(string(constants.NetworkProtocolTCP), "127.0.0.1:22", 1*time.Second)
	if err != nil {
		start = time.Now().UTC()
		conn, err = net.DialTimeout(string(constants.NetworkProtocolTCP), "127.0.0.1:80", 1*time.Second)
		if err != nil {
			return 1.0
		}
	}
	defer conn.Close()

	latency := time.Since(start).Seconds() * 1000
	return math.Round(latency*100) / 100
}

func GetUserDetails(shell string) models.HeartbeatUserDetails {
	if shell == "" {
		shell = constants.PathBinSh
	}
	currentUser, err := user.Current()
	if err != nil {
		return models.HeartbeatUserDetails{
			Username: string(constants.SystemHealthUnknown),
			Shell:    shell,
		}
	}
	// os/user returns UID/GID as decimal strings; the wire format carries them as
	// POSIX ints. Fall back to 0 on a malformed string (never expected on real systems).
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

func GetDiskDetails() models.HeartbeatDiskDetails {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(constants.PathRoot, &stat); err != nil {
		return models.HeartbeatDiskDetails{}
	}

	total := stat.Blocks * uint64(stat.Bsize) //nolint:gosec // stat.Blocks bounded by filesystem
	free := stat.Bfree * uint64(stat.Bsize)   //nolint:gosec // stat.Bfree bounded by filesystem
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
	if err := syscall.Statfs(constants.PathRoot, &stat); err != nil {
		return 0
	}
	total := stat.Blocks * uint64(stat.Bsize) //nolint:gosec // stat.Blocks bounded by filesystem
	free := stat.Bfree * uint64(stat.Bsize)   //nolint:gosec // stat.Bfree bounded by filesystem
	used := total - free
	return math.Round(float64(used)/(1024*1024*1024)*10) / 10
}

func GetDiskTotalGB() float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(constants.PathRoot, &stat); err != nil {
		return 0
	}
	total := stat.Blocks * uint64(stat.Bsize) //nolint:gosec // stat.Blocks bounded by filesystem
	return math.Round(float64(total)/(1024*1024*1024)*10) / 10
}

func GetEnvironmentDetails(lang, term, tz string) models.HeartbeatEnvironment {
	pwd, _ := os.Getwd()

	initSystem := detectInitSystem()

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

type ContainerInfo struct {
	IsContainer bool     `json:"is_container"`
	Runtime     string   `json:"container_runtime"`
	Signals     []string `json:"container_signals"`
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

func GetCurrentUser() string {
	currentUser, err := user.Current()
	if err != nil {
		return string(constants.SystemHealthUnknown)
	}
	return currentUser.Username
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
