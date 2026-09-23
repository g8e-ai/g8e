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
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func writeTestRuntimeQueue(t *testing.T, root string, queue *evaluation.CampaignQueue) fs.RuntimeFileService {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	require.NoError(t, evaluation.SaveInitCampaignQueueToRuntime(context.Background(), fileSvc, evaluation.DefaultInitCampaignQueueRelPath, queue))
	return fileSvc
}

func assertTestRuntimeFileExists(t *testing.T, root, relPath string) {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	exists, err := fileSvc.FileExists(context.Background(), relPath)
	require.NoError(t, err)
	assert.True(t, exists, relPath)
}

func TestQueueEvalNext_PrintsPendingEntry(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{ServedModelTag: "gemma3:4b", VariantID: "gemma3-4b", CampaignID: "eval-init-gemma3-4b", Status: "verified"},
			{ServedModelTag: "deepseek-r1:7b", VariantID: "deepseek-r1-7b", CampaignID: "eval-init-deepseek-r1-7b", Status: "pending", InventoryFile: "eval/inventories/eval-init-deepseek-r1-7b.json", ModelRegistryDigest: "digest", HomogeneousCellCount: 75},
		},
	}
	writeTestRuntimeQueue(t, root, &queue)

	deps := nativeEvalDeps{
		configLoader: func(string) (*config.Config, error) { return &config.Config{ProjectRoot: root}, nil },
		fileSvcFactory: func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			return fs.NewRuntimeFileService(root, slog.Default())
		},
		createRuntimeTree: func(context.Context, fs.RuntimeFileService) error { return nil },
	}
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rollout", "next", "--project-root", root})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "deepseek-r1:7b")
	assert.Contains(t, output.String(), "campaign start --queue next")
}
