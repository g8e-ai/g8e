// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCampaignEvalRepairTraceDigests_ViaCLI(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "repair", "trace-digests", "--project-root", root, active.RunID})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Repaired")
}

func TestCampaignEvalRepairTraceDigests_JSON(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	rootCmd := globalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "campaign", "repair", "trace-digests", "--project-root", root, active.RunID})
	require.NoError(t, rootCmd.Execute())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	assert.Equal(t, active.RunID, payload["run_id"])
}

func TestCampaignEvalRepairResults_ViaCLI(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "repair", "results", "--project-root", root, active.RunID})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Repaired")
}
