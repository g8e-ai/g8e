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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
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
	records, err := BuildRunAggregateViewRecords(run, state, nil, time.Unix(1_700_000_050, 0).UTC())
	require.NoError(t, err)
	require.Len(t, records, 4)

	summary := map[string]any{}
	require.NoError(t, json.Unmarshal(records[0].Body, &summary))
	assert.Equal(t, "1.5.0", summary["schema_version"])
	assert.Equal(t, "evaluation_summary", summary["kind"])
	assert.Equal(t, "model", summary["evaluation_unit"])
	assert.Equal(t, "completed", summary["lifecycle_state"])
	assert.Equal(t, "exploratory_partial", summary["quality_state"])
	assert.Equal(t, float64(50), summary["elapsed_seconds"])

	catalog := map[string]any{}
	require.NoError(t, json.Unmarshal(records[1].Body, &catalog))
	assert.Equal(t, "1.5.0", catalog["schema_version"])
	assert.Equal(t, "catalog_snapshot", catalog["kind"])
	assert.Equal(t, "ds-live-run-1", catalog["dataset_id"])
	assert.Equal(t, float64(1), catalog["assignment_count"])
	assert.Equal(t, float64(1), catalog["provider_request_count"])

	model := map[string]any{}
	require.NoError(t, json.Unmarshal(records[2].Body, &model))
	assert.Equal(t, "1.5.0", model["schema_version"])
	assert.Equal(t, "model_summary", model["kind"])
	assert.Equal(t, "qwen3-4b", model["variant_id"])
	assert.Equal(t, "primary", model["role"])
	assert.Equal(t, "Qwen3 4b", model["display_name"])
	assert.Equal(t, "qwen3:4b", model["served_model_tag"])

	methodology := map[string]any{}
	require.NoError(t, json.Unmarshal(records[3].Body, &methodology))
	assert.Equal(t, "1.5.0", methodology["schema_version"])
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
	records, err := BuildRunAggregateViewRecords(run, state, nil, time.Unix(1_700_000_050, 0).UTC())
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
	records, err := BuildRunCompletionViewRecords(run, assignments, results, state, nil, time.Unix(1_700_000_100, 0).UTC())
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
				SchemaVersion:            "2.0.0",
				RunId:                    run.GetRunId(),
				Status:                   tt.reportStatus,
				ReportDigestRef:          &compliancev1.ComplianceEvidenceReference{Sha256: strings.Repeat("a", 64)},
				VerifiedAt:               timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
				VerifierReleaseVersion:   "v2.1.10",
				VerifierContractVersion:  "2.0.0",
				VerifiedPopulationDigest: strings.Repeat("b", 64),
				ExpectedAssignmentCount:  tt.expected,
				VerifiedAssignmentCount:  tt.verified,
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

func scoredInferenceCall(tokens uint32, durationNanos uint64) *evalv1.ModelInferenceRecord {
	return &evalv1.ModelInferenceRecord{
		InferenceRecordId:        "inference-1",
		UsageAvailability:        evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
		CompletionTokens:         tokens,
		GenerationDurationNanos:  durationNanos,
	}
}

func terminalResultWithEvidence(assignmentID string, spanNanos *uint64, calls ...*evalv1.ModelInferenceRecord) *evalv1.EvaluationAssignmentResult {
	result := &evalv1.EvaluationAssignmentResult{
		AssignmentId:    assignmentID,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		DeterministicGrades: []*evalv1.DeterministicGrade{{
			Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		}},
		ModelInferences: calls,
	}
	if spanNanos != nil {
		span := *spanNanos
		result.ScoredInferenceSpanNanos = &span
	}
	return result
}

func TestCollectRunHeadlineMetricsPassRate(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
		homogeneousAssignment("assign-2", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	assignments[1].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": terminalResultWithEvidence("assign-1", nil),
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	require.NotNil(t, state.Headline)
	passRate := state.Headline.PassRate
	require.NotNil(t, passRate.Value)
	assert.Equal(t, 1.0, *passRate.Value)
	assert.Equal(t, uint32(2), passRate.Eligible)
	assert.Equal(t, uint32(2), passRate.Observed)
	assert.Equal(t, uint32(0), passRate.Unavailable)
}

func TestCollectRunHeadlineMetricsPassRateUnavailableBeforeTerminal(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	state, err := CollectRunAggregateState(assignments, map[string]*evalv1.EvaluationAssignmentResult{})
	require.NoError(t, err)
	passRate := state.Headline.PassRate
	assert.Nil(t, passRate.Value)
	assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE, passRate.UnavailableReason)
	assert.Equal(t, uint32(0), passRate.Eligible)
}

func TestCollectRunHeadlineMetricsLatencyMedian(t *testing.T) {
	tests := []struct {
		name      string
		spansMS   []uint64
		wantValue float64
	}{
		{name: "odd count picks center", spansMS: []uint64{300_000_000, 100_000_000, 200_000_000}, wantValue: 200},
		{name: "even count averages centers", spansMS: []uint64{100_000_000, 300_000_000}, wantValue: 200},
		{name: "observed zero contributes", spansMS: []uint64{0, 400_000_000}, wantValue: 200},
		{name: "single observed zero", spansMS: []uint64{0}, wantValue: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignments := make([]*evalv1.EvaluationAssignment, 0, len(tt.spansMS))
			results := make(map[string]*evalv1.EvaluationAssignmentResult, len(tt.spansMS))
			for index, span := range tt.spansMS {
				id := fmt.Sprintf("assign-%d", index)
				assignment := homogeneousAssignment(id, "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY)
				assignment.LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
				assignments = append(assignments, assignment)
				results[id] = terminalResultWithEvidence(id, &span, scoredInferenceCall(10, 1_000_000))
			}
			state, err := CollectRunAggregateState(assignments, results)
			require.NoError(t, err)
			latency := state.Headline.LatencyP50MS
			require.NotNil(t, latency.Value)
			assert.Equal(t, tt.wantValue, *latency.Value)
			assert.Equal(t, uint32(len(tt.spansMS)), latency.Observed)
			assert.Equal(t, uint32(len(tt.spansMS)), latency.Eligible)
			assert.Equal(t, uint32(0), latency.Unavailable)
		})
	}
}

func TestCollectRunHeadlineMetricsLatencyPartialCoverage(t *testing.T) {
	span := uint64(150_000_000)
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
		homogeneousAssignment("assign-2", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	assignments[1].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": terminalResultWithEvidence("assign-1", &span, scoredInferenceCall(10, 1_000_000)),
		"assign-2": terminalResultWithEvidence("assign-2", nil, scoredInferenceCall(10, 1_000_000)),
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	latency := state.Headline.LatencyP50MS
	require.NotNil(t, latency.Value)
	assert.Equal(t, 150.0, *latency.Value)
	assert.Equal(t, uint32(1), latency.Observed)
	assert.Equal(t, uint32(2), latency.Eligible)
	assert.Equal(t, uint32(1), latency.Unavailable)
}

func TestCollectRunHeadlineMetricsLatencyUnavailableWithoutSpans(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": terminalResultWithEvidence("assign-1", nil, scoredInferenceCall(10, 1_000_000)),
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	latency := state.Headline.LatencyP50MS
	assert.Nil(t, latency.Value)
	assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE, latency.UnavailableReason)
	assert.Equal(t, uint32(0), latency.Observed)
	assert.Equal(t, uint32(1), latency.Eligible)
	assert.Equal(t, uint32(1), latency.Unavailable)
}

func TestCollectRunHeadlineMetricsNoScoredCalls(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": terminalResultWithEvidence("assign-1", nil),
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	assert.Nil(t, state.Headline.LatencyP50MS.Value)
	assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS, state.Headline.LatencyP50MS.UnavailableReason)
	assert.Equal(t, uint32(0), state.Headline.LatencyP50MS.Eligible)
	assert.Nil(t, state.Headline.OutputThroughputP50.Value)
	assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS, state.Headline.OutputThroughputP50.UnavailableReason)
}

func TestCollectRunHeadlineMetricsLatencyRejectsOutOfBoundSpan(t *testing.T) {
	span := (maxPublicDurationMS + 1) * 1_000_000
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": terminalResultWithEvidence("assign-1", &span, scoredInferenceCall(10, 1_000_000)),
	}
	_, err := CollectRunAggregateState(assignments, results)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}

func TestCollectRunHeadlineMetricsOutputThroughput(t *testing.T) {
	// Two scored calls totaling 40 tokens over 2s of generation: 20 tok/s.
	call := scoredInferenceCall(20, 1_000_000_000)
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
		homogeneousAssignment("assign-2", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	assignments[1].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	secondCall := scoredInferenceCall(30, 1_000_000_000)
	secondCall.InferenceRecordId = "inference-2"
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": terminalResultWithEvidence("assign-1", nil, call, scoredInferenceCall(20, 1_000_000_000)),
		"assign-2": terminalResultWithEvidence("assign-2", nil, secondCall),
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	throughput := state.Headline.OutputThroughputP50
	require.NotNil(t, throughput.Value)
	// assign-1: 40 tokens / 2s = 20 tok/s; assign-2: 30 tok/s. Median = 25.
	assert.Equal(t, 25.0, *throughput.Value)
	assert.Equal(t, uint32(2), throughput.Observed)
	assert.Equal(t, uint32(2), throughput.Eligible)
}

func TestCollectRunHeadlineMetricsOutputThroughputIncompleteContributors(t *testing.T) {
	tests := []struct {
		name  string
		calls []*evalv1.ModelInferenceRecord
	}{
		{
			name: "usage not reported",
			calls: []*evalv1.ModelInferenceRecord{{
				InferenceRecordId:       "inference-1",
				UsageAvailability:       evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE,
				CompletionTokens:        20,
				GenerationDurationNanos: 1_000_000_000,
			}},
		},
		{
			name: "zero generation duration",
			calls: []*evalv1.ModelInferenceRecord{{
				InferenceRecordId:       "inference-1",
				UsageAvailability:       evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
				CompletionTokens:        20,
				GenerationDurationNanos: 0,
			}},
		},
		{
			name: "one incomplete call disqualifies the assignment",
			calls: []*evalv1.ModelInferenceRecord{
				scoredInferenceCall(20, 1_000_000_000),
				{
					InferenceRecordId:       "inference-2",
					UsageAvailability:       evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE,
					CompletionTokens:        20,
					GenerationDurationNanos: 1_000_000_000,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignments := []*evalv1.EvaluationAssignment{
				homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
			}
			assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
			results := map[string]*evalv1.EvaluationAssignmentResult{
				"assign-1": terminalResultWithEvidence("assign-1", nil, tt.calls...),
			}
			state, err := CollectRunAggregateState(assignments, results)
			require.NoError(t, err)
			throughput := state.Headline.OutputThroughputP50
			assert.Nil(t, throughput.Value)
			assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE, throughput.UnavailableReason)
			assert.Equal(t, uint32(0), throughput.Observed)
			assert.Equal(t, uint32(1), throughput.Eligible)
			assert.Equal(t, uint32(1), throughput.Unavailable)
		})
	}
}

func TestCollectRunHeadlineMetricsFailedAssignmentContributesMeasurements(t *testing.T) {
	span := uint64(500_000_000)
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	failed := terminalResultWithEvidence("assign-1", &span, scoredInferenceCall(15, 1_000_000_000))
	failed.DeterministicGrades[0].Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	results := map[string]*evalv1.EvaluationAssignmentResult{"assign-1": failed}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	require.NotNil(t, state.Headline.LatencyP50MS.Value)
	assert.Equal(t, 500.0, *state.Headline.LatencyP50MS.Value)
	require.NotNil(t, state.Headline.OutputThroughputP50.Value)
	assert.Equal(t, 15.0, *state.Headline.OutputThroughputP50.Value)
}

func boundTestReport(runID string, status evalv1.EvaluationVerdictStatus) *evalv1.EvaluationVerificationReport {
	return &evalv1.EvaluationVerificationReport{
		SchemaVersion:            "2.0.0",
		ReportId:                 runID,
		RunId:                    runID,
		Status:                   status,
		ReportDigestRef:          &compliancev1.ComplianceEvidenceReference{Sha256: strings.Repeat("a", 64)},
		VerifiedAt:               timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
		VerifierReleaseVersion:   "v2.1.10",
		VerifierContractVersion:  "2.0.0",
		VerifiedPopulationDigest: strings.Repeat("b", 64),
		ExpectedAssignmentCount:  1,
		VerifiedAssignmentCount:  1,
	}
}

func TestBuildRunAggregateViewRecordsVerifierStates(t *testing.T) {
	span := uint64(200_000_000)
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": terminalResultWithEvidence("assign-1", &span, scoredInferenceCall(10, 1_000_000_000)),
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	run := &evalv1.EvaluationRun{
		RunId:           "run-1",
		StartedAt:       timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "eval-smoke-mini"},
	}
	decodeSummary := func(t *testing.T, records []CampaignViewRecord) map[string]any {
		t.Helper()
		for _, record := range records {
			payload := map[string]any{}
			require.NoError(t, json.Unmarshal(record.Body, &payload))
			if payload["kind"] == "evaluation_summary" {
				return payload
			}
		}
		return nil
	}

	t.Run("unbound run reports not_run", func(t *testing.T) {
		records, err := BuildRunAggregateViewRecords(run, state, nil, time.Unix(1_700_000_050, 0).UTC())
		require.NoError(t, err)
		summary := decodeSummary(t, records)
		assert.Equal(t, "not_run", summary["verifier_state"])
		assert.Nil(t, summary["verification_metadata"])
	})

	t.Run("bound pass reports verified state and metadata", func(t *testing.T) {
		report := boundTestReport(run.GetRunId(), evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS)
		records, err := BuildRunAggregateViewRecords(run, state, report, time.Unix(1_700_000_050, 0).UTC())
		require.NoError(t, err)
		summary := decodeSummary(t, records)
		assert.Equal(t, "passed", summary["verifier_state"])
		assert.Equal(t, "exploratory_verified", summary["quality_state"])
		metadata, ok := summary["verification_metadata"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "bound", metadata["provenance"])
		assert.Equal(t, "passed", metadata["verifier_state"])
		assert.Equal(t, "v2.1.10", metadata["verifier_release_version"])
		assert.Equal(t, "2.0.0", metadata["verifier_contract_version"])
		assert.Equal(t, strings.Repeat("a", 64), metadata["report_digest"])
		assert.Equal(t, strings.Repeat("b", 64), metadata["population_digest"])
		assert.Equal(t, "2023-11-14T22:16:40Z", metadata["verified_at"])
	})

	t.Run("bound failure reports failed state", func(t *testing.T) {
		report := boundTestReport(run.GetRunId(), evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL)
		report.FailureReasons = []string{"assignment assign-1: missing window"}
		records, err := BuildRunAggregateViewRecords(run, state, report, time.Unix(1_700_000_050, 0).UTC())
		require.NoError(t, err)
		summary := decodeSummary(t, records)
		assert.Equal(t, "failed", summary["verifier_state"])
		assert.Equal(t, "assignment assign-1: missing window", summary["verifier_failure_summary"])
		metadata, ok := summary["verification_metadata"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "failed", metadata["verifier_state"])
	})

	t.Run("malformed bound report fails closed", func(t *testing.T) {
		report := boundTestReport(run.GetRunId(), evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS)
		report.VerifiedAt = nil
		_, err := BuildRunAggregateViewRecords(run, state, report, time.Unix(1_700_000_050, 0).UTC())
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
	})
}

func TestBuildRunAggregateViewRecordsHeadlineMetricsShape(t *testing.T) {
	span := uint64(200_000_000)
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED
	results := map[string]*evalv1.EvaluationAssignmentResult{
		"assign-1": terminalResultWithEvidence("assign-1", &span, scoredInferenceCall(10, 1_000_000_000)),
	}
	state, err := CollectRunAggregateState(assignments, results)
	require.NoError(t, err)
	run := &evalv1.EvaluationRun{
		RunId:           "run-1",
		StartedAt:       timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "eval-smoke-mini"},
	}
	records, err := BuildRunAggregateViewRecords(run, state, nil, time.Unix(1_700_000_050, 0).UTC())
	require.NoError(t, err)
	summary := map[string]any{}
	require.NoError(t, json.Unmarshal(records[0].Body, &summary))
	headline, ok := summary["headline_metrics"].(map[string]any)
	require.True(t, ok)
	require.Len(t, headline, 3)

	passRate, ok := headline["pass_rate"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ratio", passRate["unit"])
	assert.Equal(t, 1.0, passRate["value"])
	assert.Equal(t, 1.0, passRate["observed_count"])
	assert.Equal(t, 1.0, passRate["eligible_count"])
	assert.Equal(t, 0.0, passRate["unavailable_count"])

	latency, ok := headline["latency_p50_ms"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "milliseconds", latency["unit"])
	assert.Equal(t, 200.0, latency["value"])
	assert.Equal(t, 1.0, latency["observed_count"])
	assert.Equal(t, 1.0, latency["eligible_count"])

	throughput, ok := headline["output_throughput_p50_tokens_per_second"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tokens_per_second", throughput["unit"])
	assert.Equal(t, 10.0, throughput["value"])
	assert.Equal(t, 1.0, throughput["observed_count"])
	assert.Equal(t, 1.0, throughput["eligible_count"])
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
