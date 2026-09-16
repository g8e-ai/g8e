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
		RequiredConcepts: append([]string(nil), grading.RequiredConcepts...),
		ExpectedTools:    append([]string(nil), grading.ScenarioTools.ExpectedTools...),
		ForbiddenTools:   append([]string(nil), grading.ScenarioTools.ForbiddenTools...),
	}
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
	Assignment     *evalv1.EvaluationAssignment
	AttemptID      string
	ScenarioInput  ScenarioInputFixture
	ScenarioGold     ScenarioGoldCriteria
	ScenarioTools    ScenarioToolExpectations
	RequiredConcepts []string
	GradingMethod    evalv1.EvaluationGradingMethod
	Binding          CampaignExecutionBinding
}

// CampaignAssignmentExecutor submits one scored assignment through production chat
// and returns the terminal assignment result derived from persisted trace state.
type CampaignAssignmentExecutor interface {
	ExecuteAssignment(ctx context.Context, req AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error)
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
