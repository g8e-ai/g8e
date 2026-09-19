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
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func TestAuthContextCmdWithConfig_ExportsCanonicalCLIIdentity(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	creds := &auth.Credentials{
		OperatorSessionID: "operator-session-123",
		UserID:            "user-123",
		OperatorID:        "operator-123",
		CLISessionID:      "cli-session-123",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))

	cmd := authContextCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), fileSvcFactoryFor(fileSvc))
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.NoError(t, cmd.RunE(cmd, nil))

	var got auth.ClientAuthContext
	require.NoError(t, json.Unmarshal(output.Bytes(), &got))
	assert.Equal(t, creds.OperatorSessionID, got.OperatorSessionID)
	assert.Equal(t, creds.CLISessionID, got.CLISessionID)
	assert.Equal(t, creds.UserID, got.UserID)
	assert.Equal(t, creds.OperatorID, got.OperatorID)
	assert.Equal(t, cfg.CLICertFile(), got.ClientCert)
	assert.Equal(t, cfg.CLIKeyFile(), got.ClientKey)
}

func TestAuthContextCmdWithConfig_UsesExplicitProjectRoot(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	creds := &auth.Credentials{
		OperatorSessionID: "operator-session-123",
		UserID:            "user-123",
		CLISessionID:      "cli-session-123",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))
	var configRoot string
	configLoader := func(projectRoot string) (*config.Config, error) {
		configRoot = projectRoot
		return cfg, nil
	}
	var fileSvcRoot string
	fileSvcFactory := func(projectRoot string, _ *slog.Logger) (fs.RuntimeFileService, error) {
		fileSvcRoot = projectRoot
		return fileSvc, nil
	}
	cmd := authContextCmdWithConfig(configLoader, panickingClientFactory(), fileSvcFactory)
	require.NoError(t, cmd.Flags().Set("project-root", cfg.ProjectRoot))

	require.NoError(t, cmd.RunE(cmd, nil))

	assert.Equal(t, cfg.ProjectRoot, configRoot)
	assert.Equal(t, cfg.ProjectRoot, fileSvcRoot)
}

// TestAuthContextCmdWithConfig_ResolvesBindingFromCLISessionEndpoint
// verifies that a credentials file with no operator binding is resynced
// from the gateway's authoritative CLI session record (GET
// /api/v1/auth/cli/session) — not from an operator listing query — and
// that the resolved pair is persisted back to local credentials.
func TestAuthContextCmdWithConfig_ResolvesBindingFromCLISessionEndpoint(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	creds := &auth.Credentials{UserID: "user-123", CLISessionID: "cli-session-123"}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))
	response, err := json.Marshal(models.CLISessionInfoResponse{
		Success:           true,
		CLISessionID:      creds.CLISessionID,
		UserID:            creds.UserID,
		OperatorSessionID: "operator-session-123",
		OperatorID:        "embedded-operator",
	})
	require.NoError(t, err)
	client := &mockAPIClient{getResp: response}
	cmd := authContextCmdWithConfig(configLoaderFor(cfg), mockClientFactory(client), fileSvcFactoryFor(fileSvc))
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.NoError(t, cmd.RunE(cmd, nil))

	assert.Equal(t, []string{constants.APIPaths.AuthCLISession}, client.getCalls)

	var got auth.ClientAuthContext
	require.NoError(t, json.Unmarshal(output.Bytes(), &got))
	assert.Equal(t, "embedded-operator", got.OperatorID)
	assert.Equal(t, "operator-session-123", got.OperatorSessionID)

	// The resolved binding is persisted back to local credentials.
	saved, err := auth.LoadCredentials(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, "embedded-operator", saved.OperatorID)
	assert.Equal(t, "operator-session-123", saved.OperatorSessionID)
}

// TestAuthContextCmdWithConfig_UnboundSessionFailsClosed verifies that a
// CLI session record with no persisted operator binding fails closed with
// ErrNotAuthenticated and directs the caller to the refresh path that
// resyncs the binding.
func TestAuthContextCmdWithConfig_UnboundSessionFailsClosed(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	creds := &auth.Credentials{UserID: "user-123", CLISessionID: "cli-session-123"}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))
	response, err := json.Marshal(models.CLISessionInfoResponse{
		Success:      true,
		CLISessionID: creds.CLISessionID,
		UserID:       creds.UserID,
	})
	require.NoError(t, err)
	client := &mockAPIClient{getResp: response}
	cmd := authContextCmdWithConfig(configLoaderFor(cfg), mockClientFactory(client), fileSvcFactoryFor(fileSvc))

	err = cmd.RunE(cmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
	assert.Contains(t, err.Error(), "auth refresh")
	assert.Equal(t, []string{constants.APIPaths.AuthCLISession}, client.getCalls)
}

func TestAuthContextCmdWithConfig_RejectsIncompleteIdentity(t *testing.T) {
	tests := []struct {
		name  string
		creds *auth.Credentials
	}{
		{name: "missing credentials"},
		{name: "missing session", creds: &auth.Credentials{UserID: "user-123", CLISessionID: "cli-session-123"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileSvc, cfg := newCmdTestEnv(t)
			if tt.creds != nil {
				require.NoError(t, auth.SaveCredentials(fileSvc, cfg, tt.creds))
			}
			cmd := authContextCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), fileSvcFactoryFor(fileSvc))

			err := cmd.RunE(cmd, nil)

			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
		})
	}
}
