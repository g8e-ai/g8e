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

func TestFormationCatalogServedTags_ReturnsSortedUniqueTags(t *testing.T) {
	tags := FormationCatalogServedTags()
	require.Len(t, tags, 11)
	sort.Strings(tags)
	assert.Equal(t, FormationCatalogServedTags(), tags)
	assert.Contains(t, tags, "qwen3.5:9b")
}

func TestFormationCatalogIntakeModels_ReturnsOneSovereignLibraryPullPerTag(t *testing.T) {
	models, err := FormationCatalogIntakeModels()
	require.NoError(t, err)
	require.Len(t, models, 11)
	for _, model := range models {
		assert.Equal(t, "ollama_library_pull", model.Staging.Method)
		assert.Equal(t, model.ServedModelTag, model.Staging.OllamaPull)
	}
}

func TestMaterializeFormationCatalogVariants_SelectsAllSovereignCatalogTags(t *testing.T) {
	partial := []*evalv1.ModelVariant{
		{VariantId: "freeze-qwen35-4b", ProviderClass: "ollama", ServedModelTag: "qwen3.5:4b", ModelDigest: repeatHex('5', 64), ModelFamily: "Qwen 3.5", Quantization: "Q4_K_M"},
		{VariantId: "freeze-gemma4-e2b", ProviderClass: "ollama", ServedModelTag: "gemma4:e2b", ModelDigest: repeatHex('2', 64), ModelFamily: "Gemma 4", Quantization: "Q4_K_M"},
		{VariantId: "freeze-llama32-1b", ProviderClass: "ollama", ServedModelTag: "llama3.2:1b", ModelDigest: repeatHex('6', 64), ModelFamily: "Llama 3.2", Quantization: "Q8_0"},
	}
	_, err := MaterializeFormationCatalogVariants(partial)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelNotFound)

	source := FormationCatalogFixtureVariants(func(string) string { return repeatHex('a', 64) })
	selected, err := MaterializeFormationCatalogVariants(source)
	require.NoError(t, err)
	require.Len(t, selected, 11)

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

func TestMaterializeFormationVariants_SelectsOnlyTheNamedFormationsModels(t *testing.T) {
	source := FormationCatalogFixtureVariants(func(string) string { return repeatHex('a', 64) })

	selected, err := MaterializeFormationVariants(source, []string{"qwen-powerhouse"})
	require.NoError(t, err)
	tags := make([]string, 0, len(selected))
	for _, variant := range selected {
		tags = append(tags, variant.GetServedModelTag())
	}
	assert.Equal(t, []string{
		"gemma3:1b",
		"ministral-3:3b",
		"qwen3.5:9b",
	}, tags)

	all, err := MaterializeFormationVariants(source, nil)
	require.NoError(t, err)
	assert.Len(t, all, 11)

	_, err = MaterializeFormationVariants(source, []string{"no-such-formation"})
	require.ErrorIs(t, err, constants.ErrFormationInvalid)
}

func TestFormationCatalogFixtureVariants_MatchesCatalogServedTags(t *testing.T) {
	variants := FormationCatalogFixtureVariants(func(string) string { return repeatHex('b', 64) })
	require.Len(t, variants, 11)
	tags := make([]string, 0, len(variants))
	for _, variant := range variants {
		tags = append(tags, variant.GetServedModelTag())
	}
	sort.Strings(tags)
	assert.Equal(t, FormationCatalogServedTags(), tags)
}
