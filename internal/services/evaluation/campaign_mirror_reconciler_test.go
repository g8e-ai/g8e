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

type stubCampaignMirrorProbe struct {
	present map[string]bool
}

func (s *stubCampaignMirrorProbe) DatasetPresent(_ context.Context, datasetID string) (bool, error) {
	if s == nil || s.present == nil {
		return false, nil
	}
	return s.present[datasetID], nil
}

type blockingCampaignMirrorProbe struct{}

func (blockingCampaignMirrorProbe) DatasetPresent(ctx context.Context, _ string) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

func TestCampaignMirrorReconciler_ReconcileVerifiedQueueReportsProgressAndBoundsEachRun(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	reconciler := NewCampaignMirrorReconciler(coordinator, store, blockingCampaignMirrorProbe{})
	queue := &CampaignQueue{Models: []CampaignQueueModel{{Status: "verified", VerifiedRunID: "run-blocked"}}}
	var progress []CampaignMirrorReconcileProgress

	result, err := reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Millisecond, true, func(update CampaignMirrorReconcileProgress) {
		progress = append(progress, update)
	})

	require.NoError(t, err)
	assert.Contains(t, result.FailedRuns["run-blocked"], context.DeadlineExceeded.Error())
	require.Len(t, progress, 2)
	assert.Equal(t, CampaignMirrorReconcileChecking, progress[0].Status)
	assert.Equal(t, CampaignMirrorReconcileFailed, progress[1].Status)
	assert.ErrorIs(t, progress[1].Err, context.DeadlineExceeded)
}

func TestCampaignMirrorReconcilerRestoresMissingVerifiedRuns(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
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
	report := buildBoundTestReport(t, store, run, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS)
	require.NoError(t, store.SaveCampaignVerification(context.Background(), run.GetRunId(), report))

	before := len(exporter.records)
	_, err = coordinator.PublishRunCatchUpWithVerification(context.Background(), run.GetRunId(), report)
	require.NoError(t, err)
	assert.Greater(t, len(exporter.records), before)

	exporter.records = nil
	reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)
	queue := &CampaignQueue{
		Models: []CampaignQueueModel{{
			VariantID:     "gemma4-e4b",
			Status:        "verified",
			VerifiedRunID: run.GetRunId(),
		}},
	}
	result, err := reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Minute, true, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{run.GetRunId()}, result.RestoredRunIDs)
	assert.Greater(t, len(exporter.records), 0)

	legacyReport := &evalv1.EvaluationVerificationReport{
		SchemaVersion: CampaignSchemaVersion,
		ReportId:      run.GetRunId(),
		RunId:         run.GetRunId(),
		Status:        evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		VerifiedAt:    timestamppb.New(time.Unix(1_700_000_300, 0).UTC()),
	}
	require.NoError(t, store.SaveCampaignVerification(context.Background(), run.GetRunId(), legacyReport))
	exporter.records = nil
	result, err = reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Minute, true, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{run.GetRunId()}, result.RestoredRunIDs)
	assert.Greater(t, len(exporter.records), 0)
}

func TestCampaignMirrorReconciler_ReconcileRunRestoresMissingDataset(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
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

	exporter.records = nil
	reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)
	published, err := reconciler.ReconcileRun(context.Background(), run.GetRunId())
	require.NoError(t, err)
	assert.Greater(t, published, 0)
	assert.Greater(t, len(exporter.records), 0)
}

func TestCampaignMirrorReconciler_ReconcileRunSkipsPresentDataset(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	runID := "run-present"
	probe := &stubCampaignMirrorProbe{present: map[string]bool{CampaignDatasetID(runID): true}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
	reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)

	published, err := reconciler.ReconcileRun(context.Background(), runID)
	require.NoError(t, err)
	assert.Zero(t, published)
	assert.Empty(t, exporter.records)
}

func TestCampaignPublicationCoordinatorResetsIdempotencyWhenMirrorDatasetMissing(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
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
	require.Greater(t, before, 0)

	probe.present[CampaignDatasetID(run.GetRunId())] = true
	_, err = coordinator.PublishRunCatchUp(context.Background(), run.GetRunId())
	require.NoError(t, err)
	assert.Len(t, exporter.records, before)

	probe.present[CampaignDatasetID(run.GetRunId())] = false
	_, err = coordinator.PublishRunCatchUp(context.Background(), run.GetRunId())
	require.NoError(t, err)
	assert.Greater(t, len(exporter.records), before)
}
