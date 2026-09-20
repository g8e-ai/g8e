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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func campaignOrchestrateOperators() []models.OperatorDocumentGo {
	return []models.OperatorDocumentGo{
		{
			ID:                "infer-op",
			OperatorSessionID: "infer-session",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
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

func writeCampaignWitnessPreflightResponse(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.InferenceProviderObservations+"_preflight":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
		return true
	case r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.InferenceModelProvenanceAttestations+"_preflight":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
		return true
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, constants.APIPaths.InferenceModelProvenanceAttestations+"_attest"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
		return true
	default:
		return false
	}
}

func enableCampaignWitnessGateway(t *testing.T, root string, deps nativeEvalDeps) func() {
	t.Helper()
	writeCampaignExecuteGatewayCredentials(t, root, deps)
	original := gatewayHealthCheck
	gatewayHealthCheck = func() bool { return true }
	return func() { gatewayHealthCheck = original }
}

func setupCampaignOrchestrateEnv(t *testing.T) (root string, deps nativeEvalDeps, cmd *cobra.Command, cleanup func()) {
	t.Helper()
	root = t.TempDir()
	variant := &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProviderClass:  "ollama",
	}
	writeTestFrozenInventory(t, root, evaluation.DefaultModelInventoryRelPath, variant)

	operators := campaignOrchestrateOperators()
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: operators})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeCampaignWitnessPreflightResponse(w, r) {
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.Operators {
			w.Header().Set("Content-Type", "application/json")
			_, writeErr := w.Write(body)
			require.NoError(t, writeErr)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, constants.APIPaths.InferenceProviderObservations) {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, constants.APIPaths.InferenceModelProvenanceAttestations) {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	}))

	paths := config.DefaultPathsConfig()
	paths.Host = server.URL
	cfg := &config.Config{
		ProjectRoot: root,
		RuntimeDir:  root + "/.g8e",
		Paths:       &paths,
	}

	fixedNow := time.Unix(1789657337, 0).UTC()
	deps = nativeEvalDeps{
		configLoader: func(string) (*config.Config, error) { return cfg, nil },
		fileSvcFactory: func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			return fs.NewRuntimeFileService(root, slog.Default())
		},
		createRuntimeTree: func(context.Context, fs.RuntimeFileService) error { return nil },
		clientFactory:     harnessclient.New,
		authLoader: func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
			return &auth.ClientAuthContext{
				UserID:       "user-1",
				CLISessionID: "cli-1",
			}, nil
		},
		now:   func() time.Time { return fixedNow },
		newID: func() string { return "test-id" },
	}

	cmd = silentCobraCommand()
	cmd.SetContext(context.Background())
	cmd.Flags().String("project-root", root, "")

	return root, deps, cmd, func() { server.Close() }
}

func TestRunCampaignStartFlow_DryRunPrintsPlan(t *testing.T) {
	_, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	var output bytes.Buffer
	cmd.SetOut(&output)

	result, err := runCampaignStartFlow(cmd, deps, campaignStartFlowOptions{
		ModelTag:  "qwen3:4b",
		DryRun:    true,
		PrintPlan: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Plan)
	assert.Equal(t, "eval-init-qwen3-4b", result.Plan.CampaignID)
	assert.Contains(t, output.String(), "eval-init-qwen3-4b")
	assert.Contains(t, output.String(), "infer-session")
	assert.Contains(t, output.String(), "data-session")
}

func TestRunCampaignStartFlow_PrepareOnlyInitializesAndSchedules(t *testing.T) {
	root, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	var output bytes.Buffer
	cmd.SetOut(&output)

	result, err := runCampaignStartFlow(cmd, deps, campaignStartFlowOptions{
		ModelTag:    "qwen3:4b",
		PrepareOnly: true,
		Publish:     false,
		Daemon:      false,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Plan)
	assert.Zero(t, result.Executed)
	assert.Contains(t, output.String(), "Initialized campaign")
	assert.Contains(t, output.String(), "Scheduled")

	active, err := evaluation.LoadActiveCampaignRun(root)
	require.NoError(t, err)
	assert.Equal(t, result.Plan.RunID, active.RunID)
}

func TestResolveCampaignOperatorSessions_SelectsInferenceAndDataOperators(t *testing.T) {
	_, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	cfg, err := deps.configLoader("")
	require.NoError(t, err)

	sessions, err := resolveCampaignOperatorSessions(cmd, deps, cfg, "", "")
	require.NoError(t, err)
	assert.Equal(t, "infer-session", sessions.InferenceSessionID)
	assert.Equal(t, "data-session", sessions.DataSessionID)
	assert.Equal(t, "data-op", sessions.DataOperatorID)
}

func TestResolveCampaignOperatorSessions_RespectsPinnedSessions(t *testing.T) {
	_, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	cfg, err := deps.configLoader("")
	require.NoError(t, err)

	sessions, err := resolveCampaignOperatorSessions(cmd, deps, cfg, "infer-session", "data-session")
	require.NoError(t, err)
	assert.Equal(t, "infer-session", sessions.InferenceSessionID)
	assert.Equal(t, "data-session", sessions.DataSessionID)
}

func TestInitializeCampaignRun_RejectsInventoryCampaignMismatch(t *testing.T) {
	root, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	inventoryPath := root + "/inventory-mismatch.json"
	freeze, err := evaluation.MaterializeModelRegistry("eval-init-qwen3-4b", []*evalv1.ModelVariant{
		{
			VariantId:      "qwen3-4b",
			ServedModelTag: "qwen3:4b",
			ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ProviderClass:  "ollama",
		},
	})
	require.NoError(t, err)
	freeze.CampaignID = "wrong-campaign-id"
	require.NoError(t, writeModelInventoryFreezeFile(inventoryPath, freeze))

	plan := &evaluation.CampaignStartPlan{
		CampaignID:    "eval-init-qwen3-4b",
		RunID:         "run-mismatch-1",
		InventoryPath: inventoryPath,
	}

	err = initializeCampaignRun(cmd, deps, plan, campaignOperatorSessions{
		InferenceSessionID: "infer-session",
		DataSessionID:      "data-session",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inventory campaign_id mismatch")
}

func TestScheduleHomogeneousCampaignRun_RequiresInitializedRun(t *testing.T) {
	root, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	plan, err := evaluation.ResolveCampaignStartPlan(evaluation.CampaignStartPlanRequest{
		ProjectRoot: root,
		ModelTag:    "qwen3:4b",
		Now:         deps.now().UTC(),
	})
	require.NoError(t, err)

	cfg, err := deps.configLoader(root)
	require.NoError(t, err)
	sessions, err := resolveCampaignOperatorSessions(cmd, deps, cfg, "", "")
	require.NoError(t, err)

	require.NoError(t, initializeCampaignRun(cmd, deps, plan, sessions))
	count, err := scheduleHomogeneousCampaignRun(cmd, deps, plan.RunID, false)
	require.NoError(t, err)
	assert.Greater(t, count, 0)
}

func TestCollectCampaignListRows_IncludesVerificationStatus(t *testing.T) {
	root := t.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	store := evaluation.NewStore(fileSvc)
	controller := evaluation.NewCampaignController(store, nil, time.Now, func(prefix string) string { return prefix + "-1" })
	req := evaluation.CampaignInitRequest{
		CampaignID:                 "north-star-smoke",
		RunID:                      "run-smoke-1",
		RepetitionCount:            1,
		InferenceOperatorSessionID: "inf-session",
		DataOperatorSessionID:      "data-session",
	}
	catalog, artifacts, err := evaluation.LoadScenarioCatalog()
	require.NoError(t, err)
	inventory, err := evaluation.MaterializeModelRegistry("north-star-smoke", []*evalv1.ModelVariant{
		{
			VariantId:      "qwen3-4b",
			ServedModelTag: "qwen3:4b",
			ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ProviderClass:  "ollama",
		},
	})
	require.NoError(t, err)
	req.Catalog = catalog
	req.Inventory = inventory
	req.ScenarioArtifacts = artifacts
	_, err = controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)

	rows, err := collectCampaignListRows(context.Background(), controller, store, []evaluation.CampaignListEntry{
		{CampaignID: "north-star-smoke", RunIDs: []string{"run-smoke-1"}, ModelCount: 1, ScenarioCount: 3, RepetitionCount: 1},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "run-smoke-1", rows[0].RunID)
	assert.Equal(t, "north-star-smoke", rows[0].CampaignID)
	assert.Greater(t, rows[0].ExpectedAssignments, uint64(0))
}

func TestCampaignEvalStartCmd_DryRunViaCLI(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "start", "--project-root", root, "--model", "qwen3:4b", "--dry-run"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "eval-init-qwen3-4b")
}

// Ensure harness client factory accepts ensemble URL override used by execute path.
func TestChatEvalEnsembleClient_UsesOverrideURL(t *testing.T) {
	seen := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client, err := chatEvalEnsembleClient(
		&config.Config{},
		&auth.ClientAuthContext{UserID: "user-1"},
		server.URL,
		chatEvalDeps{clientFactory: func(cfg harnessconfig.Config) (*harnessclient.Client, error) {
			assert.Equal(t, server.URL, cfg.EnsembleBaseURL)
			return harnessclient.New(cfg)
		}},
	)
	require.NoError(t, err)
	require.NotNil(t, client)
	_ = seen
}
