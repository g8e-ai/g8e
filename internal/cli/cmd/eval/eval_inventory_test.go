// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func writeTestModelInventory(t *testing.T, root string, variants []*evalv1.ModelVariant) string {
	t.Helper()
	freeze, err := evaluation.MaterializeModelRegistry("eval-test-campaign", variants)
	require.NoError(t, err)

	inventoryDir := filepath.Join(root, constants.RuntimeDirname, "eval")
	require.NoError(t, os.MkdirAll(inventoryDir, 0o755))
	path := filepath.Join(inventoryDir, "model-inventory.json")

	rawVariants := make([]json.RawMessage, 0, len(freeze.Variants))
	for _, v := range freeze.Variants {
		body, err := protojson.Marshal(v)
		require.NoError(t, err)
		rawVariants = append(rawVariants, body)
	}

	payload, err := json.MarshalIndent(struct {
		CampaignID          string            `json:"campaign_id"`
		ModelRegistryDigest string            `json:"model_registry_digest"`
		HomogeneousCellCount uint64           `json:"homogeneous_cell_count"`
		ModelCount          int               `json:"model_count"`
		Variants            []json.RawMessage `json:"variants"`
	}{
		CampaignID:          freeze.CampaignID,
		ModelRegistryDigest: freeze.RegistryDigest,
		HomogeneousCellCount: freeze.HomogeneousCellCount,
		ModelCount:          len(rawVariants),
		Variants:            rawVariants,
	}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, 0o644))
	return path
}

func testDeps(root string) nativeEvalDeps {
	return nativeEvalDeps{
		configLoader: func(string) (*config.Config, error) { return &config.Config{ProjectRoot: root}, nil },
		fileSvcFactory: func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			return fs.NewRuntimeFileService(root, slog.Default())
		},
		createRuntimeTree: func(context.Context, fs.RuntimeFileService) error { return nil },
	}
}

func TestModelsList_FiltersByMaxParametersAndDetailed(t *testing.T) {
	root := t.TempDir()
	variants := []*evalv1.ModelVariant{
		{
			VariantId:      "gemma3-1b",
			ProviderClass:  "ollama",
			ServedModelTag: "gemma3:1b",
			ModelDigest:    "1111111111111111111111111111111111111111111111111111111111111111",
			ModelFamily:    "gemma3",
			ParameterCount: 1_000_000_000,
		},
		{
			VariantId:      "granite4-2-8b",
			ProviderClass:  "ollama",
			ServedModelTag: "granite4.2:8b",
			ModelDigest:    "2222222222222222222222222222222222222222222222222222222222222222",
			ModelFamily:    "granite",
			ParameterCount: 8_000_000_000,
		},
		{
			VariantId:      "qwen3-35b",
			ProviderClass:  "ollama",
			ServedModelTag: "qwen3:35b",
			ModelDigest:    "3333333333333333333333333333333333333333333333333333333333333333",
			ModelFamily:    "qwen3",
			ParameterCount: 35_000_000_000,
		},
	}
	writeTestModelInventory(t, root, variants)

	deps := testDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"models", "list", "--max-parameters", "12b", "-d", "--project-root", root})
	require.NoError(t, command.Execute())

	outStr := output.String()
	assert.Contains(t, outStr, "gemma3:1b")
	assert.Contains(t, outStr, "granite4.2:8b")
	assert.NotContains(t, outStr, "qwen3:35b")
	assert.Contains(t, outStr, "1B")
	assert.Contains(t, outStr, "8B")
}

func TestModelsAdd_AddsNewModelVariant(t *testing.T) {
	root := t.TempDir()
	variants := []*evalv1.ModelVariant{
		{
			VariantId:      "gemma3-1b",
			ProviderClass:  "ollama",
			ServedModelTag: "gemma3:1b",
			ModelDigest:    "1111111111111111111111111111111111111111111111111111111111111111",
			ModelFamily:    "gemma3",
			ParameterCount: 1_000_000_000,
		},
	}
	writeTestModelInventory(t, root, variants)

	deps := testDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"models", "add",
		"--tag", "gemma4:12b",
		"--params", "12b",
		"--family", "gemma4",
		"--project-root", root,
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Added gemma4:12b")

	// Verify it can be listed and parsed correctly
	output.Reset()
	listCmd := evalCmdWithConfig(deps)
	listCmd.SetOut(&output)
	listCmd.SetArgs([]string{"models", "list", "-d", "--project-root", root})
	require.NoError(t, listCmd.Execute())
	assert.Contains(t, output.String(), "gemma4:12b")
	assert.Contains(t, output.String(), "12B")
	assert.Contains(t, output.String(), "gemma4")
}

func TestModelsImport_ImportsFromBaseInventory(t *testing.T) {
	root := t.TempDir()
	baseDir := filepath.Join(root, "eval")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	basePath := filepath.Join(baseDir, "base-model-inventory.json")

	baseVariants := []*evalv1.ModelVariant{
		{
			VariantId:      "granite4-2-3b",
			ProviderClass:  "ollama",
			ServedModelTag: "granite4.2:3b",
			ModelDigest:    "4444444444444444444444444444444444444444444444444444444444444444",
			ModelFamily:    "granite",
			ParameterCount: 3_000_000_000,
		},
		{
			VariantId:      "granite4-2-8b",
			ProviderClass:  "ollama",
			ServedModelTag: "granite4.2:8b",
			ModelDigest:    "5555555555555555555555555555555555555555555555555555555555555555",
			ModelFamily:    "granite",
			ParameterCount: 8_000_000_000,
		},
	}
	baseFreeze, err := evaluation.MaterializeModelRegistry("eval-base", baseVariants)
	require.NoError(t, err)
	rawVariants := make([]json.RawMessage, 0, len(baseFreeze.Variants))
	for _, v := range baseFreeze.Variants {
		body, err := protojson.Marshal(v)
		require.NoError(t, err)
		rawVariants = append(rawVariants, body)
	}
	payload, err := json.Marshal(struct {
		CampaignID          string            `json:"campaign_id"`
		ModelRegistryDigest string            `json:"model_registry_digest"`
		Variants            []json.RawMessage `json:"variants"`
	}{
		CampaignID:          baseFreeze.CampaignID,
		ModelRegistryDigest: baseFreeze.RegistryDigest,
		Variants:            rawVariants,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(basePath, payload, 0o644))

	// Initial runtime inventory with one model
	writeTestModelInventory(t, root, []*evalv1.ModelVariant{
		{
			VariantId:      "gemma3-1b",
			ProviderClass:  "ollama",
			ServedModelTag: "gemma3:1b",
			ModelDigest:    "1111111111111111111111111111111111111111111111111111111111111111",
			ModelFamily:    "gemma3",
			ParameterCount: 1_000_000_000,
		},
	})

	deps := testDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"models", "import",
		"--from", basePath,
		"--all",
		"--project-root", root,
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Imported 2 model variants")
}

func TestRolloutRun_MaxParametersFilter(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{
				ServedModelTag:       "gemma3:1b",
				VariantID:            "gemma3-1b",
				CampaignID:           "eval-init-gemma3-1b",
				Status:               "pending",
				InventoryFile:        "eval/inventories/eval-init-gemma3-1b.json",
				ModelRegistryDigest:  "digest1",
				HomogeneousCellCount: 75,
			},
			{
				ServedModelTag:       "granite4.2:8b",
				VariantID:            "granite4-2-8b",
				CampaignID:           "eval-init-granite4-2-8b",
				Status:               "pending",
				InventoryFile:        "eval/inventories/eval-init-granite4-2-8b.json",
				ModelRegistryDigest:  "digest2",
				HomogeneousCellCount: 75,
			},
			{
				ServedModelTag:       "glm-4.7-flash",
				VariantID:            "glm-4-7-flash",
				CampaignID:           "eval-init-glm-4-7-flash",
				Status:               "pending",
				InventoryFile:        "eval/inventories/eval-init-glm-4-7-flash.json",
				ModelRegistryDigest:  "digest3",
				HomogeneousCellCount: 75,
			},
		},
	}
	writeTestRuntimeQueue(t, root, &queue)

	// Runtime inventory defining the parameter counts
	writeTestModelInventory(t, root, []*evalv1.ModelVariant{
		{
			VariantId:      "gemma3-1b",
			ProviderClass:  "ollama",
			ServedModelTag: "gemma3:1b",
			ModelDigest:    "1111111111111111111111111111111111111111111111111111111111111111",
			ModelFamily:    "gemma3",
			ParameterCount: 1_000_000_000,
		},
		{
			VariantId:      "granite4-2-8b",
			ProviderClass:  "ollama",
			ServedModelTag: "granite4.2:8b",
			ModelDigest:    "2222222222222222222222222222222222222222222222222222222222222222",
			ModelFamily:    "granite",
			ParameterCount: 8_000_000_000,
		},
		{
			VariantId:      "glm-4-7-flash",
			ProviderClass:  "ollama",
			ServedModelTag: "glm-4.7-flash",
			ModelDigest:    "3333333333333333333333333333333333333333333333333333333333333333",
			ModelFamily:    "glm4",
			ParameterCount: 30_000_000_000,
		},
	})

	deps := testDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rollout", "run", "--dry-run", "--max-parameters", "12b", "--project-root", root})
	require.NoError(t, command.Execute())

	outStr := output.String()
	assert.Contains(t, outStr, "gemma3:1b")
	assert.Contains(t, outStr, "granite4.2:8b")
	assert.NotContains(t, outStr, "glm-4.7-flash")
}

func TestModelsRemove_RemovesVariantAndUpdatesDigest(t *testing.T) {
	root := t.TempDir()
	writeTestModelInventory(t, root, []*evalv1.ModelVariant{
		{
			VariantId:      "gemma3-1b",
			ProviderClass:  "ollama",
			ServedModelTag: "gemma3:1b",
			ModelDigest:    "1111111111111111111111111111111111111111111111111111111111111111",
			ModelFamily:    "gemma3",
			ParameterCount: 1_000_000_000,
		},
		{
			VariantId:      "granite4-2-8b",
			ProviderClass:  "ollama",
			ServedModelTag: "granite4.2:8b",
			ModelDigest:    "2222222222222222222222222222222222222222222222222222222222222222",
			ModelFamily:    "granite",
			ParameterCount: 8_000_000_000,
		},
	})

	deps := testDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"models", "remove", "gemma3:1b",
		"--project-root", root,
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Removed gemma3:1b (gemma3-1b)")
	assert.Contains(t, output.String(), "models=1")

	// Verify the remaining inventory
	listCmd := evalCmdWithConfig(deps)
	var listOut bytes.Buffer
	listCmd.SetOut(&listOut)
	listCmd.SetArgs([]string{
		"models", "list",
		"--project-root", root,
	})
	require.NoError(t, listCmd.Execute())
	assert.NotContains(t, listOut.String(), "gemma3:1b")
	assert.Contains(t, listOut.String(), "granite4.2:8b")
}

