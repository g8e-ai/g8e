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

//go:build windows
// +build windows

package auth

import (
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
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
