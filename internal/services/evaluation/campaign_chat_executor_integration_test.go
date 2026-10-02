// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// harnessChatClient adapts the real harness HTTP client to CampaignChatClient,
// as the eval command does.
type harnessChatClient struct{ client *harnessclient.Client }

func (c harnessChatClient) EnsembleChat(ctx context.Context, persona harnessclient.Persona, req harnessclient.EnsembleChatRequest) (*harnessclient.EnsembleChatResponse, error) {
	return c.client.EnsembleChat(ctx, persona, req)
}

func (c harnessChatClient) GetEvaluationTrace(ctx context.Context, persona harnessclient.Persona, assignmentID, attemptID string) (EvaluationTrace, error) {
	raw, err := c.client.GetEvaluationTrace(ctx, persona, assignmentID, attemptID)
	if err != nil {
		return nil, err
	}
	return DecodeEvaluationTrace(raw)
}

// g8eeEmulator stands in for g8ee, which is an external process. It records
// every request exactly as it arrived on the wire.
type g8eeEmulator struct {
	server     *httptest.Server
	timeline   *orderedWorkspaceEvents
	mu         sync.Mutex
	chatBodies [][]byte
	chatHeader http.Header
	chatStatus int
	trace      EvaluationTrace
}

func newG8eeEmulator(t *testing.T, timeline *orderedWorkspaceEvents, trace EvaluationTrace) *g8eeEmulator {
	t.Helper()
	emulator := &g8eeEmulator{timeline: timeline, chatStatus: http.StatusOK, trace: trace}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+harnessclient.EnsembleChatPath, func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		_, _ = body.ReadFrom(r.Body)
		emulator.timeline.record("chat")
		emulator.mu.Lock()
		emulator.chatBodies = append(emulator.chatBodies, body.Bytes())
		emulator.chatHeader = r.Header.Clone()
		status := emulator.chatStatus
		emulator.mu.Unlock()
		if status != http.StatusOK {
			http.Error(w, `{"detail":"evaluation_context.seed: rejected"}`, status)
			return
		}
		_ = json.NewEncoder(w).Encode(harnessclient.EnsembleChatResponse{Success: true, CaseID: "case-1", InvestigationID: "inv-1"})
	})
	mux.HandleFunc("GET /api/v1/evaluation/trace/{assignment}/{attempt}", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(harnessclient.EnsembleEvaluationTraceResponse{Trace: mustJSON(t, emulator.trace)})
	})
	emulator.server = httptest.NewServer(mux)
	t.Cleanup(emulator.server.Close)
	return emulator
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

func (e *g8eeEmulator) chatRequests(t *testing.T) []harnessclient.EnsembleChatRequest {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	requests := make([]harnessclient.EnsembleChatRequest, 0, len(e.chatBodies))
	for _, body := range e.chatBodies {
		var request harnessclient.EnsembleChatRequest
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		require.NoError(t, decoder.Decode(&request), "the wire body decodes into the harness contract with no unknown field")
		requests = append(requests, request)
	}
	return requests
}

func newWireContractExecutor(t *testing.T, emulator *g8eeEmulator, writer WorkspaceFileWriter, traceStore CampaignTraceStore) *CampaignChatExecutor {
	t.Helper()
	client, err := harnessclient.New(harnessconfig.Config{EnsembleBaseURL: emulator.server.URL})
	require.NoError(t, err)
	return newTestCampaignChatExecutor(t, harnessChatClient{client: client}, traceStore, writer)
}

// TestCampaignChatExecutor_WireContract sends one seeded scenario through the
// real harness client and checks what g8ee would receive: a new case, the frozen
// seed and the attempt-scoped workspace byte for byte, exactly one bound
// Operator, and the fixture files written before the scored request.
func TestCampaignChatExecutor_WireContract(t *testing.T) {
	f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
	require.NotEmpty(t, f.req.ScenarioInput.WorkspaceFiles, "the scenario ships fixture files")
	timeline := &orderedWorkspaceEvents{}
	writer := &recordingWorkspaceFileWriter{timeline: timeline}
	emulator := newG8eeEmulator(t, timeline, f.trace)
	executor := newWireContractExecutor(t, emulator, writer, &stubCampaignTraceStore{})

	result, err := executor.ExecuteAssignment(context.Background(), f.req)

	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, result.GetLifecycleStatus())

	events := timeline.snapshot()
	require.Len(t, events, len(f.req.ScenarioInput.WorkspaceFiles)+1)
	assert.Equal(t, "chat", events[len(events)-1], "every fixture file is written before the scored request")
	assert.Len(t, writer.writes, len(f.req.ScenarioInput.WorkspaceFiles))

	requests := emulator.chatRequests(t)
	require.Len(t, requests, 1)
	request := requests[0]
	require.NotNil(t, request.ResourceCreation)
	assert.True(t, request.ResourceCreation.CreateCase, "a seed is accepted only with create_case (R3)")
	require.NotNil(t, request.EvaluationContext)
	require.NotNil(t, request.EvaluationContext.Workspace)
	assert.Equal(t, f.ws.Root, request.EvaluationContext.Workspace.Root)
	assert.Equal(t, f.ws.OperatorWorkingDirectory, request.EvaluationContext.Workspace.OperatorWorkingDirectory)

	wantSeed := buildHarnessInvestigationSeed(&f.req.ScenarioInput.Seed, &f.ws)
	require.NotNil(t, wantSeed)
	require.NotNil(t, request.EvaluationContext.Seed)
	assert.JSONEq(t, string(mustJSON(t, wantSeed)), string(mustJSON(t, request.EvaluationContext.Seed)))
	assert.Equal(t, f.req.ScenarioInput.Seed.CaseTitle, request.EvaluationContext.Seed.CaseTitle)
	assert.NotContains(t, strings.ToLower(request.Message), "eval", "nothing model-visible says evaluation (R4)")

	require.Len(t, request.Context.BoundOperators, 1)
	assert.Equal(t, "data-op", request.Context.BoundOperators[0].OperatorID)
	assert.Equal(t, "data-session", request.Context.BoundOperators[0].OperatorSessionID)
	assert.Equal(t, "user-1", emulator.chatHeader.Get(harnessclient.HeaderProxyUserID))
}

// A rejection from g8ee (an HTTP 400 for a bad seed, for example) is a failed
// execution of the harness, never a scored result that says the model failed.
func TestCampaignChatExecutor_RejectedChatIsAnExecutionErrorNotAModelFailure(t *testing.T) {
	f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
	timeline := &orderedWorkspaceEvents{}
	emulator := newG8eeEmulator(t, timeline, f.trace)
	emulator.chatStatus = http.StatusBadRequest
	traceStore := &stubCampaignTraceStore{}
	executor := newWireContractExecutor(t, emulator, &recordingWorkspaceFileWriter{timeline: timeline}, traceStore)

	result, err := executor.ExecuteAssignment(context.Background(), f.req)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, constants.ErrEvaluationAssignmentExecutionFailed)
	assert.ErrorContains(t, err, "status 400")
	assert.Empty(t, traceStore.bodies, "no trace evidence is persisted for a request g8ee refused")
}

// The imported result persists through the real runtime file service and reads
// back with its digest, trajectory outcome, and private and public reasons.
func TestCampaignChatExecutor_ResultPersistsThroughTheRuntimeFileServiceAndVerifiesItsDigest(t *testing.T) {
	f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
	timeline := &orderedWorkspaceEvents{}
	emulator := newG8eeEmulator(t, timeline, f.trace)
	executor := newWireContractExecutor(t, emulator, &recordingWorkspaceFileWriter{timeline: timeline}, &stubCampaignTraceStore{})

	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	store := NewStore(fileSvc)

	result, err := executor.ExecuteAssignment(context.Background(), f.req)
	require.NoError(t, err)
	require.NoError(t, store.SaveAssignmentResult(context.Background(), result))

	loaded, err := store.LoadAssignmentResult(context.Background(), result.GetRunId(), result.GetAssignmentId())
	require.NoError(t, err)
	require.NoError(t, ValidateAssignmentResultDigest(loaded))
	assert.Equal(t, result.GetResultDigest(), loaded.GetResultDigest())
	assert.Equal(t, evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_RECOVERED, loaded.GetTrajectoryOutcome())
	assert.Equal(t, result.GetFailureReason(), loaded.GetFailureReason())
	assert.Equal(t, result.GetPublicFailureReason(), loaded.GetPublicFailureReason())
}
