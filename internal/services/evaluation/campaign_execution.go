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
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// CampaignExecutionBinding pins the exact governed execution authorities for one
// scored assignment submitted through production POST /api/v1/chat.
type CampaignExecutionBinding struct {
	InferenceOperatorSessionID string
	DataOperatorID             string
	DataOperatorSessionID      string
	ModelRegistryDigest        string
	ModelRegistry              []*operatorv1.InferenceModelVariant
}

// BuildCampaignChatRequest constructs the production chat request for one
// homogeneous model-role assignment using the frozen scenario input fixture.
func BuildCampaignChatRequest(assignment *evalv1.EvaluationAssignment, attemptID string, input ScenarioInputFixture, binding CampaignExecutionBinding, grading CampaignChatGradingContext) (ChatProbeRequest, error) {
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
	message := input.UserPrompt
	if message == "" {
		return ChatProbeRequest{}, fmt.Errorf("evaluation: build campaign chat request: scenario %s missing user prompt", assignment.GetScenarioId())
	}
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
		GoldSummary:             buildChatProbeGoldSummary(input, grading),
	}, nil
}

func buildChatProbeGoldSummary(input ScenarioInputFixture, grading CampaignChatGradingContext) *ChatProbeGoldSummary {
	if grading.GradingMethod != evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE {
		return nil
	}
	return &ChatProbeGoldSummary{
		UserPrompt:       input.UserPrompt,
		ExpectedBehavior: grading.ScenarioGold.ExpectedBehavior,
		RequiredConcepts: nonNullStringSlice(grading.RequiredConcepts),
		ExpectedTools:    nonNullStringSlice(grading.ScenarioTools.ExpectedTools),
		ForbiddenTools:   nonNullStringSlice(grading.ScenarioTools.ForbiddenTools),
	}
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
	ExpectedTools  []string
	ForbiddenTools []string
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
