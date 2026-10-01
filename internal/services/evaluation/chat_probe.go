// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// renderScenarioMessage composes the outgoing chat message text from the
// frozen user prompt and any inline synthetic content. Content is appended
// beneath the prompt in labeled blocks with no stray header, so the model
// receives it as literal message text rather than through any attachment
// mechanism. The prompt and content are rendered with workspace substitution.
func renderScenarioMessage(userPrompt string, inlineContext []ScenarioInlineContent, ws *ScenarioWorkspace) string {
	var b strings.Builder
	prompt := strings.TrimSpace(userPrompt)
	if ws != nil {
		prompt = ws.Render(prompt)
	}
	b.WriteString(prompt)
	for _, item := range inlineContext {
		b.WriteString("\n\n")
		if item.Label != "" || item.Kind != "" {
			b.WriteString("[")
			if item.Label != "" {
				b.WriteString(item.Label)
				if item.Kind != "" {
					b.WriteString(" (")
					b.WriteString(item.Kind)
					b.WriteString(")")
				}
			} else if item.Kind != "" {
				b.WriteString(item.Kind)
			}
			b.WriteString("]\n")
		}
		content := item.Content
		if ws != nil {
			content = ws.Render(content)
		}
		b.WriteString(content)
	}
	return b.String()
}

func chatGradingMethodLabel(method evalv1.EvaluationGradingMethod) string {
	if method == evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE {
		return "semantic_judge"
	}
	return "deterministic"
}

// ChatProbeRequest carries a non-scored production chat probe through
// POST /api/v1/chat. It is not a scored campaign evaluation.
type ChatProbeRequest struct {
	AssignmentID            string
	EvaluationAttemptID     string
	CampaignID              string
	RunID                   string
	ScenarioID              string
	Model                   string
	ModelDigest             string
	TargetOperatorSessionID string
	ModelRegistryDigest     string
	ModelRegistry           []*operatorv1.InferenceModelVariant
	EvaluationLane          string
	DesignatedModelRole     string
	Message                 string
	GradingMethod           evalv1.EvaluationGradingMethod
	GoldSummary             *ChatProbeGoldSummary
	Seed                    *harnessclient.EnsembleInvestigationSeed
	Workspace               *ScenarioWorkspace
}

// ChatProbeGoldSummary carries private gold criteria for semantic judge grading.
type ChatProbeGoldSummary struct {
	UserPrompt       string
	ExpectedBehavior string
	RequiredConcepts []string
	ExpectedTools    []string
	ForbiddenTools   []string
}

// BuildChatProbeRequest constructs the canonical ensemble chat request for a
// Phase 1A chat-path probe. dataOperatorID and dataOperatorSessionID bind the
// governed tool Operator used for case creation and model-originated actions.
func BuildChatProbeRequest(req ChatProbeRequest, dataOperatorID, dataOperatorSessionID string) (harnessclient.EnsembleChatRequest, error) {
	if dataOperatorID == "" || dataOperatorSessionID == "" {
		return harnessclient.EnsembleChatRequest{}, fmt.Errorf("evaluation: build chat probe request: data operator binding is required")
	}
	if req.AssignmentID == "" || req.EvaluationAttemptID == "" || req.Model == "" || req.TargetOperatorSessionID == "" {
		return harnessclient.EnsembleChatRequest{}, fmt.Errorf("evaluation: build chat probe request: %w", constants.ErrMissingRequiredField)
	}
	if req.CampaignID == "" || req.RunID == "" || req.ScenarioID == "" || req.ModelRegistryDigest == "" || len(req.ModelRegistry) == 0 || req.ModelDigest == "" {
		return harnessclient.EnsembleChatRequest{}, fmt.Errorf("evaluation: build chat probe request: %w", constants.ErrInferenceModelRegistryInvalid)
	}
	lane := strings.TrimSpace(req.EvaluationLane)
	if lane == "" {
		lane = "system"
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		message = "Reply with exactly: chat-system-ok"
	}
	evalContext := harnessclient.EnsembleEvaluationContext{
		CampaignID:              req.CampaignID,
		RunID:                   req.RunID,
		AssignmentID:            req.AssignmentID,
		EvaluationAttemptID:     req.EvaluationAttemptID,
		ScenarioID:              req.ScenarioID,
		ModelRegistryDigest:     req.ModelRegistryDigest,
		ModelRegistry:           toEnsembleModelVariants(req.ModelRegistry),
		TargetOperatorSessionID: req.TargetOperatorSessionID,
		EvaluationLane:          lane,
	}
	if lane == "model_role" {
		if req.DesignatedModelRole == "" {
			return harnessclient.EnsembleChatRequest{}, fmt.Errorf("evaluation: build chat probe request: designated model role required for model_role lane")
		}
		evalContext.DesignatedModelRole = req.DesignatedModelRole
	}
	evalContext.GradingMethod = chatGradingMethodLabel(req.GradingMethod)
	if req.GoldSummary != nil {
		evalContext.GoldSummary = &harnessclient.EnsembleEvaluationGoldSummary{
			UserPrompt:       req.GoldSummary.UserPrompt,
			ExpectedBehavior: req.GoldSummary.ExpectedBehavior,
			RequiredConcepts: nonNullStringSlice(req.GoldSummary.RequiredConcepts),
			ExpectedTools:    nonNullStringSlice(req.GoldSummary.ExpectedTools),
			ForbiddenTools:   nonNullStringSlice(req.GoldSummary.ForbiddenTools),
		}
	}
	evalContext.Seed = req.Seed
	if req.Workspace != nil {
		evalContext.Workspace = &harnessclient.EnsembleEvaluationWorkspace{
			Root:                     req.Workspace.Root,
			OperatorWorkingDirectory: req.Workspace.OperatorWorkingDirectory,
		}
	}
	// The case title is model-visible, so it is never a harness constant: a
	// seeded request carries the scenario-authored title, and an unseeded one
	// leaves it empty so g8ee titles the case from the message as it does in
	// production.
	caseTitle := ""
	if req.Seed != nil {
		caseTitle = req.Seed.CaseTitle
	}
	return harnessclient.EnsembleChatRequest{
		Context: harnessclient.EnsembleRequestContext{
			SourceComponent:   "CLIENT",
			OperatorID:        dataOperatorID,
			OperatorSessionID: dataOperatorSessionID,
			BoundOperators: []harnessclient.EnsembleBoundOperator{{
				OperatorID:        dataOperatorID,
				OperatorSessionID: dataOperatorSessionID,
				Status:            string(constants.OperatorStatusBound),
			}},
		},
		Message:              message,
		SentinelMode:         true,
		ResourceCreation:     &harnessclient.EnsembleResourceCreation{CreateCase: true, CaseTitle: caseTitle},
		EvaluationContext:    &evalContext,
		LLMPrimaryProvider:   "g8e",
		LLMPrimaryModel:      req.Model,
		LLMAssistantProvider: "g8e",
		LLMAssistantModel:    req.Model,
		LLMLiteProvider:      "g8e",
		LLMLiteModel:         req.Model,
	}, nil
}

func toEnsembleModelVariants(variants []*operatorv1.InferenceModelVariant) []harnessclient.EnsembleModelVariant {
	out := make([]harnessclient.EnsembleModelVariant, 0, len(variants))
	for _, variant := range variants {
		if variant == nil {
			continue
		}
		out = append(out, harnessclient.EnsembleModelVariant{
			Model:  variant.GetModel(),
			Digest: variant.GetDigest(),
		})
	}
	return out
}

// ValidateChatProbeTrace verifies a persisted evaluation assignment trace
// imported from g8ee after chat completion.
func ValidateChatProbeTrace(req ChatProbeRequest, trace EvaluationTrace) error {
	if len(trace) == 0 {
		return fmt.Errorf("evaluation: validate chat probe trace: %w", constants.ErrMissingRequiredField)
	}
	status, _ := trace["status"].(string)
	if status != "completed" && status != "failed" {
		return fmt.Errorf("evaluation: validate chat probe trace: trace status %q is not terminal", status)
	}
	if status != "completed" {
		reason, _ := trace["error"].(string)
		if reason == "" {
			return fmt.Errorf("evaluation: validate chat probe trace: assignment failed without a recorded error")
		}
		return fmt.Errorf("evaluation: validate chat probe trace: assignment failed: %s", reason)
	}
	if err := validateTraceDigest(trace); err != nil {
		return err
	}
	evalContext, ok := evaluationTrace(trace["evaluation_context"])
	if !ok {
		return fmt.Errorf("evaluation: validate chat probe trace: missing evaluation_context")
	}
	if err := validateTraceEvaluationContext(req, evalContext); err != nil {
		return err
	}
	if chatExecutionID, _ := trace["chat_execution_id"].(string); chatExecutionID == "" {
		return fmt.Errorf("evaluation: validate chat probe trace: missing chat_execution_id")
	}
	if completedAt, _ := trace["completed_at"].(string); completedAt == "" {
		return fmt.Errorf("evaluation: validate chat probe trace: missing completed_at")
	}
	modelCalls, ok := trace["model_calls"].([]any)
	if !ok || len(modelCalls) == 0 {
		return fmt.Errorf("evaluation: validate chat probe trace: missing model_calls")
	}
	if err := validateGovernedModelCalls(modelCalls); err != nil {
		return err
	}
	return nil
}

func validateTraceDigest(trace EvaluationTrace) error {
	digest, _ := trace["trace_digest"].(string)
	if digest == "" {
		return fmt.Errorf("evaluation: validate chat probe trace: missing trace_digest")
	}
	expected, err := ComputeChatProbeTraceDigest(trace)
	if err != nil {
		return err
	}
	if digest != expected {
		return fmt.Errorf("evaluation: validate chat probe trace: trace digest mismatch")
	}
	return nil
}

// canonicalSeedBytes canonicalizes a seed after decoding it into the typed wire
// seed. g8ee records the seed through its own model, which adds defaults the
// harness never sent (an empty case_description, empty lists), so the sent seed
// and its echo are compared as the typed value both decode to, not as raw JSON.
func canonicalSeedBytes(seed any) ([]byte, error) {
	raw, err := json.Marshal(seed)
	if err != nil {
		return nil, err
	}
	var typed harnessclient.EnsembleInvestigationSeed
	if err := json.Unmarshal(raw, &typed); err != nil {
		return nil, err
	}
	return canonicalJSONBytes(typed)
}

func validateTraceEvaluationContext(req ChatProbeRequest, evalContext EvaluationTrace) error {
	checks := map[string]string{
		"campaign_id":                req.CampaignID,
		"run_id":                     req.RunID,
		"assignment_id":              req.AssignmentID,
		"evaluation_attempt_id":      req.EvaluationAttemptID,
		"scenario_id":                req.ScenarioID,
		"model_registry_digest":      req.ModelRegistryDigest,
		"target_operator_session_id": req.TargetOperatorSessionID,
	}
	for field, want := range checks {
		got, _ := evalContext[field].(string)
		if got != want {
			return fmt.Errorf("evaluation: validate chat probe trace: evaluation_context.%s=%q, want %q", field, got, want)
		}
	}
	lane, _ := evalContext["evaluation_lane"].(string)
	if lane == "" {
		lane = "system"
	}
	wantLane := req.EvaluationLane
	if wantLane == "" {
		wantLane = "system"
	}
	if lane != wantLane {
		return fmt.Errorf("evaluation: validate chat probe trace: evaluation_context.evaluation_lane=%q, want %q", lane, wantLane)
	}
	if wantLane == "model_role" {
		gotRole, _ := evalContext["designated_model_role"].(string)
		if gotRole != req.DesignatedModelRole {
			return fmt.Errorf("evaluation: validate chat probe trace: evaluation_context.designated_model_role=%q, want %q", gotRole, req.DesignatedModelRole)
		}
	}
	if req.Seed != nil {
		rawSeed, hasSeed := evalContext["seed"]
		if !hasSeed || rawSeed == nil {
			return fmt.Errorf("evaluation: validate chat probe trace: evaluation_context.seed is required")
		}
		reqBytes, err := canonicalSeedBytes(req.Seed)
		if err != nil {
			return fmt.Errorf("evaluation: validate chat probe trace: canonicalize request seed: %w", err)
		}
		traceBytes, err := canonicalSeedBytes(rawSeed)
		if err != nil {
			return fmt.Errorf("evaluation: validate chat probe trace: canonicalize trace seed: %w", err)
		}
		if !bytes.Equal(reqBytes, traceBytes) {
			return fmt.Errorf("evaluation: validate chat probe trace: evaluation_context.seed mismatch")
		}
	}
	if req.Workspace != nil {
		rawWs, hasWs := evalContext["workspace"]
		if !hasWs || rawWs == nil {
			return fmt.Errorf("evaluation: validate chat probe trace: evaluation_context.workspace is required")
		}
		wsMap, ok := evaluationTrace(rawWs)
		if !ok {
			return fmt.Errorf("evaluation: validate chat probe trace: evaluation_context.workspace must be an object")
		}
		traceWs := ScenarioWorkspace{
			Root:                     stringValue(wsMap["root"]),
			OperatorWorkingDirectory: stringValue(wsMap["operator_working_directory"]),
		}
		if traceWs.Root != req.Workspace.Root || traceWs.OperatorWorkingDirectory != req.Workspace.OperatorWorkingDirectory {
			return fmt.Errorf("evaluation: validate chat probe trace: evaluation_context.workspace mismatch")
		}
		if err := traceWs.Validate(req.RunID, req.EvaluationAttemptID); err != nil {
			return fmt.Errorf("evaluation: validate chat probe trace: %w", err)
		}
	} else if rawWs, hasWs := evalContext["workspace"]; hasWs && rawWs != nil {
		if wsMap, ok := evaluationTrace(rawWs); ok {
			traceWs := ScenarioWorkspace{
				Root:                     stringValue(wsMap["root"]),
				OperatorWorkingDirectory: stringValue(wsMap["operator_working_directory"]),
			}
			if err := traceWs.Validate(req.RunID, req.EvaluationAttemptID); err != nil {
				return fmt.Errorf("evaluation: validate chat probe trace: %w", err)
			}
		}
	}
	return nil
}

func validateGovernedModelCalls(modelCalls []any) error {
	foundGoverned := false
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
		foundGoverned = true
		for _, field := range []string{"governed_transaction_id", "governed_result_digest", "provider_attempt_id"} {
			value, _ := call[field].(string)
			if value == "" {
				return fmt.Errorf("evaluation: validate chat probe trace: governed model call missing %s", field)
			}
		}
	}
	if !foundGoverned {
		return fmt.Errorf("evaluation: validate chat probe trace: no governed model calls recorded")
	}
	return nil
}

func hasAgentRole(modelCalls []any, agentRole string) bool {
	for _, rawCall := range modelCalls {
		call, ok := evaluationTrace(rawCall)
		if !ok {
			continue
		}
		if role, _ := call["agent_role"].(string); role == agentRole {
			return true
		}
	}
	return false
}

func hasDesignatedRoleOutcome(trace EvaluationTrace, designatedRole string) bool {
	roleOutcome, _ := trace["role_outcome"].(string)
	return roleOutcome == "invoked" && designatedRole != ""
}

func hasControlledRoleAssignment(trace EvaluationTrace, designatedRole string) bool {
	assignment, ok := evaluationTrace(trace["controlled_role_assignment"])
	if !ok {
		return false
	}
	gotRole, _ := assignment["designated_model_role"].(string)
	return gotRole == designatedRole
}
