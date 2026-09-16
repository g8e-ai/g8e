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
	"sort"
	"strings"

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
	Role                 InferenceModelRole
	Model                string
	Messages             []*operatorv1.InferenceMessage
	Tools                []*operatorv1.InferenceToolDeclaration
	Temperature          float32
	MaxTokens            int32
	KeepAlive            string
	TopP                 *float32
	TopK                 *int32
	Seed                 *int32
	StopSequences        []string
	ResponseFormat       *operatorv1.InferenceResponseFormat
	RequestSchemaVersion string
	ToolChoice           *operatorv1.InferenceToolChoice
	ParallelToolCalls    *bool
	Thinking             *operatorv1.InferenceThinkingControl
	ContextLimit         *int32
	ProviderAttemptID    string
	ModelDigest          string
	CampaignID           string
	RunID                string
	AssignmentID         string
	EvaluationAttemptID  string
	ScenarioID           string
	ModelRegistry        []*operatorv1.InferenceModelVariant
	ModelRegistryDigest  string
	Stream               bool
	RetryCount           uint32
}

// GenerateResponse carries ordered model response parts, usage metadata, and finish reason returned by the backend.
type GenerateResponse struct {
	Parts                 []*operatorv1.InferenceResponsePart
	PromptTokens          int32
	CompletionTokens      int32
	TotalTokens           int32
	UsageReported         bool
	ThinkingTokens        *int32
	CacheTokens           *int32
	LoadDurationNS        *int64
	PromptEvalDurationNS  *int64
	GenerationDurationNS  *int64
	TotalDurationNS       *int64
	TimeToFirstTokenNS    *int64
	TimingSource          operatorv1.InferenceTimingSource
	FinishReason          string
	Model                 string
	ProviderAttemptID     string
	RequestedModel        string
	RequestedModelDigest  string
	ServedModelDigest     string
	NormalizedRequestHash string
	OutputHash            string
	CampaignID            string
	RunID                 string
	AssignmentID          string
	EvaluationAttemptID   string
	ScenarioID            string
	ModelRegistryDigest   string
	RetryCount            uint32
	RetryClassification   operatorv1.InferenceRetryClassification
	LoadState             operatorv1.InferenceLoadState
}

// BackendStatus reports the backend's readiness and the models available in
// its store.
type BackendStatus struct {
	Available bool
	Models    []string
}

// InferenceRequestPayload is the typed governed inference payload decoded from the protobuf InferenceRequested message.
type InferenceRequestPayload struct {
	Role                 InferenceModelRole
	Model                string
	Messages             []*operatorv1.InferenceMessage
	Tools                []*operatorv1.InferenceToolDeclaration
	Temperature          float32
	MaxTokens            int32
	KeepAlive            string
	TopP                 *float32
	TopK                 *int32
	Seed                 *int32
	StopSequences        []string
	ResponseFormat       *operatorv1.InferenceResponseFormat
	RequestSchemaVersion string
	ToolChoice           *operatorv1.InferenceToolChoice
	ParallelToolCalls    *bool
	Thinking             *operatorv1.InferenceThinkingControl
	ContextLimit         *int32
	ProviderAttemptID    string
	ModelDigest          string
	CampaignID           string
	RunID                string
	AssignmentID         string
	EvaluationAttemptID  string
	ScenarioID           string
	ModelRegistry        []*operatorv1.InferenceModelVariant
	ModelRegistryDigest  string
	Stream               bool
	RetryCount           uint32
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
	var topP *float32
	if req.TopP != nil {
		value := req.GetTopP()
		topP = &value
	}
	var topK *int32
	if req.TopK != nil {
		value := req.GetTopK()
		topK = &value
	}
	var responseFormat *operatorv1.InferenceResponseFormat
	if req.GetResponseFormat() != nil {
		responseFormat = &operatorv1.InferenceResponseFormat{}
		proto.Merge(responseFormat, req.GetResponseFormat())
	}
	var toolChoice *operatorv1.InferenceToolChoice
	if req.GetToolChoice() != nil {
		toolChoice = &operatorv1.InferenceToolChoice{}
		proto.Merge(toolChoice, req.GetToolChoice())
	}
	var parallelToolCalls *bool
	if req.ParallelToolCalls != nil {
		value := req.GetParallelToolCalls()
		parallelToolCalls = &value
	}
	var thinking *operatorv1.InferenceThinkingControl
	if req.GetThinking() != nil {
		thinking = &operatorv1.InferenceThinkingControl{}
		proto.Merge(thinking, req.GetThinking())
	}
	var contextLimit *int32
	if req.ContextLimit != nil {
		value := req.GetContextLimit()
		contextLimit = &value
	}
	var seed *int32
	if req.Seed != nil {
		value := req.GetSeed()
		seed = &value
	}
	return InferenceRequestPayload{
		Role:                 InferenceModelRoleFromProto(req.GetRole()),
		Model:                req.GetModel(),
		Messages:             messages,
		Tools:                tools,
		Temperature:          req.GetTemperature(),
		MaxTokens:            req.GetMaxTokens(),
		KeepAlive:            req.GetKeepAlive(),
		TopP:                 topP,
		TopK:                 topK,
		Seed:                 seed,
		StopSequences:        append([]string(nil), req.GetStopSequences()...),
		ResponseFormat:       responseFormat,
		RequestSchemaVersion: req.GetRequestSchemaVersion(),
		ToolChoice:           toolChoice,
		ParallelToolCalls:    parallelToolCalls,
		Thinking:             thinking,
		ContextLimit:         contextLimit,
		ProviderAttemptID:    req.GetProviderAttemptId(),
		ModelDigest:          req.GetModelDigest(),
		CampaignID:           req.GetCampaignId(),
		RunID:                req.GetRunId(),
		AssignmentID:         req.GetAssignmentId(),
		EvaluationAttemptID:  req.GetEvaluationAttemptId(),
		ScenarioID:           req.GetScenarioId(),
		ModelRegistry:        cloneInferenceModelRegistry(req.GetModelRegistry()),
		ModelRegistryDigest:  req.GetModelRegistryDigest(),
		Stream:               req.GetStream(),
		RetryCount:           req.GetRetryCount(),
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
		Role:                 p.Role,
		Model:                model,
		Messages:             p.Messages,
		Tools:                p.Tools,
		Temperature:          p.Temperature,
		MaxTokens:            p.MaxTokens,
		KeepAlive:            p.KeepAlive,
		TopP:                 p.TopP,
		TopK:                 p.TopK,
		Seed:                 p.Seed,
		StopSequences:        append([]string(nil), p.StopSequences...),
		ResponseFormat:       p.ResponseFormat,
		RequestSchemaVersion: p.RequestSchemaVersion,
		ToolChoice:           p.ToolChoice,
		ParallelToolCalls:    p.ParallelToolCalls,
		Thinking:             p.Thinking,
		ContextLimit:         p.ContextLimit,
		ProviderAttemptID:    p.ProviderAttemptID,
		ModelDigest:          p.ModelDigest,
		CampaignID:           p.CampaignID,
		RunID:                p.RunID,
		AssignmentID:         p.AssignmentID,
		EvaluationAttemptID:  p.EvaluationAttemptID,
		ScenarioID:           p.ScenarioID,
		ModelRegistry:        cloneInferenceModelRegistry(p.ModelRegistry),
		ModelRegistryDigest:  p.ModelRegistryDigest,
		Stream:               p.Stream,
		RetryCount:           p.RetryCount,
	}
}

// ToProtoInferenceResult converts a GenerateResponse to the protobuf
// InferenceResult message for result envelope publishing.
func (r GenerateResponse) ToProtoInferenceResult() *operatorv1.InferenceResult {
	return &operatorv1.InferenceResult{
		Parts:                 r.Parts,
		PromptTokens:          r.PromptTokens,
		CompletionTokens:      r.CompletionTokens,
		TotalTokens:           r.TotalTokens,
		UsageReported:         r.UsageReported,
		ThinkingTokens:        r.ThinkingTokens,
		CacheTokens:           r.CacheTokens,
		LoadDurationNs:        r.LoadDurationNS,
		PromptEvalDurationNs:  r.PromptEvalDurationNS,
		GenerationDurationNs:  r.GenerationDurationNS,
		TotalDurationNs:       r.TotalDurationNS,
		TimeToFirstTokenNs:    r.TimeToFirstTokenNS,
		TimingSource:          r.TimingSource,
		FinishReason:          r.FinishReason,
		Model:                 r.Model,
		ProviderAttemptId:     r.ProviderAttemptID,
		RequestedModel:        r.RequestedModel,
		RequestedModelDigest:  r.RequestedModelDigest,
		ServedModelDigest:     r.ServedModelDigest,
		NormalizedRequestHash: r.NormalizedRequestHash,
		OutputHash:            r.OutputHash,
		CampaignId:            r.CampaignID,
		RunId:                 r.RunID,
		AssignmentId:          r.AssignmentID,
		EvaluationAttemptId:   r.EvaluationAttemptID,
		ScenarioId:            r.ScenarioID,
		ModelRegistryDigest:   r.ModelRegistryDigest,
		RetryCount:            r.RetryCount,
		RetryClassification:   r.RetryClassification,
		LoadState:             r.LoadState,
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
	return SHA256Hex(data), nil
}

func ComputeInferenceModelRegistryDigest(campaignID string, variants []*operatorv1.InferenceModelVariant) (string, error) {
	if campaignID == "" || len(variants) == 0 {
		return "", fmt.Errorf("models: compute inference model registry digest: %w", constants.ErrMissingRequiredField)
	}
	registry := cloneInferenceModelRegistry(variants)
	sort.Slice(registry, func(i, j int) bool {
		if registry[i].GetModel() == registry[j].GetModel() {
			return registry[i].GetDigest() < registry[j].GetDigest()
		}
		return registry[i].GetModel() < registry[j].GetModel()
	})
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(&operatorv1.InferenceRequested{
		CampaignId:    campaignID,
		ModelRegistry: registry,
	})
	if err != nil {
		return "", fmt.Errorf("models: compute inference model registry digest: marshal: %w", err)
	}
	return SHA256Hex(data), nil
}

func cloneInferenceModelRegistry(variants []*operatorv1.InferenceModelVariant) []*operatorv1.InferenceModelVariant {
	clones := make([]*operatorv1.InferenceModelVariant, len(variants))
	for i, variant := range variants {
		if variant == nil {
			continue
		}
		clones[i] = proto.Clone(variant).(*operatorv1.InferenceModelVariant)
	}
	return clones
}

func ComputeInferenceOutputHash(parts []*operatorv1.InferenceResponsePart, finishReason string) (string, error) {
	output := &operatorv1.InferenceResult{Parts: parts, FinishReason: finishReason}
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("models: compute inference output hash: %w", err)
	}
	return SHA256Hex(data), nil
}

// ReconcileInferenceProgress verifies that live progress events bind to the
// verified terminal result: sequences are contiguous from 1, provider-attempt
// identity matches, and the concatenated delta parts hash to output_hash.
func ReconcileInferenceProgress(events []*operatorv1.InferenceProgressEvent, result *operatorv1.InferenceResult) error {
	if result == nil {
		return fmt.Errorf("%w: missing result", constants.ErrInferenceProgressHashMismatch)
	}
	parts := make([]*operatorv1.InferenceResponsePart, 0)
	for i, event := range events {
		if event == nil {
			return fmt.Errorf("%w: missing progress event %d", constants.ErrInferenceProgressHashMismatch, i+1)
		}
		wantSeq := uint32(i + 1)
		if event.GetSequence() != wantSeq {
			return fmt.Errorf("%w: progress sequence %d, want %d", constants.ErrInferenceProgressHashMismatch, event.GetSequence(), wantSeq)
		}
		if event.GetProviderAttemptId() != result.GetProviderAttemptId() {
			return fmt.Errorf("%w: progress provider attempt mismatch", constants.ErrInferenceProgressHashMismatch)
		}
		parts = append(parts, event.GetParts()...)
	}
	if len(parts) == 0 && len(result.GetParts()) > 0 {
		return fmt.Errorf("%w: no progress events for terminal parts", constants.ErrInferenceProgressHashMismatch)
	}
	gotHash, err := ComputeInferenceOutputHash(parts, result.GetFinishReason())
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrInferenceProgressHashMismatch, err)
	}
	if gotHash != result.GetOutputHash() {
		return fmt.Errorf("%w: concatenated progress hash mismatch", constants.ErrInferenceProgressHashMismatch)
	}
	return nil
}

func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ClassifyRetry maps the zero-based retry ordinal to a typed classification.
func ClassifyRetry(retryCount uint32) operatorv1.InferenceRetryClassification {
	if retryCount == 0 {
		return operatorv1.InferenceRetryClassification_INFERENCE_RETRY_CLASSIFICATION_NONE
	}
	return operatorv1.InferenceRetryClassification_INFERENCE_RETRY_CLASSIFICATION_INFRASTRUCTURE_PRE_RESULT
}

// ClassifyLoadState derives cold/warm classification from provider-reported
// load duration. Missing load duration is explicitly unavailable.
func ClassifyLoadState(loadDurationNS *int64) operatorv1.InferenceLoadState {
	if loadDurationNS == nil {
		return operatorv1.InferenceLoadState_INFERENCE_LOAD_STATE_UNAVAILABLE
	}
	if *loadDurationNS == 0 {
		return operatorv1.InferenceLoadState_INFERENCE_LOAD_STATE_WARM
	}
	if *loadDurationNS < 0 {
		return operatorv1.InferenceLoadState_INFERENCE_LOAD_STATE_UNAVAILABLE
	}
	return operatorv1.InferenceLoadState_INFERENCE_LOAD_STATE_COLD
}

func IsSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
