// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !windows
// +build !windows

package auth

import (
	"fmt"
	"log/slog"
)

// getWindowsMachineID is a stub for non-Windows platforms
// This should never be called on non-Windows platforms
func getWindowsMachineID(logger *slog.Logger) (string, error) {
	return "", fmt.Errorf("windows machine ID not available on this platform")
}
