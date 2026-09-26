// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package authcmd

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

var errFactory = fmt.Errorf("factory boom")

func TestApproveCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := approveCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"abc123"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestApproveRecoveryCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := approveRecoveryCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"abc123"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestApprovePlatformEnrollmentCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := approvePlatformEnrollmentCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"req-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDenyPlatformEnrollmentCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := denyPlatformEnrollmentCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"req-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestRevokePlatformEnrollmentCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := revokePlatformEnrollmentCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"req-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPendingPlatformEnrollmentCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := pendingPlatformEnrollmentCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestLogoutCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := logoutCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestAuthContextCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := authContextCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestEnrollUserCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	stubCheckOperatorRunning := func(*config.Config) error { return nil }
	cmd := enrollUserCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory), stubCheckOperatorRunning, NewDefaultEnrollmentCoordinator)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}
