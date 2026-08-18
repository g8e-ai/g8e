// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
		cmd.SetContext(context.Background())

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

		exists, err := fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CredentialsFile()))
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
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.OperatorCertFile()), []byte("op-cert"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.OperatorKeyFile()), []byte("op-key"), constants.PermFilePrivate))

		loadedCreds, err := auth.LoadCredentials(fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, loadedCreds)

		require.NoError(t, auth.DeleteCredentials(fileSvc, cfg))

		exists, err := fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CredentialsFile()))
		require.NoError(t, err)
		assert.False(t, exists)
		exists, err = fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()))
		require.NoError(t, err)
		assert.False(t, exists)
		exists, err = fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()))
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

func setupTestConfig(t *testing.T, tmpDir string) (fs.RuntimeFileService, *config.Config) {
	t.Helper()

	fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	require.Equal(t, cfg.RuntimeDir, fileSvc.Resolve(""))

	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), []byte("dummy-trust-bundle"), constants.PermFilePublic))

	return fileSvc, cfg
}
