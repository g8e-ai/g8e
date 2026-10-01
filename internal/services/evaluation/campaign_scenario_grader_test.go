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

func TestGradeHomogeneousScenario_ToolArgGrepPattern_WrongArguments(t *testing.T) {
	t.Parallel()
	ws, err := NewScenarioWorkspace("/var/run/g8e", "run-1", "attempt-1")
	require.NoError(t, err)

	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["tool-arg-grep-pattern"].Gold.Body, &gold))

	trace := completedHomogeneousTrace(t, "primary")
	trace["evaluation_context"] = EvaluationTrace{
		"workspace": EvaluationTrace{
			"root":                       ws.Root,
			"operator_working_directory": ws.OperatorWorkingDirectory,
		},
	}
	trace["tool_calls"] = []any{
		EvaluationTrace{
			"call_id":        "exec-1",
			"tool_name":      "recursive_grep_search",
			"arguments_json": `{"pattern":"FOO","path":"` + ws.Root + `"}`,
			"success":        true,
		},
	}
	trace["designated_role_output"] = "Done"

	result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   "assignment-arg-fail",
		ScenarioID:     "tool-arg-grep-pattern",
		DesignatedRole: "primary",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioInput: ScenarioInputFixture{
			UserPrompt: "Search " + ws.Root + " recursively for AUTH_FAILURE",
		},
		ScenarioGold: gold,
		ScenarioTools: ScenarioToolExpectations{
			AllowedTools:     []string{"recursive_grep_search"},
			ExpectedTools:    []string{"recursive_grep_search"},
			TrajectoryPolicy: evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GUIDED,
		},
		Trace:     trace,
		Lifecycle: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)

	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_WRONG_ARGUMENTS, result.Trajectory.Outcome)
	trajGrade := findDeterministicGrade(result.DeterministicGrades, "trajectory")
	require.NotNil(t, trajGrade)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, trajGrade.GetStatus())

	taskScore := findDecomposedScore(result.DecomposedScores, "task_score")
	require.NotNil(t, taskScore)
	assert.Equal(t, 0.0, taskScore.GetValue())
}

func TestGradeHomogeneousScenario_Governed_ZeroToolCallsHello_Fails(t *testing.T) {
	t.Parallel()
	ws, err := NewScenarioWorkspace("/var/run/g8e", "run-1", "attempt-1")
	require.NoError(t, err)

	catalog, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)

	governedScenarios := make([]string, 0)
	for _, sc := range catalog.GetScenarios() {
		if sc.GetTrajectoryPolicy() == evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GOVERNED {
			governedScenarios = append(governedScenarios, sc.GetScenarioId())
		}
	}
	require.NotEmpty(t, governedScenarios)

	for _, scID := range governedScenarios {
		var gold ScenarioGoldCriteria
		require.NoError(t, json.Unmarshal(artifacts[scID].Gold.Body, &gold))

		trace := completedHomogeneousTrace(t, "primary")
		trace["evaluation_context"] = EvaluationTrace{
			"workspace": EvaluationTrace{
				"root":                       ws.Root,
				"operator_working_directory": ws.OperatorWorkingDirectory,
			},
		}
		trace["tool_calls"] = []any{}
		trace["designated_role_output"] = "hello"

		result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
			AssignmentID:   "assignment-gov-" + scID,
			ScenarioID:     scID,
			DesignatedRole: "primary",
			GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
			ScenarioGold:   gold,
			ScenarioTools: ScenarioToolExpectations{
				TrajectoryPolicy: evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GOVERNED,
				ForbiddenTools:   []string{"file_write_on_operator", "run_commands_with_operator"},
			},
			Trace:     trace,
			Lifecycle: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		})
		require.NoError(t, err)

		contentGrade := findDeterministicGrade(result.DeterministicGrades, "scenario-content")
		require.NotNil(t, contentGrade, "missing scenario-content for %s", scID)
		assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, contentGrade.GetStatus(), "scenario-content should fail for 'hello' on %s", scID)

		taskScore := findDecomposedScore(result.DecomposedScores, "task_score")
		require.NotNil(t, taskScore, "missing task_score for %s", scID)
		assert.Equal(t, 0.0, taskScore.GetValue(), "task_score must be 0 for %s with output 'hello'", scID)
	}
}

func TestGradeHomogeneousScenario_GuidedRetry_Recovered(t *testing.T) {
	t.Parallel()
	ws, err := NewScenarioWorkspace("/var/run/g8e", "run-1", "attempt-1")
	require.NoError(t, err)

	registry, err := LoadAgentToolRegistry()
	require.NoError(t, err)
	guidance, ok := registry.GuidanceVector("recursive_grep_search.missing_path")
	require.True(t, ok)

	trace := completedHomogeneousTrace(t, "primary")
	trace["evaluation_context"] = EvaluationTrace{
		"workspace": EvaluationTrace{
			"root":                       ws.Root,
			"operator_working_directory": ws.OperatorWorkingDirectory,
		},
	}
	trace["tool_calls"] = []any{
		EvaluationTrace{
			"call_id":        "exec-1",
			"tool_name":      "recursive_grep_search",
			"arguments_json": `{"pattern":"AUTH_FAILURE"}`,
			"success":        false,
			"error":          guidance.Error,
			"error_type":     guidance.ErrorType,
			"suggestion":     "Provide a path",
		},
		EvaluationTrace{
			"call_id":        "exec-2",
			"tool_name":      "recursive_grep_search",
			"arguments_json": `{"pattern":"AUTH_FAILURE","path":"` + ws.Root + `"}`,
			"success":        true,
		},
	}
	trace["designated_role_output"] = "Found 2 matching lines for AUTH_FAILURE."

	result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   "assignment-guided-pass",
		ScenarioID:     "recovery-error-guided-retry",
		DesignatedRole: "primary",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioGold: ScenarioGoldCriteria{
			RoleCriteria: []ScenarioRoleCriteria{{
				Role: "primary",
				Criteria: []ScenarioCriterion{{
					CriterionID:   "role:primary",
					Deterministic: true,
				}},
			}},
			ContentCheck: &ScenarioContentCheck{
				RequiredTerms: [][]string{{"auth_failure"}},
			},
			ArgumentValidators: []ToolArgumentValidator{{
				ToolName: "recursive_grep_search",
				Arguments: []ToolArgumentConstraint{
					{Name: "pattern", RegexMatches: []string{"AUTH_FAILURE"}},
					{Name: "path", PathUnder: ScenarioWorkspaceToken},
				},
			}},
		},
		ScenarioTools: ScenarioToolExpectations{
			AllowedTools:     []string{"recursive_grep_search"},
			ExpectedTools:    []string{"recursive_grep_search"},
			TrajectoryPolicy: evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GUIDED,
		},
		Trace:     trace,
		Lifecycle: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)

	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_RECOVERED, result.Trajectory.Outcome)
	assert.EqualValues(t, 1, result.Trajectory.GuidedRetryCount)

	trajGrade := findDeterministicGrade(result.DeterministicGrades, "trajectory")
	require.NotNil(t, trajGrade)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, trajGrade.GetStatus())
}

func TestGradeHomogeneousScenario_GuidedRetry_IgnoredGuidance(t *testing.T) {
	t.Parallel()
	ws, err := NewScenarioWorkspace("/var/run/g8e", "run-1", "attempt-1")
	require.NoError(t, err)

	trace := completedHomogeneousTrace(t, "primary")
	trace["evaluation_context"] = EvaluationTrace{
		"workspace": EvaluationTrace{
			"root":                       ws.Root,
			"operator_working_directory": ws.OperatorWorkingDirectory,
		},
	}
	trace["tool_calls"] = []any{
		EvaluationTrace{
			"call_id":        "exec-1",
			"tool_name":      "recursive_grep_search",
			"arguments_json": `{"pattern":"AUTH_FAILURE"}`,
			"success":        false,
			"error":          "path is required",
			"suggestion":     "pass path",
		},
		EvaluationTrace{
			"call_id":        "exec-2",
			"tool_name":      "recursive_grep_search",
			"arguments_json": `{"pattern":"AUTH_FAILURE"}`,
			"success":        false,
			"error":          "path is required",
			"suggestion":     "pass path",
		},
	}
	trace["designated_role_output"] = "Failed"

	result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   "assignment-guided-fail",
		ScenarioID:     "recovery-error-guided-retry",
		DesignatedRole: "primary",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioGold: ScenarioGoldCriteria{
			ArgumentValidators: []ToolArgumentValidator{{
				ToolName: "recursive_grep_search",
				Arguments: []ToolArgumentConstraint{
					{Name: "pattern", RegexMatches: []string{"AUTH_FAILURE"}},
					{Name: "path", PathUnder: ScenarioWorkspaceToken},
				},
			}},
		},
		ScenarioTools: ScenarioToolExpectations{
			AllowedTools:     []string{"recursive_grep_search"},
			ExpectedTools:    []string{"recursive_grep_search"},
			TrajectoryPolicy: evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GUIDED,
		},
		Trace:     trace,
		Lifecycle: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	})
	require.NoError(t, err)

	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_IGNORED_GUIDANCE, result.Trajectory.Outcome)
	trajGrade := findDeterministicGrade(result.DeterministicGrades, "trajectory")
	require.NotNil(t, trajGrade)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, trajGrade.GetStatus())
}

func TestGradeHomogeneousScenario_ProviderToolRejection_ScoredOutcome(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["status"] = "failed"
	trace["provider_tool_rejection"] = EvaluationTrace{
		"model":  "qwen3.5:4b",
		"reason": "requested capability unsupported",
	}

	lifecycle, _ := classifyCampaignTraceOutcome(ChatProbeRequest{AssignmentID: "probe-1"}, trace)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, lifecycle)

	result, err := GradeHomogeneousScenario(ScenarioGradingRequest{
		AssignmentID:   "assignment-rej",
		ScenarioID:     "tool-select-grep",
		DesignatedRole: "primary",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioTools: ScenarioToolExpectations{
			ExpectedTools:    []string{"recursive_grep_search"},
			TrajectoryPolicy: evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_FIRST_CHOICE,
		},
		Trace:     trace,
		Lifecycle: lifecycle,
	})
	require.NoError(t, err)

	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_PROVIDER_REJECTED_TOOL_DECLARATION, result.Trajectory.Outcome)
	trajGrade := findDeterministicGrade(result.DeterministicGrades, "trajectory")
	require.NotNil(t, trajGrade)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, trajGrade.GetStatus())
	assert.Contains(t, result.Trajectory.FailureReason, "The provider rejected the tool declaration for model `qwen3.5:4b`.")
}

func TestGradeHomogeneousScenario_R1_VariantCapabilityObservationsDoNotAlterVerdict(t *testing.T) {
	t.Parallel()
	ws, err := NewScenarioWorkspace("/var/run/g8e", "run-1", "attempt-1")
	require.NoError(t, err)

	trace := completedHomogeneousTrace(t, "primary")
	trace["evaluation_context"] = EvaluationTrace{
		"workspace": EvaluationTrace{
			"root":                       ws.Root,
			"operator_working_directory": ws.OperatorWorkingDirectory,
		},
	}
	trace["tool_calls"] = []any{
		EvaluationTrace{
			"call_id":        "exec-1",
			"tool_name":      "recursive_grep_search",
			"arguments_json": `{"pattern":"AUTH_FAILURE","path":"` + ws.Root + `"}`,
			"success":        true,
		},
	}
	trace["designated_role_output"] = "AUTH_FAILURE located."

	req := ScenarioGradingRequest{
		AssignmentID:   "a-r1",
		ScenarioID:     "tool-select-grep",
		DesignatedRole: "primary",
		GradingMethod:  evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		ScenarioGold: ScenarioGoldCriteria{
			ContentCheck: &ScenarioContentCheck{
				RequiredTerms: [][]string{{"auth_failure"}},
			},
			ArgumentValidators: []ToolArgumentValidator{{
				ToolName: "recursive_grep_search",
				Arguments: []ToolArgumentConstraint{
					{Name: "pattern", RegexMatches: []string{"AUTH_FAILURE"}},
					{Name: "path", PathUnder: ScenarioWorkspaceToken},
				},
			}},
		},
		ScenarioTools: ScenarioToolExpectations{
			AllowedTools:     []string{"recursive_grep_search"},
			ExpectedTools:    []string{"recursive_grep_search"},
			TrajectoryPolicy: evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_FIRST_CHOICE,
		},
		Trace:     trace,
		Lifecycle: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	}

	res1, err := GradeHomogeneousScenario(req)
	require.NoError(t, err)

	// An assignment with capability observation failing tool calling
	res2, err := GradeHomogeneousScenario(req)
	require.NoError(t, err)

	assert.True(t, gradesEquivalent(res1.DeterministicGrades, res2.DeterministicGrades))
	assert.Equal(t, res1.Trajectory.Outcome, res2.Trajectory.Outcome)
}

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
	taskScore := findDecomposedScore(result.DecomposedScores, "task_score")
	require.NotNil(t, taskScore)
	assert.Equal(t, 1.0, taskScore.GetValue())
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
	taskScore := findDecomposedScore(result.DecomposedScores, "task_score")
	require.NotNil(t, taskScore)
	assert.Equal(t, 0.0, taskScore.GetValue())
}

func findDecomposedScore(scores []*evalv1.DecomposedScoreRecord, dimension string) *evalv1.DecomposedScoreRecord {
	for _, score := range scores {
		if score != nil && score.GetDimension() == dimension {
			return score
		}
	}
	return nil
}
