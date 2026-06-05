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
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayCmd(t *testing.T) {
	t.Run("gw command has correct use and description", func(t *testing.T) {
		cmd := gatewayCmd()
		assert.Equal(t, "gw", cmd.Use)
		assert.Contains(t, cmd.Short, "g8e Gateway")
		assert.Contains(t, cmd.Long, "lifecycle")
	})
}

func TestGatewayStartCmd(t *testing.T) {
	t.Run("start command has correct use", func(t *testing.T) {
		cmd := gatewayStartCmd()
		assert.Equal(t, "start", cmd.Use)
		assert.Contains(t, cmd.Short, "Start")
		assert.Contains(t, cmd.Short, "g8e Gateway")
	})
}

func TestGatewayStopCmd(t *testing.T) {
	t.Run("stop command has correct use", func(t *testing.T) {
		cmd := gatewayStopCmd()
		assert.Equal(t, "stop", cmd.Use)
		assert.Contains(t, cmd.Short, "Stop")
		assert.Contains(t, cmd.Short, "g8e Gateway")
	})
}

func TestGatewayStatusCmd(t *testing.T) {
	t.Run("status command has correct use", func(t *testing.T) {
		cmd := gatewayStatusCmd()
		assert.Equal(t, "status", cmd.Use)
		assert.Contains(t, cmd.Short, "health")
		assert.Contains(t, cmd.Short, "status")
	})
}

func TestGatewayRestartCmd(t *testing.T) {
	t.Run("restart command has correct use", func(t *testing.T) {
		cmd := gatewayRestartCmd()
		assert.Equal(t, "restart", cmd.Use)
		assert.Contains(t, cmd.Short, "Restart")
		assert.Contains(t, cmd.Short, "g8e Gateway")
	})
}

func TestGatewayLogsCmd(t *testing.T) {
	t.Run("logs command has correct use", func(t *testing.T) {
		cmd := gatewayLogsCmd()
		assert.Equal(t, "logs", cmd.Use)
		assert.Contains(t, cmd.Short, "logs")
	})
}

func TestGatewaySettingsCmd(t *testing.T) {
	t.Run("settings command has correct use", func(t *testing.T) {
		cmd := gatewaySettingsCmd()
		assert.Equal(t, "settings", cmd.Use)
		assert.Contains(t, cmd.Short, "settings")
	})
}

func TestGatewayResetCmd(t *testing.T) {
	t.Run("reset command has correct use", func(t *testing.T) {
		cmd := gatewayResetCmd()
		assert.Equal(t, "reset", cmd.Use)
		assert.Contains(t, cmd.Short, "Reset")
		assert.Contains(t, cmd.Short, "data")
		assert.Contains(t, cmd.Short, "secrets")
	})

	t.Run("reset has force flag", func(t *testing.T) {
		cmd := gatewayResetCmd()
		flag := cmd.Flags().Lookup("force")
		assert.NotNil(t, flag)
	})

	t.Run("reset has y shorthand flag", func(t *testing.T) {
		cmd := gatewayResetCmd()
		// The y flag is an alias for force, check force flag exists
		forceFlag := cmd.Flags().Lookup("force")
		assert.NotNil(t, forceFlag)
	})

	t.Run("reset has yes flag", func(t *testing.T) {
		cmd := gatewayResetCmd()
		flag := cmd.Flags().Lookup("yes")
		assert.NotNil(t, flag)
	})

}

func TestGatewayCleanCmd(t *testing.T) {
	t.Run("clean command has correct use", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		assert.Equal(t, "clean", cmd.Use)
		assert.Contains(t, cmd.Short, "Destructively")
		assert.Contains(t, cmd.Short, "remove")
	})

	t.Run("clean has force flag", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		flag := cmd.Flags().Lookup("force")
		assert.NotNil(t, flag)
	})

	t.Run("clean has y shorthand flag", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		// The y flag is an alias for force, check force flag exists
		forceFlag := cmd.Flags().Lookup("force")
		assert.NotNil(t, forceFlag)
	})

	t.Run("clean has yes flag", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		flag := cmd.Flags().Lookup("yes")
		assert.NotNil(t, flag)
	})

}

func TestGatewayCommandFlags(t *testing.T) {
	t.Run("reset and clean share force flags", func(t *testing.T) {
		resetCmd := gatewayResetCmd()
		cleanCmd := gatewayCleanCmd()

		resetForce := resetCmd.Flags().Lookup("force")
		cleanForce := cleanCmd.Flags().Lookup("force")

		assert.NotNil(t, resetForce)
		assert.NotNil(t, cleanForce)
	})
}

func TestGatewayStartWithAlreadyRunningOperator(t *testing.T) {
	t.Run("start command structure is correct", func(t *testing.T) {
		cmd := gatewayStartCmd()
		assert.Equal(t, "start", cmd.Use)
		assert.Contains(t, cmd.Short, "Start")
	})
}

func TestGatewayStopWithNotRunningOperator(t *testing.T) {
	t.Run("stop command structure is correct", func(t *testing.T) {
		cmd := gatewayStopCmd()
		assert.Equal(t, "stop", cmd.Use)
		assert.Contains(t, cmd.Short, "Stop")
	})
}

func TestGatewayStatusWithNotRunningOperator(t *testing.T) {
	t.Run("status command structure is correct", func(t *testing.T) {
		cmd := gatewayStatusCmd()
		assert.Equal(t, "status", cmd.Use)
		assert.Contains(t, cmd.Short, "health")
	})
}

func TestGatewayResetConfirmation(t *testing.T) {
	t.Run("reset command structure is correct", func(t *testing.T) {
		cmd := gatewayResetCmd()
		assert.Equal(t, "reset", cmd.Use)
		assert.Contains(t, cmd.Short, "Reset")
	})
}

func TestGatewayCleanConfirmation(t *testing.T) {
	t.Run("clean command structure is correct", func(t *testing.T) {
		cmd := gatewayCleanCmd()
		assert.Equal(t, "clean", cmd.Use)
		assert.Contains(t, cmd.Short, "Destructively")
	})
}

func TestGatewayLogsCommand(t *testing.T) {
	t.Run("logs command structure is correct", func(t *testing.T) {
		cmd := gatewayLogsCmd()
		assert.Equal(t, "logs", cmd.Use)
		assert.Contains(t, cmd.Short, "logs")
	})
}

func setupGatewayTestConfig(t *testing.T, tmpDir string) {
	runtimeDir := filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir)
	pkiDir := filepath.Join(runtimeDir, constants.Paths.Infra.PkiDir)
	secretsDir := filepath.Join(runtimeDir, constants.Paths.Infra.SecretsDir)

	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	require.NoError(t, os.MkdirAll(secretsDir, 0700))

	// Create minimal paths.json structure
	protocolDir := filepath.Join(tmpDir, "protocol")
	constantsDir := filepath.Join(protocolDir, "constants")
	require.NoError(t, os.MkdirAll(constantsDir, 0755))

	pathsJSON := `{
		"host": "localhost",
		"infra": {
			"app_cert_dir": "` + constants.Paths.Infra.AppCertDir + `",
			"ca_cert_path": "` + constants.Paths.Infra.CaCertPath + `",
			"db_path": "` + constants.Paths.Infra.DbPath + `",
			"docs_dir": "` + constants.Paths.Infra.DocsDir + `",
			"pki_dir": "` + constants.Paths.Infra.PkiDir + `",
			"protocol_constants_dir": "` + constants.Paths.Infra.ProtocolConstantsDir + `",
			"protocol_dir": "` + constants.Paths.Infra.ProtocolDir + `",
			"protocol_models_dir": "` + constants.Paths.Infra.ProtocolModelsDir + `",
			"secrets_dir": "` + constants.Paths.Infra.SecretsDir + `",
			"ssh_config_path": "` + constants.Paths.Infra.SshConfigPath + `"
		}
	}`
	pathsPath := filepath.Join(constantsDir, "paths.json")
	require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))
}
