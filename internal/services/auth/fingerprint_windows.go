// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows
// +build windows

package auth

import (
	"fmt"
	"log/slog"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/g8e-ai/g8e/internal/constants"
)

// getWindowsMachineID retrieves the MachineGuid from the Windows registry
// The MachineGuid is a unique identifier for each Windows installation
func getWindowsMachineID(logger *slog.Logger) (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, constants.PathWindowsRegistryCryptography, registry.READ)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrWindowsRegistryOpenKey, err)
	}
	defer key.Close()

	machineGuid, _, err := key.GetStringValue(constants.PathWindowsRegistryMachineGuid)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrWindowsRegistryReadValue, err)
	}

	// Clean up the GUID (remove braces and dashes for consistency)
	machineID := strings.ReplaceAll(machineGuid, "{", "")
	machineID = strings.ReplaceAll(machineID, "}", "")
	machineID = strings.ReplaceAll(machineID, "-", "")

	logger.Info("Retrieved Windows machine ID", "source", "registry")
	return machineID, nil
}
