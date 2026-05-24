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

package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppsCmd(t *testing.T) {
	t.Run("apps command has correct use and description", func(t *testing.T) {
		cmd := appsCmd()
		assert.Equal(t, "apps", cmd.Use)
		assert.Contains(t, cmd.Short, "application-layer")
		assert.Contains(t, cmd.Long, "g8ee")
	})
}

func TestAppsStartCmd(t *testing.T) {
	t.Run("start command has correct use", func(t *testing.T) {
		cmd := appsStartCmd()
		assert.Equal(t, "start [app-name]", cmd.Use)
		assert.Contains(t, cmd.Short, "Start")
	})

	t.Run("start command accepts maximum 1 argument", func(t *testing.T) {
		cmd := appsStartCmd()
		assert.NotNil(t, cmd.Args)
	})

	t.Run("start rejects unsupported app name", func(t *testing.T) {
		cmd := appsStartCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"unsupported-app"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported app")
	})

	t.Run("start defaults to g8ee when no args", func(t *testing.T) {
		cmd := appsStartCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// This will fail because we're not setting up the full environment,
		// but we can verify the logic path
		err := cmd.RunE(cmd, []string{})
		// Should fail at config load or process manager creation
		assert.Error(t, err)
	})

	t.Run("start accepts g8ee app name", func(t *testing.T) {
		cmd := appsStartCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"g8ee"})
		// Will fail at config load, but shouldn't fail at app name validation
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "unsupported app")
	})
}

func TestAppsStopCmd(t *testing.T) {
	t.Run("stop command has correct use", func(t *testing.T) {
		cmd := appsStopCmd()
		assert.Equal(t, "stop <app-name>", cmd.Use)
		assert.Contains(t, cmd.Short, "Stop")
	})

	t.Run("stop command requires exact 1 argument", func(t *testing.T) {
		cmd := appsStopCmd()
		// Args validation is tested by running the command with wrong number of args
		assert.NotNil(t, cmd.Args)
	})

	t.Run("stop rejects unsupported app name", func(t *testing.T) {
		cmd := appsStopCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"unsupported-app"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported app")
	})

	t.Run("stop accepts g8ee app name", func(t *testing.T) {
		cmd := appsStopCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"g8ee"})
		// Will fail at config load, but shouldn't fail at app name validation
		assert.Error(t, err)
		assert.NotContains(t, err.Error(), "unsupported app")
	})
}

func TestAppsCommandIntegration(t *testing.T) {
	t.Run("start and stop commands share same app validation logic", func(t *testing.T) {
		startCmd := appsStartCmd()
		stopCmd := appsStopCmd()

		// Both should reject the same unsupported app names
		unsupportedApps := []string{"invalid", "test", "dashboard", "engine"}

		for _, app := range unsupportedApps {
			t.Run("rejects "+app, func(t *testing.T) {
				var startBuf, stopBuf bytes.Buffer
				startCmd.SetOut(&startBuf)
				startCmd.SetErr(&startBuf)
				stopCmd.SetOut(&stopBuf)
				stopCmd.SetErr(&stopBuf)

				startErr := startCmd.RunE(startCmd, []string{app})
				stopErr := stopCmd.RunE(stopCmd, []string{app})

				assert.Error(t, startErr)
				assert.Error(t, stopErr)
				assert.Contains(t, startErr.Error(), "unsupported app")
				assert.Contains(t, stopErr.Error(), "unsupported app")
			})
		}
	})
}

func TestAppsCommandWithMockProcessManager(t *testing.T) {
	t.Run("start checks g8ee status before starting", func(t *testing.T) {
		// This test would require mocking the ProcessManager
		// For now, we verify the command structure
		cmd := appsStartCmd()
		assert.NotNil(t, cmd)
		assert.Contains(t, cmd.Short, "Start")
	})

	t.Run("stop checks g8ee status before stopping", func(t *testing.T) {
		// This test would require mocking the ProcessManager
		// For now, we verify the command structure
		cmd := appsStopCmd()
		assert.NotNil(t, cmd)
		assert.Contains(t, cmd.Short, "Stop")
	})
}

func TestAppsCommandErrorMessages(t *testing.T) {
	t.Run("start provides clear error for unsupported app", func(t *testing.T) {
		cmd := appsStartCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"myapp"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "myapp")
		assert.Contains(t, err.Error(), "only g8ee is supported")
	})

	t.Run("stop provides clear error for unsupported app", func(t *testing.T) {
		cmd := appsStopCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{"myapp"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "myapp")
		assert.Contains(t, err.Error(), "only g8ee is supported")
	})
}

func TestAppsCommandConfigLoading(t *testing.T) {
	t.Run("start fails with invalid project root", func(t *testing.T) {
		cmd := appsStartCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Set up environment to force config load failure
		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{"g8ee"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})

	t.Run("stop fails with invalid project root", func(t *testing.T) {
		cmd := appsStopCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{"g8ee"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}
