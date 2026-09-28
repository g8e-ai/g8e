// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"strings"
)

// CampaignQueueBatchPlanRequest selects queue entries for unattended rollout.
type CampaignQueueBatchPlanRequest struct {
	SkipVerified bool
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
		if req.SkipVerified && strings.EqualFold(entry.Status, QueueStatusVerified) {
			continue
		}
		plan = append(plan, entry)
		if req.Until > 0 && len(plan) == req.Until {
			break
		}
	}
	return plan
}

// StrictWitnessVerifyNotes returns the standard queue note for a strict-witness verified run.
func StrictWitnessVerifyNotes(runID string) string {
	return "75/75 witness verify PASS (--require-provider-observation --require-model-provenance); run " + runID
}
