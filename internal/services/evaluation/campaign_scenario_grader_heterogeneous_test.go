// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"strings"
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

// seedRoleHandoff records on a role trace the seed the formation runner sends
// that role: the case title, then one assistant turn per earlier role (Lite →
// Assistant → Primary) carrying that role's output.
func seedRoleHandoff(trace EvaluationTrace, outputs map[FormationRole]string, role FormationRole) {
	seed := EvaluationTrace{"case_title": "Release readiness check"}
	var turns []any
	for _, prior := range formationHandoffRoles {
		if prior == role {
			break
		}
		turns = append(turns, map[string]any{
			"sender":  "assistant",
			"content": formationHandoffTurnContent(prior, outputs[prior]),
		})
	}
	if len(turns) > 0 {
		seed["turns"] = turns
	}
	trace["evaluation_context"].(EvaluationTrace)["seed"] = seed
}

// digestedRoleTrace builds one completed role trace, lets the caller shape it,
// and binds the digest after the shaping, as g8ee does.
func digestedRoleTrace(t *testing.T, role FormationRole, output string, shape func(EvaluationTrace)) RoleTrace {
	t.Helper()
	trace := completedHomogeneousTrace(t, string(role))
	trace["designated_role_output"] = output
	if shape != nil {
		shape(trace)
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	return RoleTrace{Role: role, Trace: trace}
}

func heterogeneousRoleTraces(t *testing.T, outputs map[FormationRole]string) []RoleTrace {
	t.Helper()
	roleTraces := make([]RoleTrace, 0, len(formationHandoffRoles))
	for _, role := range formationHandoffRoles {
		roleTraces = append(roleTraces, digestedRoleTrace(t, role, outputs[role], func(trace EvaluationTrace) {
			seedRoleHandoff(trace, outputs, role)
		}))
	}
	return roleTraces
}

func heterogeneousToolSelectionRoleTraces(t *testing.T) []RoleTrace {
	t.Helper()
	outputs := map[FormationRole]string{
		FormationRoleLite:      "Lite output",
		FormationRoleAssistant: "Assistant output",
		FormationRolePrimary:   "Primary output",
	}
	roleTraces := make([]RoleTrace, 0, len(formationHandoffRoles))
	for _, role := range formationHandoffRoles {
		roleTraces = append(roleTraces, digestedRoleTrace(t, role, outputs[role], func(trace EvaluationTrace) {
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
			seedRoleHandoff(trace, outputs, role)
		}))
	}
	return roleTraces
}

func gradeHandoffForInstructionScenario(t *testing.T, assignmentID string, roleTraces []RoleTrace) *evalv1.DeterministicGrade {
	t.Helper()
	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["instruction-exact-format"].Gold.Body, &gold))
	result, err := GradeHeterogeneousScenario(HeterogeneousScenarioGradingRequest{
		AssignmentID:  assignmentID,
		ScenarioID:    "instruction-exact-format",
		ScenarioInput: ScenarioInputFixture{UserPrompt: "Reply with exactly: PRIMARY_RESPONSE"},
		ScenarioGold:  gold,
		RoleTraces:    roleTraces,
		Lifecycle:     evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)
	pipeline := findDeterministicGrade(result.DeterministicGrades, "heterogeneous-pipeline")
	require.NotNil(t, pipeline)
	return pipeline
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

	result, err := GradeHeterogeneousScenario(HeterogeneousScenarioGradingRequest{
		AssignmentID: "assignment-hetero-2",
		ScenarioID:   "instruction-exact-format",
		ScenarioInput: ScenarioInputFixture{
			UserPrompt: "Reply with exactly: READY",
		},
		ScenarioGold: gold,
		RoleTraces:   heterogeneousRoleTraces(t, outputs),
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

	// The pipeline-scoped grade requires verified handoff: all roles invoked
	// AND each role's investigation was seeded with the prior role outputs.
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

func TestGradeHeterogeneousScenario_VerifiesHandoffChainFromSeededTurns(t *testing.T) {
	t.Parallel()
	outputs := map[FormationRole]string{
		FormationRoleLite:      "LITE_RESPONSE",
		FormationRoleAssistant: "ASSISTANT_RESPONSE",
		FormationRolePrimary:   "PRIMARY_RESPONSE",
	}

	pipeline := gradeHandoffForInstructionScenario(t, "assignment-hetero-handoff-pass", heterogeneousRoleTraces(t, outputs))

	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, pipeline.GetStatus())
	assert.Contains(t, pipeline.GetDetail(), "verified role handoff evidence")
}

func TestGradeHeterogeneousScenario_VerifiesHandoffOfOutputBeyondSeedBound(t *testing.T) {
	t.Parallel()
	outputs := map[FormationRole]string{
		FormationRoleLite:      strings.Repeat("L", 20000),
		FormationRoleAssistant: "ASSISTANT_RESPONSE",
		FormationRolePrimary:   "PRIMARY_RESPONSE",
	}

	pipeline := gradeHandoffForInstructionScenario(t, "assignment-hetero-handoff-long", heterogeneousRoleTraces(t, outputs))

	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, pipeline.GetStatus(), "the seeded turn is the truncated form, which grading recomputes from the recorded output")
}

func TestGradeHeterogeneousScenario_HandoffFailsWhenDownstreamRoleMissesPriorOutput(t *testing.T) {
	t.Parallel()
	outputs := map[FormationRole]string{
		FormationRoleLite:      "LITE_RESPONSE",
		FormationRoleAssistant: "ASSISTANT_RESPONSE",
		FormationRolePrimary:   "PRIMARY_RESPONSE",
	}
	roleTraces := make([]RoleTrace, 0, len(formationHandoffRoles))
	for _, role := range formationHandoffRoles {
		// Nothing is seeded for any role: Assistant and Primary never saw the
		// earlier outputs, which is a broken handoff.
		roleTraces = append(roleTraces, digestedRoleTrace(t, role, outputs[role], nil))
	}

	pipeline := gradeHandoffForInstructionScenario(t, "assignment-hetero-handoff-fail", roleTraces)

	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, pipeline.GetStatus())
	assert.Contains(t, pipeline.GetDetail(), "handoff chain incomplete")
}

func TestGradeHeterogeneousScenario_HandoffChainRejectsMismatchedSeedTurns(t *testing.T) {
	t.Parallel()
	outputs := map[FormationRole]string{
		FormationRoleLite:      "LITE_RESPONSE",
		FormationRoleAssistant: "ASSISTANT_RESPONSE",
		FormationRolePrimary:   "PRIMARY_RESPONSE",
	}
	tests := []struct {
		name   string
		mutate func(roleTraces []RoleTrace)
	}{
		{
			name: "Assistant seeded with output Lite never produced",
			mutate: func(roleTraces []RoleTrace) {
				seed := roleTraces[1].Trace["evaluation_context"].(EvaluationTrace)["seed"].(EvaluationTrace)
				seed["turns"] = []any{map[string]any{"sender": "assistant", "content": formationHandoffTurnContent(FormationRoleLite, "FABRICATED")}}
			},
		},
		{
			name: "Primary seeded without Assistant's output",
			mutate: func(roleTraces []RoleTrace) {
				seed := roleTraces[2].Trace["evaluation_context"].(EvaluationTrace)["seed"].(EvaluationTrace)
				seed["turns"] = []any{map[string]any{"sender": "assistant", "content": formationHandoffTurnContent(FormationRoleLite, outputs[FormationRoleLite])}}
			},
		},
		{
			name: "Lite seeded with a handoff from a later role",
			mutate: func(roleTraces []RoleTrace) {
				seed := roleTraces[0].Trace["evaluation_context"].(EvaluationTrace)["seed"].(EvaluationTrace)
				seed["turns"] = []any{map[string]any{"sender": "assistant", "content": formationHandoffTurnContent(FormationRoleAssistant, outputs[FormationRoleAssistant])}}
			},
		},
		{
			name: "handoff text sent as a user turn is not a handoff",
			mutate: func(roleTraces []RoleTrace) {
				seed := roleTraces[1].Trace["evaluation_context"].(EvaluationTrace)["seed"].(EvaluationTrace)
				seed["turns"] = []any{map[string]any{"sender": "user", "content": formationHandoffTurnContent(FormationRoleLite, outputs[FormationRoleLite])}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			roleTraces := heterogeneousRoleTraces(t, outputs)
			tt.mutate(roleTraces)

			pipeline := gradeHandoffForInstructionScenario(t, "assignment-hetero-handoff-mismatch", roleTraces)

			assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, pipeline.GetStatus())
			assert.Contains(t, pipeline.GetDetail(), "handoff chain incomplete")
		})
	}
}
