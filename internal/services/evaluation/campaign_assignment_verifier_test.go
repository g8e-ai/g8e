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
	trace := completedHomogeneousTrace("primary")
	trace["designated_role_output"] = "READY"
	digest, err := computeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	req := homogeneousAssignmentExecutionRequest("primary")
	req.ScenarioGold = loadScenarioGold("instruction-exact-format")
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
	trace := completedHomogeneousTrace("primary")
	req := homogeneousAssignmentExecutionRequest("primary")
	req.ScenarioGold = loadScenarioGold("instruction-exact-format")
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

func loadScenarioGold(scenarioID string) ScenarioGoldCriteria {
	_, artifacts, err := BuildNorthStarScenarioCatalog()
	if err != nil {
		panic(err)
	}
	var gold ScenarioGoldCriteria
	if err := json.Unmarshal(artifacts[scenarioID].Gold.Body, &gold); err != nil {
		panic(err)
	}
	return gold
}
