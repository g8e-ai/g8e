// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// CampaignWitnessStatus summarizes enrolled witness operators for scored campaigns.
type CampaignWitnessStatus struct {
	ActiveObserverCount   int  `json:"active_observer_count"`
	ActiveProvenanceCount int  `json:"active_provenance_count"`
	Ready                 bool `json:"ready"`
}

// CampaignWitnessStatusFromOperators counts active observer and provenance sessions.
func CampaignWitnessStatusFromOperators(operators []models.OperatorDocumentGo) CampaignWitnessStatus {
	status := CampaignWitnessStatus{}
	for _, op := range operators {
		if op.Status != constants.OperatorStatusActive {
			continue
		}
		if op.RuntimeConfig == nil {
			continue
		}
		if op.RuntimeConfig.ProviderBoundaryObserverEnabled {
			status.ActiveObserverCount++
		}
		if op.RuntimeConfig.ProvenanceOperatorEnabled {
			status.ActiveProvenanceCount++
		}
	}
	status.Ready = status.ActiveObserverCount >= 1 && status.ActiveProvenanceCount >= 1
	return status
}

// CampaignQueueBatchPlanRequest selects queue entries for unattended rollout.
type CampaignQueueBatchPlanRequest struct {
	SkipVariantIDs []string
	SkipVerified   bool
}

// BuildBatchPlan returns queue entries to execute in order.
func (queue *CampaignQueue) BuildBatchPlan(req CampaignQueueBatchPlanRequest) []CampaignQueueModel {
	if queue == nil {
		return nil
	}
	skip := make(map[string]struct{}, len(req.SkipVariantIDs))
	for _, variantID := range req.SkipVariantIDs {
		variantID = strings.TrimSpace(variantID)
		if variantID == "" {
			continue
		}
		skip[variantID] = struct{}{}
	}
	plan := make([]CampaignQueueModel, 0, len(queue.Models))
	for _, entry := range queue.Models {
		if _, excluded := skip[entry.VariantID]; excluded {
			continue
		}
		if req.SkipVerified && strings.EqualFold(entry.Status, "verified") {
			continue
		}
		plan = append(plan, entry)
	}
	return plan
}

// TierAVerifyNotes returns the standard queue note for a Tier-A verified run.
func TierAVerifyNotes(runID string) string {
	return "75/75 Tier-A verify PASS (--require-provider-observation --require-model-provenance); run " + runID
}
