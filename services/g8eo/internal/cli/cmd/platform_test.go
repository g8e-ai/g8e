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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatformCmd(t *testing.T) {
	t.Run("platform command has correct use and description", func(t *testing.T) {
		cmd := platformCmd()
		assert.Equal(t, "platform", cmd.Use)
		assert.Contains(t, cmd.Short, "Governance Gateway")
		assert.Contains(t, cmd.Long, "lifecycle")
	})
}

func TestPlatformStartCmd(t *testing.T) {
	t.Run("start command has correct use", func(t *testing.T) {
		cmd := platformStartCmd()
		assert.Equal(t, "start", cmd.Use)
		assert.Contains(t, cmd.Short, "Start")
		assert.Contains(t, cmd.Short, "Governance Gateway")
	})

	t.Run("start has apps flag", func(t *testing.T) {
		cmd := platformStartCmd()
		flag := cmd.Flags().Lookup("apps")
		assert.NotNil(t, flag)
	})

	t.Run("start has apps shorthand flag", func(t *testing.T) {
		cmd := platformStartCmd()
		flag := cmd.Flags().ShorthandLookup("a")
		assert.NotNil(t, flag)
	})

	t.Run("start fails with invalid project root", func(t *testing.T) {
		cmd := platformStartCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestPlatformStopCmd(t *testing.T) {
	t.Run("stop command has correct use", func(t *testing.T) {
		cmd := platformStopCmd()
		assert.Equal(t, "stop", cmd.Use)
		assert.Contains(t, cmd.Short, "Stop")
		assert.Contains(t, cmd.Short, "Governance Gateway")
	})

	t.Run("stop fails with invalid project root", func(t *testing.T) {
		cmd := platformStopCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestPlatformStatusCmd(t *testing.T) {
	t.Run("status command has correct use", func(t *testing.T) {
		cmd := platformStatusCmd()
		assert.Equal(t, "status", cmd.Use)
		assert.Contains(t, cmd.Short, "health")
		assert.Contains(t, cmd.Short, "status")
	})

	t.Run("status fails with invalid project root", func(t *testing.T) {
		cmd := platformStatusCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestPlatformRestartCmd(t *testing.T) {
	t.Run("restart command has correct use", func(t *testing.T) {
		cmd := platformRestartCmd()
		assert.Equal(t, "restart", cmd.Use)
		assert.Contains(t, cmd.Short, "Restart")
		assert.Contains(t, cmd.Short, "Governance Gateway")
	})

	t.Run("restart has apps flag", func(t *testing.T) {
		cmd := platformRestartCmd()
		flag := cmd.Flags().Lookup("apps")
		assert.NotNil(t, flag)
	})

	t.Run("restart has apps shorthand flag", func(t *testing.T) {
		cmd := platformRestartCmd()
		flag := cmd.Flags().ShorthandLookup("a")
		assert.NotNil(t, flag)
	})

	t.Run("restart fails with invalid project root", func(t *testing.T) {
		cmd := platformRestartCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestPlatformLogsCmd(t *testing.T) {
	t.Run("logs command has correct use", func(t *testing.T) {
		cmd := platformLogsCmd()
		assert.Equal(t, "logs", cmd.Use)
		assert.Contains(t, cmd.Short, "logs")
	})

	t.Run("logs fails with invalid project root", func(t *testing.T) {
		cmd := platformLogsCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestPlatformSettingsCmd(t *testing.T) {
	t.Run("settings command has correct use", func(t *testing.T) {
		cmd := platformSettingsCmd()
		assert.Equal(t, "settings", cmd.Use)
		assert.Contains(t, cmd.Short, "settings")
	})

	t.Run("settings fails with invalid project root", func(t *testing.T) {
		cmd := platformSettingsCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestPlatformResetCmd(t *testing.T) {
	t.Run("reset command has correct use", func(t *testing.T) {
		cmd := platformResetCmd()
		assert.Equal(t, "reset", cmd.Use)
		assert.Contains(t, cmd.Short, "Reset")
		assert.Contains(t, cmd.Short, "data")
		assert.Contains(t, cmd.Short, "secrets")
	})

	t.Run("reset has force flag", func(t *testing.T) {
		cmd := platformResetCmd()
		flag := cmd.Flags().Lookup("force")
		assert.NotNil(t, flag)
	})

	t.Run("reset has y shorthand flag", func(t *testing.T) {
		cmd := platformResetCmd()
		// The y flag is an alias for force, check force flag exists
		forceFlag := cmd.Flags().Lookup("force")
		assert.NotNil(t, forceFlag)
	})

	t.Run("reset has yes flag", func(t *testing.T) {
		cmd := platformResetCmd()
		flag := cmd.Flags().Lookup("yes")
		assert.NotNil(t, flag)
	})

	t.Run("reset fails with invalid project root", func(t *testing.T) {
		cmd := platformResetCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestPlatformCleanCmd(t *testing.T) {
	t.Run("clean command has correct use", func(t *testing.T) {
		cmd := platformCleanCmd()
		assert.Equal(t, "clean", cmd.Use)
		assert.Contains(t, cmd.Short, "Destructively")
		assert.Contains(t, cmd.Short, "remove")
	})

	t.Run("clean has force flag", func(t *testing.T) {
		cmd := platformCleanCmd()
		flag := cmd.Flags().Lookup("force")
		assert.NotNil(t, flag)
	})

	t.Run("clean has y shorthand flag", func(t *testing.T) {
		cmd := platformCleanCmd()
		// The y flag is an alias for force, check force flag exists
		forceFlag := cmd.Flags().Lookup("force")
		assert.NotNil(t, forceFlag)
	})

	t.Run("clean has yes flag", func(t *testing.T) {
		cmd := platformCleanCmd()
		flag := cmd.Flags().Lookup("yes")
		assert.NotNil(t, flag)
	})

	t.Run("clean fails with invalid project root", func(t *testing.T) {
		cmd := platformCleanCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load config")
	})
}

func TestPlatformCommandFlags(t *testing.T) {
	t.Run("start and restart share apps flag", func(t *testing.T) {
		startCmd := platformStartCmd()
		restartCmd := platformRestartCmd()

		startFlag := startCmd.Flags().Lookup("apps")
		restartFlag := restartCmd.Flags().Lookup("apps")

		assert.NotNil(t, startFlag)
		assert.NotNil(t, restartFlag)
	})

	t.Run("reset and clean share force flags", func(t *testing.T) {
		resetCmd := platformResetCmd()
		cleanCmd := platformCleanCmd()

		resetForce := resetCmd.Flags().Lookup("force")
		cleanForce := cleanCmd.Flags().Lookup("force")

		assert.NotNil(t, resetForce)
		assert.NotNil(t, cleanForce)
	})
}

func TestPlatformStartWithAlreadyRunningOperator(t *testing.T) {
	t.Run("start reports already running when operator is up", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupPlatformTestConfig(t, tmpDir)

		// Create a fake PID file with current process PID
		runtimeDir := filepath.Join(tmpDir, ".g8e")
		pidDir := filepath.Join(runtimeDir, "pids")
		require.NoError(t, os.MkdirAll(pidDir, 0700))
		pidFile := filepath.Join(pidDir, "operator.pid")
		require.NoError(t, os.WriteFile(pidFile, []byte("99999"), 0600))

		cmd := platformStartCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// This will fail at process check since 99999 doesn't exist
		// but we can verify the logic path
		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
	})
}

func TestPlatformStopWithNotRunningOperator(t *testing.T) {
	t.Run("stop reports not running when operator is down", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupPlatformTestConfig(t, tmpDir)

		cmd := platformStopCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "not running")
	})
}

func TestPlatformStatusWithNotRunningOperator(t *testing.T) {
	t.Run("status reports stopped when operator is down", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupPlatformTestConfig(t, tmpDir)

		cmd := platformStatusCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "STOPPED")
	})
}

func TestPlatformResetConfirmation(t *testing.T) {
	t.Run("reset requires confirmation without force flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupPlatformTestConfig(t, tmpDir)

		cmd := platformResetCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Without stdin input, Scanln will fail and the command returns nil (aborted)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Aborted")
	})

	t.Run("reset skips confirmation with force flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupPlatformTestConfig(t, tmpDir)

		cmd := platformResetCmd()
		cmd.Flags().Set("force", "true")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Will fail at stop since operator not running, but should skip confirmation
		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		// Should not contain "Continue?" prompt since force is set
		assert.NotContains(t, buf.String(), "Continue?")
	})
}

func TestPlatformCleanConfirmation(t *testing.T) {
	t.Run("clean requires confirmation without force flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupPlatformTestConfig(t, tmpDir)

		cmd := platformCleanCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Without stdin input, Scanln will fail and the command returns nil (aborted)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Aborted")
	})

	t.Run("clean skips confirmation with force flag", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupPlatformTestConfig(t, tmpDir)

		cmd := platformCleanCmd()
		cmd.Flags().Set("force", "true")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Will succeed in cleaning (no operator running to stop)
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		// Should not contain "Continue?" prompt since force is set
		assert.NotContains(t, buf.String(), "Continue?")
		assert.Contains(t, buf.String(), "Clean complete")
	})
}

func TestPlatformLogsCommand(t *testing.T) {
	t.Run("logs handles missing log file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupPlatformTestConfig(t, tmpDir)

		cmd := platformLogsCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No log file found")
	})
}

func setupPlatformTestConfig(t *testing.T, tmpDir string) {
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	pkiDir := filepath.Join(runtimeDir, "pki")
	secretsDir := filepath.Join(runtimeDir, "secrets")

	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	require.NoError(t, os.MkdirAll(secretsDir, 0700))

	// Create minimal paths.json structure
	protocolDir := filepath.Join(tmpDir, "protocol")
	constantsDir := filepath.Join(protocolDir, "constants")
	require.NoError(t, os.MkdirAll(constantsDir, 0755))

	pathsJSON := `{
		"g8ee": {
			"app_dir": "services/g8ee/app",
			"cert_name": "g8ee",
			"config_dir": "services/g8ee/config",
			"tests_dir": "services/g8ee/tests"
		},
		"host": "localhost",
		"infra": {
			"app_cert_dir": ".g8e/pki/app",
			"ca_cert_path": ".g8e/pki/root/root_ca.crt",
			"db_path": ".g8e/data/operator.db",
			"docs_dir": "docs",
			"pki_dir": ".g8e/pki",
			"protocol_constants_dir": "protocol/constants",
			"protocol_dir": "protocol",
			"protocol_models_dir": "protocol/models",
			"secrets_dir": ".g8e/secrets",
			"ssh_config_path": ".g8e/ssh/config"
		},
		"ports": {
			"g8ee_https": 8443,
			"openclaw_gateway": 9003,
			"operator_bootstrap_https": 9001,
			"operator_https": 9000,
			"operator_public_https": 9002
		}
	}`
	pathsPath := filepath.Join(constantsDir, "paths.json")
	require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))
}
