// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package authcmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthCmd(t *testing.T) {
	t.Run("auth command has correct use and description", func(t *testing.T) {
		cmd := Cmd()
		assert.Equal(t, "auth", cmd.Use)
		assert.Contains(t, cmd.Short, "Authentication")
		assert.Contains(t, cmd.Long, "mTLS")
	})
}

func TestEnrollUserCmd(t *testing.T) {
	t.Run("user command has correct use", func(t *testing.T) {
		cmd := enrollUserCmd()
		assert.Equal(t, "user", cmd.Use)
		assert.Contains(t, cmd.Short, "Enroll")
	})
}

// TestEnrollCmd_Parent verifies the enroll parent command has no RunE
// (cobra prints help and exits non-zero when invoked without a subcommand)
// and registers the user subcommand. The operator subcommand was removed
// when the unauthenticated operator enrollment bypass was closed (Phase 4);
// operators now enroll through the owner-approved platform enrollment
// protocol. The gui subcommand was removed in Phase 5 of the v2.1.8
// browser frontend connection UX rollout — `gw connect` is the sole local
// browser connection workflow.
func TestEnrollCmd_Parent(t *testing.T) {
	cmd := enrollCmd()
	assert.Equal(t, "enroll", cmd.Use)
	assert.Nil(t, cmd.RunE, "parent enroll command must have no RunE so cobra prints help on bare invocation")

	names := map[string]bool{}
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	assert.True(t, names["user"], "enroll parent must register the user subcommand")
	assert.True(t, names["pending"], "enroll parent must register the pending subcommand")
	assert.True(t, names["list"], "enroll parent must register the list subcommand")
	assert.True(t, names["approve"], "enroll parent must register the approve subcommand")
	assert.True(t, names["deny"], "enroll parent must register the deny subcommand")
	assert.True(t, names["revoke"], "enroll parent must register the revoke subcommand")
	assert.False(t, names["operator"], "enroll parent must NOT register the removed operator subcommand")
	assert.False(t, names["gui"], "enroll parent must NOT register the removed gui subcommand")
}

func TestLogoutCmd(t *testing.T) {
	t.Run("logout command has correct use", func(t *testing.T) {
		cmd := logoutCmd()
		assert.Equal(t, "logout", cmd.Use)
		assert.Contains(t, cmd.Short, "Clear")
		assert.Contains(t, cmd.Short, "credentials")
	})

	t.Run("logout succeeds with no active session", func(t *testing.T) {
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
		cmd := logoutCmdWithConfig(func(_ string) (*config.Config, error) { return cfg, nil }, cmdtest.FileSvcFactoryFor(fileSvc))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetContext(context.Background())

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "No active session found")
	})

	t.Run("logout succeeds when no session exists", func(t *testing.T) {
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		creds, err := auth.LoadCredentials(fileSvc, cfg)
		require.NoError(t, err)
		require.Nil(t, creds)

		err = auth.DeleteCredentials(fileSvc, cfg)
		require.NoError(t, err)

		exists, err := fileSvc.FileExists(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CredentialsFile()))
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("logout deletes credentials when session exists", func(t *testing.T) {
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		creds := &auth.Credentials{
			OperatorSessionID: "op-sess-123",
			UserID:            "user-456",
			OperatorID:        "op-789",
			CLISessionID:      "cli-sess-abc",
		}

		require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
		require.NoError(t, fileSvc.WriteFile(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.OperatorCertFile()), []byte("op-cert"), constants.PermFilePrivate))
		require.NoError(t, fileSvc.WriteFile(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.OperatorKeyFile()), []byte("op-key"), constants.PermFilePrivate))

		loadedCreds, err := auth.LoadCredentials(fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, loadedCreds)

		require.NoError(t, auth.DeleteCredentials(fileSvc, cfg))

		exists, err := fileSvc.FileExists(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CredentialsFile()))
		require.NoError(t, err)
		assert.False(t, exists)
		exists, err = fileSvc.FileExists(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CLICertFile()))
		require.NoError(t, err)
		assert.False(t, exists)
		exists, err = fileSvc.FileExists(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CLIKeyFile()))
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestAuthCommandFlags(t *testing.T) {
	t.Run("enroll has no count flag", func(t *testing.T) {
		cmd := enrollUserCmd()
		countFlag := cmd.Flags().Lookup("count")
		assert.Nil(t, countFlag)
	})

	t.Run("enroll has no ttl flag", func(t *testing.T) {
		cmd := enrollUserCmd()
		ttlFlag := cmd.Flags().Lookup("ttl")
		assert.Nil(t, ttlFlag)
	})
}
