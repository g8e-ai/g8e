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

func TestReadTrajectory_ToolArgGrepPattern_WrongArguments(t *testing.T) {
	t.Parallel()
	ws, err := NewScenarioWorkspace("/var/run/g8e", "run-1", "attempt-1")
	require.NoError(t, err)

	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts["tool-arg-grep-pattern"].Gold.Body, &gold))

	trace := completedHomogeneousTrace(t, "primary")
	trace["tool_calls"] = []any{
		EvaluationTrace{
			"call_id":        "exec-1",
			"tool_name":      "recursive_grep_search",
			"arguments_json": `{"pattern":"FOO","path":"` + ws.Root + `"}`,
			"success":        true,
		},
	}

	req := ScenarioGradingRequest{
		AssignmentID:   "a-1",
		ScenarioID:     "tool-arg-grep-pattern",
		DesignatedRole: "primary",
		ScenarioGold:   gold,
		ScenarioTools: ScenarioToolExpectations{
			AllowedTools:     []string{"recursive_grep_search"},
			ExpectedTools:    []string{"recursive_grep_search"},
			TrajectoryPolicy: evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GUIDED,
		},
		Trace: trace,
	}

	res := readTrajectory(req, ws)
	assert.False(t, res.Passed)
	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_WRONG_ARGUMENTS, res.Outcome)
}

func TestReadTrajectory_GuidedRetry_Recovered(t *testing.T) {
	t.Parallel()
	ws, err := NewScenarioWorkspace("/var/run/g8e", "run-1", "attempt-1")
	require.NoError(t, err)

	registry, err := LoadAgentToolRegistry()
	require.NoError(t, err)
	guidance, ok := registry.GuidanceVector("recursive_grep_search.missing_path")
	require.True(t, ok)

	trace := completedHomogeneousTrace(t, "primary")
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

	req := ScenarioGradingRequest{
		AssignmentID:   "a-guided-1",
		ScenarioID:     "recovery-error-guided-retry",
		DesignatedRole: "primary",
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
		Trace: trace,
	}

	res := readTrajectory(req, ws)
	assert.True(t, res.Passed)
	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_RECOVERED, res.Outcome)
	assert.EqualValues(t, 1, res.GuidedRetries)
}

func TestReadTrajectory_GuidedRetry_IgnoredGuidance(t *testing.T) {
	t.Parallel()
	ws, err := NewScenarioWorkspace("/var/run/g8e", "run-1", "attempt-1")
	require.NoError(t, err)

	trace := completedHomogeneousTrace(t, "primary")
	trace["tool_calls"] = []any{
		EvaluationTrace{
			"call_id":        "exec-1",
			"tool_name":      "recursive_grep_search",
			"arguments_json": `{"pattern":"AUTH_FAILURE"}`,
			"success":        false,
			"error":          "path argument is required",
			"suggestion":     "Provide path",
		},
		EvaluationTrace{
			"call_id":        "exec-2",
			"tool_name":      "recursive_grep_search",
			"arguments_json": `{"pattern":"AUTH_FAILURE"}`,
			"success":        false,
			"error":          "path argument is required",
			"suggestion":     "Provide path",
		},
	}

	req := ScenarioGradingRequest{
		AssignmentID:   "a-guided-ignored",
		ScenarioID:     "recovery-error-guided-retry",
		DesignatedRole: "primary",
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
		Trace: trace,
	}

	res := readTrajectory(req, ws)
	assert.False(t, res.Passed)
	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_IGNORED_GUIDANCE, res.Outcome)
}

func TestFailureSentences_FormatsExactSentences(t *testing.T) {
	t.Parallel()
	ws, err := NewScenarioWorkspace("/var/run/g8e", "run-1", "attempt-1")
	require.NoError(t, err)

	req := ScenarioGradingRequest{
		AssignmentID:   "a-sent",
		ScenarioID:     "tool-select-grep",
		DesignatedRole: "primary",
		ScenarioGold: ScenarioGoldCriteria{
			PromptHint: &ScenarioPromptHint{
				HintedTools: []string{"recursive_grep_search"},
				Arguments: []ScenarioHintArgument{
					{
						ToolName: "recursive_grep_search",
						Name:     "pattern",
						Source:   evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_PROMPT,
						Value:    "AUTH_FAILURE",
					},
					{
						ToolName: "recursive_grep_search",
						Name:     "path",
						Source:   evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_WORKSPACE,
						Value:    ScenarioWorkspaceToken,
					},
				},
			},
		},
		ScenarioTools: ScenarioToolExpectations{
			ExpectedTools: []string{"recursive_grep_search"},
		},
		Trace: EvaluationTrace{
			"model_calls": []any{
				EvaluationTrace{
					"agent_role":     "primary",
					"tools_declared": []any{"recursive_grep_search"},
				},
			},
			"designated_role_output": "I did not find anything.",
		},
	}

	traj := trajectoryResult{
		Outcome: evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_NO_TOOL_CALL,
		Passed:  false,
	}

	priv, pub := failureSentences(req, traj, true, "", ws)
	assert.Contains(t, priv, "`recursive_grep_search` was declared to the model and named in the prompt (hint: pattern `AUTH_FAILURE` from the prompt, path `"+ws.Root+"` from the workspace). The model made no tool call.")
	assert.Contains(t, priv, "Its output began: “I did not find anything.”.")
	assert.Contains(t, pub, "`recursive_grep_search` was declared to the model and hinted by the prompt (arguments: pattern from the prompt, path from the workspace). The model made no tool call.")
	assert.NotContains(t, pub, "Its output began")
	assert.NotContains(t, pub, "AUTH_FAILURE")
}
