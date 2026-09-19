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

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func TestQueueEvalNext_PrintsPendingEntry(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{ServedModelTag: "gemma3:4b", VariantID: "gemma3-4b", CampaignID: "eval-init-gemma3-4b", Status: "verified"},
			{ServedModelTag: "deepseek-r1:7b", VariantID: "deepseek-r1-7b", CampaignID: "eval-init-deepseek-r1-7b", Status: "pending", InventoryFile: ".g8e/eval/inventories/eval-init-deepseek-r1-7b.json", ModelRegistryDigest: "digest", HomogeneousCellCount: 75},
		},
	}
	queuePath := filepath.Join(root, evaluation.DefaultInitCampaignQueueRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(queuePath), 0o755))
	body, err := json.Marshal(queue)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(queuePath, body, 0o600))

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
