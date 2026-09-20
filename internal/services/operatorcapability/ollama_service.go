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

	// OllamaWindowsKillCommand stops the Ollama daemon on Windows provider hosts.
	OllamaWindowsKillCommand = "cmd.exe /C %SystemRoot%/System32/taskkill.exe /IM ollama.exe /F"
	// OllamaWindowsStartCommand starts the Ollama daemon in the background.
	OllamaWindowsStartCommand = "cmd.exe /C %SystemRoot%/System32/WindowsPowerShell/v1.0/powershell.exe -NoProfile -NonInteractive -Command Start-Process -FilePath %LOCALAPPDATA%/Programs/Ollama/ollama.exe -ArgumentList serve -WindowStyle Hidden"
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

// RestartOllamaCommands returns the governed command sequence dispatched
// before each campaign assignment when the observer started with --ollama.
// The sequence stops the daemon, starts it again, waits for readiness, and
// confirms the provider is quiescent with ollama ps.
func RestartOllamaCommands(platform string) []string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "windows":
		return []string{
			OllamaWindowsKillCommand,
			RestartPostKillSettleCommand("windows"),
			OllamaWindowsStartCommand,
			OllamaRestartReadySettleCommand("windows"),
			OllamaServiceCommandPS,
		}
	default:
		return []string{
			RestartOllamaDaemonCommand(platform),
			OllamaRestartReadySettleCommand(platform),
			OllamaServiceCommandPS,
		}
	}
}

// RestartOllamaDaemonCommand restarts the Ollama daemon on Unix provider
// hosts. systemctl is preferred when available; otherwise the process is
// recycled and serve is launched in the background.
func RestartOllamaDaemonCommand(platform string) string {
	_ = platform
	return `sh -c 'systemctl restart ollama 2>/dev/null || { pkill -x ollama 2>/dev/null; sleep 2; nohup ollama serve >/dev/null 2>&1 &; }'`
}

// RestartPostKillSettleCommand waits briefly after the daemon is stopped so
// provider ports and GPU contexts can drain before restart.
func RestartPostKillSettleCommand(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "windows":
		return "cmd.exe /C %SystemRoot%/System32/WindowsPowerShell/v1.0/powershell.exe -NoProfile -NonInteractive -Command Start-Sleep -Seconds 3"
	default:
		return "sleep 3"
	}
}

// OllamaRestartReadySettleCommand waits for the restarted daemon to accept RPCs.
func OllamaRestartReadySettleCommand(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "windows":
		return "cmd.exe /C %SystemRoot%/System32/WindowsPowerShell/v1.0/powershell.exe -NoProfile -NonInteractive -Command Start-Sleep -Seconds 8"
	default:
		return "sleep 8"
	}
}

// ToleratedOllamaRestartExitCode reports whether a non-zero return code from
// one restart lifecycle command can be ignored.
func ToleratedOllamaRestartExitCode(command string, returnCode int32) bool {
	if returnCode == 0 {
		return true
	}
	switch strings.TrimSpace(command) {
	case OllamaWindowsKillCommand:
		// taskkill: process not found (already stopped).
		return returnCode == 128
	case OllamaWindowsStartCommand:
		// start: daemon may already be running after tray auto-restart.
		return returnCode == 1
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
// the operator host platform recorded in runtime_config.platform. Commands must
// not require a POSIX shell on Windows: the operator execution service runs
// shell metacharacters through Git Bash, where cmd-style redirects like "> nul"
// are unreliable.
func RestartSettleCommand(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "windows":
		return "cmd.exe /C %SystemRoot%/System32/WindowsPowerShell/v1.0/powershell.exe -NoProfile -NonInteractive -Command Start-Sleep -Seconds 5"
	default:
		return "sleep 5"
	}
}
