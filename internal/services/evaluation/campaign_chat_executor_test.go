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
