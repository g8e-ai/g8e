// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func TestSuitesList_ShowsBuiltinSuites(t *testing.T) {
	env := setupRunEnv(t)

	out := env.mustRun(t, "suites", "list")
	assert.Contains(t, out, evaluation.DefaultSuiteID)
	assert.Contains(t, out, evaluation.SmokeSuiteID)
	assert.Contains(t, out, "built-in")

	var payload suiteListJSON
	require.NoError(t, env.runJSON(t, &payload, "suites", "list"))
	require.Len(t, payload.Suites, 2)
	assert.Equal(t, evaluation.DefaultSuiteID, payload.Suites[0].ID)
	assert.True(t, payload.Suites[0].Builtin)
	assert.Equal(t, evaluation.DefaultSuiteScenarioCount, payload.Suites[0].ScenarioCount)
}

func TestSuites_HasNoSingularAlias(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "suite", "list")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown command "suite"`)
}

func TestSuitesExport_WritesTheCreateFormat(t *testing.T) {
	env := setupRunEnv(t)

	out := env.mustRun(t, "suites", "export", evaluation.DefaultSuiteID)
	def, err := evaluation.DecodeScenarioSuite([]byte(out))
	require.NoError(t, err)
	assert.Equal(t, evaluation.DefaultSuiteID, def.ID)
	assert.Len(t, def.Scenarios, evaluation.DefaultSuiteScenarioCount)
}

func TestSuitesCreate_ThenCampaignFreezesIt(t *testing.T) {
	env := setupRunEnv(t)
	path := writeSuiteFile(t, "my-suite", "1.0.0", "instruction-exact-format", "tool-select-file-read")

	out := env.mustRun(t, "suites", "create", path)
	assert.Contains(t, out, "my-suite@1.0.0 created")
	assert.Contains(t, out, "--suite my-suite")

	var show suiteShowJSON
	require.NoError(t, env.runJSON(t, &show, "suites", "show", "my-suite"))
	assert.False(t, show.Builtin)
	assert.Equal(t, 2, show.ScenarioCount)
	assert.NotEmpty(t, show.CatalogDigest)
	require.Len(t, show.Scenarios, 2)

	var created campaignCreateJSON
	require.NoError(t, env.runJSON(t, &created, "campaigns", "create", "eval-custom", "qwen3:4b", "--suite", "my-suite"))
	assert.Equal(t, "my-suite@1.0.0", created.Suite)
	assert.Equal(t, uint32(2), created.ScenarioCount)
	assert.Equal(t, show.CatalogDigest, created.CatalogDigest)

	// The campaign carries its own copy: deleting the suite changes nothing.
	env.mustRun(t, "suites", "delete", "my-suite")
	_, err := env.run(t, "suites", "show", "my-suite")
	require.ErrorIs(t, err, constants.ErrNotFound)

	var campaign campaignShowJSON
	require.NoError(t, env.runJSON(t, &campaign, "campaigns", "show", "eval-custom"))
	assert.Equal(t, "my-suite@1.0.0", campaign.Suite)
	assert.Equal(t, uint32(2), campaign.ScenarioCount)
	env.startPrepared(t, "eval-custom", "run-custom-1")
}

func TestCampaignsCreate_DefaultsToTheDefaultSuite(t *testing.T) {
	env := setupRunEnv(t)

	var created campaignCreateJSON
	require.NoError(t, env.runJSON(t, &created, "campaigns", "create", "eval-a", "qwen3:4b"))
	assert.Equal(t, evaluation.DefaultSuiteID+"@"+evaluation.DefaultSuiteVersion, created.Suite)
	assert.Equal(t, uint32(evaluation.DefaultSuiteScenarioCount), created.ScenarioCount)
}

func TestCampaignsCreate_UnknownSuiteFailsBeforeFreezing(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "campaigns", "create", "eval-a", "qwen3:4b", "--suite", "never-created")
	require.ErrorIs(t, err, constants.ErrNotFound)

	out := env.mustRun(t, "campaigns", "list")
	assert.Contains(t, out, "No campaigns found")
}

func TestSuitesUpdate_RequiresANewVersionForChangedContent(t *testing.T) {
	env := setupRunEnv(t)
	env.mustRun(t, "suites", "create", writeSuiteFile(t, "my-suite", "1.0.0", "instruction-exact-format"))

	same := writeSuiteFile(t, "my-suite", "1.0.0", "instruction-exact-format")
	env.mustRun(t, "suites", "update", same)

	_, err := env.run(t, "suites", "update", writeSuiteFile(t, "my-suite", "1.0.0", "instruction-exact-format", "tool-select-file-read"))
	require.ErrorIs(t, err, constants.ErrEvaluationSuiteInvalid)

	env.mustRun(t, "suites", "update", writeSuiteFile(t, "my-suite", "1.1.0", "instruction-exact-format", "tool-select-file-read"))
	var show suiteShowJSON
	require.NoError(t, env.runJSON(t, &show, "suites", "show", "my-suite"))
	assert.Equal(t, "1.1.0", show.Version)
	assert.Equal(t, 2, show.ScenarioCount)
}

func TestSuites_BuiltinsAreProtectedAndBadFilesRejected(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "suites", "delete", evaluation.DefaultSuiteID)
	require.ErrorIs(t, err, constants.ErrEvaluationSuiteBuiltin)

	_, err = env.run(t, "suites", "create", writeSuiteFile(t, evaluation.SmokeSuiteID, "1.0.0", "instruction-exact-format"))
	require.ErrorIs(t, err, constants.ErrEvaluationSuiteBuiltin)

	bad := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{"schema_version":"1.0.0","id":"bad","version":"1","scenarios":[],"oops":true}`), 0o600))
	_, err = env.run(t, "suites", "create", bad)
	require.ErrorIs(t, err, constants.ErrEvaluationSuiteInvalid)

	_, err = env.run(t, "suites", "create", filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
}
