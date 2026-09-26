// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"fmt"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

var errFactory = fmt.Errorf("factory boom")

func TestMcpStdioCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	origConfigLoad := shared.ConfigLoad
	shared.ConfigLoad = func(string) (*config.Config, error) { return cfg, nil }
	t.Cleanup(func() { shared.ConfigLoad = origConfigLoad })
	cmd := McpStdioCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory))
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestAgentRunCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	origConfigLoad := shared.ConfigLoad
	shared.ConfigLoad = func(string) (*config.Config, error) { return cfg, nil }
	t.Cleanup(func() { shared.ConfigLoad = origConfigLoad })
	cmd := agentRunCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory), authcmd.PanickingEnrollerFactory())
	err := cmd.RunE(cmd, []string{"claude"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}
