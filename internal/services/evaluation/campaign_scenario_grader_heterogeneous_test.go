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

func findDeterministicGradeByID(grades []*evalv1.DeterministicGrade, gradeID string) *evalv1.DeterministicGrade {
	for _, grade := range grades {
		if grade.GetGradeId() == gradeID {
			return grade
		}
	}
	return nil
}

// heterogeneousHandoffEvidence returns the prior_role_outputs for a given role
// based on cumulative formation execution order (Lite → Assistant → Primary).
func heterogeneousHandoffEvidence(outputs map[FormationRole]string, currentRole FormationRole) []any {
	var result []any
	if currentRole == FormationRoleLite {
		// Lite gets no prior outputs (first in chain).
		return result
	}
	// Assistant and Primary receive Lite's output.
	if liteOutput, ok := outputs[FormationRoleLite]; ok && liteOutput != "" {
		result = append(result, map[string]string{
			"role":   string(FormationRoleLite),
			"output": liteOutput,
		})
	}
	if currentRole == FormationRolePrimary {
		// Primary also receives Assistant's output.
		if assistantOutput, ok := outputs[FormationRoleAssistant]; ok && assistantOutput != "" {
			result = append(result, map[string]string{
				"role":   string(FormationRoleAssistant),
				"output": assistantOutput,
			})
		}
	}
	return result
}

func heterogeneousToolSelectionRoleTraces(t *testing.T) []RoleTrace {
	t.Helper()
	roleTraces := make([]RoleTrace, 0, 3)
	outputs := map[FormationRole]string{
		FormationRoleLite:      "Lite output",
		FormationRoleAssistant: "Assistant output",
		FormationRolePrimary:   "Primary output",
	}
	for _, role := range []FormationRole{FormationRoleLite, FormationRoleAssistant, FormationRolePrimary} {
		trace := completedHomogeneousTrace(t, string(role))
		trace["designated_role_output"] = outputs[role]
		trace["tool_decisions"] = []any{
			EvaluationTrace{
				"decision_id": "exec-" + string(role),
				"tool_name":   "recursive_grep_search",
				"selected":    true,
				"outcome":     "pass",
			},
		}
		trace["tool_calls"] = []any{
			EvaluationTrace{
				"call_id":          "exec-" + string(role),
				"tool_name":        "recursive_grep_search",
				"arguments_hash":   "a" + repeatHex('a', 63),
				"success":          true,
				"is_operator_tool": false,
			},
		}
		// Add handoff evidence: each role has prior outputs from earlier roles.
		priorOutputs := heterogeneousHandoffEvidence(outputs, role)
		if len(priorOutputs) > 0 {
			trace["prior_role_outputs"] = priorOutputs
		}
		digest, err := ComputeChatProbeTraceDigest(trace)
		require.NoError(t, err)
		trace["trace_digest"] = digest
		roleTraces = append(roleTraces, RoleTrace{Role: role, Trace: trace})
	}
	return roleTraces
}

func TestGradeHeterogeneousScenario_ToolSelectionCategoryProducesRoleAndPipelineGrades(t *testing.T) {
	t.Parallel()
	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["tool-select-grep"].Gold.Body, &gold))

	result, err := GradeHeterogeneousScenario(HeterogeneousScenarioGradingRequest{
		AssignmentID:  "assignment-hetero-1",
		ScenarioID:    "tool-select-grep",
		GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioGold:  gold,
		ScenarioTools: ScenarioToolExpectations{
			ExpectedTools:  []string{"recursive_grep_search"},
			ForbiddenTools: []string{"run_commands_with_operator"},
		},
		RoleTraces: heterogeneousToolSelectionRoleTraces(t),
		Lifecycle:  evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)

	for _, role := range []FormationRole{FormationRoleLite, FormationRoleAssistant, FormationRolePrimary} {
		traj := findDeterministicGradeByID(result.DeterministicGrades, "assignment-hetero-1:"+string(role)+":trajectory")
		require.NotNil(t, traj, "missing trajectory grade for role %s", role)
		assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, traj.GetStatus())
	}

	pipelineGrades := 0
	seenIDs := make(map[string]bool, len(result.DeterministicGrades))
	for _, grade := range result.DeterministicGrades {
		require.False(t, seenIDs[grade.GetGradeId()], "duplicate grade id %s", grade.GetGradeId())
		seenIDs[grade.GetGradeId()] = true
		assert.NotEqual(t, "homogeneous-pipeline", grade.GetCriterionId())
		if grade.GetCriterionId() == "heterogeneous-pipeline" {
			pipelineGrades++
			assert.Equal(t, "assignment-hetero-1:heterogeneous-pipeline", grade.GetGradeId())
			assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, grade.GetStatus())
		}
	}
	assert.Equal(t, 1, pipelineGrades, "expected exactly one heterogeneous-pipeline grade")
	require.NotEmpty(t, result.DecomposedScores)
	assert.Equal(t, "task_score", result.DecomposedScores[0].GetDimension())
}

func TestGradeHeterogeneousScenario_OneRoleFailsScenarioContentIndependently(t *testing.T) {
	t.Parallel()
	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["instruction-exact-format"].Gold.Body, &gold))

	outputs := map[FormationRole]string{
		FormationRoleLite:      "READY",
		FormationRoleAssistant: "READY",
		FormationRolePrimary:   "WRONG",
	}
	roleTraces := make([]RoleTrace, 0, 3)
	for _, role := range []FormationRole{FormationRoleLite, FormationRoleAssistant, FormationRolePrimary} {
		trace := completedHomogeneousTrace(t, string(role))
		trace["designated_role_output"] = outputs[role]
		// Add handoff evidence: each role has prior outputs from earlier roles.
		priorOutputs := heterogeneousHandoffEvidence(outputs, role)
		if len(priorOutputs) > 0 {
			trace["prior_role_outputs"] = priorOutputs
		}
		digest, err := ComputeChatProbeTraceDigest(trace)
		require.NoError(t, err)
		trace["trace_digest"] = digest
		roleTraces = append(roleTraces, RoleTrace{Role: role, Trace: trace})
	}

	result, err := GradeHeterogeneousScenario(HeterogeneousScenarioGradingRequest{
		AssignmentID: "assignment-hetero-2",
		ScenarioID:   "instruction-exact-format",
		ScenarioInput: ScenarioInputFixture{
			UserPrompt: "Reply with exactly: READY",
		},
		ScenarioGold: gold,
		RoleTraces:   roleTraces,
		Lifecycle:    evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)

	lite := findDeterministicGradeByID(result.DeterministicGrades, "assignment-hetero-2:lite:scenario-content")
	require.NotNil(t, lite)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, lite.GetStatus())

	assistant := findDeterministicGradeByID(result.DeterministicGrades, "assignment-hetero-2:assistant:scenario-content")
	require.NotNil(t, assistant)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, assistant.GetStatus())

	primary := findDeterministicGradeByID(result.DeterministicGrades, "assignment-hetero-2:primary:scenario-content")
	require.NotNil(t, primary)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, primary.GetStatus())

	// The pipeline-scoped grade now requires verified handoff: all roles
	// invoked AND each role received the prior role outputs.
	pipeline := findDeterministicGrade(result.DeterministicGrades, "heterogeneous-pipeline")
	require.NotNil(t, pipeline)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, pipeline.GetStatus())
	assert.Contains(t, pipeline.GetDetail(), "verified role handoff evidence")
}

func TestGradeHeterogeneousScenario_RequiresRoleTraces(t *testing.T) {
	t.Parallel()
	_, err := GradeHeterogeneousScenario(HeterogeneousScenarioGradingRequest{
		AssignmentID: "assignment-hetero-3",
		ScenarioID:   "instruction-exact-format",
	})
	require.Error(t, err)
}

func TestGradeHeterogeneousScenario_VerifiesHandoffChain(t *testing.T) {
	t.Parallel()
	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["instruction-exact-format"].Gold.Body, &gold))

	outputs := map[FormationRole]string{
		FormationRoleLite:      "LITE_RESPONSE",
		FormationRoleAssistant: "ASSISTANT_RESPONSE",
		FormationRolePrimary:   "PRIMARY_RESPONSE",
	}
	roleTraces := make([]RoleTrace, 0, 3)
	for _, role := range []FormationRole{FormationRoleLite, FormationRoleAssistant, FormationRolePrimary} {
		trace := completedHomogeneousTrace(t, string(role))
		trace["designated_role_output"] = outputs[role]
		// Add handoff evidence: each role has prior outputs from earlier roles.
		priorOutputs := heterogeneousHandoffEvidence(outputs, role)
		if len(priorOutputs) > 0 {
			trace["prior_role_outputs"] = priorOutputs
		}
		digest, err := ComputeChatProbeTraceDigest(trace)
		require.NoError(t, err)
		trace["trace_digest"] = digest
		roleTraces = append(roleTraces, RoleTrace{Role: role, Trace: trace})
	}

	result, err := GradeHeterogeneousScenario(HeterogeneousScenarioGradingRequest{
		AssignmentID: "assignment-hetero-handoff-pass",
		ScenarioID:   "instruction-exact-format",
		ScenarioInput: ScenarioInputFixture{
			UserPrompt: "Reply with exactly: PRIMARY_RESPONSE",
		},
		ScenarioGold: gold,
		RoleTraces:   roleTraces,
		Lifecycle:    evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)

	pipeline := findDeterministicGrade(result.DeterministicGrades, "heterogeneous-pipeline")
	require.NotNil(t, pipeline)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, pipeline.GetStatus())
	assert.Contains(t, pipeline.GetDetail(), "verified role handoff evidence")
}

func TestGradeHeterogeneousScenario_HandoffFailsWhenDownstreamRoleMissesPriorOutput(t *testing.T) {
	t.Parallel()
	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["instruction-exact-format"].Gold.Body, &gold))

	outputs := map[FormationRole]string{
		FormationRoleLite:      "LITE_RESPONSE",
		FormationRoleAssistant: "ASSISTANT_RESPONSE",
		FormationRolePrimary:   "PRIMARY_RESPONSE",
	}
	roleTraces := make([]RoleTrace, 0, 3)
	for _, role := range []FormationRole{FormationRoleLite, FormationRoleAssistant, FormationRolePrimary} {
		trace := completedHomogeneousTrace(t, string(role))
		trace["designated_role_output"] = outputs[role]
		// Only add handoff evidence for Lite (no prior outputs needed, which is correct).
		// Intentionally omit prior_role_outputs for Assistant and Primary (broken handoff).
		// Assistant and Primary missing prior_role_outputs = broken handoff.
		digest, err := ComputeChatProbeTraceDigest(trace)
		require.NoError(t, err)
		trace["trace_digest"] = digest
		roleTraces = append(roleTraces, RoleTrace{Role: role, Trace: trace})
	}

	result, err := GradeHeterogeneousScenario(HeterogeneousScenarioGradingRequest{
		AssignmentID: "assignment-hetero-handoff-fail",
		ScenarioID:   "instruction-exact-format",
		ScenarioInput: ScenarioInputFixture{
			UserPrompt: "Reply with exactly: PRIMARY_RESPONSE",
		},
		ScenarioGold: gold,
		RoleTraces:   roleTraces,
		Lifecycle:    evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)

	pipeline := findDeterministicGrade(result.DeterministicGrades, "heterogeneous-pipeline")
	require.NotNil(t, pipeline)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, pipeline.GetStatus())
	assert.Contains(t, pipeline.GetDetail(), "handoff chain incomplete")
}
