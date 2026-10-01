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
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

// CanaryID names one environment canary.
type CanaryID string

const (
	CanaryToolsDeclared      CanaryID = "tools-declared"
	CanarySeedDelivered      CanaryID = "seed-delivered"
	CanaryWorkspaceReachable CanaryID = "workspace-reachable"
	CanaryGuidanceDelivered  CanaryID = "guidance-delivered"
	CanaryRegistryMCP        CanaryID = "registry-mcp"
)

// EnvironmentCanaryCampaignID is the campaign the canary chat is issued under.
// The registry digest a chat request carries is bound to its campaign ID, so
// callers building CanaryDeps.Probe freeze the registry under this ID.
const EnvironmentCanaryCampaignID = "environment-canary"

const (
	canaryChatTimeout     = 5 * time.Minute
	canaryScenarioID      = "environment-canary"
	canaryWorkspaceFile   = "canary/probe.txt"
	canaryGuidanceVector  = "recursive_grep_search.missing_path"
	canaryGuidanceEvent   = "g8e.v1.operator.filesystem.grep.failed"
	canaryBoundAgentMode  = "g8e.bound"
	canaryToolGateBypass  = "bypassed_for_eval"
	canaryChatInstruction = "Reply with exactly: canary-ok"
)

// WorkspaceFileReader reads one workspace file back through a governed read on
// the bound Data Operator. *CommandLane implements this.
type WorkspaceFileReader interface {
	ReadWorkspaceFile(ctx context.Context, target Target, runID, scenarioID, attemptID, absPath string) (string, error)
}

// CanaryMCPVerifier covers the registry and Gateway MCP smoke checks.
type CanaryMCPVerifier interface {
	// VerifyAgentIntegrations runs the verification `g8e mcp agent verify`
	// performs for every agent registry entry.
	VerifyAgentIntegrations(ctx context.Context) error
	// GatewayToolNames returns the tool names a Gateway /mcp tools/list returns.
	GatewayToolNames(ctx context.Context) ([]string, error)
}

// CanaryDeps are the explicit collaborators the canaries use. Probe supplies
// the model, registry, and inference-operator identity; the canaries fill in
// the assignment identity, message, seed, and workspace themselves.
type CanaryDeps struct {
	Chat            CampaignChatClient
	WaitForTrace    CampaignTraceWaiter
	WorkspaceWriter WorkspaceFileWriter
	WorkspaceReader WorkspaceFileReader
	MCP             CanaryMCPVerifier
	Persona         harnessclient.Persona

	DataOperatorID               string
	DataOperatorSessionID        string
	DataOperatorWorkingDirectory string

	Probe ChatProbeRequest
	NewID func(prefix string) string
}

// CanaryResult is the outcome of one canary.
type CanaryResult struct {
	ID     CanaryID
	Passed bool
	Detail string
}

// CanaryReport lists every canary result in execution order.
type CanaryReport struct {
	Results []CanaryResult
}

// RunEnvironmentCanaries proves the evaluation harness is intact before any
// model is scored: the full tool set reaches the provider, a seed is applied
// and echoed, the fixture workspace is reachable, seeded guidance is delivered
// verbatim, and the agent registry and Gateway MCP surface verify. A failure is
// always an environment error (ErrEvaluationEnvironmentCanaryFailed naming the
// canary), never a statement about model behavior, so no canary inspects what
// the model answered.
func RunEnvironmentCanaries(ctx context.Context, deps CanaryDeps) (CanaryReport, error) {
	registry, err := LoadAgentToolRegistry()
	if err != nil {
		return CanaryReport{}, fmt.Errorf("%w: %w", constants.ErrEvaluationEnvironmentCanaryFailed, err)
	}
	if deps.NewID == nil {
		deps.NewID = func(prefix string) string { return prefix }
	}
	runID := deps.NewID("environment-canary-run")
	attemptID := deps.NewID("environment-canary-attempt")

	var report CanaryReport
	record := func(id CanaryID, detail string, err error) {
		if err != nil {
			report.Results = append(report.Results, CanaryResult{ID: id, Detail: err.Error()})
			return
		}
		report.Results = append(report.Results, CanaryResult{ID: id, Passed: true, Detail: detail})
	}

	ws, wsErr := NewScenarioWorkspace(deps.DataOperatorWorkingDirectory, runID, attemptID)
	vector, vectorOK := registry.GuidanceVector(canaryGuidanceVector)
	seed := canarySeed(vector)

	var trace EvaluationTrace
	var chatErr error
	switch {
	case wsErr != nil:
		chatErr = wsErr
	case !vectorOK:
		chatErr = fmt.Errorf("guidance vector %q is missing from the agent tool registry", canaryGuidanceVector)
	default:
		trace, chatErr = runCanaryChat(ctx, deps, runID, attemptID, seed, &ws)
	}

	if chatErr != nil {
		detail := fmt.Errorf("canary chat failed: %w", chatErr)
		record(CanaryToolsDeclared, "", detail)
		record(CanarySeedDelivered, "", detail)
	} else {
		detail, err := checkToolsDeclared(trace, registry)
		record(CanaryToolsDeclared, detail, err)
		detail, err = checkSeedDelivered(trace, seed, &ws)
		record(CanarySeedDelivered, detail, err)
	}

	detail, err := checkWorkspaceReachable(ctx, deps, runID, attemptID, ws, wsErr)
	record(CanaryWorkspaceReachable, detail, err)

	if chatErr != nil {
		record(CanaryGuidanceDelivered, "", fmt.Errorf("canary chat failed: %w", chatErr))
	} else {
		detail, err := checkGuidanceDelivered(trace, vector)
		record(CanaryGuidanceDelivered, detail, err)
	}

	detail, err = checkRegistryAndMCP(ctx, deps)
	record(CanaryRegistryMCP, detail, err)

	var failures []error
	for _, result := range report.Results {
		if !result.Passed {
			failures = append(failures, fmt.Errorf("%w: %s: %s", constants.ErrEvaluationEnvironmentCanaryFailed, result.ID, result.Detail))
		}
	}
	return report, errors.Join(failures...)
}

// canarySeed is the minimal realistic seed the canary chat carries: one turn,
// one history event quoting the registry's real guidance, and a case memory.
func canarySeed(vector AgentToolGuidanceVector) *harnessclient.EnsembleInvestigationSeed {
	return &harnessclient.EnsembleInvestigationSeed{
		CaseTitle: "Build log review",
		Turns: []harnessclient.EnsembleSeedTurn{
			{Sender: "user", Content: "Can you help me look through the build logs?"},
		},
		HistoryEvents: []harnessclient.EnsembleSeedHistoryEvent{{
			EventType:     canaryGuidanceEvent,
			Actor:         "system",
			Summary:       "Log search failed",
			ToolName:      vector.ToolName,
			ExecutionID:   vector.ExecutionID,
			ArgumentsJSON: vector.ArgumentsJSON,
			Error:         vector.Error,
			ErrorType:     vector.ErrorType,
		}},
		CaseMemory: &harnessclient.EnsembleSeedMemory{
			InvestigationSummary: "Reviewing build logs for a failed deploy.",
		},
	}
}

func runCanaryChat(ctx context.Context, deps CanaryDeps, runID, attemptID string, seed *harnessclient.EnsembleInvestigationSeed, ws *ScenarioWorkspace) (EvaluationTrace, error) {
	if deps.Chat == nil || deps.WaitForTrace == nil {
		return nil, fmt.Errorf("chat client and trace waiter are required")
	}
	probe := deps.Probe
	probe.CampaignID = EnvironmentCanaryCampaignID
	probe.RunID = runID
	probe.ScenarioID = canaryScenarioID
	probe.AssignmentID = deps.NewID("environment-canary-assignment")
	probe.EvaluationAttemptID = attemptID
	probe.EvaluationLane = "system"
	probe.DesignatedModelRole = ""
	probe.Message = canaryChatInstruction
	probe.Seed = seed
	probe.Workspace = ws
	chatReq, err := BuildChatProbeRequest(probe, deps.DataOperatorID, deps.DataOperatorSessionID)
	if err != nil {
		return nil, err
	}
	chatReq.Context.UserID = deps.Persona.UserID
	chatReq.Context.CLISessionID = deps.Persona.CLISessionID

	chatCtx, cancel := context.WithTimeout(ctx, canaryChatTimeout)
	defer cancel()
	if _, err := deps.Chat.EnsembleChat(chatCtx, deps.Persona, chatReq); err != nil {
		return nil, fmt.Errorf("submit chat: %w", err)
	}
	trace, err := deps.WaitForTrace(chatCtx, func(pollCtx context.Context) (EvaluationTrace, error) {
		return deps.Chat.GetEvaluationTrace(pollCtx, deps.Persona, probe.AssignmentID, probe.EvaluationAttemptID)
	})
	if err != nil {
		return nil, fmt.Errorf("wait for trace: %w", err)
	}
	return trace, nil
}

// scoredModelCall returns the first model call made by the scored agent, never
// triage, memory, or a grader.
func scoredModelCall(trace EvaluationTrace) (EvaluationTrace, bool) {
	modelCalls, _ := trace["model_calls"].([]any)
	for _, raw := range modelCalls {
		call, ok := evaluationTrace(raw)
		if !ok {
			continue
		}
		if isScoredAgentRole(stringValue(call["agent_role"])) {
			return call, true
		}
	}
	return nil, false
}

func checkToolsDeclared(trace EvaluationTrace, registry *AgentToolRegistry) (string, error) {
	if gate := stringValue(trace["tool_gate"]); gate != canaryToolGateBypass {
		return "", fmt.Errorf("trace tool_gate is %q, want %q", gate, canaryToolGateBypass)
	}
	call, ok := scoredModelCall(trace)
	if !ok {
		return "", fmt.Errorf("trace records no scored model call")
	}
	if raw, present := call["tools_declared"]; !present || raw == nil {
		return "", fmt.Errorf("scored model call did not report the tools it declared")
	}
	declared := stringSliceFromAny(call["tools_declared"])
	var missing []string
	expected := 0
	for _, tool := range registry.Tools {
		if tool.RequiresWebSearch || !slices.Contains(tool.AgentModes, canaryBoundAgentMode) {
			continue
		}
		expected++
		if !slices.Contains(declared, tool.Name) {
			missing = append(missing, tool.Name)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("tools missing from the declaration: %s", strings.Join(missing, ", "))
	}
	return fmt.Sprintf("%d registry tools declared with the eval tool-gate bypass recorded", expected), nil
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func checkSeedDelivered(trace EvaluationTrace, seed *harnessclient.EnsembleInvestigationSeed, ws *ScenarioWorkspace) (string, error) {
	raw, present := trace["seed_application"]
	if !present || raw == nil {
		return "", fmt.Errorf("trace records no seed_application")
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("encode seed_application: %w", err)
	}
	var applied struct {
		Turns         int  `json:"turns"`
		HistoryEvents int  `json:"history_events"`
		CaseMemory    bool `json:"case_memory"`
	}
	if err := json.Unmarshal(encoded, &applied); err != nil {
		return "", fmt.Errorf("decode seed_application: %w", err)
	}
	if applied.Turns != len(seed.Turns) || applied.HistoryEvents != len(seed.HistoryEvents) || applied.CaseMemory != (seed.CaseMemory != nil) {
		return "", fmt.Errorf("seed_application is turns=%d history_events=%d case_memory=%t, want %d/%d/%t",
			applied.Turns, applied.HistoryEvents, applied.CaseMemory, len(seed.Turns), len(seed.HistoryEvents), seed.CaseMemory != nil)
	}
	evalContext, ok := evaluationTrace(trace["evaluation_context"])
	if !ok {
		return "", fmt.Errorf("trace records no evaluation_context")
	}
	if err := validateEchoedSeedAndWorkspace(seed, ws, evalContext); err != nil {
		return "", err
	}
	return fmt.Sprintf("seed applied (%d turn, %d history event, case memory) and echoed unchanged", applied.Turns, applied.HistoryEvents), nil
}

// validateEchoedSeedAndWorkspace compares the seed and workspace g8ee echoed
// with the ones the canary sent.
func validateEchoedSeedAndWorkspace(seed *harnessclient.EnsembleInvestigationSeed, ws *ScenarioWorkspace, evalContext EvaluationTrace) error {
	rawSeed := evalContext["seed"]
	if rawSeed == nil {
		return fmt.Errorf("evaluation_context.seed was not echoed")
	}
	want, err := canonicalSeedBytes(seed)
	if err != nil {
		return fmt.Errorf("canonicalize sent seed: %w", err)
	}
	got, err := canonicalSeedBytes(rawSeed)
	if err != nil {
		return fmt.Errorf("canonicalize echoed seed: %w", err)
	}
	if string(want) != string(got) {
		return fmt.Errorf("evaluation_context.seed differs from the seed that was sent")
	}
	rawWs, ok := evaluationTrace(evalContext["workspace"])
	if !ok {
		return fmt.Errorf("evaluation_context.workspace was not echoed")
	}
	traceWs := ScenarioWorkspace{Root: stringValue(rawWs["root"]), OperatorWorkingDirectory: stringValue(rawWs["operator_working_directory"])}
	if traceWs != *ws {
		return fmt.Errorf("evaluation_context.workspace differs from the workspace that was sent")
	}
	return nil
}

func checkWorkspaceReachable(ctx context.Context, deps CanaryDeps, runID, attemptID string, ws ScenarioWorkspace, wsErr error) (string, error) {
	if wsErr != nil {
		return "", wsErr
	}
	if deps.WorkspaceWriter == nil || deps.WorkspaceReader == nil {
		return "", fmt.Errorf("workspace file writer and reader are required")
	}
	absPath, err := ws.FilePath(canaryWorkspaceFile)
	if err != nil {
		return "", err
	}
	want := "canary-" + deps.NewID("workspace")
	target := Target{OperatorID: deps.DataOperatorID, SessionID: deps.DataOperatorSessionID}
	if err := deps.WorkspaceWriter.WriteWorkspaceFile(ctx, target, runID, canaryScenarioID, attemptID, absPath, want); err != nil {
		return "", fmt.Errorf("write %s: %w", absPath, err)
	}
	got, err := deps.WorkspaceReader.ReadWorkspaceFile(ctx, target, runID, canaryScenarioID, attemptID, absPath)
	if err != nil {
		return "", fmt.Errorf("read back %s: %w", absPath, err)
	}
	if got != want {
		return "", fmt.Errorf("read back %s returned %q, want %q", absPath, got, want)
	}
	return "workspace file written and read back through governed dispatch", nil
}

func checkGuidanceDelivered(trace EvaluationTrace, vector AgentToolGuidanceVector) (string, error) {
	evalContext, ok := evaluationTrace(trace["evaluation_context"])
	if !ok {
		return "", fmt.Errorf("trace records no evaluation_context")
	}
	seed, ok := evaluationTrace(evalContext["seed"])
	if !ok {
		return "", fmt.Errorf("evaluation_context.seed was not echoed")
	}
	events, _ := seed["history_events"].([]any)
	for _, raw := range events {
		event, ok := evaluationTrace(raw)
		if !ok || stringValue(event["tool_name"]) != vector.ToolName {
			continue
		}
		if got := stringValue(event["error"]); got != vector.Error {
			return "", fmt.Errorf("echoed guidance for %s differs from registry vector %s", vector.ToolName, vector.VectorID)
		}
		return fmt.Sprintf("registry vector %s delivered byte for byte", vector.VectorID), nil
	}
	return "", fmt.Errorf("echoed seed has no history event for %s", vector.ToolName)
}

func checkRegistryAndMCP(ctx context.Context, deps CanaryDeps) (string, error) {
	if deps.MCP == nil {
		return "", fmt.Errorf("MCP verifier is required")
	}
	if err := deps.MCP.VerifyAgentIntegrations(ctx); err != nil {
		return "", fmt.Errorf("agent registry verification: %w", err)
	}
	tools, err := deps.MCP.GatewayToolNames(ctx)
	if err != nil {
		return "", fmt.Errorf("gateway /mcp tools/list: %w", err)
	}
	if len(tools) == 0 {
		return "", fmt.Errorf("gateway /mcp tools/list returned no tools")
	}
	return fmt.Sprintf("agent registry verified; gateway lists %d MCP tools", len(tools)), nil
}
