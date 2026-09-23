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

func TestBuildPublicGradeSummarySetRemovesPrivateDetails(t *testing.T) {
	t.Parallel()
	result := &evalv1.EvaluationAssignmentResult{
		DeterministicGrades: []*evalv1.DeterministicGrade{{CriterionId: "criterion-1", Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, Detail: "private canary"}},
		SemanticGrades:      []*evalv1.SemanticGrade{{CriterionId: "criterion-2", Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE, JudgeVariantId: "judge-1", Detail: "private judge output"}},
	}
	grades, err := BuildPublicGradeSummarySet(result, &PublicScenarioContext{GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE})
	require.NoError(t, err)
	require.Len(t, grades.Deterministic, 1)
	require.Len(t, grades.Semantic, 1)
	assert.Empty(t, grades.Deterministic[0].Detail)
	assert.Equal(t, evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_CRITERION_FAILED, PublicGradeExplanation(result.GetDeterministicGrades()[0].GetStatus()))
	assert.Equal(t, evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_GRADER_UNAVAILABLE, grades.Semantic[0].GetExplanationCode())
	assert.Equal(t, "judge-1", grades.Semantic[0].GetJudgeVariantId())
}

func TestPublicGradeExplanationUsesClosedCodes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_UNSUPPORTED, PublicGradeExplanation(evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED))
	assert.Equal(t, evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_INVALID_EVIDENCE, PublicGradeExplanation(evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE))
	assert.Equal(t, evalv1.PublicGradeExplanationCode_PUBLIC_GRADE_EXPLANATION_CODE_EVIDENCE_UNAVAILABLE, PublicGradeExplanation(evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE))
}

func TestBuildToolScorecardForScenarioKeepsRequiredDimensionsUnavailable(t *testing.T) {
	t.Parallel()
	scorecard := buildToolScorecardForScenario(&evalv1.EvaluationAssignmentResult{}, &PublicScenarioContext{ToolScoreDimensions: []*evalv1.PublicToolScoreDimensionRequirement{{Dimension: evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_SELECTION, Required: true}}})
	assert.Equal(t, "source_not_captured", scorecard["tool_selection"].UnavailableReason)
	assert.Equal(t, "scenario_not_applicable", scorecard["tool_recognition"].UnavailableReason)
}

func TestToolScorecardMetricFromGradeMapsPrivateFailureDetailToClosedUnavailableReason(t *testing.T) {
	t.Parallel()
	metric := toolScorecardMetricFromGrade(&evalv1.DeterministicGrade{
		Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE,
		Detail: "compliance: evidence schema or version mismatch",
	})
	assert.Equal(t, "source_unavailable", metric.UnavailableReason)
}
