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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
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
	writeTestRuntimeQueue(t, root, &queue)

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
	assert.Contains(t, output.String(), "eval/init-campaign-queue.json")
	assertTestRuntimeFileExists(t, root, evaluation.DefaultInitCampaignQueueRelPath)
	assertTestRuntimeFileExists(t, root, "eval/inventories/eval-init-qwen3-4b.json")
}

func TestQueueEvalMarkVerified(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{VariantID: "qwen3-4b", ServedModelTag: "qwen3:4b", Status: "pending"},
		},
	}
	fileSvc := writeTestRuntimeQueue(t, root, &queue)

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

	loaded, err := evaluation.LoadInitCampaignQueueFromRuntime(context.Background(), fileSvc, evaluation.DefaultInitCampaignQueueRelPath)
	require.NoError(t, err)
	assert.Equal(t, "verified", loaded.Models[0].Status)
	assert.Equal(t, "eval-init-qwen3-4b-123", loaded.Models[0].VerifiedRunID)
}

func TestInventoryEvalMaterializeFormationCatalog(t *testing.T) {
	root := t.TempDir()
	variants := evaluation.FormationCatalogFixtureVariants(func(tag string) string {
		if tag == "gemini-1.5-pro" {
			return evaluation.FormationDelegatedRegistryDigestPlaceholder
		}
		return repeatTestHex('f', 64)
	})
	sovereignOnly := make([]*evalv1.ModelVariant, 0, len(variants)-1)
	for _, variant := range variants {
		if variant.GetServedModelTag() == "gemini-1.5-pro" {
			continue
		}
		sovereignOnly = append(sovereignOnly, variant)
	}
	sourcePath := filepath.Join(root, "provider-freeze.json")
	freeze, err := evaluation.MaterializeModelRegistry("provider-freeze", sovereignOnly)
	require.NoError(t, err)
	require.NoError(t, writeModelInventoryFreezeFile(sourcePath, freeze))

	outputRel := "eval/inventories/eval-formations-benchmark.json"
	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"models", "materialize", "--project-root", root,
		"--from", sourcePath,
		"--formation-catalog",
		"--campaign-id", "eval-formations-benchmark",
		"--output", outputRel,
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "10 models")
	assertTestRuntimeFileExists(t, root, outputRel)

	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	loaded, err := evaluation.LoadModelInventoryFreezeFromRuntime(context.Background(), fileSvc, outputRel)
	require.NoError(t, err)
	assert.Equal(t, "eval-formations-benchmark", loaded.CampaignID)
	assert.Len(t, loaded.Variants, 10)
	require.NoError(t, evaluation.ValidateModelRegistry(loaded))
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
	assertTestRuntimeFileExists(t, root, "eval/inventories/eval-init-qwen3-4b.json")
}

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

func TestInventoryEvalList_PrintsFrozenVariants(t *testing.T) {
	root := t.TempDir()
	writeTestFrozenInventory(t, root, evaluation.DefaultModelInventoryRelPath,
		&evalv1.ModelVariant{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: "digest", ProviderClass: "ollama"},
	)

	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"models", "list", "--project-root", root})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "qwen3:4b")
	assert.Contains(t, output.String(), "qwen3-4b")
}

func TestInventoryEvalFreeze_RequiresCampaignID(t *testing.T) {
	command := evalCmdWithConfig(testNativeEvalDeps(t.TempDir()))
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetArgs([]string{"models", "freeze", "--ollama-endpoint", "http://127.0.0.1:11434"})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestModelInventoryFreezeJSON_RejectsNilFreeze(t *testing.T) {
	_, err := modelInventoryFreezeJSON(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestWriteModelInventoryFreezeFile_WritesJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "inventory-freeze.json")
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

func TestFinishCampaignQueueRun_TextAndJSON(t *testing.T) {
	command := &cobra.Command{}
	var output bytes.Buffer
	command.SetOut(&output)

	result := campaignQueueRunResult{Planned: 2, Succeeded: 2, Failed: 0, LogDir: "/tmp/logs"}
	require.NoError(t, finishCampaignQueueRun(command, "/queue/path", result))
	assert.Contains(t, output.String(), "planned=2")
	assert.Contains(t, output.String(), "/tmp/logs")

	command = evalCmdWithConfig(testNativeEvalDeps(t.TempDir()))
	enableGlobalJSON(t, command)
	output.Reset()
	command.SetOut(&output)
	result = campaignQueueRunResult{
		Planned:   2,
		Succeeded: 1,
		Failed:    1,
		Failures: []campaignQueueRunFailure{{
			VariantID: "qwen3-4b",
			Tag:       "qwen3:4b",
			Error:     "boom",
		}},
	}
	err := finishCampaignQueueRun(command, "/queue/path", result)
	require.Error(t, err)
	var payload campaignQueueRunResult
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	assert.Equal(t, 2, payload.Planned)
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

func TestWriteCampaignQueueRunPlan_TextAndJSON(t *testing.T) {
	plan := []evaluation.CampaignQueueModel{
		{VariantID: "qwen3-4b", ServedModelTag: "qwen3:4b", Status: "pending"},
	}
	command := &cobra.Command{}
	var output bytes.Buffer
	command.SetOut(&output)
	require.NoError(t, writeCampaignQueueRunPlan(command, plan, false))
	assert.Contains(t, output.String(), "qwen3-4b")

	command = evalCmdWithConfig(testNativeEvalDeps(t.TempDir()))
	enableGlobalJSON(t, command)
	output.Reset()
	command.SetOut(&output)
	require.NoError(t, writeCampaignQueueRunPlan(command, plan, true))
	assert.Contains(t, output.String(), `"models"`)
}

func TestRolloutEvalListCmd_TextAndJSON(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{VariantID: "qwen3-4b", ServedModelTag: "qwen3:4b", Status: "pending", CampaignID: "eval-init-qwen3-4b", HomogeneousCellCount: 3},
			{VariantID: "gemma3-4b", ServedModelTag: "gemma3:4b", Status: "verified", CampaignID: "eval-init-gemma3-4b", HomogeneousCellCount: 3},
		},
	}
	writeTestRuntimeQueue(t, root, &queue)

	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rollout", "list", "--project-root", root, "--status", "pending"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "qwen3-4b")
	assert.NotContains(t, output.String(), "gemma3-4b")

	rootCmd := globalJSONRoot(t, command)
	output.Reset()
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "rollout", "list", "--project-root", root})
	require.NoError(t, rootCmd.Execute())
	assert.Contains(t, output.String(), `"models"`)
}

func TestRolloutEvalListCmd_ReportsEmptyQueue(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{VariantID: "qwen3-4b", ServedModelTag: "qwen3:4b", Status: "pending"},
		},
	}
	writeTestRuntimeQueue(t, root, &queue)

	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rollout", "list", "--project-root", root, "--status", "verified"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "No queue entries found")
}

func TestRolloutEvalNextCmd_ShowsPendingEntry(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{
				VariantID:            "qwen3-4b",
				ServedModelTag:       "qwen3:4b",
				Status:               "pending",
				CampaignID:           "eval-init-qwen3-4b",
				InventoryFile:        "eval/inventories/eval-init-qwen3-4b.json",
				ModelRegistryDigest:  "digest-1",
				HomogeneousCellCount: 3,
			},
		},
	}
	writeTestRuntimeQueue(t, root, &queue)

	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rollout", "next", "--project-root", root})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "qwen3:4b")
	assert.Contains(t, output.String(), "eval-init-qwen3-4b")
}

func TestRolloutEvalRunCmd_RecordsStartFailure(t *testing.T) {
	root, deps, _, cleanup := setupCampaignWitnessEnv(t)
	defer cleanup()

	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{
				VariantID:      "qwen3-4b",
				ServedModelTag: "qwen3:4b",
				Status:         "pending",
				CampaignID:     "north-star-smoke",
			},
			{
				VariantID:      "gemma3-4b",
				ServedModelTag: "gemma3:4b",
				Status:         "pending",
				CampaignID:     "north-star-smoke",
			},
		},
	}
	fileSvc := writeTestRuntimeQueue(t, root, &queue)

	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() { health.Close(); mirror.Close() })

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"rollout", "run", "--project-root", root,
		"--ensemble-health-url", health.URL,
		"--mirror-bootstrap-url", mirror.URL,
	})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 model(s) failed")
	assert.Contains(t, output.String(), "FAIL qwen3-4b")
	assert.Contains(t, output.String(), "FAIL gemma3-4b")
	assert.Contains(t, output.String(), "Queue run finished: planned=2 succeeded=0 failed=2")

	loaded, loadErr := evaluation.LoadInitCampaignQueueFromRuntime(context.Background(), fileSvc, evaluation.DefaultInitCampaignQueueRelPath)
	require.NoError(t, loadErr)
	assert.Equal(t, "failed", loaded.Models[0].Status)
	assert.Equal(t, "failed", loaded.Models[1].Status)
}

func campaignWitnessOperators() []models.OperatorDocumentGo {
	ops := campaignOrchestrateOperators()
	return append(ops,
		models.OperatorDocumentGo{
			Status:        constants.OperatorStatusActive,
			OperatorType:  constants.OperatorTypeRemote,
			RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true},
		},
		models.OperatorDocumentGo{
			Status:        constants.OperatorStatusActive,
			OperatorType:  constants.OperatorTypeRemote,
			RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
		},
	)
}

func setupCampaignWitnessEnv(t *testing.T) (root string, deps nativeEvalDeps, cmd *cobra.Command, cleanup func()) {
	t.Helper()
	root = t.TempDir()
	writeTestFrozenInventory(t, root, evaluation.DefaultModelInventoryRelPath, &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProviderClass:  "ollama",
	})

	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: campaignWitnessOperators()})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, constants.APIPaths.Operators, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))

	paths := config.DefaultPathsConfig()
	paths.Host = server.URL
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	cfg := &config.Config{ProjectRoot: root, RuntimeDir: fileSvc.Resolve(""), Paths: &paths}
	deps = nativeEvalDeps{
		configLoader: func(string) (*config.Config, error) { return cfg, nil },
		fileSvcFactory: func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			return fs.NewRuntimeFileService(root, slog.Default())
		},
		createRuntimeTree: func(context.Context, fs.RuntimeFileService) error { return nil },
		clientFactory:     harnessclient.New,
		authLoader: func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
			return &auth.ClientAuthContext{UserID: "user-1", CLISessionID: "cli-1"}, nil
		},
		now:   func() time.Time { return time.Unix(1789657337, 0).UTC() },
		newID: func() string { return "test-id" },
	}
	cmd = silentCobraCommand()
	cmd.SetContext(context.Background())
	cmd.Flags().String("project-root", root, "")
	return root, deps, cmd, func() { server.Close() }
}

func TestPreflightCampaignQueueRun_SucceedsWhenWitnessesReady(t *testing.T) {
	_, _, cmd, cleanup := setupCampaignWitnessEnv(t)
	defer cleanup()

	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(health.Close)
	t.Cleanup(mirror.Close)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	require.NoError(t, preflightCampaignQueueRun(cmd, health.URL, mirror.URL))
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Preflight ok (platform health and mirror reachable)")
}

func TestPreflightCampaignQueueRun_DoesNotRequireWitnesses(t *testing.T) {
	_, _, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(health.Close)
	t.Cleanup(mirror.Close)

	err := preflightCampaignQueueRun(cmd, health.URL, mirror.URL)
	require.NoError(t, err)
}

func TestCampaignWitnessStatus_UsesOperatorList(t *testing.T) {
	_, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	cfg, err := deps.configLoader("")
	require.NoError(t, err)

	status, err := campaignWitnessStatus(cmd, deps, cfg)
	require.NoError(t, err)
	assert.Zero(t, status.ActiveObserverCount)
	assert.Zero(t, status.ActiveProvenanceCount)
}

func TestRolloutEvalRunCmd_ReportsEmptySelection(t *testing.T) {
	root := t.TempDir()
	queue := evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{
			{VariantID: "qwen3-4b", ServedModelTag: "qwen3:4b", Status: "verified"},
		},
	}
	writeTestRuntimeQueue(t, root, &queue)

	deps := testNativeEvalDeps(root)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"rollout", "run", "--project-root", root, "--skip-verified"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "No queue entries selected")
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
