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
	"fmt"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/models"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// CampaignFormationChatRunner executes one heterogeneous formation through the
// same governed FormationRunner as the direct-dispatch runner — storage-side
// provenance attestation before allocation, co-resident allocation and
// release, provider-boundary observation per role, and the mutation policy
// gate — but executes each role (Lite, then Assistant, then Primary) through
// g8ee's production chat/trace pipeline instead of a bare Inference Operator
// dispatch. Roles therefore make real tool calls and produce digest-bound
// traces that are graded, while every per-formation witness metric the
// direct runner records is still recorded.
type CampaignFormationChatRunner struct {
	client         CampaignChatClient
	persona        harnessclient.Persona
	dataOperatorID string
	waitForTrace   CampaignTraceWaiter
	fileWriter     WorkspaceFileWriter
	production     FormationProductionDependencies
}

// NewCampaignFormationChatRunner wires the g8ee-routed heterogeneous formation
// runner. production supplies the same provenance, observation, allocation,
// and release dependencies the direct-dispatch runner uses; its RoleExecutor
// is set per run. fileWriter may be nil for a deployment with no scenario that
// sets ScenarioInputFixture.WorkspaceFiles; RunHeterogeneousFormation fails
// closed if a scenario needs one and none is configured.
func NewCampaignFormationChatRunner(client CampaignChatClient, persona harnessclient.Persona, dataOperatorID string, waitForTrace CampaignTraceWaiter, fileWriter WorkspaceFileWriter, production FormationProductionDependencies) *CampaignFormationChatRunner {
	return &CampaignFormationChatRunner{
		client:         client,
		persona:        persona,
		dataOperatorID: dataOperatorID,
		waitForTrace:   waitForTrace,
		fileWriter:     fileWriter,
		production:     production,
	}
}

// formationRoleOutput pairs one completed role with its designated output
// text, threaded into every subsequent role's outgoing chat message.
type formationRoleOutput struct {
	Role   FormationRole
	Output string
}

// RunHeterogeneousFormation implements CampaignFormationRunner. initialState
// is the JSON ScenarioInputFixture. FormationRunner stops on the first role
// that fails to submit, fails to produce a terminal trace, produces a
// "failed" trace, or lacks witness evidence, returning the roles completed so
// far. A trace that completed with a role_outcome other than "invoked" is not
// a hard failure; it clears Passed instead, the same class of non-fatal
// outcome homogeneous assignments classify as PARTIAL.
func (r *CampaignFormationChatRunner) RunHeterogeneousFormation(ctx context.Context, binding FormationBindingRequest, runContext FormationRunContext, initialState []byte) (*FormationRunResult, error) {
	if r == nil || r.client == nil || r.waitForTrace == nil {
		return nil, fmt.Errorf("evaluation: run heterogeneous formation: chat client and trace waiter are required")
	}
	var input ScenarioInputFixture
	if err := json.Unmarshal(initialState, &input); err != nil {
		return nil, fmt.Errorf("evaluation: run heterogeneous formation: decode initial state: %w", err)
	}
	if err := r.materializeWorkspaceFiles(ctx, runContext, input); err != nil {
		return nil, fmt.Errorf("evaluation: run heterogeneous formation: materialize workspace files: %w", err)
	}
	deps := r.production
	deps.RunContext = runContext
	deps.RoleExecutor = &formationChatRoleExecutor{
		runner:     r,
		runContext: runContext,
		input:      input,
		registry:   InferenceVariantsFromEvalRegistry(binding.Variants),
	}
	result, err := RunHeterogeneousFormationProduction(ctx, binding, deps, nil)
	if err != nil || result == nil {
		return result, err
	}
	for _, role := range result.Roles {
		if !traceRoleInvoked(role.Trace, string(role.Role)) {
			result.Passed = false
		}
	}
	return result, nil
}

// formationChatRoleExecutor executes one formation role as one g8ee chat
// turn. FormationRunner calls it sequentially, so priorOutputs needs no lock.
type formationChatRoleExecutor struct {
	runner       *CampaignFormationChatRunner
	runContext   FormationRunContext
	input        ScenarioInputFixture
	registry     []*operatorv1.InferenceModelVariant
	priorOutputs []formationRoleOutput
}

func (e *formationChatRoleExecutor) ExecuteRole(ctx context.Context, req FormationRoleRequest) (FormationRoleResult, error) {
	r := e.runner
	probeReq := ChatProbeRequest{
		AssignmentID:            e.runContext.AssignmentID,
		EvaluationAttemptID:     e.runContext.EvaluationAttemptID + ":" + string(req.Role),
		CampaignID:              e.runContext.CampaignID,
		RunID:                   e.runContext.RunID,
		ScenarioID:              e.runContext.ScenarioID,
		Model:                   req.Model.ServedModelTag,
		ModelDigest:             req.Model.ModelDigest,
		TargetOperatorSessionID: e.runContext.InferenceSessionID,
		ModelRegistryDigest:     e.runContext.ModelRegistryDigest,
		ModelRegistry:           e.registry,
		EvaluationLane:          "model_role",
		DesignatedModelRole:     string(req.Role),
		Message:                 formationRoleChatMessage(e.input, e.priorOutputs),
		GradingMethod:           evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
	}
	chatReq, err := BuildChatProbeRequest(probeReq, r.dataOperatorID, e.runContext.DataSessionID)
	if err != nil {
		return FormationRoleResult{}, fmt.Errorf("evaluation: formation role %s: build chat request: %w", req.Role, err)
	}
	chatReq.Context.UserID = r.persona.UserID
	chatReq.Context.CLISessionID = r.persona.CLISessionID
	if _, err := r.client.EnsembleChat(ctx, r.persona, chatReq); err != nil {
		return FormationRoleResult{}, fmt.Errorf("evaluation: formation role %s: submit chat: %w", req.Role, err)
	}
	trace, err := r.waitForTrace(ctx, func(pollCtx context.Context) (EvaluationTrace, error) {
		return r.client.GetEvaluationTrace(pollCtx, r.persona, probeReq.AssignmentID, probeReq.EvaluationAttemptID)
	})
	if err != nil {
		return FormationRoleResult{}, fmt.Errorf("evaluation: formation role %s: wait for trace: %w", req.Role, err)
	}
	switch status, _ := trace["status"].(string); status {
	case "completed":
	case "failed":
		return FormationRoleResult{}, fmt.Errorf("evaluation: formation role %s: trace failed", req.Role)
	default:
		return FormationRoleResult{}, fmt.Errorf("evaluation: formation role %s: trace status %q is not terminal", req.Role, status)
	}
	output := designatedRoleOutput(trace)
	if output != "" {
		e.priorOutputs = append(e.priorOutputs, formationRoleOutput{Role: req.Role, Output: output})
	}
	// Store prior role outputs in trace for grading verification of handoff.
	if len(e.priorOutputs) > 0 {
		priorMap := make([]map[string]string, 0, len(e.priorOutputs))
		for _, prior := range e.priorOutputs {
			priorMap = append(priorMap, map[string]string{
				"role":   string(prior.Role),
				"output": prior.Output,
			})
		}
		trace["prior_role_outputs"] = priorMap
	}
	result := formationRoleResultFromTrace(trace)
	result.OutputState = formationAppendRoleState(req.InputState, req.Role, output)
	result.MutationCandidate = append([]byte(nil), result.OutputState...)
	result.StateMutation = req.Role == FormationRolePrimary
	return result, nil
}

func (r *CampaignFormationChatRunner) materializeWorkspaceFiles(_ context.Context, runContext FormationRunContext, input ScenarioInputFixture) error {
	if len(input.WorkspaceFiles) == 0 {
		return nil
	}
	if r.fileWriter == nil {
		return fmt.Errorf("evaluation: scenario %s requires a workspace file writer", runContext.ScenarioID)
	}
	return fmt.Errorf("evaluation: workspace materialization lands in WP5")
}

// formationRoleChatMessage renders the outgoing chat message for one role:
// the base scenario message (same rendering homogeneous assignments use),
// plus every already-completed role's designated output appended as a
// cumulative handoff block — the only context-threading channel
// EnsembleChatRequest exposes is its single Message text field, so threading
// is necessarily textual here, matching the cumulative state the
// direct-dispatch runner already builds via formationAppendRoleState
// (formation_production_adapter.go).
func formationRoleChatMessage(input ScenarioInputFixture, priorOutputs []formationRoleOutput) string {
	base := renderScenarioMessage(input.UserPrompt, input.InlineContext)
	if len(priorOutputs) == 0 {
		return base
	}
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\nPrior formation role output:")
	for _, prior := range priorOutputs {
		fmt.Fprintf(&b, "\n\n[%s]:\n%s", prior.Role, prior.Output)
	}
	return b.String()
}

// formationRoleResultFromTrace aggregates every scored G8EProvider call in one
// role's trace into the role's formation telemetry: the first call names the
// role's provider attempt (its bound observer window) and load state; the rest
// are AdditionalProviderAttemptIDs. Tokens, durations, and retries are summed;
// TTFT comes from the first call and the finish reason from the last. Usage is
// reported only when every call reported it.
func formationRoleResultFromTrace(trace EvaluationTrace) FormationRoleResult {
	result := FormationRoleResult{Trace: trace}
	calls := scoredProviderCalls(trace)
	if len(calls) == 0 {
		result.UsageAvailability = evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE
		return result
	}
	result.UsageAvailability = evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED
	for index, call := range calls {
		attemptID := stringValue(call["provider_attempt_id"])
		if index == 0 {
			result.ProviderAttemptID = attemptID
			if raw, present := call["time_to_first_token_seconds"]; present && raw != nil {
				if converted, err := durationSecondsToNanosChecked(raw); err == nil {
					result.TTFTNanos = converted
				}
			}
			if raw, present := call["load_duration_seconds"]; present && raw != nil {
				if converted, err := durationSecondsToNanosChecked(raw); err == nil {
					nanos := int64(converted)
					result.LoadState = operatorLoadStateToEvaluation(models.ClassifyLoadState(&nanos))
				}
			}
		} else {
			result.AdditionalProviderAttemptIDs = append(result.AdditionalProviderAttemptIDs, attemptID)
		}
		result.FinishReason = stringValue(call["finish_reason"])
		if reported, ok := call["usage_reported"].(bool); ok && reported {
			if promptTokens, err := requiredUint32FromTraceCall(call, "input_tokens"); err == nil {
				result.PromptTokens += promptTokens
			}
			if completionTokens, err := requiredUint32FromTraceCall(call, "output_tokens"); err == nil {
				result.GenerationTokens += completionTokens
			}
		} else {
			result.UsageAvailability = evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE
		}
		if retry, present := call["retry_count"]; present {
			if converted, err := uint32Value(retry); err == nil {
				result.RetryCount += converted
			}
		}
		if raw, present := call["generation_duration_seconds"]; present && raw != nil {
			if converted, err := durationSecondsToNanosChecked(raw); err == nil {
				result.GenerationDurationNanos += converted
			}
		}
	}
	return result
}

// scoredProviderCalls returns the trace's successful G8EProvider model calls
// that carry a provider attempt ID, in call order — the same selection
// modelInferenceRecordsFromTrace applies for homogeneous assignments.
func scoredProviderCalls(trace EvaluationTrace) []EvaluationTrace {
	modelCalls, _ := trace["model_calls"].([]any)
	calls := make([]EvaluationTrace, 0, len(modelCalls))
	for _, rawCall := range modelCalls {
		call, ok := evaluationTrace(rawCall)
		if !ok {
			continue
		}
		if provider, _ := call["provider"].(string); !strings.EqualFold(provider, "G8EProvider") {
			continue
		}
		if succeeded, ok := call["succeeded"].(bool); ok && !succeeded {
			continue
		}
		if stringValue(call["provider_attempt_id"]) == "" {
			continue
		}
		calls = append(calls, call)
	}
	return calls
}

// traceProviderAttemptIDs lists every scored provider attempt in one role
// trace, in call order.
func traceProviderAttemptIDs(trace EvaluationTrace) []string {
	calls := scoredProviderCalls(trace)
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		ids = append(ids, stringValue(call["provider_attempt_id"]))
	}
	return ids
}
