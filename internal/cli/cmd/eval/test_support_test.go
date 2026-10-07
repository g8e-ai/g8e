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

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func writeTestFrozenInventory(t *testing.T, root string, variants ...*evalv1.ModelVariant) {
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
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	require.NoError(t, fileSvc.WriteFile(context.Background(), evaluation.DefaultModelInventoryRelPath, payload, constants.PermFileReadOnly))
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
		HomogeneousCellCount: 41,
		Variants:             []*evalv1.ModelVariant{{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "digest"}},
	}
	require.NoError(t, writeModelInventoryFreezeFile(path, freeze))
	assert.FileExists(t, path)
	payload, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(payload), "eval-init-qwen3-4b")
}

func assertTestRuntimeFileExists(t *testing.T, root string, relPath string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	assert.FileExists(t, path)
}

// fakeBindClient stands in for the enrollment client: it reports the bound
// sessions it holds and records every bind call, replacing the held sessions
// with the requested ones.
type fakeBindClient struct {
	bound   []string
	binds   [][]string
	infoErr error
	bindErr error
}

func (f *fakeBindClient) SessionInfo(context.Context, fs.RuntimeFileService) (auth.CLISessionInfo, error) {
	if f.infoErr != nil {
		return auth.CLISessionInfo{}, f.infoErr
	}
	info := auth.CLISessionInfo{CLISessionID: "cli-1", UserID: "user-1", BoundOperatorSessionIDs: f.bound}
	if len(f.bound) > 0 {
		info.OperatorSessionID = f.bound[0]
	}
	return info, nil
}

func (f *fakeBindClient) Bind(_ context.Context, _ fs.RuntimeFileService, operatorSessionIDs []string) (auth.CLISessionBind, error) {
	if f.bindErr != nil {
		return auth.CLISessionBind{}, f.bindErr
	}
	f.binds = append(f.binds, operatorSessionIDs)
	f.bound = operatorSessionIDs
	bound := make([]models.CLIBoundOperator, 0, len(operatorSessionIDs))
	for _, sessionID := range operatorSessionIDs {
		bound = append(bound, models.CLIBoundOperator{OperatorSessionID: sessionID, OperatorID: "operator-of-" + sessionID})
	}
	return auth.CLISessionBind{
		CLISessionID:      "cli-bound",
		UserID:            "user-1",
		OperatorSessionID: operatorSessionIDs[0],
		OperatorID:        "operator-of-" + operatorSessionIDs[0],
		Bound:             bound,
	}, nil
}

func campaignOrchestrateOperators() []*operatorv1.OperatorDocument {
	return []*operatorv1.OperatorDocument{
		{
			Id:                "infer-op",
			OperatorSessionId: "infer-session",
			Status:            string(constants.OperatorStatusActive),
			OperatorType:      string(constants.OperatorTypeRemote),
			RuntimeConfig: &operatorv1.OperatorRuntimeConfig{
				InferenceEnabled:        true,
				InferenceOllamaEndpoint: "http://provider.example:11434",
			},
		},
		{
			Id:                "data-op",
			OperatorSessionId: "data-session",
			CurrentHostname:   constants.DataOperatorHostname,
			Status:            string(constants.OperatorStatusActive),
			OperatorType:      string(constants.OperatorTypeRemote),
			RuntimeConfig:     &operatorv1.OperatorRuntimeConfig{InferenceEnabled: false},
		},
	}
}
