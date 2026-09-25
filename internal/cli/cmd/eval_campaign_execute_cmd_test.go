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
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type campaignInventoryFreezeJSON struct {
	CampaignID          string            `json:"campaign_id"`
	ModelRegistryDigest string            `json:"model_registry_digest"`
	Variants            []json.RawMessage `json:"variants"`
}

func TestCampaignEvalExecute_RejectsPreflightWhenGatewayUnhealthy(t *testing.T) {
	withGatewayHealthCheck(t, false)
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{
		"campaign", "execute", "--project-root", root,
		"--no-auto-refresh",
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
		"--no-auto-refresh",
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
		"--no-auto-refresh",
		"--inference-session", "infer-session",
		"--data-session", "data-session",
		runID,
	})
	require.NoError(t, rootCmd.Execute())

	var payload campaignExecuteOutput
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	assert.Equal(t, runID, payload.RunID)
	assert.Equal(t, 1, payload.Executed)
	assert.NotEmpty(t, payload.Results)
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
	payload, err := json.Marshal(campaignInventoryFreezeJSON{
		CampaignID:          inventory.CampaignID,
		ModelRegistryDigest: inventory.RegistryDigest,
		Variants:            []json.RawMessage{rawVariant},
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

	var payload campaignScheduleOutput
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	assert.Equal(t, runID, payload.RunID)
	assert.NotZero(t, payload.AssignmentCount)
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
	payload, err := json.Marshal(campaignInventoryFreezeJSON{
		CampaignID:          inventory.CampaignID,
		ModelRegistryDigest: inventory.RegistryDigest,
		Variants:            rawVariants,
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
		"--system-lane",
	})
	require.NoError(t, command.Execute())
	return runID
}

func TestCampaignEvalSchedule_HeterogeneousMaterializesAssignments(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	variants := []*evalv1.ModelVariant{
		{VariantId: "gemma4-e4b", ServedModelTag: "gemma4:e4b", ModelDigest: repeatTestHex('a', 64), ProviderClass: "ollama", ParameterCount: 4_000_000_000, ModelFamily: "Gemma"},
		{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: repeatTestHex('b', 64), ProviderClass: "ollama", ParameterCount: 4_000_000_000, ModelFamily: "Qwen"},
		{VariantId: "llama3-8b", ServedModelTag: "llama3:8b", ModelDigest: repeatTestHex('c', 64), ProviderClass: "ollama", ParameterCount: 8_000_000_000, ModelFamily: "Llama"},
	}
	runID := prepareUnscheduledCampaignRunWithVariants(t, root, deps, variants)

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{
		"campaign", "stacks", "generate", "--project-root", root,
		"--campaign-id", "north-star-smoke", "--seed", "17",
	})
	require.NoError(t, command.Execute())

	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"campaign", "schedule", "--project-root", root,
		"--heterogeneous", runID,
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "heterogeneous")

	fileSvc, err := deps.fileSvcFactory(root, slog.Default())
	require.NoError(t, err)
	store := evaluation.NewStore(fileSvc)
	assignments, err := store.ListAssignments(context.Background(), runID)
	require.NoError(t, err)
	require.NotEmpty(t, assignments)
	assert.NotNil(t, assignments[0].GetHeterogeneous())
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

func TestCampaignEvalStacksGenerate_FormationCatalogMaterializesFiveStacks(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	variants := testFormationCatalogCLIVariants()
	_ = prepareUnscheduledCampaignRunWithVariants(t, root, deps, variants)
	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"campaign", "stacks", "generate", "--project-root", root,
		"--campaign-id", "north-star-smoke", "--seed", "17", "--formation-catalog",
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "formation catalog")
	assert.Contains(t, output.String(), evaluation.FormationCatalogStackGenerationRule)

	fileSvc, err := deps.fileSvcFactory(root, slog.Default())
	require.NoError(t, err)
	store := evaluation.NewStore(fileSvc)
	stackSet, err := store.LoadHeterogeneousStackSet(context.Background(), "north-star-smoke")
	require.NoError(t, err)
	assert.Len(t, stackSet.Stacks, 5)
	assert.Equal(t, evaluation.FormationCatalogStackGenerationRule, stackSet.GenerationRule)
}

func TestCampaignEvalSchedule_FormationCatalogMaterializesAssignments(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	runID := prepareUnscheduledCampaignRunWithVariants(t, root, deps, testFormationCatalogCLIVariants())
	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{
		"campaign", "stacks", "generate", "--project-root", root,
		"--campaign-id", "north-star-smoke", "--seed", "17", "--formation-catalog",
	})
	require.NoError(t, command.Execute())

	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"campaign", "schedule", "--project-root", root,
		"--heterogeneous", runID,
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "heterogeneous")

	fileSvc, err := deps.fileSvcFactory(root, slog.Default())
	require.NoError(t, err)
	store := evaluation.NewStore(fileSvc)
	assignments, err := store.ListAssignments(context.Background(), runID)
	require.NoError(t, err)
	assert.Len(t, assignments, 5*evaluation.StandardScenarioCount)
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
		"--no-auto-refresh",
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

func TestCampaignEvalPublish_IgnoresLegacyFailedReport(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, _, cleanup := setupCampaignPublishGatewayEnv(t)
	defer cleanup()

	runID := prepareUnscheduledCampaignRun(t, root, deps)
	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "schedule", "--project-root", root, runID})
	require.NoError(t, command.Execute())

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	store := evaluation.NewStore(fileSvc)
	require.NoError(t, store.SaveCampaignVerification(context.Background(), runID, &evalv1.EvaluationVerificationReport{
		SchemaVersion: evaluation.CampaignSchemaVersion,
		ReportId:      runID,
		RunId:         runID,
		Status:        evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		VerifiedAt:    timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
	}))

	command = evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "publish", "--project-root", root, runID})
	require.NoError(t, command.Execute())
}

func TestCampaignEvalPublish_RejectsPersistedPassingReportThatDoesNotApply(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, _, cleanup := setupCampaignPublishGatewayEnv(t)
	defer cleanup()

	runID := prepareUnscheduledCampaignRun(t, root, deps)
	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "schedule", "--project-root", root, runID})
	require.NoError(t, command.Execute())

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	store := evaluation.NewStore(fileSvc)
	require.NoError(t, store.SaveCampaignVerification(context.Background(), runID, &evalv1.EvaluationVerificationReport{
		SchemaVersion:            constants.CampaignVerifierVersion,
		ReportId:                 runID,
		RunId:                    runID,
		Status:                   evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		VerifiedAt:               timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
		VerifiedPopulationDigest: strings.Repeat("9", 64),
	}))

	command = evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "publish", "--project-root", root, runID})
	err = command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
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
		if writeCampaignPublicationProofResponse(w, r) {
			return
		}
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
		if writeCampaignPublicationProofResponse(w, r) {
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

func testFormationCatalogCLIVariants() []*evalv1.ModelVariant {
	digestByTag := map[string]byte{
		"qwen2.5:14b-instruct-q4_K_M":      '1',
		"gemma2:2b-instruct-q4_K_M":        '2',
		"llama3.2:1b-instruct-q4_K_M":      '3',
		"llama3.1:8b-instruct-q4_K_M":      '4',
		"phi3.5:3.8b-mini-instruct-q4_K_M": '5',
		"qwen2.5:0.5b-instruct-q4_K_M":     '6',
		"gemma2:9b-instruct-q4_K_M":        '7',
		"qwen2.5-coder:7b-instruct-q4_K_M": '8',
		"gemini-1.5-pro":                   '0',
		"qwen2.5:1.5b-instruct-q4_K_M":     '9',
	}
	return evaluation.FormationCatalogFixtureVariants(func(tag string) string {
		ch := digestByTag[tag]
		if ch == 0 {
			ch = 'a'
		}
		return repeatTestHex(ch, 64)
	})
}
