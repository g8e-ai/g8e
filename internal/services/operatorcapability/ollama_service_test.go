// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, []string{
		OllamaServiceCommandStop,
		"cmd.exe /C timeout /t 5 /nobreak",
		OllamaServiceCommandPS,
	}, RestartOllamaCommands("windows"))
	assert.Equal(t, []string{
		OllamaServiceCommandStop,
		"sleep 5",
		OllamaServiceCommandPS,
	}, RestartOllamaCommands("linux"))
}

func TestRestartSettleCommand(t *testing.T) {
	windowsSettle := RestartSettleCommand("windows")
	assert.Equal(t, "cmd.exe /C timeout /t 5 /nobreak", windowsSettle)
	assert.False(t, security.IsShellRequired(windowsSettle), "windows settle must run without a POSIX shell")

	assert.Equal(t, "sleep 5", RestartSettleCommand("linux"))
}
