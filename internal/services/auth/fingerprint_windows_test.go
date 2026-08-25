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
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestGetWindowsMachineID(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	t.Run("reads MachineGuid from registry", func(t *testing.T) {
		t.Parallel()
		machineID, err := getWindowsMachineID(logger)
		require.NoError(t, err)
		require.NotEmpty(t, machineID)

		// MachineGuid should be a UUID without braces or dashes
		// After cleaning, it should be 32 hex characters
		require.Len(t, machineID, 32, "MachineID should be 32 hex characters after cleaning")
	})

	t.Run("machine ID does not contain braces or dashes", func(t *testing.T) {
		t.Parallel()
		machineID, err := getWindowsMachineID(logger)
		require.NoError(t, err)

		require.False(t, strings.Contains(machineID, "{"), "MachineID should not contain braces")
		require.False(t, strings.Contains(machineID, "}"), "MachineID should not contain braces")
		require.False(t, strings.Contains(machineID, "-"), "MachineID should not contain dashes")
	})
}
