// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

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

// GenerateRequest carries the Ollama model name, ordered scrubbed conversation, callable tools, and generation parameters.
type GenerateRequest struct {
	Role        InferenceModelRole
	Model       string
	Messages    []*operatorv1.InferenceMessage
	Tools       []*operatorv1.InferenceToolDeclaration
	Temperature float32
	MaxTokens   int32
	KeepAlive   string
}

// GenerateResponse carries ordered model response parts, usage metadata, and finish reason returned by the backend.
type GenerateResponse struct {
	Parts            []*operatorv1.InferenceResponsePart
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

// InferenceRequestPayload is the typed governed inference payload decoded from the protobuf InferenceRequested message.
type InferenceRequestPayload struct {
	Role        InferenceModelRole
	Model       string
	Messages    []*operatorv1.InferenceMessage
	Tools       []*operatorv1.InferenceToolDeclaration
	Temperature float32
	MaxTokens   int32
	KeepAlive   string
}

// FromProtoInferenceRequested decodes a protobuf InferenceRequested message into an isolated typed payload.
func FromProtoInferenceRequested(req *operatorv1.InferenceRequested) InferenceRequestPayload {
	messages := make([]*operatorv1.InferenceMessage, len(req.GetMessages()))
	for i, message := range req.GetMessages() {
		messages[i] = &operatorv1.InferenceMessage{}
		proto.Merge(messages[i], message)
	}
	tools := make([]*operatorv1.InferenceToolDeclaration, len(req.GetTools()))
	for i, tool := range req.GetTools() {
		tools[i] = &operatorv1.InferenceToolDeclaration{}
		proto.Merge(tools[i], tool)
	}
	return InferenceRequestPayload{
		Role:        InferenceModelRoleFromProto(req.GetRole()),
		Model:       req.GetModel(),
		Messages:    messages,
		Tools:       tools,
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
		Messages:    p.Messages,
		Tools:       p.Tools,
		Temperature: p.Temperature,
		MaxTokens:   p.MaxTokens,
		KeepAlive:   p.KeepAlive,
	}
}

// ToProtoInferenceResult converts a GenerateResponse to the protobuf
// InferenceResult message for result envelope publishing.
func (r GenerateResponse) ToProtoInferenceResult() *operatorv1.InferenceResult {
	return &operatorv1.InferenceResult{
		Parts:            r.Parts,
		PromptTokens:     r.PromptTokens,
		CompletionTokens: r.CompletionTokens,
		TotalTokens:      r.TotalTokens,
		FinishReason:     r.FinishReason,
		Model:            r.Model,
	}
}

// ComputeInferenceResultDigest returns the canonical digest of an
// InferenceResult: lowercase hex SHA-256 over the deterministic protobuf
// serialization with result_digest cleared. The Inference Node computes it
// at execution time and returns it as the receipt summary so the signed
// ActionReceipt binds the complete result; the User Gateway recomputes it
// and requires equality before reporting dispatch success.
func ComputeInferenceResultDigest(result *operatorv1.InferenceResult) (string, error) {
	if result == nil {
		return "", fmt.Errorf("models: compute inference result digest: %w", constants.ErrMissingRequiredField)
	}
	clone, ok := proto.Clone(result).(*operatorv1.InferenceResult)
	if !ok {
		return "", fmt.Errorf("models: compute inference result digest: %w", constants.ErrInferenceResultDigest)
	}
	clone.ResultDigest = ""
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("models: compute inference result digest: marshal: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
