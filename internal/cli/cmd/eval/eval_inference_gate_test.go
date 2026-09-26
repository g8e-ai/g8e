// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"io"
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
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func writeInferenceAppCredentialFiles(t *testing.T, root string) (certPath, keyPath string) {
	t.Helper()
	certDir := filepath.Join(root, "apps")
	require.NoError(t, os.MkdirAll(certDir, 0o755))
	certPath = filepath.Join(certDir, "g8ee"+constants.FileExtCert)
	keyPath = filepath.Join(certDir, "g8ee"+constants.FileExtKey)

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600))
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cmdtest.GenerateApproveTestCertDER(t, priv)}), 0o600))
	return certPath, keyPath
}

func inferenceProbeResponseText(req *operatorv1.InferenceDispatchRequest) string {
	for i := len(req.GetMessages()) - 1; i >= 0; i-- {
		for _, part := range req.GetMessages()[i].GetParts() {
			text := part.GetText()
			if text == "" {
				continue
			}
			if strings.Contains(text, "Reply with exactly:") {
				return strings.TrimSpace(strings.SplitN(text, "Reply with exactly:", 2)[1])
			}
			if strings.Contains(text, `{"answer":"structured-ok"}`) {
				return `{"answer":"structured-ok"}`
			}
		}
	}
	if len(req.GetTools()) > 0 {
		return ""
	}
	return "probe-ok"
}

func buildInferenceDispatchResponse(req *operatorv1.InferenceDispatchRequest) (*operatorv1.InferenceDispatchResponse, []*operatorv1.InferenceProgressEvent) {
	text := inferenceProbeResponseText(req)
	parts := make([]*operatorv1.InferenceResponsePart, 0, 1)
	if text != "" {
		parts = append(parts, &operatorv1.InferenceResponsePart{Part: &operatorv1.InferenceResponsePart_Text{Text: text}})
	}
	if len(req.GetTools()) > 0 {
		parts = append(parts, &operatorv1.InferenceResponsePart{Part: &operatorv1.InferenceResponsePart_ToolCall{
			ToolCall: &operatorv1.InferenceToolCall{
				CallId:        "probe-call-1",
				Name:          "probe_echo",
				ArgumentsJson: `{"message":"tool-ok"}`,
			},
		}})
	}
	digest := "digest-" + req.GetProviderAttemptId()
	result := &operatorv1.InferenceResult{
		ProviderAttemptId: req.GetProviderAttemptId(),
		RequestedModel:    req.GetModel(),
		ResultDigest:      digest,
		Parts:             parts,
		FinishReason:      "stop",
	}
	if req.GetStream() {
		outputHash, err := models.ComputeInferenceOutputHash(parts, "stop")
		if err == nil {
			result.OutputHash = outputHash
		}
		progress := []*operatorv1.InferenceProgressEvent{{
			ProviderAttemptId: req.GetProviderAttemptId(),
			Sequence:          1,
			Parts:             parts,
		}}
		return &operatorv1.InferenceDispatchResponse{
			Result:  result,
			Receipt: &operatorv1.ActionReceipt{ResultSummary: digest},
		}, progress
	}
	return &operatorv1.InferenceDispatchResponse{
		Result:  result,
		Receipt: &operatorv1.ActionReceipt{ResultSummary: digest},
	}, nil
}

func writeInferenceDispatchStream(w io.Writer, resp *operatorv1.InferenceDispatchResponse, progress []*operatorv1.InferenceProgressEvent) error {
	marshal := protojson.MarshalOptions{EmitUnpopulated: false}
	for _, event := range progress {
		frame := &operatorv1.InferenceDispatchStreamFrame{Frame: &operatorv1.InferenceDispatchStreamFrame_Progress{Progress: event}}
		body, err := marshal.Marshal(frame)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(w, string(body)+"\n"); err != nil {
			return err
		}
	}
	completion := &operatorv1.InferenceDispatchStreamFrame{Frame: &operatorv1.InferenceDispatchStreamFrame_Completion{Completion: resp}}
	body, err := marshal.Marshal(completion)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, string(body)+"\n")
	return err
}

func setupInferenceEvalEnv(t *testing.T) (root string, deps nativeEvalDeps, cmd *cobra.Command, cleanup func()) {
	t.Helper()
	root = t.TempDir()
	writeTestFrozenInventory(t, root, evaluation.DefaultModelInventoryRelPath, &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProviderClass:  "ollama",
	})

	certPath, keyPath := writeInferenceAppCredentialFiles(t, root)
	t.Setenv(string(constants.EnvVar.AppCert), certPath)
	t.Setenv(string(constants.EnvVar.AppKey), keyPath)

	operators := campaignOrchestrateOperators()
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: operators})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.Operators:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case r.Method == http.MethodPost && r.URL.Path == constants.APIPaths.InferenceDispatch:
			payload, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				http.Error(w, readErr.Error(), http.StatusBadRequest)
				return
			}
			req := &operatorv1.InferenceDispatchRequest{}
			if err := protojson.Unmarshal(payload, req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			resp, progress := buildInferenceDispatchResponse(req)
			if req.GetStream() {
				w.Header().Set("Content-Type", constants.HeaderValueApplicationNDJSON)
				_ = writeInferenceDispatchStream(w, resp, progress)
				return
			}
			out, err := protojson.Marshal(resp)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		default:
			http.NotFound(w, r)
		}
	}))

	paths := config.DefaultPathsConfig()
	paths.Host = server.URL
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	cfg := &config.Config{
		ProjectRoot: root,
		RuntimeDir:  fileSvc.Resolve(""),
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
			return &auth.ClientAuthContext{UserID: "user-1", CLISessionID: "cli-1"}, nil
		},
		now:   func() time.Time { return fixedNow },
		newID: func() string { return "attempt-test-1" },
	}

	cmd = cmdtest.SilentCobraCommand()
	cmd.SetContext(context.Background())
	cmd.Flags().String("project-root", root, "")

	return root, deps, cmd, func() { server.Close() }
}

func TestResolveInferenceProbeAppCredentials_UsesEnvPair(t *testing.T) {
	root := t.TempDir()
	certPath, keyPath := writeInferenceAppCredentialFiles(t, root)
	t.Setenv(string(constants.EnvVar.AppCert), certPath)
	t.Setenv(string(constants.EnvVar.AppKey), keyPath)

	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	paths := config.DefaultPathsConfig()
	gotCert, gotKey, err := resolveInferenceProbeAppCredentials(fileSvc, &config.Config{ProjectRoot: root, Paths: &paths})
	require.NoError(t, err)
	assert.Equal(t, certPath, gotCert)
	assert.Equal(t, keyPath, gotKey)
}

func TestResolveInferenceProbeAppCredentials_RejectsMissingCredentials(t *testing.T) {
	root := t.TempDir()
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)

	t.Setenv(string(constants.EnvVar.AppCert), "")
	t.Setenv(string(constants.EnvVar.AppKey), "")

	paths := config.DefaultPathsConfig()
	_, _, err = resolveInferenceProbeAppCredentials(fileSvc, &config.Config{ProjectRoot: root, Paths: &paths})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delegated app credentials")
}

func TestInferenceEvalGatewayClient_LoadsGatewayClient(t *testing.T) {
	_, deps, cmd, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	cfg, fileSvc, authContext, client, err := inferenceEvalGatewayClient(cmd, inferenceEvalDeps{
		configLoader:   deps.configLoader,
		fileSvcFactory: deps.fileSvcFactory,
		authLoader:     deps.authLoader,
		clientFactory:  deps.clientFactory,
	})
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.NotNil(t, fileSvc)
	require.NotNil(t, authContext)
	require.NotNil(t, client)
}

func TestInferenceEvalPrepareProbe_BuildsRequest(t *testing.T) {
	_, deps, cmd, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	selected, probeReq, appClient, err := inferenceEvalPrepareProbe(
		cmd,
		inferenceEvalDeps{
			configLoader:     deps.configLoader,
			fileSvcFactory:   deps.fileSvcFactory,
			authLoader:       deps.authLoader,
			clientFactory:    deps.clientFactory,
			appClientFactory: deps.clientFactory,
			now:              deps.now,
			newID:            deps.newID,
		},
		"infer-session",
		"qwen3:4b",
		"primary",
		"",
		"",
		"",
		"Reply with exactly: probe-ok",
		42,
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.NotNil(t, appClient)
	assert.Equal(t, "infer-session", selected.OperatorSessionID)
	assert.Equal(t, "qwen3:4b", probeReq.Model)
	require.NotNil(t, probeReq.Seed)
	assert.Equal(t, int32(42), *probeReq.Seed)
}

func TestInferenceEvalExecuteProbe_DispatchAndStream(t *testing.T) {
	_, deps, cmd, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	shared := inferenceEvalDeps{
		configLoader:     deps.configLoader,
		fileSvcFactory:   deps.fileSvcFactory,
		authLoader:       deps.authLoader,
		clientFactory:    deps.clientFactory,
		appClientFactory: deps.clientFactory,
		now:              deps.now,
		newID:            deps.newID,
	}
	_, probeReq, appClient, err := inferenceEvalPrepareProbe(cmd, shared, "infer-session", "qwen3:4b", "primary", "", "", "", "Reply with exactly: probe-ok", -1, false)
	require.NoError(t, err)

	resp, progress, err := inferenceEvalExecuteProbe(context.Background(), appClient, probeReq)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, progress)

	probeReq.Stream = true
	_, streamProbe, streamClient, err := inferenceEvalPrepareProbe(cmd, shared, "infer-session", "qwen3:4b", "primary", "", "", "", "Reply with exactly: stream-ok", -1, true)
	require.NoError(t, err)
	streamResp, streamProgress, err := inferenceEvalExecuteProbe(context.Background(), streamClient, streamProbe)
	require.NoError(t, err)
	require.NotNil(t, streamResp)
	assert.NotEmpty(t, streamProgress)
	assert.Contains(t, streamResp.GetResult().GetParts()[0].GetText(), "stream-ok")
}

func TestGateInferenceEvalStatusCmd_ReportsActiveOperator(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"gate", "inference", "status", "--project-root", root})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "infer-session")
}

func TestGateInferenceEvalProbeCmd_RequiresModel(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"gate", "inference", "probe", "--project-root", root})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--model is required")
}

func TestGateInferenceEvalProbeCmd_AcceptsProbe(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"gate", "inference", "probe", "--project-root", root, "--model", "qwen3:4b"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Probe accepted")
}

func TestGateInferenceEvalRunCmd_RunsUnaryBasicCase(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"gate", "inference", "run", "--project-root", root,
		"--model", "qwen3:4b",
		"--cases", "unary-basic",
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "PASS unary-basic")
}

func TestGateInferenceEvalRunCmd_JSONSummary(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	rootCmd := cmdtest.GlobalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{
		"eval", "gate", "inference", "run", "--project-root", root,
		"--model", "qwen3:4b",
		"--cases", "unary-basic",
	})
	require.NoError(t, rootCmd.Execute())
	assert.Contains(t, output.String(), `"passed"`)
}

func TestGateInferenceEvalStatusCmd_JSON(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	rootCmd := cmdtest.GlobalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "gate", "inference", "status", "--project-root", root})
	require.NoError(t, rootCmd.Execute())
	assert.Contains(t, output.String(), `"operator_session_id"`)
}

func TestGateInferenceEvalProbeCmd_Stream(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"gate", "inference", "probe", "--project-root", root, "--model", "qwen3:4b", "--stream"})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "Progress events:")
}

func TestInferenceEvalAppClient_UsesDelegatedCredentials(t *testing.T) {
	_, deps, cmd, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	cfg, fileSvc, err := nativeEvalEnvironment(cmd, deps)
	require.NoError(t, err)
	authContext, err := deps.authLoader(fileSvc, cfg)
	require.NoError(t, err)
	client, err := inferenceEvalAppClient(cfg, fileSvc, authContext, inferenceEvalDeps{
		appClientFactory: deps.clientFactory,
	})
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestInferenceEvalPrepareProbe_WithRegistryFile(t *testing.T) {
	root, deps, cmd, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	registryPath := filepath.Join(root, "registry.json")
	require.NoError(t, os.WriteFile(registryPath, []byte(`{
		"campaign_id": "eval-init-qwen3-4b",
		"model_registry_digest": "digest-1",
		"variants": [{"model": "qwen3:4b", "digest": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}]
	}`), 0o600))

	shared := inferenceEvalDeps{
		configLoader:     deps.configLoader,
		fileSvcFactory:   deps.fileSvcFactory,
		authLoader:       deps.authLoader,
		clientFactory:    deps.clientFactory,
		appClientFactory: deps.clientFactory,
		now:              deps.now,
		newID:            deps.newID,
	}
	_, probeReq, _, err := inferenceEvalPrepareProbe(cmd, shared, "infer-session", "qwen3:4b", "primary", "", "", registryPath, "", -1, false)
	require.NoError(t, err)
	assert.Equal(t, "eval-init-qwen3-4b", probeReq.CampaignID)
	assert.Equal(t, "digest-1", probeReq.ModelRegistryDigest)
	assert.NotEmpty(t, probeReq.ModelDigest)
}

func TestGateInferenceEvalRunCmd_ReportsFailure(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	operators := campaignOrchestrateOperators()
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: operators})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.Operators:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case r.Method == http.MethodPost && r.URL.Path == constants.APIPaths.InferenceDispatch:
			payload, _ := io.ReadAll(r.Body)
			req := &operatorv1.InferenceDispatchRequest{}
			_ = protojson.Unmarshal(payload, req)
			resp := &operatorv1.InferenceDispatchResponse{
				Result: &operatorv1.InferenceResult{
					ProviderAttemptId: req.GetProviderAttemptId(),
					RequestedModel:    req.GetModel(),
					ResultDigest:      "digest-bad",
					Parts: []*operatorv1.InferenceResponsePart{{
						Part: &operatorv1.InferenceResponsePart_Text{Text: "wrong"},
					}},
				},
				Receipt: &operatorv1.ActionReceipt{ResultSummary: "digest-bad"},
			}
			out, _ := protojson.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(server.Close)

	cfg, err := deps.configLoader(root)
	require.NoError(t, err)
	cfg.Paths.Host = server.URL

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"gate", "inference", "run", "--project-root", root, "--model", "qwen3:4b", "--cases", "unary-basic"})
	err = command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "case(s) failed")
}

func TestReconcileVerifiedCampaignMirrorFromDockerInit_RequiresHealthyGateway(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, false)
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	cfg, err := deps.configLoader(root)
	require.NoError(t, err)
	_, err = ReconcileVerifiedCampaignMirrorFromDockerInit(context.Background(), fileSvc, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gateway is not healthy")
}

func TestReconcileVerifiedCampaignMirrorFromDockerInit_RestoresQueue(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	root, deps, _, cleanup := setupCampaignPublishGatewayEnv(t)
	defer cleanup()

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	store := evaluation.NewStore(fileSvc)
	exporter := &evaluationRecordingCampaignFeedExporter{}
	probe := &evaluationStubCampaignMirrorProbe{present: map[string]bool{}}
	coordinator := evaluation.NewCampaignPublicationCoordinator(
		store, fileSvc, evaluation.NewMemoryCampaignPublicationStateStore(), exporter, nil,
	).WithMirrorProbe(probe)
	controller := evaluation.NewCampaignController(
		store,
		&evaluationStubCampaignExecutor{},
		deps.now,
		func(prefix string) string { return prefix + "-1" },
	).WithPublication(coordinator)

	req := cmdtest.EvaluationTestCampaignInitRequest(t)
	run, err := controller.InitializeCampaign(context.Background(), req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(context.Background(), run.GetRunId())
	require.NoError(t, err)
	_, _, err = controller.ExecuteNextAssignment(context.Background(), run.GetRunId(), evaluation.CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              evaluation.InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
	}, req.ScenarioArtifacts)
	require.NoError(t, err)

	report := &evalv1.EvaluationVerificationReport{
		SchemaVersion: evaluation.CampaignSchemaVersion,
		ReportId:      run.GetRunId(),
		RunId:         run.GetRunId(),
		Status:        evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		VerifiedAt:    timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
	}
	boundReport, _, err := evaluation.BindStoredCampaignVerificationReport(context.Background(), store, report, evaluation.CampaignVerificationPolicy{
		VerifierReleaseVersion: constants.EvaluationSourceVersion,
		ProviderObservation:    evaluation.ProviderObservationPolicyInterim,
		ModelProvenance:        evaluation.ModelProvenancePolicyInterim,
		AssessmentTime:         deps.now,
	})
	require.NoError(t, err)
	require.NoError(t, store.SaveCampaignVerification(context.Background(), run.GetRunId(), boundReport))
	_, err = coordinator.PublishRunCatchUpWithVerification(context.Background(), run.GetRunId(), report)
	require.NoError(t, err)

	queue := &evaluation.CampaignQueue{
		Models: []evaluation.CampaignQueueModel{{
			VariantID:     "qwen3-4b",
			Status:        "verified",
			VerifiedRunID: run.GetRunId(),
		}},
	}
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	require.NoError(t, evaluation.SaveInitCampaignQueueToRuntime(context.Background(), fileSvc, evaluation.DefaultInitCampaignQueueRelPath, queue))

	cfg, err := deps.configLoader(root)
	require.NoError(t, err)
	result, err := ReconcileVerifiedCampaignMirrorFromDockerInit(context.Background(), fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{run.GetRunId()}, result.MissingRunIDs)
}

func TestInferenceEvalPrepareProbe_RejectsUnknownSession(t *testing.T) {
	_, deps, cmd, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	shared := inferenceEvalDeps{
		configLoader:     deps.configLoader,
		fileSvcFactory:   deps.fileSvcFactory,
		authLoader:       deps.authLoader,
		clientFactory:    deps.clientFactory,
		appClientFactory: deps.clientFactory,
		now:              deps.now,
		newID:            deps.newID,
	}
	_, _, _, err := inferenceEvalPrepareProbe(cmd, shared, "missing-session", "qwen3:4b", "primary", "", "", "", "", -1, false)
	require.Error(t, err)
}

func TestGateInferenceEvalProbeCmd_JSON(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	rootCmd := cmdtest.GlobalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "gate", "inference", "probe", "--project-root", root, "--model", "qwen3:4b"})
	require.NoError(t, rootCmd.Execute())
	assert.Contains(t, output.String(), `"result"`)
}

func TestGateInferenceEvalRunCmd_RequiresModel(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"gate", "inference", "run", "--project-root", root})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--model is required")
}

func TestGateInferenceEvalProbeCmd_InvalidRole(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"gate", "inference", "probe", "--project-root", root, "--model", "qwen3:4b", "--role", "invalid"})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceRoleInvalid)
}

func TestInferenceEvalGatewayClient_RejectsMissingProjectRoot(t *testing.T) {
	cmd := cmdtest.SilentCobraCommand()
	cmd.SetContext(context.Background())
	_, _, _, _, err := inferenceEvalGatewayClient(cmd, inferenceEvalDeps{
		configLoader: func(string) (*config.Config, error) { return nil, nil },
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root")
}

func TestGateInferenceEvalRunCmd_RunsMultipleCases(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"gate", "inference", "run", "--project-root", root,
		"--model", "qwen3:4b",
		"--cases", "unary-basic,role-primary,streaming-progress",
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "PASS unary-basic")
	assert.Contains(t, output.String(), "PASS role-primary")
	assert.Contains(t, output.String(), "PASS streaming-progress")
}

func TestGateInferenceEvalRunCmd_RunsDefaultMatrix(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"gate", "inference", "run", "--project-root", root, "--model", "qwen3:4b"})
	require.NoError(t, command.Execute())
}

func TestInferenceEvalExecuteProbe_StreamValidationFailure(t *testing.T) {
	root, deps, cmd, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	operators := campaignOrchestrateOperators()
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: operators})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.Operators:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case r.Method == http.MethodPost && r.URL.Path == constants.APIPaths.InferenceDispatch:
			payload, _ := io.ReadAll(r.Body)
			req := &operatorv1.InferenceDispatchRequest{}
			_ = protojson.Unmarshal(payload, req)
			resp, _ := buildInferenceDispatchResponse(req)
			// Drop progress events to force stream validation failure.
			out, _ := protojson.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(func() { server.Close() })

	cfg, err := deps.configLoader(root)
	require.NoError(t, err)
	cfg.Paths.Host = server.URL

	shared := inferenceEvalDeps{
		configLoader:     deps.configLoader,
		fileSvcFactory:   deps.fileSvcFactory,
		authLoader:       deps.authLoader,
		clientFactory:    deps.clientFactory,
		appClientFactory: deps.clientFactory,
		now:              deps.now,
		newID:            deps.newID,
	}
	_, probeReq, appClient, err := inferenceEvalPrepareProbe(cmd, shared, "infer-session", "qwen3:4b", "primary", "", "", "", "Reply with exactly: stream-ok", -1, true)
	require.NoError(t, err)
	_, _, err = inferenceEvalExecuteProbe(context.Background(), appClient, probeReq)
	require.Error(t, err)
}

func TestGateInferenceEvalRunCmd_JSONFailureSummary(t *testing.T) {
	root, deps, _, cleanup := setupInferenceEvalEnv(t)
	defer cleanup()

	operators := campaignOrchestrateOperators()
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: operators})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.Operators:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case r.Method == http.MethodPost && r.URL.Path == constants.APIPaths.InferenceDispatch:
			payload, _ := io.ReadAll(r.Body)
			req := &operatorv1.InferenceDispatchRequest{}
			_ = protojson.Unmarshal(payload, req)
			resp := &operatorv1.InferenceDispatchResponse{
				Result: &operatorv1.InferenceResult{
					ProviderAttemptId: req.GetProviderAttemptId(),
					RequestedModel:    req.GetModel(),
					ResultDigest:      "digest-bad",
					Parts: []*operatorv1.InferenceResponsePart{{
						Part: &operatorv1.InferenceResponsePart_Text{Text: "wrong"},
					}},
				},
				Receipt: &operatorv1.ActionReceipt{ResultSummary: "digest-bad"},
			}
			out, _ := protojson.Marshal(resp)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(out)
		}
	}))
	t.Cleanup(func() { server.Close() })

	cfg, err := deps.configLoader(root)
	require.NoError(t, err)
	cfg.Paths.Host = server.URL

	command := evalCmdWithConfig(deps)
	rootCmd := cmdtest.GlobalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "gate", "inference", "run", "--project-root", root, "--model", "qwen3:4b", "--cases", "unary-basic"})
	err = rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, output.String(), `"failed"`)
}
