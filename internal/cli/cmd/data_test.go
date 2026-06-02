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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/config"
	clierrors "github.com/g8e-ai/g8e/internal/cli/errors"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataCmd(t *testing.T) {
	t.Run("data command has correct use and description", func(t *testing.T) {
		cmd := dataCmd()
		assert.Equal(t, "data", cmd.Use)
		assert.Contains(t, cmd.Short, "Administer")
		assert.Contains(t, cmd.Short, "mTLS")
	})
}

func TestDataUsersCmd(t *testing.T) {
	t.Run("users command has correct use", func(t *testing.T) {
		cmd := dataUsersCmd()
		assert.Equal(t, "users", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage user accounts")
	})
}

func TestDataOperatorsCmd(t *testing.T) {
	t.Run("operators command has correct use", func(t *testing.T) {
		cmd := dataOperatorsCmd()
		assert.Equal(t, "operators", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage Operator instances")
	})
}

func TestDataSettingsCmd(t *testing.T) {
	t.Run("settings command has correct use", func(t *testing.T) {
		cmd := dataSettingsCmd()
		assert.Equal(t, "settings", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage Gateway settings")
	})
}

func TestDataStoreCmd(t *testing.T) {
	t.Run("store command has correct use", func(t *testing.T) {
		cmd := dataStoreCmd()
		assert.Equal(t, "store", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage document storage")
	})

	t.Run("store has collection flag", func(t *testing.T) {
		cmd := dataStoreCmd()
		flag := cmd.Flags().Lookup("collection")
		assert.NotNil(t, flag)
	})

	t.Run("store has document-id flag", func(t *testing.T) {
		cmd := dataStoreCmd()
		flag := cmd.Flags().Lookup("document-id")
		assert.NotNil(t, flag)
	})

}

func TestDataAuditCmd(t *testing.T) {
	t.Run("audit command has correct use", func(t *testing.T) {
		cmd := dataAuditCmd()
		assert.Equal(t, "audit", cmd.Use)
		assert.Contains(t, cmd.Short, "Query audit vault")
	})

	t.Run("audit has operator-session-id flag", func(t *testing.T) {
		cmd := dataAuditListCmd()
		flag := cmd.Flags().Lookup("operator-session-id")
		assert.NotNil(t, flag)
	})

	t.Run("audit has limit flag", func(t *testing.T) {
		cmd := dataAuditListCmd()
		flag := cmd.Flags().Lookup("limit")
		assert.NotNil(t, flag)
	})

}

func TestDataCommandsRequireAuthentication(t *testing.T) {
	testCases := []struct {
		name string
		cmd  func() *cobra.Command
	}{
		{"users", dataUsersCmd},
		{"operators", dataOperatorsCmd},
		{"settings", dataSettingsCmd},
		{"store", dataStoreCmd},
		{"audit list", dataAuditListCmd},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.cmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			originalWd, _ := os.Getwd()
			tmpDir := t.TempDir()
			os.Chdir(tmpDir)
			defer os.Chdir(originalWd)

			// Set up minimal config structure so config loads, then auth fails
			setupDataTestConfig(t, tmpDir)

			err := cmd.RunE(cmd, []string{})
			assert.Error(t, err)
			assert.True(t, errors.Is(err, clierrors.ErrNotAuthenticated))
		})
	}
}

func TestDataCommandFlags(t *testing.T) {
	t.Run("audit limit flag has default", func(t *testing.T) {
		cmd := dataAuditListCmd()
		limitFlag := cmd.Flags().Lookup("limit")
		assert.NotNil(t, limitFlag)
		assert.Equal(t, "100", limitFlag.DefValue)
	})
}

func setupDataTestConfig(t *testing.T, tmpDir string) *config.Config {
	runtimeDir := filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir)
	pkiDir := filepath.Join(runtimeDir, constants.Paths.Infra.PkiDir)
	secretsDir := filepath.Join(runtimeDir, constants.Paths.Infra.SecretsDir)
	credentialsDir := filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir)

	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	// Create credentials directory but NOT the credentials file itself
	// This ensures auth.LoadCredentials returns (nil, nil) which triggers ErrNotAuthenticated
	require.NoError(t, os.MkdirAll(credentialsDir, 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(pkiDir, "root"), 0755))

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
		},
		"ports": {
			"insecure_mcp_gateway": 18789,
			"operator_bootstrap_https": 8441,
			"operator_https": 8440,
			"operator_public_https": 8443
		}
	}`
	pathsPath := filepath.Join(constantsDir, "paths.json")
	require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))

	return &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     runtimeDir,
		PKIDir:         pkiDir,
		SecretsDir:     secretsDir,
		CredentialsDir: credentialsDir,
		Paths: &config.PathsConfig{
			Host: "localhost",
			Infra: struct {
				AppCertDir           string `json:"app_cert_dir"`
				CACertPath           string `json:"ca_cert_path"`
				DBPath               string `json:"db_path"`
				DocsDir              string `json:"docs_dir"`
				PKIDir               string `json:"pki_dir"`
				ProtocolConstantsDir string `json:"protocol_constants_dir"`
				ProtocolDir          string `json:"protocol_dir"`
				ProtocolModelsDir    string `json:"protocol_models_dir"`
				SecretsDir           string `json:"secrets_dir"`
				SSHConfigPath        string `json:"ssh_config_path"`
			}{
				AppCertDir:           filepath.Join(tmpDir, constants.Paths.Infra.AppCertDir),
				CACertPath:           filepath.Join(tmpDir, constants.Paths.Infra.CaCertPath),
				DBPath:               filepath.Join(tmpDir, constants.Paths.Infra.DbPath),
				DocsDir:              filepath.Join(tmpDir, constants.Paths.Infra.DocsDir),
				PKIDir:               filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
				ProtocolConstantsDir: filepath.Join(tmpDir, constants.Paths.Infra.ProtocolConstantsDir),
				ProtocolDir:          filepath.Join(tmpDir, constants.Paths.Infra.ProtocolDir),
				ProtocolModelsDir:    filepath.Join(tmpDir, constants.Paths.Infra.ProtocolModelsDir),
				SecretsDir:           filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
				SSHConfigPath:        filepath.Join(tmpDir, constants.Paths.Infra.SshConfigPath),
			},
			Ports: struct {
				InsecureMcpGateway     int `json:"insecure_mcp_gateway"`
				OperatorBootstrapHTTPS int `json:"operator_bootstrap_https"`
				OperatorHTTPS          int `json:"operator_https"`
				OperatorMcpHttp        int `json:"operator_mcp_http"`
				OperatorPublicHTTPS    int `json:"operator_public_https"`
			}{
				InsecureMcpGateway:     18789,
				OperatorBootstrapHTTPS: 8441,
				OperatorHTTPS:          8440,
				OperatorMcpHttp:        8442,
				OperatorPublicHTTPS:    8443,
			},
		},
	}
}
