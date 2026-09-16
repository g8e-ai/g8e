// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestImportAssignmentResultFromTrace_CompletedHomogeneousRole(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace("primary")
	req := homogeneousAssignmentExecutionRequest("primary")
	evidence, err := BuildAssignmentTraceEvidenceReference(req.Assignment.GetRunId(), req.Assignment.GetAssignmentId(), req.AttemptID, trace, time.Unix(1_700_000_000, 0).UTC())
	require.NoError(t, err)
	result, err := ImportAssignmentResultFromTrace(req, trace, evidence, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, result.GetLifecycleStatus())
	assert.Len(t, result.GetModelInferences(), 1)
	roleGrade := findDeterministicGrade(result.GetDeterministicGrades(), "role-invoked")
	require.NotNil(t, roleGrade)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, roleGrade.GetStatus())
	require.NoError(t, ValidateAssignmentResultDigest(result))
}

func TestImportAssignmentResultFromTrace_RoleNotInvokedIsPartial(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace("primary")
	trace["role_outcome"] = "role_not_invoked"
	digest, err := computeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	req := homogeneousAssignmentExecutionRequest("primary")
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL, result.GetLifecycleStatus())
}

func homogeneousAssignmentExecutionRequest(role string) AssignmentExecutionRequest {
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId: "assignment-1",
		RunId:        "run-1",
		CampaignId:   "campaign-1",
		ScenarioId:   "instruction-exact-format",
		Lane:         evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				CandidateVariant: &evalv1.ModelVariant{
					VariantId:      "model-a",
					ProviderClass:  "ollama",
					ServedModelTag: "model-a",
					ModelDigest:    "a" + repeatHex('a', 63),
				},
				DesignatedRole: parseModelCampaignRole(role),
			},
		},
	}
	return AssignmentExecutionRequest{
		Assignment: assignment,
		AttemptID:  "attempt-1",
		ScenarioInput: ScenarioInputFixture{
			ScenarioID: "instruction-exact-format",
			UserPrompt: "Reply with exactly: NORTH-STAR-OK",
		},
		ScenarioGold:  loadScenarioGold("instruction-exact-format"),
		GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		Binding: CampaignExecutionBinding{
			InferenceOperatorSessionID: "session-1",
			DataOperatorID:             "data-op",
			DataOperatorSessionID:      "data-session",
			ModelRegistryDigest:        "d" + repeatHex('d', 63),
			ModelRegistry:              InferenceVariantsFromEvalRegistry([]*evalv1.ModelVariant{assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous).Homogeneous.GetCandidateVariant()}),
		},
	}
}

func TestImportAssignmentResultFromTrace_MaterializesToolEvidence(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace("primary")
	trace["tool_decisions"] = []any{
		map[string]any{
			"decision_id": "exec-1",
			"tool_name":   "recursive_grep_search",
			"selected":    true,
			"outcome":     "pass",
		},
	}
	trace["tool_calls"] = []any{
		map[string]any{
			"call_id":          "exec-1",
			"tool_name":        "recursive_grep_search",
			"arguments_hash":   "a" + repeatHex('a', 63),
			"success":          true,
			"is_operator_tool": true,
			"execution_id":     "exec-1",
		},
	}
	trace["governed_actions"] = []any{
		map[string]any{
			"binding_id":          "exec-1",
			"transaction_id":      "exec-1",
			"operator_id":         "operator-1",
			"operator_session_id": "session-1",
			"policy_decision":     "allow",
		},
	}
	digest, err := computeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	req := homogeneousAssignmentExecutionRequest("primary")
	req.ScenarioTools = ScenarioToolExpectations{ExpectedTools: []string{"recursive_grep_search"}}
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Now().UTC(), func(prefix string) string { return prefix })
	require.NoError(t, err)
	require.Len(t, result.GetToolDecisions(), 1)
	require.Len(t, result.GetToolCalls(), 1)
	require.Len(t, result.GetGovernedActions(), 1)
}

func completedHomogeneousTrace(role string) map[string]any {
	trace := map[string]any{
		"schema_version":    "1",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"role_outcome":      "invoked",
		"evaluation_context": map[string]any{
			"campaign_id":                "campaign-1",
			"run_id":                     "run-1",
			"assignment_id":              "assignment-1",
			"evaluation_attempt_id":      "attempt-1",
			"scenario_id":                "instruction-exact-format",
			"model_registry_digest":      "d" + repeatHex('d', 63),
			"target_operator_session_id": "session-1",
			"evaluation_lane":            "model_role",
			"designated_model_role":      role,
		},
		"controlled_role_assignment": map[string]any{
			"designated_model_role": role,
		},
		"model_calls": []any{
			map[string]any{
				"agent_role":              "sage",
				"model_role":              role,
				"provider":                "G8EProvider",
				"governed_transaction_id": "tx-1",
				"governed_result_digest":  "a" + repeatHex('a', 63),
				"provider_attempt_id":     "attempt-1",
				"normalized_request_hash": "b" + repeatHex('b', 63),
				"output_hash":             "c" + repeatHex('c', 63),
			},
		},
	}
	digest, err := computeTraceDigest(trace)
	if err != nil {
		panic(err)
	}
	trace["trace_digest"] = digest
	return trace
}
