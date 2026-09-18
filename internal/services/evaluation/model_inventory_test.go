// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func textProbeResponse(text string) *models.GenerateResponse {
	return &models.GenerateResponse{Parts: []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: text}}}}
}

func TestBuildModelVariantsFromProviderInventory_PreservesEveryServedTag(t *testing.T) {
	entries := []inference.ProviderModelInventoryEntry{
		{ServedModelTag: "gemma3:4b", ModelDigest: repeatHex('a', 64), ModelFamily: "gemma3", ParameterCount: 4_000_000_000, Quantization: "Q4_K_M", ContextLimit: 8192, ProviderClass: "ollama"},
		{ServedModelTag: "gemma3:4b-instruct", ModelDigest: repeatHex('b', 64), ModelFamily: "gemma3", ParameterCount: 4_000_000_000, Quantization: "Q4_K_M", ContextLimit: 8192, ProviderClass: "ollama"},
	}
	variants, err := BuildModelVariantsFromProviderInventory(context.Background(), entries, ModelInventoryOptions{})
	require.NoError(t, err)
	require.Len(t, variants, 2)
	assert.Equal(t, "gemma3-4b", variants[0].GetVariantId())
	assert.Equal(t, "gemma3:4b", variants[0].GetServedModelTag())
	assert.Equal(t, "gemma3-4b-instruct", variants[1].GetVariantId())
}

func TestMaterializeModelRegistry_BindsDigestAndMatrixSize(t *testing.T) {
	variants := []*evalv1.ModelVariant{
		{VariantId: "qwen3-4b", ProviderClass: "ollama", ServedModelTag: "qwen3:4b", ModelDigest: repeatHex('b', 64)},
		{VariantId: "gemma3-4b", ProviderClass: "ollama", ServedModelTag: "gemma3:4b", ModelDigest: repeatHex('c', 64)},
	}
	freeze, err := MaterializeModelRegistry("north-star-smoke", variants)
	require.NoError(t, err)
	assert.Equal(t, uint64(150), freeze.HomogeneousCellCount)
	require.NoError(t, ValidateModelRegistry(freeze))

	freeze.RegistryDigest = repeatHex('f', 64)
	assert.Error(t, ValidateModelRegistry(freeze))
}

func TestComputeHomogeneousMatrixSize(t *testing.T) {
	assert.Equal(t, uint64(75), ComputeHomogeneousMatrixSize(1))
	assert.Equal(t, uint64(2625), ComputeHomogeneousMatrixSize(35))
}

func TestBuildModelVariantsFromProviderInventory_AttachesCapabilityObservations(t *testing.T) {
	entries := []inference.ProviderModelInventoryEntry{
		{ServedModelTag: "probe-model:latest", ModelDigest: repeatHex('d', 64), ProviderClass: "ollama", ContextLimit: 4096},
	}
	backend := &stubCapabilityProbeBackend{byAttempt: map[string]func(models.GenerateRequest) (*models.GenerateResponse, error){
		capabilityProbeAttemptPrefix + "-completion": func(models.GenerateRequest) (*models.GenerateResponse, error) {
			return textProbeResponse("capability-probe-ok"), nil
		},
		capabilityProbeAttemptPrefix + "-tools": func(models.GenerateRequest) (*models.GenerateResponse, error) {
			return nil, fmt.Errorf("tools unsupported")
		},
		capabilityProbeAttemptPrefix + "-structured": func(models.GenerateRequest) (*models.GenerateResponse, error) {
			return textProbeResponse(`{"answer":"capability-probe-json"}`), nil
		},
		capabilityProbeAttemptPrefix + "-thinking": func(models.GenerateRequest) (*models.GenerateResponse, error) {
			return textProbeResponse("capability-probe-thinking"), nil
		},
	}}
	variants, err := BuildModelVariantsFromProviderInventory(context.Background(), entries, ModelInventoryOptions{
		RunCapabilityProbes: true,
		ProbeBackend:        backend,
	})
	require.NoError(t, err)
	require.Len(t, variants, 1)
	require.Len(t, variants[0].GetCapabilityObservations(), len(RequiredModelCapabilityKinds()))
}
