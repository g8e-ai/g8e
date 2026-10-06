// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
		newID:             func() (string, error) { return "test-id", nil },
	}

	cmd := cmdtest.SilentCobraCommand()
	cmd.SetContext(testVersionContext())
	cmd.Flags().String("project-root", root, "")
	return &runEnv{root: root, deps: deps, cmd: cmd, control: control}
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

// newTestEnsembleServer fakes g8ee's chat and trace endpoints. Like g8ee, it
// stores the posted evaluation_context whole in the trace it serves, so the
// seed and workspace a request carried are echoed back and the trace digest
// covers them.
func newTestEnsembleServer(t *testing.T, traceFn func(assignmentID, attemptID string) map[string]any) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	postedContexts := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == harnessclient.EnsembleChatPath:
			var posted struct {
				EvaluationContext map[string]any `json:"evaluation_context"`
			}
			if err := json.NewDecoder(r.Body).Decode(&posted); err == nil && posted.EvaluationContext != nil {
				key := fmt.Sprint(posted.EvaluationContext["assignment_id"]) + "/" + fmt.Sprint(posted.EvaluationContext["evaluation_attempt_id"])
				mu.Lock()
				postedContexts[key] = posted.EvaluationContext
				mu.Unlock()
			}
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
			mu.Lock()
			posted, echoed := postedContexts[parts[0]+"/"+parts[1]]
			mu.Unlock()
			if echoed {
				trace["evaluation_context"] = posted
				digest, err := evaluation.ComputeChatProbeTraceDigest(trace)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				trace["trace_digest"] = digest
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

// firstAssignmentServer claims a run's first queued assignment and returns an
// ensemble server that completes exactly that assignment.
func (e *runEnv) firstAssignmentServer(t *testing.T, runID string) (*httptest.Server, *evalv1.EvaluationAssignment) {
	t.Helper()
	store := e.store(t)
	controller := evaluation.NewCampaignController(store, nil, e.deps.now, adaptNewID(e.deps.newID))
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
