// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License 2.0.

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func ultraEfficientSpeedsterRegistryVariants() []*evalv1.ModelVariant {
	return []*evalv1.ModelVariant{
		{
			VariantId:      "qwen35-4b",
			ProviderClass:  "ollama",
			ServedModelTag: "qwen3.5:4b",
			ModelDigest:    repeatHex('a', 64),
			ModelFamily:    "Qwen 3.5",
			ParameterCount: 4_000_000_000,
			Quantization:   "Q4_K_M",
		},
		{
			VariantId:      "gemma4-e2b",
			ProviderClass:  "ollama",
			ServedModelTag: "gemma4:e2b",
			ModelDigest:    repeatHex('b', 64),
			ModelFamily:    "Gemma 4",
			ParameterCount: 5_100_000_000,
			Quantization:   "Q4_K_M",
		},
		{
			VariantId:      "llama32-1b-speed",
			ProviderClass:  "ollama",
			ServedModelTag: "llama3.2:1b",
			ModelDigest:    repeatHex('c', 64),
			ModelFamily:    "Llama 3.2",
			ParameterCount: 1_200_000_000,
			Quantization:   "Q8_0",
		},
	}
}

func ultraEfficientSpeedsterBindingRequest(t *testing.T) FormationBindingRequest {
	t.Helper()
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	formation, err := topologies.Formation("ultra-efficient-speedster")
	require.NoError(t, err)
	stack, err := formation.ToStackDefinition()
	require.NoError(t, err)
	return FormationBindingRequest{
		Stack:    stack,
		Variants: ultraEfficientSpeedsterRegistryVariants(),
	}
}

func TestFormationBindingFromCatalog_MaterializesUltraEfficientSpeedsterStack(t *testing.T) {
	binding, err := FormationBindingFromCatalog("ultra-efficient-speedster", ultraEfficientSpeedsterRegistryVariants())
	require.NoError(t, err)
	assert.Equal(t, "ultra-efficient-speedster", binding.FormationID)
	assert.Equal(t, "ultra-efficient-speedster", binding.Stack.GetStackId())
	require.NoError(t, ValidateHeterogeneousStackDigest(binding.Stack))
}

func TestBindFormation_BindsUltraEfficientSpeedsterDigestsFromFrozenRegistry(t *testing.T) {
	req := ultraEfficientSpeedsterBindingRequest(t)

	formation, err := BindFormation(req)
	require.NoError(t, err)
	assert.Equal(t, repeatHex('a', 64), formation.Primary.ModelDigest)
	assert.Equal(t, repeatHex('b', 64), formation.Assistant.ModelDigest)
	assert.Equal(t, repeatHex('c', 64), formation.Lite.ModelDigest)
	assert.Equal(t, "qwen3.5:4b", formation.Primary.ServedModelTag)
}

func TestBindFormation_PrefersServedTagOverCollidingVariantID(t *testing.T) {
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	formation, err := topologies.Formation("gemma-cascade")
	require.NoError(t, err)
	stack, err := formation.ToStackDefinition()
	require.NoError(t, err)

	variants := formationCatalogTestVariants()
	variants = append(variants, &evalv1.ModelVariant{
		VariantId:      "gemma4-e4b",
		ProviderClass:  "ollama",
		ServedModelTag: "gemma4:e4b-other",
		ModelDigest:    repeatHex('x', 64),
		ModelFamily:    "gemma4",
		Quantization:   "Q4_0",
	})

	bound, err := BindFormation(FormationBindingRequest{
		FormationID: "gemma-cascade",
		Stack:       stack,
		Variants:    variants,
	})
	require.NoError(t, err)
	assert.Equal(t, "gemma4:e4b", bound.Primary.ServedModelTag)
	assert.Equal(t, repeatHex('6', 64), bound.Primary.ModelDigest)
}

func TestBindFormation_BindsUltraEfficientSpeedsterByServedTagWhenFreezeVariantIDsDiffer(t *testing.T) {
	req := ultraEfficientSpeedsterBindingRequest(t)
	req.Variants = []*evalv1.ModelVariant{
		{
			VariantId:      "qwen3-5-4b",
			ProviderClass:  "ollama",
			ServedModelTag: "qwen3.5:4b",
			ModelDigest:    repeatHex('a', 64),
			ModelFamily:    "Qwen 3.5",
			ParameterCount: 4_000_000_000,
			Quantization:   "Q4_K_M",
		},
		{
			VariantId:      "gemma4-e2b-q4-k-m",
			ProviderClass:  "ollama",
			ServedModelTag: "gemma4:e2b",
			ModelDigest:    repeatHex('b', 64),
			ModelFamily:    "Gemma 4",
			ParameterCount: 5_100_000_000,
			Quantization:   "Q4_K_M",
		},
		{
			VariantId:      "llama3-2-1b-q8-0",
			ProviderClass:  "ollama",
			ServedModelTag: "llama3.2:1b",
			ModelDigest:    repeatHex('c', 64),
			ModelFamily:    "Llama 3.2",
			ParameterCount: 1_200_000_000,
			Quantization:   "Q8_0",
		},
	}

	formation, err := BindFormation(req)
	require.NoError(t, err)
	assert.Equal(t, "qwen3-5-4b", formation.Primary.VariantID)
	assert.Equal(t, repeatHex('a', 64), formation.Primary.ModelDigest)
	assert.Equal(t, "gemma4-e2b-q4-k-m", formation.Assistant.VariantID)
	assert.Equal(t, "llama3-2-1b-q8-0", formation.Lite.VariantID)
}

func TestBindFormation_RejectsMissingRegistryVariant(t *testing.T) {
	req := ultraEfficientSpeedsterBindingRequest(t)
	req.Variants = req.Variants[:2]

	_, err := BindFormation(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationRegistryBinding)
}

func TestBindFormation_RejectsEmptySovereignDigest(t *testing.T) {
	req := ultraEfficientSpeedsterBindingRequest(t)
	req.Variants[0].ModelDigest = ""

	_, err := BindFormation(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationAttestationRequired)
}

func TestBindFormation_RejectsServedTagMismatch(t *testing.T) {
	req := ultraEfficientSpeedsterBindingRequest(t)
	req.Variants[0].ServedModelTag = "wrong:tag"

	_, err := BindFormation(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationStackMismatch)
}

func TestBindFormation_RejectsInvalidStackDigest(t *testing.T) {
	req := ultraEfficientSpeedsterBindingRequest(t)
	req.Stack.StackDigest = "invalid"

	_, err := BindFormation(req)
	require.Error(t, err)
}

func TestBindFormation_RejectsUnknownFormationID(t *testing.T) {
	req := ultraEfficientSpeedsterBindingRequest(t)
	req.FormationID = "missing-formation"

	_, err := BindFormation(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationInvalid)
}

func TestBindHeterogeneousStack_BindsSchedulerStackFromRegistry(t *testing.T) {
	variants := ultraEfficientSpeedsterRegistryVariants()
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	catalogFormation, err := topologies.Formation("ultra-efficient-speedster")
	require.NoError(t, err)
	stack, err := catalogFormation.ToStackDefinition()
	require.NoError(t, err)

	formation, err := BindHeterogeneousStack(FormationBindingRequest{
		FormationID: stack.GetStackId(),
		Stack:       stack,
		Variants:    variants,
	})
	require.NoError(t, err)
	assert.True(t, formation.RelaxedValidation)
	assert.Equal(t, repeatHex('a', 64), formation.Primary.ModelDigest)
	assert.Equal(t, repeatHex('c', 64), formation.Lite.ModelDigest)
}

func TestFormationHarness_RunsBoundUltraEfficientSpeedsterFormation(t *testing.T) {
	formation, err := BindFormation(ultraEfficientSpeedsterBindingRequest(t))
	require.NoError(t, err)

	harness, err := NewFormationHarness(
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-attempt" },
	)
	require.NoError(t, err)

	result, err := harness.RunBoundFormation(context.Background(), formation, []byte("initial"))
	require.NoError(t, err)
	require.True(t, result.Passed)
	assert.Equal(t, FormationSchemaVersion, result.SchemaVersion)
	assert.Equal(t, "ultra-efficient-speedster", result.FormationID)
	assert.Len(t, result.Roles, 3)
	assert.Equal(t, 1, harness.Policy.Calls)
	assert.Equal(t, []string{
		"lite:initial",
		"assistant:initial/lite",
		"primary:initial/lite/assistant",
	}, harness.Executor.States)
	assert.Equal(t, []string{
		"allocate:qwen35-4b",
		"allocate:gemma4-e2b",
		"allocate:llama32-1b-speed",
		"release:llama32-1b-speed",
		"release:gemma4-e2b",
		"release:qwen35-4b",
	}, harness.Allocator.Events)
}
