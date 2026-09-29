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
	"google.golang.org/protobuf/proto"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type g8eeRoutedFormationFixture struct {
	req      AssignmentExecutionRequest
	result   *evalv1.EvaluationAssignmentResult
	evidence *FormationRunEvidence
}

// newG8eeRoutedFormationFixture builds a completed formation whose three roles
// ran through g8ee, grades it on the write path, and round-trips its formation
// run evidence through the store exactly as the verifier will read it.
func newG8eeRoutedFormationFixture(t *testing.T) g8eeRoutedFormationFixture {
	t.Helper()
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	formation, err := BindHeterogeneousStack(FormationBindingRequest{Stack: stack, Variants: variants})
	require.NoError(t, err)
	_, artifacts, err := BuildScenarioCatalog()
	require.NoError(t, err)
	req := heterogeneousAssignmentExecutionRequest(t, stack, variants)
	require.NoError(t, json.Unmarshal(artifacts["instruction-exact-format"].Gold.Body, &req.ScenarioGold))

	formationResult := &FormationRunResult{
		FormationID: formation.ID,
		Passed:      true,
		Roles: []FormationRoleTelemetry{
			{Role: FormationRoleLite, Model: formation.Lite, ProviderAttemptID: "lite-attempt", Trace: completedHomogeneousTrace(t, "lite")},
			{Role: FormationRoleAssistant, Model: formation.Assistant, ProviderAttemptID: "assistant-attempt", Trace: completedHomogeneousTrace(t, "assistant")},
			{Role: FormationRolePrimary, Model: formation.Primary, ProviderAttemptID: "primary-attempt", Trace: completedHomogeneousTrace(t, "primary")},
		},
	}
	result, err := ImportAssignmentResultFromFormationRun(req, formationResult, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)

	runContext := FormationRunContext{
		CampaignID:          req.Assignment.GetCampaignId(),
		RunID:               req.Assignment.GetRunId(),
		AssignmentID:        req.Assignment.GetAssignmentId(),
		EvaluationAttemptID: req.AttemptID,
		ScenarioID:          req.Assignment.GetScenarioId(),
		ModelRegistryDigest: req.Binding.ModelRegistryDigest,
		InferenceSessionID:  req.Binding.InferenceOperatorSessionID,
		DataSessionID:       req.Binding.DataOperatorSessionID,
	}
	body, _, err := BuildFormationRunEvidence(req, runContext, formationResult)
	require.NoError(t, err)
	store := NewStore(newCampaignMemoryFileService())
	require.NoError(t, store.SaveAssignmentFormationRun(context.Background(), req.Assignment.GetRunId(), req.Assignment.GetAssignmentId(), body))
	evidence, err := store.LoadAssignmentFormationRun(context.Background(), req.Assignment.GetRunId(), req.Assignment.GetAssignmentId())
	require.NoError(t, err)
	return g8eeRoutedFormationFixture{req: req, result: result, evidence: evidence}
}

func (f g8eeRoutedFormationFixture) verify(t *testing.T, result *evalv1.EvaluationAssignmentResult, evidence *FormationRunEvidence) *evalv1.EvaluationVerificationReport {
	t.Helper()
	report, err := NewCampaignAssignmentVerifier(func() time.Time { return time.Unix(1_700_000_100, 0) }).Verify(context.Background(), CampaignAssignmentVerificationRequest{
		Assignment:           f.req.Assignment,
		Result:               result,
		ScenarioInput:        f.req.ScenarioInput,
		ScenarioGold:         f.req.ScenarioGold,
		ScenarioTools:        f.req.ScenarioTools,
		GradingMethod:        f.req.GradingMethod,
		FormationRunEvidence: evidence,
	})
	require.NoError(t, err)
	return report
}

func TestCampaignAssignmentVerifier_AcceptsG8eeRoutedFormationFromRoleTraces(t *testing.T) {
	fixture := newG8eeRoutedFormationFixture(t)

	report := fixture.verify(t, fixture.result, fixture.evidence)

	assert.Empty(t, report.GetFailureReasons())
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, report.GetStatus())
}

func TestCampaignAssignmentVerifier_RejectsTamperedFormationRoleGrade(t *testing.T) {
	fixture := newG8eeRoutedFormationFixture(t)
	tampered := proto.Clone(fixture.result).(*evalv1.EvaluationAssignmentResult)
	// Assistant's role-invoked grade shares its criterion ID with Lite's and
	// Primary's; only the grade ID tells them apart.
	grade := findDeterministicGradeByID(tampered.GetDeterministicGrades(), fixture.req.Assignment.GetAssignmentId()+":assistant:role-invoked")
	require.NotNil(t, grade)
	grade.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	digest, err := ComputeAssignmentResultDigest(tampered)
	require.NoError(t, err)
	tampered.ResultDigest = digest

	report := fixture.verify(t, tampered, fixture.evidence)

	assert.Contains(t, report.GetFailureReasons(), "stored deterministic grades do not match formation recomputation")
}

func TestCampaignAssignmentVerifier_RejectsTamperedFormationRoleTrace(t *testing.T) {
	fixture := newG8eeRoutedFormationFixture(t)
	fixture.evidence.Result.Roles[2].Trace["designated_role_output"] = "rewritten after the fact"

	report := fixture.verify(t, fixture.result, fixture.evidence)

	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, report.GetStatus())
	assert.Contains(t, report.GetFailureReasons(), "formation grade recomputation failed: role primary: evaluation: validate chat probe trace: trace digest mismatch")
}
