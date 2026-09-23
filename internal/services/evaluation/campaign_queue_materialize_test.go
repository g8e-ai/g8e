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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func writeFrozenVariants(t *testing.T, root, relPath string, variants ...*evalv1.ModelVariant) {
	t.Helper()
	bodies := make([]json.RawMessage, 0, len(variants))
	for _, variant := range variants {
		raw, err := protojson.Marshal(variant)
		require.NoError(t, err)
		bodies = append(bodies, raw)
	}
	path := filepath.Join(root, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	payload, err := json.Marshal(map[string]any{"variants": bodies})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, 0o600))
}

func TestMaterializeInitCampaignInventory(t *testing.T) {
	root := t.TempDir()
	variant := &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "digest",
		ProviderClass:  "ollama",
	}
	entry, err := MaterializeInitCampaignInventory(MaterializeInitCampaignInventoryRequest{
		ProjectRoot: root,
		Variant:     variant,
	})
	require.NoError(t, err)
	assert.Equal(t, "eval-init-qwen3-4b", entry.CampaignID)
	assert.Equal(t, "eval/inventories/eval-init-qwen3-4b.json", entry.InventoryFile)
	assert.FileExists(t, filepath.Join(root, entry.InventoryFile))
}

func TestMaterializeInitCampaignInventory_AbsoluteDirectoryWritesOutsideProjectRoot(t *testing.T) {
	root := t.TempDir()
	outputDir := t.TempDir()
	variant := &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "digest",
		ProviderClass:  "ollama",
	}
	entry, err := MaterializeInitCampaignInventory(MaterializeInitCampaignInventoryRequest{
		ProjectRoot:     root,
		InventoryRelDir: outputDir,
		Variant:         variant,
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(outputDir, entry.CampaignID+".json"), entry.InventoryFile)
	assert.FileExists(t, entry.InventoryFile)
}

func TestInitCampaignQueueMaterializeAndMerge(t *testing.T) {
	root := t.TempDir()
	writeFrozenVariants(t, root, DefaultModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "gemma3-4b", ServedModelTag: "gemma3:4b", ModelDigest: "d1", ProviderClass: "ollama"},
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "d2", ProviderClass: "ollama"},
	)

	existing := &CampaignQueue{
		Models: []CampaignQueueModel{
			{VariantID: "gemma3-4b", ServedModelTag: "gemma3:4b", Status: "verified", VerifiedRunID: "run-1", Notes: "keep"},
		},
	}
	queuePath := filepath.Join(root, DefaultInitCampaignQueueRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(queuePath), 0o755))
	body, err := json.Marshal(existing)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(queuePath, body, 0o600))

	writeFrozenVariants(t, root, DefaultBaseModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "gemma3-4b", ServedModelTag: "gemma3:4b", ModelDigest: "d1", ProviderClass: "ollama"},
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "d2", ProviderClass: "ollama"},
	)
	result, err := InitCampaignQueue(InitCampaignQueueRequest{
		ProjectRoot:         root,
		SourceInventoryPath: DefaultModelInventoryRelPath,
		Materialize:         true,
		MergeExisting:       true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.ModelCount)
	assert.Equal(t, 2, result.Materialized)
	assert.Equal(t, 1, result.Preserved)

	loaded, err := LoadInitCampaignQueue(queuePath)
	require.NoError(t, err)
	gemma, err := loaded.FindByTagOrVariantID("gemma3:4b")
	require.NoError(t, err)
	assert.Equal(t, "verified", gemma.Status)
	assert.Equal(t, "run-1", gemma.VerifiedRunID)

	qwen, err := loaded.FindByTagOrVariantID("qwen3:4b")
	require.NoError(t, err)
	assert.Equal(t, "pending", qwen.Status)
	assert.FileExists(t, filepath.Join(root, qwen.InventoryFile))
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
