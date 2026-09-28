// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCampaignQueueBuildBatchPlan(t *testing.T) {
	queue := &CampaignQueue{
		Models: []CampaignQueueModel{
			{VariantID: "granite3-3-2b", Status: QueueStatusVerified},
			{VariantID: "skipme", Status: QueueStatusSkipped},
			{VariantID: "qwen3-4b", Status: QueueStatusFailed},
			{VariantID: "gemma3-4b", Status: QueueStatusPending},
			{VariantID: "llama3-8b", Status: QueueStatusPending},
		},
	}

	tests := []struct {
		name string
		req  CampaignQueueBatchPlanRequest
		want []string
	}{
		{name: "skip verified drops verified and skipped", req: CampaignQueueBatchPlanRequest{SkipVerified: true}, want: []string{"qwen3-4b", "gemma3-4b", "llama3-8b"}},
		{name: "keep verified still drops skipped", req: CampaignQueueBatchPlanRequest{}, want: []string{"granite3-3-2b", "qwen3-4b", "gemma3-4b", "llama3-8b"}},
		{name: "until bounds the batch", req: CampaignQueueBatchPlanRequest{SkipVerified: true, Until: 2}, want: []string{"qwen3-4b", "gemma3-4b"}},
		{name: "until larger than the plan", req: CampaignQueueBatchPlanRequest{SkipVerified: true, Until: 10}, want: []string{"qwen3-4b", "gemma3-4b", "llama3-8b"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got []string
			for _, entry := range queue.BuildBatchPlan(test.req) {
				got = append(got, entry.VariantID)
			}
			assert.Equal(t, test.want, got)
		})
	}
}

func TestStrictWitnessVerifyNotes(t *testing.T) {
	assert.Equal(t, "75/75 witness verify PASS (--require-provider-observation --require-model-provenance); run run-abc", StrictWitnessVerifyNotes("run-abc"))
}
