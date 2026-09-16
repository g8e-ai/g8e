// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/jsonschema"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/proto"
)

const (
	inferenceJSONMediaType        = "application/json"
	maxInferenceTopK              = 1000
	maxInferenceStopSequences     = 16
	maxInferenceStopSequenceBytes = 256
	maxInferenceContextLimit      = 1 << 20
)

// InferenceExecutionHandler implements governance.ExecutionHandler. It decodes and validates the governed typed conversation, recursively scrubs data-bearing values, calls Backend.Generate, and returns the digest the L5 actuator records in the receipt.
type InferenceExecutionHandler struct {
	backend   Backend
	cfg       *config.Config
	scrubbing *scrubbing.ScrubbingService
	logger    *slog.Logger
}

// NewInferenceExecutionHandler constructs an InferenceExecutionHandler with
// the given backend, config, scrubbing service, and logger.
func NewInferenceExecutionHandler(backend Backend, cfg *config.Config, scrubbingSvc *scrubbing.ScrubbingService, logger *slog.Logger) *InferenceExecutionHandler {
	return &InferenceExecutionHandler{
		backend:   backend,
		cfg:       cfg,
		scrubbing: scrubbingSvc,
		logger:    logger,
	}
}

// ExecuteVerifiedTransaction implements governance.ExecutionHandler. It is
// called by the L5 actuator after L1–L4 verification passes. It delegates to
// ExecuteInference and returns the canonical result digest as the receipt
// summary so the signed ActionReceipt binds the complete InferenceResult
// (see operator.proto InferenceResult.result_digest).
func (h *InferenceExecutionHandler) ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg governance.CommandMessage) (string, error) {
	resp, err := h.ExecuteInference(ctx, cmdMsg)
	if err != nil {
		return "", err
	}
	digest, err := models.ComputeInferenceResultDigest(resp.ToProtoInferenceResult())
	if err != nil {
		return "", err
	}
	return digest, nil
}

// ExecuteInference decodes the protobuf InferenceRequested payload, validates and scrubs its typed conversation, resolves the approved role model, and calls Backend.Generate.
func (h *InferenceExecutionHandler) ExecuteInference(ctx context.Context, cmdMsg governance.CommandMessage) (*models.GenerateResponse, error) {
	if h.backend == nil {
		return nil, fmt.Errorf("inference handler: %w", constants.ErrInferenceBackendNotRegistered)
	}

	payloadBytes := cmdMsg.GetPayload()
	if len(payloadBytes) == 0 {
		return nil, fmt.Errorf("inference handler: empty payload: %w", constants.ErrPubSubEmptyPayload)
	}

	req := &operatorv1.InferenceRequested{}
	if err := proto.Unmarshal(payloadBytes, req); err != nil {
		return nil, fmt.Errorf("inference handler: unmarshal payload: %w", err)
	}

	infReq := models.FromProtoInferenceRequested(req)
	if err := h.normalizeInferenceInput(&infReq); err != nil {
		return nil, fmt.Errorf("inference handler: normalize request: %w", err)
	}

	// Resolve the approved model for the role. The configured role-to-model
	// mapping is the active typed authority: a request model is accepted
	// only when it names the configured model for the requested role; any
	// other value is an unauthorized override.
	if infReq.Role == models.InferenceModelRoleUnspecified {
		return nil, fmt.Errorf("inference handler: %w", constants.ErrInferenceRoleInvalid)
	}
	approved := h.defaultModelForRole(infReq.Role)
	if approved == "" {
		return nil, fmt.Errorf("inference handler: %w: role %d", constants.ErrInferenceModelRefInvalid, infReq.Role)
	}
	if infReq.Model != "" && infReq.Model != approved {
		return nil, fmt.Errorf("inference handler: %w: role %d", constants.ErrInferenceModelOverrideDenied, infReq.Role)
	}
	genReq := infReq.ToGenerateRequest(approved)
	// Apply config default keep-alive when the request does not override it.
	if genReq.KeepAlive == "" {
		genReq.KeepAlive = h.cfg.Inference.KeepAlive
	}

	h.logger.Info("Dispatching governed inference request",
		"role", infReq.Role,
		"model", genReq.Model,
		"message_count", len(genReq.Messages),
		"tool_count", len(genReq.Tools))

	resp, err := h.backend.Generate(ctx, genReq)
	if err != nil {
		return nil, fmt.Errorf("inference handler: %w", err)
	}
	if resp == nil || resp.Model != genReq.Model {
		return nil, fmt.Errorf("inference handler: %w", constants.ErrInferenceIdentityMismatch)
	}
	if !models.IsSHA256Hex(resp.NormalizedRequestHash) || !models.IsSHA256Hex(resp.OutputHash) {
		return nil, fmt.Errorf("inference handler: %w", constants.ErrInferenceEvidenceHashInvalid)
	}
	if genReq.ModelDigest != "" && resp.ServedModelDigest != genReq.ModelDigest {
		return nil, fmt.Errorf("inference handler: %w", constants.ErrInferenceModelDigestMismatch)
	}
	resp.ProviderAttemptID = genReq.ProviderAttemptID
	resp.RequestedModel = genReq.Model
	resp.RequestedModelDigest = genReq.ModelDigest
	resp.CampaignID = genReq.CampaignID
	resp.RunID = genReq.RunID
	resp.AssignmentID = genReq.AssignmentID
	resp.EvaluationAttemptID = genReq.EvaluationAttemptID
	resp.ScenarioID = genReq.ScenarioID

	return resp, nil
}

func (h *InferenceExecutionHandler) normalizeInferenceInput(req *models.InferenceRequestPayload) error {
	if req.ProviderAttemptID == "" {
		return constants.ErrInferenceProviderAttemptRequired
	}
	if req.ModelDigest != "" && !models.IsSHA256Hex(req.ModelDigest) {
		return constants.ErrInferenceEvidenceHashInvalid
	}
	if req.RequestSchemaVersion != constants.InferenceRequestSchemaVersion {
		return fmt.Errorf("%w: request schema version", constants.ErrInferenceGenerationOptionsInvalid)
	}
	if math.IsNaN(float64(req.Temperature)) || math.IsInf(float64(req.Temperature), 0) || req.Temperature < 0 || req.Temperature > 2 {
		return fmt.Errorf("%w: temperature", constants.ErrInferenceGenerationOptionsInvalid)
	}
	if req.MaxTokens < 0 {
		return fmt.Errorf("%w: max tokens", constants.ErrInferenceGenerationOptionsInvalid)
	}
	if req.TopP != nil && (math.IsNaN(float64(*req.TopP)) || math.IsInf(float64(*req.TopP), 0) || *req.TopP < 0 || *req.TopP > 1) {
		return fmt.Errorf("%w: top_p", constants.ErrInferenceGenerationOptionsInvalid)
	}
	if req.TopK != nil && (*req.TopK <= 0 || *req.TopK > maxInferenceTopK) {
		return fmt.Errorf("%w: top_k", constants.ErrInferenceGenerationOptionsInvalid)
	}
	if len(req.StopSequences) > maxInferenceStopSequences {
		return fmt.Errorf("%w: stop sequences", constants.ErrInferenceGenerationOptionsInvalid)
	}
	for i, stop := range req.StopSequences {
		if stop == "" || len(stop) > maxInferenceStopSequenceBytes {
			return fmt.Errorf("%w: stop sequence %d", constants.ErrInferenceGenerationOptionsInvalid, i)
		}
	}
	if req.ResponseFormat != nil {
		if req.ResponseFormat.GetMediaType() != inferenceJSONMediaType {
			return fmt.Errorf("%w: response media type", constants.ErrInferenceCapabilityUnsupported)
		}
		if err := validateInferenceToolSchema(req.ResponseFormat.GetJsonSchema()); err != nil {
			return fmt.Errorf("%w: response schema: %w", constants.ErrInferenceGenerationOptionsInvalid, err)
		}
	}
	if req.ToolChoice != nil {
		if req.ToolChoice.GetMode() != operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO {
			return fmt.Errorf("%w: tool choice mode", constants.ErrInferenceCapabilityUnsupported)
		}
		declared := make(map[string]struct{}, len(req.Tools))
		for _, tool := range req.Tools {
			if tool != nil {
				declared[tool.GetName()] = struct{}{}
			}
		}
		allowed := make(map[string]struct{}, len(req.ToolChoice.GetAllowedToolNames()))
		for _, name := range req.ToolChoice.GetAllowedToolNames() {
			if name == "" {
				return fmt.Errorf("%w: empty allowed tool name", constants.ErrInferenceGenerationOptionsInvalid)
			}
			if _, exists := declared[name]; !exists {
				return fmt.Errorf("%w: undeclared allowed tool", constants.ErrInferenceGenerationOptionsInvalid)
			}
			if _, exists := allowed[name]; exists {
				return fmt.Errorf("%w: duplicate allowed tool", constants.ErrInferenceGenerationOptionsInvalid)
			}
			allowed[name] = struct{}{}
		}
	}
	if req.Thinking != nil {
		switch mode := req.Thinking.GetMode().(type) {
		case *operatorv1.InferenceThinkingControl_Enabled:
			if !mode.Enabled && req.Thinking.GetIncludeThoughts() {
				return fmt.Errorf("%w: disabled thinking includes thoughts", constants.ErrInferenceGenerationOptionsInvalid)
			}
		case *operatorv1.InferenceThinkingControl_Level:
			return fmt.Errorf("%w: thinking level %q", constants.ErrInferenceCapabilityUnsupported, mode.Level)
		default:
			return fmt.Errorf("%w: thinking mode", constants.ErrInferenceGenerationOptionsInvalid)
		}
	}
	if req.ContextLimit != nil && (*req.ContextLimit <= 0 || *req.ContextLimit > maxInferenceContextLimit) {
		return fmt.Errorf("%w: context limit", constants.ErrInferenceGenerationOptionsInvalid)
	}
	if len(req.Messages) == 0 {
		return constants.ErrInferenceMessagesRequired
	}
	scrubText := func(value string) string { return value }
	if h.scrubbing != nil && h.scrubbing.IsEnabled() {
		scrubText = h.scrubbing.ScrubText
	}
	for messageIndex, message := range req.Messages {
		if message == nil || !validInferenceMessageRole(message.GetRole()) || len(message.GetParts()) == 0 {
			return fmt.Errorf("%w: message %d", constants.ErrInferenceMessageInvalid, messageIndex)
		}
		for partIndex, part := range message.GetParts() {
			if part == nil || part.GetPart() == nil {
				return fmt.Errorf("%w: message %d part %d", constants.ErrInferenceMessageInvalid, messageIndex, partIndex)
			}
			switch value := part.GetPart().(type) {
			case *operatorv1.InferenceMessagePart_Text:
				if message.GetRole() == operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL {
					return fmt.Errorf("%w: message %d text part", constants.ErrInferenceMessageInvalid, messageIndex)
				}
				value.Text = scrubText(value.Text)
			case *operatorv1.InferenceMessagePart_ToolCall:
				if message.GetRole() != operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT || value.ToolCall == nil || value.ToolCall.GetName() == "" {
					return fmt.Errorf("%w: message %d tool call", constants.ErrInferenceMessageInvalid, messageIndex)
				}
				normalized, err := normalizeCanonicalJSON(value.ToolCall.GetArgumentsJson(), true, scrubText)
				if err != nil {
					return fmt.Errorf("message %d tool call arguments: %w", messageIndex, err)
				}
				value.ToolCall.ArgumentsJson = normalized
			case *operatorv1.InferenceMessagePart_ToolResult:
				if message.GetRole() != operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL || value.ToolResult == nil || value.ToolResult.GetName() == "" {
					return fmt.Errorf("%w: message %d tool result", constants.ErrInferenceMessageInvalid, messageIndex)
				}
				normalized, err := normalizeCanonicalJSON(value.ToolResult.GetResultJson(), false, scrubText)
				if err != nil {
					return fmt.Errorf("message %d tool result: %w", messageIndex, err)
				}
				value.ToolResult.ResultJson = normalized
			default:
				return fmt.Errorf("%w: message %d part %d", constants.ErrInferenceMessageInvalid, messageIndex, partIndex)
			}
		}
	}
	for toolIndex, tool := range req.Tools {
		if tool == nil || tool.GetName() == "" {
			return fmt.Errorf("%w: tool %d", constants.ErrInferenceToolSchemaInvalid, toolIndex)
		}
		if err := validateInferenceToolSchema(tool.GetJsonSchema()); err != nil {
			return fmt.Errorf("%w: tool %d: %w", constants.ErrInferenceToolSchemaInvalid, toolIndex, err)
		}
	}
	return nil
}

func validateInferenceToolSchema(value string) error {
	if _, err := normalizeCanonicalJSON(value, true, nil); err != nil {
		return err
	}
	if _, err := jsonschema.NewCompiler().Compile([]byte(value)); err != nil {
		return err
	}
	return nil
}

func validInferenceMessageRole(role operatorv1.InferenceMessageRole) bool {
	switch role {
	case operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_SYSTEM,
		operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
		operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT,
		operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL:
		return true
	default:
		return false
	}
}

func normalizeCanonicalJSON(value string, requireObject bool, scrubText func(string) string) (string, error) {
	canonical, err := canonicalizeJSON(value, requireObject, nil)
	if err != nil {
		return "", err
	}
	if canonical != value {
		return "", constants.ErrInferenceJSONNonCanonical
	}
	if scrubText == nil {
		return canonical, nil
	}
	return canonicalizeJSON(value, requireObject, scrubText)
}

func canonicalizeJSON(value string, requireObject bool, scrubText func(string) string) (string, error) {
	if value == "" {
		return "", constants.ErrInferenceJSONInvalid
	}
	acceptAll := true
	if failures := jsonschema.NewValidator().Validate(&jsonschema.Schema{Boolean: &acceptAll}, []byte(value)); len(failures) > 0 {
		return "", fmt.Errorf("%w: %s", constants.ErrInferenceJSONInvalid, failures.Error())
	}
	canonical, err := normalizeJSONValue(json.RawMessage(value), scrubText)
	if err != nil {
		return "", err
	}
	if requireObject && (len(canonical) == 0 || canonical[0] != '{') {
		return "", constants.ErrInferenceJSONInvalid
	}
	return string(canonical), nil
}

func normalizeJSONValue(raw json.RawMessage, scrubText func(string) string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, constants.ErrInferenceJSONInvalid
	}
	switch trimmed[0] {
	case '{':
		values := map[string]json.RawMessage{}
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return nil, fmt.Errorf("%w: %v", constants.ErrInferenceJSONInvalid, err)
		}
		for key, value := range values {
			normalized, err := normalizeJSONValue(value, scrubText)
			if err != nil {
				return nil, err
			}
			values[key] = normalized
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return nil, fmt.Errorf("normalize JSON object: %w", err)
		}
		return encoded, nil
	case '[':
		var values []json.RawMessage
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return nil, fmt.Errorf("%w: %v", constants.ErrInferenceJSONInvalid, err)
		}
		for i, value := range values {
			normalized, err := normalizeJSONValue(value, scrubText)
			if err != nil {
				return nil, err
			}
			values[i] = normalized
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return nil, fmt.Errorf("normalize JSON array: %w", err)
		}
		return encoded, nil
	case '"':
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return nil, fmt.Errorf("%w: %v", constants.ErrInferenceJSONInvalid, err)
		}
		if scrubText != nil {
			value = scrubText(value)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("normalize JSON string: %w", err)
		}
		return encoded, nil
	default:
		if !json.Valid(trimmed) {
			return nil, constants.ErrInferenceJSONInvalid
		}
		return append(json.RawMessage(nil), trimmed...), nil
	}
}

// defaultModelForRole returns the configured default Ollama model name for
// the given chat-tier role.
func (h *InferenceExecutionHandler) defaultModelForRole(role models.InferenceModelRole) string {
	switch role {
	case models.InferenceModelRolePrimary:
		return h.cfg.Inference.PrimaryModel
	case models.InferenceModelRoleAssistant:
		return h.cfg.Inference.AssistantModel
	case models.InferenceModelRoleLite:
		return h.cfg.Inference.LiteModel
	default:
		return ""
	}
}
