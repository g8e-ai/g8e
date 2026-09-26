// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

var errFactory = fmt.Errorf("factory boom")

func TestOperatorStopCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := operatorStopCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"session-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestOperatorBindCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	bindClientFactory := func(*config.Config) operatorBindClient {
		panic("bind client factory should not be called when fileSvcFactory fails")
	}
	cmd := operatorBindCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), bindClientFactory, cmdtest.FailingFileSvcFactory(errFactory))

	err := cmd.RunE(cmd, []string{"list"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestOperatorShowCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := operatorShowCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))

	err := cmd.RunE(cmd, []string{"operator-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestOperatorRunCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	clientFactory := func(fs.RuntimeFileService, *config.Config, time.Duration) (authcmd.APIClient, error) {
		panic("operator run client factory should not be called when fileSvcFactory fails")
	}
	cmd := operatorRunCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), clientFactory, cmdtest.FailingFileSvcFactory(errFactory))
	require.NoError(t, cmd.Flags().Set("cmd", "printf test"))

	err := cmd.RunE(cmd, []string{"operator-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestOperatorListCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := operatorListCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestOperatorDeployCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := operatorDeployCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}
