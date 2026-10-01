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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
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
		calls := []any{g8eeModelCall(role, 1, 0.1, 100, 20)}
		if role == FormationRolePrimary {
			calls = append(calls, g8eeModelCall(role, 2, 0.3, 150, 30))
		}
		trace["model_calls"] = calls
		digest, err := ComputeChatProbeTraceDigest(trace)
		require.NoError(t, err)
		trace["trace_digest"] = digest
		traces[baseAttempt+":"+string(role)] = trace
	}
	return traces
}

// g8eeModelCall is one scored G8EProvider call as g8ee records it in a trace.
func g8eeModelCall(role FormationRole, turn int, ttftSeconds float64, inputTokens, outputTokens int) EvaluationTrace {
	return EvaluationTrace{
		"agent_role":                  "sage",
		"model_role":                  string(role),
		"provider":                    "G8EProvider",
		"succeeded":                   true,
		"provider_attempt_id":         g8eeAttemptID(role, turn),
		"finish_reason":               "stop",
		"usage_reported":              true,
		"input_tokens":                float64(inputTokens),
		"output_tokens":               float64(outputTokens),
		"generation_duration_seconds": 0.5,
		"time_to_first_token_seconds": ttftSeconds,
	}
}

func g8eeAttemptID(role FormationRole, turn int) string {
	return fmt.Sprintf("g8ee-%s-%d", role, turn)
}

// g8eeFormationObserver seeds a provider-boundary window for every g8ee
// provider attempt; Primary's second turn peaks higher than its first.
func g8eeFormationObserver(skip ...string) *stubFormationObservationLoader {
	observer := &stubFormationObservationLoader{windows: map[string]*evalv1.ProviderBoundaryObservationWindow{}}
	for _, id := range []string{g8eeAttemptID(FormationRoleLite, 1), g8eeAttemptID(FormationRoleAssistant, 1), g8eeAttemptID(FormationRolePrimary, 1), g8eeAttemptID(FormationRolePrimary, 2)} {
		if slices.Contains(skip, id) {
			continue
		}
		seedFormationObservationWindow(observer, id)
	}
	if window := observer.windows[g8eeAttemptID(FormationRolePrimary, 2)]; window != nil {
		window.GetSamples()[0].VramUsedBytes = 3 * 1024 * 1024 * 1024
	}
	return observer
}

// g8eeFormationProductionDeps wires the same governed dependencies the
// direct-dispatch runner uses (attestation, allocation, observation, release).
func g8eeFormationProductionDeps(observer *stubFormationObservationLoader, dispatcher *recordingFormationInferenceDispatcher, modelDispatcher *recordingOllamaModelCommandDispatcher) FormationProductionDependencies {
	return FormationProductionDependencies{
		Variants:               testHeterogeneousVariants(),
		ProvenancePreflight:    stubFormationProvenancePreflight{},
		ObservationLoader:      observer,
		InferenceDispatcher:    dispatcher,
		ModelCommandDispatcher: modelDispatcher,
		OllamaEnvironment:      map[string]string{"OLLAMA_HOST": "http://127.0.0.1:11434"},
		NewID:                  func(prefix string) string { return prefix + "-id" },
		Now:                    func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	}
}

func newTestFormationChatRunner(client *recordingFormationChatClient, writer WorkspaceFileWriter, observer *stubFormationObservationLoader) *CampaignFormationChatRunner {
	return NewCampaignFormationChatRunner(client, harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"}, "data-op", waitForTraceImmediately, writer,
		g8eeFormationProductionDeps(observer, &recordingFormationInferenceDispatcher{}, &recordingOllamaModelCommandDispatcher{}))
}

func heterogeneousFormationRunContext() FormationRunContext {
	// Allocation validates the frozen registry binding, so the digest is real.
	registryDigest, err := models.ComputeInferenceModelRegistryDigest("campaign-heterogeneous-1", InferenceVariantsFromEvalRegistry(testHeterogeneousVariants()))
	if err != nil {
		panic(err)
	}
	return FormationRunContext{
		CampaignID:          "campaign-heterogeneous-1",
		RunID:               "run-heterogeneous-1",
		AssignmentID:        "assignment-heterogeneous-1",
		EvaluationAttemptID: "attempt-heterogeneous-1",
		ScenarioID:          "instruction-exact-format",
		ModelRegistryDigest: registryDigest,
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
	dispatcher := &recordingFormationInferenceDispatcher{}
	modelDispatcher := &recordingOllamaModelCommandDispatcher{}
	runner := NewCampaignFormationChatRunner(client, harnessclient.Persona{ID: "campaign-cli", UserID: "user-1", CLISessionID: "cli-1"}, "data-op", waitForTraceImmediately, nil,
		g8eeFormationProductionDeps(g8eeFormationObserver(), dispatcher, modelDispatcher))

	initialState, err := BuildFormationInitialState(ScenarioInputFixture{ScenarioID: "instruction-exact-format", UserPrompt: "Reply with exactly: READY"})
	require.NoError(t, err)

	result, err := runner.RunHeterogeneousFormation(context.Background(), FormationBindingRequest{Stack: stack, Variants: variants}, heterogeneousFormationRunContext(), initialState)
	require.NoError(t, err)
	require.Len(t, result.Roles, 3)
	assert.True(t, result.Passed)

	// Per-formation witness metrics are recorded exactly as on the direct path:
	// storage attestation, co-resident allocation and release, an observer
	// window per role, and peak VRAM spanning every provider attempt.
	for _, role := range result.Roles {
		assert.True(t, role.AttestationVerified, role.Role)
		assert.Equal(t, role.Model.ModelDigest, role.AttestationDigest, role.Role)
		require.NotNil(t, role.ObserverEvidence, role.Role)
		assert.Equal(t, role.ProviderAttemptID, role.ObserverEvidence.Window.GetProviderAttemptId(), role.Role)
		assert.Equal(t, g8eeAttemptID(role.Role, 1), role.ProviderAttemptID)
	}
	assert.Len(t, dispatcher.requests, 3, "each sovereign model is warmed (allocated) before any role runs")
	assert.NotEmpty(t, modelDispatcher.requests, "allocated models are released after the formation")
	primary := result.Roles[2]
	assert.Equal(t, uint64(1024), result.Roles[0].PeakVRAMMiB)
	assert.Equal(t, uint64(3072), primary.PeakVRAMMiB, "Primary's second turn peaked higher")
	assert.Equal(t, uint64(3072), result.PeakVRAMMiB)
	require.Len(t, primary.ObserverEvidence.AdditionalWindows, 1)
	assert.Equal(t, g8eeAttemptID(FormationRolePrimary, 2), primary.ObserverEvidence.AdditionalWindows[0].GetProviderAttemptId())
	assert.Equal(t, uint32(250), primary.PromptTokens, "tokens summed across Primary's two turns")
	assert.Equal(t, uint64(100_000_000), primary.TTFTNanos)
	assert.True(t, result.MutationIntercepted)
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

func TestCampaignFormationChatRunner_FailsClosedWhenWorkspaceFileWriterMissing(t *testing.T) {
	t.Parallel()
	variants := testHeterogeneousVariants()
	stack := mustHeterogeneousStack(t)
	client := &recordingFormationChatClient{traces: heterogeneousFormationTraces(t, "attempt-heterogeneous-1", nil, "")}
	runner := newTestFormationChatRunner(client, nil, g8eeFormationObserver())

	initialState, err := BuildFormationInitialState(ScenarioInputFixture{
		ScenarioID: "instruction-exact-format",
		UserPrompt: "Reply with exactly: READY",
		WorkspaceFiles: []ScenarioWorkspaceFile{
			{Label: "network-summary", RelPath: "net/network-summary.txt", Content: "upstream_host=payments.internal.example"},
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
	runner := newTestFormationChatRunner(client, nil, g8eeFormationObserver())

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
	runner := newTestFormationChatRunner(client, nil, g8eeFormationObserver())

	initialState, err := BuildFormationInitialState(ScenarioInputFixture{ScenarioID: "instruction-exact-format", UserPrompt: "Reply with exactly: READY"})
	require.NoError(t, err)

	result, err := runner.RunHeterogeneousFormation(context.Background(), FormationBindingRequest{Stack: stack, Variants: variants}, heterogeneousFormationRunContext(), initialState)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Roles, 1, "only Lite should be recorded; Assistant's failed trace stops the pipeline before being appended")
	assert.True(t, strings.Contains(err.Error(), "assistant"))
}

func TestFormationRoleResultFromTrace_AggregatesEveryProviderCall(t *testing.T) {
	trace := EvaluationTrace{"model_calls": []any{
		EvaluationTrace{"provider": "OpenAIProvider", "provider_attempt_id": "ignored", "input_tokens": float64(999)},
		g8eeModelCall(FormationRolePrimary, 1, 0.25, 120, 40),
		EvaluationTrace{"provider": "G8EProvider", "provider_attempt_id": "failed-turn", "succeeded": false},
		g8eeModelCall(FormationRolePrimary, 2, 0.75, 80, 60),
	}}

	result := formationRoleResultFromTrace(trace)

	assert.Equal(t, g8eeAttemptID(FormationRolePrimary, 1), result.ProviderAttemptID)
	assert.Equal(t, []string{g8eeAttemptID(FormationRolePrimary, 2)}, result.AdditionalProviderAttemptIDs)
	assert.Equal(t, uint32(200), result.PromptTokens)
	assert.Equal(t, uint32(100), result.GenerationTokens)
	assert.Equal(t, uint64(1_000_000_000), result.GenerationDurationNanos)
	assert.Equal(t, uint64(250_000_000), result.TTFTNanos, "TTFT is the first call's")
	assert.Equal(t, evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED, result.UsageAvailability)
	assert.Equal(t, trace, result.Trace)
}

func TestCampaignFormationChatRunner_FailsWithoutObserverWindowForEveryAttempt(t *testing.T) {
	t.Parallel()
	client := &recordingFormationChatClient{traces: heterogeneousFormationTraces(t, "attempt-heterogeneous-1", nil, "")}
	runner := newTestFormationChatRunner(client, nil, g8eeFormationObserver(g8eeAttemptID(FormationRolePrimary, 2)))
	initialState, err := BuildFormationInitialState(ScenarioInputFixture{ScenarioID: "instruction-exact-format", UserPrompt: "Reply with exactly: READY"})
	require.NoError(t, err)

	result, err := runner.RunHeterogeneousFormation(context.Background(), FormationBindingRequest{Stack: mustHeterogeneousStack(t), Variants: testHeterogeneousVariants()}, heterogeneousFormationRunContext(), initialState)

	require.Error(t, err)
	assert.ErrorContains(t, err, "observer finalize primary attempt "+g8eeAttemptID(FormationRolePrimary, 2))
	assert.False(t, result.Passed)
	assert.Len(t, result.Roles, 2, "Primary is not recorded without witness coverage for its second turn")
}
