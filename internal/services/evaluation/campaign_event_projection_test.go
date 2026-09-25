// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License 1.1 —
// see LICENSE for details.

package evaluation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/publicdisclosure"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestProjectModelRoleInvocationEvent_ExcludesProviderBoundaryFields(t *testing.T) {
	event, err := ProjectModelRoleInvocationEvent(PublicModelRoleInvocationSignal{
		RunID:        "run-live-1",
		AssignmentID: "assign-1",
		VariantID:    "qwen3-4b",
		Role:         models.ModelRolePrimary,
		TaskID:       "instruction-exact-format",
		ObservedAt:   "2026-09-24T12:00:00.000Z",
		EventID:      "run-live-1:assign-1:invocation:primary:42",
		Completed:    2,
		Total:        5,
	})
	require.NoError(t, err)
	assert.Equal(t, "stage_updated", event["kind"])
	assert.Equal(t, "ds-live-run-live-1", event["dataset_id"])
	assert.NotContains(t, event, "served_model_tag")
	assert.NotContains(t, event, "backend_name")
	assert.NotContains(t, event, "quantization")
	stageLabel, ok := event["stage_label"].(string)
	require.True(t, ok)
	assert.Contains(t, stageLabel, "model role invoked")
	assert.Contains(t, stageLabel, "primary")

	body, err := MarshalPublicLiveEvent(event)
	require.NoError(t, err)
	require.NoError(t, publicdisclosure.ValidatePublicFeedRecord(models.PublicFeedRecordTypeEvent, body))
}

func TestProjectMetricAvailabilityEvent_ProjectsPassRateDelta(t *testing.T) {
	event, err := ProjectMetricAvailabilityEvent(PublicMetricAvailabilitySignal{
		RunID:        "run-live-1",
		AssignmentID: "assign-1",
		VariantID:    "qwen3-4b",
		MetricID:     "pass_rate",
		Numerator:    3,
		Denominator:  4,
		Rate:         ptrFloat64(0.75),
		ObservedAt:   "2026-09-24T12:00:05.000Z",
		EventID:      "run-live-1:assign-1:metric:pass_rate:42",
		Completed:    1,
		Total:        5,
	})
	require.NoError(t, err)
	assert.Equal(t, "metric_updated", event["kind"])
	delta, ok := event["metric_delta"].(map[string]any)
	require.True(t, ok)
	passRate, ok := delta["pass_rate"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 0.75, passRate["value"], 0.0001)

	body, err := MarshalPublicLiveEvent(event)
	require.NoError(t, err)
	require.NoError(t, publicdisclosure.ValidatePublicFeedRecord(models.PublicFeedRecordTypeEvent, body))
}

func TestProjectMetricAvailabilityEvent_ZeroDenominatorIsUnavailable(t *testing.T) {
	event, err := ProjectMetricAvailabilityEvent(PublicMetricAvailabilitySignal{
		RunID:        "run-live-1",
		AssignmentID: "assign-1",
		VariantID:    "qwen3-4b",
		MetricID:     "pass_rate",
		Numerator:    0,
		Denominator:  0,
		ObservedAt:   "2026-09-24T12:00:05.000Z",
		EventID:      "run-live-1:assign-1:metric:pass_rate:43",
		Completed:    0,
		Total:        5,
	})
	require.NoError(t, err)
	delta, ok := event["metric_delta"].(map[string]any)
	require.True(t, ok)
	passRate, ok := delta["pass_rate"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "no_scored_calls", passRate["unavailable_reason"])
}

func TestHeadlineMetricID_RecognizesExplorerHeadlineMetrics(t *testing.T) {
	assert.True(t, HeadlineMetricID("pass_rate"))
	assert.True(t, HeadlineMetricID("latency_p50_ms"))
	assert.True(t, HeadlineMetricID("output_throughput_p50_tokens_per_second"))
	assert.False(t, HeadlineMetricID("task_score"))
}

func TestCampaignPublicationCoordinator_ExportsProjectionRecordsOnly(t *testing.T) {
	files := newCampaignMemoryFileService()
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(NewStore(files), files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId:    "assign-1",
		RunId:           "run-1",
		ScenarioId:      "scenario-1",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED,
		QueuedAt:        timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
	}
	category := evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE
	require.NoError(t, coordinator.PublishAssignmentLifecycle(context.Background(), assignment, category, assignment.GetQueuedAt().AsTime()))
	require.NotEmpty(t, exporter.records)
	for _, record := range exporter.records {
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(record.RecordBytes), &body))
		assert.Contains(t, body, "message_type")
		assert.NotEqual(t, "metric_updated", body["kind"])
		assert.NotEqual(t, "stage_updated", body["kind"])
	}
}

const campaignPublicationEmitsNativeLiveEvents = true

func TestCampaignPublication_PublishesNativeLiveEventsFromTerminalResult(t *testing.T) {
	require.True(t, campaignPublicationEmitsNativeLiveEvents)
	files := newCampaignMemoryFileService()
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(NewStore(files), files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	runID := "run-live-1"
	assignmentID := "assign-1"
	assignment := &evalv1.EvaluationAssignment{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    assignmentID,
		RunId:           runID,
		CampaignId:      "campaign-1",
		ScenarioId:      "scenario-1",
		Repetition:      1,
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				DesignatedRole: evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
				CandidateVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
			},
		},
	}
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    assignmentID,
		RunId:           runID,
		CampaignId:      "campaign-1",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		CompletedAt:     timestamppb.New(time.Unix(1_700_000_010, 0).UTC()),
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			ModelRole: evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			ModelVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
			UsageAvailability: evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
		}},
		DeterministicGrades: []*evalv1.DeterministicGrade{{
			CriterionId: "role-invoked",
			Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		}},
	}
	digest, err := ComputeAssignmentResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = digest
	store := NewStore(files)
	require.NoError(t, store.SaveRun(context.Background(), &evalv1.EvaluationRun{
		SchemaVersion: CampaignSchemaVersion,
		RunId:         runID,
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "campaign-1"},
	}))
	require.NoError(t, store.SaveAssignment(context.Background(), assignment))
	require.NoError(t, store.SaveAssignmentResult(context.Background(), result))
	require.NoError(t, coordinator.PublishAssignmentLiveEvents(context.Background(), assignment, result))
	require.NotEmpty(t, exporter.records)
	foundInvocation := false
	foundMetric := false
	for _, record := range exporter.records {
		require.Equal(t, models.PublicFeedRecordTypeEvent, record.RecordType)
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(record.RecordBytes), &body))
		switch body["kind"] {
		case "stage_updated":
			foundInvocation = true
		case "metric_updated":
			foundMetric = true
		}
	}
	assert.True(t, foundInvocation)
	assert.True(t, foundMetric)
}

func ptrFloat64(value float64) *float64 {
	return &value
}
