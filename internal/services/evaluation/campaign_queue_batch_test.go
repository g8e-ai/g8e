// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestCampaignQueueBuildBatchPlan(t *testing.T) {
	queue := &CampaignQueue{
		Models: []CampaignQueueModel{
			{VariantID: "granite3-3-2b", Status: "verified"},
			{VariantID: "qwen3-4b", Status: "verified"},
			{VariantID: "gemma3-4b", Status: "pending"},
		},
	}
	plan := queue.BuildBatchPlan(CampaignQueueBatchPlanRequest{
		SkipVariantIDs: []string{"granite3-3-2b"},
		SkipVerified:   true,
	})
	assert.Len(t, plan, 1)
	assert.Equal(t, "gemma3-4b", plan[0].VariantID)
}

func TestCampaignWitnessStatusFromOperators(t *testing.T) {
	status := CampaignWitnessStatusFromOperators([]models.OperatorDocumentGo{
		{
			Status: constants.OperatorStatusActive,
			RuntimeConfig: &models.RuntimeConfig{
				ProviderBoundaryObserverEnabled: true,
			},
		},
		{
			Status: constants.OperatorStatusActive,
			RuntimeConfig: &models.RuntimeConfig{
				ProvenanceOperatorEnabled: true,
			},
		},
	})
	assert.True(t, status.Ready)
	assert.Equal(t, 1, status.ActiveObserverCount)
	assert.Equal(t, 1, status.ActiveProvenanceCount)
}

func TestTierAVerifyNotes(t *testing.T) {
	assert.Equal(t, "75/75 Tier-A verify PASS (--require-provider-observation --require-model-provenance); run run-abc", TierAVerifyNotes("run-abc"))
}
