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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// CampaignExecutionBinding pins the exact governed execution authorities for one
// scored assignment submitted through production POST /api/v1/chat.
type CampaignExecutionBinding struct {
	InferenceOperatorSessionID   string
	DataOperatorID               string
	DataOperatorSessionID        string
	DataOperatorWorkingDirectory string
	ModelRegistryDigest          string
	ModelRegistry                []*operatorv1.InferenceModelVariant
}

// BuildCampaignChatRequest constructs the production chat request for one
// homogeneous model-role assignment using the frozen scenario input fixture.
// ws renders the prompt and seed and is echoed on the request. The executor
// derives it from the binding's working directory and fails closed when that
// is missing; import and verification pass the workspace recorded in the trace
// (nil for a trace that carries none), so the binding's working directory is
// not required here.
func BuildCampaignChatRequest(assignment *evalv1.EvaluationAssignment, attemptID string, input ScenarioInputFixture, binding CampaignExecutionBinding, grading CampaignChatGradingContext, ws *ScenarioWorkspace) (ChatProbeRequest, error) {
	if assignment == nil || attemptID == "" || binding.InferenceOperatorSessionID == "" || binding.DataOperatorID == "" || binding.DataOperatorSessionID == "" {
		return ChatProbeRequest{}, fmt.Errorf("evaluation: build campaign chat request: %w", constants.ErrMissingRequiredField)
	}
	if IsHeterogeneousAssignment(assignment) {
		return ChatProbeRequest{}, fmt.Errorf("evaluation: build campaign chat request: heterogeneous assignments execute through formation runner")
	}
	homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous)
	if !ok || homogeneous.Homogeneous == nil || homogeneous.Homogeneous.GetCandidateVariant() == nil {
		return ChatProbeRequest{}, fmt.Errorf("evaluation: build campaign chat request: homogeneous target required")
	}
	role, err := modelCampaignRoleLabel(homogeneous.Homogeneous.GetDesignatedRole())
	if err != nil {
		return ChatProbeRequest{}, err
	}
	variant := homogeneous.Homogeneous.GetCandidateVariant()
	if input.UserPrompt == "" {
		return ChatProbeRequest{}, fmt.Errorf("evaluation: build campaign chat request: scenario %s missing user prompt", assignment.GetScenarioId())
	}
	message := renderScenarioMessage(input.UserPrompt, input.InlineContext, ws)
	harnessSeed := buildHarnessInvestigationSeed(&input.Seed, ws)
	return ChatProbeRequest{
		AssignmentID:            assignment.GetAssignmentId(),
		EvaluationAttemptID:     attemptID,
		CampaignID:              assignment.GetCampaignId(),
		RunID:                   assignment.GetRunId(),
		ScenarioID:              assignment.GetScenarioId(),
		Model:                   variant.GetServedModelTag(),
		ModelDigest:             variant.GetModelDigest(),
		TargetOperatorSessionID: binding.InferenceOperatorSessionID,
		ModelRegistryDigest:     binding.ModelRegistryDigest,
		ModelRegistry:           binding.ModelRegistry,
		EvaluationLane:          "model_role",
		DesignatedModelRole:     role,
		Message:                 message,
		GradingMethod:           grading.GradingMethod,
		GoldSummary:             buildChatProbeGoldSummary(message, grading),
		Seed:                    harnessSeed,
		Workspace:               ws,
	}, nil
}

func buildChatProbeGoldSummary(message string, grading CampaignChatGradingContext) *ChatProbeGoldSummary {
	if grading.GradingMethod != evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE {
		return nil
	}
	return &ChatProbeGoldSummary{
		UserPrompt:       message,
		ExpectedBehavior: grading.ScenarioGold.ExpectedBehavior,
		RequiredConcepts: nonNullStringSlice(grading.RequiredConcepts),
		ExpectedTools:    nonNullStringSlice(grading.ScenarioTools.ExpectedTools),
		ForbiddenTools:   nonNullStringSlice(grading.ScenarioTools.ForbiddenTools),
	}
}

// buildHarnessInvestigationSeed converts a frozen fixture seed to the wire seed,
// rendering every string through the workspace. A fixture without a case title
// carries no seed (catalog 1.0.0 fixtures predate seeds, and every 1.1.0
// scenario has a title), so the request sends none and the trace is not
// expected to echo one.
func buildHarnessInvestigationSeed(seed *InvestigationSeed, ws *ScenarioWorkspace) *harnessclient.EnsembleInvestigationSeed {
	if seed == nil || seed.CaseTitle == "" {
		return nil
	}
	harnesseSeed := &harnessclient.EnsembleInvestigationSeed{
		CaseTitle:       seed.CaseTitle,
		CaseDescription: seed.CaseDescription,
	}
	if ws != nil {
		harnesseSeed.CaseTitle = ws.Render(harnesseSeed.CaseTitle)
		harnesseSeed.CaseDescription = ws.Render(harnesseSeed.CaseDescription)
	}
	for _, turn := range seed.Turns {
		content := turn.Content
		if ws != nil {
			content = ws.Render(content)
		}
		harnesseSeed.Turns = append(harnesseSeed.Turns, harnessclient.EnsembleSeedTurn{
			Sender:  turn.Sender,
			Content: content,
		})
	}
	for _, event := range seed.HistoryEvents {
		summary := event.Summary
		arguments := event.ArgumentsJSON
		command := event.Command
		errorText := event.Error
		if ws != nil {
			summary = ws.Render(summary)
			arguments = ws.Render(arguments)
			command = ws.Render(command)
			errorText = ws.Render(errorText)
		}
		harnesseSeed.HistoryEvents = append(harnesseSeed.HistoryEvents, harnessclient.EnsembleSeedHistoryEvent{
			EventType:     event.EventType,
			Actor:         event.Actor,
			Summary:       summary,
			ToolName:      event.ToolName,
			ExecutionID:   event.ExecutionID,
			ArgumentsJSON: arguments,
			Command:       command,
			Error:         errorText,
			ErrorType:     event.ErrorType,
		})
	}
	if seed.CaseMemory != nil {
		cm := seed.CaseMemory
		memory := &harnessclient.EnsembleSeedMemory{
			InvestigationSummary:     cm.InvestigationSummary,
			CommunicationPreferences: cm.CommunicationPreferences,
			TechnicalBackground:      cm.TechnicalBackground,
			ResponseStyle:            cm.ResponseStyle,
			ProblemSolvingApproach:   cm.ProblemSolvingApproach,
			InteractionStyle:         cm.InteractionStyle,
		}
		if ws != nil {
			memory.InvestigationSummary = ws.Render(memory.InvestigationSummary)
			memory.CommunicationPreferences = ws.Render(memory.CommunicationPreferences)
			memory.TechnicalBackground = ws.Render(memory.TechnicalBackground)
			memory.ResponseStyle = ws.Render(memory.ResponseStyle)
			memory.ProblemSolvingApproach = ws.Render(memory.ProblemSolvingApproach)
			memory.InteractionStyle = ws.Render(memory.InteractionStyle)
		}
		harnesseSeed.CaseMemory = memory
	}
	return harnesseSeed
}

func nonNullStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func modelCampaignRoleLabel(role evalv1.ModelCampaignRole) (string, error) {
	switch role {
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY:
		return "primary", nil
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT:
		return "assistant", nil
	case evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE:
		return "lite", nil
	default:
		return "", fmt.Errorf("evaluation: unsupported model campaign role %s", role.String())
	}
}

// ScenarioToolExpectations carries frozen scenario tool constraints for grading.
type ScenarioToolExpectations struct {
	AllowedTools     []string
	ExpectedTools    []string
	ForbiddenTools   []string
	TrajectoryPolicy evalv1.EvaluationTrajectoryPolicy
}

// CampaignChatGradingContext carries private grading inputs for one chat assignment.
type CampaignChatGradingContext struct {
	GradingMethod    evalv1.EvaluationGradingMethod
	ScenarioGold     ScenarioGoldCriteria
	ScenarioTools    ScenarioToolExpectations
	RequiredConcepts []string
}

// AssignmentExecutionRequest carries one resumable controller execution attempt.
type AssignmentExecutionRequest struct {
	Assignment       *evalv1.EvaluationAssignment
	AttemptID        string
	ScenarioInput    ScenarioInputFixture
	ScenarioGold     ScenarioGoldCriteria
	ScenarioTools    ScenarioToolExpectations
	RequiredConcepts []string
	GradingMethod    evalv1.EvaluationGradingMethod
	Binding          CampaignExecutionBinding
	// OnTraceProgress is optional. When set, the chat executor invokes it after
	// each non-terminal trace poll so publication can emit scored-inference live
	// events before the assignment reaches a terminal result.
	OnTraceProgress func(ctx context.Context, trace EvaluationTrace) error
	// OnFormationRoleStarting is optional. When set, the heterogeneous formation
	// executor invokes it before each role executes so one planned invocation row
	// reaches the live feed instead of batching all roles at assignment start.
	OnFormationRoleStarting func(ctx context.Context, role FormationRole) error
	// OnFormationRoleProgress is optional. When set, the heterogeneous formation
	// executor invokes it after each completed role so role telemetry reaches the
	// live feed before the terminal assignment result exists.
	OnFormationRoleProgress func(ctx context.Context, result *FormationRunResult) error
}

// CampaignAssignmentExecutor submits one scored assignment through production chat
// and returns the terminal assignment result derived from persisted trace state.
type CampaignAssignmentExecutor interface {
	ExecuteAssignment(ctx context.Context, req AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error)
}

// StubHomogeneousAssignmentModelInferences returns minimal reported inference rows
// for homogeneous assignment stubs used in publication and export tests.
func StubHomogeneousAssignmentModelInferences(assignment *evalv1.EvaluationAssignment) []*evalv1.ModelInferenceRecord {
	if assignment == nil {
		return nil
	}
	homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous)
	if !ok || homogeneous.Homogeneous == nil {
		return nil
	}
	role := homogeneous.Homogeneous.GetDesignatedRole()
	if role == evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_UNSPECIFIED {
		role = evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY
	}
	variant := homogeneous.Homogeneous.GetCandidateVariant()
	if variant == nil {
		variant = &evalv1.ModelVariant{VariantId: "qwen3-4b"}
	}
	return []*evalv1.ModelInferenceRecord{{
		InferenceRecordId: "stub-inference",
		ModelRole:         role,
		ModelVariant:      variant,
		AgentPersona:      "sage",
		UsageAvailability: evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
	}}
}

// InferenceVariantsFromEvalRegistry converts frozen eval model variants into the
// governed inference registry shape used by production chat requests.
func InferenceVariantsFromEvalRegistry(variants []*evalv1.ModelVariant) []*operatorv1.InferenceModelVariant {
	out := make([]*operatorv1.InferenceModelVariant, 0, len(variants))
	for _, variant := range variants {
		if variant == nil {
			continue
		}
		out = append(out, &operatorv1.InferenceModelVariant{
			Model:  variant.GetServedModelTag(),
			Digest: variant.GetModelDigest(),
		})
	}
	return out
}

// CampaignModelBinding describes one frozen served model tag and digest pair.
type CampaignModelBinding struct {
	ServedModelTag string
	ModelDigest    string
}

// CampaignModelBindingsFromFormation returns provenance preflight bindings for
// the sovereign roles in one bound formation. Delegated roles are omitted.
func CampaignModelBindingsFromFormation(formation Formation) []CampaignModelBinding {
	bindings := make([]CampaignModelBinding, 0, 3)
	for _, model := range []FormationModel{formation.Primary, formation.Assistant, formation.Lite} {
		if model.Trust == FormationTrustDelegated || model.ServedModelTag == "" || model.ModelDigest == "" {
			continue
		}
		if model.ModelDigest == FormationDelegatedRegistryDigestPlaceholder {
			continue
		}
		bindings = append(bindings, CampaignModelBinding{
			ServedModelTag: model.ServedModelTag,
			ModelDigest:    model.ModelDigest,
		})
	}
	return bindings
}

// CampaignModelBindingsFromSpec returns attestation bindings for every frozen
// model variant in one campaign spec.
func CampaignModelBindingsFromSpec(spec *evalv1.EvaluationCampaignSpec) ([]CampaignModelBinding, error) {
	if spec == nil {
		return nil, fmt.Errorf("evaluation: campaign model bindings: %w", constants.ErrMissingRequiredField)
	}
	variants := spec.GetModelRegistry()
	if len(variants) == 0 {
		return nil, fmt.Errorf("evaluation: campaign model bindings: empty model registry")
	}
	bindings := make([]CampaignModelBinding, 0, len(variants))
	for _, variant := range variants {
		if variant == nil || variant.GetServedModelTag() == "" || variant.GetModelDigest() == "" {
			return nil, fmt.Errorf("evaluation: campaign model bindings: %w", constants.ErrMissingRequiredField)
		}
		bindings = append(bindings, CampaignModelBinding{
			ServedModelTag: variant.GetServedModelTag(),
			ModelDigest:    variant.GetModelDigest(),
		})
	}
	return bindings, nil
}
