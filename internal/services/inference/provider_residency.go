// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ProviderResidencyModel is one model currently resident in the provider.
type ProviderResidencyModel struct {
	Name string `json:"name"`
}

// ProviderResidency is the typed Ollama /api/ps response used for residency
// postconditions. It does not claim that a provider is idle or that no request
// can start between samples.
type ProviderResidency struct {
	Models []ProviderResidencyModel `json:"models"`
}

// ReadResidency reads and validates one bounded typed /api/ps response. It is
// bounded by ProviderStatusTimeout like every other read-only provider query.
func (b *OllamaBackend) ReadResidency(ctx context.Context) (ProviderResidency, error) {
	ctx, cancel := context.WithTimeout(ctx, ProviderStatusTimeout)
	defer cancel()
	var residency ProviderResidency
	if err := b.getJSON(ctx, "read residency", &residency, "api", "ps"); err != nil {
		return ProviderResidency{}, err
	}
	for _, model := range residency.Models {
		if model.Name == "" {
			return ProviderResidency{}, fmt.Errorf("ollama_backend: read residency: %w: model name is required", constants.ErrInferenceProviderResponseInvalid)
		}
	}
	return residency, nil
}
