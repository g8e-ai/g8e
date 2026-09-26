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

func validateServedModelTag(modelTag string) error {
	if modelTag == "" || strings.HasPrefix(modelTag, "-") {
		return fmt.Errorf("%w: %q", constants.ErrInferenceModelTagInvalid, modelTag)
	}
	for _, character := range modelTag {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._:/-", character) {
			continue
		}
		return fmt.Errorf("%w: %q", constants.ErrInferenceModelTagInvalid, modelTag)
	}
	return nil
}

// OllamaReleaseCommand returns the fixed embedded-client model-release command
// for a validated served model tag.
func OllamaReleaseCommand(modelTag string) (string, error) {
	if err := validateServedModelTag(modelTag); err != nil {
		return "", err
	}
	return constants.ContainerBinaryPath + " operator model release " + modelTag, nil
}

// OllamaPullCommand returns the embedded-client model-pull command for one
// validated served model tag.
func OllamaPullCommand(modelTag string) (string, error) {
	if err := validateServedModelTag(modelTag); err != nil {
		return "", err
	}
	return constants.ContainerBinaryPath + " operator model pull " + modelTag, nil
}

// OllamaCopyCommand returns the embedded-client model-copy command for one
// validated source and destination served model tag pair.
func OllamaCopyCommand(source, destination string) (string, error) {
	if err := validateServedModelTag(source); err != nil {
		return "", err
	}
	if err := validateServedModelTag(destination); err != nil {
		return "", err
	}
	return constants.ContainerBinaryPath + " operator model copy " + source + " " + destination, nil
}

// OllamaInventoryCommand returns the embedded-client provider inventory query.
func OllamaInventoryCommand() string {
	return constants.ContainerBinaryPath + " operator model inventory"
}

// OllamaResidencyCommand returns the embedded-client provider residency query.
func OllamaResidencyCommand() string {
	return constants.ContainerBinaryPath + " operator model residency"
}

// ValidateWitnessCommand rejects generic command execution on read-only witness
// operators. Their narrowly scoped observation handlers remain available.
func ValidateWitnessCommand(cfg *models.RuntimeConfig, command string) error {
	if cfg == nil || (!cfg.ProviderBoundaryObserverEnabled && !cfg.ProvenanceOperatorEnabled) {
		return nil
	}
	return fmt.Errorf("%w: command %q", constants.ErrWitnessCommandNotCapable, strings.TrimSpace(command))
}
