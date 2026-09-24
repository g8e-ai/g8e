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
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// AbandonedCampaignRun summarizes one scheduled campaign run with no terminal
// progress that is safe to discard.
type AbandonedCampaignRun struct {
	RunID              string
	CampaignID         string
	Scheduled          uint32
	Queued             uint32
	QueueVariantID     string
	QueueStatus        string
	ProtectedByQueue   bool
	ProtectionReason   string
}

// ListAbandonedCampaignRunsRequest selects abandoned rollout runs on the host.
type ListAbandonedCampaignRunsRequest struct {
	Context            context.Context
	Store              *Store
	Controller         *CampaignController
	Queue              *CampaignQueue
	ProtectedRunIDs    map[string]string
	IncludeRunID       string
}

// DiscardAbandonedCampaignRunsRequest removes abandoned host evidence and clears
// queue references for matching runs.
type DiscardAbandonedCampaignRunsRequest struct {
	Context         context.Context
	FileService     fs.RuntimeFileService
	Store           *Store
	Queue           *CampaignQueue
	QueuePath       string
	Runs            []AbandonedCampaignRun
	WithdrawMirror  func(ctx context.Context, runID string) error
}

// DiscardAbandonedCampaignRunsResult reports one discard pass.
type DiscardAbandonedCampaignRunsResult struct {
	DiscardedRunIDs []string `json:"discarded_run_ids"`
	QueueUpdates    int      `json:"queue_updates"`
	MirrorWithdrawn int      `json:"mirror_withdrawn"`
}

// ProtectedRunIDsFromQueue returns verified rollout run IDs that must never be
// discarded automatically.
func ProtectedRunIDsFromQueue(queue *CampaignQueue) map[string]string {
	protected := make(map[string]string)
	if queue == nil {
		return protected
	}
	for _, entry := range queue.Models {
		if !strings.EqualFold(entry.Status, "verified") || entry.VerifiedRunID == "" {
			continue
		}
		protected[entry.VerifiedRunID] = entry.VariantID
	}
	return protected
}

// QueueReferenceForRun returns queue metadata for one run ID when present.
func QueueReferenceForRun(queue *CampaignQueue, runID string) (variantID, status string) {
	if queue == nil || runID == "" {
		return "", ""
	}
	for _, entry := range queue.Models {
		if entry.VerifiedRunID == runID {
			return entry.VariantID, entry.Status
		}
	}
	return "", ""
}

// IsAbandonedCampaignSummary reports whether one run was scheduled but never
// started executing or reaching a terminal outcome.
func IsAbandonedCampaignSummary(summary *CampaignRunSummary) bool {
	if summary == nil || summary.Run == nil {
		return false
	}
	if summary.RunningCount > 0 || summary.TerminalCount > 0 {
		return false
	}
	scheduled := summary.QueuedCount + summary.RunningCount + summary.TerminalCount
	return scheduled > 0
}

// ListAbandonedCampaignRuns returns host campaign runs that were scheduled but
// never made execution progress.
func ListAbandonedCampaignRuns(req ListAbandonedCampaignRunsRequest) ([]AbandonedCampaignRun, error) {
	if req.Store == nil || req.Controller == nil {
		return nil, fmt.Errorf("evaluation: list abandoned campaign runs: %w", constants.ErrMissingRequiredField)
	}
	if req.Context == nil {
		req.Context = context.Background()
	}
	if req.ProtectedRunIDs == nil {
		req.ProtectedRunIDs = ProtectedRunIDsFromQueue(req.Queue)
	}
	inventory, err := req.Store.ListRunInventory(req.Context)
	if err != nil {
		return nil, fmt.Errorf("evaluation: list abandoned campaign runs: %w", err)
	}
	abandoned := make([]AbandonedCampaignRun, 0)
	for _, entry := range inventory {
		if entry.Kind != RunKindCampaign {
			continue
		}
		if req.IncludeRunID != "" && entry.RunID != req.IncludeRunID {
			continue
		}
		summary, err := req.Controller.RunSummary(req.Context, entry.RunID)
		if err != nil {
			return nil, fmt.Errorf("evaluation: list abandoned campaign runs: %w", err)
		}
		if !IsAbandonedCampaignSummary(summary) {
			continue
		}
		variantID, queueStatus := QueueReferenceForRun(req.Queue, entry.RunID)
		record := AbandonedCampaignRun{
			RunID:          entry.RunID,
			CampaignID:     summary.Run.GetCampaignBinding().GetCampaignId(),
			Scheduled:      summary.QueuedCount + summary.RunningCount + summary.TerminalCount,
			Queued:         summary.QueuedCount,
			QueueVariantID: variantID,
			QueueStatus:    queueStatus,
		}
		if variantID, ok := req.ProtectedRunIDs[entry.RunID]; ok {
			record.ProtectedByQueue = true
			record.ProtectionReason = "verified queue entry for variant " + variantID
			continue
		}
		abandoned = append(abandoned, record)
	}
	return abandoned, nil
}

// DeleteCampaignRun removes canonical host evidence for one campaign run.
func (s *Store) DeleteCampaignRun(ctx context.Context, runID string) error {
	if s == nil || s.files == nil || runID == "" {
		return fmt.Errorf("evaluation: delete campaign run: %w", constants.ErrMissingRequiredField)
	}
	runDir := evaluationRunDir(runID)
	exists, err := s.files.FileExists(ctx, runDir)
	if err != nil {
		return fmt.Errorf("evaluation: delete campaign run: %w", err)
	}
	if !exists {
		return nil
	}
	if err := s.files.RemoveAll(ctx, runDir); err != nil {
		return fmt.Errorf("evaluation: delete campaign run: %w", err)
	}
	return nil
}

// ClearDiscardedRunReference resets one non-verified queue entry that still
// points at a discarded run.
func (queue *CampaignQueue) ClearDiscardedRunReference(runID, note string) bool {
	if queue == nil || runID == "" {
		return false
	}
	updated := false
	for index, entry := range queue.Models {
		if entry.VerifiedRunID != runID || strings.EqualFold(entry.Status, "verified") {
			continue
		}
		queue.Models[index].VerifiedRunID = ""
		queue.Models[index].Status = "pending"
		if note != "" {
			queue.Models[index].Notes = note
		}
		updated = true
	}
	return updated
}

// ClearActiveCampaignRunIfMatch removes the active-run marker when it still
// references one discarded run.
func ClearActiveCampaignRunIfMatch(ctx context.Context, fileSvc fs.RuntimeFileService, runID string) error {
	if fileSvc == nil || runID == "" {
		return nil
	}
	active, err := LoadActiveCampaignRunFromRuntime(ctx, fileSvc)
	if err != nil {
		return nil
	}
	if active.RunID != runID {
		return nil
	}
	return fileSvc.Remove(ctx, constants.EvaluationActiveRunPath)
}

// DiscardAbandonedCampaignRuns deletes host evidence, clears queue references,
// and withdraws public mirror datasets for each abandoned run.
func DiscardAbandonedCampaignRuns(req DiscardAbandonedCampaignRunsRequest) (*DiscardAbandonedCampaignRunsResult, error) {
	if req.Store == nil || req.FileService == nil || len(req.Runs) == 0 {
		return &DiscardAbandonedCampaignRunsResult{}, nil
	}
	if req.Context == nil {
		req.Context = context.Background()
	}
	result := &DiscardAbandonedCampaignRunsResult{}
	for _, run := range req.Runs {
		if run.ProtectedByQueue {
			return nil, fmt.Errorf("evaluation: discard abandoned campaign run: protected run %s (%s)", run.RunID, run.ProtectionReason)
		}
		if err := req.Store.DeleteCampaignRun(req.Context, run.RunID); err != nil {
			return nil, err
		}
		if req.Queue != nil && req.Queue.ClearDiscardedRunReference(run.RunID, "discarded abandoned run "+run.RunID) {
			result.QueueUpdates++
		}
		if err := ClearActiveCampaignRunIfMatch(req.Context, req.FileService, run.RunID); err != nil {
			return nil, err
		}
		if req.WithdrawMirror != nil {
			if err := req.WithdrawMirror(req.Context, run.RunID); err != nil {
				return nil, fmt.Errorf("evaluation: discard abandoned campaign run %s: withdraw mirror dataset: %w", run.RunID, err)
			}
			result.MirrorWithdrawn++
		}
		result.DiscardedRunIDs = append(result.DiscardedRunIDs, run.RunID)
	}
	if req.Queue != nil && req.QueuePath != "" && result.QueueUpdates > 0 {
		if err := SaveInitCampaignQueueToRuntime(req.Context, req.FileService, req.QueuePath, req.Queue); err != nil {
			return nil, err
		}
	}
	return result, nil
}
