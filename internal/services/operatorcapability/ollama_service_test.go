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
)

func TestOllamaStopCommand_ValidatesServedModelTag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tag  string
		want string
		ok   bool
	}{
		{name: "namespaced tag", tag: "registry.example/team/qwen3:0.6b", want: "ollama stop registry.example/team/qwen3:0.6b", ok: true},
		{name: "empty tag", tag: "", ok: false},
		{name: "option-like tag", tag: "--all", ok: false},
		{name: "shell metacharacter", tag: "qwen3:0.6b;whoami", ok: false},
		{name: "whitespace", tag: "qwen3 0.6b", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, err := OllamaStopCommand(tt.tag)
			if tt.ok {
				require.NoError(t, err)
				assert.Equal(t, tt.want, command)
				return
			}
			require.ErrorIs(t, err, constants.ErrInferenceModelTagInvalid)
		})
	}
}

func TestValidateWitnessCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  *models.RuntimeConfig
		want bool
	}{
		{name: "ordinary operator is allowed", cfg: &models.RuntimeConfig{}, want: true},
		{name: "provider observer is rejected", cfg: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true}, want: false},
		{name: "provenance operator is rejected", cfg: &models.RuntimeConfig{ProvenanceOperatorEnabled: true}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWitnessCommand(tt.cfg, "ollama ps")
			if tt.want {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, constants.ErrWitnessCommandNotCapable)
		})
	}
}
