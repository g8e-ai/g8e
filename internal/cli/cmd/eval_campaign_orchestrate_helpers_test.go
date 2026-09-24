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
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestWriteCampaignStartPlan_TextAndJSON(t *testing.T) {
	plan := &evaluation.CampaignStartPlan{
		CampaignID:           "eval-init-qwen3-4b",
		RunID:                "run-123",
		InventoryPath:        filepath.Join(constants.EvaluationDirname, constants.EvaluationInventoriesDirname, "eval-init-qwen3-4b.json"),
		ModelTags:            []string{"qwen3:4b"},
		RegistryDigest:       "digest-1",
		HomogeneousCellCount: 75,
	}
	sessions := campaignOperatorSessions{
		InferenceSessionID: "inference-session-1",
		DataSessionID:      "data-session-1",
	}

	var text bytes.Buffer
	require.NoError(t, writeCampaignStartPlan(&text, plan, sessions, false))
	textOut := text.String()
	assert.Contains(t, textOut, "eval-init-qwen3-4b")
	assert.Contains(t, textOut, "run-123")
	assert.Contains(t, textOut, "inference-session-1")
	assert.Contains(t, textOut, "data-session-1")

	var jsonOut bytes.Buffer
	require.NoError(t, writeCampaignStartPlan(&jsonOut, plan, sessions, true))
	var payload campaignStartPlanJSON
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &payload))
	assert.Equal(t, "eval-init-qwen3-4b", payload.CampaignID)
	assert.Equal(t, "run-123", payload.RunID)
	assert.Equal(t, "inference-session-1", payload.InferenceSession)
}

func TestPersistActiveCampaignRun_WritesMarker(t *testing.T) {
	root := t.TempDir()
	startedAt := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	plan := &evaluation.CampaignStartPlan{
		RunID:         "run-abc",
		CampaignID:    "eval-init-gemma3-4b",
		InventoryPath: filepath.Join(constants.EvaluationDirname, constants.EvaluationInventoriesDirname, "eval-init-gemma3-4b.json"),
		ModelTags:     []string{"gemma3:4b"},
	}
	fileSvc, err := fs.NewRuntimeFileService(root, nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	require.NoError(t, persistActiveCampaignRunWithFileService(context.Background(), fileSvc, plan, startedAt))

	loaded, err := evaluation.LoadActiveCampaignRunFromRuntime(context.Background(), fileSvc)
	require.NoError(t, err)
	assert.Equal(t, plan.RunID, loaded.RunID)
	assert.Equal(t, plan.CampaignID, loaded.CampaignID)
	assert.Equal(t, plan.InventoryPath, loaded.InventoryFile)
	assert.Equal(t, plan.ModelTags, loaded.ModelTags)
	assert.Equal(t, startedAt, loaded.StartedAt)
}

func TestResolveCampaignRunID(t *testing.T) {
	root := t.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(root, nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	require.NoError(t, evaluation.SaveActiveCampaignRunToRuntime(context.Background(), fileSvc, evaluation.ActiveCampaignRun{
		RunID:      "active-run-1",
		CampaignID: "eval-init-qwen3-4b",
	}))
	deps := nativeEvalDeps{
		configLoader: func(projectRoot string) (*config.Config, error) {
			return &config.Config{ProjectRoot: projectRoot}, nil
		},
		fileSvcFactory: func(projectRoot string, _ *slog.Logger) (fs.RuntimeFileService, error) {
			return fs.NewRuntimeFileService(projectRoot, nil)
		},
		createRuntimeTree: func(ctx context.Context, service fs.RuntimeFileService) error {
			return service.CreateRuntimeTree(ctx)
		},
	}

	tests := []struct {
		name      string
		command   string
		flagValue string
		args      []string
		project   string
		want      string
		wantErr   string
	}{
		{
			name:    "positional run id",
			command: "verify",
			args:    []string{"run-positional"},
			want:    "run-positional",
		},
		{
			name:      "flag run id",
			command:   "verify",
			flagValue: "run-flag",
			want:      "run-flag",
		},
		{
			name:      "conflicting flag and positional",
			command:   "verify",
			flagValue: "run-a",
			args:      []string{"run-b"},
			wantErr:   "conflicting run ID",
		},
		{
			name:    "too many positional args",
			command: "verify",
			args:    []string{"run-a", "run-b"},
			wantErr: "accepts at most one run ID argument",
		},
		{
			name:    "loads active run marker",
			command: "verify",
			project: root,
			want:    "active-run-1",
		},
		{
			name:    "missing run id",
			command: "verify",
			project: filepath.Join(root, "missing"),
			wantErr: constants.ErrMissingRequiredField.Error(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := &cobra.Command{Use: "eval"}
			command.SetContext(context.Background())
			command.Flags().String("project-root", test.project, "")
			got, err := resolveCampaignRunID(command, deps, test.command, test.flagValue, test.args)
			if test.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestMarkQueueEntryVerifiedAfterPass(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{VariantID: "qwen3-4b", ServedModelTag: "qwen3:4b", Status: "pending"},
		},
	}
	fileSvc, err := fs.NewRuntimeFileService(root, nil)
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	require.NoError(t, evaluation.SaveInitCampaignQueueToRuntime(context.Background(), fileSvc, evaluation.DefaultInitCampaignQueueRelPath, &queue))

	plan := &evaluation.CampaignStartPlan{
		RunID: "run-pass-1",
		QueueEntry: &evaluation.CampaignQueueModel{
			VariantID: "qwen3-4b",
		},
	}
	report := &evalv1.EvaluationVerificationReport{
		Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
	}

	require.NoError(t, markQueueEntryVerifiedAfterPassWithFileService(context.Background(), fileSvc, plan, report, false))
	loaded, err := evaluation.LoadInitCampaignQueueFromRuntime(context.Background(), fileSvc, evaluation.DefaultInitCampaignQueueRelPath)
	require.NoError(t, err)
	require.Len(t, loaded.Models, 1)
	assert.Equal(t, "verified", loaded.Models[0].Status)
	assert.Equal(t, "run-pass-1", loaded.Models[0].VerifiedRunID)
	assert.Contains(t, loaded.Models[0].Notes, "75/75 verify PASS")

	require.NoError(t, markQueueEntryVerifiedAfterPassWithFileService(context.Background(), fileSvc, plan, report, true))
	loaded, err = evaluation.LoadInitCampaignQueueFromRuntime(context.Background(), fileSvc, evaluation.DefaultInitCampaignQueueRelPath)
	require.NoError(t, err)
	assert.Equal(t, evaluation.StrictWitnessVerifyNotes("run-pass-1"), loaded.Models[0].Notes)

	require.NoError(t, markQueueEntryVerifiedAfterPassWithFileService(context.Background(), fileSvc, nil, report, false))
	require.NoError(t, markQueueEntryVerifiedAfterPassWithFileService(context.Background(), fileSvc, plan, nil, false))
	failReport := &evalv1.EvaluationVerificationReport{
		Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
	}
	require.NoError(t, markQueueEntryVerifiedAfterPassWithFileService(context.Background(), fileSvc, plan, failReport, false))
}
