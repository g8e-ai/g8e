// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package public

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

var errFactory = fmt.Errorf("factory boom")

func TestPublicInitCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := publicInitCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	cmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", "https://mirror.example"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicConfigSetCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := publicConfigSetCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	cmd.SetArgs([]string{"--mirror-origin", "https://mirror.example"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicSourceTransitionCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := publicSourceTransitionCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	cmd.SetArgs([]string{"--source-id", "deployment-new", "--yes"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicPublishCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := publicPublishCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	cmd.SetArgs([]string{constants.TestPublicFeedRecordsFilename})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicPushCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := publicPushCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicStatusCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := publicStatusCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicRotateKeyCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := publicRotateKeyCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicRepairOutboxCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := publicRepairOutboxCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}
