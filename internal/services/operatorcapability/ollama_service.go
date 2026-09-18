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

const (
	OllamaServiceCommandStop   = "ollama stop"
	OllamaServiceCommandStart  = "ollama start"
	OllamaServiceCommandStatus = "ollama status"
)

// IsOllamaServiceCommand reports whether command is a governed Ollama service
// lifecycle invocation (stop, start, or status).
func IsOllamaServiceCommand(command string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(command))
	switch trimmed {
	case OllamaServiceCommandStop, OllamaServiceCommandStart, OllamaServiceCommandStatus:
		return true
	default:
		return false
	}
}

// ProviderBoundaryObserverOllamaEnabled returns true when the operator started
// with --provider-boundary-observer-enabled --ollama.
func ProviderBoundaryObserverOllamaEnabled(cfg *models.RuntimeConfig) bool {
	return cfg != nil && cfg.ProviderBoundaryObserverEnabled && cfg.ProviderBoundaryObserverOllamaEnabled
}

// ValidateOllamaServiceCommand rejects Ollama service lifecycle commands when
// the target operator did not opt in at startup with --ollama.
func ValidateOllamaServiceCommand(cfg *models.RuntimeConfig, command string) error {
	if !IsOllamaServiceCommand(command) {
		return nil
	}
	if ProviderBoundaryObserverOllamaEnabled(cfg) {
		return nil
	}
	return fmt.Errorf("%w: command %q", constants.ErrProviderBoundaryObserverOllamaNotCapable, strings.TrimSpace(command))
}

// RestartSettleCommand returns a short blocking delay command appropriate for
// the operator host platform recorded in runtime_config.platform.
func RestartSettleCommand(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "windows":
		return "ping 127.0.0.1 -n 6 > nul"
	default:
		return "sleep 5"
	}
}
