// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestHasCampaignAuthority(t *testing.T) {
	t.Parallel()
	variants := []*operatorv1.InferenceModelVariant{{Model: "m", Digest: strings.Repeat("a", 64)}}
	tests := []struct {
		name       string
		campaignID string
		digest     string
		registry   []*operatorv1.InferenceModelVariant
		want       bool
	}{
		{name: "none"},
		{name: "campaign id only", campaignID: "c1", want: true},
		{name: "registry digest only", digest: strings.Repeat("b", 64), want: true},
		{name: "registry only", registry: variants, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, HasCampaignAuthority(test.campaignID, test.digest, test.registry))
		})
	}
}

func TestVerifyModelRegistryBinding(t *testing.T) {
	t.Parallel()
	const campaignID = "campaign-1"
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)
	registry := []*operatorv1.InferenceModelVariant{
		{Model: "model-a", Digest: digestA},
		{Model: "model-b", Digest: digestB},
	}
	registryDigest, err := models.ComputeInferenceModelRegistryDigest(campaignID, registry)
	require.NoError(t, err)

	tests := []struct {
		name           string
		registry       []*operatorv1.InferenceModelVariant
		registryDigest string
		model          string
		modelDigest    string
		wantErr        error
	}{
		{name: "bound model", registry: registry, registryDigest: registryDigest, model: "model-a", modelDigest: digestA},
		{name: "model not in registry", registry: registry, registryDigest: registryDigest, model: "model-c", modelDigest: digestA, wantErr: constants.ErrInferenceModelOverrideDenied},
		{name: "model digest differs from registry", registry: registry, registryDigest: registryDigest, model: "model-a", modelDigest: digestB, wantErr: constants.ErrInferenceModelOverrideDenied},
		{name: "registry digest mismatch", registry: registry, registryDigest: digestA, model: "model-a", modelDigest: digestA, wantErr: constants.ErrInferenceModelRegistryInvalid},
		{name: "nil variant", registry: []*operatorv1.InferenceModelVariant{nil}, registryDigest: registryDigest, model: "model-a", modelDigest: digestA, wantErr: constants.ErrInferenceModelRegistryInvalid},
		{name: "variant without model", registry: []*operatorv1.InferenceModelVariant{{Digest: digestA}}, registryDigest: registryDigest, model: "model-a", modelDigest: digestA, wantErr: constants.ErrInferenceModelRegistryInvalid},
		{name: "variant with malformed digest", registry: []*operatorv1.InferenceModelVariant{{Model: "model-a", Digest: "nope"}}, registryDigest: registryDigest, model: "model-a", modelDigest: digestA, wantErr: constants.ErrInferenceModelRegistryInvalid},
		{
			name:           "duplicate model",
			registry:       []*operatorv1.InferenceModelVariant{{Model: "model-a", Digest: digestA}, {Model: "model-a", Digest: digestB}},
			registryDigest: registryDigest,
			model:          "model-a",
			modelDigest:    digestA,
			wantErr:        constants.ErrInferenceModelRegistryInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := VerifyModelRegistryBinding(campaignID, test.registryDigest, test.model, test.modelDigest, test.registry)
			if test.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, test.wantErr)
		})
	}
}
