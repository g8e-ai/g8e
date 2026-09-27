// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildScenarioCatalog_HasTwentyFiveScenariosWithExpectedCategoryCounts(t *testing.T) {
	catalog, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	require.NotNil(t, catalog)
	require.Len(t, catalog.Scenarios, 25)
	require.Len(t, artifacts, 25)

	counts := map[evalv1.EvaluationScenarioCategory]int{}
	for _, scenario := range catalog.Scenarios {
		counts[scenario.GetCategory()]++
	}
	assert.Equal(t, 4, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE])
	assert.Equal(t, 4, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION])
	assert.Equal(t, 3, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT])
	assert.Equal(t, 4, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TECHNICAL_ANALYSIS])
	assert.Equal(t, 3, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_ROUTING_DELEGATION])
	assert.Equal(t, 2, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_VERIFICATION])
	assert.Equal(t, 2, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_SECURITY_POLICY])
	assert.Equal(t, 2, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_RECOVERY])
	assert.Equal(t, 1, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_FINAL_RESPONSE])
}

func TestValidateScenarioCatalog_EnforcesCoverageConstraints(t *testing.T) {
	catalog, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	require.NoError(t, ValidateScenarioCatalog(catalog, artifacts))

	var tinyTasks, toolDecisions, governedActions, failureScenarios int
	for _, blueprint := range scenarioBlueprints() {
		if blueprint.TinyTask {
			tinyTasks++
		}
		if blueprint.RequiresToolDecision {
			toolDecisions++
		}
		if blueprint.RequiresGovernedAction {
			governedActions++
		}
		if blueprint.ExpectsFailureOrUnavailable {
			failureScenarios++
		}
	}
	assert.GreaterOrEqual(t, tinyTasks, 5)
	assert.GreaterOrEqual(t, toolDecisions, 4)
	assert.GreaterOrEqual(t, governedActions, 2)
	assert.GreaterOrEqual(t, failureScenarios, 2)
}

func TestBuildScenarioCatalog_DigestIsStableAndBound(t *testing.T) {
	first, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	second, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	assert.Equal(t, first.GetCatalogDigest(), second.GetCatalogDigest())
	assert.Len(t, first.GetCatalogDigest(), 64)
	require.NoError(t, ValidateScenarioCatalogDigest(first))
}

func TestBuildScenarioCatalog_FixtureReferencesMatchEmbeddedBodies(t *testing.T) {
	catalog, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	for _, scenario := range catalog.Scenarios {
		pair, ok := artifacts[scenario.GetScenarioId()]
		require.True(t, ok, "missing artifacts for %s", scenario.GetScenarioId())
		assert.Equal(t, scenario.GetInputFixtureRef().GetSha256(), pair.Input.Reference.GetSha256())
		assert.Equal(t, scenario.GetGoldCriteriaRef().GetSha256(), pair.Gold.Reference.GetSha256())
	}
}

func TestBuildSmokeGateScenarioCatalog_MaterializesFiveDiscriminativeScenarios(t *testing.T) {
	catalog, artifacts, err := BuildSmokeGateScenarioCatalog()
	require.NoError(t, err)
	require.NotNil(t, catalog)
	require.Len(t, catalog.Scenarios, SmokeGateScenarioCount)
	require.Len(t, artifacts, SmokeGateScenarioCount)

	require.NoError(t, ValidateSmokeGateScenarioCatalog(catalog, artifacts))
	require.NoError(t, ValidateScenarioCatalogDigest(catalog))

	seenIDs := make(map[string]bool)
	for _, sc := range catalog.Scenarios {
		seenIDs[sc.GetScenarioId()] = true
	}
	for _, expectedID := range SmokeGateScenarioIDs {
		assert.True(t, seenIDs[expectedID], "missing expected smoke scenario %s", expectedID)
	}
}

func TestHomogeneousAssignmentMatrix_FiltersBySmokeGateScenarioIDs(t *testing.T) {
	catalog, _, err := LoadScenarioCatalog()
	require.NoError(t, err)
	variant := &evalv1.ModelVariant{
		VariantId:      "granite-3-8b",
		ServedModelTag: "granite3.3:8b",
	}

	assignments, err := BuildHomogeneousAssignmentMatrix(HomogeneousScheduleRequest{
		CampaignID:  "smoke-test-campaign",
		RunID:       "smoke-test-run",
		Catalog:     catalog,
		Variants:    []*evalv1.ModelVariant{variant},
		ScenarioIDs: SmokeGateScenarioIDs,
	})
	require.NoError(t, err)
	// 5 scenarios × 1 variant × 3 roles = 15 assignments
	assert.Len(t, assignments, 15)
}
