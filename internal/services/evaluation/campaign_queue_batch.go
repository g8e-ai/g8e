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
)

// CampaignQueueBatchPlanRequest selects queue entries for unattended rollout.
type CampaignQueueBatchPlanRequest struct {
	SkipVerified bool
	// VerifiedIsStale reports a verified entry whose evidence is not for the
	// current scenario catalog. SkipVerified never skips such an entry. Nil means
	// no verified entry is stale.
	VerifiedIsStale func(CampaignQueueModel) bool
	// Until bounds the batch to this many entries. Zero means no bound.
	Until int
}

// BuildBatchPlan returns queue entries to execute in order. Skipped entries
// never run.
func (queue *CampaignQueue) BuildBatchPlan(req CampaignQueueBatchPlanRequest) []CampaignQueueModel {
	if queue == nil {
		return nil
	}
	plan := make([]CampaignQueueModel, 0, len(queue.Models))
	for _, entry := range queue.Models {
		if strings.EqualFold(entry.Status, QueueStatusSkipped) {
			continue
		}
		if req.SkipVerified && strings.EqualFold(entry.Status, QueueStatusVerified) && (req.VerifiedIsStale == nil || !req.VerifiedIsStale(entry)) {
			continue
		}
		plan = append(plan, entry)
		if req.Until > 0 && len(plan) == req.Until {
			break
		}
	}
	return plan
}

// StaleVerifiedVariants returns the variant IDs of the queue's verified entries
// whose verified run did not score the catalog with catalogDigest. A verified
// entry is a claim about the catalog its run froze, so a catalog change makes it
// stale. An entry with no run, or whose run or campaign is no longer on disk,
// has no evidence that it is current and is stale too. Any other read failure is
// returned and decides nothing.
func StaleVerifiedVariants(ctx context.Context, store *Store, queue *CampaignQueue, catalogDigest string) (map[string]struct{}, error) {
	stale := make(map[string]struct{})
	if queue == nil {
		return stale, nil
	}
	for _, entry := range queue.Models {
		if !strings.EqualFold(entry.Status, QueueStatusVerified) {
			continue
		}
		verifiedDigest, err := verifiedCatalogDigest(ctx, store, entry)
		if err != nil {
			return nil, fmt.Errorf("evaluation: stale verified variants: %s: %w", entry.VariantID, err)
		}
		if verifiedDigest != catalogDigest {
			stale[entry.VariantID] = struct{}{}
		}
	}
	return stale, nil
}

// verifiedCatalogDigest returns the catalog digest frozen by the campaign of the
// entry's verified run, or "" when that run or campaign cannot be found.
func verifiedCatalogDigest(ctx context.Context, store *Store, entry CampaignQueueModel) (string, error) {
	if entry.VerifiedRunID == "" {
		return "", nil
	}
	run, err := store.LoadRun(ctx, entry.VerifiedRunID)
	if err != nil {
		if isNotFound(err) {
			return "", nil
		}
		return "", err
	}
	spec, err := store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		if isNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return spec.GetCatalogDigest(), nil
}

// StrictWitnessVerifyNotes returns the standard queue note for a strict-witness verified run.
func StrictWitnessVerifyNotes(runID string) string {
	return "witness verify PASS (--require-provider-observation --require-model-provenance); run " + runID
}
