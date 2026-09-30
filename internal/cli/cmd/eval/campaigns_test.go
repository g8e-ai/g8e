// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func threeModelRegistry() []*evalv1.ModelVariant {
	return []*evalv1.ModelVariant{
		{VariantId: "gemma4-e4b", ServedModelTag: "gemma4:e4b", ModelDigest: repeatTestHex('a', 64), ProviderClass: "ollama", ParameterCount: 4_000_000_000, ModelFamily: "Gemma"},
		{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: repeatTestHex('b', 64), ProviderClass: "ollama", ParameterCount: 4_000_000_000, ModelFamily: "Qwen"},
		{VariantId: "llama3-8b", ServedModelTag: "llama3:8b", ModelDigest: repeatTestHex('c', 64), ProviderClass: "ollama", ParameterCount: 8_000_000_000, ModelFamily: "Llama"},
	}
}

func TestCampaignsList_EmptyProject(t *testing.T) {
	env := setupRunEnv(t)

	out := env.mustRun(t, "campaigns", "list")
	assert.Contains(t, out, "No campaigns found")
}

func TestCampaignsList_AfterCreate(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	out := env.mustRun(t, "campaigns", "list")
	assert.Contains(t, out, "eval-a")
	assert.Contains(t, out, "model-role")

	var payload campaignListJSON
	require.NoError(t, env.runJSON(t, &payload, "campaigns", "list"))
	require.Len(t, payload.Campaigns, 1)
	row := payload.Campaigns[0]
	assert.Equal(t, "eval-a", row.CampaignID)
	assert.Equal(t, campaignLaneModelRole, row.Lane)
	assert.Equal(t, 1, row.ModelCount)
	assert.Equal(t, []string{"run-a-1"}, row.RunIDs)
	assert.False(t, row.Archived)
	assert.Equal(t, "scheduled", row.Status)
}

func TestCampaignsList_ShowsCampaignWithNoRuns(t *testing.T) {
	env := setupRunEnv(t)
	env.createCampaign(t, "eval-idle")

	var payload campaignListJSON
	require.NoError(t, env.runJSON(t, &payload, "campaigns", "list"))
	require.Len(t, payload.Campaigns, 1)
	assert.Equal(t, campaignStatusNoRuns, payload.Campaigns[0].Status)
	assert.Empty(t, payload.Campaigns[0].RunIDs)
}

func TestCampaignsList_StatusFilter(t *testing.T) {
	env := setupRunEnv(t)
	env.createCampaign(t, "eval-idle")
	env.prepareRun(t, "eval-a", "run-a-1")

	var payload campaignListJSON
	require.NoError(t, env.runJSON(t, &payload, "campaigns", "list", "--status", campaignStatusNoRuns))
	require.Len(t, payload.Campaigns, 1)
	assert.Equal(t, "eval-idle", payload.Campaigns[0].CampaignID)
}

func TestCampaignsShow_ReportsSpecAndRuns(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	out := env.mustRun(t, "campaigns", "show", "eval-a")
	assert.Contains(t, out, "Campaign: eval-a")
	assert.Contains(t, out, "qwen3:4b")
	assert.Contains(t, out, "run-a-1")

	var payload campaignShowJSON
	require.NoError(t, env.runJSON(t, &payload, "campaigns", "show", "eval-a"))
	assert.Equal(t, "eval-a", payload.CampaignID)
	assert.Equal(t, []string{"qwen3:4b"}, payload.Models)
	require.Len(t, payload.Runs, 1)
	assert.Equal(t, "run-a-1", payload.Runs[0].RunID)
	assert.Equal(t, uint64(37), payload.CellsPer)
}

func TestCampaignsShow_RejectsUnknownCampaign(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "campaigns", "show", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "campaigns show")
}

func TestCampaignsCreate_FromModelSelector(t *testing.T) {
	env := setupRunEnv(t)

	out := env.mustRun(t, "campaigns", "create", "eval-a", "qwen3:4b", "--reps", "2")
	assert.Contains(t, out, "Campaign created")
	assert.Contains(t, out, "g8e eval runs start eval-a")

	var payload campaignShowJSON
	require.NoError(t, env.runJSON(t, &payload, "campaigns", "show", "eval-a"))
	assert.Equal(t, uint32(2), payload.RepetitionCount)
	assert.Equal(t, []string{"qwen3:4b"}, payload.Models)
}

func TestCampaignsCreate_JSONReportsFrozenSpec(t *testing.T) {
	env := setupRunEnv(t)

	var payload campaignCreateJSON
	require.NoError(t, env.runJSON(t, &payload, "campaigns", "create", "eval-a", "--all"))
	assert.Equal(t, "eval-a", payload.CampaignID)
	assert.Equal(t, campaignLaneModelRole, payload.Lane)
	assert.Equal(t, 1, payload.ModelCount)
	assert.Equal(t, uint64(37), payload.CellsPerRun)
	assert.NotEmpty(t, payload.ModelRegistryDigest)
	assert.NotEmpty(t, payload.CatalogDigest)
}

func TestCampaignsCreate_RequiresSelectionOrFormations(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "campaigns", "create", "eval-a")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationSelectionEmpty)

	_, err = env.run(t, "campaigns")
	require.NoError(t, err)
}

func TestCampaignsCreate_RejectsUnknownModel(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "campaigns", "create", "eval-a", "absent:1b")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
}

func TestCampaignsCreate_RejectsEmptyRegistry(t *testing.T) {
	env := setupRunEnv(t)
	require.NoError(t, env.fileSvc(t).Remove(context.Background(), evaluation.DefaultModelInventoryRelPath))

	_, err := env.run(t, "campaigns", "create", "eval-a", "--all")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationSelectionEmpty)
	assert.Contains(t, err.Error(), "registry is empty")
}

func TestCampaignsCreate_RejectsConflictingModes(t *testing.T) {
	env := setupRunEnv(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "formations and all-formations", args: []string{"campaigns", "create", "eval-f", "--formations", "qwen-powerhouse", "--all-formations"}, want: "mutually exclusive"},
		{name: "formations with a model selector", args: []string{"campaigns", "create", "eval-f", "qwen3:4b", "--all-formations"}, want: "take no model selector"},
		{name: "unknown lane", args: []string{"campaigns", "create", "eval-f", "--all", "--lane", "warp"}, want: "--lane must be"},
		{name: "positional models with a filter", args: []string{"campaigns", "create", "eval-f", "qwen3:4b", "--family", "qwen"}, want: "cannot be combined"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := env.run(t, test.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestCampaignsCreate_InvalidCampaignID(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "campaigns", "create", "../escape", "--all")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestCampaignsCreate_IsIdempotentForTheSameFrozenSpec(t *testing.T) {
	env := setupRunEnv(t)

	env.mustRun(t, "campaigns", "create", "eval-a", "qwen3:4b")
	before, err := evaluation.NewStore(env.fileSvc(t)).LoadCampaignSpec(context.Background(), "eval-a")
	require.NoError(t, err)

	env.mustRun(t, "campaigns", "create", "eval-a", "qwen3:4b")
	after, err := evaluation.NewStore(env.fileSvc(t)).LoadCampaignSpec(context.Background(), "eval-a")
	require.NoError(t, err)
	assert.Equal(t, before.GetCampaignDigest(), after.GetCampaignDigest())
}

func TestCampaignsCreate_ConflictsWhenTheSpecDiffers(t *testing.T) {
	env := setupRunEnv(t)
	env.mustRun(t, "campaigns", "create", "eval-a", "qwen3:4b", "--reps", "1")

	_, err := env.run(t, "campaigns", "create", "eval-a", "qwen3:4b", "--reps", "3")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationCampaignConflict)
}

func TestCampaignsCreate_SystemLanePersistsStacks(t *testing.T) {
	env := setupRunEnv(t)
	writeTestFrozenInventory(t, env.root, evaluation.DefaultModelInventoryRelPath, threeModelRegistry()...)

	out := env.mustRun(t, "campaigns", "create", "eval-sys", "--all", "--lane", "system", "--seed", "17")
	assert.Contains(t, out, "Lane: system")
	assert.Contains(t, out, "Stacks:")

	stackSet, err := env.store(t).LoadHeterogeneousStackSet(context.Background(), "eval-sys")
	require.NoError(t, err)
	assert.NotEmpty(t, stackSet.Stacks)

	var payload campaignShowJSON
	require.NoError(t, env.runJSON(t, &payload, "campaigns", "show", "eval-sys"))
	assert.Equal(t, campaignLaneSystem, payload.Lane)
	assert.Equal(t, len(stackSet.Stacks), payload.StackCount)
}

func TestCampaignsCreate_SystemLaneSchedulesHeterogeneousAssignments(t *testing.T) {
	env := setupRunEnv(t)
	writeTestFrozenInventory(t, env.root, evaluation.DefaultModelInventoryRelPath, threeModelRegistry()...)
	env.mustRun(t, "campaigns", "create", "eval-sys", "--all", "--lane", "system", "--seed", "17")

	env.startPrepared(t, "eval-sys", "run-sys-1")

	assignments, err := env.store(t).ListAssignments(context.Background(), "run-sys-1")
	require.NoError(t, err)
	require.NotEmpty(t, assignments)
	assert.NotNil(t, assignments[0].GetHeterogeneous())
}

func TestCampaignsCreate_AllFormationsMaterializesEveryCatalogStack(t *testing.T) {
	env := setupRunEnv(t)
	writeTestFrozenInventory(t, env.root, evaluation.DefaultModelInventoryRelPath, testFormationCatalogCLIVariants()...)

	out := env.mustRun(t, "campaigns", "create", "eval-formations", "--all-formations", "--seed", "17")
	assert.Contains(t, out, "Lane: system")

	stackSet, err := env.store(t).LoadHeterogeneousStackSet(context.Background(), "eval-formations")
	require.NoError(t, err)
	assert.Len(t, stackSet.Stacks, 5)
	assert.Equal(t, evaluation.FormationCatalogStackGenerationRule, stackSet.GenerationRule)

	env.startPrepared(t, "eval-formations", "run-formations-1")
	assignments, err := env.store(t).ListAssignments(context.Background(), "run-formations-1")
	require.NoError(t, err)
	assert.Len(t, assignments, 5*evaluation.StandardScenarioCount)
}

func TestCampaignsCreate_FormationSubsetSchedulesOnlyThoseFormations(t *testing.T) {
	env := setupRunEnv(t)
	writeTestFrozenInventory(t, env.root, evaluation.DefaultModelInventoryRelPath, testFormationCatalogCLIVariants()...)

	env.mustRun(t, "campaigns", "create", "eval-two", "--formations", "qwen-powerhouse", "--formations", "ultra-efficient-speedster", "--seed", "17")

	stackSet, err := env.store(t).LoadHeterogeneousStackSet(context.Background(), "eval-two")
	require.NoError(t, err)
	assert.Len(t, stackSet.Stacks, 2)

	env.startPrepared(t, "eval-two", "run-two-1")
	assignments, err := env.store(t).ListAssignments(context.Background(), "run-two-1")
	require.NoError(t, err)
	assert.Len(t, assignments, 2*evaluation.StandardScenarioCount)
}

func TestCampaignsCreate_FormationsRequireTheirModelsInTheRegistry(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "campaigns", "create", "eval-f", "--formations", "qwen-powerhouse")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
}
