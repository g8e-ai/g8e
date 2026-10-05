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
		newID: func() (string, error) { return "run-id", nil },
	}
	tests := []struct {
		name string
		args []string
	}{
		// boundary
		{name: "boundary list", args: []string{"boundary", "list"}},
		{name: "boundary run", args: []string{"boundary", "run"}},
		{name: "boundary verify", args: []string{"boundary", "verify", "run-id"}},
		{name: "boundary show", args: []string{"boundary", "show", "run-id"}},

		// models
		{name: "models list", args: []string{"models", "list"}},
		{name: "models show", args: []string{"models", "show", "qwen3:4b"}},
		{name: "models remove", args: []string{"models", "remove", "qwen3:4b"}},
		{name: "models import", args: []string{"models", "import", "qwen3:4b"}},
		{name: "models freeze", args: []string{"models", "freeze"}},
		{name: "models pull", args: []string{"models", "pull", "qwen3:4b"}},
		{name: "models diff", args: []string{"models", "diff"}},

		// campaigns
		{name: "campaigns list", args: []string{"campaigns", "list"}},
		{name: "campaigns show", args: []string{"campaigns", "show", "camp-id"}},
		{name: "campaigns create", args: []string{"campaigns", "create", "camp-id", "qwen3:4b"}},
		{name: "campaigns archive", args: []string{"campaigns", "archive", "camp-id"}},
		{name: "campaigns unarchive", args: []string{"campaigns", "unarchive", "camp-id"}},

		// runs
		{name: "runs list", args: []string{"runs", "list"}},
		{name: "runs show", args: []string{"runs", "show", "run-id"}},
		{name: "runs start", args: []string{"runs", "start", "camp-id"}},
		{name: "runs resume", args: []string{"runs", "resume", "run-id"}},
		{name: "runs cancel", args: []string{"runs", "cancel", "run-id"}},
		{name: "runs logs", args: []string{"runs", "logs", "run-id"}},
		{name: "runs verify", args: []string{"runs", "verify", "run-id"}},
		{name: "runs publish", args: []string{"runs", "publish", "run-id"}},
		{name: "runs export", args: []string{"runs", "export", "run-id", "--output-dir", "out"}},
		{name: "runs repair", args: []string{"runs", "repair", "run-id", "--results"}},
		{name: "runs compare", args: []string{"runs", "compare", "run-1", "run-2"}},
		{name: "runs archive", args: []string{"runs", "archive", "run-id"}},
		{name: "runs unarchive", args: []string{"runs", "unarchive", "run-id"}},

		// rollout
		{name: "rollout list", args: []string{"rollout", "list"}},
		{name: "rollout add", args: []string{"rollout", "add", "qwen3:4b"}},
		{name: "rollout remove", args: []string{"rollout", "remove", "qwen3:4b"}},
		{name: "rollout next", args: []string{"rollout", "next"}},
		{name: "rollout retry", args: []string{"rollout", "retry", "qwen3:4b"}},
		{name: "rollout skip", args: []string{"rollout", "skip", "qwen3:4b"}},
		{name: "rollout run", args: []string{"rollout", "run"}},

		// formations
		{name: "formations smoke", args: []string{"formations", "smoke", "qwen-powerhouse"}},

		// gates
		{name: "gates chat", args: []string{"gates", "chat", "--model", "qwen3:4b"}},
		{name: "gates inference", args: []string{"gates", "inference", "--model", "qwen3:4b"}},
		{name: "gates probe", args: []string{"gates", "probe", "qwen3:4b", "--prompt", "hi"}},

		// backup
		{name: "backup", args: []string{"backup", "--output-dir", "out"}},
		{name: "restore", args: []string{"restore", "snapshot-dir"}},
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
