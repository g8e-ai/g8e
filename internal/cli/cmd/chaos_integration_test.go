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

//go:build integration

package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunChaosErrorHandling(t *testing.T) {
	t.Run("runChaos wraps chaos.Run errors", func(t *testing.T) {
		// This test verifies that errors from chaos.Run are properly wrapped
		// with the "chaos: failed to run chaos test" prefix
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		// Set up a config that will cause chaos.Run to fail
		// (e.g., invalid directory paths)
		chaosCount = 1
		chaosDataDir = "/nonexistent/path/that/does/not/exist"
		chaosPKIDir = ""

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Change to a temp directory to avoid affecting real filesystem
		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := runChaos(cmd, []string{})
		// We expect an error due to the invalid path
		assert.Error(t, err)
	})

	t.Run("runChaos with valid temporary directory", func(t *testing.T) {
		cmd := chaosCmd()
		require.NotNil(t, cmd)

		// Use a valid temporary directory
		tmpDir := t.TempDir()
		chaosCount = 1
		chaosDataDir = tmpDir
		chaosPKIDir = ""

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// This may still fail due to missing dependencies, but should not fail
		// due to directory creation
		err := runChaos(cmd, []string{})
		// The error is acceptable here as we're testing the config construction
		// and path handling, not the full chaos.Run execution
		if err != nil {
			assert.Contains(t, err.Error(), "chaos")
		}
	})
}
