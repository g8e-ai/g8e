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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestIsAbandonedCampaignSummary(t *testing.T) {
	assert.True(t, IsAbandonedCampaignSummary(&CampaignRunSummary{
		Run:         &evalv1.EvaluationRun{RunId: "run-1"},
		QueuedCount: 75,
	}))
	assert.False(t, IsAbandonedCampaignSummary(&CampaignRunSummary{
		Run:           &evalv1.EvaluationRun{RunId: "run-1"},
		QueuedCount:   10,
		TerminalCount: 1,
	}))
	assert.False(t, IsAbandonedCampaignSummary(&CampaignRunSummary{
		Run:          &evalv1.EvaluationRun{RunId: "run-1"},
		QueuedCount:  10,
		RunningCount: 1,
	}))
}

func TestListAndDiscardAbandonedCampaignRuns(t *testing.T) {
	ctx := context.Background()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	controller := NewCampaignController(store, nil, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" })

	runID := "eval-init-qwen3-4b-1789000001"
	req := testCampaignInitRequest(t)
	req.RunID = runID
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:1],
	}
	truncatedDigest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = truncatedDigest
	req.Catalog = truncated
	_, err = controller.InitializeCampaign(ctx, req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(ctx, runID)
	require.NoError(t, err)

	queue := &CampaignQueue{
		Models: []CampaignQueueModel{
			{VariantID: "qwen3-4b", Status: "failed", VerifiedRunID: runID},
			{VariantID: "gemma3-4b", Status: "verified", VerifiedRunID: "eval-init-gemma3-4b-verified"},
		},
	}

	abandoned, err := ListAbandonedCampaignRuns(ListAbandonedCampaignRunsRequest{
		Context:    ctx,
		Store:      store,
		Controller: controller,
		Queue:      queue,
	})
	require.NoError(t, err)
	require.Len(t, abandoned, 1)
	assert.Equal(t, runID, abandoned[0].RunID)

	result, err := DiscardAbandonedCampaignRuns(DiscardAbandonedCampaignRunsRequest{
		Context:     ctx,
		FileService: files,
		Store:       store,
		Queue:       queue,
		QueuePath:   DefaultInitCampaignQueueRelPath,
		Runs:        abandoned,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{runID}, result.DiscardedRunIDs)
	assert.Equal(t, 1, result.QueueUpdates)
	assert.Equal(t, "pending", queue.Models[0].Status)
	assert.Empty(t, queue.Models[0].VerifiedRunID)

	exists, err := store.RunExists(ctx, runID)
	require.NoError(t, err)
	assert.False(t, exists)
}
