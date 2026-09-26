// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// ModelRegistryFreeze is the immutable campaign model registry derived from a
// live provider inventory query. It is used to commit the campaign Inference
// Operator and to authorize governed campaign-mode dispatches.
type ModelRegistryFreeze struct {
	CampaignID string
	Digest     string
	Variants   []*operatorv1.InferenceModelVariant
}

// LookupModelVariant returns the frozen variant for model when present.
func (freeze *ModelRegistryFreeze) LookupModelVariant(model string) (*operatorv1.InferenceModelVariant, error) {
	if freeze == nil {
		return nil, fmt.Errorf("evaluation: lookup model variant: %w", constants.ErrMissingRequiredField)
	}
	for _, variant := range freeze.Variants {
		if variant != nil && variant.GetModel() == model {
			return variant, nil
		}
	}
	return nil, fmt.Errorf("evaluation: lookup model variant: %w: %s", constants.ErrInferenceModelNotFound, model)
}
