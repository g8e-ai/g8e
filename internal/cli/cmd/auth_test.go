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
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
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
		fileSvc, cfg := newCmdTestEnv(t)
		cmd := logoutCmdWithConfig(func(_ string) (*config.Config, error) { return cfg, nil }, fileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No active session found")
	})

	t.Run("logout succeeds when no session exists", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		creds, err := auth.LoadCredentials(fileSvc, cfg)
		require.NoError(t, err)
		require.Nil(t, creds)

		err = auth.DeleteCredentials(fileSvc, cfg)
		require.NoError(t, err)

		exists, err := fileSvc.FileExists(context.Background(), relFromAbs(fileSvc, cfg.CredentialsFile()))
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("logout deletes credentials when session exists", func(t *testing.T) {
		fileSvc, cfg := newCmdTestEnv(t)

		creds := &auth.Credentials{
			OperatorSessionID: "op-sess-123",
			UserID:            "user-456",
			OperatorID:        "op-789",
			CLISessionID:      "cli-sess-abc",
		}

		require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
		require.NoError(t, fileSvc.WriteFile(context.Background(), relFromAbs(fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(context.Background(), relFromAbs(fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(context.Background(), relFromAbs(fileSvc, cfg.OperatorCertFile()), []byte("op-cert"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(context.Background(), relFromAbs(fileSvc, cfg.OperatorKeyFile()), []byte("op-key"), constants.PermFilePrivate))

		loadedCreds, err := auth.LoadCredentials(fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, loadedCreds)

		require.NoError(t, auth.DeleteCredentials(fileSvc, cfg))

		exists, err := fileSvc.FileExists(context.Background(), relFromAbs(fileSvc, cfg.CredentialsFile()))
		require.NoError(t, err)
		assert.False(t, exists)
		exists, err = fileSvc.FileExists(context.Background(), relFromAbs(fileSvc, cfg.CLICertFile()))
		require.NoError(t, err)
		assert.False(t, exists)
		exists, err = fileSvc.FileExists(context.Background(), relFromAbs(fileSvc, cfg.CLIKeyFile()))
		require.NoError(t, err)
		assert.False(t, exists)
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
