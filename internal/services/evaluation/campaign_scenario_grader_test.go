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

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestGradeHomogeneousScenario_InstructionExactFormatPassesWithMatchingOutput(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace("primary")
	trace["designated_role_output"] = "READY"
	digest, err := computeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	_, artifacts, err := BuildNorthStarScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["instruction-exact-format"].Gold.Body, &gold))
	result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   "assignment-1",
		ScenarioID:     "instruction-exact-format",
		DesignatedRole: "primary",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioInput: ScenarioInputFixture{
			UserPrompt: "Reply with exactly: READY",
		},
		ScenarioGold: gold,
		Trace:        trace,
		Lifecycle:    evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)
	content := findDeterministicGrade(result.DeterministicGrades, "scenario-content")
	require.NotNil(t, content)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, content.GetStatus())
	assert.NotEmpty(t, result.DecomposedScores)
}

func TestGradeHomogeneousScenario_InstructionBoundedCountFailsWithWrongWordCount(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace("lite")
	trace["designated_role_output"] = "too many words here now"
	digest, err := computeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	_, artifacts, err := BuildNorthStarScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["instruction-bounded-count"].Gold.Body, &gold))
	result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   "assignment-2",
		ScenarioID:     "instruction-bounded-count",
		DesignatedRole: "lite",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioInput: ScenarioInputFixture{
			UserPrompt: "Answer using exactly three words describing the color of the sky on a clear day.",
		},
		ScenarioGold: gold,
		Trace:        trace,
		Lifecycle:    evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)
	content := findDeterministicGrade(result.DeterministicGrades, "scenario-content")
	require.NotNil(t, content)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, content.GetStatus())
}

func TestGradeHomogeneousScenario_SemanticScenarioMarksJudgeUnavailable(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace("primary")
	_, artifacts, err := BuildNorthStarScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["final-response-diagnosis"].Gold.Body, &gold))
	result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   "assignment-3",
		ScenarioID:     "final-response-diagnosis",
		DesignatedRole: "primary",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE,
		ScenarioGold:   gold,
		Trace:          trace,
		Lifecycle:      evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)
	require.Len(t, result.SemanticGrades, 1)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE, result.SemanticGrades[0].GetStatus())
}

func findDeterministicGrade(grades []*evalv1.DeterministicGrade, criterionID string) *evalv1.DeterministicGrade {
	for _, grade := range grades {
		if grade.GetCriterionId() == criterionID {
			return grade
		}
	}
	return nil
}
