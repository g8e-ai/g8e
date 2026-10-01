// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func testHeterogeneousVariants() []*evalv1.ModelVariant {
	return []*evalv1.ModelVariant{
		{VariantId: "qwen3-8b", ProviderClass: "ollama", ServedModelTag: "qwen3:8b", ModelDigest: repeatHex('a', 64), ModelFamily: "qwen3", ParameterCount: 8_000_000_000},
		{VariantId: "phi4-mini", ProviderClass: "ollama", ServedModelTag: "phi4:mini", ModelDigest: repeatHex('b', 64), ModelFamily: "phi", ParameterCount: 3_800_000_000, CapabilityObservations: []*evalv1.ModelCapabilityObservation{
			{Capability: evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING, Outcome: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS},
		}},
		{VariantId: "smollm2-360m", ProviderClass: "ollama", ServedModelTag: "smollm2:360m", ModelDigest: repeatHex('c', 64), ModelFamily: "smollm", ParameterCount: 360_000_000, Quantization: "q4_0"},
		{VariantId: "gemma3-4b", ProviderClass: "ollama", ServedModelTag: "gemma3:4b", ModelDigest: repeatHex('d', 64), ModelFamily: "gemma", ParameterCount: 4_000_000_000, Quantization: "q8_0"},
	}
}

// A stack set persisted under the pre-formation generation rule must keep
// validating so its runs stay resumable and verifiable.
func TestValidateHeterogeneousStackSet_AcceptsAPersistedLegacyRuleSet(t *testing.T) {
	set, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "heterogeneous-campaign",
		Seed:       42,
		Variants:   testHeterogeneousVariants(),
	})
	require.NoError(t, err)
	assert.Equal(t, HeterogeneousStackGenerationRule, set.GenerationRule)
	for _, variantID := range set.VariantIDs {
		assert.Len(t, set.Coverage.VariantRoleCoverage[variantID], 3)
	}
	require.NoError(t, ValidateHeterogeneousStackSet(set))
}

func TestValidateHeterogeneousStackSet_RejectsAVariantMissingARole(t *testing.T) {
	set, err := GenerateHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: "heterogeneous-campaign",
		Variants:   testHeterogeneousVariants(),
	})
	require.NoError(t, err)
	set.Stacks = set.Stacks[:1]
	set.SetDigest, err = ComputeHeterogeneousStackSetDigest(set)
	require.NoError(t, err)
	assert.ErrorContains(t, ValidateHeterogeneousStackSet(set), "missing role coverage")
}

func TestComputeHeterogeneousStackDigest_RejectsDuplicateVariants(t *testing.T) {
	variants := testHeterogeneousVariants()
	_, err := materializeStack("campaign", "invalid", variants[0], variants[0], variants[1])
	assert.Error(t, err)
}
