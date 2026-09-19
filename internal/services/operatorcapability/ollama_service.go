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
	OllamaServiceCommandStop  = "ollama stop"
	OllamaServiceCommandServe = "ollama serve"
	OllamaServiceCommandPS    = "ollama ps"
)

// IsOllamaServiceCommand reports whether command is a governed Ollama CLI
// invocation (stop, serve, or ps).
func IsOllamaServiceCommand(command string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(command))
	switch trimmed {
	case OllamaServiceCommandStop, OllamaServiceCommandServe, OllamaServiceCommandPS:
		return true
	default:
		return false
	}
}

// RestartOllamaCommands returns the governed Ollama CLI sequence dispatched
// before each campaign assignment when the observer started with --ollama.
// stop unloads in-memory models; ps confirms the provider is quiescent. serve
// is omitted because it blocks in the foreground and the provider daemon is
// expected to stay running (Windows tray app or existing serve process).
func RestartOllamaCommands(platform string) []string {
	return []string{
		OllamaServiceCommandStop,
		RestartSettleCommand(platform),
		OllamaServiceCommandPS,
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
// the operator host platform recorded in runtime_config.platform. Commands must
// not require a POSIX shell on Windows: the operator execution service runs
// shell metacharacters through Git Bash, where cmd-style redirects like "> nul"
// are unreliable.
func RestartSettleCommand(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "windows":
		return "cmd.exe /C timeout /t 5 /nobreak"
	default:
		return "sleep 5"
	}
}
