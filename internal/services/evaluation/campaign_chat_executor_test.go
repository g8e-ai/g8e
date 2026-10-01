// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type stubCampaignChatClient struct {
	trace EvaluationTrace
}

func (s *stubCampaignChatClient) EnsembleChat(_ context.Context, _ harnessclient.Persona, _ harnessclient.EnsembleChatRequest) (*harnessclient.EnsembleChatResponse, error) {
	return &harnessclient.EnsembleChatResponse{CaseID: "case-1", InvestigationID: "inv-1"}, nil
}

func (s *stubCampaignChatClient) GetEvaluationTrace(_ context.Context, _ harnessclient.Persona, _, _ string) (EvaluationTrace, error) {
	return s.trace, nil
}

type stubCampaignTraceStore struct {
	bodies [][]byte
}

func (s *stubCampaignTraceStore) SaveAssignmentTrace(_ context.Context, _, _ string, body []byte) error {
	s.bodies = append(s.bodies, body)
	return nil
}

func TestCampaignChatExecutor_ImportsCompletedTrace(t *testing.T) {
	t.Parallel()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	traceStore := &stubCampaignTraceStore{}
	client := &stubCampaignChatClient{trace: completedHomogeneousTrace(t, "primary")}
	executor := NewCampaignChatExecutor(
		client,
		harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"},
		"data-op",
		"data-session",
		traceStore,
		func(ctx context.Context, fetch func(context.Context) (EvaluationTrace, error)) (EvaluationTrace, error) {
			return fetch(ctx)
		},
		nil,
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-1" },
	)
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	result, err := executor.ExecuteAssignment(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED, result.GetLifecycleStatus())
	assert.Len(t, traceStore.bodies, 1)
	require.NoError(t, store.SaveAssignmentResult(context.Background(), result))
}

// orderedWorkspaceEvents is the shared timeline a recording writer and a
// recording chat client append to, so a test can prove fixture files exist on
// the operator before the scored chat request is sent.
type orderedWorkspaceEvents struct {
	mu     sync.Mutex
	events []string
}

func (o *orderedWorkspaceEvents) record(event string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, event)
}

func (o *orderedWorkspaceEvents) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.events...)
}

type workspaceFileWrite struct {
	Target     Target
	RunID      string
	ScenarioID string
	AttemptID  string
	AbsPath    string
	Content    string
}

type recordingWorkspaceFileWriter struct {
	timeline   *orderedWorkspaceEvents
	writes     []workspaceFileWrite
	failOnPath string
}

func (w *recordingWorkspaceFileWriter) WriteWorkspaceFile(_ context.Context, target Target, runID, scenarioID, attemptID, absPath, content string) error {
	if w.timeline != nil {
		w.timeline.record("write:" + absPath)
	}
	if absPath == w.failOnPath {
		return fmt.Errorf("simulated governed write failure for %s", absPath)
	}
	w.writes = append(w.writes, workspaceFileWrite{Target: target, RunID: runID, ScenarioID: scenarioID, AttemptID: attemptID, AbsPath: absPath, Content: content})
	return nil
}

type timelineCampaignChatClient struct {
	stubCampaignChatClient
	timeline *orderedWorkspaceEvents
	requests []harnessclient.EnsembleChatRequest
}

func (c *timelineCampaignChatClient) EnsembleChat(ctx context.Context, persona harnessclient.Persona, req harnessclient.EnsembleChatRequest) (*harnessclient.EnsembleChatResponse, error) {
	c.timeline.record("chat")
	c.requests = append(c.requests, req)
	return c.stubCampaignChatClient.EnsembleChat(ctx, persona, req)
}

func newTestCampaignChatExecutor(t *testing.T, client CampaignChatClient, traceStore CampaignTraceStore, writer WorkspaceFileWriter) *CampaignChatExecutor {
	t.Helper()
	return NewCampaignChatExecutor(
		client,
		harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"},
		"data-op",
		"data-session",
		traceStore,
		func(ctx context.Context, fetch func(context.Context) (EvaluationTrace, error)) (EvaluationTrace, error) {
			return fetch(ctx)
		},
		writer,
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-1" },
	)
}

func workspaceScenarioRequest(t *testing.T) AssignmentExecutionRequest {
	t.Helper()
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	req.ScenarioInput.UserPrompt = "Read " + ScenarioWorkspaceToken + "/config/retry-config.env and report the value of retry_limit."
	req.ScenarioInput.WorkspaceFiles = []ScenarioWorkspaceFile{
		{Label: "retry-config", RelPath: "config/retry-config.env", Content: "retry_limit=3\nbackoff_seconds=5"},
		{Label: "retry-config-backup", RelPath: "config/retry-config.env.bak", Content: "retry_limit=9", Decoy: true},
		{Label: "readme", RelPath: "notes/readme.txt", Content: "rotation runbook"},
	}
	return req
}

// TestCampaignChatExecutor_MaterializesEveryWorkspaceFileBeforeChat guards the
// fixture contract: every workspace file, decoys included, is written under
// the attempt-scoped root on the bound Data Operator before the scored chat
// request is sent, and the request carries that same root.
func TestCampaignChatExecutor_MaterializesEveryWorkspaceFileBeforeChat(t *testing.T) {
	t.Parallel()
	timeline := &orderedWorkspaceEvents{}
	writer := &recordingWorkspaceFileWriter{timeline: timeline}
	client := &timelineCampaignChatClient{stubCampaignChatClient: stubCampaignChatClient{trace: completedHomogeneousTrace(t, "primary")}, timeline: timeline}
	executor := newTestCampaignChatExecutor(t, client, &stubCampaignTraceStore{}, writer)
	req := workspaceScenarioRequest(t)
	ws, err := NewScenarioWorkspace(req.Binding.DataOperatorWorkingDirectory, req.Assignment.GetRunId(), req.AttemptID)
	require.NoError(t, err)

	_, err = executor.ExecuteAssignment(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"write:" + ws.Root + "/config/retry-config.env",
		"write:" + ws.Root + "/config/retry-config.env.bak",
		"write:" + ws.Root + "/notes/readme.txt",
		"chat",
	}, timeline.snapshot())
	require.Len(t, writer.writes, 3)
	for _, write := range writer.writes {
		assert.Equal(t, Target{OperatorID: "data-op", SessionID: "data-session"}, write.Target)
		assert.Equal(t, req.Assignment.GetRunId(), write.RunID)
		assert.Equal(t, req.Assignment.GetScenarioId(), write.ScenarioID)
		assert.Equal(t, req.AttemptID, write.AttemptID)
	}
	assert.Equal(t, "retry_limit=3\nbackoff_seconds=5", writer.writes[0].Content)
	assert.Equal(t, "retry_limit=9", writer.writes[1].Content, "decoys are written too, so the model must pick the right file")
	require.Len(t, client.requests, 1)
	chatReq := client.requests[0]
	require.NotNil(t, chatReq.EvaluationContext.Workspace)
	assert.Equal(t, ws.Root, chatReq.EvaluationContext.Workspace.Root)
	assert.Equal(t, ws.OperatorWorkingDirectory, chatReq.EvaluationContext.Workspace.OperatorWorkingDirectory)
	assert.Contains(t, chatReq.Message, "Read "+ws.Root+"/config/retry-config.env")
	assert.NotContains(t, chatReq.Message, ScenarioWorkspaceToken)
}

// TestCampaignChatExecutor_StopsBeforeChatWhenWorkspaceFileWriteFails ensures a
// fixture that could not be written is an execution error, never a scored
// assignment whose model searched an empty or partial workspace.
func TestCampaignChatExecutor_StopsBeforeChatWhenWorkspaceFileWriteFails(t *testing.T) {
	t.Parallel()
	timeline := &orderedWorkspaceEvents{}
	req := workspaceScenarioRequest(t)
	ws, err := NewScenarioWorkspace(req.Binding.DataOperatorWorkingDirectory, req.Assignment.GetRunId(), req.AttemptID)
	require.NoError(t, err)
	writer := &recordingWorkspaceFileWriter{timeline: timeline, failOnPath: ws.Root + "/config/retry-config.env.bak"}
	client := &timelineCampaignChatClient{stubCampaignChatClient: stubCampaignChatClient{trace: completedHomogeneousTrace(t, "primary")}, timeline: timeline}
	traceStore := &stubCampaignTraceStore{}
	executor := newTestCampaignChatExecutor(t, client, traceStore, writer)

	_, err = executor.ExecuteAssignment(context.Background(), req)

	require.ErrorIs(t, err, constants.ErrEvaluationAssignmentExecutionFailed)
	assert.ErrorContains(t, err, "retry-config.env.bak")
	assert.Equal(t, []string{
		"write:" + ws.Root + "/config/retry-config.env",
		"write:" + ws.Root + "/config/retry-config.env.bak",
	}, timeline.snapshot(), "no chat request and no further writes after the failure")
	assert.Empty(t, traceStore.bodies)
}

// TestCampaignChatExecutor_FailsClosedWhenWorkspaceFileWriterMissing ensures a
// scenario that requires workspace files is never silently run against a
// Data Operator that lacks the fixture content: without a configured writer,
// execution must fail before the chat request reaches the model rather than
// let the model's tool call fail with a misleading missing-path error.
func TestCampaignChatExecutor_FailsClosedWhenWorkspaceFileWriterMissing(t *testing.T) {
	t.Parallel()
	timeline := &orderedWorkspaceEvents{}
	client := &timelineCampaignChatClient{stubCampaignChatClient: stubCampaignChatClient{trace: completedHomogeneousTrace(t, "primary")}, timeline: timeline}
	traceStore := &stubCampaignTraceStore{}
	executor := newTestCampaignChatExecutor(t, client, traceStore, nil)

	_, err := executor.ExecuteAssignment(context.Background(), workspaceScenarioRequest(t))

	require.ErrorIs(t, err, constants.ErrEvaluationWorkspaceUnavailable)
	assert.Empty(t, timeline.snapshot(), "the chat request must not be sent")
	assert.Empty(t, traceStore.bodies)
}

// TestCampaignChatExecutor_FailsClosedWithoutOperatorWorkingDirectory ensures
// an operator that never reported its working directory cannot be given a
// workspace: the assignment fails before anything is written or sent.
func TestCampaignChatExecutor_FailsClosedWithoutOperatorWorkingDirectory(t *testing.T) {
	t.Parallel()
	timeline := &orderedWorkspaceEvents{}
	writer := &recordingWorkspaceFileWriter{timeline: timeline}
	client := &timelineCampaignChatClient{stubCampaignChatClient: stubCampaignChatClient{trace: completedHomogeneousTrace(t, "primary")}, timeline: timeline}
	executor := newTestCampaignChatExecutor(t, client, &stubCampaignTraceStore{}, writer)
	req := workspaceScenarioRequest(t)
	req.Binding.DataOperatorWorkingDirectory = ""

	_, err := executor.ExecuteAssignment(context.Background(), req)

	require.ErrorIs(t, err, constants.ErrEvaluationWorkspaceUnavailable)
	assert.Empty(t, timeline.snapshot())
}

// TestCampaignChatExecutor_SkipsWriterForScenarioWithoutWorkspaceFiles keeps
// the writer optional for answer-only scenarios.
func TestCampaignChatExecutor_SkipsWriterForScenarioWithoutWorkspaceFiles(t *testing.T) {
	t.Parallel()
	timeline := &orderedWorkspaceEvents{}
	client := &timelineCampaignChatClient{stubCampaignChatClient: stubCampaignChatClient{trace: completedHomogeneousTrace(t, "primary")}, timeline: timeline}
	executor := newTestCampaignChatExecutor(t, client, &stubCampaignTraceStore{}, nil)

	_, err := executor.ExecuteAssignment(context.Background(), homogeneousAssignmentExecutionRequest(t, "primary"))

	require.NoError(t, err)
	assert.Equal(t, []string{"chat"}, timeline.snapshot())
}
