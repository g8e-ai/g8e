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

	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type recordingCampaignFeedExporter struct {
	records []CampaignPublicFeedRecord
}

func (r *recordingCampaignFeedExporter) HighWaterSequence(context.Context) (int64, error) {
	return int64(len(r.records)), nil
}

func (r *recordingCampaignFeedExporter) ExportBatch(_ context.Context, records []CampaignPublicFeedRecord) error {
	r.records = append(r.records, records...)
	return nil
}

type sequenceTrackingCampaignFeedExporter struct {
	highWater int64
	batches   [][]CampaignPublicFeedRecord
}

func (s *sequenceTrackingCampaignFeedExporter) HighWaterSequence(context.Context) (int64, error) {
	return s.highWater, nil
}

func (s *sequenceTrackingCampaignFeedExporter) ExportBatch(_ context.Context, records []CampaignPublicFeedRecord) error {
	if len(records) == 0 {
		return nil
	}
	expected := s.highWater + 1
	if records[0].Sequence != expected {
		return fmt.Errorf("public-feed: batch sequence is out of order")
	}
	for index, record := range records {
		if record.Sequence != expected+int64(index) {
			return fmt.Errorf("public-feed: batch sequence is out of order")
		}
	}
	s.batches = append(s.batches, append([]CampaignPublicFeedRecord(nil), records...))
	s.highWater = records[len(records)-1].Sequence
	return nil
}

func TestCampaignPublicationCoordinatorExportFeedRecordsBatches(t *testing.T) {
	files := newCampaignMemoryFileService()
	exporter := &sequenceTrackingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(NewStore(files), files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	requests := make([]campaignFeedPublishRequest, 0, 3)
	for index := 0; index < 3; index++ {
		requests = append(requests, campaignFeedPublishRequest{
			IdempotencyKey: fmt.Sprintf("run-1:key-%d", index),
			Body:           []byte(fmt.Sprintf("{\"index\":%d}", index)),
		})
	}
	count, err := coordinator.exportFeedRecords(context.Background(), "run-1", requests)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.Len(t, exporter.batches, 1)
	assert.Len(t, exporter.batches[0], 3)
	assert.Equal(t, int64(3), exporter.highWater)
}

func TestCampaignPublicationCoordinatorIdempotentLifecyclePublish(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId:    "assign-1",
		RunId:           "run-1",
		ScenarioId:      "instruction-exact-format",
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED,
		QueuedAt:        timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
	}
	category := evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE
	require.NoError(t, coordinator.PublishAssignmentLifecycle(context.Background(), assignment, category, assignment.GetQueuedAt().AsTime()))
	require.NoError(t, coordinator.PublishAssignmentLifecycle(context.Background(), assignment, category, assignment.GetQueuedAt().AsTime()))
	assert.Len(t, exporter.records, 1)
}

func TestCampaignPublicationCoordinatorPublishRunCatchUp(t *testing.T) {
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
	assert.GreaterOrEqual(t, len(exporter.records), 3)
}

func TestCampaignPublicationCoordinatorPublishRunCompletion(t *testing.T) {
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
	for i := 0; i < 3; i++ {
		_, _, err = controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), CampaignExecutionBinding{
			InferenceOperatorSessionID: "inf-session",
			DataOperatorID:             "data-op",
			DataOperatorSessionID:      "data-session",
			ModelRegistryDigest:        req.Inventory.RegistryDigest,
			ModelRegistry:              InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
		}, req.ScenarioArtifacts)
		require.NoError(t, err)
	}
	before := len(exporter.records)
	count, err := coordinator.PublishRunCompletion(context.Background(), run.GetRunId(), time.Unix(1_700_000_100, 0).UTC())
	require.NoError(t, err)
	assert.Greater(t, count, 0)
	assert.Greater(t, len(exporter.records), before)
	count, err = coordinator.PublishRunCompletion(context.Background(), run.GetRunId(), time.Unix(1_700_000_200, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestCampaignPublicationCoordinatorPublishRunVerification(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	publicationState := NewMemoryCampaignPublicationStateStore()
	coordinator := NewCampaignPublicationCoordinator(store, files, publicationState, exporter, nil)
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
	for i := 0; i < 3; i++ {
		_, _, err = controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), CampaignExecutionBinding{
			InferenceOperatorSessionID: "inf-session",
			DataOperatorID:             "data-op",
			DataOperatorSessionID:      "data-session",
			ModelRegistryDigest:        req.Inventory.RegistryDigest,
			ModelRegistry:              InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
		}, req.ScenarioArtifacts)
		require.NoError(t, err)
	}
	report := &evalv1.EvaluationVerificationReport{
		SchemaVersion:           "2.0.0",
		ReportId:                run.GetRunId(),
		RunId:                   run.GetRunId(),
		Status:                  evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		VerifiedAt:              timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
		VerifierReleaseVersion:  "test-release",
		VerifierContractVersion: "2.0.0",
		ReportDigestRef:         &compliancev1.ComplianceEvidenceReference{Sha256: strings.Repeat("a", 64)},
	}
	assignments, err := store.ListAssignments(context.Background(), run.GetRunId())
	require.NoError(t, err)
	results := make(map[string]*evalv1.EvaluationAssignmentResult, len(assignments))
	for _, assignment := range assignments {
		exists, existsErr := store.AssignmentResultExists(context.Background(), run.GetRunId(), assignment.GetAssignmentId())
		require.NoError(t, existsErr)
		if !exists {
			continue
		}
		result, loadErr := store.LoadAssignmentResult(context.Background(), run.GetRunId(), assignment.GetAssignmentId())
		require.NoError(t, loadErr)
		results[assignment.GetAssignmentId()] = result
	}
	spec, err := store.LoadCampaignSpec(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)
	catalog, err = store.LoadScenarioCatalog(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)
	applicability, err := BuildRunVerificationApplicability(run, spec, catalog, assignments, results, report)
	require.NoError(t, err)
	populationDigest, err := digestProto(applicability.Population)
	require.NoError(t, err)
	report.VerifiedPopulationDigest = populationDigest
	report.ExpectedAssignmentCount = applicability.ExpectedAssignmentCount
	report.VerifiedAssignmentCount = applicability.VerifiedAssignmentCount
	report.CampaignDigest = spec.GetCampaignDigest()
	report.CatalogDigest = spec.GetCatalogDigest()
	report.ModelRegistryDigest = spec.GetModelRegistryDigest()
	before := len(exporter.records)
	count, err := coordinator.PublishRunVerification(context.Background(), run.GetRunId(), report)
	require.NoError(t, err)
	assert.Greater(t, count, 0)
	assert.Greater(t, len(exporter.records), before)

	var summary map[string]any
	for _, record := range exporter.records[len(exporter.records)-count:] {
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(record.RecordBytes), &payload))
		if payload["kind"] == "evaluation_summary" {
			summary = payload
			break
		}
	}
	require.NotNil(t, summary)
	assert.Equal(t, "passed", summary["verifier_state"])

	modelSummaries := make([]map[string]any, 0)
	for _, record := range exporter.records[len(exporter.records)-count:] {
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(record.RecordBytes), &payload))
		if payload["kind"] == "model_summary" {
			modelSummaries = append(modelSummaries, payload)
		}
	}
	require.Len(t, modelSummaries, 3)
	roles := make(map[string]struct{}, len(modelSummaries))
	for _, modelSummary := range modelSummaries {
		assert.Equal(t, "exploratory_verified", modelSummary["quality_state"])
		assert.Equal(t, CampaignDatasetID(run.GetRunId()), modelSummary["dataset_id"])
		assert.NotEmpty(t, modelSummary["variant_id"])
		role, ok := modelSummary["role"].(string)
		require.True(t, ok)
		roles[role] = struct{}{}
	}
	assert.Equal(t, map[string]struct{}{"assistant": {}, "lite": {}, "primary": {}}, roles)

	state, err := publicationState.Load(context.Background(), run.GetRunId())
	require.NoError(t, err)
	filteredKeys := make([]string, 0, len(state.PublishedIdempotency))
	for _, key := range state.PublishedIdempotency {
		if !strings.Contains(key, ":verification:model:") {
			filteredKeys = append(filteredKeys, key)
		}
	}
	filteredKeys = append(filteredKeys, RunVerificationIdempotencyKey(run.GetRunId()))
	state.PublishedIdempotency = filteredKeys
	require.NoError(t, publicationState.Save(context.Background(), state))

	backfillCount, err := coordinator.PublishRunVerification(context.Background(), run.GetRunId(), report)
	require.NoError(t, err)
	assert.Equal(t, 3, backfillCount)
	retryCount, err := coordinator.PublishRunVerification(context.Background(), run.GetRunId(), report)
	require.NoError(t, err)
	assert.Equal(t, 0, retryCount)
}

func TestCampaignPublicationCoordinatorForceRepublish(t *testing.T) {
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
	_, err = coordinator.PublishRunCatchUp(context.Background(), run.GetRunId())
	require.NoError(t, err)
	before := len(exporter.records)
	assert.Greater(t, before, 0)
	_, err = coordinator.PublishRunCatchUp(context.Background(), run.GetRunId())
	require.NoError(t, err)
	assert.Len(t, exporter.records, before)
	require.NoError(t, coordinator.ResetPublicationIdempotency(context.Background(), run.GetRunId()))
	_, err = coordinator.PublishRunCatchUp(context.Background(), run.GetRunId())
	require.NoError(t, err)
	assert.Greater(t, len(exporter.records), before)
}

func TestCampaignControllerWithPublicationPublishesQueuedAssignments(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" }).WithPublication(coordinator)
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
	count, err := controller.ScheduleHomogeneousRun(context.Background(), run.GetRunId())
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.GreaterOrEqual(t, len(exporter.records), 8)
}

func TestCampaignPublicationCoordinatorPublishAssignmentResultUsesRemoteObservationWindows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	req := testCampaignInitRequest(t)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })
	run, err := controller.InitializeCampaign(ctx, req)
	require.NoError(t, err)
	exporter := &recordingCampaignFeedExporter{}
	attempt := &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: "attempt-remote",
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		StartedAtUnixMs:   time.Unix(1_700_000_000, 0).UnixMilli(),
		CompletedAtUnixMs: time.Unix(1_700_000_010, 0).UnixMilli(),
	}
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          "attempt-remote",
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		AttemptStartedAtUnixMs:     attempt.GetStartedAtUnixMs(),
		AttemptCompletedAtUnixMs:   attempt.GetCompletedAtUnixMs(),
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			ObservedAtUnixNanos:        uint64(time.Unix(1_700_000_001, 0).UnixNano()),
			VramBytesAvailability:      evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			VramUsedBytes:              16_000_000_000,
			GpuUtilizationAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			GpuUtilizationPercent:      42,
		}},
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest

	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, &stubProviderObservationRemote{
		window:  window,
		attempt: attempt,
	})
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId:    "assign-1",
		RunId:           run.GetRunId(),
		CampaignId:      run.GetCampaignBinding().GetCampaignId(),
		ScenarioId:      req.Catalog.GetScenarios()[0].GetScenarioId(),
		ScenarioRef:     &compliancev1.VersionedReference{Id: req.Catalog.GetScenarios()[0].GetScenarioId(), Version: req.Catalog.GetScenarios()[0].GetScenarioVersion()},
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
	}
	result := &evalv1.EvaluationAssignmentResult{
		AssignmentId: "assign-1",
		RunId:        run.GetRunId(),
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId: "inference-1",
			ProviderAttemptId: "attempt-remote",
		}},
	}
	require.NoError(t, coordinator.PublishAssignmentResult(
		ctx,
		assignment,
		result,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE,
		"unverified",
	))
	require.Len(t, exporter.records, 1)

	envelope := CampaignProjectionEnvelope{}
	require.NoError(t, json.Unmarshal([]byte(exporter.records[0].RecordBytes), &envelope))
	record := map[string]any{}
	require.NoError(t, json.Unmarshal(envelope.Record, &record))
	benchmark, ok := record["benchmark_observations"].(map[string]any)
	require.True(t, ok)
	gpu, ok := benchmark["gpu"].(map[string]any)
	require.True(t, ok)
	peak, ok := gpu["vram_peak_bytes"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(16_000_000_000), peak["value"])
}
