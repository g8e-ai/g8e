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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type modelInventoryFreezePayload struct {
	CampaignID          string            `json:"campaign_id"`
	ModelRegistryDigest string            `json:"model_registry_digest"`
	Variants            []json.RawMessage `json:"variants"`
}

func TestComputeHomogeneousMatrixSize(t *testing.T) {
	t.Parallel()
	assert.Equal(t, uint64(37), ComputeHomogeneousMatrixSize(1))
	assert.Equal(t, uint64(148), ComputeHomogeneousMatrixSize(4))
}

func TestBuildModelVariantsFromProviderInventory_SortsAndDedupes(t *testing.T) {
	t.Parallel()
	entries := []inference.ProviderModelInventoryEntry{
		{ServedModelTag: "z-model:7b", ModelDigest: repeatHex('a', 64), ModelFamily: "z"},
		{ServedModelTag: "a-model:7b", ModelDigest: repeatHex('b', 64), ModelFamily: "a"},
	}
	variants, err := BuildModelVariantsFromProviderInventory(context.Background(), entries, ModelInventoryOptions{})
	require.NoError(t, err)
	require.Len(t, variants, 2)
	assert.Equal(t, "a-model:7b", variants[0].GetServedModelTag())
	assert.Equal(t, "z-model:7b", variants[1].GetServedModelTag())
}

func TestBuildModelVariantsFromProviderInventory_RejectsDuplicateTags(t *testing.T) {
	t.Parallel()
	entries := []inference.ProviderModelInventoryEntry{
		{ServedModelTag: "dup:7b", ModelDigest: repeatHex('a', 64)},
		{ServedModelTag: "dup:7b", ModelDigest: repeatHex('b', 64)},
	}
	_, err := BuildModelVariantsFromProviderInventory(context.Background(), entries, ModelInventoryOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate served tag")
}

func TestLookupModelVariant_ReturnsFrozenVariant(t *testing.T) {
	t.Parallel()
	freeze, err := MaterializeModelRegistry("campaign-1", []*evalv1.ModelVariant{testModelVariant()})
	require.NoError(t, err)
	variant, err := freeze.LookupModelVariant("qwen3:4b")
	require.NoError(t, err)
	assert.Equal(t, "qwen3-4b", variant.GetVariantId())
	_, err = freeze.LookupModelVariant("missing:tag")
	require.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
}

func TestLoadModelInventoryFreezeFile_RoundTrip(t *testing.T) {
	t.Parallel()
	freeze, err := MaterializeModelRegistry("campaign-1", []*evalv1.ModelVariant{testModelVariant()})
	require.NoError(t, err)
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")
	rawVariants := make([]json.RawMessage, 0, len(freeze.Variants))
	for _, variant := range freeze.Variants {
		body, err := protojson.Marshal(variant)
		require.NoError(t, err)
		rawVariants = append(rawVariants, body)
	}
	payload, err := json.Marshal(modelInventoryFreezePayload{
		CampaignID:          freeze.CampaignID,
		ModelRegistryDigest: freeze.RegistryDigest,
		Variants:            rawVariants,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, constants.PermFilePublic))

	loaded, err := LoadModelInventoryFreezeFile(path)
	require.NoError(t, err)
	assert.Equal(t, freeze.RegistryDigest, loaded.RegistryDigest)
	assert.Equal(t, freeze.HomogeneousCellCount, loaded.HomogeneousCellCount)
	require.NoError(t, ValidateModelRegistry(loaded))
}

func TestLoadModelInventoryFreezeFile_RejectsMissingDigest(t *testing.T) {
	t.Parallel()
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "inventory.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"campaign_id":"c1","variants":[]}`), constants.PermFilePublic))
	_, err := LoadModelInventoryFreezeFile(path)
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestToModelRegistryFreeze_ReturnsInferenceShape(t *testing.T) {
	t.Parallel()
	freeze, err := MaterializeModelRegistry("campaign-1", []*evalv1.ModelVariant{testModelVariant()})
	require.NoError(t, err)
	registry := freeze.ToModelRegistryFreeze()
	require.NotNil(t, registry)
	assert.Equal(t, freeze.RegistryDigest, registry.Digest)
	require.Len(t, registry.Variants, 1)
	assert.Equal(t, "qwen3:4b", registry.Variants[0].GetModel())
}

func TestBuildModelVariantsFromProviderInventory_RequiresGovernedProbeRunnerWhenRequested(t *testing.T) {
	t.Parallel()
	entries := []inference.ProviderModelInventoryEntry{
		{ServedModelTag: "probe:7b", ModelDigest: repeatHex('a', 64)},
	}
	_, err := BuildModelVariantsFromProviderInventory(context.Background(), entries, ModelInventoryOptions{RunCapabilityProbes: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "governed probe runner")
}

func TestBuildModelVariantsFromProviderInventory_RejectsEmptyInventory(t *testing.T) {
	t.Parallel()
	_, err := BuildModelVariantsFromProviderInventory(context.Background(), nil, ModelInventoryOptions{})
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestLookupModelVariant_RejectsNilFreeze(t *testing.T) {
	t.Parallel()
	var freeze *ModelInventoryFreeze
	_, err := freeze.LookupModelVariant("qwen3:4b")
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestLoadModelInventoryFreezeFile_WrapsReadErrors(t *testing.T) {
	t.Parallel()
	_, err := LoadModelInventoryFreezeFile(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	assert.False(t, errors.Is(err, constants.ErrMissingRequiredField))
}

func TestParseParameterCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		expected uint64
		wantErr  bool
	}{
		{"", 0, false},
		{"12b", 12_000_000_000, false},
		{"8B", 8_000_000_000, false},
		{"3.8b", 3_800_000_000, false},
		{"700m", 700_000_000, false},
		{"135M", 135_000_000, false},
		{"500k", 500_000, false},
		{"1000", 1000, false},
		{"invalid", 0, true},
		{"-5b", 0, true},
	}
	for _, tc := range cases {
		val, err := ParseParameterCount(tc.input)
		if tc.wantErr {
			assert.Error(t, err, "input: %s", tc.input)
		} else {
			require.NoError(t, err, "input: %s", tc.input)
			assert.Equal(t, tc.expected, val, "input: %s", tc.input)
		}
	}
}

func TestFormatParameterCount(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "-", FormatParameterCount(0))
	assert.Equal(t, "12B", FormatParameterCount(12_000_000_000))
	assert.Equal(t, "3.8B", FormatParameterCount(3_800_000_000))
	assert.Equal(t, "700M", FormatParameterCount(700_000_000))
	assert.Equal(t, "135M", FormatParameterCount(135_000_000))
	assert.Equal(t, "500K", FormatParameterCount(500_000))
	assert.Equal(t, "42", FormatParameterCount(42))
}

func TestFilterVariantsByMaxParameters(t *testing.T) {
	t.Parallel()
	variants := []*evalv1.ModelVariant{
		{VariantId: "m1", ParameterCount: 3_000_000_000},
		{VariantId: "m2", ParameterCount: 8_000_000_000},
		{VariantId: "m3", ParameterCount: 14_000_000_000},
		{VariantId: "m4", ParameterCount: 0},
	}
	filtered := FilterVariantsByMaxParameters(variants, 12_000_000_000)
	require.Len(t, filtered, 3)
	assert.Equal(t, "m1", filtered[0].VariantId)
	assert.Equal(t, "m2", filtered[1].VariantId)
	assert.Equal(t, "m4", filtered[2].VariantId)

	all := FilterVariantsByMaxParameters(variants, 0)
	assert.Equal(t, len(variants), len(all))
}

func TestFilterVariantsByFamily(t *testing.T) {
	t.Parallel()
	variants := []*evalv1.ModelVariant{
		{VariantId: "g1", ModelFamily: "gemma"},
		{VariantId: "g2", ModelFamily: "granite"},
		{VariantId: "q1", ModelFamily: "qwen"},
	}
	filtered := FilterVariantsByFamily(variants, "granite")
	require.Len(t, filtered, 1)
	assert.Equal(t, "g2", filtered[0].VariantId)

	filteredSub := FilterVariantsByFamily(variants, "g")
	assert.Len(t, filteredSub, 2)
}

func TestAddOrUpdateModelVariant_And_Merge(t *testing.T) {
	t.Parallel()
	freeze, err := MaterializeModelRegistry("campaign-1", []*evalv1.ModelVariant{
		{
			VariantId:      "model-a-1b",
			ProviderClass:  "ollama",
			ServedModelTag: "model-a:1b",
			ModelDigest:    repeatHex('a', 64),
			ModelFamily:    "test",
		},
	})
	require.NoError(t, err)

	updated, err := AddOrUpdateModelVariant(freeze, &evalv1.ModelVariant{
		ServedModelTag: "model-b:2b",
		ModelDigest:    repeatHex('b', 64),
		ModelFamily:    "test",
	})
	require.NoError(t, err)
	require.Len(t, updated.Variants, 2)
	assert.Equal(t, "model-a:1b", updated.Variants[0].ServedModelTag)
	assert.Equal(t, "model-b:2b", updated.Variants[1].ServedModelTag)

	// Replace existing
	updated2, err := AddOrUpdateModelVariant(updated, &evalv1.ModelVariant{
		ServedModelTag: "model-a:1b",
		ModelDigest:    repeatHex('c', 64),
		ModelFamily:    "test",
		ParameterCount: 1_000_000_000,
	})
	require.NoError(t, err)
	require.Len(t, updated2.Variants, 2)
	assert.Equal(t, uint64(1_000_000_000), updated2.Variants[0].ParameterCount)

	// Merge
	merged, err := MergeModelVariants(freeze, []*evalv1.ModelVariant{
		{ServedModelTag: "model-c:3b", ModelDigest: repeatHex('d', 64), ModelFamily: "test"},
	})
	require.NoError(t, err)
	require.Len(t, merged.Variants, 2)
	assert.Equal(t, "model-c:3b", merged.Variants[1].ServedModelTag)
}

func TestRemoveModelVariant(t *testing.T) {
	t.Parallel()
	freeze, err := MaterializeModelRegistry("campaign-1", []*evalv1.ModelVariant{
		{
			VariantId:      "model-a-1b",
			ProviderClass:  "ollama",
			ServedModelTag: "model-a:1b",
			ModelDigest:    repeatHex('a', 64),
			ModelFamily:    "test",
		},
		{
			VariantId:      "model-b-2b",
			ProviderClass:  "ollama",
			ServedModelTag: "model-b:2b",
			ModelDigest:    repeatHex('b', 64),
			ModelFamily:    "test",
		},
	})
	require.NoError(t, err)

	// Remove by served tag
	updated, removed, err := RemoveModelVariant(freeze, "model-a:1b")
	require.NoError(t, err)
	require.NotNil(t, removed)
	assert.Equal(t, "model-a:1b", removed.ServedModelTag)
	require.Len(t, updated.Variants, 1)
	assert.Equal(t, "model-b:2b", updated.Variants[0].ServedModelTag)

	// Remove by variant ID
	updated2, removed2, err := RemoveModelVariant(freeze, "model-b-2b")
	require.NoError(t, err)
	require.NotNil(t, removed2)
	assert.Equal(t, "model-b:2b", removed2.ServedModelTag)
	require.Len(t, updated2.Variants, 1)
	assert.Equal(t, "model-a:1b", updated2.Variants[0].ServedModelTag)

	// Non-existent model fails
	_, _, err = RemoveModelVariant(freeze, "non-existent:1b")
	require.Error(t, err)

	// Removing the last model fails
	_, _, err = RemoveModelVariant(updated, "model-b:2b")
	require.Error(t, err)
}
