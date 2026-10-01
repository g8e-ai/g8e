// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// fakeCanaryChat plays g8ee: it records the chat request and answers with the
// trace g8ee would persist for it. Each field a canary inspects can be bent.
type fakeCanaryChat struct {
	submitErr      error
	request        harnessclient.EnsembleChatRequest
	toolGate       string
	dropTool       string
	omitDeclared   bool
	seedApplied    map[string]any
	dropSeedEcho   bool
	alterGuidance  bool
	replyText      string
	submittedCount int
}

func (f *fakeCanaryChat) EnsembleChat(_ context.Context, _ harnessclient.Persona, req harnessclient.EnsembleChatRequest) (*harnessclient.EnsembleChatResponse, error) {
	f.submittedCount++
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	f.request = req
	return &harnessclient.EnsembleChatResponse{Success: true, CaseID: "case-1", InvestigationID: "inv-1"}, nil
}

func (f *fakeCanaryChat) GetEvaluationTrace(_ context.Context, _ harnessclient.Persona, _, _ string) (EvaluationTrace, error) {
	registry, err := LoadAgentToolRegistry()
	if err != nil {
		return nil, err
	}
	declared := []any{}
	for _, tool := range registry.Tools {
		if tool.RequiresWebSearch || !slices.Contains(tool.AgentModes, canaryBoundAgentMode) || tool.Name == f.dropTool {
			continue
		}
		declared = append(declared, tool.Name)
	}
	scored := map[string]any{"agent_role": "sage", "provider": "G8EProvider"}
	if !f.omitDeclared {
		scored["tools_declared"] = declared
	}
	evalContext := map[string]any{}
	if !f.dropSeedEcho {
		evalContext["seed"] = jsonValue(f.request.EvaluationContext.Seed)
		if f.alterGuidance {
			events := evalContext["seed"].(map[string]any)["history_events"].([]any)
			events[0].(map[string]any)["error"] = "paraphrased guidance"
		}
	}
	evalContext["workspace"] = jsonValue(f.request.EvaluationContext.Workspace)
	applied := f.seedApplied
	if applied == nil {
		applied = map[string]any{"turns": float64(1), "history_events": float64(1), "case_memory": true}
	}
	gate := f.toolGate
	if gate == "" {
		gate = canaryToolGateBypass
	}
	return EvaluationTrace{
		"status":                 "completed",
		"tool_gate":              gate,
		"seed_application":       applied,
		"evaluation_context":     evalContext,
		"designated_role_output": f.replyText,
		"model_calls": []any{
			map[string]any{"agent_role": "triage", "provider": "G8EProvider"},
			scored,
			map[string]any{"agent_role": "codex", "provider": "G8EProvider"},
		},
	}, nil
}

func jsonValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return out
}

type fakeCanaryWorkspace struct {
	writeErr error
	readBack *string
	files    map[string]string
}

func (f *fakeCanaryWorkspace) WriteWorkspaceFile(_ context.Context, _ Target, _, _, _, absPath, content string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.files == nil {
		f.files = map[string]string{}
	}
	f.files[absPath] = content
	return nil
}

func (f *fakeCanaryWorkspace) ReadWorkspaceFile(_ context.Context, _ Target, _, _, _, absPath string) (string, error) {
	if f.readBack != nil {
		return *f.readBack, nil
	}
	content, ok := f.files[absPath]
	if !ok {
		return "", fmt.Errorf("no such file %s", absPath)
	}
	return content, nil
}

type fakeCanaryMCP struct {
	verifyErr error
	tools     []string
}

func (f fakeCanaryMCP) VerifyAgentIntegrations(context.Context) error { return f.verifyErr }
func (f fakeCanaryMCP) GatewayToolNames(context.Context) ([]string, error) {
	return f.tools, nil
}

type canaryHarness struct {
	chat      *fakeCanaryChat
	workspace *fakeCanaryWorkspace
	mcp       fakeCanaryMCP
}

func newCanaryHarness() *canaryHarness {
	return &canaryHarness{
		chat:      &fakeCanaryChat{},
		workspace: &fakeCanaryWorkspace{},
		mcp:       fakeCanaryMCP{tools: []string{"run_command"}},
	}
}

func (h *canaryHarness) deps() CanaryDeps {
	counter := 0
	return CanaryDeps{
		Chat: h.chat,
		WaitForTrace: func(ctx context.Context, fetch func(context.Context) (EvaluationTrace, error)) (EvaluationTrace, error) {
			return fetch(ctx)
		},
		WorkspaceWriter: h.workspace,
		WorkspaceReader: h.workspace,
		MCP:             h.mcp,
		Persona:         harnessclient.Persona{UserID: "user-1", CLISessionID: "cli-1"},

		DataOperatorID:               "data-op",
		DataOperatorSessionID:        "data-session",
		DataOperatorWorkingDirectory: "/srv/data-operator",
		Probe: ChatProbeRequest{
			Model:                   "model-a",
			ModelDigest:             "digest-a",
			ModelRegistryDigest:     "registry-digest",
			ModelRegistry:           []*operatorv1.InferenceModelVariant{{Model: "model-a", Digest: "digest-a"}},
			TargetOperatorSessionID: "inference-session",
		},
		NewID: func(prefix string) string {
			counter++
			return fmt.Sprintf("%s-%d", prefix, counter)
		},
	}
}

func canaryResult(t *testing.T, report CanaryReport, id CanaryID) CanaryResult {
	t.Helper()
	for _, result := range report.Results {
		if result.ID == id {
			return result
		}
	}
	t.Fatalf("canary %s was not run", id)
	return CanaryResult{}
}

func TestEnvironmentCanary_PassesOnAnIntactHarness(t *testing.T) {
	h := newCanaryHarness()

	report, err := RunEnvironmentCanaries(context.Background(), h.deps())

	require.NoError(t, err)
	require.Len(t, report.Results, 5)
	for _, result := range report.Results {
		assert.True(t, result.Passed, "%s: %s", result.ID, result.Detail)
	}
	assert.Equal(t, 1, h.chat.submittedCount, "the tools, seed, and guidance canaries share one chat")
}

func TestEnvironmentCanary_SendsAScenarioNeutralSeededRequest(t *testing.T) {
	h := newCanaryHarness()

	_, err := RunEnvironmentCanaries(context.Background(), h.deps())
	require.NoError(t, err)

	req := h.chat.request
	assert.Equal(t, canaryChatInstruction, req.Message)
	require.NotNil(t, req.ResourceCreation)
	assert.Equal(t, "Build log review", req.ResourceCreation.CaseTitle)
	assert.NotContains(t, req.ResourceCreation.CaseTitle, "phase1a")
	require.NotNil(t, req.EvaluationContext.Seed)
	assert.Len(t, req.EvaluationContext.Seed.Turns, 1)
	assert.Len(t, req.EvaluationContext.Seed.HistoryEvents, 1)
	require.NotNil(t, req.EvaluationContext.Workspace)
	assert.Equal(t, "/srv/data-operator", req.EvaluationContext.Workspace.OperatorWorkingDirectory)
}

func TestEnvironmentCanary_ToolsDeclaredIgnoresWhatTheModelReplied(t *testing.T) {
	h := newCanaryHarness()
	h.chat.replyText = "I would rather talk about the weather."

	report, err := RunEnvironmentCanaries(context.Background(), h.deps())

	require.NoError(t, err)
	assert.True(t, canaryResult(t, report, CanaryToolsDeclared).Passed)
}

func TestEnvironmentCanary_FailuresAreEnvironmentErrorsNamingTheCanary(t *testing.T) {
	missingTool := func() string {
		registry, err := LoadAgentToolRegistry()
		require.NoError(t, err)
		for _, tool := range registry.Tools {
			if !tool.RequiresWebSearch && slices.Contains(tool.AgentModes, canaryBoundAgentMode) {
				return tool.Name
			}
		}
		t.Fatal("registry has no bound tool")
		return ""
	}()
	readBack := "something else"
	tests := []struct {
		name     string
		mutate   func(h *canaryHarness)
		canary   CanaryID
		contains string
	}{
		{"a bound tool is not declared", func(h *canaryHarness) { h.chat.dropTool = missingTool }, CanaryToolsDeclared, missingTool},
		{"the provider reports no declaration", func(h *canaryHarness) { h.chat.omitDeclared = true }, CanaryToolsDeclared, "did not report"},
		{"the tool gate was not bypassed", func(h *canaryHarness) { h.chat.toolGate = "registry" }, CanaryToolsDeclared, "tool_gate"},
		{"the seed was applied partially", func(h *canaryHarness) {
			h.chat.seedApplied = map[string]any{"turns": float64(1), "history_events": float64(0), "case_memory": true}
		}, CanarySeedDelivered, "seed_application"},
		{"the seed was not echoed", func(h *canaryHarness) { h.chat.dropSeedEcho = true }, CanarySeedDelivered, "not echoed"},
		{"the workspace write is rejected", func(h *canaryHarness) { h.workspace.writeErr = errors.New("operator rejected write") }, CanaryWorkspaceReachable, "operator rejected write"},
		{"the workspace reads back other content", func(h *canaryHarness) { h.workspace.readBack = &readBack }, CanaryWorkspaceReachable, "returned"},
		{"the guidance is paraphrased", func(h *canaryHarness) { h.chat.alterGuidance = true }, CanaryGuidanceDelivered, "differs from registry vector"},
		{"agent registry verification fails", func(h *canaryHarness) { h.mcp.verifyErr = errors.New("lockdown drift") }, CanaryRegistryMCP, "lockdown drift"},
		{"the gateway lists no MCP tools", func(h *canaryHarness) { h.mcp.tools = nil }, CanaryRegistryMCP, "no tools"},
		{"the chat cannot be submitted", func(h *canaryHarness) { h.chat.submitErr = errors.New("g8ee unreachable") }, CanaryToolsDeclared, "g8ee unreachable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCanaryHarness()
			tt.mutate(h)

			report, err := RunEnvironmentCanaries(context.Background(), h.deps())

			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvaluationEnvironmentCanaryFailed)
			assert.Contains(t, err.Error(), string(tt.canary))
			result := canaryResult(t, report, tt.canary)
			assert.False(t, result.Passed)
			assert.Contains(t, result.Detail, tt.contains)
		})
	}
}

func TestEnvironmentCanary_UnreachableWorkspaceDoesNotHideOtherCanaries(t *testing.T) {
	h := newCanaryHarness()
	deps := h.deps()
	deps.DataOperatorWorkingDirectory = ""

	report, err := RunEnvironmentCanaries(context.Background(), deps)

	require.ErrorIs(t, err, constants.ErrEvaluationEnvironmentCanaryFailed)
	assert.False(t, canaryResult(t, report, CanaryWorkspaceReachable).Passed)
	assert.True(t, canaryResult(t, report, CanaryRegistryMCP).Passed, "independent canaries still run")
}
