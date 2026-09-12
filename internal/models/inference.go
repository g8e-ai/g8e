// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

// InferenceModelRole is the typed chat-tier role for a governed inference
// request. It maps directly to the three tiers in ensemble's LLMSettings.
// Distinct from the eval-campaign ModelRole (string) in observe.go.
type InferenceModelRole int32

const (
	InferenceModelRoleUnspecified InferenceModelRole = 0
	InferenceModelRolePrimary     InferenceModelRole = 1
	InferenceModelRoleAssistant   InferenceModelRole = 2
	InferenceModelRoleLite        InferenceModelRole = 3
)

// ToProto converts the typed InferenceModelRole to the protobuf enum.
func (r InferenceModelRole) ToProto() operatorv1.ModelRole {
	switch r {
	case InferenceModelRolePrimary:
		return operatorv1.ModelRole_MODEL_ROLE_PRIMARY
	case InferenceModelRoleAssistant:
		return operatorv1.ModelRole_MODEL_ROLE_ASSISTANT
	case InferenceModelRoleLite:
		return operatorv1.ModelRole_MODEL_ROLE_LITE
	default:
		return operatorv1.ModelRole_MODEL_ROLE_UNSPECIFIED
	}
}

// InferenceModelRoleFromProto converts the protobuf enum to the typed
// InferenceModelRole.
func InferenceModelRoleFromProto(r operatorv1.ModelRole) InferenceModelRole {
	switch r {
	case operatorv1.ModelRole_MODEL_ROLE_PRIMARY:
		return InferenceModelRolePrimary
	case operatorv1.ModelRole_MODEL_ROLE_ASSISTANT:
		return InferenceModelRoleAssistant
	case operatorv1.ModelRole_MODEL_ROLE_LITE:
		return InferenceModelRoleLite
	default:
		return InferenceModelRoleUnspecified
	}
}

// GenerateRequest carries the Ollama model name (which encodes the role), the
// scrubbed prompt, and generation parameters. The handler receives only
// scrubbed, tokenized prompts; it never receives raw vault material.
type GenerateRequest struct {
	Role        InferenceModelRole
	Model       string
	Prompt      string
	Temperature float32
	MaxTokens   int32
	KeepAlive   string
}

// GenerateResponse carries the generated text, usage metadata, and finish
// reason returned by the backend.
type GenerateResponse struct {
	Text             string
	PromptTokens     int32
	CompletionTokens int32
	TotalTokens      int32
	FinishReason     string
	Model            string
}

// BackendStatus reports the backend's readiness and the models available in
// its store.
type BackendStatus struct {
	Available bool
	Models    []string
}

// InferenceRequestPayload is the typed governed inference payload decoded from
// the protobuf InferenceRequested message. The handler decodes this from the
// governed envelope, scrubs the prompt, and calls Backend.Generate.
type InferenceRequestPayload struct {
	Role        InferenceModelRole
	Model       string
	Prompt      string
	Temperature float32
	MaxTokens   int32
	KeepAlive   string
}

// InferenceResultPayload is the typed result payload returned by the handler
// for the receipt and audit chain.
type InferenceResultPayload struct {
	Text             string `json:"text"`
	PromptTokens     int32  `json:"prompt_tokens"`
	CompletionTokens int32  `json:"completion_tokens"`
	TotalTokens      int32  `json:"total_tokens"`
	FinishReason     string `json:"finish_reason"`
	Model            string `json:"model"`
}

// FromProtoInferenceRequested decodes a protobuf InferenceRequested message
// into the typed InferenceRequestPayload.
func FromProtoInferenceRequested(req *operatorv1.InferenceRequested) InferenceRequestPayload {
	return InferenceRequestPayload{
		Role:        InferenceModelRoleFromProto(req.GetRole()),
		Model:       req.GetModel(),
		Prompt:      req.GetPrompt(),
		Temperature: req.GetTemperature(),
		MaxTokens:   req.GetMaxTokens(),
		KeepAlive:   req.GetKeepAlive(),
	}
}

// ToGenerateRequest converts an InferenceRequestPayload to a GenerateRequest
// for the backend, applying the default model for the role when the request
// does not override it.
func (p InferenceRequestPayload) ToGenerateRequest(defaultModel string) GenerateRequest {
	model := p.Model
	if model == "" {
		model = defaultModel
	}
	return GenerateRequest{
		Role:        p.Role,
		Model:       model,
		Prompt:      p.Prompt,
		Temperature: p.Temperature,
		MaxTokens:   p.MaxTokens,
		KeepAlive:   p.KeepAlive,
	}
}

// ToInferenceResultPayload converts a GenerateResponse to an InferenceResultPayload.
func (r GenerateResponse) ToInferenceResultPayload() InferenceResultPayload {
	return InferenceResultPayload{
		Text:             r.Text,
		PromptTokens:     r.PromptTokens,
		CompletionTokens: r.CompletionTokens,
		TotalTokens:      r.TotalTokens,
		FinishReason:     r.FinishReason,
		Model:            r.Model,
	}
}
