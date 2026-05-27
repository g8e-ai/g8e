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

	"github.com/g8e-ai/g8e/internal/cli/config"
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

	t.Run("users fails with invalid project root", func(t *testing.T) {
		cmd := dataUsersCmd()
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

func TestDataOperatorsCmd(t *testing.T) {
	t.Run("operators command has correct use", func(t *testing.T) {
		cmd := dataOperatorsCmd()
		assert.Equal(t, "operators", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage operator instances")
	})

	t.Run("operators fails with invalid project root", func(t *testing.T) {
		cmd := dataOperatorsCmd()
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

func TestDataDeviceLinksCmd(t *testing.T) {
	t.Run("device-links command has correct use", func(t *testing.T) {
		cmd := dataDeviceLinksCmd()
		assert.Equal(t, "device-links", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage device-link tokens")
	})

	t.Run("device-links has expected subcommands", func(t *testing.T) {
		cmd := dataDeviceLinksCmd()
		expectedSubcommands := []string{"list", "create", "delete"}

		for _, subcmd := range expectedSubcommands {
			found := false
			for _, c := range cmd.Commands() {
				if c.Name() == subcmd {
					found = true
					break
				}
			}
			assert.True(t, found, "device-links should have %s subcommand", subcmd)
		}
	})
}

func TestDataDeviceLinksListCmd(t *testing.T) {
	t.Run("list command has correct use", func(t *testing.T) {
		cmd := dataDeviceLinksListCmd()
		assert.Equal(t, "list", cmd.Use)
		assert.Contains(t, cmd.Short, "List device-link tokens")
	})

	t.Run("list has user-id flag", func(t *testing.T) {
		cmd := dataDeviceLinksListCmd()
		flag := cmd.Flags().Lookup("user-id")
		assert.NotNil(t, flag)
	})

	t.Run("list fails with invalid project root", func(t *testing.T) {
		cmd := dataDeviceLinksListCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse paths.json")
	})

	t.Run("list fails without user-id", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		// Clean up any existing credentials file in the real ~/.g8e directory
		// to avoid test pollution from previous runs
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		realCredsFile := filepath.Join(homeDir, ".g8e", "credentials")
		os.Remove(realCredsFile)

		cmd := dataDeviceLinksListCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err = cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})
}

func TestDataDeviceLinksCreateCmd(t *testing.T) {
	t.Run("create command has correct use", func(t *testing.T) {
		cmd := dataDeviceLinksCreateCmd()
		assert.Equal(t, "create", cmd.Use)
		assert.Contains(t, cmd.Short, "Create a device-link token")
	})

	t.Run("create has user-id flag", func(t *testing.T) {
		cmd := dataDeviceLinksCreateCmd()
		flag := cmd.Flags().Lookup("user-id")
		assert.NotNil(t, flag)
	})

	t.Run("create has count flag", func(t *testing.T) {
		cmd := dataDeviceLinksCreateCmd()
		flag := cmd.Flags().Lookup("count")
		assert.NotNil(t, flag)
	})

	t.Run("create has ttl flag", func(t *testing.T) {
		cmd := dataDeviceLinksCreateCmd()
		flag := cmd.Flags().Lookup("ttl")
		assert.NotNil(t, flag)
	})

	t.Run("create fails with invalid project root", func(t *testing.T) {
		cmd := dataDeviceLinksCreateCmd()
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

	t.Run("create fails without user-id", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		cmd := dataDeviceLinksCreateCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
	})
}

func TestDataDeviceLinksDeleteCmd(t *testing.T) {
	t.Run("delete command has correct use", func(t *testing.T) {
		cmd := dataDeviceLinksDeleteCmd()
		assert.Equal(t, "delete", cmd.Use)
		assert.Contains(t, cmd.Short, "Delete a device-link token")
	})

	t.Run("delete has token flag", func(t *testing.T) {
		cmd := dataDeviceLinksDeleteCmd()
		flag := cmd.Flags().Lookup("token")
		assert.NotNil(t, flag)
	})

	t.Run("delete has user-id flag", func(t *testing.T) {
		cmd := dataDeviceLinksDeleteCmd()
		flag := cmd.Flags().Lookup("user-id")
		assert.NotNil(t, flag)
	})

	t.Run("delete fails with invalid project root", func(t *testing.T) {
		cmd := dataDeviceLinksDeleteCmd()
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

	t.Run("delete fails without token", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		cmd := dataDeviceLinksDeleteCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "--token is required")
	})

	t.Run("delete fails without user-id when env not set", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		// Clean up any existing credentials file in the real ~/.g8e directory
		// to avoid test pollution from previous runs
		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)
		realCredsFile := filepath.Join(homeDir, ".g8e", "credentials")
		os.Remove(realCredsFile)

		cmd := dataDeviceLinksDeleteCmd()
		cmd.Flags().Set("token", "test-token")
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err = cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})
}

func TestDataSettingsCmd(t *testing.T) {
	t.Run("settings command has correct use", func(t *testing.T) {
		cmd := dataSettingsCmd()
		assert.Equal(t, "settings", cmd.Use)
		assert.Contains(t, cmd.Short, "Manage Gateway settings")
	})

	t.Run("settings fails with invalid project root", func(t *testing.T) {
		cmd := dataSettingsCmd()
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

	t.Run("store fails with invalid project root", func(t *testing.T) {
		cmd := dataStoreCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})

	t.Run("store fails without collection", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		cmd := dataStoreCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
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

	t.Run("audit fails with invalid project root", func(t *testing.T) {
		cmd := dataAuditListCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})

	t.Run("audit fails without operator-session-id when env not set", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupDataTestConfig(t, tmpDir)

		cmd := dataAuditListCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not authenticated")
	})
}

func TestDataCommandFlags(t *testing.T) {
	t.Run("device-links create count flag has default", func(t *testing.T) {
		cmd := dataDeviceLinksCreateCmd()
		countFlag := cmd.Flags().Lookup("count")
		assert.NotNil(t, countFlag)
		assert.Equal(t, "1", countFlag.DefValue)
	})

	t.Run("device-links create ttl flag has default", func(t *testing.T) {
		cmd := dataDeviceLinksCreateCmd()
		ttlFlag := cmd.Flags().Lookup("ttl")
		assert.NotNil(t, ttlFlag)
		assert.Equal(t, "3600", ttlFlag.DefValue)
	})

	t.Run("audit limit flag has default", func(t *testing.T) {
		cmd := dataAuditListCmd()
		limitFlag := cmd.Flags().Lookup("limit")
		assert.NotNil(t, limitFlag)
		assert.Equal(t, "100", limitFlag.DefValue)
	})
}

func setupDataTestConfig(t *testing.T, tmpDir string) *config.Config {
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	pkiDir := filepath.Join(runtimeDir, "pki")
	secretsDir := filepath.Join(runtimeDir, "secrets")
	credentialsDir := filepath.Join(runtimeDir, "credentials")

	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	require.NoError(t, os.MkdirAll(credentialsDir, 0700))

	// Create minimal paths.json structure
	protocolDir := filepath.Join(tmpDir, "protocol")
	constantsDir := filepath.Join(protocolDir, "constants")
	require.NoError(t, os.MkdirAll(constantsDir, 0755))

	pathsJSON := `{
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
			"insecure_mcp_gateway": 9003,
			"operator_bootstrap_https": 9001,
			"operator_https": 9000,
			"operator_public_https": 9002
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
	}
}
