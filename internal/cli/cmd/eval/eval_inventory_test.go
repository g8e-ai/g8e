// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func inventoryPayload(t *testing.T, campaignID string, variants []*evalv1.ModelVariant) []byte {
	t.Helper()
	freeze, err := evaluation.MaterializeModelRegistry(campaignID, variants)
	require.NoError(t, err)

	rawVariants := make([]json.RawMessage, 0, len(freeze.Variants))
	for _, v := range freeze.Variants {
		body, err := protojson.Marshal(v)
		require.NoError(t, err)
		rawVariants = append(rawVariants, body)
	}
	payload, err := json.MarshalIndent(struct {
		CampaignID           string            `json:"campaign_id"`
		ModelRegistryDigest  string            `json:"model_registry_digest"`
		HomogeneousCellCount uint64            `json:"homogeneous_cell_count"`
		ModelCount           int               `json:"model_count"`
		Variants             []json.RawMessage `json:"variants"`
	}{
		CampaignID:           freeze.CampaignID,
		ModelRegistryDigest:  freeze.RegistryDigest,
		HomogeneousCellCount: freeze.HomogeneousCellCount,
		ModelCount:           len(rawVariants),
		Variants:             rawVariants,
	}, "", "  ")
	require.NoError(t, err)
	return payload
}

// writeTestModelInventory writes the runtime model registry.
func writeTestModelInventory(t *testing.T, root string, variants []*evalv1.ModelVariant) string {
	t.Helper()
	inventoryDir := filepath.Join(root, constants.RuntimeDirname, "eval")
	require.NoError(t, os.MkdirAll(inventoryDir, 0o755))
	path := filepath.Join(inventoryDir, "model-inventory.json")
	require.NoError(t, os.WriteFile(path, inventoryPayload(t, "eval-test-campaign", variants), 0o644))
	return path
}

// writeTestCatalogInventory writes the checked-in model catalog.
func writeTestCatalogInventory(t *testing.T, root string, variants []*evalv1.ModelVariant) string {
	t.Helper()
	baseDir := filepath.Join(root, "eval")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	path := filepath.Join(baseDir, "base-model-inventory.json")
	require.NoError(t, os.WriteFile(path, inventoryPayload(t, "eval-base", variants), 0o644))
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
