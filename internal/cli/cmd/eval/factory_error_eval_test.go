// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

var errFactory = fmt.Errorf("factory boom")

func TestEvalCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	deps := nativeEvalDeps{
		configLoader:   cmdtest.ConfigLoaderFor(cfg),
		fileSvcFactory: cmdtest.FailingFileSvcFactory(errFactory),
		clientFactory: func(harnessconfig.Config) (*harnessclient.Client, error) {
			panic("client factory should not be called when fileSvcFactory fails")
		},
		authLoader: func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
			panic("auth loader should not be called when fileSvcFactory fails")
		},
		now:   time.Now,
		newID: func() string { return "run-id" },
	}
	tests := []struct {
		name string
		args []string
	}{
		{name: "run", args: []string{"boundary", "run"}},
		{name: "verify", args: []string{"boundary", "verify", "run-id"}},
		{name: "show", args: []string{"boundary", "show", "run-id"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := evalCmdWithConfig(deps)
			cmd.SetArgs(test.args)
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			err := cmd.Execute()
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrFileServiceInit)
			assert.ErrorIs(t, err, errFactory)
		})
	}
}
