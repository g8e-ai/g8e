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

func TestCampaignEvalFormationsList_ShowCatalog(t *testing.T) {
	command := evalCmdWithConfig(panickingNativeEvalDeps(t))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "formations", "list", "--project-root", t.TempDir()})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "ultra-light-speedster")

	output.Reset()
	command.SetArgs([]string{"campaign", "formations", "show", "ultra-light-speedster", "--project-root", t.TempDir()})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "phi3.5:3.8b-mini-instruct-q4_K_M")
}

func TestCampaignEvalFormationsList_JSON(t *testing.T) {
	command := evalCmdWithConfig(panickingNativeEvalDeps(t))
	rootCmd := globalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "campaign", "formations", "list", "--project-root", t.TempDir()})
	require.NoError(t, rootCmd.Execute())

	var payload formationListJSON
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	require.Len(t, payload.Formations, 5)
}

func TestCampaignEvalFormationsRun_RequiresInferenceSession(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "formations", "run", "--project-root", root})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--inference-session is required")
}
