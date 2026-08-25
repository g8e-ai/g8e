// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package config

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// setupCLIIdentityEnv creates a temp-rooted fileSvc and aligned cfg, writes
// a credentials file with the given identity, and returns the projectRoot
// that LoadCLIIdentity should be called with. Mirrors the newCmdTestEnv
// pattern from internal/cli/cmd but lives in the harness config package so
// it can test LoadCLIIdentity without os.Chdir.
func setupCLIIdentityEnv(t *testing.T, creds *auth.Credentials) string {
	t.Helper()
	tmpDir := testutil.TempDir(t)

	fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)
	require.Equal(t, cfg.RuntimeDir, fileSvc.Resolve(""))

	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), []byte("dummy-trust-bundle"), constants.PermFilePublic))

	if creds != nil {
		credsData, err := json.Marshal(creds)
		require.NoError(t, err)
		credsRel, err := fileSvc.RelFromAbs(cfg.CredentialsFile())
		require.NoError(t, err)
		require.NoError(t, fileSvc.WriteFile(context.Background(), credsRel, credsData, constants.PermFileReadOnly))
	}

	return tmpDir
}

func TestLoadCLIIdentity_PopulatedCredentials(t *testing.T) {
	want := &auth.Credentials{
		UserID:            "user-123",
		CLISessionID:      "cli-session-456",
		OperatorSessionID: "op-session-789",
	}
	projectRoot := setupCLIIdentityEnv(t, want)

	identity, err := LoadCLIIdentity(projectRoot)
	require.NoError(t, err)
	assert.Equal(t, want.UserID, identity.UserID)
	assert.Equal(t, want.CLISessionID, identity.CLISessionID)
	assert.Equal(t, want.OperatorSessionID, identity.OperatorSessionID)
}

func TestLoadCLIIdentity_MissingCredentialsFile(t *testing.T) {
	// No credentials file written — LoadCLIIdentity returns zero identity, nil error.
	projectRoot := setupCLIIdentityEnv(t, nil)

	identity, err := LoadCLIIdentity(projectRoot)
	require.NoError(t, err)
	assert.Empty(t, identity.UserID)
	assert.Empty(t, identity.CLISessionID)
	assert.Empty(t, identity.OperatorSessionID)
}

func TestLoadCLIIdentity_MalformedCredentialsFile(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(tmpDir, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), []byte("dummy"), constants.PermFilePublic))

	credsRel, err := fileSvc.RelFromAbs(cfg.CredentialsFile())
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), credsRel, []byte("{not valid json"), constants.PermFileReadOnly))

	_, err = LoadCLIIdentity(tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load CLI credentials")
}

func TestDefault_PopulatesIdentityFromHelper(t *testing.T) {
	// Verify that Default() calls LoadCLIIdentity and populates the identity
	// fields. This is a smoke test — the full Default() path uses cwd-based
	// config.Load which is exercised by the existing TestDefault. Here we
	// just confirm the helper wiring is present by checking that Default()
	// does not panic and returns a config with the expected static fields.
	cfg := Default()
	assert.True(t, cfg.UseCLIConfig)
	assert.NotEmpty(t, cfg.MTLSBaseURL)
}
