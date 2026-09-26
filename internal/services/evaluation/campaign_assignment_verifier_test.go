// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCampaignAssignmentVerifier_RecomputesMatchingGrades(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	trace["designated_role_output"] = "READY"
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	req.ScenarioGold = loadScenarioGold(t, "instruction-exact-format")
	req.GradingMethod = evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	verifier := NewCampaignAssignmentVerifier(func() time.Time { return time.Unix(1_700_000_100, 0).UTC() })
	report, err := verifier.Verify(context.Background(), CampaignAssignmentVerificationRequest{
		Assignment:    req.Assignment,
		Result:        result,
		ScenarioInput: req.ScenarioInput,
		ScenarioGold:  req.ScenarioGold,
		GradingMethod: req.GradingMethod,
		Trace:         trace,
	})
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, report.GetStatus())
	assert.Zero(t, report.GetFailureCount())
}

func TestCampaignAssignmentVerifier_FailsWhenStoredGradesDrift(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	req.ScenarioGold = loadScenarioGold(t, "instruction-exact-format")
	req.GradingMethod = evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	result.DeterministicGrades = append(result.DeterministicGrades, &evalv1.DeterministicGrade{
		GradeId:     req.Assignment.GetAssignmentId() + ":tampered",
		CriterionId: "tampered",
		Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		Detail:      "injected drift",
	})
	digest, err := ComputeAssignmentResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = digest
	verifier := NewCampaignAssignmentVerifier(func() time.Time { return time.Unix(1_700_000_100, 0).UTC() })
	report, err := verifier.Verify(context.Background(), CampaignAssignmentVerificationRequest{
		Assignment:    req.Assignment,
		Result:        result,
		ScenarioInput: req.ScenarioInput,
		ScenarioGold:  req.ScenarioGold,
		GradingMethod: req.GradingMethod,
		Trace:         trace,
	})
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, report.GetStatus())
	assert.NotZero(t, report.GetFailureCount())
}

func TestCampaignAssignmentVerifier_PassesHeterogeneousFormationResult(t *testing.T) {
	t.Parallel()
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	req := heterogeneousAssignmentExecutionRequest(t, stack, variants)
	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)
	formation, err := BindHeterogeneousStack(FormationBindingRequest{Stack: stack, Variants: variants})
	require.NoError(t, err)
	initialState, err := BuildFormationInitialState(req.ScenarioInput)
	require.NoError(t, err)
	formationResult, err := harness.RunBoundFormation(context.Background(), formation, initialState)
	require.NoError(t, err)
	result, err := ImportAssignmentResultFromFormationRun(req, formationResult, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	body, evidence, err := BuildFormationRunEvidence(req, FormationRunContext{
		CampaignID:          req.Assignment.GetCampaignId(),
		RunID:               req.Assignment.GetRunId(),
		AssignmentID:        req.Assignment.GetAssignmentId(),
		EvaluationAttemptID: req.AttemptID,
		ScenarioID:          req.Assignment.GetScenarioId(),
		ModelRegistryDigest: req.Binding.ModelRegistryDigest,
		InferenceSessionID:  req.Binding.InferenceOperatorSessionID,
		DataSessionID:       req.Binding.DataOperatorSessionID,
	}, formationResult)
	require.NoError(t, err)
	require.NotEmpty(t, body)
	verifier := NewCampaignAssignmentVerifier(func() time.Time { return time.Unix(1_700_000_100, 0).UTC() })
	report, err := verifier.Verify(context.Background(), CampaignAssignmentVerificationRequest{
		Assignment:           req.Assignment,
		Result:               result,
		FormationRunEvidence: evidence,
	})
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, report.GetStatus())
	assert.Zero(t, report.GetFailureCount())
}

func TestCampaignAssignmentVerifier_FailsWhenCapturedTelemetryDriftsFromTrace(t *testing.T) {
	t.Parallel()
	trace := completedHomogeneousTrace(t, "primary")
	call := trace["model_calls"].([]any)[0].(EvaluationTrace)
	call["usage_reported"] = true
	call["input_tokens"] = float64(1)
	call["output_tokens"] = float64(2)
	call["thinking_tokens"] = float64(3)
	call["cache_tokens"] = float64(4)
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	result, err := ImportAssignmentResultFromTrace(req, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	result.ModelInferences[0].PromptTokens = 99
	result.ResultDigest, err = ComputeAssignmentResultDigest(result)
	require.NoError(t, err)

	report, err := NewCampaignAssignmentVerifier(func() time.Time { return time.Unix(1_700_000_100, 0).UTC() }).Verify(context.Background(), CampaignAssignmentVerificationRequest{
		Assignment:    req.Assignment,
		Result:        result,
		ScenarioInput: req.ScenarioInput,
		ScenarioGold:  req.ScenarioGold,
		GradingMethod: req.GradingMethod,
		Trace:         trace,
	})
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, report.GetStatus())
	assert.Contains(t, report.GetFailureReasons(), "imported evidence does not match trace: model inference 0 mismatch")
}

func loadScenarioGold(t *testing.T, scenarioID string) ScenarioGoldCriteria {
	t.Helper()
	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	var gold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifacts[scenarioID].Gold.Body, &gold))
	return gold
}
