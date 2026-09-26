// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestParseInferenceAcceptanceCases(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantLen int
		wantErr string
	}{
		{name: "empty", raw: "", wantLen: 0},
		{name: "single", raw: "basic_chat", wantLen: 1},
		{name: "comma separated", raw: "basic_chat, tool_use", wantLen: 2},
		{name: "no cases", raw: " , ", wantErr: "no cases selected"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseInferenceAcceptanceCases(test.raw)
			if test.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, test.wantLen)
		})
	}
}

func TestParseInferenceProbeRole(t *testing.T) {
	primary, err := parseInferenceProbeRole("primary")
	require.NoError(t, err)
	assert.Equal(t, models.InferenceModelRolePrimary, primary)

	lite, err := parseInferenceProbeRole("light")
	require.NoError(t, err)
	assert.Equal(t, models.InferenceModelRoleLite, lite)

	_, err = parseInferenceProbeRole("invalid")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceRoleInvalid)
}

func TestLoadRegistryFreezeFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "registry.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"campaign_id": "eval-init-qwen3-4b",
		"model_registry_digest": "digest-1",
		"variants": [{"model": "qwen3:4b", "digest": "abc"}]
	}`), 0o600))

	freeze, err := loadRegistryFreezeFile(path)
	require.NoError(t, err)
	require.NotNil(t, freeze)
	assert.Equal(t, "eval-init-qwen3-4b", freeze.CampaignID)
	assert.Equal(t, "digest-1", freeze.Digest)
	require.Len(t, freeze.Variants, 1)

	_, err = loadRegistryFreezeFile(filepath.Join(root, "missing.json"))
	require.Error(t, err)

	invalidPath := filepath.Join(root, "invalid.json")
	require.NoError(t, os.WriteFile(invalidPath, []byte(`{"campaign_id":""}`), 0o600))
	_, err = loadRegistryFreezeFile(invalidPath)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelRegistryInvalid)
}
