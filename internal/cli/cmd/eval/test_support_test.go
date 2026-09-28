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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func writeTestFrozenInventory(t *testing.T, root, relPath string, variants ...*evalv1.ModelVariant) {
	t.Helper()
	bodies := make([]json.RawMessage, 0, len(variants))
	for _, variant := range variants {
		raw, err := protojson.Marshal(variant)
		require.NoError(t, err)
		bodies = append(bodies, raw)
	}
	payload, err := json.Marshal(struct {
		Variants []json.RawMessage `json:"variants"`
	}{Variants: bodies})
	require.NoError(t, err)
	if relPath == evaluation.DefaultModelInventoryRelPath {
		fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
		require.NoError(t, fileSvc.WriteFile(context.Background(), relPath, payload, constants.PermFileReadOnly))
		return
	}
	path := filepath.Join(root, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(path, payload, constants.PermFileReadOnly))
}

func writeTestRuntimeQueue(t *testing.T, root string, queue *evaluation.CampaignQueue) fs.RuntimeFileService {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	require.NoError(t, evaluation.SaveInitCampaignQueueToRuntime(context.Background(), fileSvc, evaluation.DefaultInitCampaignQueueRelPath, queue))
	return fileSvc
}

// testNativeEvalDeps is the minimal dependency set for commands that only read
// and write project files.
func testNativeEvalDeps(root string) nativeEvalDeps {
	return nativeEvalDeps{
		configLoader: func(string) (*config.Config, error) { return &config.Config{ProjectRoot: root}, nil },
		fileSvcFactory: func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			return fs.NewRuntimeFileService(root, slog.Default())
		},
		createRuntimeTree: func(context.Context, fs.RuntimeFileService) error { return nil },
		runControl:        testRunControl(newFakeProcessControl()),
	}
}

func TestModelInventoryFreezeJSON_RejectsNilFreeze(t *testing.T) {
	_, err := modelInventoryFreezeJSON(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestWriteModelInventoryFreezeFile_WritesJSON(t *testing.T) {
	path := filepath.Join(testutil.TempDir(t), "inventory-freeze.json")
	freeze := &evaluation.ModelInventoryFreeze{
		CampaignID:           "eval-init-qwen3-4b",
		RegistryDigest:       "digest-1",
		HomogeneousCellCount: 75,
		Variants:             []*evalv1.ModelVariant{{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "digest"}},
	}
	require.NoError(t, writeModelInventoryFreezeFile(path, freeze))
	assert.FileExists(t, path)
	payload, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(payload), "eval-init-qwen3-4b")
}

func TestCheckHTTPReachable_AcceptsHealthyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	require.NoError(t, checkHTTPReachable(context.Background(), server.URL))
}

func TestCheckHTTPReachable_RejectsMissingURLAndNon2xx(t *testing.T) {
	require.Error(t, checkHTTPReachable(context.Background(), ""))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	require.Error(t, checkHTTPReachable(context.Background(), server.URL))
}

func assertTestRuntimeFileExists(t *testing.T, root string, relPath string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	assert.FileExists(t, path)
}

func campaignOrchestrateOperators() []models.OperatorDocumentGo {
	return []models.OperatorDocumentGo{
		{
			ID:                "infer-op",
			OperatorSessionID: "infer-session",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig: &models.RuntimeConfig{
				InferenceEnabled:        true,
				InferenceOllamaEndpoint: "http://provider.example:11434",
			},
		},
		{
			ID:                "data-op",
			OperatorSessionID: "data-session",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: false},
		},
	}
}
