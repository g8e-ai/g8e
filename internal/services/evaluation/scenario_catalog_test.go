// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildScenarioCatalog_HasTwentySevenScenariosWithExpectedCategoryCounts(t *testing.T) {
	catalog, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	require.NotNil(t, catalog)
	require.Len(t, catalog.Scenarios, 27)
	require.Len(t, artifacts, 27)
	require.NoError(t, ValidateScenarioCatalog(catalog, artifacts))
	assert.Equal(t, uint64(41), ComputeHomogeneousMatrixSize(1))

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
	assert.Equal(t, 3, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_SECURITY_POLICY])
	assert.Equal(t, 3, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_RECOVERY])
	assert.Equal(t, 1, counts[evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_FINAL_RESPONSE])
}

func TestValidateScenarioContract_AcceptsEveryAuthoredBlueprint(t *testing.T) {
	registry, err := LoadAgentToolRegistry()
	require.NoError(t, err)
	for _, blueprint := range scenarioBlueprints() {
		assert.NoError(t, validateScenarioContract(blueprint, registry), blueprint.ScenarioID)
	}
}

func TestValidateScenarioContract_RejectsContractViolations(t *testing.T) {
	registry, err := LoadAgentToolRegistry()
	require.NoError(t, err)

	tests := []struct {
		name     string
		base     func() ScenarioBlueprint
		mutate   func(*ScenarioBlueprint)
		sentinel error
	}{
		{"unspecified trajectory policy", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.TrajectoryPolicy = evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_UNSPECIFIED
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"empty case title", toolArgGrepPattern, func(b *ScenarioBlueprint) { b.Input.Seed.CaseTitle = "  " }, constants.ErrEvaluationScenarioContractInvalid},
		{"case title names the evaluation", toolArgGrepPattern, func(b *ScenarioBlueprint) { b.Input.Seed.CaseTitle = "Eval run 3" }, constants.ErrEvaluationScenarioContractInvalid},
		{"case title carries a template token", toolArgGrepPattern, func(b *ScenarioBlueprint) { b.Input.Seed.CaseTitle = "Sweep {{workspace}}" }, constants.ErrEvaluationScenarioContractInvalid},
		{"allowed tool not in registry", toolArgGrepPattern, func(b *ScenarioBlueprint) { b.AllowedTools = append(b.AllowedTools, "no_such_tool") }, constants.ErrEvaluationScenarioContractInvalid},
		{"expected tool not allowed", toolArgGrepPattern, func(b *ScenarioBlueprint) { b.AllowedTools = []string{"file_read_on_operator"} }, constants.ErrEvaluationScenarioContractInvalid},
		{"tool both allowed and forbidden", toolArgGrepPattern, func(b *ScenarioBlueprint) { b.ForbiddenTools = []string{"recursive_grep_search"} }, constants.ErrEvaluationScenarioContractInvalid},
		{"ANSWER scenario declares expected tools", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.TrajectoryPolicy = evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_ANSWER
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"GUIDED scenario has no prompt hint", toolArgGrepPattern, func(b *ScenarioBlueprint) { b.Gold.PromptHint = nil }, constants.ErrEvaluationScenarioContractInvalid},
		{"hinted tool is not an expected tool", toolSelectFileRead, func(b *ScenarioBlueprint) {
			b.Gold.PromptHint.HintedTools = []string{"recursive_grep_search"}
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"GOVERNED scenario has no forbidden tools", securityPolicyBlockRun, func(b *ScenarioBlueprint) { b.ForbiddenTools = nil }, constants.ErrEvaluationScenarioContractInvalid},
		{"deterministic scenario has no content check", techNetworkSummary, func(b *ScenarioBlueprint) { b.Gold.ContentCheck = nil }, constants.ErrEvaluationScenarioContractInvalid},
		{"validator constrains unknown argument", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.ArgumentValidators[0].Arguments[0].Name = "no_such_argument"
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"regex constraint has no reject samples", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.ArgumentValidators[0].Arguments[0].RegexRejects = nil
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"required argument has no hint source", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.PromptHint.Arguments = b.Gold.PromptHint.Arguments[:2]
		}, constants.ErrEvaluationPromptUnanswerable},
		{"required argument has two hint sources", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.PromptHint.Arguments = append(b.Gold.PromptHint.Arguments, hintArg("recursive_grep_search", "pattern", sourcePrompt, "AUTH_FAILURE"))
		}, constants.ErrEvaluationPromptUnanswerable},
		{"prompt source value missing from prompt", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Input.UserPrompt = "Search " + ScenarioWorkspaceToken + " for the failures."
		}, constants.ErrEvaluationPromptUnanswerable},
		{"seed source value missing from seed", recoveryErrorGuidedRetry, func(b *ScenarioBlueprint) {
			b.Gold.PromptHint.Arguments[0].Value = "PAYMENT_TIMEOUT"
		}, constants.ErrEvaluationPromptUnanswerable},
		{"workspace source but prompt never names the workspace", toolArgGrepPattern, func(b *ScenarioBlueprint) { b.Input.UserPrompt = "Search for the exact pattern AUTH_FAILURE." }, constants.ErrEvaluationPromptUnanswerable},
		{"workspace source value outside the workspace", toolSelectFileRead, func(b *ScenarioBlueprint) {
			b.Gold.PromptHint.Arguments[0].Value = "/etc/retry-config.env"
		}, constants.ErrEvaluationPromptUnanswerable},
		{"operator context source on the wrong argument", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.PromptHint.Arguments[1].Source = sourceOperatorContext
		}, constants.ErrEvaluationPromptUnanswerable},
		{"model authored source on the wrong argument", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.PromptHint.Arguments[0].Source = sourceModelAuthored
		}, constants.ErrEvaluationPromptUnanswerable},
		{"workspace file escapes the workspace", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Input.WorkspaceFiles[0].RelPath = "../escape.log"
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"duplicate workspace file", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Input.WorkspaceFiles = append(b.Input.WorkspaceFiles, b.Input.WorkspaceFiles[0])
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"workspace has only decoy files", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Input.WorkspaceFiles = b.Input.WorkspaceFiles[1:]
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"retired handoff evidence type", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.RequiredEvidenceTypes = append(b.Gold.RequiredEvidenceTypes, "handoff")
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"triage expectation grades no label", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.Players = &ScenarioPlayerExpectations{Triage: &TriageExpectation{}}
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"triage complexity g8ee never emits", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.Players = &ScenarioPlayerExpectations{Triage: &TriageExpectation{Complexity: []constants.TriageComplexity{"medium"}}}
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"triage intent g8ee never emits", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.Players = &ScenarioPlayerExpectations{Triage: &TriageExpectation{Intent: []constants.TriageIntent{"request"}}}
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"triage posture g8ee never emits", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.Players = &ScenarioPlayerExpectations{Triage: &TriageExpectation{Posture: []constants.TriagePosture{"angry"}}}
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"command expectation has no constraint", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.Players = &ScenarioPlayerExpectations{Command: &ScenarioContentCheck{}}
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"marshal expectation accepts no risk level", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.Players = &ScenarioPlayerExpectations{Marshal: &MarshalExpectation{}}
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"marshal risk level g8ee never emits", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.Players = &ScenarioPlayerExpectations{Marshal: &MarshalExpectation{Risk: []MarshalRisk{"CRITICAL"}}}
		}, constants.ErrEvaluationScenarioContractInvalid},
		{"codex expectation has no constraint", toolArgGrepPattern, func(b *ScenarioBlueprint) {
			b.Gold.Players = &ScenarioPlayerExpectations{Codex: &ScenarioContentCheck{}}
		}, constants.ErrEvaluationScenarioContractInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blueprint := tt.base()
			require.NoError(t, validateScenarioContract(blueprint, registry), "base blueprint must be valid")
			tt.mutate(&blueprint)
			err := validateScenarioContract(blueprint, registry)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.sentinel)
			assert.Contains(t, err.Error(), blueprint.ScenarioID)
		})
	}
}

func TestBuildScenarioCatalog_RejectsABlueprintThatBreaksTheContract(t *testing.T) {
	registry, err := LoadAgentToolRegistry()
	require.NoError(t, err)
	blueprint := toolArgGrepPattern()
	blueprint.Gold.PromptHint = nil

	_, _, err = materializeScenarioBlueprint(DefaultSuiteID, blueprint, registry)

	require.ErrorIs(t, err, constants.ErrEvaluationScenarioContractInvalid)
}

// TestHintLint_RejectsCatalog100GrepPrompt reproduces finding E3: the 1.0.0
// prompt named no path although recursive_grep_search requires one, so the
// task was not answerable. The 1.1.0 prompt names the workspace and passes.
func TestHintLint_RejectsCatalog100GrepPrompt(t *testing.T) {
	registry, err := LoadAgentToolRegistry()
	require.NoError(t, err)

	blueprint := toolArgGrepPattern()
	blueprint.Input.UserPrompt = "Search the synthetic workspace for the exact pattern AUTH_FAILURE using recursive grep."
	err = validateScenarioContract(blueprint, registry)
	require.ErrorIs(t, err, constants.ErrEvaluationPromptUnanswerable)
	assert.Contains(t, err.Error(), "path")

	require.NoError(t, validateScenarioContract(toolArgGrepPattern(), registry))
}

func TestBuildScenarioCatalog_SeedsCarryRegistryGuidanceVerbatim(t *testing.T) {
	registry, err := LoadAgentToolRegistry()
	require.NoError(t, err)
	catalog, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	require.NotNil(t, catalog)

	vector, ok := registry.GuidanceVector("recursive_grep_search.missing_path")
	require.True(t, ok)
	var input ScenarioInputFixture
	require.NoError(t, json.Unmarshal(artifacts["recovery-error-guided-retry"].Input.Body, &input))

	require.Len(t, input.Seed.HistoryEvents, 1)
	event := input.Seed.HistoryEvents[0]
	assert.Equal(t, vector.Error, event.Error)
	assert.Equal(t, vector.ToolName, event.ToolName)
	assert.Equal(t, vector.ArgumentsJSON, event.ArgumentsJSON)
	assert.Equal(t, vector.ErrorType, event.ErrorType)
	assert.Equal(t, vector.ExecutionID, event.ExecutionID)
	assert.Empty(t, event.GuidanceVectorID)

	require.Len(t, input.Seed.Turns, 2)
	assert.Contains(t, input.Seed.Turns[1].Content, "Tool result:\n"+vector.Error)
	assert.Empty(t, input.Seed.Turns[1].GuidanceVectorID)
}

func TestBuildScenarioCatalog_RejectsUnknownGuidanceVector(t *testing.T) {
	registry, err := LoadAgentToolRegistry()
	require.NoError(t, err)
	blueprint := recoveryErrorGuidedRetry()
	blueprint.Input.Seed.HistoryEvents[0].GuidanceVectorID = "no_such_tool.no_such_vector"

	_, _, err = materializeScenarioBlueprint(DefaultSuiteID, blueprint, registry)

	require.ErrorIs(t, err, constants.ErrEvaluationScenarioContractInvalid)
}

func TestBuildScenarioCatalog_PublicHintNeverCarriesValues(t *testing.T) {
	catalog, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	byID := make(map[string]*evalv1.EvaluationScenarioDefinition, len(catalog.Scenarios))
	for _, scenario := range catalog.Scenarios {
		byID[scenario.GetScenarioId()] = scenario
	}

	hinted := 0
	for _, blueprint := range scenarioBlueprints() {
		scenario := byID[blueprint.ScenarioID]
		require.NotNil(t, scenario, blueprint.ScenarioID)
		assert.Equal(t, blueprint.TrajectoryPolicy, scenario.GetTrajectoryPolicy(), blueprint.ScenarioID)
		private := blueprint.Gold.PromptHint
		if private == nil {
			assert.Nil(t, scenario.GetPromptHint(), blueprint.ScenarioID)
			continue
		}
		hinted++
		public := scenario.GetPromptHint()
		require.NotNil(t, public, blueprint.ScenarioID)
		assert.Equal(t, private.HintedTools, public.GetHintedTools())
		require.Len(t, public.GetArguments(), len(private.Arguments))
		body, err := protojson.Marshal(scenario)
		require.NoError(t, err)
		for index, argument := range private.Arguments {
			assert.Equal(t, argument.Name, public.GetArguments()[index].GetArgumentName())
			assert.Equal(t, argument.Source, public.GetArguments()[index].GetSource())
			if argument.Value != "" {
				assert.NotContains(t, string(body), argument.Value, "scenario %s leaks hint value for %s", blueprint.ScenarioID, argument.Name)
			}
		}
	}
	assert.Equal(t, 9, hinted)
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
	// instruction-exact-format and tech-error-diagnosis are Lite-only; the
	// three tool/policy/recovery scenarios are Primary and Assistant.
	assert.Len(t, assignments, 8)
}
