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
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	testInferenceSession = "infer-session"
	testDataSession      = "data-session"
	testHost             = "test-host"
	testPID              = 4242
)

// fakeProcessControl is the run-control process boundary for tests. It makes
// lease liveness deterministic: a lease is live when its host is testHost and
// its PID is marked alive.
type fakeProcessControl struct {
	mu         sync.Mutex
	pid        int
	host       string
	alive      map[int]bool
	interrupts []int
}

func newFakeProcessControl() *fakeProcessControl {
	return &fakeProcessControl{pid: testPID, host: testHost, alive: map[int]bool{testPID: true}}
}

func (f *fakeProcessControl) PID() int { return f.pid }

func (f *fakeProcessControl) Hostname() (string, error) { return f.host, nil }

func (f *fakeProcessControl) Alive(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive[pid]
}

func (f *fakeProcessControl) Interrupt(pid int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interrupts = append(f.interrupts, pid)
	return nil
}

func (f *fakeProcessControl) setAlive(pid int, alive bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alive[pid] = alive
}

func (f *fakeProcessControl) interrupted() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.interrupts...)
}

func testRunControl(control *fakeProcessControl) runControlDeps {
	return runControlDeps{
		processFactory: func(fs.RuntimeFileService) (processControl, error) { return control, nil },
		pollInterval:   5 * time.Millisecond,
	}
}

func testQwenVariant() *evalv1.ModelVariant {
	return &evalv1.ModelVariant{
		VariantId:      "qwen3-4b",
		ServedModelTag: "qwen3:4b",
		ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ProviderClass:  "ollama",
	}
}

func testRunOperators() []models.OperatorDocumentGo {
	return []models.OperatorDocumentGo{
		{
			ID:                "infer-op",
			OperatorSessionID: testInferenceSession,
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig: &models.RuntimeConfig{
				InferenceEnabled:        true,
				InferenceOllamaEndpoint: "http://provider.example:11434",
			},
		},
		{
			ID:                "data-op",
			OperatorSessionID: testDataSession,
			CurrentHostname:   constants.DataOperatorHostname,
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: false},
		},
	}
}

func writePublicationProofResponse(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case constants.APIPaths.PublicFeedProofs, constants.APIPaths.PublicFeedProofsBatch, constants.APIPaths.PublicFeedProofsPush:
		if r.Method != http.MethodPost {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
		return true
	default:
		return false
	}
}

func writeWitnessPreflightResponse(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.InferenceProviderObservations+"_preflight",
		r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.InferenceModelProvenanceAttestations+"_preflight",
		r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, constants.APIPaths.InferenceModelProvenanceAttestations+"_attest"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
		return true
	default:
		return false
	}
}

// runEnv is one project root wired to a fake Gateway and a fake process
// boundary, with the qwen3:4b variant in its model registry.
type runEnv struct {
	root    string
	deps    nativeEvalDeps
	cmd     *cobra.Command
	control *fakeProcessControl
}

func setupRunEnv(t *testing.T) *runEnv {
	t.Helper()
	root := testutil.TempDir(t)
	writeTestFrozenInventory(t, root, evaluation.DefaultModelInventoryRelPath, testQwenVariant())

	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: testRunOperators()})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeWitnessPreflightResponse(w, r) {
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.Operators {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	paths := config.DefaultPathsConfig()
	paths.Host = server.URL
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	cfg := &config.Config{ProjectRoot: root, RuntimeDir: fileSvc.Resolve(""), Paths: &paths}

	control := newFakeProcessControl()
	fixedNow := time.Unix(1789657337, 0).UTC()
	deps := nativeEvalDeps{
		configLoader: func(string) (*config.Config, error) { return cfg, nil },
		fileSvcFactory: func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			return fs.NewRuntimeFileService(root, slog.Default())
		},
		createRuntimeTree: func(context.Context, fs.RuntimeFileService) error { return nil },
		clientFactory:     harnessclient.New,
		authLoader: func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
			return &auth.ClientAuthContext{UserID: "user-1", CLISessionID: "cli-1", OperatorID: "operator-1"}, nil
		},
		bindClientFactory: func(*config.Config) chatEvalBindClient { return &fakeBindClient{bound: []string{testDataSession}} },
		runControl:        testRunControl(control),
		now:               func() time.Time { return fixedNow },
		newID:             func() string { return "test-id" },
	}

	cmd := cmdtest.SilentCobraCommand()
	cmd.SetContext(context.Background())
	cmd.Flags().String("project-root", root, "")
	return &runEnv{root: root, deps: deps, cmd: cmd, control: control}
}

func (e *runEnv) fileSvc(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	fileSvc, err := e.deps.fileSvcFactory(e.root, slog.Default())
	require.NoError(t, err)
	return fileSvc
}

func (e *runEnv) store(t *testing.T) *evaluation.Store {
	t.Helper()
	return evaluation.NewStore(e.fileSvc(t))
}

// run executes a g8e eval command line and returns what it printed.
func (e *runEnv) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := evalCmdWithConfig(e.deps)
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&out)
	command.SetArgs(append(append([]string{}, args...), "--project-root", e.root))
	err := command.Execute()
	return out.String(), err
}

// runJSON executes a g8e eval command line with the global --json flag and
// decodes its single JSON document into payload.
func (e *runEnv) runJSON(t *testing.T, payload any, args ...string) error {
	t.Helper()
	rootCmd := cmdtest.GlobalJSONRoot(t, evalCmdWithConfig(e.deps))
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(append(append([]string{"eval"}, args...), "--project-root", e.root))
	cmdErr := rootCmd.Execute()
	if out.Len() > 0 {
		decoder := json.NewDecoder(&out)
		require.NoError(t, decoder.Decode(payload), out.String())
	}
	if cmdErr != nil {
		return cmdErr
	}
	return nil
}

func (e *runEnv) mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := e.run(t, args...)
	require.NoError(t, err, out)
	return out
}

// createCampaign freezes a one-model campaign over the registry's qwen3:4b.
func (e *runEnv) createCampaign(t *testing.T, campaignID string) {
	t.Helper()
	e.createCampaignOver(t, campaignID, testQwenVariant())
}

func (e *runEnv) createCampaignOver(t *testing.T, campaignID string, variants ...*evalv1.ModelVariant) {
	t.Helper()
	_, err := createCampaign(context.Background(), e.deps, e.fileSvc(t), campaignCreateSpec{
		CampaignID:  campaignID,
		Variants:    variants,
		Repetitions: 1,
	})
	require.NoError(t, err)
}

// prepareRun creates a campaign and starts a prepared-only run of it: the run
// and its assignment matrix are persisted and nothing executes.
func (e *runEnv) prepareRun(t *testing.T, campaignID, runID string) string {
	t.Helper()
	e.createCampaign(t, campaignID)
	return e.startPrepared(t, campaignID, runID)
}

func (e *runEnv) startPrepared(t *testing.T, campaignID, runID string) string {
	t.Helper()
	e.cmd.SetOut(&bytes.Buffer{})
	result, err := runStartFlow(e.cmd, e.deps, runStartFlowOptions{CampaignID: campaignID, RunID: runID, PrepareOnly: true})
	require.NoError(t, err)
	require.True(t, result.Prepared)
	return result.RunID
}

func writeGatewayCredentials(t *testing.T, e *runEnv) {
	t.Helper()
	fileSvc := e.fileSvc(t)
	cfg, err := e.deps.configLoader(e.root)
	require.NoError(t, err)

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, fileSvc.WriteFile(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CLIKeyFile()), keyPEM, constants.PermFilePrivate))

	certDER := cmdtest.GenerateApproveTestCertDER(t, priv)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, fileSvc.WriteFile(context.Background(), cmdtest.MustRel(t, fileSvc, cfg.CLICertFile()), certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), certPEM, constants.PermFilePrivate))

	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, &auth.Credentials{
		OperatorSessionID: testDataSession,
		UserID:            "user-1",
		OperatorID:        "data-op",
		CLISessionID:      "cli-1",
	}))
}

// enableWitnessGateway marks the Gateway healthy and enrolls CLI credentials so
// runs can preflight provider observation delivery.
func (e *runEnv) enableWitnessGateway(t *testing.T) {
	t.Helper()
	gwremote.WithGatewayHealthCheck(t, true)
	writeGatewayCredentials(t, e)
	e.startGatewayFake(t, true)
}

// withPublishGateway fakes the Gateway public feed and the public mirror.
func (e *runEnv) withPublishGateway(t *testing.T) {
	t.Helper()
	writeGatewayCredentials(t, e)
	e.startGatewayFake(t, false)
}

// withExecuteGateway is withPublishGateway plus operator listing and witness
// preflight, which execution needs.
func (e *runEnv) withExecuteGateway(t *testing.T) {
	t.Helper()
	writeGatewayCredentials(t, e)
	e.startGatewayFake(t, true)
}

func (e *runEnv) startGatewayFake(t *testing.T, executing bool) {
	t.Helper()
	mirrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"snapshot":{"high_water_sequence":0},"recent_projections":[]}`))
	}))
	originalBootstrap := gwremote.PublicMirrorBootstrapURL
	gwremote.PublicMirrorBootstrapURL = mirrorServer.URL

	operators, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: testRunOperators()})
	require.NoError(t, err)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if executing && writeWitnessPreflightResponse(w, r) {
			return
		}
		if writePublicationProofResponse(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.Operators:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(operators)
		case executing && r.Method == http.MethodPost && r.URL.Path == constants.APIPaths.OperatorsCommands:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"transaction_id":"tx-simulated-file"}`))
		case executing && r.Method == http.MethodGet && r.URL.Path == constants.APIPaths.AuditReceipts:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"transactionId":"tx-simulated-file","finalPersistenceAttestation":{"transactionId":"tx-simulated-file"}}`))
		case r.URL.Path == constants.APIPaths.PublicFeedSnapshot:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"high_water_sequence":0}`))
		case r.URL.Path == constants.APIPaths.PublicFeedBatches:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"high_water_sequence":1}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "publication-state"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "publication-state"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"schema_version":"eval-campaign-publication-state/v1","run_id":"run","published_idempotency_keys":[],"last_published_sequence":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	config.SetEndpointOverride(gateway.URL)
	t.Cleanup(func() {
		gateway.Close()
		mirrorServer.Close()
		gwremote.PublicMirrorBootstrapURL = originalBootstrap
		config.SetEndpointOverride("")
	})
}

type recordingCampaignVerificationPublication struct {
	completionRunIDs []string
	reports          []*evalv1.EvaluationVerificationReport
}

func (p *recordingCampaignVerificationPublication) PublishRunCompletion(_ context.Context, runID string, _ time.Time) (int, error) {
	p.completionRunIDs = append(p.completionRunIDs, runID)
	return 1, nil
}

func (p *recordingCampaignVerificationPublication) PublishRunVerification(_ context.Context, _ string, report *evalv1.EvaluationVerificationReport) (int, error) {
	p.reports = append(p.reports, report)
	return 1, nil
}

// recordPublication routes verification publication to a recorder.
func (e *runEnv) recordPublication() *recordingCampaignVerificationPublication {
	publication := &recordingCampaignVerificationPublication{}
	e.deps.campaignPublicationFactory = func(*cobra.Command, fs.RuntimeFileService) (campaignVerificationPublication, error) {
		return publication, nil
	}
	return publication
}

func assertPopulationBoundCampaignReport(t *testing.T, report *evalv1.EvaluationVerificationReport, expectedAssignments, verifiedAssignments uint32) {
	t.Helper()
	require.NotNil(t, report)
	assert.Equal(t, constants.CampaignVerifierVersion, report.GetSchemaVersion())
	assert.Equal(t, constants.CampaignVerifierVersion, report.GetVerifierContractVersion())
	assert.Equal(t, constants.EvaluationSourceVersion, report.GetVerifierReleaseVersion())
	assert.Equal(t, expectedAssignments, report.GetExpectedAssignmentCount())
	assert.Equal(t, verifiedAssignments, report.GetVerifiedAssignmentCount())
	assert.NotEmpty(t, report.GetVerifiedPopulationDigest())
	assert.NotEmpty(t, report.GetCampaignDigest())
	assert.NotEmpty(t, report.GetCatalogDigest())
	assert.NotEmpty(t, report.GetModelRegistryDigest())
	require.NotNil(t, report.GetReportDigestRef())
	assert.Len(t, report.GetReportDigestRef().GetSha256(), 64)
}

func designatedRoleLabel(assignment *evalv1.EvaluationAssignment) string {
	homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous)
	if !ok || homogeneous.Homogeneous == nil {
		return "primary"
	}
	switch homogeneous.Homogeneous.GetDesignatedRole() {
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT:
		return "assistant"
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE:
		return "lite"
	default:
		return "primary"
	}
}

func buildCompletedCampaignTrace(assignment *evalv1.EvaluationAssignment, attemptID, registryDigest, inferenceSessionID string) map[string]any {
	role := designatedRoleLabel(assignment)
	trace := map[string]any{
		"schema_version":    "1",
		"chat_execution_id": "exec-1",
		"status":            "completed",
		"completed_at":      "2026-09-15T00:00:00+00:00",
		"role_outcome":      "invoked",
		"evaluation_context": map[string]any{
			"campaign_id":                assignment.GetCampaignId(),
			"run_id":                     assignment.GetRunId(),
			"assignment_id":              assignment.GetAssignmentId(),
			"evaluation_attempt_id":      attemptID,
			"scenario_id":                assignment.GetScenarioId(),
			"model_registry_digest":      registryDigest,
			"target_operator_session_id": inferenceSessionID,
			"evaluation_lane":            "model_role",
			"designated_model_role":      role,
		},
		"controlled_role_assignment": map[string]any{
			"designated_model_role": role,
		},
		"model_calls": []any{
			map[string]any{
				"agent_role":              "sage",
				"model_role":              role,
				"provider":                "G8EProvider",
				"governed_transaction_id": "tx-1",
				"governed_result_digest":  repeatTestHex('a', 64),
				"provider_attempt_id":     "attempt-1",
				"normalized_request_hash": repeatTestHex('b', 64),
				"output_hash":             repeatTestHex('c', 64),
				"usage_reported":          true,
				"input_tokens":            10,
				"output_tokens":           5,
			},
		},
	}
	digest, err := evaluation.ComputeChatProbeTraceDigest(trace)
	if err != nil {
		panic(fmt.Sprintf("buildCompletedCampaignTrace: %v", err))
	}
	trace["trace_digest"] = digest
	return trace
}

func repeatTestHex(ch byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = ch
	}
	return string(out)
}

func newTestEnsembleServer(t *testing.T, traceFn func(assignmentID, attemptID string) map[string]any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == harnessclient.EnsembleChatPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"case_id":"case-1","investigation_id":"inv-1"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/evaluation/trace/"):
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/evaluation/trace/"), "/")
			if len(parts) != 2 || traceFn == nil {
				http.NotFound(w, r)
				return
			}
			trace := traceFn(parts[0], parts[1])
			if trace == nil {
				http.NotFound(w, r)
				return
			}
			raw, err := json.Marshal(trace)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(harnessclient.EnsembleEvaluationTraceResponse{Trace: raw})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// traceForAnyRun serves a completed trace for whichever run owns the
// assignment.
func (e *runEnv) traceForAnyRun(assignmentID, attemptID string) map[string]any {
	fileSvc, err := e.deps.fileSvcFactory(e.root, slog.Default())
	if err != nil {
		return nil
	}
	store := evaluation.NewStore(fileSvc)
	ctx := context.Background()
	runIDs, err := store.ListRunIDs(ctx)
	if err != nil {
		return nil
	}
	for _, runID := range runIDs {
		assignment, err := store.LoadAssignment(ctx, runID, assignmentID)
		if err != nil {
			continue
		}
		run, err := store.LoadRun(ctx, runID)
		if err != nil {
			return nil
		}
		spec, err := store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
		if err != nil {
			return nil
		}
		return buildCompletedCampaignTrace(assignment, attemptID, spec.GetModelRegistryDigest(), testInferenceSession)
	}
	return nil
}

// firstAssignmentServer claims a run's first queued assignment and returns an
// ensemble server that completes exactly that assignment.
func (e *runEnv) firstAssignmentServer(t *testing.T, runID string) (*httptest.Server, *evalv1.EvaluationAssignment) {
	t.Helper()
	store := e.store(t)
	controller := evaluation.NewCampaignController(store, nil, e.deps.now, func(prefix string) string { return prefix + "-" + e.deps.newID() })
	assignment, ok, err := controller.ResumeNextAssignment(context.Background(), runID)
	require.NoError(t, err)
	require.True(t, ok)
	run, err := store.LoadRun(context.Background(), runID)
	require.NoError(t, err)
	spec, err := store.LoadCampaignSpec(context.Background(), run.GetCampaignBinding().GetCampaignId())
	require.NoError(t, err)
	server := newTestEnsembleServer(t, func(assignmentID, attemptID string) map[string]any {
		if assignmentID != assignment.GetAssignmentId() {
			return nil
		}
		return buildCompletedCampaignTrace(assignment, attemptID, spec.GetModelRegistryDigest(), testInferenceSession)
	})
	return server, assignment
}

func testFormationCatalogCLIVariants() []*evalv1.ModelVariant {
	digestByTag := map[string]byte{
		"qwen3.5:9b":     '1',
		"ministral-3:3b": '2',
		"gemma3:1b":      '3',
		"deepseek-r1:7b": '4',
		"llama3.2:1b":    '5',
		"gemma4:e4b":     '6',
		"qwen2.5:7b":     '7',
		"llama3.1:8b":    '8',
		"phi4-mini:3.8b": '9',
		"qwen3.5:4b":     'b',
		"gemma4:e2b":     'c',
	}
	return evaluation.FormationCatalogFixtureVariants(func(tag string) string {
		ch := digestByTag[tag]
		if ch == 0 {
			ch = 'a'
		}
		return repeatTestHex(ch, 64)
	})
}
