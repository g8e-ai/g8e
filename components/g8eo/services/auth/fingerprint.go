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

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

// SystemFingerprint represents a unique, stable identifier for the system
type SystemFingerprint struct {
	Fingerprint  string `json:"fingerprint"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	CPUCount     int    `json:"cpu_count"`
	MachineID    string `json:"machine_id,omitempty"`
}

// GenerateSystemFingerprint creates a unique fingerprint based on immutable system properties
func GenerateSystemFingerprint(logger *slog.Logger) (*SystemFingerprint, error) {
	logger.Info("Generating system fingerprint based on immutable system properties...")

	osType := runtime.GOOS
	arch := runtime.GOARCH
	cpuCount := runtime.NumCPU()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	machineID, err := getMachineID(logger)
	if err != nil {
		logger.Warn("Failed to get machine ID, using fallback method", "error", err)
		machineID = "fallback"
	}

	components := []string{
		fmt.Sprintf("os:%s", osType),
		fmt.Sprintf("arch:%s", arch),
		fmt.Sprintf("cpu_count:%d", cpuCount),
		fmt.Sprintf("machine_id:%s", machineID),
		fmt.Sprintf("hostname:%s", hostname),
	}

	hasher := sha256.New()
	fingerprintInput := strings.Join(components, "|")
	hasher.Write([]byte(fingerprintInput))
	fingerprintHash := hex.EncodeToString(hasher.Sum(nil))

	fingerprint := &SystemFingerprint{
		Fingerprint:  fingerprintHash,
		OS:           osType,
		Architecture: arch,
		CPUCount:     cpuCount,
		MachineID:    machineID,
	}

	logger.Info("System fingerprint generated successfully",
		"os", fingerprint.OS,
		"architecture", fingerprint.Architecture,
		"cpu_count", fingerprint.CPUCount,
		"machine_id", machineID,
		"hostname", hostname,
		"fingerprint", fingerprintHash[:16])

	return fingerprint, nil
}

// getMachineID retrieves a stable machine identifier based on the OS
func getMachineID(logger *slog.Logger) (string, error) {
	switch runtime.GOOS {
	case constants.Status.Platform.Linux:
		return getLinuxMachineID(logger)
	case constants.Status.Platform.Darwin:
		return getDarwinMachineID()
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// getLinuxMachineID reads a stable machine identifier from the kernel.
// For containers, uses container-specific identifiers (cgroup container ID or hostname).
// For bare metal/VMs, tries persistent identity files first (/etc/machine-id, /var/lib/dbus/machine-id),
// then falls back to /proc/sys/kernel/random/boot_id.
func getLinuxMachineID(logger *slog.Logger) (string, error) {
	// Check if running in a container and use container-specific ID
	if containerID, err := getContainerMachineID(logger); err == nil && containerID != "" {
		logger.Info("Retrieved container machine ID", "source", "container")
		return containerID, nil
	}

	// Fallback to bare metal/VM logic
	paths := []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
		"/proc/sys/kernel/random/boot_id",
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			machineID := strings.TrimSpace(string(data))
			if machineID != "" {
				logger.Info("Retrieved Linux machine ID", "source", path)
				return machineID, nil
			}
		}
	}

	return "", fmt.Errorf("could not read machine ID from any known path")
}

// getContainerMachineID attempts to get a container-specific identifier.
// Returns empty string if not running in a container or if detection fails.
func getContainerMachineID(logger *slog.Logger) (string, error) {
	// Method 1: Check for Docker container ID from /proc/self/cgroup (cgroup v1)
	cgroupData, err := os.ReadFile("/proc/self/cgroup")
	if err == nil {
		cgroupLines := strings.Split(string(cgroupData), "\n")
		for _, line := range cgroupLines {
			if strings.Contains(line, "docker") || strings.Contains(line, "kubepods") {
				// Extract container ID from cgroup path (format: .../docker/<container_id>/...)
				parts := strings.Split(line, "/")
				for i, part := range parts {
					if part == "docker" && i+1 < len(parts) {
						containerID := parts[i+1]
						if len(containerID) >= 12 { // Docker container IDs are at least 12 chars
							return containerID[:12], nil
						}
					}
					// Also handle kubernetes pod format
					if strings.HasPrefix(part, "cri-") || strings.HasPrefix(part, "docker-") {
						containerID := strings.TrimPrefix(part, "docker-")
						if len(containerID) >= 12 {
							return containerID[:12], nil
						}
					}
				}
			}
		}
	}

	// Method 2: Try /proc/self/mountinfo for cgroup v2 (works with Docker and containerd)
	mountInfoData, err := os.ReadFile("/proc/self/mountinfo")
	if err == nil {
		mountInfoLines := strings.Split(string(mountInfoData), "\n")
		for _, line := range mountInfoLines {
			// Look for container ID in mountinfo (format includes container_id)
			if strings.Contains(line, "container_id") {
				parts := strings.Fields(line)
				for _, part := range parts {
					if strings.HasPrefix(part, "container_id=") {
						containerID := strings.TrimPrefix(part, "container_id=")
						if len(containerID) >= 12 {
							logger.Info("Retrieved container ID from mountinfo", "container_id", containerID[:12])
							return containerID[:12], nil
						}
					}
				}
			}
			// Alternative: look for docker container ID pattern in mount source
			if strings.Contains(line, "docker") {
				parts := strings.Fields(line)
				for _, part := range parts {
					// Docker container IDs are 64-character hex strings
					if len(part) == 64 && isHex(part) {
						logger.Info("Retrieved container ID from mountinfo", "container_id", part[:12])
						return part[:12], nil
					}
				}
			}
		}
	}

	// Method 3: Fallback to /etc/hostname which is unique per container
	hostname, err := os.ReadFile("/etc/hostname")
	if err == nil {
		hn := strings.TrimSpace(string(hostname))
		if hn != "" && hn != "localhost" {
			logger.Info("Using container hostname as machine ID", "hostname", hn)
			return hn, nil
		}
	}

	return "", nil
}

// isHex checks if a string is a valid hexadecimal string
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// getDarwinMachineID uses the system preferences plist as a stable machine identifier on macOS
func getDarwinMachineID() (string, error) {
	data, err := os.ReadFile("/Library/Preferences/SystemConfiguration/preferences.plist")
	if err != nil {
		hostname, _ := os.Hostname()
		return fmt.Sprintf("darwin-%s", hostname), nil
	}

	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))[:32], nil
}
