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
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type modelInventoryFreezePayload struct {
	CampaignID          string            `json:"campaign_id"`
	ModelRegistryDigest string            `json:"model_registry_digest"`
	Variants            []json.RawMessage `json:"variants"`
}

func TestComputeHomogeneousMatrixSize(t *testing.T) {
	t.Parallel()
	assert.Equal(t, uint64(75), ComputeHomogeneousMatrixSize(1))
	assert.Equal(t, uint64(300), ComputeHomogeneousMatrixSize(4))
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
	require.NoError(t, os.WriteFile(path, payload, 0o644))

	loaded, err := LoadModelInventoryFreezeFile(path)
	require.NoError(t, err)
	assert.Equal(t, freeze.RegistryDigest, loaded.RegistryDigest)
	assert.Equal(t, freeze.HomogeneousCellCount, loaded.HomogeneousCellCount)
	require.NoError(t, ValidateModelRegistry(loaded))
}

func TestLoadModelInventoryFreezeFile_RejectsMissingDigest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"campaign_id":"c1","variants":[]}`), 0o644))
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

func TestBuildModelVariantsFromProviderInventory_RequiresProbeBackendWhenRequested(t *testing.T) {
	t.Parallel()
	entries := []inference.ProviderModelInventoryEntry{
		{ServedModelTag: "probe:7b", ModelDigest: repeatHex('a', 64)},
	}
	_, err := BuildModelVariantsFromProviderInventory(context.Background(), entries, ModelInventoryOptions{RunCapabilityProbes: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe backend")
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
