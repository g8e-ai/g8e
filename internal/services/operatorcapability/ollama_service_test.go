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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsOllamaServiceCommand(t *testing.T) {
	assert.True(t, IsOllamaServiceCommand("ollama stop"))
	assert.True(t, IsOllamaServiceCommand("  OLLAMA START  "))
	assert.True(t, IsOllamaServiceCommand("ollama status"))
	assert.False(t, IsOllamaServiceCommand("ollama ps"))
	assert.False(t, IsOllamaServiceCommand("curl http://127.0.0.1:11434/api/ps"))
}

func TestValidateOllamaServiceCommand(t *testing.T) {
	capable := &models.RuntimeConfig{
		ProviderBoundaryObserverEnabled:      true,
		ProviderBoundaryObserverOllamaEnabled: true,
	}
	require.NoError(t, ValidateOllamaServiceCommand(capable, "ollama stop"))

	incapable := &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true}
	err := ValidateOllamaServiceCommand(incapable, "ollama start")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrProviderBoundaryObserverOllamaNotCapable)

	assert.NoError(t, ValidateOllamaServiceCommand(nil, "echo hello"))
}

func TestRestartSettleCommand(t *testing.T) {
	assert.Equal(t, "ping 127.0.0.1 -n 6 > nul", RestartSettleCommand("windows"))
	assert.Equal(t, "sleep 5", RestartSettleCommand("linux"))
}
