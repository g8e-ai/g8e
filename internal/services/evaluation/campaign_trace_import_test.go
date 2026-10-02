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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestImportAssignmentResultFromTrace_CompletedHomogeneousRole(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	req := homogeneousAssignmentExecutionRequest(t, "primary")
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
	trace := completedHomogeneousTrace(t, "primary")
	trace["role_outcome"] = "role_not_invoked"
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL, result.GetLifecycleStatus())
}

func TestImportAssignmentResultFromTrace_FailedRoleNotInvokedWithoutModelCallsIsProviderFailed(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["status"] = "failed"
	trace["role_outcome"] = "role_not_invoked"
	trace["model_calls"] = []any{}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED, result.GetLifecycleStatus())
}

func TestImportAssignmentResultFromTrace_FailedRoleNotInvokedWithModelCallsIsPartial(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["status"] = "failed"
	trace["role_outcome"] = "role_not_invoked"
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL, result.GetLifecycleStatus())
}

// A trace whose digest does not hold is rejected at import whatever outcome it
// reports, not scored as a failed assignment and not skipped as one that never
// reached validation.
func TestImportAssignmentResultFromTrace_RejectsDigestMismatchWhateverTheOutcome(t *testing.T) {
	t.Parallel()
	cases := map[string]func(EvaluationTrace){
		"completed":              func(EvaluationTrace) {},
		"role not invoked":       func(trace EvaluationTrace) { trace["role_outcome"] = "role_not_invoked" },
		"failed before any call": func(trace EvaluationTrace) { trace["status"] = "failed"; trace["model_calls"] = []any{} },
		"provider tool rejected": func(trace EvaluationTrace) { trace["status"] = "failed"; trace["provider_tool_rejection"] = "rejected" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			trace := completedHomogeneousTrace(t, "primary")
			mutate(trace)
			digest, err := ComputeChatProbeTraceDigest(trace)
			require.NoError(t, err)
			trace["trace_digest"] = digest
			trace["chat_execution_id"] = "rewritten-after-the-digest"
			req := homogeneousAssignmentExecutionRequest(t, "primary")

			result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })

			require.ErrorIs(t, err, constants.ErrEvaluationTraceDigestMismatch)
			assert.Nil(t, result)
		})
	}
}

func homogeneousAssignmentExecutionRequest(t *testing.T, role string) AssignmentExecutionRequest {
	t.Helper()
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
			UserPrompt: "Reply with exactly: SUITE-OK",
		},
		ScenarioGold:  loadScenarioGold(t, "instruction-exact-format"),
		GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		Binding: CampaignExecutionBinding{
			InferenceOperatorSessionID:   "session-1",
			DataOperatorID:               "data-op",
			DataOperatorSessionID:        "data-session",
			DataOperatorWorkingDirectory: "/home/operator",
			ModelRegistryDigest:          "d" + repeatHex('d', 63),
			ModelRegistry:                InferenceVariantsFromEvalRegistry([]*evalv1.ModelVariant{assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous).Homogeneous.GetCandidateVariant()}),
		},
	}
}

func TestImportAssignmentResultFromTrace_MaterializesToolEvidence(t *testing.T) {
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
			"is_operator_tool": true,
			"execution_id":     "exec-1",
		},
	}
	trace["governed_actions"] = []any{
		EvaluationTrace{
			"binding_id":          "exec-1",
			"transaction_id":      "exec-1",
			"operator_id":         "operator-1",
			"operator_session_id": "session-1",
			"policy_decision":     "allow",
		},
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	req.ScenarioTools = ScenarioToolExpectations{ExpectedTools: []string{"recursive_grep_search"}}
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Now().UTC(), func(prefix string) string { return prefix })
	require.NoError(t, err)
	require.Len(t, result.GetToolDecisions(), 1)
	require.Len(t, result.GetToolCalls(), 1)
	require.Len(t, result.GetGovernedActions(), 1)
}

func TestImportAssignmentResultFromTrace_MapsTelemetryAndPolicyPresence(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["schema_version"] = "2"
	call := trace["model_calls"].([]any)[0].(EvaluationTrace)
	call["usage_reported"] = true
	call["input_tokens"] = float64(0)
	call["output_tokens"] = float64(12)
	call["thinking_tokens"] = float64(0)
	call["cache_tokens"] = float64(3)
	call["retry_count"] = float64(0)
	call["finish_reason"] = "stop"
	call["governed_output_hash"] = "d" + repeatHex('d', 63)
	call["monotonic_start"] = 10.25
	call["monotonic_end"] = 11.5
	trace["policy_decisions"] = []any{EvaluationTrace{
		"decision_id": "policy-1",
		"tool_name":   "",
		"outcome":     "deny",
		"detail":      "application-reported refusal",
	}}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest

	result, err := ImportAssignmentResultFromTrace(homogeneousAssignmentExecutionRequest(t, "primary"), trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	inference := result.GetModelInferences()[0]
	assert.Equal(t, evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED, inference.GetUsageAvailability())
	assert.Equal(t, uint32(0), inference.GetPromptTokens())
	assert.Equal(t, uint32(12), inference.GetCompletionTokens())
	require.NotNil(t, inference.ThinkingTokens)
	assert.Equal(t, uint32(0), inference.GetThinkingTokens())
	require.NotNil(t, inference.CacheTokens)
	assert.Equal(t, uint32(3), inference.GetCacheTokens())
	require.NotNil(t, inference.RetryCount)
	assert.Equal(t, uint32(0), inference.GetRetryCount())
	assert.Equal(t, "d"+repeatHex('d', 63), inference.GetOutputHash())
	assert.Equal(t, uint64(1_250_000_000), result.GetScoredInferenceSpanNanos())
	assert.True(t, result.GetPolicyDecisionsCaptured())
	require.Len(t, result.GetPolicyDecisions(), 1)
	assert.Equal(t, evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_DENY, result.GetPolicyDecisions()[0].GetOutcome())
	assert.Empty(t, result.GetPolicyDecisions()[0].GetToolName())
}

func TestImportAssignmentResultFromTrace_RejectsUnknownPolicyOutcome(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["policy_decisions"] = []any{EvaluationTrace{
		"decision_id": "policy-1",
		"tool_name":   "read_file",
		"outcome":     "maybe",
		"detail":      "bad",
	}}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	_, err = ImportAssignmentResultFromTrace(homogeneousAssignmentExecutionRequest(t, "primary"), trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown outcome")
}

func TestDurationSecondsToNanosChecked_RoundsProviderTelemetryFloats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		seconds float64
		want    uint64
	}{
		{name: "load duration", seconds: 0.0068479, want: 6_847_900},
		{name: "generation duration", seconds: 0.956517, want: 956_517_000},
		{name: "total duration", seconds: 1.043945, want: 1_043_945_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := durationSecondsToNanosChecked(tt.seconds)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestImportAssignmentResultFromTrace_SchemaV1IgnoresOptionalUsageCounters(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	call := trace["model_calls"].([]any)[0].(EvaluationTrace)
	call["usage_reported"] = true
	call["input_tokens"] = float64(1)
	call["output_tokens"] = float64(2)
	call["thinking_tokens"] = float64(0)
	call["cache_tokens"] = float64(3)
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest

	result, err := ImportAssignmentResultFromTrace(homogeneousAssignmentExecutionRequest(t, "primary"), trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	inference := result.GetModelInferences()[0]
	assert.Nil(t, inference.ThinkingTokens)
	assert.Nil(t, inference.CacheTokens)
}

func TestImportAssignmentResultFromTrace_SchemaV2AllowsAbsentOptionalUsageCounters(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["schema_version"] = "2"
	call := trace["model_calls"].([]any)[0].(EvaluationTrace)
	call["usage_reported"] = true
	call["input_tokens"] = float64(5)
	call["output_tokens"] = float64(6)
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest

	result, err := ImportAssignmentResultFromTrace(homogeneousAssignmentExecutionRequest(t, "primary"), trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	inference := result.GetModelInferences()[0]
	assert.Nil(t, inference.ThinkingTokens)
	assert.Nil(t, inference.CacheTokens)
}

func TestImportAssignmentResultFromTrace_NewFieldsRoundTrip(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	call := trace["model_calls"].([]any)[0].(EvaluationTrace)
	call["tools_declared"] = []any{"recursive_grep_search", "file_read_on_operator"}
	trace["tool_calls"] = []any{
		EvaluationTrace{
			"call_id":          "call-1",
			"tool_name":        "recursive_grep_search",
			"arguments_hash":   "hash-1",
			"success":          false,
			"loop_turn":        float64(2),
			"error_type":       "runtime.error",
			"error":            "syntax error",
			"suggestion":       "check patterns",
			"is_operator_tool": true,
			"execution_id":     "exec-1",
		},
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest

	req := homogeneousAssignmentExecutionRequest(t, "primary")
	req.ScenarioTools = ScenarioToolExpectations{ExpectedTools: []string{"recursive_grep_search"}}
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)

	// Model inference tools declared
	require.Len(t, result.GetModelInferences(), 1)
	inf := result.GetModelInferences()[0]
	assert.True(t, inf.GetToolsDeclaredReported())
	assert.Equal(t, []string{"recursive_grep_search", "file_read_on_operator"}, inf.GetToolsDeclared())

	// Tool call record
	require.Len(t, result.GetToolCalls(), 1)
	tc := result.GetToolCalls()[0]
	assert.Equal(t, uint32(2), tc.GetLoopTurn())
	assert.Equal(t, "runtime.error", tc.GetErrorType())
	assert.True(t, tc.GetGuidanceShown())
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, tc.GetSchemaOutcome())

	// Trajectory fields copied
	assert.NotEqual(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_UNSPECIFIED, result.GetTrajectoryOutcome())
}

func TestImportAssignmentResultFromTrace_CodexCallsExcludedFromScoredInferenceSpan(t *testing.T) {
	t.Parallel()
	traceWithoutCodex := completedHomogeneousTrace(t, "primary")
	scoredCall := traceWithoutCodex["model_calls"].([]any)[0].(EvaluationTrace)
	scoredCall["monotonic_start"] = 100.0
	scoredCall["monotonic_end"] = 102.5 // 2.5s -> 2_500_000_000 ns
	digestWithout, err := ComputeChatProbeTraceDigest(traceWithoutCodex)
	require.NoError(t, err)
	traceWithoutCodex["trace_digest"] = digestWithout

	resultWithout, err := ImportAssignmentResultFromTrace(homogeneousAssignmentExecutionRequest(t, "primary"), traceWithoutCodex, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix })
	require.NoError(t, err)
	require.NotNil(t, resultWithout.ScoredInferenceSpanNanos)
	spanWithout := resultWithout.GetScoredInferenceSpanNanos()
	assert.Equal(t, uint64(2_500_000_000), spanWithout)

	// Now create a trace WITH an additional codex call spanning past the scored calls
	traceWithCodex := completedHomogeneousTrace(t, "primary")
	call1 := traceWithCodex["model_calls"].([]any)[0].(EvaluationTrace)
	call1["monotonic_start"] = 100.0
	call1["monotonic_end"] = 102.5

	codexCall := EvaluationTrace{
		"agent_role":              "codex",
		"classification":          "post_turn",
		"model_role":              "primary",
		"provider":                "G8EProvider",
		"governed_transaction_id": "tx-2",
		"governed_result_digest":  "b" + repeatHex('b', 63),
		"provider_attempt_id":     "attempt-codex-1",
		"normalized_request_hash": "c" + repeatHex('c', 63),
		"monotonic_start":         105.0,
		"monotonic_end":           110.0, // Would make span 10s if included
	}
	traceWithCodex["model_calls"] = []any{call1, codexCall}
	digestWith, err := ComputeChatProbeTraceDigest(traceWithCodex)
	require.NoError(t, err)
	traceWithCodex["trace_digest"] = digestWith

	resultWith, err := ImportAssignmentResultFromTrace(homogeneousAssignmentExecutionRequest(t, "primary"), traceWithCodex, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix })
	require.NoError(t, err)
	require.NotNil(t, resultWith.ScoredInferenceSpanNanos)
	spanWith := resultWith.GetScoredInferenceSpanNanos()

	// Codex call is emitted in records
	assert.Len(t, resultWith.GetModelInferences(), 2)
	assert.Equal(t, "codex", resultWith.GetModelInferences()[1].GetAgentPersona())

	// ScoredInferenceSpanNanos for trace with a codex call equals the span without it!
	assert.Equal(t, spanWithout, spanWith)
}

func completedHomogeneousTrace(t *testing.T, role string) EvaluationTrace {
	t.Helper()
	trace := EvaluationTrace{
		"schema_version":    "1",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"role_outcome":      "invoked",
		"evaluation_context": EvaluationTrace{
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
		"controlled_role_assignment": EvaluationTrace{
			"designated_model_role": role,
		},
		"model_calls": []any{
			EvaluationTrace{
				"agent_role":              "sage",
				"classification":          "scored_chain",
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
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	return trace
}
