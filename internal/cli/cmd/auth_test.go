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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthCmd(t *testing.T) {
	t.Run("auth command has correct use and description", func(t *testing.T) {
		cmd := authCmd()
		assert.Equal(t, "auth", cmd.Use)
		assert.Contains(t, cmd.Short, "Authentication")
		assert.Contains(t, cmd.Long, "mTLS")
	})
}

func TestLoginCmd(t *testing.T) {
	t.Run("login command has correct use", func(t *testing.T) {
		cmd := loginCmd()
		assert.Equal(t, "login", cmd.Use)
		assert.Contains(t, cmd.Short, "Authenticate")
	})

	t.Run("login has count flag", func(t *testing.T) {
		cmd := loginCmd()
		flag := cmd.Flags().Lookup("count")
		assert.NotNil(t, flag)
	})

	t.Run("login has ttl flag", func(t *testing.T) {
		cmd := loginCmd()
		flag := cmd.Flags().Lookup("ttl")
		assert.NotNil(t, flag)
	})

	t.Run("login fails with invalid project root", func(t *testing.T) {
		cmd := loginCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trust bundle not found")
	})

	t.Run("login fails when operator not running", func(t *testing.T) {
		tmpDir := t.TempDir()
		setupTestConfig(t, tmpDir)

		cmd := loginCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check bootstrap status")
	})

	t.Run("login fails when trust bundle missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Setup config without trust bundle
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
				"ca_cert_path": ".g8e/pki/trust/hub-bundle.pem",
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
				"insecure_mcp_gateway": 18789,
				"operator_bootstrap_https": 8441,
				"operator_https": 8440,
				"operator_public_https": 8442
			}
		}`
		pathsPath := filepath.Join(constantsDir, "paths.json")
		require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))

		cmd := loginCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		err := cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trust bundle not found")
	})
}

func TestLogoutCmd(t *testing.T) {
	t.Run("logout command has correct use", func(t *testing.T) {
		cmd := logoutCmd()
		assert.Equal(t, "logout", cmd.Use)
		assert.Contains(t, cmd.Short, "Clear")
		assert.Contains(t, cmd.Short, "credentials")
	})

	t.Run("logout succeeds with no active session", func(t *testing.T) {
		cmd := logoutCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		originalWd, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalWd)

		// Set up minimal config structure so config loads, then auth fails
		runtimeDir := filepath.Join(tmpDir, ".g8e")
		credentialsParentDir := filepath.Join(tmpDir, ".g8e")
		require.NoError(t, os.MkdirAll(credentialsParentDir, 0700))
		require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "pki"), 0755))

		// Create minimal paths.json structure
		protocolDir := filepath.Join(tmpDir, "protocol")
		constantsDir := filepath.Join(protocolDir, "constants")
		require.NoError(t, os.MkdirAll(constantsDir, 0755))

		pathsJSON := `{
			"host": "localhost",
			"infra": {
				"app_cert_dir": ".g8e/pki/app",
				"ca_cert_path": ".g8e/pki/trust/hub-bundle.pem",
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
				"insecure_mcp_gateway": 18789,
				"operator_bootstrap_https": 8441,
				"operator_https": 8440,
				"operator_public_https": 8442
			}
		}`
		pathsPath := filepath.Join(constantsDir, "paths.json")
		require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))

		// Set environment variable to override credentials directory
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", originalHome)

		err := cmd.RunE(cmd, []string{})
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No active session found")
	})

	t.Run("logout succeeds when no session exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := setupTestConfig(t, tmpDir)

		// Verify no credentials exist in test config
		creds, err := auth.LoadCredentials(cfg)
		require.NoError(t, err)
		require.Nil(t, creds)

		// Delete credentials using the auth function directly
		err = auth.DeleteCredentials(cfg)
		require.NoError(t, err)

		// Verify it succeeds even when no credentials exist
		_, err = os.Stat(cfg.CredentialsFile())
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("logout deletes credentials when session exists", func(t *testing.T) {
		// Test the underlying auth.DeleteCredentials function directly
		// since config.Load always uses the real home directory
		tmpDir := t.TempDir()
		cfg := setupTestConfig(t, tmpDir)

		// Create credentials
		creds := &auth.Credentials{
			OperatorSessionID: "op-sess-123",
			UserID:            "user-456",
			OperatorID:        "op-789",
			CLISessionID:      "cli-sess-abc",
		}
		require.NoError(t, auth.SaveCredentials(cfg, creds))

		// Create cert files
		certDir := cfg.CredentialsDir
		require.NoError(t, os.MkdirAll(certDir, 0700))
		require.NoError(t, os.WriteFile(cfg.CLICertFile(), []byte("cli-cert"), 0600))
		require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), []byte("cli-key"), 0600))
		require.NoError(t, os.WriteFile(cfg.OperatorCertFile(), []byte("op-cert"), 0600))
		require.NoError(t, os.WriteFile(cfg.OperatorKeyFile(), []byte("op-key"), 0600))

		// Verify credentials were saved
		loadedCreds, err := auth.LoadCredentials(cfg)
		require.NoError(t, err)
		require.NotNil(t, loadedCreds)

		// Delete credentials using the auth function
		require.NoError(t, auth.DeleteCredentials(cfg))

		// Verify files deleted
		_, err = os.Stat(cfg.CredentialsFile())
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(cfg.CLICertFile())
		assert.True(t, os.IsNotExist(err))
		_, err = os.Stat(cfg.CLIKeyFile())
		assert.True(t, os.IsNotExist(err))
	})
}

func TestAuthCommandFlags(t *testing.T) {
	t.Run("login count flag has default value", func(t *testing.T) {
		cmd := loginCmd()
		countFlag := cmd.Flags().Lookup("count")
		assert.NotNil(t, countFlag)
		assert.Equal(t, "1", countFlag.DefValue)
	})

	t.Run("login ttl flag has default value", func(t *testing.T) {
		cmd := loginCmd()
		ttlFlag := cmd.Flags().Lookup("ttl")
		assert.NotNil(t, ttlFlag)
		assert.Equal(t, "3600", ttlFlag.DefValue)
	})
}

func setupTestConfig(t *testing.T, tmpDir string) *config.Config {
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	pkiDir := filepath.Join(runtimeDir, "pki")
	secretsDir := filepath.Join(runtimeDir, "secrets")
	credentialsDir := filepath.Join(runtimeDir, "credentials")

	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	require.NoError(t, os.MkdirAll(credentialsDir, 0700))

	// Create trust bundle
	trustBundlePath := filepath.Join(pkiDir, "trust", "hub-bundle.pem")
	require.NoError(t, os.MkdirAll(filepath.Dir(trustBundlePath), 0755))
	require.NoError(t, os.WriteFile(trustBundlePath, []byte("dummy-trust-bundle"), 0644))

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
			"insecure_mcp_gateway": 18789,
			"operator_bootstrap_https": 8441,
			"operator_https": 8440,
			"operator_public_https": 8443
		}
	}`
	pathsPath := filepath.Join(constantsDir, "paths.json")
	require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))

	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     runtimeDir,
		PKIDir:         pkiDir,
		SecretsDir:     secretsDir,
		CredentialsDir: credentialsDir,
	}

	// Load paths.json to complete the config
	pathsData, err := os.ReadFile(pathsPath)
	if err != nil {
		require.NoError(t, err)
	}

	var paths config.PathsConfig
	require.NoError(t, json.Unmarshal(pathsData, &paths))
	cfg.Paths = &paths

	return cfg
}
