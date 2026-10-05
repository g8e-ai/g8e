// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeProviderModelVariantID(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "qwen3-4b", NormalizeProviderModelVariantID("qwen3:4b"))
	assert.Equal(t, "sam860-lfm2-700m", NormalizeProviderModelVariantID("sam860/LFM2:700m"))
}

func TestParseProviderParameterCount(t *testing.T) {
	t.Parallel()
	count, err := parseProviderParameterCount("4.0B")
	require.NoError(t, err)
	assert.Equal(t, uint64(4_000_000_000), count)

	count, err = parseProviderParameterCount("270M")
	require.NoError(t, err)
	assert.Equal(t, uint64(270_000_000), count)
}

func TestParseProviderContextLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		parameters string
		modelInfo  map[string]json.RawMessage
		want       uint32
	}{
		{name: "num_ctx parameter", parameters: "num_ctx                        4096\nstop \"<end>\"", want: 4096},
		{
			name:       "num_ctx parameter wins over model_info",
			parameters: "num_ctx 2048",
			modelInfo:  map[string]json.RawMessage{"gemma3.context_length": json.RawMessage("131072")},
			want:       2048,
		},
		{
			name:      "architecture-prefixed context_length",
			modelInfo: map[string]json.RawMessage{"gemma3.context_length": json.RawMessage("131072")},
			want:      131072,
		},
		{
			name:      "block_count is a layer count and never a context limit",
			modelInfo: map[string]json.RawMessage{"llama.block_count": json.RawMessage("32")},
			want:      0,
		},
		{name: "context_length out of uint32 range", modelInfo: map[string]json.RawMessage{"llama.context_length": json.RawMessage("4294967296")}, want: 0},
		{name: "no context information"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, parseProviderContextLimit(test.parameters, test.modelInfo))
		})
	}
}
