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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// stubRefreshClient is a test-only refreshClient that returns a canned
// CLISessionRefresh result or error without network I/O.

// stubRefreshClientFactory returns a refreshClientFactory that always
// returns the given stub.

// panickingRefreshClientFactory returns a refreshClientFactory whose client
// panics if called. Used to assert that refresh is not attempted on a code
// path that must not reach the gateway.
func panickingRefreshClientFactory() RefreshClientFactory {
	return func(cfg *config.Config) refreshClient {
		return &panickingRefreshClient{}
	}
}

type panickingRefreshClient struct{}

func (p *panickingRefreshClient) Refresh(ctx context.Context, fileSvc fs.RuntimeFileService) (auth.CLISessionRefresh, error) {
	panic("refreshClient should not be called on this code path")
}

// TestRefreshCmd_Success verifies the success path: credentials exist, the
// refresh client returns a new session, and the command updates the local
// credentials with the new session ID and prints the confirmation.
func TestRefreshCmd_Success(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

	// Seed local credentials with an old session ID.
	creds := &auth.Credentials{
		OperatorSessionID: "op-sess-old",
		UserID:            "user-refresh-success",
		OperatorID:        "op-id-old",
		CLISessionID:      "cli-sess-old",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))

	stub := &StubRefreshClient{
		Result: auth.CLISessionRefresh{
			CLISessionID: "cli-sess-new",
			UserID:       "user-refresh-success",
		},
	}

	cmd := refreshCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FileSvcFactoryFor(fileSvc),
		StubRefreshClientFactory(stub),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())

	require.NoError(t, cmd.RunE(cmd, nil))

	assert.True(t, stub.called, "refresh client should have been called")
	assert.Contains(t, buf.String(), "CLI session refreshed successfully")
	assert.Contains(t, buf.String(), "cli-sess-new")

	// The local credentials must be updated with the new session ID.
	loaded, Err := auth.LoadCredentials(fileSvc, cfg)
	require.NoError(t, Err)
	require.NotNil(t, loaded)
	assert.Equal(t, "cli-sess-new", loaded.CLISessionID, "local credentials must reflect the new session ID")
	assert.Equal(t, "user-refresh-success", loaded.UserID, "user ID must be unchanged")
}

// TestRefreshCmd_NoLocalIdentity verifies that when no local credentials
// exist, the command prints an actionable message pointing to enrollment
// and returns nil (not an error).
func TestRefreshCmd_NoLocalIdentity(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

	stub := &StubRefreshClient{}

	cmd := refreshCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FileSvcFactoryFor(fileSvc),
		StubRefreshClientFactory(stub),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())

	require.NoError(t, cmd.RunE(cmd, nil))

	assert.False(t, stub.called, "refresh client should NOT be called when no local identity exists")
	assert.Contains(t, buf.String(), "No local CLI identity found")
	assert.Contains(t, buf.String(), "auth enroll user")
}

// TestRefreshCmd_RefreshFailure verifies that a refresh client error is
// returned by the command (the gateway rejected the refresh, e.g., the
// cert itself is expired).
func TestRefreshCmd_RefreshFailure(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

	creds := &auth.Credentials{
		OperatorSessionID: "op-sess-fail",
		UserID:            "user-refresh-fail",
		OperatorID:        "op-id-fail",
		CLISessionID:      "cli-sess-fail",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))

	stub := &StubRefreshClient{
		Err: fmt.Errorf("gateway rejected refresh: certificate expired"),
	}

	cmd := refreshCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FileSvcFactoryFor(fileSvc),
		StubRefreshClientFactory(stub),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())

	Err := cmd.RunE(cmd, nil)
	require.Error(t, Err)
	assert.Contains(t, Err.Error(), "auth refresh")
	assert.Contains(t, Err.Error(), "certificate expired")

	// The local credentials must NOT be updated on failure.
	loaded, Err := auth.LoadCredentials(fileSvc, cfg)
	require.NoError(t, Err)
	require.NotNil(t, loaded)
	assert.Equal(t, "cli-sess-fail", loaded.CLISessionID, "local credentials must be unchanged on refresh failure")
}

// TestRefreshCmdWithConfig_FileSvcFactoryError verifies that a fileSvcFactory
// failure is wrapped with constants.ErrFileServiceInit and the refresh client
// is not called.
func TestRefreshCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := refreshCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FailingFileSvcFactory(errFactory),
		panickingRefreshClientFactory(),
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())

	Err := cmd.RunE(cmd, nil)
	require.Error(t, Err)
	assert.ErrorIs(t, Err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, Err, errFactory)
}
