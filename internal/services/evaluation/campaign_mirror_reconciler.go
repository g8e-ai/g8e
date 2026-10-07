// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// CampaignMirrorProbe reports whether one explorer dataset is present in the
// gateway-owned public mirror feed.
type CampaignMirrorProbe interface {
	DatasetPresent(ctx context.Context, datasetID string) (bool, error)
}

// CampaignMirrorReconcileResult summarizes one queue or run reconcile pass.
type CampaignMirrorReconcileResult struct {
	RestoredRunIDs    []string          `json:"restored_run_ids"`
	RepublishedRunIDs []string          `json:"republished_run_ids,omitempty"`
	SkippedRunIDs     []string          `json:"skipped_run_ids"`
	MissingRunIDs     []string          `json:"missing_run_ids,omitempty"`
	IneligibleRunIDs  []string          `json:"ineligible_run_ids,omitempty"`
	HostAbsentRunIDs  []string          `json:"host_absent_run_ids"`
	FailedRuns        map[string]string `json:"failed_runs,omitempty"`
	PublishedRecords  int               `json:"published_records"`
}

type CampaignMirrorReconcileStatus string

const (
	CampaignMirrorReconcileIneligible  CampaignMirrorReconcileStatus = "ineligible"
	CampaignMirrorReconcileChecking    CampaignMirrorReconcileStatus = "checking"
	CampaignMirrorReconcileRestored    CampaignMirrorReconcileStatus = "restored"
	CampaignMirrorReconcileRepublished CampaignMirrorReconcileStatus = "republished"
	CampaignMirrorReconcilePresent     CampaignMirrorReconcileStatus = "already_present"
	CampaignMirrorReconcileMissing     CampaignMirrorReconcileStatus = "missing"
	CampaignMirrorReconcileHostAbsent  CampaignMirrorReconcileStatus = "host_absent"
	CampaignMirrorReconcileFailed      CampaignMirrorReconcileStatus = "failed"
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

// ReconcileAllVerifiedRunsFromStore walks the run store and reconciles all runs
// that have valid verification reports. When restoreMissing is true, it
// republishes canonical host artifacts for datasets that are absent from the
// mirror. This is used as an alternative to ReconcileVerifiedQueue when the
// campaign queue is not available (e.g., on restored backups).
func (r *CampaignMirrorReconciler) ReconcileAllVerifiedRunsFromStore(ctx context.Context, runTimeout time.Duration, restoreMissing bool, force bool, progress CampaignMirrorReconcileProgressFunc) (*CampaignMirrorReconcileResult, error) {
	if r == nil || r.publication == nil || r.store == nil || runTimeout <= 0 {
		return nil, fmt.Errorf("evaluation: reconcile verified runs from store: dependencies: %w", constants.ErrMissingRequiredField)
	}
	runIDs, err := r.store.ListRunIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluation: reconcile verified runs from store: list runs: %w", err)
	}
	return r.reconcileRuns(ctx, runIDs, runTimeout, restoreMissing, force, progress)
}

// ReconcileVerifiedQueue compares verified queue entries against the public
// mirror. When restoreMissing is true, it republishes canonical host artifacts
// for datasets that are absent from the mirror. Present datasets are always
// skipped so queue restores can resume after partial failures. When force is
// true, host publication idempotency is cleared before restoring each missing
// dataset. Use ReconcileRun with force to republish one already-present run.
func (r *CampaignMirrorReconciler) ReconcileVerifiedQueue(ctx context.Context, queue *CampaignQueue, runTimeout time.Duration, restoreMissing bool, force bool, progress CampaignMirrorReconcileProgressFunc) (*CampaignMirrorReconcileResult, error) {
	if r == nil || r.publication == nil || r.store == nil || queue == nil || runTimeout <= 0 {
		return nil, fmt.Errorf("evaluation: reconcile verified queue: dependencies: %w", constants.ErrMissingRequiredField)
	}
	entries := queue.FilterByStatus("verified")
	runIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.VerifiedRunID == "" {
			continue
		}
		runIDs = append(runIDs, entry.VerifiedRunID)
	}
	return r.reconcileRuns(ctx, runIDs, runTimeout, restoreMissing, force, progress)
}

func (r *CampaignMirrorReconciler) reconcileRuns(ctx context.Context, runIDs []string, runTimeout time.Duration, restoreMissing, force bool, progress CampaignMirrorReconcileProgressFunc) (*CampaignMirrorReconcileResult, error) {
	result := &CampaignMirrorReconcileResult{FailedRuns: map[string]string{}}
	for index, runID := range runIDs {
		update := func(status CampaignMirrorReconcileStatus, published int, err error) {
			if progress != nil {
				progress(CampaignMirrorReconcileProgress{Index: index + 1, Total: len(runIDs), RunID: runID, Status: status, PublishedRecords: published, Err: err})
			}
		}
		update(CampaignMirrorReconcileChecking, 0, nil)
		effectiveTimeout := runTimeout
		if restoreMissing {
			effectiveTimeout = CampaignMirrorReconcileRunTimeout(runTimeout, assignmentCountForMirrorTimeout(ctx, r.store, runID))
		}
		runCtx, cancel := context.WithTimeout(ctx, effectiveTimeout)
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
		eligible, err := r.completedAndVerified(runCtx, runID)
		if err != nil {
			cancel()
			result.FailedRuns[runID] = err.Error()
			update(CampaignMirrorReconcileFailed, 0, err)
			continue
		}
		if !eligible {
			cancel()
			result.IneligibleRunIDs = append(result.IneligibleRunIDs, runID)
			update(CampaignMirrorReconcileIneligible, 0, nil)
			continue
		}
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
		if !restoreMissing {
			cancel()
			result.MissingRunIDs = append(result.MissingRunIDs, runID)
			update(CampaignMirrorReconcileMissing, 0, nil)
			continue
		}
		published, err := r.restoreRun(runCtx, runID, force)
		cancel()
		if err != nil {
			result.FailedRuns[runID] = err.Error()
			update(CampaignMirrorReconcileFailed, published, err)
			if campaignMirrorGatewayUnreachable(err) {
				for _, remainingRunID := range runIDs[index+1:] {
					result.FailedRuns[remainingRunID] = "skipped after gateway publication failure"
				}
				break
			}
			continue
		}
		result.RestoredRunIDs = append(result.RestoredRunIDs, runID)
		update(CampaignMirrorReconcileRestored, published, nil)
		result.PublishedRecords += published
	}
	return result, nil
}

// ReconcileRun restores one run when its dataset is missing from the mirror.
// When force is true, it republishes even when the dataset is already present.
func (r *CampaignMirrorReconciler) ReconcileRun(ctx context.Context, runID string, force bool) (int, error) {
	if r == nil || r.publication == nil || r.store == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: reconcile run: dependencies: %w", constants.ErrMissingRequiredField)
	}
	eligible, err := r.completedAndVerified(ctx, runID)
	if err != nil {
		return 0, err
	}
	if !eligible {
		return 0, fmt.Errorf("evaluation: run %q is not completed and verified: %w", runID, constants.ErrNotFound)
	}
	if r.probe != nil && !force {
		present, err := r.probe.DatasetPresent(ctx, CampaignDatasetID(runID))
		if err != nil {
			return 0, err
		}
		if present {
			return 0, nil
		}
	}
	return r.restoreRun(ctx, runID, force)
}

func (r *CampaignMirrorReconciler) completedAndVerified(ctx context.Context, runID string) (bool, error) {
	summary, err := NewCampaignController(r.store, nil, nil, nil).RunSummary(ctx, runID)
	if err != nil {
		return false, err
	}
	if CampaignRunStatus(summary, nil, false) != "completed" {
		return false, nil
	}
	report, err := r.store.LoadCampaignVerification(ctx, runID)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil || report == nil || report.GetStatus() != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		return false, err
	}
	return true, nil
}

func (r *CampaignMirrorReconciler) restoreRun(ctx context.Context, runID string, force bool) (int, error) {
	report, err := r.store.LoadCampaignVerification(ctx, runID)
	if err != nil {
		return 0, fmt.Errorf("evaluation: restore run verification: %w", err)
	}
	if report == nil || report.GetStatus() != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		return 0, fmt.Errorf("evaluation: restore run requires passing verification: %w", constants.ErrEvidenceScopeMismatch)
	}
	if force {
		if err := r.publication.ResetFeedPublicationIdempotency(ctx, runID); err != nil {
			return 0, err
		}
	}
	if legacyCampaignVerificationReport(report) {
		return r.publication.PublishRunCatchUp(ctx, runID)
	}
	return r.publication.PublishRunCatchUpWithVerification(ctx, runID, report)
}

func legacyCampaignVerificationReport(report *evalv1.EvaluationVerificationReport) bool {
	return report != nil && report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS && report.GetVerifiedPopulationDigest() == ""
}
