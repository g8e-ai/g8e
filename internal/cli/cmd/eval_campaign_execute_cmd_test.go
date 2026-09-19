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
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestCampaignEvalExecute_RejectsPreflightWhenGatewayUnhealthy(t *testing.T) {
	withGatewayHealthCheck(t, false)
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{
		"campaign", "execute", "--project-root", root,
		"--no-auto-refresh", "--wait-for-provider-idle=false",
		active.RunID,
	})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
}

func TestCampaignEvalExecute_ExecutesOneAssignmentViaCLI(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, cmd, cleanup := setupCampaignExecuteGatewayEnv(t)
	defer cleanup()

	runID, store := prepareCampaignRunForExecute(t, deps, cmd)
	controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string {
		return prefix + "-" + deps.newID()
	})
	assignment, ok, err := controller.ResumeNextAssignment(context.Background(), runID)
	require.NoError(t, err)
	require.True(t, ok)

	run, err := store.LoadRun(context.Background(), runID)
	require.NoError(t, err)
	spec, err := store.LoadCampaignSpec(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)

	ensemble := newTestEnsembleServer(func(assignmentID, attemptID string) map[string]any {
		if assignmentID != assignment.GetAssignmentId() {
			return nil
		}
		return buildCompletedCampaignTrace(assignment, attemptID, spec.GetModelRegistryDigest(), "infer-session")
	})
	defer ensemble.Close()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"campaign", "execute", "--project-root", root,
		"--ensemble-url", ensemble.URL,
		"--no-auto-refresh", "--wait-for-provider-idle=false",
		"--inference-session", "infer-session",
		"--data-session", "data-session",
		runID,
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), assignment.GetAssignmentId())
}

func TestCampaignEvalExecute_JSONOutput(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, cmd, cleanup := setupCampaignExecuteGatewayEnv(t)
	defer cleanup()

	runID, store := prepareCampaignRunForExecute(t, deps, cmd)
	controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string {
		return prefix + "-" + deps.newID()
	})
	assignment, ok, err := controller.ResumeNextAssignment(context.Background(), runID)
	require.NoError(t, err)
	require.True(t, ok)

	run, err := store.LoadRun(context.Background(), runID)
	require.NoError(t, err)
	spec, err := store.LoadCampaignSpec(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)

	ensemble := newTestEnsembleServer(func(assignmentID, attemptID string) map[string]any {
		if assignmentID != assignment.GetAssignmentId() {
			return nil
		}
		return buildCompletedCampaignTrace(assignment, attemptID, spec.GetModelRegistryDigest(), "infer-session")
	})
	defer ensemble.Close()

	command := evalCmdWithConfig(deps)
	rootCmd := globalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{
		"eval", "campaign", "execute", "--project-root", root,
		"--ensemble-url", ensemble.URL,
		"--no-auto-refresh", "--wait-for-provider-idle=false",
		"--inference-session", "infer-session",
		"--data-session", "data-session",
		runID,
	})
	require.NoError(t, rootCmd.Execute())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	assert.Equal(t, runID, payload["run_id"])
	assert.Equal(t, float64(1), payload["executed"])
}

func prepareUnscheduledCampaignRun(t *testing.T, root string, deps nativeEvalDeps) string {
	t.Helper()
	variant := &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProviderClass:  "ollama",
	}
	inventory, err := evaluation.MaterializeModelRegistry("north-star-smoke", []*evalv1.ModelVariant{variant})
	require.NoError(t, err)
	rawVariant, err := protojson.Marshal(variant)
	require.NoError(t, err)
	inventoryPath := filepath.Join(root, "inventory-freeze.json")
	payload, err := json.Marshal(map[string]any{
		"campaign_id":           inventory.CampaignID,
		"model_registry_digest": inventory.RegistryDigest,
		"variants":              []json.RawMessage{rawVariant},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(inventoryPath, payload, 0o600))

	runID := "eval-init-qwen3-4b-unscheduled"
	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{
		"campaign", "init", "--project-root", root,
		"--campaign-id", "north-star-smoke",
		"--run-id", runID,
		"--inventory-file", inventoryPath,
		"--inference-session", "infer-session",
		"--data-session", "data-session",
	})
	require.NoError(t, command.Execute())
	return runID
}

func TestCampaignEvalSchedule_MaterializesAssignments(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	runID := prepareUnscheduledCampaignRun(t, root, deps)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "schedule", "--project-root", root, runID})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Scheduled")
}

func TestCampaignEvalSchedule_JSONOutput(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	runID := prepareUnscheduledCampaignRun(t, root, deps)

	command := evalCmdWithConfig(deps)
	rootCmd := globalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "campaign", "schedule", "--project-root", root, runID})
	require.NoError(t, rootCmd.Execute())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	assert.Equal(t, runID, payload["run_id"])
	assert.NotZero(t, payload["assignment_count"])
}

func prepareUnscheduledCampaignRunWithVariants(t *testing.T, root string, deps nativeEvalDeps, variants []*evalv1.ModelVariant) string {
	t.Helper()
	inventory, err := evaluation.MaterializeModelRegistry("north-star-smoke", variants)
	require.NoError(t, err)
	rawVariants := make([]json.RawMessage, 0, len(variants))
	for _, variant := range variants {
		raw, err := protojson.Marshal(variant)
		require.NoError(t, err)
		rawVariants = append(rawVariants, raw)
	}
	inventoryPath := filepath.Join(root, "inventory-freeze.json")
	payload, err := json.Marshal(map[string]any{
		"campaign_id":           inventory.CampaignID,
		"model_registry_digest": inventory.RegistryDigest,
		"variants":              rawVariants,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(inventoryPath, payload, 0o600))

	runID := "eval-init-heterogeneous-unscheduled"
	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{
		"campaign", "init", "--project-root", root,
		"--campaign-id", "north-star-smoke",
		"--run-id", runID,
		"--inventory-file", inventoryPath,
		"--inference-session", "infer-session",
		"--data-session", "data-session",
	})
	require.NoError(t, command.Execute())
	return runID
}

func TestCampaignEvalStacksGenerate_PersistsStacks(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	variants := []*evalv1.ModelVariant{
		{VariantId: "gemma4-e4b", ServedModelTag: "gemma4:e4b", ModelDigest: repeatTestHex('a', 64), ProviderClass: "ollama", ParameterCount: 4_000_000_000},
		{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: repeatTestHex('b', 64), ProviderClass: "ollama", ParameterCount: 4_000_000_000},
		{VariantId: "llama3-8b", ServedModelTag: "llama3:8b", ModelDigest: repeatTestHex('c', 64), ProviderClass: "ollama", ParameterCount: 8_000_000_000},
	}
	_ = prepareUnscheduledCampaignRunWithVariants(t, root, deps, variants)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"campaign", "stacks", "generate", "--project-root", root,
		"--campaign-id", "north-star-smoke", "--seed", "17",
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Generated")
	assert.Contains(t, output.String(), "north-star-smoke")
}

func TestCampaignEvalExecute_WithPublishFlag(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, cmd, cleanup := setupCampaignExecuteGatewayEnv(t)
	defer cleanup()

	runID, store := prepareCampaignRunForExecute(t, deps, cmd)
	controller := evaluation.NewCampaignController(store, nil, deps.now, func(prefix string) string {
		return prefix + "-" + deps.newID()
	})
	assignment, ok, err := controller.ResumeNextAssignment(context.Background(), runID)
	require.NoError(t, err)
	require.True(t, ok)

	run, err := store.LoadRun(context.Background(), runID)
	require.NoError(t, err)
	spec, err := store.LoadCampaignSpec(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)

	ensemble := newTestEnsembleServer(func(assignmentID, attemptID string) map[string]any {
		if assignmentID != assignment.GetAssignmentId() {
			return nil
		}
		return buildCompletedCampaignTrace(assignment, attemptID, spec.GetModelRegistryDigest(), "infer-session")
	})
	defer ensemble.Close()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"campaign", "execute", "--project-root", root,
		"--ensemble-url", ensemble.URL,
		"--no-auto-refresh", "--wait-for-provider-idle=false",
		"--inference-session", "infer-session",
		"--data-session", "data-session",
		"--publish",
		runID,
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), assignment.GetAssignmentId())
}

func TestCampaignEvalSchedule_WithPublish(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, _, cleanup := setupCampaignPublishGatewayEnv(t)
	defer cleanup()

	runID := prepareUnscheduledCampaignRun(t, root, deps)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "schedule", "--project-root", root, "--publish", runID})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Scheduled")
}

func TestCampaignEvalPublish_ForceRepublish(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, _, cleanup := setupCampaignPublishGatewayEnv(t)
	defer cleanup()

	runID := prepareUnscheduledCampaignRun(t, root, deps)
	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "schedule", "--project-root", root, runID})
	require.NoError(t, command.Execute())

	command = evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "publish", "--project-root", root, "--force", runID})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "forced republish")
}

func TestCampaignEvalPublish_PublishesScheduledRun(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, _, cleanup := setupCampaignPublishGatewayEnv(t)
	defer cleanup()

	runID := prepareUnscheduledCampaignRun(t, root, deps)
	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "schedule", "--project-root", root, runID})
	require.NoError(t, command.Execute())

	command = evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "publish", "--project-root", root, runID})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Published")
}

func TestCampaignEvalStacksGenerate_RequiresCampaignID(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "stacks", "generate", "--project-root", root})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestCampaignEvalPublish_RejectsWhenGatewayUnhealthy(t *testing.T) {
	withGatewayHealthCheck(t, false)
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "publish", "--project-root", root, active.RunID})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "campaign publish")
}

func writeCampaignExecuteGatewayCredentials(t *testing.T, root string, deps nativeEvalDeps) {
	t.Helper()
	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	cfg, err := deps.configLoader(root)
	require.NoError(t, err)

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), keyPEM, constants.PermFilePrivate))

	certDER := generateApproveTestCertDER(t, priv)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), certPEM, constants.PermFilePrivate))

	creds := &auth.Credentials{
		OperatorSessionID: "data-session",
		UserID:            "user-1",
		OperatorID:        "data-op",
		CLISessionID:      "cli-1",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
}

func setupCampaignPublishGatewayEnv(t *testing.T) (root string, deps nativeEvalDeps, cmd *cobra.Command, cleanup func()) {
	root, deps, cmd, orchestrateCleanup := setupCampaignOrchestrateEnv(t)
	writeCampaignExecuteGatewayCredentials(t, root, deps)

	mirrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"snapshot":{"high_water_sequence":0},"recent_projections":[]}`))
	}))
	originalBootstrap := publicMirrorBootstrapURL
	publicMirrorBootstrapURL = mirrorServer.URL

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case constants.APIPaths.PublicFeedSnapshot:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"high_water_sequence":0}`))
		case constants.APIPaths.PublicFeedBatches:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"high_water_sequence":1}`))
		default:
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "publication-state") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
				return
			}
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "publication-state") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"schema_version":"eval-campaign-publication-state/v1","run_id":"run","published_idempotency_keys":[],"last_published_sequence":0}`))
				return
			}
			http.NotFound(w, r)
		}
	}))

	config.SetEndpointOverride(gateway.URL)

	return root, deps, cmd, func() {
		gateway.Close()
		mirrorServer.Close()
		publicMirrorBootstrapURL = originalBootstrap
		config.SetEndpointOverride("")
		orchestrateCleanup()
	}
}

func setupCampaignExecuteGatewayEnv(t *testing.T) (root string, deps nativeEvalDeps, cmd *cobra.Command, cleanup func()) {
	t.Helper()
	root, deps, cmd, orchestrateCleanup := setupCampaignOrchestrateEnv(t)
	writeCampaignExecuteGatewayCredentials(t, root, deps)

	mirrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"snapshot":{"high_water_sequence":0},"recent_projections":[]}`))
	}))
	originalBootstrap := publicMirrorBootstrapURL
	publicMirrorBootstrapURL = mirrorServer.URL

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeCampaignWitnessPreflightResponse(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.Operators:
			body, err := json.Marshal(models.OperatorSlotResponse{
				Success:   true,
				Operators: campaignOrchestrateOperators(),
			})
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case r.URL.Path == constants.APIPaths.PublicFeedSnapshot:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"high_water_sequence":0}`))
		case r.URL.Path == constants.APIPaths.PublicFeedBatches:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"high_water_sequence":1}`))
		default:
			if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "publication-state") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
				return
			}
			if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "publication-state") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"schema_version":"eval-campaign-publication-state/v1","run_id":"run","published_idempotency_keys":[],"last_published_sequence":0}`))
				return
			}
			http.NotFound(w, r)
		}
	}))

	config.SetEndpointOverride(gateway.URL)

	return root, deps, cmd, func() {
		gateway.Close()
		mirrorServer.Close()
		publicMirrorBootstrapURL = originalBootstrap
		config.SetEndpointOverride("")
		orchestrateCleanup()
	}
}
