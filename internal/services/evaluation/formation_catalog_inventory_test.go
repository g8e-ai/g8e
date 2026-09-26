// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License 2.0.

package evaluation

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestFormationCatalogServedTags_ReturnsEightUniqueTags(t *testing.T) {
	tags := FormationCatalogServedTags()
	require.Len(t, tags, 8)
	sort.Strings(tags)
	assert.Equal(t, FormationCatalogServedTags(), tags)
	assert.Contains(t, tags, "qwen2.5:14b-instruct-q4_K_M")
}

func TestFormationCatalogIntakeModels_ReturnsEightSovereignLibraryPulls(t *testing.T) {
	models, err := FormationCatalogIntakeModels()
	require.NoError(t, err)
	require.Len(t, models, 8)
	for _, model := range models {
		assert.Equal(t, "ollama_library_pull", model.Staging.Method)
		assert.Equal(t, model.ServedModelTag, model.Staging.OllamaPull)
	}
}

func TestMaterializeFormationCatalogVariants_SelectsAllSovereignCatalogTags(t *testing.T) {
	partial := []*evalv1.ModelVariant{
		{VariantId: "freeze-phi35-mini", ProviderClass: "ollama", ServedModelTag: "phi3.5:3.8b-mini-instruct-q4_K_M", ModelDigest: repeatHex('5', 64), ModelFamily: "Phi-3.5", Quantization: "Q4_K_M"},
		{VariantId: "freeze-gemma2-2b", ProviderClass: "ollama", ServedModelTag: "gemma2:2b-instruct-q4_K_M", ModelDigest: repeatHex('2', 64), ModelFamily: "Gemma 2", Quantization: "Q4_K_M"},
		{VariantId: "freeze-qwen25-05b", ProviderClass: "ollama", ServedModelTag: "qwen2.5:0.5b-instruct-q4_K_M", ModelDigest: repeatHex('6', 64), ModelFamily: "Qwen 2.5", Quantization: "Q4_K_M"},
	}
	_, err := MaterializeFormationCatalogVariants(partial)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelNotFound)

	source := FormationCatalogFixtureVariants(func(string) string { return repeatHex('a', 64) })
	selected, err := MaterializeFormationCatalogVariants(source)
	require.NoError(t, err)
	require.Len(t, selected, 8)

	freeze, err := MaterializeModelRegistry("eval-formations-benchmark", selected)
	require.NoError(t, err)
	require.NoError(t, ValidateModelRegistry(freeze))
	_, err = GenerateFormationCatalogStackSet(FormationCatalogStackGenerationRequest{
		CampaignID: "eval-formations-benchmark",
		Seed:       17,
		Variants:   selected,
	})
	require.NoError(t, err)
}

func TestFormationCatalogFixtureVariants_MatchesCatalogServedTags(t *testing.T) {
	variants := FormationCatalogFixtureVariants(func(string) string { return repeatHex('b', 64) })
	require.Len(t, variants, 8)
	tags := make([]string, 0, len(variants))
	for _, variant := range variants {
		tags = append(tags, variant.GetServedModelTag())
	}
	sort.Strings(tags)
	assert.Equal(t, FormationCatalogServedTags(), tags)
}
