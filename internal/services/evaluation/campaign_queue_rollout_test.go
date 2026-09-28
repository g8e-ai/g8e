// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func rolloutVariant(id, tag string, params uint64) *evalv1.ModelVariant {
	return &evalv1.ModelVariant{VariantId: id, ServedModelTag: tag, ModelDigest: "digest-" + id, ProviderClass: "ollama", ParameterCount: params}
}

func rolloutQueueEntryIDs(queue *CampaignQueue) []string {
	ids := make([]string, 0, len(queue.Models))
	for _, entry := range queue.Models {
		ids = append(ids, entry.VariantID)
	}
	return ids
}

func TestLoadRolloutQueue_MissingQueueIsEmpty(t *testing.T) {
	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	queue, err := LoadRolloutQueue(context.Background(), fileSvc)
	require.NoError(t, err)
	assert.Empty(t, queue.Models)
}

func TestSaveRolloutQueue_EmptyQueueRemovesFile(t *testing.T) {
	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), nil)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, fileSvc.CreateRuntimeTree(ctx))

	queue := BuildInitCampaignQueue("", nil)
	_, err = queue.AddVariants([]*evalv1.ModelVariant{rolloutVariant("qwen3-4b", "qwen3:4b", 4_000_000_000)}, nil)
	require.NoError(t, err)
	require.NoError(t, SaveRolloutQueue(ctx, fileSvc, queue))
	exists, err := fileSvc.FileExists(ctx, DefaultInitCampaignQueueRelPath)
	require.NoError(t, err)
	require.True(t, exists)

	queue.RemoveEntries([]string{"qwen3-4b"})
	require.NoError(t, SaveRolloutQueue(ctx, fileSvc, queue))
	exists, err = fileSvc.FileExists(ctx, DefaultInitCampaignQueueRelPath)
	require.NoError(t, err)
	assert.False(t, exists)

	// Removing an already-absent queue is not an error.
	require.NoError(t, SaveRolloutQueue(ctx, fileSvc, queue))
}

func TestCampaignQueueAddVariants_OrdersByPriorityThenSize(t *testing.T) {
	queue := BuildInitCampaignQueue("", nil)
	added, err := queue.AddVariants([]*evalv1.ModelVariant{
		rolloutVariant("big", "m:big", 12_000_000_000),
		rolloutVariant("tiny", "m:tiny", 1_000_000_000),
		rolloutVariant("mid", "m:mid", 4_000_000_000),
		rolloutVariant("pinned", "m:pinned", 30_000_000_000),
	}, []string{"pinned"})
	require.NoError(t, err)
	assert.Len(t, added, 4)
	assert.Equal(t, []string{"pinned", "tiny", "mid", "big"}, rolloutQueueEntryIDs(queue))
	for _, entry := range queue.Models {
		assert.Equal(t, QueueStatusPending, entry.Status)
		assert.Equal(t, "eval-init-"+entry.VariantID, entry.CampaignID)
		assert.NotEmpty(t, entry.ModelRegistryDigest)
	}
}

func TestCampaignQueueAddVariants_KeepsQueuedEntriesAndStatus(t *testing.T) {
	queue := BuildInitCampaignQueue("", nil)
	_, err := queue.AddVariants([]*evalv1.ModelVariant{rolloutVariant("mid", "m:mid", 4_000_000_000)}, nil)
	require.NoError(t, err)
	queue.SetEntryStatus([]string{"mid"}, QueueStatusVerified, "done")
	queue.Models[0].VerifiedRunID = "run-1"

	added, err := queue.AddVariants([]*evalv1.ModelVariant{
		rolloutVariant("mid", "m:mid", 4_000_000_000),
		rolloutVariant("tiny", "m:tiny", 1_000_000_000),
		nil,
	}, nil)
	require.NoError(t, err)
	require.Len(t, added, 1)
	assert.Equal(t, "tiny", added[0].VariantID)
	assert.Equal(t, []string{"mid", "tiny"}, rolloutQueueEntryIDs(queue))
	assert.Equal(t, QueueStatusVerified, queue.Models[0].Status)
	assert.Equal(t, "run-1", queue.Models[0].VerifiedRunID)
}

func TestCampaignQueueSelectEntries(t *testing.T) {
	queue := BuildInitCampaignQueue("", nil)
	_, err := queue.AddVariants([]*evalv1.ModelVariant{
		rolloutVariant("tiny", "m:tiny", 1_000_000_000),
		rolloutVariant("mid", "m:mid", 4_000_000_000),
	}, nil)
	require.NoError(t, err)

	selected, err := queue.SelectEntries([]string{"m:mid", "tiny", "mid"})
	require.NoError(t, err)
	require.Len(t, selected, 2)
	assert.Equal(t, "mid", selected[0].VariantID)
	assert.Equal(t, "tiny", selected[1].VariantID)

	_, err = queue.SelectEntries([]string{"m:absent"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
}

func TestCampaignQueueEntriesForVariants_UsesQueueOrder(t *testing.T) {
	queue := BuildInitCampaignQueue("", nil)
	variants := []*evalv1.ModelVariant{
		rolloutVariant("tiny", "m:tiny", 1_000_000_000),
		rolloutVariant("mid", "m:mid", 4_000_000_000),
		rolloutVariant("big", "m:big", 12_000_000_000),
	}
	_, err := queue.AddVariants(variants[:2], nil)
	require.NoError(t, err)

	entries := queue.EntriesForVariants([]*evalv1.ModelVariant{variants[1], variants[2], variants[0], nil})
	require.Len(t, entries, 2)
	assert.Equal(t, "tiny", entries[0].VariantID)
	assert.Equal(t, "mid", entries[1].VariantID)
}

func TestCampaignQueueSetEntryStatus(t *testing.T) {
	queue := BuildInitCampaignQueue("", nil)
	_, err := queue.AddVariants([]*evalv1.ModelVariant{
		rolloutVariant("tiny", "m:tiny", 1_000_000_000),
		rolloutVariant("mid", "m:mid", 4_000_000_000),
	}, nil)
	require.NoError(t, err)
	queue.SetEntryStatus([]string{"tiny"}, QueueStatusVerified, "ok")
	queue.Models[0].VerifiedRunID = "run-1"

	queue.SetEntryStatus([]string{"tiny"}, QueueStatusPending, "retry")
	assert.Equal(t, QueueStatusPending, queue.Models[0].Status)
	assert.Equal(t, "retry", queue.Models[0].Notes)
	assert.Empty(t, queue.Models[0].VerifiedRunID, "a status other than verified clears the verified run")
	assert.Equal(t, QueueStatusPending, queue.Models[1].Status, "unselected entries are untouched")
}

func TestLoadRolloutIntakePriorityIDs_ReadsProjectRoot(t *testing.T) {
	root := testutil.TempDir(t)
	assert.Nil(t, LoadRolloutIntakePriorityIDs(root))

	path := filepath.Join(root, DefaultRolloutIntakePriorityRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`["b","a"]`), 0o644))
	assert.Equal(t, []string{"b", "a"}, LoadRolloutIntakePriorityIDs(root))

	require.NoError(t, os.WriteFile(path, []byte(`not json`), 0o644))
	assert.Nil(t, LoadRolloutIntakePriorityIDs(root))
}
