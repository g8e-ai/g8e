// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"fmt"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// OllamaReleaseCommand returns the fixed embedded-client model-release command
// for a validated served model tag.
func OllamaReleaseCommand(modelTag string) (string, error) {
	if modelTag == "" || strings.HasPrefix(modelTag, "-") {
		return "", fmt.Errorf("%w: %q", constants.ErrInferenceModelTagInvalid, modelTag)
	}
	for _, character := range modelTag {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._:/-", character) {
			continue
		}
		return "", fmt.Errorf("%w: %q", constants.ErrInferenceModelTagInvalid, modelTag)
	}
	return constants.ContainerBinaryPath + " operator model release " + modelTag, nil
}

// ValidateWitnessCommand rejects generic command execution on read-only witness
// operators. Their narrowly scoped observation handlers remain available.
func ValidateWitnessCommand(cfg *models.RuntimeConfig, command string) error {
	if cfg == nil || (!cfg.ProviderBoundaryObserverEnabled && !cfg.ProvenanceOperatorEnabled) {
		return nil
	}
	return fmt.Errorf("%w: command %q", constants.ErrWitnessCommandNotCapable, strings.TrimSpace(command))
}
