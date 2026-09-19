// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaBackend_ListProviderModelInventoryPreservesDistinctServedTags(t *testing.T) {
	t.Parallel()
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			require.NoError(t, json.NewEncoder(w).Encode(ollamaTagsResponse{Models: []ollamaTagModel{
				{Name: "alias-one:latest", Digest: "sha256:" + digestA},
				{Name: "alias-two:latest", Digest: "sha256:" + digestB},
			}}))
		case "/api/show":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var req struct {
				Name string `json:"name"`
			}
			require.NoError(t, json.Unmarshal(body, &req))
			show := ollamaShowResponse{
				Parameters: "num_ctx                        8192\n",
				Details: ollamaModelDetails{
					Format:            "gguf",
					Family:            "qwen3",
					ParameterSize:     "4.0B",
					QuantizationLevel: "Q4_K_M",
				},
				Capabilities: []string{"completion", "tools"},
			}
			if req.Name == "alias-two:latest" {
				show.Details.ParameterSize = "1.7B"
			}
			require.NoError(t, json.NewEncoder(w).Encode(show))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)
	entries, err := backend.ListProviderModelInventory(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "alias-one:latest", entries[0].ServedModelTag)
	assert.Equal(t, digestA, entries[0].ModelDigest)
	assert.Equal(t, uint64(4_000_000_000), entries[0].ParameterCount)
	assert.Equal(t, uint32(8192), entries[0].ContextLimit)
	assert.Equal(t, []string{"completion", "tools"}, entries[0].AdvertisedCapabilities)
	assert.Equal(t, "alias-two:latest", entries[1].ServedModelTag)
	assert.Equal(t, digestB, entries[1].ModelDigest)
}

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
