// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type frozenVariantsPayload struct {
	Variants []json.RawMessage `json:"variants"`
}

func writeFrozenVariants(t *testing.T, fileSvc fs.RuntimeFileService, relPath string, variants ...*evalv1.ModelVariant) {
	t.Helper()
	bodies := make([]json.RawMessage, 0, len(variants))
	for _, variant := range variants {
		raw, err := protojson.Marshal(variant)
		require.NoError(t, err)
		bodies = append(bodies, raw)
	}
	payload, err := json.Marshal(frozenVariantsPayload{Variants: bodies})
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, payload, constants.PermFilePrivate))
}

func TestMaterializeInitCampaignInventory(t *testing.T) {
	root := t.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(root, nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	variant := &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "digest",
		ProviderClass:  "ollama",
	}
	entry, err := MaterializeInitCampaignInventory(MaterializeInitCampaignInventoryRequest{
		Context:     context.Background(),
		FileService: fileSvc,
		Variant:     variant,
	})
	require.NoError(t, err)
	assert.Equal(t, "eval-init-qwen3-4b", entry.CampaignID)
	assert.Equal(t, "eval/inventories/eval-init-qwen3-4b.json", entry.InventoryFile)
	exists, err := fileSvc.FileExists(context.Background(), entry.InventoryFile)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestMaterializeInitCampaignInventory_RejectsAbsoluteDirectory(t *testing.T) {
	root := t.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(root, nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	variant := &evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "digest", ProviderClass: "ollama"}
	_, err = MaterializeInitCampaignInventory(MaterializeInitCampaignInventoryRequest{
		Context:         context.Background(),
		FileService:     fileSvc,
		InventoryRelDir: t.TempDir(),
		Variant:         variant,
	})
	require.Error(t, err)
}

func TestInitCampaignQueueMaterializeAndMerge(t *testing.T) {
	root := t.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(root, nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	writeFrozenVariants(t, fileSvc, DefaultModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "gemma3-4b", ServedModelTag: "gemma3:4b", ModelDigest: "d1", ProviderClass: "ollama"},
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "d2", ProviderClass: "ollama"},
	)

	existing := &CampaignQueue{
		Models: []CampaignQueueModel{
			{VariantID: "gemma3-4b", ServedModelTag: "gemma3:4b", Status: "verified", VerifiedRunID: "run-1", Notes: "keep"},
		},
	}
	writeFrozenVariants(t, fileSvc, DefaultBaseModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "gemma3-4b", ServedModelTag: "gemma3:4b", ModelDigest: "d1", ProviderClass: "ollama"},
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "d2", ProviderClass: "ollama"},
	)
	require.NoError(t, SaveInitCampaignQueueToRuntime(context.Background(), fileSvc, DefaultInitCampaignQueueRelPath, existing))
	result, err := InitCampaignQueue(InitCampaignQueueRequest{
		Context:              context.Background(),
		FileService:          fileSvc,
		RuntimeInventoryPath: DefaultModelInventoryRelPath,
		Materialize:          true,
		MergeExisting:        true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.ModelCount)
	assert.Equal(t, 2, result.Materialized)
	assert.Equal(t, 1, result.Preserved)

	loaded, err := LoadInitCampaignQueueFromRuntime(context.Background(), fileSvc, DefaultInitCampaignQueueRelPath)
	require.NoError(t, err)
	gemma, err := loaded.FindByTagOrVariantID("gemma3:4b")
	require.NoError(t, err)
	assert.Equal(t, "verified", gemma.Status)
	assert.Equal(t, "run-1", gemma.VerifiedRunID)

	qwen, err := loaded.FindByTagOrVariantID("qwen3:4b")
	require.NoError(t, err)
	assert.Equal(t, "pending", qwen.Status)
	exists, err := fileSvc.FileExists(context.Background(), qwen.InventoryFile)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestMarkCampaignQueueEntry(t *testing.T) {
	root := t.TempDir()
	queue := CampaignQueue{
		Models: []CampaignQueueModel{
			{VariantID: "gemma3-4b", ServedModelTag: "gemma3:4b", Status: "pending"},
		},
	}
	fileSvc, err := fs.NewRuntimeFileService(root, nil)
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
		Notes:          "Tier-A PASS",
	})
	require.NoError(t, err)
	assert.Equal(t, "verified", entry.Status)
	assert.Equal(t, "eval-init-gemma3-4b-123", entry.VerifiedRunID)

	loaded, err := LoadInitCampaignQueueFromRuntime(context.Background(), fileSvc, DefaultInitCampaignQueueRelPath)
	require.NoError(t, err)
	assert.Equal(t, "verified", loaded.Models[0].Status)
}
