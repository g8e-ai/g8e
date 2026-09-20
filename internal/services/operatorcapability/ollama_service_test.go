// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/security"
)

func TestIsOllamaServiceCommand(t *testing.T) {
	assert.True(t, IsOllamaServiceCommand("ollama stop"))
	assert.True(t, IsOllamaServiceCommand("  OLLAMA SERVE  "))
	assert.True(t, IsOllamaServiceCommand("ollama ps"))
	assert.False(t, IsOllamaServiceCommand("ollama start"))
	assert.False(t, IsOllamaServiceCommand("ollama status"))
	assert.False(t, IsOllamaServiceCommand("curl http://127.0.0.1:11434/api/ps"))
}

func TestValidateOllamaServiceCommand(t *testing.T) {
	capable := &models.RuntimeConfig{
		ProviderBoundaryObserverEnabled:       true,
		ProviderBoundaryObserverOllamaEnabled: true,
	}
	require.NoError(t, ValidateOllamaServiceCommand(capable, "ollama stop"))

	incapable := &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true}
	err := ValidateOllamaServiceCommand(incapable, "ollama serve")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrProviderBoundaryObserverOllamaNotCapable)

	assert.NoError(t, ValidateOllamaServiceCommand(nil, "echo hello"))
}

func TestRestartOllamaCommands(t *testing.T) {
	assert.Equal(t, "cmd.exe /C %SystemRoot%/System32/WindowsPowerShell/v1.0/powershell.exe -NoProfile -NonInteractive -Command Start-Process -FilePath %LOCALAPPDATA%/Programs/Ollama/ollama.exe -ArgumentList serve -WindowStyle Hidden", OllamaWindowsStartCommand)
	assert.Equal(t, []string{
		OllamaWindowsKillCommand,
		"cmd.exe /C %SystemRoot%/System32/WindowsPowerShell/v1.0/powershell.exe -NoProfile -NonInteractive -Command Start-Sleep -Seconds 3",
		OllamaWindowsStartCommand,
		"cmd.exe /C %SystemRoot%/System32/WindowsPowerShell/v1.0/powershell.exe -NoProfile -NonInteractive -Command Start-Sleep -Seconds 8",
		OllamaServiceCommandPS,
	}, RestartOllamaCommands("windows"))
	assert.Equal(t, []string{
		RestartOllamaDaemonCommand("linux"),
		"sleep 8",
		OllamaServiceCommandPS,
	}, RestartOllamaCommands("linux"))
}

func TestRestartSettleCommand(t *testing.T) {
	windowsSettle := RestartSettleCommand("windows")
	assert.Equal(t, "cmd.exe /C %SystemRoot%/System32/WindowsPowerShell/v1.0/powershell.exe -NoProfile -NonInteractive -Command Start-Sleep -Seconds 5", windowsSettle)
	assert.False(t, security.IsShellRequired(windowsSettle), "windows settle must run without a POSIX shell")
	assert.False(t, security.IsShellRequired(OllamaWindowsKillCommand), "windows kill must run without a POSIX shell")

	assert.Equal(t, "sleep 5", RestartSettleCommand("linux"))
}

func TestToleratedOllamaRestartExitCode(t *testing.T) {
	assert.True(t, ToleratedOllamaRestartExitCode(OllamaWindowsKillCommand, 0))
	assert.True(t, ToleratedOllamaRestartExitCode(OllamaWindowsKillCommand, 128))
	assert.False(t, ToleratedOllamaRestartExitCode(OllamaWindowsKillCommand, 1))

	assert.True(t, ToleratedOllamaRestartExitCode(OllamaWindowsStartCommand, 0))
	assert.True(t, ToleratedOllamaRestartExitCode(OllamaWindowsStartCommand, 1))
	assert.False(t, ToleratedOllamaRestartExitCode(OllamaWindowsStartCommand, 2))
}
