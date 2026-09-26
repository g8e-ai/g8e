// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tuicmd

import (
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

var errFactory = fmt.Errorf("factory boom")

func TestTUI_FileSvcFactoryError(t *testing.T) {
	cfg := setupTUITestConfig(t)

	deps := stubTUIDeps(t, cfg)
	deps.fileSvcFactory = cmdtest.FailingFileSvcFactory(errFactory)
	deps.loadCredentials = func(_ fs.RuntimeFileService, _ *config.Config) (*auth.Credentials, error) {
		panic("loadCredentials should not be called when fileSvcFactory fails")
	}

	cmd := tuiCmdWithDeps(deps)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}
