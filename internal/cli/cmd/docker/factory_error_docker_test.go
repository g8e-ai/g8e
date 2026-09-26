// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package docker

import (
	"bytes"
	"fmt"
	"testing"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

var errFactory = fmt.Errorf("factory boom")

func TestDockerStartCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	writeRootCompose(t)

	stubCheckOperatorRunning := func(*config.Config) error { return nil }
	cmd := dockerStartCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FailingFileSvcFactory(errFactory),
		authcmd.PanickingClientFactory(),
		stubCheckOperatorRunning,
		authcmd.PanickingEnrollerFactory(),
	)
	cmd.Flags().Set("full", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}
