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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCollectRunAggregateState(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
		homogeneousAssignment("assign-2", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT),
	}
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": {
			AssignmentId:    "assign-1",
			LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
			DeterministicGrades: []*evalv1.DeterministicGrade{{
				Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
			}},
		},
		"assign-2": {
			AssignmentId:    "assign-2",
			LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL,
		},
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	assert.Equal(t, uint32(2), state.Scheduled)
	assert.Equal(t, uint32(2), state.Terminal)
	assert.Equal(t, uint32(1), state.Passed)
	assert.Equal(t, uint32(1), state.Failed)
	assert.Equal(t, uint32(1), state.ModelCount)
	assert.Equal(t, uint32(1), state.EvaluatedCount)
	assert.Equal(t, uint32(1), state.VariantRoles["qwen3-4b:primary"].Passed)
	assert.Equal(t, uint32(1), state.VariantRoles["qwen3-4b:assistant"].Failed)
}

func TestBuildRunAggregateViewRecords(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": {
			AssignmentId:    "assign-1",
			LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
			DeterministicGrades: []*evalv1.DeterministicGrade{{
				Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
			}},
		},
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	run := &evalv1.EvaluationRun{
		RunId:     "run-1",
		StartedAt: timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId: "eval-smoke-mini",
		},
	}
	records, err := BuildRunAggregateViewRecords(run, state, time.Unix(1_700_000_050, 0).UTC())
	require.NoError(t, err)
	require.Len(t, records, 4)

	summary := map[string]any{}
	require.NoError(t, json.Unmarshal(records[0].Body, &summary))
	assert.Equal(t, "evaluation_summary", summary["kind"])
	assert.Equal(t, "completed", summary["lifecycle_state"])
	assert.Equal(t, "exploratory_partial", summary["quality_state"])
	assert.Equal(t, float64(50), summary["elapsed_seconds"])

	catalog := map[string]any{}
	require.NoError(t, json.Unmarshal(records[1].Body, &catalog))
	assert.Equal(t, "catalog_snapshot", catalog["kind"])
	assert.Equal(t, "ds-live-run-1", catalog["dataset_id"])
	assert.Equal(t, float64(1), catalog["assignment_count"])
	assert.Equal(t, float64(1), catalog["provider_request_count"])

	model := map[string]any{}
	require.NoError(t, json.Unmarshal(records[2].Body, &model))
	assert.Equal(t, "model_summary", model["kind"])
	assert.Equal(t, "qwen3-4b", model["variant_id"])
	assert.Equal(t, "primary", model["role"])
	assert.Equal(t, "Qwen3 4b", model["display_name"])
	assert.Equal(t, "qwen3:4b", model["served_model_tag"])

	methodology := map[string]any{}
	require.NoError(t, json.Unmarshal(records[3].Body, &methodology))
	assert.Equal(t, "methodology_snapshot", methodology["kind"])
}

func TestBuildRunAggregateViewRecordsPartialProgress(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
		homogeneousAssignment("assign-2", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT),
	}
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": {
			AssignmentId:    "assign-1",
			LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
			DeterministicGrades: []*evalv1.DeterministicGrade{{
				Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
			}},
		},
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	run := &evalv1.EvaluationRun{
		RunId:     "run-1",
		StartedAt: timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId: "eval-smoke-mini",
		},
	}
	records, err := BuildRunAggregateViewRecords(run, state, time.Unix(1_700_000_050, 0).UTC())
	require.NoError(t, err)

	summary := map[string]any{}
	require.NoError(t, json.Unmarshal(records[0].Body, &summary))
	assert.Equal(t, "running", summary["lifecycle_state"])
	assert.Equal(t, "live_in_progress", summary["quality_state"])
}

func TestBuildRunCompletionViewRecords(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
		homogeneousAssignment("assign-2", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT),
	}
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": {
			AssignmentId:    "assign-1",
			LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
			DeterministicGrades: []*evalv1.DeterministicGrade{{
				Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
			}},
		},
		"assign-2": {
			AssignmentId:    "assign-2",
			LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL,
		},
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	require.True(t, RunAggregateComplete(assignments, results, state))
	run := &evalv1.EvaluationRun{
		RunId:     "run-1",
		StartedAt: timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId: "eval-smoke-mini",
		},
	}
	records, err := BuildRunCompletionViewRecords(run, assignments, results, state, time.Unix(1_700_000_100, 0).UTC())
	require.NoError(t, err)
	require.Len(t, records, 5)

	summary := map[string]any{}
	require.NoError(t, json.Unmarshal(records[0].Body, &summary))
	assert.Equal(t, "evaluation_summary", summary["kind"])
	assert.IsType(t, map[string]any{}, summary["model_role_mapping"])
	assert.Equal(t, "completed", summary["lifecycle_state"])
	assert.Equal(t, float64(100), summary["elapsed_seconds"])
	assert.Equal(t, float64(2), summary["assignment_total"])
	assert.Equal(t, float64(1), summary["assignment_completed"])
	assert.Equal(t, float64(1), summary["assignment_failed"])
	assert.Equal(t, "exploratory_partial", summary["quality_state"])

	catalog := map[string]any{}
	require.NoError(t, json.Unmarshal(records[1].Body, &catalog))
	assert.Equal(t, "catalog_snapshot", catalog["kind"])
	assert.Equal(t, "exploratory_partial", catalog["quality_state"])
}

func TestBuildRunVerificationViewRecords(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": {
			AssignmentId:    "assign-1",
			LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
			DeterministicGrades: []*evalv1.DeterministicGrade{{
				Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
			}},
		},
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	run := &evalv1.EvaluationRun{
		RunId:     "run-1",
		StartedAt: timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId:          "eval-smoke-mini",
			CampaignDigest:      "campaign-digest",
			CatalogDigest:       "catalog-digest",
			ModelRegistryDigest: "model-registry-digest",
		},
	}
	report := &evalv1.EvaluationVerificationReport{
		SchemaVersion:            "2.0.0",
		ReportId:                 "run-1",
		RunId:                    "run-1",
		Status:                   evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		ReportDigestRef:          &compliancev1.ComplianceEvidenceReference{Sha256: strings.Repeat("a", 64)},
		VerifiedAt:               timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
		VerifierReleaseVersion:   "v2.1.10",
		VerifierContractVersion:  "2.0.0",
		VerifiedPopulationDigest: strings.Repeat("b", 64),
		CampaignDigest:           "campaign-digest",
		CatalogDigest:            "catalog-digest",
		ModelRegistryDigest:      "model-registry-digest",
		VerifiedAssignmentCount:  1,
		ExpectedAssignmentCount:  1,
	}
	records, err := BuildRunVerificationViewRecords(run, state, report, time.Unix(1_700_000_200, 0).UTC())
	require.NoError(t, err)
	require.Len(t, records, 2)

	summary := map[string]any{}
	require.NoError(t, json.Unmarshal(records[0].Body, &summary))
	assert.Equal(t, "passed", summary["verifier_state"])
	assert.Equal(t, "exploratory_verified", summary["quality_state"])

	model := map[string]any{}
	require.NoError(t, json.Unmarshal(records[1].Body, &model))
	assert.Equal(t, "model_summary", model["kind"])
	assert.Equal(t, "exploratory_verified", model["quality_state"])
	assert.Equal(t, "qwen3-4b", model["variant_id"])
	assert.Equal(t, "primary", model["role"])
	assert.Equal(t, float64(1), model["evaluation_coverage"])
	assert.Equal(t, float64(1), model["pass_rate"].(map[string]any)["denominator"])
}

func TestBuildRunVerificationViewRecordsEligibility(t *testing.T) {
	tests := []struct {
		name          string
		reportStatus  evalv1.EvaluationVerdictStatus
		expected      uint32
		verified      uint32
		terminal      uint32
		wantModelRows int
	}{
		{name: "failed report", reportStatus: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, expected: 1, verified: 1, terminal: 1},
		{name: "mismatched population", reportStatus: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, expected: 2, verified: 1, terminal: 1},
		{name: "incomplete aggregate", reportStatus: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, expected: 2, verified: 1, terminal: 1},
		{name: "uncovered report", reportStatus: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, expected: 2, verified: 1, terminal: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &runAggregateState{
				Scheduled: tt.expected,
				Terminal:  tt.terminal,
				VariantRoles: map[string]*variantRoleAggregate{
					"qwen3-4b:primary": {VariantID: "qwen3-4b", Role: "primary", Scheduled: 1, Terminal: 1, Passed: 1, Outcomes: map[string]uint32{"completed": 1}},
				},
			}
			run := &evalv1.EvaluationRun{RunId: "run-eligibility"}
			report := &evalv1.EvaluationVerificationReport{
				RunId:                   run.GetRunId(),
				Status:                  tt.reportStatus,
				ExpectedAssignmentCount: tt.expected,
				VerifiedAssignmentCount: tt.verified,
			}
			records, err := BuildRunVerificationViewRecords(run, state, report, time.Unix(1_700_000_200, 0).UTC())
			require.NoError(t, err)
			assert.Len(t, records, 1+tt.wantModelRows)
		})
	}
}

func TestBuildRunVerificationViewRecordsEmitsEveryEligibleVariantRole(t *testing.T) {
	run := &evalv1.EvaluationRun{
		RunId: "run-multi-model",
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignDigest:      "campaign-digest",
			CatalogDigest:       "catalog-digest",
			ModelRegistryDigest: "model-registry-digest",
		},
	}
	state := &runAggregateState{
		Scheduled: 2,
		Terminal:  2,
		VariantRoles: map[string]*variantRoleAggregate{
			"model-a:primary":   {VariantID: "model-a", Role: "primary", Scheduled: 1, Terminal: 1, Passed: 1, Outcomes: map[string]uint32{"completed": 1}},
			"model-b:assistant": {VariantID: "model-b", Role: "assistant", Scheduled: 1, Terminal: 1, Failed: 1, Outcomes: map[string]uint32{"model_failed": 1}},
		},
	}
	report := &evalv1.EvaluationVerificationReport{
		SchemaVersion:            "2.0.0",
		RunId:                    run.GetRunId(),
		Status:                   evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		ReportDigestRef:          &compliancev1.ComplianceEvidenceReference{Sha256: strings.Repeat("a", 64)},
		VerifiedAt:               timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
		VerifierReleaseVersion:   "v2.1.10",
		VerifierContractVersion:  "2.0.0",
		VerifiedPopulationDigest: strings.Repeat("b", 64),
		CampaignDigest:           "campaign-digest",
		CatalogDigest:            "catalog-digest",
		ModelRegistryDigest:      "model-registry-digest",
		ExpectedAssignmentCount:  2,
		VerifiedAssignmentCount:  2,
	}
	records, err := BuildRunVerificationViewRecords(run, state, report, time.Unix(1_700_000_200, 0).UTC())
	require.NoError(t, err)
	require.Len(t, records, 3)
	firstModel := map[string]any{}
	secondModel := map[string]any{}
	require.NoError(t, json.Unmarshal(records[1].Body, &firstModel))
	require.NoError(t, json.Unmarshal(records[2].Body, &secondModel))
	assert.Equal(t, "exploratory_verified", firstModel["quality_state"])
	assert.Equal(t, "exploratory_verified", secondModel["quality_state"])
	assert.Equal(t, float64(0), secondModel["pass_rate"].(map[string]any)["estimate"])
	assert.NotEqual(t, firstModel["variant_id"], secondModel["variant_id"])
}

func TestBuildRunVerificationViewRecordsBoundFailurePublishesPartialModelRevisions(t *testing.T) {
	run := &evalv1.EvaluationRun{RunId: "run-failed", CampaignBinding: &evalv1.ModelCampaignBinding{CampaignDigest: "campaign", CatalogDigest: "catalog", ModelRegistryDigest: "registry"}}
	state := &runAggregateState{Scheduled: 1, Terminal: 1, VariantRoles: map[string]*variantRoleAggregate{"model-a:primary": {VariantID: "model-a", Role: "primary", Scheduled: 1, Terminal: 1, Failed: 1, Outcomes: map[string]uint32{"model_failed": 1}}}}
	report := &evalv1.EvaluationVerificationReport{SchemaVersion: "2.0.0", RunId: run.GetRunId(), Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, ReportDigestRef: &compliancev1.ComplianceEvidenceReference{Sha256: strings.Repeat("c", 64)}, VerifiedAt: timestamppb.New(time.Unix(1_700_000_200, 0).UTC()), VerifierReleaseVersion: "v2.1.10", VerifierContractVersion: "2.0.0", VerifiedPopulationDigest: strings.Repeat("d", 64), CampaignDigest: "campaign", CatalogDigest: "catalog", ModelRegistryDigest: "registry", ExpectedAssignmentCount: 1, VerifiedAssignmentCount: 1}
	records, err := BuildRunVerificationViewRecords(run, state, report, time.Unix(1_700_000_200, 0).UTC())
	require.NoError(t, err)
	require.Len(t, records, 2)
	model := map[string]any{}
	require.NoError(t, json.Unmarshal(records[1].Body, &model))
	assert.Equal(t, "exploratory_partial", model["quality_state"])
}

func TestBoundVerificationRevisionKeysAreReportScoped(t *testing.T) {
	assert.NotEqual(t, BoundVerificationSummaryIdempotencyKey("run-1", "a"), BoundVerificationSummaryIdempotencyKey("run-1", "b"))
	assert.NotEqual(t, VerifiedModelSummaryIdempotencyKey("run-1", "model-1", "primary", "a"), VerifiedModelSummaryIdempotencyKey("run-1", "model-1", "primary", "b"))
}

func TestVerifiedModelSummaryIdempotencyKeyIsDistinctAndScoped(t *testing.T) {
	assert.NotEqual(t, RunVerificationIdempotencyKey("run-1"), VerifiedModelSummaryIdempotencyKey("run-1", "model-1", "primary", "a"))
	assert.NotEqual(t, VerifiedModelSummaryIdempotencyKey("run-1", "model-1", "primary", "a"), VerifiedModelSummaryIdempotencyKey("run-1", "model-1", "assistant", "a"))
	assert.NotEqual(t, VerifiedModelSummaryIdempotencyKey("run-1", "model-1", "primary", "a"), VerifiedModelSummaryIdempotencyKey("run-2", "model-1", "primary", "a"))
	assert.NotEqual(t, VerifiedModelSummaryIdempotencyKey("run-1", "model-1", "primary", "a"), VerifiedModelSummaryIdempotencyKey("run-1", "model-1", "primary", "b"))
}

func TestFormatCampaignVerifierFailureSummary(t *testing.T) {
	assert.Equal(t, "", formatCampaignVerifierFailureSummary(nil))
	assert.Equal(t, "assignment a: missing window", formatCampaignVerifierFailureSummary(&evalv1.EvaluationVerificationReport{
		FailureReasons: []string{"assignment a: missing window"},
	}))
	summary := formatCampaignVerifierFailureSummary(&evalv1.EvaluationVerificationReport{
		FailureReasons: []string{"one", "two", "three", "four"},
	})
	assert.Contains(t, summary, "4 verification failure(s)")
	assert.Contains(t, summary, "one")
	assert.Contains(t, summary, "... and 1 more")
	assert.IsType(t, "", summary)
}

func TestRunAggregateCompleteLifecycleWithoutPersistedResult(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	results := map[string]*evalv1.EvaluationAssignmentResult{}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	require.True(t, RunAggregateComplete(assignments, results, state))
	assert.Equal(t, uint32(1), state.Terminal)
}

func TestCampaignPublicationCoordinatorPublishRunAggregates(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	controller := NewCampaignController(store, &stubCampaignExecutor{}, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" }).WithPublication(coordinator)
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
	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), run.GetRunId())
	require.NoError(t, err)
	_, _, err = controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
	}, req.ScenarioArtifacts)
	require.NoError(t, err)

	var aggregateKinds []string
	for _, record := range exporter.records {
		payload := map[string]any{}
		require.NoError(t, json.Unmarshal([]byte(record.RecordBytes), &payload))
		if kind, ok := payload["kind"].(string); ok {
			aggregateKinds = append(aggregateKinds, kind)
		}
	}
	assert.Contains(t, aggregateKinds, "evaluation_summary")
	assert.Contains(t, aggregateKinds, "catalog_snapshot")
	assert.Contains(t, aggregateKinds, "model_summary")
	assert.Contains(t, aggregateKinds, "methodology_snapshot")
}

func homogeneousAssignment(assignmentID, variantID string, role evalv1.ModelCampaignRole) *evalv1.EvaluationAssignment {
	return &evalv1.EvaluationAssignment{
		AssignmentId: assignmentID,
		RunId:        "run-1",
		ScenarioId:   "instruction-exact-format",
		Lane:         evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				CandidateVariant: &evalv1.ModelVariant{VariantId: variantID},
				DesignatedRole:   role,
			},
		},
		QueuedAt: timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
	}
}
