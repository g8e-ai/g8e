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

func ultraLightSpeedsterRegistryVariants() []*evalv1.ModelVariant {
	return []*evalv1.ModelVariant{
		{
			VariantId:      "phi35-mini-38b-speed",
			ProviderClass:  "ollama",
			ServedModelTag: "phi3.5:3.8b-mini-instruct-q4_K_M",
			ModelDigest:    repeatHex('a', 64),
			ModelFamily:    "Phi-3.5",
			ParameterCount: 3_800_000_000,
			Quantization:   "Q4_K_M",
		},
		{
			VariantId:      "gemma2-2b-speed",
			ProviderClass:  "ollama",
			ServedModelTag: "gemma2:2b-instruct-q4_K_M",
			ModelDigest:    repeatHex('b', 64),
			ModelFamily:    "Gemma 2",
			ParameterCount: 2_000_000_000,
			Quantization:   "Q4_K_M",
		},
		{
			VariantId:      "qwen25-05b-speed",
			ProviderClass:  "ollama",
			ServedModelTag: "qwen2.5:0.5b-instruct-q4_K_M",
			ModelDigest:    repeatHex('c', 64),
			ModelFamily:    "Qwen 2.5",
			ParameterCount: 500_000_000,
			Quantization:   "Q4_K_M",
		},
	}
}

func ultraLightSpeedsterBindingRequest(t *testing.T) FormationBindingRequest {
	t.Helper()
	topologies, err := NewExecutionTopologies()
	require.NoError(t, err)
	formation, err := topologies.Formation("ultra-light-speedster")
	require.NoError(t, err)
	stack, err := formation.ToStackDefinition()
	require.NoError(t, err)
	return FormationBindingRequest{
		Stack:    stack,
		Variants: ultraLightSpeedsterRegistryVariants(),
	}
}

func TestFormationBindingFromCatalog_MaterializesUltraLightSpeedsterStack(t *testing.T) {
	binding, err := FormationBindingFromCatalog("ultra-light-speedster", ultraLightSpeedsterRegistryVariants())
	require.NoError(t, err)
	assert.Equal(t, "ultra-light-speedster", binding.FormationID)
	assert.Equal(t, "ultra-light-speedster", binding.Stack.GetStackId())
	require.NoError(t, ValidateHeterogeneousStackDigest(binding.Stack))
}

func TestBindFormation_BindsUltraLightSpeedsterDigestsFromFrozenRegistry(t *testing.T) {
	req := ultraLightSpeedsterBindingRequest(t)

	formation, err := BindFormation(req)
	require.NoError(t, err)
	assert.Equal(t, repeatHex('a', 64), formation.Primary.ModelDigest)
	assert.Equal(t, repeatHex('b', 64), formation.Assistant.ModelDigest)
	assert.Equal(t, repeatHex('c', 64), formation.Lite.ModelDigest)
	assert.Equal(t, "phi3.5:3.8b-mini-instruct-q4_K_M", formation.Primary.ServedModelTag)
}

func TestBindFormation_RejectsMissingRegistryVariant(t *testing.T) {
	req := ultraLightSpeedsterBindingRequest(t)
	req.Variants = req.Variants[:2]

	_, err := BindFormation(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationRegistryBinding)
}

func TestBindFormation_RejectsEmptySovereignDigest(t *testing.T) {
	req := ultraLightSpeedsterBindingRequest(t)
	req.Variants[0].ModelDigest = ""

	_, err := BindFormation(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationAttestationRequired)
}

func TestBindFormation_RejectsServedTagMismatch(t *testing.T) {
	req := ultraLightSpeedsterBindingRequest(t)
	req.Variants[0].ServedModelTag = "wrong:tag"

	_, err := BindFormation(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationStackMismatch)
}

func TestBindFormation_RejectsInvalidStackDigest(t *testing.T) {
	req := ultraLightSpeedsterBindingRequest(t)
	req.Stack.StackDigest = "invalid"

	_, err := BindFormation(req)
	require.Error(t, err)
}

func TestBindFormation_RejectsUnknownFormationID(t *testing.T) {
	req := ultraLightSpeedsterBindingRequest(t)
	req.FormationID = "missing-formation"

	_, err := BindFormation(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationInvalid)
}

func TestFormationHarness_RunsBoundUltraLightSpeedsterFormation(t *testing.T) {
	formation, err := BindFormation(ultraLightSpeedsterBindingRequest(t))
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
	assert.Equal(t, "ultra-light-speedster", result.FormationID)
	assert.Len(t, result.Roles, 3)
	assert.Equal(t, 1, harness.Policy.Calls)
	assert.Equal(t, []string{
		"lite:initial",
		"assistant:initial/lite",
		"primary:initial/lite/assistant",
	}, harness.Executor.States)
	assert.Equal(t, []string{
		"allocate:phi35-mini-38b-speed",
		"allocate:gemma2-2b-speed",
		"allocate:qwen25-05b-speed",
		"release:qwen25-05b-speed",
		"release:gemma2-2b-speed",
		"release:phi35-mini-38b-speed",
	}, harness.Allocator.Events)
}
