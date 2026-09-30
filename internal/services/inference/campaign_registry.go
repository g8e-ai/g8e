// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// HasCampaignAuthority reports whether a request carries any campaign binding
// field. Such a request must then satisfy the full campaign contract; it never
// falls back to the standard configured-model path.
func HasCampaignAuthority(campaignID, modelRegistryDigest string, modelRegistry []*operatorv1.InferenceModelVariant) bool {
	return campaignID != "" || modelRegistryDigest != "" || len(modelRegistry) != 0
}

// VerifyModelRegistryBinding verifies that every registry variant is
// well-formed and unique by model, that the registry hashes to
// modelRegistryDigest for campaignID, and that it contains exactly the
// requested (model, modelDigest) pair. The Gateway and the executing Operator
// each call it so neither trusts the other's verification.
func VerifyModelRegistryBinding(campaignID, modelRegistryDigest, model, modelDigest string, modelRegistry []*operatorv1.InferenceModelVariant) error {
	seen := make(map[string]struct{}, len(modelRegistry))
	matched := false
	for _, variant := range modelRegistry {
		if variant == nil || variant.GetModel() == "" || !models.IsSHA256Hex(variant.GetDigest()) {
			return constants.ErrInferenceModelRegistryInvalid
		}
		if _, exists := seen[variant.GetModel()]; exists {
			return constants.ErrInferenceModelRegistryInvalid
		}
		seen[variant.GetModel()] = struct{}{}
		if variant.GetModel() == model && variant.GetDigest() == modelDigest {
			matched = true
		}
	}
	digest, err := models.ComputeInferenceModelRegistryDigest(campaignID, modelRegistry)
	if err != nil || digest != modelRegistryDigest {
		return constants.ErrInferenceModelRegistryInvalid
	}
	if !matched {
		return constants.ErrInferenceModelOverrideDenied
	}
	return nil
}
