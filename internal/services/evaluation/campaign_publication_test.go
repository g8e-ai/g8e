// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
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

func TestCampaignPublicationCoordinatorIdempotentLifecyclePublish(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(store, files, exporter)
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
	coordinator := NewCampaignPublicationCoordinator(store, files, exporter)
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
	coordinator := NewCampaignPublicationCoordinator(store, files, exporter)
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

func TestCampaignControllerWithPublicationPublishesQueuedAssignments(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(store, files, exporter)
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
