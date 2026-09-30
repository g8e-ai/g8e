// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

type fakeSimulatedFileWriter struct {
	written []ScenarioSimulatedFile
	err     error
}

func (f *fakeSimulatedFileWriter) WriteSimulatedFile(_ context.Context, _ Target, _, _, _ string, file ScenarioSimulatedFile) error {
	if f.err != nil {
		return f.err
	}
	f.written = append(f.written, file)
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

// TestCampaignChatExecutor_MaterializesSimulatedFilesBeforeChat guards the fix
// for the gap where ScenarioSimulatedFile content was frozen fixture data
// with no executor behind it: a tool-selection/tool-argument scenario's
// simulated file must be written to the bound Data Operator before the chat
// request is dispatched, not after and not never.
func TestCampaignChatExecutor_MaterializesSimulatedFilesBeforeChat(t *testing.T) {
	t.Parallel()
	traceStore := &stubCampaignTraceStore{}
	client := &stubCampaignChatClient{trace: completedHomogeneousTrace(t, "primary")}
	writer := &fakeSimulatedFileWriter{}
	executor := NewCampaignChatExecutor(
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
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	simulated := ScenarioSimulatedFile{Kind: "file", Label: "network-summary", Path: "/synthetic/eval/network-summary.txt", Content: "upstream_host=payments.internal.example"}
	req.ScenarioInput.SimulatedFiles = []ScenarioSimulatedFile{simulated}

	_, err := executor.ExecuteAssignment(context.Background(), req)

	require.NoError(t, err)
	require.Len(t, writer.written, 1)
	assert.Equal(t, simulated, writer.written[0])
}

// TestCampaignChatExecutor_FailsClosedWhenSimulatedFileWriterMissing ensures a
// scenario that requires a simulated file is never silently run against a
// Data Operator that lacks the fixture content: without a configured writer,
// execution must fail before the chat request reaches the model rather than
// let the model's tool call fail with a misleading missing-path error.
func TestCampaignChatExecutor_FailsClosedWhenSimulatedFileWriterMissing(t *testing.T) {
	t.Parallel()
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
	req.ScenarioInput.SimulatedFiles = []ScenarioSimulatedFile{{Kind: "file", Label: "network-summary", Path: "/synthetic/eval/network-summary.txt", Content: "upstream_host=payments.internal.example"}}

	_, err := executor.ExecuteAssignment(context.Background(), req)

	require.Error(t, err)
	assert.Empty(t, traceStore.bodies)
}
