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

// VerifyProviderReady performs an explicit read-only readiness check against
// the configured remote inference provider. It queries Backend.Status and
// verifies the provider is reachable and answers with a well-formed status.
// It does not gate Operator startup. It never mutates the provider: no pulls, creates, renames, or deletes. The
// Operator configures no models, so there is none to verify here: each
// request's model is the user's choice and a model the provider lacks fails
// that request with ErrInferenceModelNotFound. It fails closed with centralized
// typed errors for an unreachable provider (ErrInferenceBackendUnavailable) and
// a malformed response (ErrInferenceProviderResponseInvalid).
func VerifyProviderReady(ctx context.Context, backend Backend) error {
	if backend == nil {
		return fmt.Errorf("inference: provider readiness: %w", constants.ErrInferenceBackendNotRegistered)
	}

	statusCtx, cancel := context.WithTimeout(ctx, ProviderStatusTimeout)
	defer cancel()
	status, err := backend.Status(statusCtx)
	if err != nil {
		return fmt.Errorf("inference: provider readiness: %w", err)
	}
	if status == nil || !status.Available {
		return fmt.Errorf("inference: provider readiness: %w", constants.ErrInferenceBackendUnavailable)
	}
	return nil
}
