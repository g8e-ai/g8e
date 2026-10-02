// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	privateHintValue    = "AUTH_FAILURE_PRIVATE_HINT"
	privateOutputPrefix = "I could not find anything useful in the logs"
)

func fidelityScenarioContext() *PublicScenarioContext {
	return &PublicScenarioContext{
		ScenarioID: "scenario-1", ScenarioVersion: "1.1.0",
		Category:          evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT,
		PublicDescription: "Public", GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ExpectedTools:    []string{"recursive_grep_search"},
		TrajectoryPolicy: evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GUIDED,
		PromptHint: &evalv1.PromptHint{
			HintedTools: []string{"recursive_grep_search"},
			Arguments: []*evalv1.PromptHintArgument{
				{ToolName: "recursive_grep_search", ArgumentName: "pattern", Source: evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_PROMPT},
			},
		},
		Criteria: []*evalv1.PublicScenarioCriterion{{CriterionId: "criterion-1", PublicLabel: "Criterion", PublicDescription: "Public criterion"}},
	}
}

func fidelityInference(id, persona string, classification evalv1.EvaluationCallClassification, declared ...string) *evalv1.ModelInferenceRecord {
	return &evalv1.ModelInferenceRecord{
		InferenceRecordId: id, AgentPersona: persona, Classification: classification,
		ModelRole:    evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
		ModelVariant: &evalv1.ModelVariant{VariantId: "variant-1"},
		FinishReason: "stop", ToolsDeclaredReported: true, ToolsDeclared: declared,
	}
}

func fidelityResult() *evalv1.EvaluationAssignmentResult {
	return &evalv1.EvaluationAssignmentResult{
		AssignmentId: "assignment-1", RunId: "run-1",
		TrajectoryOutcome: evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_IGNORED_GUIDANCE,
		GuidedRetryCount:  1,
		FailureReason: "`recursive_grep_search` was declared to the model and named in the prompt (hint: pattern `" + privateHintValue + "` from the prompt). " +
			"Its output began: “" + privateOutputPrefix + "”.",
		PublicFailureReason: "`recursive_grep_search` was declared to the model and hinted by the prompt (arguments: pattern from the prompt). The model made no tool call.",
		ModelInferences: []*evalv1.ModelInferenceRecord{
			fidelityInference("inference-triage", "triage", evalv1.EvaluationCallClassification_EVALUATION_CALL_CLASSIFICATION_SCORED_CHAIN, "triage_only_tool"),
			fidelityInference("inference-codex", "codex", evalv1.EvaluationCallClassification_EVALUATION_CALL_CLASSIFICATION_POST_TURN, "codex_only_tool"),
			fidelityInference("inference-scored", "sage", evalv1.EvaluationCallClassification_EVALUATION_CALL_CLASSIFICATION_SCORED_CHAIN, "recursive_grep_search", "file_read_on_operator"),
		},
		ToolCallsCaptured: true,
		ToolCalls: []*evalv1.ToolCallRecord{{
			CallId: "call-1", ToolName: "recursive_grep_search",
			SchemaOutcome:   evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
			SemanticOutcome: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
			LoopTurn:        2, ErrorType: "validation.error", GuidanceShown: true,
		}},
	}
}

func buildFidelityRecord(t *testing.T) *PublicAssignmentRecord {
	t.Helper()
	assignment := &evalv1.EvaluationAssignment{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1"}
	record, err := BuildPublicAssignmentProjection(context.Background(), PublicAssignmentBuildInput{
		Assignment: assignment, Result: fidelityResult(), ScenarioContext: fidelityScenarioContext(),
	})
	require.NoError(t, err)
	return record
}

func TestPublicProjection_CarriesTrajectoryFields(t *testing.T) {
	t.Parallel()
	record := buildFidelityRecord(t)
	projection := record.Projection

	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_IGNORED_GUIDANCE, projection.GetTrajectoryOutcome())
	assert.Equal(t, uint32(1), projection.GetGuidedRetryCount())
	assert.Equal(t, fidelityResult().GetPublicFailureReason(), projection.GetFailureReason())
	assert.Equal(t, []string{"recursive_grep_search", "file_read_on_operator"}, projection.GetToolsDeclared(),
		"tools_declared comes from the scored agent call, not triage or memory")

	summary := projection.GetScenarioSummary()
	assert.Equal(t, evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GUIDED, summary.GetTrajectoryPolicy())
	require.NotNil(t, summary.GetPromptHint())
	assert.Equal(t, []string{"recursive_grep_search"}, summary.GetPromptHint().GetHintedTools())
	require.Len(t, summary.GetPromptHint().GetArguments(), 1)
	assert.Equal(t, evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_PROMPT, summary.GetPromptHint().GetArguments()[0].GetSource())
}

func TestPublicProjection_ToolCallActivityCarriesLoopTurnAndGuidance(t *testing.T) {
	t.Parallel()
	record := buildFidelityRecord(t)

	calls := record.Projection.GetActivitySummary().GetToolCalls().GetRecords()
	require.Len(t, calls, 1)
	assert.Equal(t, uint32(2), calls[0].GetLoopTurn())
	assert.Equal(t, "validation.error", calls[0].GetErrorType())
	assert.True(t, calls[0].GetGuidanceShown())
}

func TestPublicProjection_ToolsDeclaredStaysEmptyWhenNotReported(t *testing.T) {
	t.Parallel()
	result := fidelityResult()
	for _, call := range result.ModelInferences {
		call.ToolsDeclaredReported = false
	}
	projection, err := BuildAssignmentResultProjection(
		&evalv1.EvaluationAssignment{AssignmentId: "assignment-1", RunId: "run-1", ScenarioId: "scenario-1"},
		result, evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT,
		evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "unverified", CampaignRelease{})
	require.NoError(t, err)
	assert.Empty(t, projection.GetToolsDeclared())
}

func TestPublicProjection_NeverCarriesPrivateFailureText(t *testing.T) {
	t.Parallel()
	record := buildFidelityRecord(t)

	body, err := MarshalPublicAssignmentRecord(record)
	require.NoError(t, err)
	assert.NotContains(t, string(body), privateHintValue)
	assert.NotContains(t, string(body), privateOutputPrefix)
	assert.NotContains(t, string(body), "Its output began")
	assert.Contains(t, string(body), `"failure_reason"`)
	assert.Contains(t, string(body), `"guided_retry_count":1`)
}

func TestPublicProjection_ScenarioSummaryDoesNotAliasContextHint(t *testing.T) {
	t.Parallel()
	context := fidelityScenarioContext()
	summary, err := BuildPublicScenarioSummary(context)
	require.NoError(t, err)

	summary.PromptHint.HintedTools[0] = "mutated"
	assert.Equal(t, "recursive_grep_search", context.PromptHint.GetHintedTools()[0])
}
