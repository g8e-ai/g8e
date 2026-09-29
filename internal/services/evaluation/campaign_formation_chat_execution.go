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
	"time"

	"github.com/g8e-ai/g8e/v2/internal/models"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// CampaignFormationChatRunner executes one heterogeneous formation by routing
// each role (Lite, then Assistant, then Primary) through g8ee's production
// chat/trace pipeline — the same POST /api/v1/chat + GET
// /api/v1/evaluation/trace round trip CampaignChatExecutor already uses once
// per homogeneous assignment — instead of dispatching directly to the
// Inference Operator. It is a drop-in alternative implementation of
// CampaignFormationRunner alongside campaignFormationProductionRunner
// (formation_production_adapter.go); which one a given campaign uses is a
// construction-time choice made by the caller.
type CampaignFormationChatRunner struct {
	client         CampaignChatClient
	persona        harnessclient.Persona
	dataOperatorID string
	waitForTrace   CampaignTraceWaiter
	fileWriter     SimulatedFileWriter
}

// NewCampaignFormationChatRunner wires the g8ee-routed heterogeneous formation
// runner. fileWriter may be nil for a deployment with no scenario that sets
// ScenarioInputFixture.SimulatedFiles; RunHeterogeneousFormation fails closed
// if a scenario needs one and none is configured.
func NewCampaignFormationChatRunner(client CampaignChatClient, persona harnessclient.Persona, dataOperatorID string, waitForTrace CampaignTraceWaiter, fileWriter SimulatedFileWriter) *CampaignFormationChatRunner {
	return &CampaignFormationChatRunner{
		client:         client,
		persona:        persona,
		dataOperatorID: dataOperatorID,
		waitForTrace:   waitForTrace,
		fileWriter:     fileWriter,
	}
}

// formationRoleOutput pairs one completed role with its designated output
// text, threaded into every subsequent role's outgoing chat message.
type formationRoleOutput struct {
	Role   FormationRole
	Output string
}

// RunHeterogeneousFormation implements CampaignFormationRunner. It stops on
// the first role that fails to submit, fails to produce a terminal trace, or
// produces a "failed" trace — returning whatever roles completed so far
// alongside the error, mirroring FormationRunner.Run's existing contract
// (execution_topologies.go). A trace that is terminal with status
// "completed" but role_outcome other than "invoked" is not treated as a hard
// failure: telemetry is still recorded for that role and the pipeline
// continues, the same class of non-fatal outcome homogeneous assignments
// already classify as a PARTIAL lifecycle rather than an execution error.
func (r *CampaignFormationChatRunner) RunHeterogeneousFormation(ctx context.Context, binding FormationBindingRequest, runContext FormationRunContext, initialState []byte) (*FormationRunResult, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("evaluation: run heterogeneous formation: chat runner is required")
	}
	formation, err := ResolveFormationBinding(binding)
	if err != nil {
		return nil, err
	}
	var input ScenarioInputFixture
	if err := json.Unmarshal(initialState, &input); err != nil {
		return nil, fmt.Errorf("evaluation: run heterogeneous formation: decode initial state: %w", err)
	}
	result := &FormationRunResult{SchemaVersion: FormationSchemaVersion, FormationID: formation.ID, Roles: make([]FormationRoleTelemetry, 0, 3)}
	if err := r.materializeSimulatedFiles(ctx, runContext, input); err != nil {
		return result, fmt.Errorf("evaluation: run heterogeneous formation: materialize simulated files: %w", err)
	}
	modelRegistry := InferenceVariantsFromEvalRegistry(binding.Variants)
	priorOutputs := make([]formationRoleOutput, 0, 3)
	for _, role := range formation.Roles() {
		model, err := formation.Model(role)
		if err != nil {
			return result, err
		}
		if runContext.OnRoleStarting != nil {
			if err := runContext.OnRoleStarting(ctx, role); err != nil {
				return result, fmt.Errorf("evaluation: run heterogeneous formation: role starting: %w", err)
			}
		}
		probeReq := ChatProbeRequest{
			AssignmentID:            runContext.AssignmentID,
			EvaluationAttemptID:     runContext.EvaluationAttemptID + ":" + string(role),
			CampaignID:              runContext.CampaignID,
			RunID:                   runContext.RunID,
			ScenarioID:              runContext.ScenarioID,
			Model:                   model.ServedModelTag,
			ModelDigest:             model.ModelDigest,
			TargetOperatorSessionID: runContext.InferenceSessionID,
			ModelRegistryDigest:     runContext.ModelRegistryDigest,
			ModelRegistry:           modelRegistry,
			EvaluationLane:          "model_role",
			DesignatedModelRole:     string(role),
			Message:                 formationRoleChatMessage(input, priorOutputs),
			GradingMethod:           evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_DETERMINISTIC,
		}
		chatReq, err := BuildChatProbeRequest(probeReq, r.dataOperatorID, runContext.DataSessionID)
		if err != nil {
			return result, fmt.Errorf("evaluation: run heterogeneous formation: build chat request %s: %w", role, err)
		}
		chatReq.Context.UserID = r.persona.UserID
		chatReq.Context.CLISessionID = r.persona.CLISessionID
		if _, err := r.client.EnsembleChat(ctx, r.persona, chatReq); err != nil {
			return result, fmt.Errorf("evaluation: run heterogeneous formation: submit chat %s: %w", role, err)
		}
		if r.waitForTrace == nil {
			return result, fmt.Errorf("evaluation: run heterogeneous formation: trace waiter is required")
		}
		fetchTrace := func(pollCtx context.Context) (EvaluationTrace, error) {
			return r.client.GetEvaluationTrace(pollCtx, r.persona, probeReq.AssignmentID, probeReq.EvaluationAttemptID)
		}
		trace, err := r.waitForTrace(ctx, fetchTrace)
		if err != nil {
			return result, fmt.Errorf("evaluation: run heterogeneous formation: wait for trace %s: %w", role, err)
		}
		status, _ := trace["status"].(string)
		if status != "completed" && status != "failed" {
			return result, fmt.Errorf("evaluation: run heterogeneous formation: role %s trace status %q is not terminal", role, status)
		}
		if status == "failed" {
			return result, fmt.Errorf("evaluation: run heterogeneous formation: role %s trace failed", role)
		}
		result.Roles = append(result.Roles, formationRoleTelemetryFromTrace(role, model, probeReq.EvaluationAttemptID, trace))
		if output := designatedRoleOutput(trace); output != "" {
			priorOutputs = append(priorOutputs, formationRoleOutput{Role: role, Output: output})
		}
		if runContext.OnRoleProgress != nil {
			if err := runContext.OnRoleProgress(ctx, result); err != nil {
				return result, fmt.Errorf("evaluation: run heterogeneous formation: role progress: %w", err)
			}
		}
	}
	allInvoked := len(result.Roles) == 3
	for _, role := range result.Roles {
		if !traceRoleInvoked(role.Trace, string(role.Role)) {
			allInvoked = false
		}
	}
	result.Passed = allInvoked
	return result, nil
}

func (r *CampaignFormationChatRunner) materializeSimulatedFiles(ctx context.Context, runContext FormationRunContext, input ScenarioInputFixture) error {
	if len(input.SimulatedFiles) == 0 {
		return nil
	}
	if r.fileWriter == nil {
		return fmt.Errorf("evaluation: scenario %s requires a simulated file writer", runContext.ScenarioID)
	}
	target := Target{OperatorID: r.dataOperatorID, SessionID: runContext.DataSessionID}
	for _, file := range input.SimulatedFiles {
		if err := r.fileWriter.WriteSimulatedFile(ctx, target, runContext.RunID, runContext.ScenarioID, runContext.EvaluationAttemptID, file); err != nil {
			return err
		}
	}
	return nil
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
		b.WriteString(fmt.Sprintf("\n\n[%s]:\n%s", prior.Role, prior.Output))
	}
	return b.String()
}

// formationRoleTelemetryFromTrace extracts the FormationRoleTelemetry fields
// available from one role's g8ee trace, the same fields
// modelInferenceRecordsFromTrace (campaign_trace_import.go) extracts for
// homogeneous assignments. AttestationStatus/AttestationVerified/
// AttestationDigest/PeakVRAMMiB/ObserverEvidence/ProvenanceEvidence stay at
// zero value: no direct provenance/observer operator interaction happens on
// this path, an accepted consequence of routing through g8ee instead of the
// direct-dispatch runner. TTFTNanos also stays 0: it is populated in the
// direct-dispatch path from the Inference Operator's dispatch response,
// which a g8ee chat trace does not carry.
func formationRoleTelemetryFromTrace(role FormationRole, model FormationModel, attemptID string, trace EvaluationTrace) FormationRoleTelemetry {
	telemetry := FormationRoleTelemetry{
		Role:      role,
		Model:     model,
		AttemptID: attemptID,
		Trace:     trace,
	}
	modelCalls, _ := trace["model_calls"].([]any)
	for _, rawCall := range modelCalls {
		call, ok := evaluationTrace(rawCall)
		if !ok {
			continue
		}
		provider, _ := call["provider"].(string)
		if !strings.EqualFold(provider, "G8EProvider") {
			continue
		}
		if succeeded, ok := call["succeeded"].(bool); ok && !succeeded {
			continue
		}
		providerAttemptID := stringValue(call["provider_attempt_id"])
		if providerAttemptID == "" {
			continue
		}
		telemetry.ProviderAttemptID = providerAttemptID
		telemetry.FinishReason = stringValue(call["finish_reason"])
		if reported, ok := call["usage_reported"].(bool); ok && reported {
			telemetry.UsageAvailability = evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED
			if promptTokens, err := requiredUint32FromTraceCall(call, "input_tokens"); err == nil {
				telemetry.PromptTokens = promptTokens
			}
			if completionTokens, err := requiredUint32FromTraceCall(call, "output_tokens"); err == nil {
				telemetry.GenerationTokens = completionTokens
			}
		} else {
			telemetry.UsageAvailability = evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE
		}
		if retry, present := call["retry_count"]; present {
			if converted, err := uint32Value(retry); err == nil {
				telemetry.RetryCount = converted
			}
		}
		if raw, present := call["generation_duration_seconds"]; present && raw != nil {
			if converted, err := durationSecondsToNanosChecked(raw); err == nil {
				telemetry.GenerationDurationNanos = converted
			}
		}
		if raw, present := call["load_duration_seconds"]; present && raw != nil {
			if converted, err := durationSecondsToNanosChecked(raw); err == nil {
				nanos := int64(converted)
				telemetry.LoadState = operatorLoadStateToEvaluation(models.ClassifyLoadState(&nanos))
			}
		}
		break
	}
	if telemetry.GenerationDurationNanos > 0 {
		telemetry.GenerationTokensPerSec = float64(telemetry.GenerationTokens) / (float64(telemetry.GenerationDurationNanos) / float64(time.Second))
	}
	return telemetry
}
