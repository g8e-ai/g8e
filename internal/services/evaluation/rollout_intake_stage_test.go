// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStageRolloutIntakePullsAndAliases(t *testing.T) {
	t.Parallel()

	var pullModel string
	var copySource string
	var copyDestination string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pull":
			var payload struct {
				Model string `json:"model"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			pullModel = payload.Model
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte(`{"status":"success"}`))
		case "/api/copy":
			var payload struct {
				Source      string `json:"source"`
				Destination string `json:"destination"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			copySource = payload.Source
			copyDestination = payload.Destination
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	catalogPath := writeRolloutIntakeCatalog(t, RolloutIntakeCatalog{
		Models: []RolloutIntakeModel{{
			VariantID:      "qwen3-8-27b",
			ServedModelTag: "qwen3.8:27b",
			Staging: RolloutIntakeStaging{
				Method:     "ollama_hf_pull",
				OllamaPull: "huggingface.co/unsloth/Qwen3.8-27B-GGUF:UD-Q4_K_M",
				Alias:      "qwen3.8:27b",
			},
		}},
	})

	result, err := StageRolloutIntake(RolloutIntakeStageRequest{
		Context:        context.Background(),
		CatalogPath:    catalogPath,
		OllamaEndpoint: server.URL,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"huggingface.co/unsloth/Qwen3.8-27B-GGUF:UD-Q4_K_M"}, result.Pulled)
	assert.Equal(t, []string{"qwen3.8:27b"}, result.Aliased)
	assert.Equal(t, "huggingface.co/unsloth/Qwen3.8-27B-GGUF:UD-Q4_K_M", pullModel)
	assert.Equal(t, "huggingface.co/unsloth/Qwen3.8-27B-GGUF:UD-Q4_K_M", copySource)
	assert.Equal(t, "qwen3.8:27b", copyDestination)
}

func TestStageRolloutIntakeSkipsManualAndPendingEntries(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s", r.URL.Path)
	}))
	t.Cleanup(server.Close)

	catalogPath := writeRolloutIntakeCatalog(t, RolloutIntakeCatalog{
		Models: []RolloutIntakeModel{
			{
				VariantID:      "glm-5-3-air",
				ServedModelTag: "glm-5.3-air",
				Staging:        RolloutIntakeStaging{Method: "pending", Status: "awaiting_single_file_gguf"},
			},
			{
				VariantID:      "glm-5-3-flash",
				ServedModelTag: "glm-5.3-flash",
				Staging: RolloutIntakeStaging{
					Method:          "manual_create",
					OllamaPullError: "sharded GGUF",
				},
			},
		},
	})

	result, err := StageRolloutIntake(RolloutIntakeStageRequest{
		Context:        context.Background(),
		CatalogPath:    catalogPath,
		OllamaEndpoint: server.URL,
	})
	require.NoError(t, err)
	require.Len(t, result.Skipped, 2)
	assert.Equal(t, "awaiting_single_file_gguf", result.Skipped[0].Reason)
	assert.Equal(t, "sharded GGUF", result.Skipped[1].Reason)
}

func TestStageFormationCatalogIntake_PullsMissingLibraryTag(t *testing.T) {
	t.Parallel()

	pulled := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pull":
			var payload struct {
				Model string `json:"model"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			pulled = append(pulled, payload.Model)
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte(`{"status":"success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	result, err := StageFormationCatalogIntake(RolloutIntakeStageRequest{
		Context:        context.Background(),
		OllamaEndpoint: server.URL,
		VariantIDs:     []string{"gemma2-2b"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"gemma2:2b-instruct-q4_K_M"}, result.Pulled)
	assert.Equal(t, []string{"gemma2:2b-instruct-q4_K_M"}, pulled)
}

func writeRolloutIntakeCatalog(t *testing.T, catalog RolloutIntakeCatalog) string {
	t.Helper()
	path := t.TempDir() + "/rollout-intake-hf.json"
	payload, err := json.Marshal(catalog)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, 0o644))
	return path
}
