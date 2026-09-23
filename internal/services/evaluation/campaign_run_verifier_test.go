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
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCampaignVerificationBinding_BindsExactVerifiedPopulation(t *testing.T) {
	catalogRef := &compliancev1.VersionedReference{Id: "catalog-1", Version: "1.0.0"}
	run := &evalv1.EvaluationRun{
		RunId: "run-1",
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId:          "campaign-1",
			CampaignDigest:      "campaign-digest",
			CatalogRef:          catalogRef,
			CatalogDigest:       "catalog-digest",
			ModelRegistryDigest: "registry-digest",
		},
	}
	spec := &evalv1.EvaluationCampaignSpec{CampaignId: "campaign-1", CampaignDigest: "campaign-digest", CatalogRef: catalogRef, CatalogDigest: "catalog-digest", ModelRegistryDigest: "registry-digest"}
	catalog := &evalv1.EvaluationScenarioCatalog{CatalogRef: catalogRef, CatalogDigest: "catalog-digest"}
	assignments := []*evalv1.EvaluationAssignment{{AssignmentId: "assignment-1", RunId: "run-1", CampaignId: "campaign-1", ScenarioId: "scenario-1", DeterministicIdentity: "identity-1", LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED}}
	results := map[string]*evalv1.EvaluationAssignmentResult{"assignment-1": {AssignmentId: "assignment-1", RunId: "run-1", CampaignId: "campaign-1", ResultDigest: "result-digest"}}
	report := &evalv1.EvaluationVerificationReport{SchemaVersion: CampaignSchemaVersion, ReportId: "run-1", RunId: "run-1", Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, VerifiedAt: timestamppb.New(time.Unix(1_700_000_000, 0).UTC())}

	bound, applicability, err := bindCampaignVerificationReport(report, run, spec, catalog, assignments, results, CampaignVerificationPolicy{
		VerifierReleaseVersion: "v2.1.12",
		ProviderObservation:    ProviderObservationPolicyStrict,
		ModelProvenance:        ModelProvenancePolicyInterim,
	})

	require.NoError(t, err)
	assert.True(t, applicability.Applicable)
	assert.Equal(t, campaignVerificationSchemaVersion, bound.GetSchemaVersion())
	assert.Equal(t, constants.CampaignVerifierVersion, bound.GetVerifierContractVersion())
	assert.Equal(t, "v2.1.12", bound.GetVerifierReleaseVersion())
	assert.Equal(t, uint32(1), bound.GetExpectedAssignmentCount())
	assert.Equal(t, uint32(1), bound.GetVerifiedAssignmentCount())
	assert.Equal(t, applicability.Population.GetCampaignDigest(), bound.GetCampaignDigest())
	assert.Equal(t, applicability.Population.GetCatalogDigest(), bound.GetCatalogDigest())
	assert.Equal(t, applicability.Population.GetModelRegistryDigest(), bound.GetModelRegistryDigest())
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_STRICT, bound.GetProviderObservationPolicy())
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM, bound.GetModelProvenancePolicy())
	assert.Equal(t, mustPopulationDigest(t, applicability.Population), bound.GetVerifiedPopulationDigest())
	require.NotNil(t, bound.GetReportDigestRef())
	assert.Len(t, bound.GetReportDigestRef().GetSha256(), 64)
	assert.Empty(t, report.GetVerifierContractVersion())
}

func mustPopulationDigest(t *testing.T, population *evalv1.EvaluationVerifiedPopulation) string {
	t.Helper()
	digest, err := ComputeVerifiedPopulationDigest(population)
	require.NoError(t, err)
	return digest
}

func TestVerifyCampaignRunReadOnly_BindsIncompletePopulationWithoutPersistingVerification(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	_, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	count, err := controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)

	result, err := verifyCampaignRunReadOnly(context.Background(), store, req.RunID, CampaignVerificationPolicy{
		VerifierReleaseVersion: "v2.1.12",
		ProviderObservation:    ProviderObservationPolicyInterim,
		ModelProvenance:        ModelProvenancePolicyInterim,
		AssessmentTime:         func() time.Time { return time.Unix(1_700_000_100, 0).UTC() },
	})

	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, result.Report.GetStatus())
	assert.False(t, result.Population.Complete)
	assert.True(t, result.Applicability.Applicable)
	assert.Equal(t, uint32(count), result.Report.GetExpectedAssignmentCount())
	assert.Zero(t, result.Report.GetVerifiedAssignmentCount())
	_, err = store.LoadCampaignVerification(context.Background(), req.RunID)
	require.Error(t, err)
}

func TestCampaignRunVerifier_PassesCompletedAssignment(t *testing.T) {
	t.Parallel()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	executor := &stubCampaignExecutor{}
	controller := NewCampaignController(store, executor, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	req := testCampaignInitRequest(t)
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:1],
	}
	truncatedDigest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = truncatedDigest
	req.Catalog = truncated
	_, err = controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), req.RunID)
	require.NoError(t, err)
	binding := CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              req.Inventory.ToModelRegistryFreeze().Variants,
	}
	result, executed, err := controller.ExecuteNextAssignment(context.Background(), req.RunID, binding, req.ScenarioArtifacts)
	require.NoError(t, err)
	require.True(t, executed)
	assignment, err := store.LoadAssignment(context.Background(), req.RunID, result.GetAssignmentId())
	require.NoError(t, err)
	trace := completedHomogeneousTrace(t, "primary")
	trace["evaluation_context"] = map[string]any{
		"campaign_id":                req.CampaignID,
		"run_id":                     req.RunID,
		"assignment_id":              assignment.GetAssignmentId(),
		"evaluation_attempt_id":      "attempt-1",
		"scenario_id":                assignment.GetScenarioId(),
		"model_registry_digest":      binding.ModelRegistryDigest,
		"target_operator_session_id": binding.InferenceOperatorSessionID,
		"evaluation_lane":            "model_role",
		"designated_model_role":      "primary",
	}
	digest, err := ComputeChatProbeTraceDigest(trace)
	require.NoError(t, err)
	trace["trace_digest"] = digest
	traceBody, err := marshalSortedJSON(trace)
	require.NoError(t, err)
	require.NoError(t, store.SaveAssignmentTrace(context.Background(), req.RunID, assignment.GetAssignmentId(), traceBody))
	artifact := req.ScenarioArtifacts[assignment.GetScenarioId()]
	var scenarioInput ScenarioInputFixture
	require.NoError(t, json.Unmarshal(artifact.Input.Body, &scenarioInput))
	var scenarioGold ScenarioGoldCriteria
	require.NoError(t, json.Unmarshal(artifact.Gold.Body, &scenarioGold))
	imported, err := ImportAssignmentResultFromTrace(AssignmentExecutionRequest{
		Assignment:    assignment,
		AttemptID:     "attempt-1",
		ScenarioInput: scenarioInput,
		ScenarioGold:  scenarioGold,
		GradingMethod: evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		Binding:       binding,
	}, trace, nil, time.Unix(1_700_000_000, 0).UTC(), func(prefix string) string { return prefix + "-1" })
	require.NoError(t, err)
	require.NoError(t, store.SaveAssignmentResult(context.Background(), imported))

	report, err := NewCampaignRunVerifier(func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }).VerifyRun(context.Background(), store, req.RunID, truncated, req.ScenarioArtifacts)
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, report.GetStatus())
	require.NoError(t, store.SaveCampaignVerification(context.Background(), req.RunID, report))
}
