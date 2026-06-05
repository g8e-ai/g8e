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

	"github.com/g8e-ai/g8e/internal/constants"
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
		hostname = string(constants.SystemHealthUnknown)
	}

	machineID, err := getMachineID(logger)
	if err != nil {
		logger.Warn("Failed to get machine ID, using fallback method", string(constants.ConnectionStateError), err)
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
	switch constants.Platform(runtime.GOOS) {
	case constants.PlatformLinux:
		return getLinuxMachineID(logger)
	case constants.PlatformDarwin:
		return getDarwinMachineID()
	case constants.PlatformWindows:
		return getWindowsMachineID(logger)
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// getLinuxMachineID reads a stable machine identifier from the kernel.
// For bare metal/VMs, tries persistent identity files first (/etc/machine-id, /var/lib/dbus/machine-id),
// then falls back to /proc/sys/kernel/random/boot_id.
func getLinuxMachineID(logger *slog.Logger) (string, error) {
	// Try persistent identity files
	paths := []string{
		constants.PathEtcMachineID,
		constants.PathVarLibDbusMachineID,
		constants.PathProcSysKernelRandomBootID,
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

// isHex checks if a string is a valid hexadecimal string
func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}

// getDarwinMachineID uses the system preferences plist as a stable machine identifier on macOS
func getDarwinMachineID() (string, error) {
	data, err := os.ReadFile(constants.PathLibraryPreferencesSystemConfigurationPreferencesPlist)
	if err != nil {
		hostname, _ := os.Hostname()
		return fmt.Sprintf("darwin-%s", hostname), nil
	}

	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))[:32], nil
}
