// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

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
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestQueueEvalRunDryRun(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{VariantID: "granite3-3-2b", ServedModelTag: "granite3.3:2b", Status: "verified"},
			{VariantID: "qwen3-4b", ServedModelTag: "qwen3:4b", Status: "pending"},
		},
	}
	queuePath := filepath.Join(root, evaluation.DefaultInitCampaignQueueRelPath)
	require.NoError(t, evaluation.SaveInitCampaignQueue(queuePath, &queue))

	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rollout", "run", "--project-root", root, "--dry-run", "--skip-variant", "granite3-3-2b"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "qwen3-4b")
	assert.NotContains(t, output.String(), "granite3-3-2b")
}

func TestQueueEvalInitMaterialize(t *testing.T) {
	root := t.TempDir()
	writeTestFrozenInventory(t, root, evaluation.DefaultModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "digest", ProviderClass: "ollama"},
	)

	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rollout", "init", "--project-root", root, "--materialize"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), ".g8e/eval/init-campaign-queue.json")
	assert.FileExists(t, filepath.Join(root, evaluation.DefaultInitCampaignQueueRelPath))
	assert.FileExists(t, filepath.Join(root, ".g8e/eval/inventories/eval-init-qwen3-4b.json"))
}

func TestQueueEvalMarkVerified(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{VariantID: "qwen3-4b", ServedModelTag: "qwen3:4b", Status: "pending"},
		},
	}
	queuePath := filepath.Join(root, evaluation.DefaultInitCampaignQueueRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(queuePath), 0o755))
	body, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(queuePath, body, 0o600))

	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"rollout", "mark",
		"--project-root", root,
		"--tag", "qwen3:4b",
		"--status", "verified",
		"--run-id", "eval-init-qwen3-4b-123",
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "status=verified")

	loaded, err := evaluation.LoadInitCampaignQueue(queuePath)
	require.NoError(t, err)
	assert.Equal(t, "verified", loaded.Models[0].Status)
	assert.Equal(t, "eval-init-qwen3-4b-123", loaded.Models[0].VerifiedRunID)
}

func TestInventoryEvalMaterializeTag(t *testing.T) {
	root := t.TempDir()
	writeTestFrozenInventory(t, root, evaluation.DefaultModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "digest", ProviderClass: "ollama"},
	)

	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"models", "materialize", "--project-root", root, "--tag", "qwen3:4b"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "eval-init-qwen3-4b")
	assert.FileExists(t, filepath.Join(root, ".g8e/eval/inventories/eval-init-qwen3-4b.json"))
}

func writeTestFrozenInventory(t *testing.T, root, relPath string, variants ...*evalv1.ModelVariant) {
	t.Helper()
	bodies := make([]json.RawMessage, 0, len(variants))
	for _, variant := range variants {
		raw, err := protojson.Marshal(variant)
		require.NoError(t, err)
		bodies = append(bodies, raw)
	}
	path := filepath.Join(root, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	payload, err := json.Marshal(map[string]any{"variants": bodies})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, 0o600))
}

func testNativeEvalDeps(root string) nativeEvalDeps {
	return nativeEvalDeps{
		configLoader: func(string) (*config.Config, error) { return &config.Config{ProjectRoot: root}, nil },
		fileSvcFactory: func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			return fs.NewRuntimeFileService(root, slog.Default())
		},
		createRuntimeTree: func(context.Context, fs.RuntimeFileService) error { return nil },
	}
}
