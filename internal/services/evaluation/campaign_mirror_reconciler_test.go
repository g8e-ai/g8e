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
	"testing/synctest"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func completedVerifiedMirrorTestCampaign(t *testing.T, store *Store) *evalv1.EvaluationRun {
	t.Helper()
	run := completedTestCampaign(t, store)
	persisted, err := store.LoadRun(context.Background(), run.GetRunId())
	require.NoError(t, err)
	require.Nil(t, persisted.GetCompletedAt(), "campaign completion is derived from assignment records")
	summary, err := NewCampaignController(store, nil, nil, nil).RunSummary(context.Background(), run.GetRunId())
	require.NoError(t, err)
	require.Equal(t, "completed", CampaignRunStatus(summary, nil, false))
	report := buildBoundTestReport(t, store, persisted, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS)
	require.NoError(t, store.SaveCampaignVerification(context.Background(), run.GetRunId(), report))
	return persisted
}

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
	synctest.Test(t, func(t *testing.T) {
		files := newCampaignMemoryFileService()
		store := NewStore(files)
		exporter := &recordingCampaignFeedExporter{}
		coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
		reconciler := NewCampaignMirrorReconciler(coordinator, store, blockingCampaignMirrorProbe{})
		run := completedVerifiedMirrorTestCampaign(t, store)
		queue := &CampaignQueue{Models: []CampaignQueueModel{{Status: "verified", VerifiedRunID: run.GetRunId()}}}
		var progress []CampaignMirrorReconcileProgress

		result, err := reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Millisecond, true, false, func(update CampaignMirrorReconcileProgress) {
			progress = append(progress, update)
		})

		require.NoError(t, err)
		assert.Contains(t, result.FailedRuns[run.GetRunId()], context.DeadlineExceeded.Error())
		require.Len(t, progress, 2)
		assert.Equal(t, CampaignMirrorReconcileChecking, progress[0].Status)
		assert.Equal(t, CampaignMirrorReconcileFailed, progress[1].Status)
		assert.ErrorIs(t, progress[1].Err, context.DeadlineExceeded)
	})
}

func TestCampaignMirrorReconcilerRestoresMissingVerifiedRuns(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
	run := completedVerifiedMirrorTestCampaign(t, store)
	report := buildBoundTestReport(t, store, run, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS)
	require.NoError(t, store.SaveCampaignVerification(context.Background(), run.GetRunId(), report))

	before := len(exporter.records)
	_, err := coordinator.PublishRunCatchUpWithVerification(context.Background(), run.GetRunId(), report)
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
	result, err := reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Minute, true, false, nil)
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
	result, err = reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Minute, true, false, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{run.GetRunId()}, result.RestoredRunIDs)
	assert.Greater(t, len(exporter.records), 0)
}

func TestCampaignMirrorReconciler_ReconcileVerifiedQueueForceSkipsPresentDataset(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
	run := completedVerifiedMirrorTestCampaign(t, store)
	report := buildBoundTestReport(t, store, run, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS)
	require.NoError(t, store.SaveCampaignVerification(context.Background(), run.GetRunId(), report))

	_, err := coordinator.PublishRunCatchUpWithVerification(context.Background(), run.GetRunId(), report)
	require.NoError(t, err)
	probe.present[CampaignDatasetID(run.GetRunId())] = true
	before := len(exporter.records)

	reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)
	queue := &CampaignQueue{Models: []CampaignQueueModel{{Status: "verified", VerifiedRunID: run.GetRunId()}}}
	result, err := reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Minute, true, true, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{run.GetRunId()}, result.SkippedRunIDs)
	assert.Empty(t, result.RestoredRunIDs)
	assert.Empty(t, result.RepublishedRunIDs)
	assert.Len(t, exporter.records, before)
}

func TestCampaignMirrorReconciler_ReconcileRunRestoresMissingDataset(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
	run := completedVerifiedMirrorTestCampaign(t, store)

	exporter.records = nil
	reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)
	published, err := reconciler.ReconcileRun(context.Background(), run.GetRunId(), false)
	require.NoError(t, err)
	assert.Greater(t, published, 0)
	assert.Greater(t, len(exporter.records), 0)
}

func TestCampaignMirrorReconciler_ReconcileRunSkipsPresentDataset(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	run := completedVerifiedMirrorTestCampaign(t, store)
	runID := run.GetRunId()
	probe := &stubCampaignMirrorProbe{present: map[string]bool{CampaignDatasetID(runID): true}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
	reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)

	published, err := reconciler.ReconcileRun(context.Background(), runID, false)
	require.NoError(t, err)
	assert.Zero(t, published)
	assert.Empty(t, exporter.records)
}

func TestCampaignMirrorReconciler_ReconcileRunForceRepublishesPresentDataset(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
	run := completedVerifiedMirrorTestCampaign(t, store)

	_, err := coordinator.PublishRunCatchUp(context.Background(), run.GetRunId())
	require.NoError(t, err)
	probe.present[CampaignDatasetID(run.GetRunId())] = true
	before := len(exporter.records)

	reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)
	published, err := reconciler.ReconcileRun(context.Background(), run.GetRunId(), true)
	require.NoError(t, err)
	assert.Greater(t, published, 0)
	assert.Greater(t, len(exporter.records), before)
}

func TestCampaignPublicationCoordinatorResetsIdempotencyWhenMirrorDatasetMissing(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
	run := completedVerifiedMirrorTestCampaign(t, store)

	_, err := coordinator.PublishRunCatchUp(context.Background(), run.GetRunId())
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

func TestCampaignMirrorReconciler_EligibilityUsesCanonicalCampaignState(t *testing.T) {
	for _, source := range []string{"queue", "store"} {
		for _, state := range []string{"verified", "unfinished", "unverified", "verification_failed", "corrupt_verification"} {
			t.Run(source+"/"+state, func(t *testing.T) {
				ctx := context.Background()
				files := newCampaignMemoryFileService()
				store := NewStore(files)
				run := completedVerifiedMirrorTestCampaign(t, store)
				runID := run.GetRunId()
				switch state {
				case "unfinished":
					assignments, err := store.ListAssignments(ctx, runID)
					require.NoError(t, err)
					assignment := assignments[0]
					require.NoError(t, files.Remove(ctx, store.layout.assignmentResultPath(runID, assignment.GetAssignmentId())))
					assignment.LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED
					require.NoError(t, store.SaveAssignment(ctx, assignment))
				case "unverified":
					require.NoError(t, files.Remove(ctx, store.layout.campaignVerificationPath(runID)))
				case "verification_failed":
					report := buildBoundTestReport(t, store, run, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL)
					require.NoError(t, store.SaveCampaignVerification(ctx, runID, report))
				case "corrupt_verification":
					require.NoError(t, files.WriteFile(ctx, store.layout.campaignVerificationPath(runID), []byte("invalid"), constants.PermFilePrivate))
				}
				exporter := &recordingCampaignFeedExporter{}
				probe := &stubCampaignMirrorProbe{present: map[string]bool{}}
				coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil).WithMirrorProbe(probe)
				reconciler := NewCampaignMirrorReconciler(coordinator, store, probe)
				var updates []CampaignMirrorReconcileProgress
				progress := func(update CampaignMirrorReconcileProgress) { updates = append(updates, update) }
				var result *CampaignMirrorReconcileResult
				var err error
				if source == "queue" {
					queue := &CampaignQueue{Models: []CampaignQueueModel{{Status: "verified", VerifiedRunID: runID}}}
					result, err = reconciler.ReconcileVerifiedQueue(ctx, queue, time.Minute, true, true, progress)
				} else {
					result, err = reconciler.ReconcileAllVerifiedRunsFromStore(ctx, time.Minute, true, true, progress)
				}
				require.NoError(t, err)
				require.Len(t, updates, 2)
				if state == "verified" {
					assert.Equal(t, []string{runID}, result.RestoredRunIDs)
					assert.NotEmpty(t, exporter.records)
				} else {
					assert.Empty(t, exporter.records, "force must preserve eligibility requirements")
					assert.Empty(t, result.RestoredRunIDs)
					if state == "corrupt_verification" {
						assert.ErrorIs(t, updates[1].Err, constants.ErrEvidenceArtifactMalformed)
						assert.Contains(t, result.FailedRuns, runID)
					} else {
						assert.Equal(t, []string{runID}, result.IneligibleRunIDs)
						assert.Empty(t, result.FailedRuns)
					}
					_, err := reconciler.ReconcileRun(ctx, runID, true)
					require.Error(t, err)
					assert.Empty(t, exporter.records)
				}
			})
		}
	}
}

func TestCampaignMirrorReconciler_QueueReportsMissingHostEvidence(t *testing.T) {
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	reconciler := NewCampaignMirrorReconciler(coordinator, store, blockingCampaignMirrorProbe{})
	queue := &CampaignQueue{Models: []CampaignQueueModel{{Status: "verified", VerifiedRunID: "run-absent"}}}
	result, err := reconciler.ReconcileVerifiedQueue(context.Background(), queue, time.Minute, true, true, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"run-absent"}, result.HostAbsentRunIDs)
	assert.Empty(t, result.FailedRuns)
	assert.Empty(t, exporter.records)
}
