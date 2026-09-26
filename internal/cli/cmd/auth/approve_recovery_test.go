// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package authcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// TestApproveRecoveryCmd_PostsApproveAndPrintsState verifies that the
// approve-recovery command posts the token + approve flag to the mTLS
// approve-cli endpoint and prints the resulting state.
func TestApproveRecoveryCmd_PostsApproveAndPrintsState(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	respBody, err := json.Marshal(models.CLIRecoveryApproveResponse{
		Success: true,
		State:   models.CLIRecoveryStateApproved,
	})
	require.NoError(t, err)

	mockClient := &cmdtest.MockAPIClient{PostResp: respBody}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(_ fs.RuntimeFileService, _ *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveRecoveryCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"test-token"})
	require.NoError(t, err)

	require.Len(t, mockClient.PostCalls, 1)
	assert.Equal(t, constants.APIPaths.AuthCLIRecoveryApproveCLI, mockClient.PostCalls[0].Path)
	assert.Equal(t, "test-token", mockClient.PostCalls[0].Body.(models.CLIRecoveryApproveRequest).Token)
	assert.True(t, mockClient.PostCalls[0].Body.(models.CLIRecoveryApproveRequest).Approve)
	assert.Contains(t, buf.String(), "Recovery request approved")
}

// TestApproveRecoveryCmd_DenyPrintsDeniedState verifies that the --deny flag
// posts Approve=false and prints the denied state.
func TestApproveRecoveryCmd_DenyPrintsDeniedState(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	respBody, err := json.Marshal(models.CLIRecoveryApproveResponse{
		Success: true,
		State:   models.CLIRecoveryStateDenied,
	})
	require.NoError(t, err)

	mockClient := &cmdtest.MockAPIClient{PostResp: respBody}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(_ fs.RuntimeFileService, _ *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveRecoveryCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(nil))
	cmd.Flags().Set("deny", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"test-token"})
	require.NoError(t, err)

	require.Len(t, mockClient.PostCalls, 1)
	assert.False(t, mockClient.PostCalls[0].Body.(models.CLIRecoveryApproveRequest).Approve)
	assert.Contains(t, buf.String(), "Recovery request denied")
}

// TestApproveRecoveryCmd_RejectsMissingToken verifies that the command
// rejects a missing token argument via cobra's Args validation.
func TestApproveRecoveryCmd_RejectsMissingToken(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	mockClient := &cmdtest.MockAPIClient{}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(_ fs.RuntimeFileService, _ *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveRecoveryCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(nil))
	cmd.SetArgs([]string{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Empty(t, mockClient.PostCalls, "client must not be called when args validation fails")
}

// TestApproveRecoveryCmd_UnexpectedStateReturnsError verifies that a
// response state other than approved/denied returns a wrapped
// ErrCLIRecoveryRequestFailed.
func TestApproveRecoveryCmd_UnexpectedStateReturnsError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	respBody, err := json.Marshal(models.CLIRecoveryApproveResponse{
		Success: true,
		State:   models.CLIRecoveryStatePending,
	})
	require.NoError(t, err)

	mockClient := &cmdtest.MockAPIClient{PostResp: respBody}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(_ fs.RuntimeFileService, _ *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveRecoveryCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"test-token"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrCLIRecoveryRequestFailed)
}

// TestApproveRecoveryCmd_PostErrorReturnsWrappedError verifies that a post
// failure is surfaced as a wrapped error.
func TestApproveRecoveryCmd_PostErrorReturnsWrappedError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	postErr := fmt.Errorf("network failure")
	mockClient := &cmdtest.MockAPIClient{PostErr: postErr}
	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(_ fs.RuntimeFileService, _ *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveRecoveryCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"test-token"})
	require.Error(t, err)
	assert.ErrorIs(t, err, postErr)
}
