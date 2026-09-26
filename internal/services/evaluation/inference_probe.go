// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/protocol"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// InferenceProbeRequest carries a non-scored governed inference probe through
// the exact Inference Operator session. It is not a scored campaign evaluation.
type InferenceProbeRequest struct {
	ProviderAttemptID       string
	Role                    models.InferenceModelRole
	Model                   string
	ModelDigest             string
	TargetOperatorSessionID string
	Prompt                  string
	Messages                []*operatorv1.InferenceMessage
	Tools                   []*operatorv1.InferenceToolDeclaration
	ToolChoice              *operatorv1.InferenceToolChoice
	ResponseFormat          *operatorv1.InferenceResponseFormat
	CampaignID              string
	ModelRegistryDigest     string
	ModelRegistry           []*operatorv1.InferenceModelVariant
	Seed                    *int32
	Stream                  bool
	Thinking                *operatorv1.InferenceThinkingControl
}

// BuildInferenceProbeDispatchRequest constructs the canonical governed dispatch
// request for a unary or streaming probe.
func BuildInferenceProbeDispatchRequest(req InferenceProbeRequest) (*operatorv1.InferenceDispatchRequest, error) {
	if req.ProviderAttemptID == "" || req.Model == "" || req.TargetOperatorSessionID == "" {
		return nil, fmt.Errorf("evaluation: build inference probe request: %w", constants.ErrMissingRequiredField)
	}
	if req.Role == models.InferenceModelRoleUnspecified {
		return nil, fmt.Errorf("evaluation: build inference probe request: %w", constants.ErrInferenceRoleInvalid)
	}
	messages := req.Messages
	if len(messages) == 0 {
		prompt := req.Prompt
		if prompt == "" {
			prompt = "Reply with exactly: probe-ok"
		}
		messages = []*operatorv1.InferenceMessage{{
			Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
			Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: prompt}}},
		}}
	}
	dispatch := &operatorv1.InferenceDispatchRequest{
		RequestSchemaVersion:    constants.InferenceRequestSchemaVersion,
		Role:                    req.Role.ToProto(),
		Model:                   req.Model,
		Messages:                messages,
		Tools:                   req.Tools,
		ToolChoice:              req.ToolChoice,
		ResponseFormat:          req.ResponseFormat,
		ProviderAttemptId:       req.ProviderAttemptID,
		TargetOperatorSessionId: req.TargetOperatorSessionID,
		ActingAppId:             protocol.EnsembleAppID,
		CampaignId:              req.CampaignID,
		ModelRegistryDigest:     req.ModelRegistryDigest,
		ModelRegistry:           req.ModelRegistry,
		ModelDigest:             req.ModelDigest,
		Stream:                  req.Stream,
		Thinking:                req.Thinking,
	}
	if req.Seed != nil {
		dispatch.Seed = req.Seed
	}
	hasCampaignAuthority := req.CampaignID != "" || req.ModelRegistryDigest != "" || len(req.ModelRegistry) > 0
	if hasCampaignAuthority {
		if req.CampaignID == "" || req.ModelRegistryDigest == "" || len(req.ModelRegistry) == 0 || req.ModelDigest == "" {
			return nil, fmt.Errorf("evaluation: build inference probe request: %w", constants.ErrInferenceModelRegistryInvalid)
		}
		digest, err := models.ComputeInferenceModelRegistryDigest(req.CampaignID, req.ModelRegistry)
		if err != nil || digest != req.ModelRegistryDigest {
			return nil, fmt.Errorf("evaluation: build inference probe request: %w", constants.ErrInferenceModelRegistryInvalid)
		}
		dispatch.RunId = "probe-run"
		dispatch.AssignmentId = "probe-assignment"
		dispatch.EvaluationAttemptId = req.ProviderAttemptID
		dispatch.ScenarioId = "probe-scenario"
	}
	return dispatch, nil
}

// ValidateInferenceProbeResponse verifies the governed probe returned a receipt,
// result digest binding, and identity fields matching the submitted request.
func ValidateInferenceProbeResponse(req InferenceProbeRequest, resp *operatorv1.InferenceDispatchResponse) error {
	if resp == nil || resp.GetResult() == nil || resp.GetReceipt() == nil {
		return fmt.Errorf("evaluation: validate inference probe response: %w", constants.ErrMissingRequiredField)
	}
	result := resp.GetResult()
	receipt := resp.GetReceipt()
	if result.GetProviderAttemptId() != req.ProviderAttemptID {
		return fmt.Errorf("evaluation: validate inference probe response: %w", constants.ErrIdentityBindingFailed)
	}
	if result.GetRequestedModel() != req.Model {
		return fmt.Errorf("evaluation: validate inference probe response: %w", constants.ErrInferenceModelOverrideDenied)
	}
	if result.GetResultDigest() == "" || receipt.GetResultSummary() != result.GetResultDigest() {
		return fmt.Errorf("evaluation: validate inference probe response: %w", constants.ErrInferenceResultDigest)
	}
	if len(result.GetParts()) == 0 {
		return fmt.Errorf("evaluation: validate inference probe response: %w", constants.ErrInferenceProviderResponseInvalid)
	}
	if req.CampaignID != "" && result.GetCampaignId() != req.CampaignID {
		return fmt.Errorf("evaluation: validate inference probe response: %w", constants.ErrInferenceModelRegistryInvalid)
	}
	if req.ModelRegistryDigest != "" && result.GetModelRegistryDigest() != req.ModelRegistryDigest {
		return fmt.Errorf("evaluation: validate inference probe response: %w", constants.ErrInferenceModelRegistryInvalid)
	}
	return nil
}

// ValidateInferenceProbeStream verifies live progress events bind to the terminal
// governed result after a streaming probe completes.
func ValidateInferenceProbeStream(req InferenceProbeRequest, progress []*operatorv1.InferenceProgressEvent, resp *operatorv1.InferenceDispatchResponse) error {
	if err := ValidateInferenceProbeResponse(req, resp); err != nil {
		return err
	}
	if !req.Stream {
		return nil
	}
	if len(progress) == 0 {
		return fmt.Errorf("evaluation: validate inference probe stream: %w", constants.ErrInferenceProgressHashMismatch)
	}
	return models.ReconcileInferenceProgress(progress, resp.GetResult())
}

// ProbeEchoToolDeclaration is the canonical probe tool used by Phase 1A acceptance cases.
func ProbeEchoToolDeclaration() *operatorv1.InferenceToolDeclaration {
	return &operatorv1.InferenceToolDeclaration{
		Name:        "probe_echo",
		Description: "Echo a short message for governed tool-call acceptance probes.",
		JsonSchema:  `{"additionalProperties":false,"properties":{"message":{"type":"string"}},"required":["message"],"type":"object"}`,
	}
}

// ProbeStructuredResponseFormat is the canonical structured-output schema for Phase 1A acceptance.
func ProbeStructuredResponseFormat() *operatorv1.InferenceResponseFormat {
	return &operatorv1.InferenceResponseFormat{
		MediaType:  "application/json",
		JsonSchema: `{"additionalProperties":false,"properties":{"answer":{"type":"string"}},"required":["answer"],"type":"object"}`,
	}
}
