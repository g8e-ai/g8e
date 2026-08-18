// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
		return nil, fmt.Errorf("%w: %w", constants.ErrFingerprintGetHostname, err)
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
	return getMachineIDWithPlatform(logger, runtime.GOOS)
}

// getMachineIDWithPlatform dispatches to the platform-specific machine ID
// implementation. Extracted from getMachineID for testability of the
// unsupported OS branch.
func getMachineIDWithPlatform(logger *slog.Logger, platform string) (string, error) {
	switch constants.Platform(platform) {
	case constants.PlatformLinux:
		return getLinuxMachineID(logger)
	case constants.PlatformDarwin:
		return getDarwinMachineID()
	case constants.PlatformWindows:
		return getWindowsMachineID(logger)
	default:
		return "", fmt.Errorf("%w: %s", constants.ErrFingerprintUnsupportedOS, platform)
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

	return "", constants.ErrFingerprintMachineIDRead
}

// getDarwinMachineID uses the system preferences plist as a stable machine identifier on macOS
func getDarwinMachineID() (string, error) {
	data, err := os.ReadFile(constants.PathLibraryPreferencesSystemConfigurationPreferencesPlist)
	if err != nil {
		hostname, err := os.Hostname()
		if err != nil {
			return "", fmt.Errorf("%w: %w", constants.ErrFingerprintGetHostname, err)
		}
		return fmt.Sprintf("darwin-%s", hostname), nil
	}

	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))[:32], nil
}
