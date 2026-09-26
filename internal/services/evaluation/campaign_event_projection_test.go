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
	assert.Equal(t, "stage_updated", event.Kind)
	assert.Equal(t, "ds-live-run-live-1", event.DatasetID)
	assert.Contains(t, event.StageLabel, "model role invoked")
	assert.Contains(t, event.StageLabel, "primary")

	body, err := MarshalPublicLiveEvent(event)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "served_model_tag")
	assert.NotContains(t, string(body), "backend_name")
	assert.NotContains(t, string(body), "quantization")
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
	assert.Equal(t, "metric_updated", event.Kind)
	passRate, ok := event.MetricDelta["pass_rate"]
	require.True(t, ok)
	require.NotNil(t, passRate.Value)
	assert.InDelta(t, 0.75, *passRate.Value, 0.0001)

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
	passRate, ok := event.MetricDelta["pass_rate"]
	require.True(t, ok)
	assert.Equal(t, "no_scored_calls", passRate.UnavailableReason)
	assert.Nil(t, passRate.Value)
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
				DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
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
			ModelRole:         evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			ModelVariant:      &evalv1.ModelVariant{VariantId: "qwen3-4b"},
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
		SchemaVersion:   CampaignSchemaVersion,
		RunId:           runID,
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "campaign-1"},
	}))
	require.NoError(t, store.SaveAssignment(context.Background(), assignment))
	require.NoError(t, store.SaveAssignmentResult(context.Background(), result))
	require.NoError(t, coordinator.PublishAssignmentLiveEvents(context.Background(), assignment, result))
	require.NotEmpty(t, exporter.records)
	foundScoredInvocation := false
	foundMetric := false
	for _, record := range exporter.records {
		require.Equal(t, models.PublicFeedRecordTypeEvent, record.RecordType)
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(record.RecordBytes), &body))
		switch body["kind"] {
		case "stage_updated":
			foundScoredInvocation = true
		case "metric_updated":
			foundMetric = true
		}
	}
	assert.True(t, foundScoredInvocation)
	assert.True(t, foundMetric)
}

func TestCampaignPublication_PublishesInvocationAtAssignmentStart(t *testing.T) {
	files := newCampaignMemoryFileService()
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(NewStore(files), files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	runID := "run-live-start"
	assignment := &evalv1.EvaluationAssignment{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    "assign-start",
		RunId:           runID,
		CampaignId:      "campaign-1",
		ScenarioId:      "scenario-1",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING,
		StartedAt:       timestamppb.New(time.Unix(1_700_000_001, 0).UTC()),
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
				CandidateVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
			},
		},
	}
	store := NewStore(files)
	require.NoError(t, store.SaveRun(context.Background(), &evalv1.EvaluationRun{
		SchemaVersion:   CampaignSchemaVersion,
		RunId:           runID,
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "campaign-1"},
	}))
	require.NoError(t, store.SaveAssignment(context.Background(), assignment))
	require.NoError(t, coordinator.PublishAssignmentInvocationLiveEvents(context.Background(), assignment))
	require.Len(t, exporter.records, 1)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(exporter.records[0].RecordBytes), &body))
	assert.Equal(t, "stage_updated", body["kind"])
	assert.NotContains(t, body, "metric_delta")
}

func TestCampaignPublication_PublishesScoredInferenceDuringTraceProgress(t *testing.T) {
	files := newCampaignMemoryFileService()
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(NewStore(files), files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	runID := "run-live-progress"
	assignment := &evalv1.EvaluationAssignment{
		SchemaVersion:   CampaignSchemaVersion,
		AssignmentId:    "assign-progress",
		RunId:           runID,
		CampaignId:      "campaign-1",
		ScenarioId:      "scenario-1",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING,
		StartedAt:       timestamppb.New(time.Unix(1_700_000_001, 0).UTC()),
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
				CandidateVariant: &evalv1.ModelVariant{VariantId: "qwen3-4b"},
			},
		},
	}
	store := NewStore(files)
	require.NoError(t, store.SaveRun(context.Background(), &evalv1.EvaluationRun{
		SchemaVersion:   CampaignSchemaVersion,
		RunId:           runID,
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "campaign-1"},
	}))
	require.NoError(t, store.SaveAssignment(context.Background(), assignment))
	trace := completedHomogeneousTrace(t, "primary")
	trace["status"] = "running"
	call := trace["model_calls"].([]any)[0].(EvaluationTrace)
	call["usage_reported"] = true
	call["input_tokens"] = 12
	call["output_tokens"] = 34
	partial, ok, err := PartialAssignmentResultFromScoredTrace(AssignmentExecutionRequest{
		Assignment: assignment,
		AttemptID:  "attempt-1",
	}, trace, func(prefix string) string { return prefix })
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, partial)
	require.NoError(t, coordinator.PublishAssignmentScoredInferenceLiveEvents(context.Background(), assignment, partial))
	require.Len(t, exporter.records, 1)
	assertScoredInvocationMetricDelta(t, exporter.records[0].RecordBytes, "primary", 12, 34)
}

func TestCampaignPublication_PublishesFormationRoleInvocationIncrementally(t *testing.T) {
	files := newCampaignMemoryFileService()
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(NewStore(files), files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	assignment := heterogeneousAssignmentExecutionRequest(t, stack, variants).Assignment
	assignment.SchemaVersion = CampaignSchemaVersion
	store := NewStore(files)
	require.NoError(t, store.SaveRun(context.Background(), &evalv1.EvaluationRun{
		SchemaVersion:   CampaignSchemaVersion,
		RunId:           assignment.GetRunId(),
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: assignment.GetCampaignId()},
	}))
	require.NoError(t, store.SaveAssignment(context.Background(), assignment))

	require.NoError(t, coordinator.PublishFormationRoleInvocationLiveEvent(context.Background(), assignment, FormationRoleLite))
	require.NoError(t, coordinator.PublishFormationRoleInvocationLiveEvent(context.Background(), assignment, FormationRoleAssistant))
	require.Len(t, exporter.records, 2)
	for _, record := range exporter.records {
		var body map[string]any
		require.NoError(t, json.Unmarshal([]byte(record.RecordBytes), &body))
		assert.Equal(t, "stage_updated", body["kind"])
		assert.NotContains(t, body, "metric_delta")
	}
}

func TestCampaignPublication_PublishesFormationRoleProgressWithPerRoleMetrics(t *testing.T) {
	files := newCampaignMemoryFileService()
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(NewStore(files), files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	req := heterogeneousAssignmentExecutionRequest(t, stack, variants)
	assignment := req.Assignment
	assignment.SchemaVersion = CampaignSchemaVersion
	store := NewStore(files)
	require.NoError(t, store.SaveRun(context.Background(), &evalv1.EvaluationRun{
		SchemaVersion:   CampaignSchemaVersion,
		RunId:           assignment.GetRunId(),
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: assignment.GetCampaignId()},
	}))
	require.NoError(t, store.SaveAssignment(context.Background(), assignment))
	observedAt := time.Unix(1_700_000_100, 0).UTC()
	newID := func(prefix string) string { return prefix + "-1" }

	publishProgress := func(roles []partialFormationRoleSpec) {
		formationResult := partialFormationRunResult(stack, variants, roles)
		partial, err := ImportAssignmentResultFromFormationRun(req, formationResult, observedAt, newID)
		require.NoError(t, err)
		require.NoError(t, coordinator.PublishAssignmentScoredInferenceLiveEvents(context.Background(), assignment, partial))
	}

	publishProgress([]partialFormationRoleSpec{{FormationRoleLite, 116, 8, 40}})
	require.Len(t, exporter.records, 1)
	assertScoredInvocationMetricDelta(t, exporter.records[0].RecordBytes, "lite", 116, 8)

	publishProgress([]partialFormationRoleSpec{
		{FormationRoleLite, 116, 8, 40},
		{FormationRoleAssistant, 152, 12, 55},
	})
	require.Len(t, exporter.records, 2)
	assertScoredInvocationMetricDelta(t, exporter.records[1].RecordBytes, "assistant", 152, 12)

	publishProgress([]partialFormationRoleSpec{
		{FormationRoleLite, 116, 8, 40},
		{FormationRoleAssistant, 152, 12, 55},
		{FormationRolePrimary, 168, 16, 57},
	})
	require.Len(t, exporter.records, 3)
	assertScoredInvocationMetricDelta(t, exporter.records[2].RecordBytes, "primary", 168, 16)
}

type partialFormationRoleSpec struct {
	role             FormationRole
	promptTokens     uint32
	completionTokens uint32
	latencyMs        int
}

func partialFormationRunResult(stack *evalv1.HeterogeneousStackDefinition, variants []*evalv1.ModelVariant, roles []partialFormationRoleSpec) *FormationRunResult {
	result := &FormationRunResult{
		FormationID: stack.GetStackId(),
		Roles:       make([]FormationRoleTelemetry, 0, len(roles)),
	}
	for _, spec := range roles {
		variant := variantForFormationRole(stack, spec.role, variants)
		result.Roles = append(result.Roles, FormationRoleTelemetry{
			Role:                    spec.role,
			Model:                   formationModelFromVariant(variant),
			ProviderAttemptID:       "provider-" + string(spec.role),
			UsageAvailability:       evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
			PromptTokens:            spec.promptTokens,
			GenerationTokens:        spec.completionTokens,
			GenerationDurationNanos: uint64(spec.latencyMs) * uint64(time.Millisecond),
		})
	}
	return result
}

func variantForFormationRole(stack *evalv1.HeterogeneousStackDefinition, role FormationRole, variants []*evalv1.ModelVariant) *evalv1.ModelVariant {
	variantID := ""
	switch role {
	case FormationRolePrimary:
		variantID = stack.GetPrimarySlot().GetVariantId()
	case FormationRoleAssistant:
		variantID = stack.GetAssistantSlot().GetVariantId()
	case FormationRoleLite:
		variantID = stack.GetLiteSlot().GetVariantId()
	}
	for _, variant := range variants {
		if variant.GetVariantId() == variantID {
			return variant
		}
	}
	return nil
}

func assertScoredInvocationMetricDelta(t *testing.T, recordBytes string, role string, inputTokens float64, outputTokens float64) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(recordBytes), &body))
	assert.Equal(t, "stage_updated", body["kind"])
	assert.Equal(t, role, body["role"])
	delta, ok := body["metric_delta"].(map[string]any)
	require.True(t, ok)
	input, ok := delta["input_tokens"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, inputTokens, input["value"])
	output, ok := delta["output_tokens"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, outputTokens, output["value"])
}

func ptrFloat64(value float64) *float64 {
	return &value
}
