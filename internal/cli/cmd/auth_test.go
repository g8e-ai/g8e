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
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
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

func TestEnrollCmd(t *testing.T) {
	t.Run("enroll command has correct use", func(t *testing.T) {
		cmd := enrollCmd()
		assert.Equal(t, "enroll", cmd.Use)
		assert.Contains(t, cmd.Short, "Enroll")
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
		tmpDir := testutil.TempDir(t)
		os.Chdir(tmpDir)
		t.Cleanup(func() { os.Chdir(originalWd) })

		// Set up minimal config structure so config loads, then auth fails
		runtimeDir := filepath.Join(tmpDir, ".g8e")
		credentialsParentDir := runtimeDir
		require.NoError(t, os.MkdirAll(credentialsParentDir, 0700))
		require.NoError(t, os.MkdirAll(filepath.Join(runtimeDir, "pki"), 0755))

		// Create minimal paths.json structure
		protocolDir := filepath.Join(tmpDir, "protocol")
		constantsDir := filepath.Join(protocolDir, "constants")
		require.NoError(t, os.MkdirAll(constantsDir, 0755))

		pathsJSON := minimalPathsJSON(t)
		pathsPath := filepath.Join(constantsDir, "paths.json")
		require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))

		// Set environment variable to override credentials directory
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", originalHome)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No active session found")
	})

	t.Run("logout succeeds when no session exists", func(t *testing.T) {
		tmpDir := testutil.TempDir(t)

		// Set HOME to tmpDir to ensure credentials are read from temp directory
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", originalHome)

		// Create a simple config that points to tmpDir for credentials
		// Avoid using setupTestConfig which creates a conflicting .g8e directory
		cfg := &config.Config{
			ProjectRoot: tmpDir,
			RuntimeDir:  filepath.Join(tmpDir, paths.Infra.RuntimeDir),
			PKIDir:      filepath.Join(tmpDir, paths.Infra.PkiDir),
			SecretsDir:  filepath.Join(tmpDir, paths.Infra.SecretsDir),
			Paths: &config.PathsConfig{
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
					VaultDir             string `json:"vault_dir"`
					VaultKeyPath         string `json:"vault_key_path"`
				}{
					CACertPath:           paths.Infra.CaCertPath,
					PKIDir:               paths.Infra.PkiDir,
					SecretsDir:           paths.Infra.SecretsDir,
					AppCertDir:           paths.Infra.AppCertDir,
					ProtocolDir:          paths.Infra.ProtocolDir,
					ProtocolConstantsDir: paths.Infra.ProtocolConstantsDir,
					ProtocolModelsDir:    paths.Infra.ProtocolModelsDir,
					DocsDir:              paths.Infra.DocsDir,
					SSHConfigPath:        paths.Infra.SshConfigPath,
					DBPath:               paths.Infra.DbPath,
				},
			},
		}

		fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
		require.NoError(t, err)

		// Verify no credentials exist in test config
		creds, err := auth.LoadCredentials(fileSvc, cfg)
		require.NoError(t, err)
		require.Nil(t, creds)

		// Delete credentials using the auth function directly
		err = auth.DeleteCredentials(fileSvc, cfg)
		require.NoError(t, err)

		// Verify it succeeds even when no credentials exist
		_, err = os.Stat(cfg.CredentialsFile())
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("logout deletes credentials when session exists", func(t *testing.T) {
		// Test the underlying auth.DeleteCredentials function directly
		// since config.Load always uses the real home directory
		tmpDir := testutil.TempDir(t)
		cfg := setupTestConfig(t, tmpDir)

		// Create credentials directory (but not the credentials file itself)
		certDir := cfg.RuntimeDir
		require.NoError(t, os.MkdirAll(certDir, 0700))

		// Create credentials
		creds := &auth.Credentials{
			OperatorSessionID: "op-sess-123",
			UserID:            "user-456",
			OperatorID:        "op-789",
			CLISessionID:      "cli-sess-abc",
		}
		fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
		require.NoError(t, err)

		require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
		require.NoError(t, os.WriteFile(cfg.CLICertFile(), []byte("cli-cert"), 0600))
		require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), []byte("cli-key"), 0600))
		require.NoError(t, os.WriteFile(cfg.OperatorCertFile(), []byte("op-cert"), 0600))
		require.NoError(t, os.WriteFile(cfg.OperatorKeyFile(), []byte("op-key"), 0600))

		// Verify credentials were saved
		loadedCreds, err := auth.LoadCredentials(fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, loadedCreds)

		// Delete credentials using the auth function
		require.NoError(t, auth.DeleteCredentials(fileSvc, cfg))

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
	t.Run("enroll has no count flag", func(t *testing.T) {
		cmd := enrollCmd()
		countFlag := cmd.Flags().Lookup("count")
		assert.Nil(t, countFlag)
	})

	t.Run("enroll has no ttl flag", func(t *testing.T) {
		cmd := enrollCmd()
		ttlFlag := cmd.Flags().Lookup("ttl")
		assert.Nil(t, ttlFlag)
	})
}

func setupTestConfig(t *testing.T, tmpDir string) *config.Config {
	runtimeDir := filepath.Join(tmpDir, ".g8e")
	pkiDir := filepath.Join(runtimeDir, "pki")
	secretsDir := filepath.Join(runtimeDir, "secrets")

	require.NoError(t, os.MkdirAll(pkiDir, 0755))
	require.NoError(t, os.MkdirAll(secretsDir, 0700))
	// Note: credentialsDir is .g8e by default, do NOT create a "credentials" subdirectory
	// The credentials file is written to .g8e/credentials, not .g8e/credentials/credentials

	// Create trust bundle
	trustBundlePath := filepath.Join(pkiDir, "trust", "g8eg-ca-bundle.pem")
	require.NoError(t, os.MkdirAll(filepath.Dir(trustBundlePath), 0755))
	require.NoError(t, os.WriteFile(trustBundlePath, []byte("dummy-trust-bundle"), 0644))

	// Use LoadWithPaths for hermetic test execution
	// This ensures the test does not depend on any running operator
	pathsJSON := minimalPathsJSON(t)

	cfg, err := config.LoadWithPaths(tmpDir, []byte(pathsJSON))
	require.NoError(t, err)
	return cfg
}
