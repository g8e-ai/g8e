// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"time"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// CampaignMirrorProbe reports whether one explorer dataset is present in the
// gateway-owned public mirror feed.
type CampaignMirrorProbe interface {
	DatasetPresent(ctx context.Context, datasetID string) (bool, error)
}

// CampaignMirrorReconcileResult summarizes one queue or run reconcile pass.
type CampaignMirrorReconcileResult struct {
	RestoredRunIDs   []string          `json:"restored_run_ids"`
	SkippedRunIDs    []string          `json:"skipped_run_ids"`
	HostAbsentRunIDs []string          `json:"host_absent_run_ids"`
	FailedRuns       map[string]string `json:"failed_runs,omitempty"`
	PublishedRecords int               `json:"published_records"`
}

type CampaignMirrorReconcileStatus string

const (
	CampaignMirrorReconcileChecking   CampaignMirrorReconcileStatus = "checking"
	CampaignMirrorReconcileRestored   CampaignMirrorReconcileStatus = "restored"
	CampaignMirrorReconcilePresent    CampaignMirrorReconcileStatus = "already_present"
	CampaignMirrorReconcileHostAbsent CampaignMirrorReconcileStatus = "host_absent"
	CampaignMirrorReconcileFailed     CampaignMirrorReconcileStatus = "failed"
)

type CampaignMirrorReconcileProgress struct {
	Index            int
	Total            int
	RunID            string
	Status           CampaignMirrorReconcileStatus
	PublishedRecords int
	Err              error
}

type CampaignMirrorReconcileProgressFunc func(CampaignMirrorReconcileProgress)

// CampaignMirrorReconciler compares queue-verified runs against the public
// mirror and republishes canonical host artifacts when datasets are missing.
type CampaignMirrorReconciler struct {
	publication *CampaignPublicationCoordinator
	store       CampaignStore
	probe       CampaignMirrorProbe
}

// NewCampaignMirrorReconciler wires one publication coordinator and optional
// mirror probe for drift detection.
func NewCampaignMirrorReconciler(publication *CampaignPublicationCoordinator, store CampaignStore, probe CampaignMirrorProbe) *CampaignMirrorReconciler {
	return &CampaignMirrorReconciler{
		publication: publication,
		store:       store,
		probe:       probe,
	}
}

// ReconcileVerifiedQueue restores every verified queue entry whose canonical
// dataset is absent from the public mirror.
func (r *CampaignMirrorReconciler) ReconcileVerifiedQueue(ctx context.Context, queue *CampaignQueue, runTimeout time.Duration, progress CampaignMirrorReconcileProgressFunc) (*CampaignMirrorReconcileResult, error) {
	if r == nil || r.publication == nil || r.store == nil || queue == nil || runTimeout <= 0 {
		return nil, fmt.Errorf("evaluation: reconcile verified queue: missing required dependencies")
	}
	entries := queue.FilterByStatus("verified")
	runIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.VerifiedRunID != "" {
			runIDs = append(runIDs, entry.VerifiedRunID)
		}
	}
	result := &CampaignMirrorReconcileResult{FailedRuns: map[string]string{}}
	for index, runID := range runIDs {
		update := func(status CampaignMirrorReconcileStatus, published int, err error) {
			if progress != nil {
				progress(CampaignMirrorReconcileProgress{Index: index + 1, Total: len(runIDs), RunID: runID, Status: status, PublishedRecords: published, Err: err})
			}
		}
		update(CampaignMirrorReconcileChecking, 0, nil)
		runCtx, cancel := context.WithTimeout(ctx, runTimeout)
		if r.probe != nil {
			present, err := r.probe.DatasetPresent(runCtx, CampaignDatasetID(runID))
			if err != nil {
				cancel()
				result.FailedRuns[runID] = err.Error()
				update(CampaignMirrorReconcileFailed, 0, err)
				continue
			}
			if present {
				cancel()
				result.SkippedRunIDs = append(result.SkippedRunIDs, runID)
				update(CampaignMirrorReconcilePresent, 0, nil)
				continue
			}
		}
		exists, err := r.store.RunExists(runCtx, runID)
		if err != nil {
			cancel()
			result.FailedRuns[runID] = err.Error()
			update(CampaignMirrorReconcileFailed, 0, err)
			continue
		}
		if !exists {
			cancel()
			result.HostAbsentRunIDs = append(result.HostAbsentRunIDs, runID)
			update(CampaignMirrorReconcileHostAbsent, 0, nil)
			continue
		}
		published, err := r.restoreRun(runCtx, runID)
		cancel()
		if err != nil {
			result.FailedRuns[runID] = err.Error()
			update(CampaignMirrorReconcileFailed, published, err)
			continue
		}
		result.RestoredRunIDs = append(result.RestoredRunIDs, runID)
		result.PublishedRecords += published
		update(CampaignMirrorReconcileRestored, published, nil)
	}
	return result, nil
}

// ReconcileRun restores one run when its dataset is missing from the mirror.
func (r *CampaignMirrorReconciler) ReconcileRun(ctx context.Context, runID string) (int, error) {
	if r == nil || r.publication == nil || r.store == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: reconcile run: missing required dependencies")
	}
	if r.probe != nil {
		present, err := r.probe.DatasetPresent(ctx, CampaignDatasetID(runID))
		if err != nil {
			return 0, err
		}
		if present {
			return 0, nil
		}
	}
	return r.restoreRun(ctx, runID)
}

func (r *CampaignMirrorReconciler) restoreRun(ctx context.Context, runID string) (int, error) {
	report, err := r.store.LoadCampaignVerification(ctx, runID)
	if err != nil || legacyCampaignVerificationReport(report) {
		return r.publication.PublishRunCatchUp(ctx, runID)
	}
	return r.publication.PublishRunCatchUpWithVerification(ctx, runID, report)
}

func legacyCampaignVerificationReport(report *evalv1.EvaluationVerificationReport) bool {
	return report != nil && report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS && report.GetVerifiedPopulationDigest() == ""
}
