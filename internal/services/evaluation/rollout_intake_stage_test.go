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
	"os"
	"strings"
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stagingOllamaModelCommandDispatcher struct {
	commands []string
}

func (d *stagingOllamaModelCommandDispatcher) DispatchOllamaModelCommand(_ context.Context, request OllamaModelCommandDispatchRequest) (*OllamaModelCommandDispatchResult, error) {
	d.commands = append(d.commands, request.Command)
	return &OllamaModelCommandDispatchResult{
		Status:  200,
		Success: true,
		CommandResult: &operatorv1.CommandResult{
			Status:     operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ReturnCode: 0,
		},
	}, nil
}

func TestStageRolloutIntakePullsAndAliases(t *testing.T) {
	t.Parallel()

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
	dispatcher := &stagingOllamaModelCommandDispatcher{}
	result, err := StageRolloutIntake(RolloutIntakeStageRequest{
		Context:            context.Background(),
		CatalogPath:        catalogPath,
		Dispatcher:         dispatcher,
		InferenceSessionID: "infer-session",
		NewID:              func(prefix string) string { return prefix },
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"huggingface.co/unsloth/Qwen3.8-27B-GGUF:UD-Q4_K_M"}, result.Pulled)
	assert.Equal(t, []string{"qwen3.8:27b"}, result.Aliased)
	require.Len(t, dispatcher.commands, 2)
	assert.True(t, strings.Contains(dispatcher.commands[0], "operator model pull"))
	assert.True(t, strings.Contains(dispatcher.commands[1], "operator model copy"))
}

func TestStageRolloutIntakeSkipsManualAndPendingEntries(t *testing.T) {
	t.Parallel()

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
		Context:            context.Background(),
		CatalogPath:        catalogPath,
		Dispatcher:         &stagingOllamaModelCommandDispatcher{},
		InferenceSessionID: "infer-session",
		NewID:              func(prefix string) string { return prefix },
	})
	require.NoError(t, err)
	require.Len(t, result.Skipped, 2)
	assert.Equal(t, "awaiting_single_file_gguf", result.Skipped[0].Reason)
	assert.Equal(t, "sharded GGUF", result.Skipped[1].Reason)
}

func TestStageFormationCatalogIntake_PullsMissingLibraryTag(t *testing.T) {
	t.Parallel()

	dispatcher := &stagingOllamaModelCommandDispatcher{}
	result, err := StageFormationCatalogIntake(RolloutIntakeStageRequest{
		Context:            context.Background(),
		Dispatcher:         dispatcher,
		InferenceSessionID: "infer-session",
		VariantIDs:         []string{"gemma2-2b"},
		NewID:              func(prefix string) string { return prefix },
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"gemma2:2b-instruct-q4_K_M"}, result.Pulled)
	require.Len(t, dispatcher.commands, 1)
	assert.True(t, strings.Contains(dispatcher.commands[0], "gemma2:2b-instruct-q4_K_M"))
}

func writeRolloutIntakeCatalog(t *testing.T, catalog RolloutIntakeCatalog) string {
	t.Helper()
	path := t.TempDir() + "/rollout-intake-hf.json"
	payload, err := json.Marshal(catalog)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, 0o644))
	return path
}
