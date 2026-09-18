// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCampaignQueueNextPendingAndLookup(t *testing.T) {
	queue := &CampaignQueue{
		Models: []CampaignQueueModel{
			{VariantID: "a", ServedModelTag: "a:1", Status: "verified"},
			{VariantID: "b", ServedModelTag: "b:2", Status: "pending", CampaignID: "eval-init-b"},
		},
	}
	entry, err := queue.NextPending()
	require.NoError(t, err)
	assert.Equal(t, "b:2", entry.ServedModelTag)

	found, err := queue.FindByTagOrVariantID("b")
	require.NoError(t, err)
	assert.Equal(t, "eval-init-b", found.CampaignID)
}

func TestCampaignIDForVariant(t *testing.T) {
	assert.Equal(t, "init-campaign", CampaignIDForVariant(&evalv1.ModelVariant{ServedModelTag: "gemma4:e4b", VariantId: "gemma4-e4b"}))
	assert.Equal(t, "eval-init-gemma3-4b", CampaignIDForVariant(&evalv1.ModelVariant{ServedModelTag: "gemma3:4b", VariantId: "gemma3-4b"}))
}

func TestResolveCampaignStartPlanFromQueue(t *testing.T) {
	root := t.TempDir()
	queue := CampaignQueue{
		Models: []CampaignQueueModel{
			{
				VariantID:            "gemma3-4b",
				ServedModelTag:       "gemma3:4b",
				CampaignID:           "eval-init-gemma3-4b",
				InventoryFile:        ".local.dev/inventories/eval-init-gemma3-4b.json",
				ModelRegistryDigest:  "digest",
				HomogeneousCellCount: 75,
				Status:               "pending",
			},
		},
	}
	queuePath := filepath.Join(root, DefaultInitCampaignQueueRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(queuePath), 0o755))
	body, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(queuePath, body, 0o600))

	now := time.Unix(1789669555, 0).UTC()
	plan, err := ResolveCampaignStartPlan(CampaignStartPlanRequest{
		ProjectRoot: root,
		QueueRef:    "next",
		Now:         now,
	})
	require.NoError(t, err)
	assert.Equal(t, "eval-init-gemma3-4b", plan.CampaignID)
	assert.Equal(t, "eval-init-gemma3-4b-1789669555", plan.RunID)
	assert.Equal(t, []string{"gemma3:4b"}, plan.ModelTags)
}

func TestResolveCampaignStartPlanForModelTags(t *testing.T) {
	root := t.TempDir()
	variant := &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "digest",
		ProviderClass:  "ollama",
	}
	raw, err := protojson.Marshal(variant)
	require.NoError(t, err)
	inventoryPath := filepath.Join(root, DefaultModelInventoryRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(inventoryPath), 0o755))
	require.NoError(t, os.WriteFile(inventoryPath, []byte(`{"variants":[`+string(raw)+`]}`), 0o600))

	now := time.Unix(1789657337, 0).UTC()
	plan, err := ResolveCampaignStartPlan(CampaignStartPlanRequest{
		ProjectRoot: root,
		ModelTag:    "qwen3:4b",
		Now:         now,
	})
	require.NoError(t, err)
	assert.Equal(t, "eval-init-qwen3-4b", plan.CampaignID)
	assert.Equal(t, "eval-init-qwen3-4b-1789657337", plan.RunID)
	assert.FileExists(t, plan.InventoryPath)
}

func TestActiveCampaignRunRoundTrip(t *testing.T) {
	root := t.TempDir()
	run := ActiveCampaignRun{
		RunID:         "eval-init-gemma3-4b-1789669555",
		CampaignID:    "eval-init-gemma3-4b",
		InventoryFile: ".local.dev/inventories/eval-init-gemma3-4b.json",
		ModelTags:     []string{"gemma3:4b"},
		StartedAt:     time.Unix(1789669555, 0).UTC(),
	}
	require.NoError(t, SaveActiveCampaignRun(root, run))
	loaded, err := LoadActiveCampaignRun(root)
	require.NoError(t, err)
	assert.Equal(t, run.RunID, loaded.RunID)
	assert.Equal(t, run.CampaignID, loaded.CampaignID)
}
