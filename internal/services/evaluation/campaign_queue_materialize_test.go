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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestMarkCampaignQueueEntry(t *testing.T) {
	queue := CampaignQueue{
		Models: []CampaignQueueModel{
			{VariantID: "gemma3-4b", ServedModelTag: "gemma3:4b", Status: "pending"},
		},
	}
	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	require.NoError(t, SaveInitCampaignQueueToRuntime(context.Background(), fileSvc, DefaultInitCampaignQueueRelPath, &queue))

	entry, err := MarkCampaignQueueEntry(MarkCampaignQueueEntryRequest{
		Context:        context.Background(),
		FileService:    fileSvc,
		QueuePath:      DefaultInitCampaignQueueRelPath,
		ServedModelTag: "gemma3:4b",
		Status:         "verified",
		VerifiedRunID:  "eval-init-gemma3-4b-123",
		Notes:          "witness PASS",
	})
	require.NoError(t, err)
	assert.Equal(t, "verified", entry.Status)
	assert.Equal(t, "eval-init-gemma3-4b-123", entry.VerifiedRunID)

	loaded, err := LoadInitCampaignQueueFromRuntime(context.Background(), fileSvc, DefaultInitCampaignQueueRelPath)
	require.NoError(t, err)
	assert.Equal(t, "verified", loaded.Models[0].Status)
}
