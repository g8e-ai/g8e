// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

var errExecutorTestCause = errors.New("injected cause")

// scriptedChatClient fails the stage a test selects and records which stages ran.
type scriptedChatClient struct {
	chatErr  error
	traceErr error
	trace    EvaluationTrace
	stages   []string
}

func (c *scriptedChatClient) EnsembleChat(_ context.Context, _ harnessclient.Persona, _ harnessclient.EnsembleChatRequest) (*harnessclient.EnsembleChatResponse, error) {
	c.stages = append(c.stages, "chat")
	if c.chatErr != nil {
		return nil, c.chatErr
	}
	return &harnessclient.EnsembleChatResponse{CaseID: "case-1", InvestigationID: "inv-1"}, nil
}

func (c *scriptedChatClient) GetEvaluationTrace(_ context.Context, _ harnessclient.Persona, _, _ string) (EvaluationTrace, error) {
	c.stages = append(c.stages, "trace")
	return c.trace, c.traceErr
}

type failingTraceStore struct{ err error }

func (s failingTraceStore) SaveAssignmentTrace(context.Context, string, string, []byte) error {
	return s.err
}

func executorWith(client CampaignChatClient, traceStore CampaignTraceStore, waiter CampaignTraceWaiter) *CampaignChatExecutor {
	return NewCampaignChatExecutor(
		client,
		harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"},
		"data-op", "data-session", traceStore, waiter, nil,
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		func(prefix string) string { return prefix + "-1" },
	)
}

func fetchOnce(ctx context.Context, fetch func(context.Context) (EvaluationTrace, error)) (EvaluationTrace, error) {
	return fetch(ctx)
}

func TestCampaignChatExecutor_FailuresAreRecoverableExecutionErrors(t *testing.T) {
	t.Parallel()
	good := completedHomogeneousTrace(t, "primary")
	tests := []struct {
		name       string
		client     *scriptedChatClient
		traceStore CampaignTraceStore
		waiter     CampaignTraceWaiter
		wantStages []string
		wantErr    string
	}{
		{
			name: "the chat request is rejected", client: &scriptedChatClient{chatErr: errExecutorTestCause}, waiter: fetchOnce,
			wantStages: []string{"chat"}, wantErr: "submit chat",
		},
		{
			name: "the trace cannot be fetched", client: &scriptedChatClient{traceErr: errExecutorTestCause}, waiter: fetchOnce,
			wantStages: []string{"chat", "trace"}, wantErr: "wait for trace",
		},
		{
			name: "the trace evidence cannot be persisted", client: &scriptedChatClient{trace: good}, waiter: fetchOnce,
			traceStore: failingTraceStore{err: errExecutorTestCause}, wantStages: []string{"chat", "trace"}, wantErr: "persist trace evidence",
		},
		{
			name: "the trace is not a completed attempt of this request", client: &scriptedChatClient{trace: EvaluationTrace{"status": "completed"}}, waiter: fetchOnce,
			wantStages: []string{"chat", "trace"}, wantErr: "import assignment result from trace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := executorWith(tt.client, tt.traceStore, tt.waiter).ExecuteAssignment(context.Background(), homogeneousAssignmentExecutionRequest(t, "primary"))

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, constants.ErrEvaluationAssignmentExecutionFailed)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Equal(t, tt.wantStages, tt.client.stages)
			if tt.client.chatErr != nil || tt.client.traceErr != nil || tt.traceStore != nil {
				assert.ErrorIs(t, err, errExecutorTestCause, "the cause is preserved")
			}
		})
	}
}

// A missing trace waiter is a wiring defect, not a transient execution failure,
// so it must not be retried as one.
func TestCampaignChatExecutor_MissingTraceWaiterIsAWiringErrorNotARecoverableOne(t *testing.T) {
	t.Parallel()
	client := &scriptedChatClient{trace: completedHomogeneousTrace(t, "primary")}

	_, err := executorWith(client, nil, nil).ExecuteAssignment(context.Background(), homogeneousAssignmentExecutionRequest(t, "primary"))

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
	assert.NotErrorIs(t, err, constants.ErrEvaluationAssignmentExecutionFailed)
	assert.Equal(t, []string{"chat"}, client.stages)
}

func TestCampaignChatExecutor_WithoutAClientIsAnError(t *testing.T) {
	t.Parallel()
	var nilExecutor *CampaignChatExecutor
	_, err := nilExecutor.ExecuteAssignment(context.Background(), AssignmentExecutionRequest{})
	require.Error(t, err)

	_, err = executorWith(nil, nil, fetchOnce).ExecuteAssignment(context.Background(), AssignmentExecutionRequest{})
	require.Error(t, err)
}

func TestCampaignChatExecutor_ProgressCallbackReplacesTheConfiguredWaiter(t *testing.T) {
	t.Parallel()
	client := &scriptedChatClient{trace: completedHomogeneousTrace(t, "primary")}
	progress := 0
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	req.OnTraceProgress = func(context.Context, EvaluationTrace) error {
		progress++
		return nil
	}

	result, err := executorWith(client, nil, nil).ExecuteAssignment(context.Background(), req)

	require.NoError(t, err, "a progress callback needs no separate waiter")
	require.NotNil(t, result)
	assert.Equal(t, []string{"chat", "trace"}, client.stages)
}
