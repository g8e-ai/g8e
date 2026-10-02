// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// A trace grading cannot decode is a harness defect. Grading it as "the model
// made no tool call" would blame the model for it.
func TestGradeHomogeneousScenario_UnreadableTraceIsAHarnessFailureNotAModelFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(trace EvaluationTrace)
	}{
		{name: "tool calls that are not a list", mutate: func(trace EvaluationTrace) { trace["tool_calls"] = "not a list" }},
		{name: "a tool call with a wrongly typed field", mutate: func(trace EvaluationTrace) {
			trace["tool_calls"] = []any{EvaluationTrace{"call_id": "c1", "success": "yes"}}
		}},
		{name: "a workspace that is not an object", mutate: func(trace EvaluationTrace) {
			trace["evaluation_context"] = EvaluationTrace{"workspace": []any{1}}
		}},
		{name: "a seed that is not an object", mutate: func(trace EvaluationTrace) {
			trace["evaluation_context"] = EvaluationTrace{"seed": []any{1}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			trace := completedHomogeneousTrace(t, "primary")
			tt.mutate(trace)
			req := evidenceRequest(trace, policyFirstChoice)
			req.ScenarioTools.ExpectedTools = []string{toolGrep}

			result, err := GradeHomogeneousScenario(req)

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, constants.ErrEvaluationTraceUnreadable)
		})
	}
}

func TestGradeHeterogeneousScenario_UnreadableRoleTraceIsAHarnessFailure(t *testing.T) {
	t.Parallel()
	good := completedHomogeneousTrace(t, "lite")
	bad := completedHomogeneousTrace(t, "primary")
	bad["tool_calls"] = "not a list"

	result, err := GradeHeterogeneousScenario(HeterogeneousScenarioGradingRequest{
		AssignmentID:  "a-1",
		ScenarioID:    "evidence-test",
		ScenarioTools: ScenarioToolExpectations{TrajectoryPolicy: policyAnswer},
		RoleTraces:    []RoleTrace{{Role: FormationRoleLite, Trace: good}, {Role: FormationRolePrimary, Trace: bad}},
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, constants.ErrEvaluationTraceUnreadable)
}

// The grader's denial classification and g8ee's `deny` policy decision are one
// concept. The registry is generated from g8ee, so this fails when either side
// changes without the other.
func TestDeniedErrorTypes_MatchG8eePolicyDenySet(t *testing.T) {
	t.Parallel()
	registry, err := LoadAgentToolRegistry()
	require.NoError(t, err)

	var graderSet []string
	for errorType := range deniedErrorTypes {
		graderSet = append(graderSet, errorType)
	}
	sort.Strings(graderSet)
	exported := append([]string(nil), registry.PolicyDenyErrorTypes...)
	sort.Strings(exported)

	assert.NotEmpty(t, exported, "the registry exports g8ee's deny set")
	assert.Equal(t, exported, graderSet)
}

// An allow decision that resolves to a forbidden call contradicts "unchanged"
// even when the call record says it failed, and an empty id resolves to nothing.
func TestRequiredEvidenceGrade_StateObservationBindsOnlyByRealCallIds(t *testing.T) {
	t.Parallel()
	forbidden := []string{toolWrite}
	tests := []struct {
		name string
		call EvaluationTrace
		want evalv1.EvaluationVerdictStatus
	}{
		{name: "an allow decision bound to a failed forbidden call", call: toolCall("c1", toolWrite, false, nil), want: verdictFail},
		{name: "an allow decision whose id matches no call", call: toolCall("other", toolWrite, false, nil), want: verdictPass},
		{name: "empty ids never bind", call: toolCall("", toolWrite, false, nil), want: verdictPass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			bindingID := "c1"
			if tt.name == "empty ids never bind" {
				bindingID = ""
			}
			trace := traceWithToolCalls(t, tt.call)
			trace["governed_actions"] = []any{EvaluationTrace{"binding_id": bindingID, "transaction_id": bindingID, "policy_decision": "allow"}}
			req := evidenceRequest(trace, policyGoverned)
			req.ScenarioTools.ForbiddenTools = forbidden

			status, _, _ := evidenceGrade(t, req, "state_observation", trajectoryResult{}, true)

			assert.Equal(t, tt.want, status)
		})
	}
}

// Recovery expectations are typed gold, not a scenario id the grader knows.
func TestRequiredEvidenceGrade_RecoveryDoesNotKeyOnTheScenarioId(t *testing.T) {
	t.Parallel()
	trace := traceWithToolCalls(t, toolCall("c1", toolRead, false, nil))
	req := evidenceRequest(trace, policyGuided)
	req.ScenarioID = "recovery-tool-failure"
	req.ScenarioTools.ExpectedTools = []string{toolRead}

	status, _, _ := evidenceGrade(t, req, "recovery", trajectoryResult{}, true)

	assert.Equal(t, verdictFail, status, "without a missing-resource expectation a guided scenario needs a recovered trajectory")
}
