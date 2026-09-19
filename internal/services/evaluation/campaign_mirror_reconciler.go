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
func (r *CampaignMirrorReconciler) ReconcileVerifiedQueue(ctx context.Context, queue *CampaignQueue) (*CampaignMirrorReconcileResult, error) {
	if r == nil || r.publication == nil || r.store == nil || queue == nil {
		return nil, fmt.Errorf("evaluation: reconcile verified queue: missing required dependencies")
	}
	result := &CampaignMirrorReconcileResult{FailedRuns: map[string]string{}}
	for _, entry := range queue.FilterByStatus("verified") {
		runID := entry.VerifiedRunID
		if runID == "" {
			continue
		}
		if r.probe != nil {
			present, err := r.probe.DatasetPresent(ctx, CampaignDatasetID(runID))
			if err != nil {
				result.FailedRuns[runID] = err.Error()
				continue
			}
			if present {
				result.SkippedRunIDs = append(result.SkippedRunIDs, runID)
				continue
			}
		}
		exists, err := r.store.RunExists(ctx, runID)
		if err != nil {
			result.FailedRuns[runID] = err.Error()
			continue
		}
		if !exists {
			result.HostAbsentRunIDs = append(result.HostAbsentRunIDs, runID)
			continue
		}
		published, err := r.restoreRun(ctx, runID)
		if err != nil {
			result.FailedRuns[runID] = err.Error()
			continue
		}
		result.RestoredRunIDs = append(result.RestoredRunIDs, runID)
		result.PublishedRecords += published
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
	if err != nil {
		return r.publication.PublishRunCatchUp(ctx, runID)
	}
	return r.publication.PublishRunCatchUpWithVerification(ctx, runID, report)
}
