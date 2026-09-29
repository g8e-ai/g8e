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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

type recordingFormationChatClient struct {
	calls         []harnessclient.EnsembleChatRequest
	traces        map[string]EvaluationTrace
	failOnAttempt string
}

func (c *recordingFormationChatClient) EnsembleChat(_ context.Context, _ harnessclient.Persona, req harnessclient.EnsembleChatRequest) (*harnessclient.EnsembleChatResponse, error) {
	if c.failOnAttempt != "" && req.EvaluationContext != nil && req.EvaluationContext.EvaluationAttemptID == c.failOnAttempt {
		return nil, fmt.Errorf("simulated ensemble chat failure for attempt %s", c.failOnAttempt)
	}
	c.calls = append(c.calls, req)
	return &harnessclient.EnsembleChatResponse{CaseID: "case-1", InvestigationID: "inv-1"}, nil
}

func (c *recordingFormationChatClient) GetEvaluationTrace(_ context.Context, _ harnessclient.Persona, _, evaluationAttemptID string) (EvaluationTrace, error) {
	trace, ok := c.traces[evaluationAttemptID]
	if !ok {
		return nil, fmt.Errorf("no trace configured for attempt %s", evaluationAttemptID)
	}
	return trace, nil
}

func heterogeneousFormationTraces(t *testing.T, baseAttempt string, outputs map[FormationRole]string, failRole FormationRole) map[string]EvaluationTrace {
	t.Helper()
	traces := make(map[string]EvaluationTrace, 3)
	for _, role := range []FormationRole{FormationRoleLite, FormationRoleAssistant, FormationRolePrimary} {
		trace := completedHomogeneousTrace(t, string(role))
		if role == failRole {
			trace["status"] = "failed"
		}
		if output, ok := outputs[role]; ok {
			trace["designated_role_output"] = output
		}
		digest, err := ComputeChatProbeTraceDigest(trace)
		require.NoError(t, err)
		trace["trace_digest"] = digest
		traces[baseAttempt+":"+string(role)] = trace
	}
	return traces
}

func heterogeneousFormationRunContext() FormationRunContext {
	return FormationRunContext{
		CampaignID:          "campaign-heterogeneous-1",
		RunID:               "run-heterogeneous-1",
		AssignmentID:        "assignment-heterogeneous-1",
		EvaluationAttemptID: "attempt-heterogeneous-1",
		ScenarioID:          "instruction-exact-format",
		ModelRegistryDigest: "d" + repeatHex('d', 63),
		InferenceSessionID:  "infer-session",
		DataSessionID:       "data-session",
	}
}

func waitForTraceImmediately(ctx context.Context, fetch func(context.Context) (EvaluationTrace, error)) (EvaluationTrace, error) {
	return fetch(ctx)
}

func TestCampaignFormationChatRunner_ExecutesRolesInOrderThroughG8ee(t *testing.T) {
	t.Parallel()
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	outputs := map[FormationRole]string{
		FormationRoleLite:      "lite output",
		FormationRoleAssistant: "assistant output",
		FormationRolePrimary:   "primary output",
	}
	client := &recordingFormationChatClient{traces: heterogeneousFormationTraces(t, "attempt-heterogeneous-1", outputs, "")}
	runner := NewCampaignFormationChatRunner(client, harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"}, "data-op", waitForTraceImmediately, nil)

	initialState, err := BuildFormationInitialState(ScenarioInputFixture{ScenarioID: "instruction-exact-format", UserPrompt: "Reply with exactly: READY"})
	require.NoError(t, err)

	result, err := runner.RunHeterogeneousFormation(context.Background(), FormationBindingRequest{Stack: stack, Variants: variants}, heterogeneousFormationRunContext(), initialState)
	require.NoError(t, err)
	require.Len(t, result.Roles, 3)
	assert.True(t, result.Passed)
	assert.Equal(t, FormationRoleLite, result.Roles[0].Role)
	assert.Equal(t, FormationRoleAssistant, result.Roles[1].Role)
	assert.Equal(t, FormationRolePrimary, result.Roles[2].Role)
	for _, role := range result.Roles {
		assert.NotEmpty(t, role.Trace)
		assert.NotEmpty(t, role.ProviderAttemptID)
	}

	require.Len(t, client.calls, 3)
	wantAttempts := []string{"attempt-heterogeneous-1:lite", "attempt-heterogeneous-1:assistant", "attempt-heterogeneous-1:primary"}
	for index, call := range client.calls {
		require.NotNil(t, call.EvaluationContext)
		assert.Equal(t, "assignment-heterogeneous-1", call.EvaluationContext.AssignmentID, "AssignmentID must stay constant across roles")
		assert.Equal(t, wantAttempts[index], call.EvaluationContext.EvaluationAttemptID)
	}

	// Context threading: each subsequent role's outgoing message carries the
	// prior roles' designated output, cumulatively.
	assert.NotContains(t, client.calls[0].Message, "Prior formation role output")
	assert.Contains(t, client.calls[1].Message, "lite output")
	assert.NotContains(t, client.calls[1].Message, "assistant output")
	assert.Contains(t, client.calls[2].Message, "lite output")
	assert.Contains(t, client.calls[2].Message, "assistant output")
}

func TestCampaignFormationChatRunner_MaterializesSimulatedFilesBeforeFirstRole(t *testing.T) {
	t.Parallel()
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	client := &recordingFormationChatClient{traces: heterogeneousFormationTraces(t, "attempt-heterogeneous-1", nil, "")}
	writer := &fakeSimulatedFileWriter{}
	runner := NewCampaignFormationChatRunner(client, harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"}, "data-op", waitForTraceImmediately, writer)

	simulated := ScenarioSimulatedFile{Kind: "file", Label: "network-summary", Path: "/synthetic/eval/network-summary.txt", Content: "upstream_host=payments.internal.example"}
	initialState, err := BuildFormationInitialState(ScenarioInputFixture{
		ScenarioID:     "instruction-exact-format",
		UserPrompt:     "Reply with exactly: READY",
		SimulatedFiles: []ScenarioSimulatedFile{simulated},
	})
	require.NoError(t, err)

	_, err = runner.RunHeterogeneousFormation(context.Background(), FormationBindingRequest{Stack: stack, Variants: variants}, heterogeneousFormationRunContext(), initialState)
	require.NoError(t, err)
	require.Len(t, writer.written, 1)
	assert.Equal(t, simulated, writer.written[0])
}

func TestCampaignFormationChatRunner_FailsClosedWhenSimulatedFileWriterMissing(t *testing.T) {
	t.Parallel()
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	client := &recordingFormationChatClient{traces: heterogeneousFormationTraces(t, "attempt-heterogeneous-1", nil, "")}
	runner := NewCampaignFormationChatRunner(client, harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"}, "data-op", waitForTraceImmediately, nil)

	initialState, err := BuildFormationInitialState(ScenarioInputFixture{
		ScenarioID: "instruction-exact-format",
		UserPrompt: "Reply with exactly: READY",
		SimulatedFiles: []ScenarioSimulatedFile{
			{Kind: "file", Label: "network-summary", Path: "/synthetic/eval/network-summary.txt", Content: "upstream_host=payments.internal.example"},
		},
	})
	require.NoError(t, err)

	_, err = runner.RunHeterogeneousFormation(context.Background(), FormationBindingRequest{Stack: stack, Variants: variants}, heterogeneousFormationRunContext(), initialState)
	require.Error(t, err)
	assert.Empty(t, client.calls)
}

func TestCampaignFormationChatRunner_StopsOnFirstRoleSubmissionFailure(t *testing.T) {
	t.Parallel()
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	client := &recordingFormationChatClient{
		traces:        heterogeneousFormationTraces(t, "attempt-heterogeneous-1", nil, ""),
		failOnAttempt: "attempt-heterogeneous-1:assistant",
	}
	runner := NewCampaignFormationChatRunner(client, harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"}, "data-op", waitForTraceImmediately, nil)

	initialState, err := BuildFormationInitialState(ScenarioInputFixture{ScenarioID: "instruction-exact-format", UserPrompt: "Reply with exactly: READY"})
	require.NoError(t, err)

	result, err := runner.RunHeterogeneousFormation(context.Background(), FormationBindingRequest{Stack: stack, Variants: variants}, heterogeneousFormationRunContext(), initialState)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Roles, 1, "only Lite should have completed before Assistant's submission failed")
	assert.Equal(t, FormationRoleLite, result.Roles[0].Role)
	assert.Len(t, client.calls, 1, "Primary must never be dispatched once Assistant fails")
}

func TestCampaignFormationChatRunner_StopsOnFailedRoleTrace(t *testing.T) {
	t.Parallel()
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	client := &recordingFormationChatClient{traces: heterogeneousFormationTraces(t, "attempt-heterogeneous-1", nil, FormationRoleAssistant)}
	runner := NewCampaignFormationChatRunner(client, harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"}, "data-op", waitForTraceImmediately, nil)

	initialState, err := BuildFormationInitialState(ScenarioInputFixture{ScenarioID: "instruction-exact-format", UserPrompt: "Reply with exactly: READY"})
	require.NoError(t, err)

	result, err := runner.RunHeterogeneousFormation(context.Background(), FormationBindingRequest{Stack: stack, Variants: variants}, heterogeneousFormationRunContext(), initialState)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Roles, 1, "only Lite should be recorded; Assistant's failed trace stops the pipeline before being appended")
	assert.True(t, strings.Contains(err.Error(), "assistant"))
}
