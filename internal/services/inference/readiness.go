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
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// VerifyProviderReady performs the read-only startup readiness check against
// the configured remote inference provider. It queries Backend.Status and
// verifies that every configured role model is present in the provider's
// model store. It never mutates the provider: no pulls, creates, renames,
// or deletes. It fails closed with centralized typed errors for an
// unreachable provider (ErrInferenceBackendUnavailable), a malformed
// response (ErrInferenceProviderResponseInvalid), and a missing configured
// model (ErrInferenceModelNotFound). Roles without a configured model are
// skipped: they fail closed at request time via ErrInferenceModelRefInvalid.
func VerifyProviderReady(ctx context.Context, backend Backend, cfg config.InferenceConfig) error {
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

	required := []struct {
		role  models.InferenceModelRole
		model string
	}{
		{models.InferenceModelRolePrimary, cfg.PrimaryModel},
		{models.InferenceModelRoleAssistant, cfg.AssistantModel},
		{models.InferenceModelRoleLite, cfg.LiteModel},
	}
	for _, rm := range required {
		if rm.model == "" {
			continue
		}
		if !providerModelPresent(status.Models, rm.model) {
			return fmt.Errorf("inference: provider readiness: %w: role %d configured model %q", constants.ErrInferenceModelNotFound, rm.role, rm.model)
		}
	}
	return nil
}

// providerModelPresent reports whether the configured model resolves against
// the provider's model store. Ollama resolves an untagged model name to the
// ":latest" tag, so a configured "gemma3" matches a stored "gemma3:latest";
// tagged names must match exactly.
func providerModelPresent(available []string, want string) bool {
	candidate := want
	if !strings.Contains(want, ":") {
		candidate = want + ":latest"
	}
	for _, name := range available {
		if name == want || name == candidate {
			return true
		}
	}
	return false
}
