// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	factoryErr := fmt.Errorf("factory boom")

	for _, args := range [][]string{{"show"}, {"set", "--judge", "qwen3:1.7b"}} {
		cmd := cmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(factoryErr))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs(args)

		err := cmd.Execute()

		require.ErrorIs(t, err, constants.ErrFileServiceInit, args)
		assert.ErrorIs(t, err, factoryErr)
	}
}
