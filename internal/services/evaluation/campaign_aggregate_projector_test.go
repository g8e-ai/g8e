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
	assert.Equal(t, "running", summary["lifecycle_state"])
	assert.Equal(t, "live_in_progress", summary["quality_state"])
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
			CampaignId: "eval-smoke-mini",
		},
	}
	report := &evalv1.EvaluationVerificationReport{
		SchemaVersion: CampaignSchemaVersion,
		ReportId:      "run-1",
		RunId:         "run-1",
		Status:        evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		VerifiedAt:    timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
	}
	records, err := BuildRunVerificationViewRecords(run, state, report, time.Unix(1_700_000_200, 0).UTC())
	require.NoError(t, err)
	require.Len(t, records, 1)

	summary := map[string]any{}
	require.NoError(t, json.Unmarshal(records[0].Body, &summary))
	assert.Equal(t, "passed", summary["verifier_state"])
	assert.Equal(t, "exploratory_verified", summary["quality_state"])
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
	coordinator := NewCampaignPublicationCoordinator(store, files, exporter, nil)
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
