// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderObserverRun_JSONPrintsConfigAndExitsOnCancel(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	rootCmd := globalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rootCmd.SetContext(ctx)
	rootCmd.SetArgs([]string{
		"eval", "dev", "provider-observer", "run",
		"--project-root", root,
		"--observer-id", "observer-test",
		"--sample-interval-ms", "100",
		"--poll-interval-ms", "100",
	})
	execErr := rootCmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "provider observer run")
	assert.Contains(t, output.String(), "observer-test")
	assert.Contains(t, output.String(), "nvidia-smi+proc-meminfo")
}
