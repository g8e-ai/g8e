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
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
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

func TestSortModelVariantsForRollout_PrioritizesCurrentSmallModels(t *testing.T) {
	variants := []*evalv1.ModelVariant{
		{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b"},
		{VariantId: "qwen3-5-9b", ServedModelTag: "qwen3.5:9b"},
		{VariantId: "granite4-2-8b", ServedModelTag: "granite4.2:8b"},
		{VariantId: "qwen3-5-0-8b", ServedModelTag: "qwen3.5:0.8b"},
		{VariantId: "granite4-2-3b", ServedModelTag: "granite4.2:3b"},
		{VariantId: "gemma3-4b", ServedModelTag: "gemma3:4b"},
	}

	SortModelVariantsForRollout(variants)

	orderedTags := make([]string, 0, len(variants))
	for _, variant := range variants {
		orderedTags = append(orderedTags, variant.GetServedModelTag())
	}
	assert.Equal(t, []string{
		"granite4.2:3b",
		"granite4.2:8b",
		"qwen3.5:0.8b",
		"qwen3.5:9b",
		"gemma3:4b",
		"qwen3:4b",
	}, orderedTags)
}

func TestResolveCampaignStartPlanFromQueue(t *testing.T) {
	root := t.TempDir()
	queue := CampaignQueue{
		Models: []CampaignQueueModel{
			{
				VariantID:            "gemma3-4b",
				ServedModelTag:       "gemma3:4b",
				CampaignID:           "eval-init-gemma3-4b",
				InventoryFile:        ".g8e/eval/inventories/eval-init-gemma3-4b.json",
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

func TestResolveModelInventoryPathDefaultsToBaseInventory(t *testing.T) {
	root := t.TempDir()
	basePath := ResolveBaseModelInventoryPath(root)
	require.NoError(t, os.MkdirAll(filepath.Dir(basePath), 0o755))
	require.NoError(t, os.WriteFile(basePath, []byte(`{"variants":[]}`), 0o600))

	assert.Equal(t, basePath, ResolveModelInventoryPath(root, ""))
	assert.Equal(t, filepath.Join(root, "custom.json"), ResolveModelInventoryPath(root, "custom.json"))
}

func TestInitCampaignQueueFiltersRuntimeFreezeToBaseTags(t *testing.T) {
	root := t.TempDir()
	writeFrozenVariantsForQueueTest(t, root, DefaultBaseModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "d1", ProviderClass: "ollama"},
	)
	writeFrozenVariantsForQueueTest(t, root, DefaultModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "d1", ProviderClass: "ollama"},
		&evalv1.ModelVariant{VariantId: "extra-model", ServedModelTag: "extra:model", ModelDigest: "d2", ProviderClass: "ollama"},
	)

	result, err := InitCampaignQueue(InitCampaignQueueRequest{
		ProjectRoot:         root,
		SourceInventoryPath: DefaultModelInventoryRelPath,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.ModelCount)
	assert.Equal(t, "qwen3-4b", result.Queue.Models[0].VariantID)
}

func writeFrozenVariantsForQueueTest(t *testing.T, root, relPath string, variants ...*evalv1.ModelVariant) {
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

	fileSvc, err := fs.NewRuntimeFileService(root, nil)
	require.NoError(t, err)
	now := time.Unix(1789657337, 0).UTC()
	plan, err := ResolveCampaignStartPlan(CampaignStartPlanRequest{
		Context:     context.Background(),
		FileService: fileSvc,
		ProjectRoot: root,
		ModelTag:    "qwen3:4b",
		Now:         now,
	})
	require.NoError(t, err)
	assert.Equal(t, "eval-init-qwen3-4b", plan.CampaignID)
	assert.Equal(t, "eval-init-qwen3-4b-1789657337", plan.RunID)
	assert.FileExists(t, plan.InventoryPath)
}

type immutableInventoryFileService struct {
	*campaignMemoryFileService
	readErr  error
	writeErr error
}

func (s *immutableInventoryFileService) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	return s.campaignMemoryFileService.ReadFile(ctx, relPath)
}

func (s *immutableInventoryFileService) WriteFile(ctx context.Context, relPath string, data []byte, mode os.FileMode) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	return s.campaignMemoryFileService.WriteFile(ctx, relPath, data, mode)
}

func TestMaterializeImmutableModelInventory_ReturnsReadWriteAndMarshalErrors(t *testing.T) {
	freeze, err := MaterializeModelRegistry("eval-init-qwen3-4b", []*evalv1.ModelVariant{{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "digest",
		ProviderClass:  "ollama",
	}})
	require.NoError(t, err)
	errRead := errors.New("read failure")
	errWrite := errors.New("write failure")
	tests := []struct {
		name    string
		service *immutableInventoryFileService
		freeze  *ModelInventoryFreeze
		want    error
	}{
		{name: "read failure", service: &immutableInventoryFileService{campaignMemoryFileService: newCampaignMemoryFileService(), readErr: errRead}, freeze: freeze, want: errRead},
		{name: "write failure", service: &immutableInventoryFileService{campaignMemoryFileService: newCampaignMemoryFileService(), readErr: constants.ErrNotFound, writeErr: errWrite}, freeze: freeze, want: errWrite},
		{name: "missing freeze", service: &immutableInventoryFileService{campaignMemoryFileService: newCampaignMemoryFileService()}, want: constants.ErrMissingRequiredField},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			relPath := filepath.Join(constants.EvaluationDirname, constants.EvaluationInventoriesDirname, freeze.CampaignID+".json")
			err := materializeImmutableModelInventory(context.Background(), test.service, relPath, test.freeze)
			require.Error(t, err)
			assert.ErrorIs(t, err, test.want)
		})
	}
}

func TestResolveCampaignStartPlan_ReusesIdenticalReadOnlyInventoryAndRejectsDifferentContent(t *testing.T) {
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
	writeFrozenVariantsForQueueTest(t, root, DefaultBaseModelInventoryRelPath, variant)

	request := CampaignStartPlanRequest{
		Context:     context.Background(),
		FileService: fileSvc,
		ProjectRoot: root,
		ModelTag:    variant.GetServedModelTag(),
		CampaignID:  "eval-init-qwen3-4b",
		Now:         time.Unix(1789657337, 0).UTC(),
	}
	first, err := ResolveCampaignStartPlan(request)
	require.NoError(t, err)
	info, err := os.Stat(first.InventoryPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermFileReadOnly), info.Mode().Perm())

	second, err := ResolveCampaignStartPlan(request)
	require.NoError(t, err)
	assert.Equal(t, first.InventoryPath, second.InventoryPath)

	variant.ModelDigest = "different-digest"
	writeFrozenVariantsForQueueTest(t, root, DefaultBaseModelInventoryRelPath, variant)
	_, err = ResolveCampaignStartPlan(request)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrImmutableInventoryConflict))
}

func TestActiveCampaignRunRoundTrip(t *testing.T) {
	root := t.TempDir()
	run := ActiveCampaignRun{
		RunID:         "eval-init-gemma3-4b-1789669555",
		CampaignID:    "eval-init-gemma3-4b",
		InventoryFile: ".g8e/eval/inventories/eval-init-gemma3-4b.json",
		ModelTags:     []string{"gemma3:4b"},
		StartedAt:     time.Unix(1789669555, 0).UTC(),
	}
	require.NoError(t, SaveActiveCampaignRun(root, run))
	loaded, err := LoadActiveCampaignRun(root)
	require.NoError(t, err)
	assert.Equal(t, run.RunID, loaded.RunID)
	assert.Equal(t, run.CampaignID, loaded.CampaignID)
}
