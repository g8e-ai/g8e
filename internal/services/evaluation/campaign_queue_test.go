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
	"fmt"
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
		{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ParameterCount: 4_000_000_000},
		{VariantId: "qwen3-5-9b", ServedModelTag: "qwen3.5:9b", ParameterCount: 9_000_000_000},
		{VariantId: "granite4-2-8b", ServedModelTag: "granite4.2:8b", ParameterCount: 8_000_000_000},
		{VariantId: "qwen3-5-0-8b", ServedModelTag: "qwen3.5:0.8b", ParameterCount: 800_000_000},
		{VariantId: "granite4-2-3b", ServedModelTag: "granite4.2:3b", ParameterCount: 3_000_000_000},
		{VariantId: "gemma3-4b", ServedModelTag: "gemma3:4b", ParameterCount: 4_000_000_000},
	}

	SortModelVariantsForRollout(variants)

	orderedTags := make([]string, 0, len(variants))
	for _, variant := range variants {
		orderedTags = append(orderedTags, variant.GetServedModelTag())
	}
	assert.Equal(t, []string{
		"qwen3.5:0.8b",
		"granite4.2:3b",
		"gemma3:4b",
		"qwen3:4b",
		"granite4.2:8b",
		"qwen3.5:9b",
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
				InventoryFile:        "eval/inventories/eval-init-gemma3-4b.json",
				ModelRegistryDigest:  "digest",
				HomogeneousCellCount: 75,
				Status:               "pending",
			},
		},
	}
	fileSvc, err := fs.NewRuntimeFileService(root, nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	require.NoError(t, SaveInitCampaignQueueToRuntime(context.Background(), fileSvc, DefaultInitCampaignQueueRelPath, &queue))

	now := time.Unix(1789669555, 0).UTC()
	plan, err := ResolveCampaignStartPlan(CampaignStartPlanRequest{
		Context:     context.Background(),
		FileService: fileSvc,
		QueueRef:    "next",
		Now:         now,
	})
	require.NoError(t, err)
	assert.Equal(t, "eval-init-gemma3-4b", plan.CampaignID)
	assert.Equal(t, "eval-init-gemma3-4b-1789669555", plan.RunID)
	assert.Equal(t, []string{"gemma3:4b"}, plan.ModelTags)
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

	fileSvc, err := fs.NewRuntimeFileService(root, nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	result, err := InitCampaignQueue(InitCampaignQueueRequest{
		Context:             context.Background(),
		FileService:         fileSvc,
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
	errRead := fmt.Errorf("read failure")
	errWrite := fmt.Errorf("write failure")
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
			relPath := campaignInventoryPath(DefaultCampaignInventoryRelDirname, freeze.CampaignID)
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
		ModelTag:    variant.GetServedModelTag(),
		CampaignID:  "eval-init-qwen3-4b",
		Now:         time.Unix(1789657337, 0).UTC(),
	}
	first, err := ResolveCampaignStartPlan(request)
	require.NoError(t, err)
	relPath, err := fileSvc.Rel(first.InventoryPath)
	require.NoError(t, err)
	info, err := fileSvc.Stat(context.Background(), relPath)
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

func TestQueueLogDirReturnsCanonicalRuntimeRelativePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "rollout name", input: "queue-run-20260923", want: "eval/logs/queue-run-20260923"},
		{name: "blank name", input: "", want: ""},
		{name: "nested name rejected", input: "batch/one", want: ""},
		{name: "parent name rejected", input: "..", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, QueueLogDir(test.input))
		})
	}
}

func TestActiveCampaignRunRoundTrip(t *testing.T) {
	fileSvc, err := fs.NewRuntimeFileService(t.TempDir(), nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	run := ActiveCampaignRun{
		RunID:         "eval-init-gemma3-4b-1789669555",
		CampaignID:    "eval-init-gemma3-4b",
		InventoryFile: "eval/inventories/eval-init-gemma3-4b.json",
		ModelTags:     []string{"gemma3:4b"},
		StartedAt:     time.Unix(1789669555, 0).UTC(),
	}
	require.NoError(t, SaveActiveCampaignRunToRuntime(context.Background(), fileSvc, run))
	exists, err := fileSvc.FileExists(context.Background(), constants.EvaluationActiveRunPath)
	require.NoError(t, err)
	assert.True(t, exists)
	loaded, err := LoadActiveCampaignRunFromRuntime(context.Background(), fileSvc)
	require.NoError(t, err)
	assert.Equal(t, run.RunID, loaded.RunID)
	assert.Equal(t, run.CampaignID, loaded.CampaignID)
}
