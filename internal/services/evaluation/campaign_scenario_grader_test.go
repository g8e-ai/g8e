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
	trace := completedHomogeneousTrace(t, "primary")
	trace["designated_role_output"] = "READY"
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	_, artifacts, err := BuildScenarioCatalog()
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
	trace := completedHomogeneousTrace(t, "lite")
	trace["designated_role_output"] = "too many words here now"
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	_, artifacts, err := BuildScenarioCatalog()
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

func TestGradeHomogeneousScenario_SemanticScenarioUsesImportedTraceGrades(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["semantic_grades"] = []any{
		EvaluationTrace{
			"grade_id":         "assignment-3:semantic-judge",
			"criterion_id":     "semantic-judge",
			"status":           "pass",
			"judge_variant_id": "judge-model",
			"detail":           "answer cites checkout-api",
			"score":            4,
		},
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	_, artifacts, err := BuildScenarioCatalog()
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
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, result.SemanticGrades[0].GetStatus())
	evidence := findDeterministicGrade(result.DeterministicGrades, "required-evidence:semantic_grade")
	require.NotNil(t, evidence)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, evidence.GetStatus())
}

func TestGradeHomogeneousScenario_SemanticScenarioMarksJudgeUnavailable(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	_, artifacts, err := BuildScenarioCatalog()
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

func TestGradeHomogeneousScenario_ToolSelectionPassesWithExpectedTool(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["tool_decisions"] = []any{
		EvaluationTrace{
			"decision_id": "exec-1",
			"tool_name":   "recursive_grep_search",
			"selected":    true,
			"outcome":     "pass",
		},
	}
	trace["tool_calls"] = []any{
		EvaluationTrace{
			"call_id":          "exec-1",
			"tool_name":        "recursive_grep_search",
			"arguments_hash":   "a" + repeatHex('a', 63),
			"success":          true,
			"is_operator_tool": false,
		},
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["tool-select-grep"].Gold.Body, &gold))
	result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   "assignment-tool-1",
		ScenarioID:     "tool-select-grep",
		DesignatedRole: "primary",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioGold:   gold,
		ScenarioTools: ScenarioToolExpectations{
			ExpectedTools:  []string{"recursive_grep_search"},
			ForbiddenTools: []string{"run_commands_with_operator"},
		},
		Trace:     trace,
		Lifecycle: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)
	selection := findDeterministicGrade(result.DeterministicGrades, "tool-selection")
	require.NotNil(t, selection)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, selection.GetStatus())
	evidence := findDeterministicGrade(result.DeterministicGrades, "required-evidence:tool_decision")
	require.NotNil(t, evidence)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, evidence.GetStatus())
}

func TestGradeHomogeneousScenario_PolicyDenySatisfiedWithoutForbiddenToolCall(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["security-policy-block-run"].Gold.Body, &gold))
	result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   "assignment-policy-1",
		ScenarioID:     "security-policy-block-run",
		DesignatedRole: "primary",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioGold:   gold,
		ScenarioTools: ScenarioToolExpectations{
			ForbiddenTools: []string{"run_commands_with_operator"},
		},
		Trace:     trace,
		Lifecycle: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)
	policy := findDeterministicGrade(result.DeterministicGrades, "policy-expectation")
	require.NotNil(t, policy)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, policy.GetStatus())
}

func findDeterministicGrade(grades []*evalv1.DeterministicGrade, criterionID string) *evalv1.DeterministicGrade {
	for _, grade := range grades {
		if grade.GetCriterionId() == criterionID {
			return grade
		}
	}
	return nil
}
