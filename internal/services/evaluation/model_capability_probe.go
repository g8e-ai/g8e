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
	"log/slog"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const capabilityProbeAttemptPrefix = "capability-probe"

// CapabilityProbeBackend executes bounded non-scored provider probes.
type CapabilityProbeBackend interface {
	Generate(ctx context.Context, req models.GenerateRequest) (*models.GenerateResponse, error)
}

// RequiredModelCapabilityKinds returns the bounded probe set for Phase 3 inventory freeze.
func RequiredModelCapabilityKinds() []evalv1.ModelCapabilityKind {
	return []evalv1.ModelCapabilityKind{
		evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION,
		evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING,
		evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT,
		evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_THINKING,
		evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_CONTEXT_LIMIT,
	}
}

// RunModelCapabilityProbes executes descriptive non-scored probes for one variant.
func RunModelCapabilityProbes(ctx context.Context, backend CapabilityProbeBackend, variant *evalv1.ModelVariant) ([]*evalv1.ModelCapabilityObservation, error) {
	if backend == nil || variant == nil || variant.GetServedModelTag() == "" || variant.GetModelDigest() == "" {
		return nil, fmt.Errorf("evaluation: run model capability probes: %w", constants.ErrMissingRequiredField)
	}
	observations := make([]*evalv1.ModelCapabilityObservation, 0, len(RequiredModelCapabilityKinds()))
	for _, kind := range RequiredModelCapabilityKinds() {
		observation, err := probeModelCapability(ctx, backend, variant, kind)
		if err != nil {
			return nil, fmt.Errorf("evaluation: run model capability probes: variant %s capability %s: %w", variant.GetVariantId(), kind.String(), err)
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

func probeModelCapability(ctx context.Context, backend CapabilityProbeBackend, variant *evalv1.ModelVariant, kind evalv1.ModelCapabilityKind) (*evalv1.ModelCapabilityObservation, error) {
	switch kind {
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION:
		return probeCompletionCapability(ctx, backend, variant)
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING:
		return probeToolCallingCapability(ctx, backend, variant)
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT:
		return probeStructuredOutputCapability(ctx, backend, variant)
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_THINKING:
		return probeThinkingCapability(ctx, backend, variant)
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_CONTEXT_LIMIT:
		return probeContextLimitCapability(variant), nil
	default:
		return nil, fmt.Errorf("evaluation: probe model capability: unsupported kind %s", kind.String())
	}
}

func probeCompletionCapability(ctx context.Context, backend CapabilityProbeBackend, variant *evalv1.ModelVariant) (*evalv1.ModelCapabilityObservation, error) {
	response, err := backend.Generate(ctx, capabilityProbeRequest(variant, capabilityProbeAttemptPrefix+"-completion", nil, nil, nil, "Reply with exactly: capability-probe-ok"))
	if err != nil {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "provider completion probe failed"), nil
	}
	if responseHasText(response, "capability-probe-ok") {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "completion probe returned expected text"), nil
	}
	return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "completion probe returned unexpected output"), nil
}

func probeToolCallingCapability(ctx context.Context, backend CapabilityProbeBackend, variant *evalv1.ModelVariant) (*evalv1.ModelCapabilityObservation, error) {
	toolChoiceRequired := operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_REQUIRED
	response, err := backend.Generate(ctx, capabilityProbeRequest(
		variant,
		capabilityProbeAttemptPrefix+"-tools",
		[]*operatorv1.InferenceToolDeclaration{ProbeEchoToolDeclaration()},
		&operatorv1.InferenceToolChoice{Mode: toolChoiceRequired},
		nil,
		"Call probe_echo with message capability-probe-tool",
	))
	if err != nil {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED, "provider rejected tool-call probe"), nil
	}
	if responseHasToolCall(response, "probe_echo") {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "tool-call probe returned probe_echo"), nil
	}
	return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "tool-call probe did not return probe_echo"), nil
}

func probeStructuredOutputCapability(ctx context.Context, backend CapabilityProbeBackend, variant *evalv1.ModelVariant) (*evalv1.ModelCapabilityObservation, error) {
	response, err := backend.Generate(ctx, capabilityProbeRequest(
		variant,
		capabilityProbeAttemptPrefix+"-structured",
		nil,
		nil,
		ProbeStructuredResponseFormat(),
		`Return JSON with answer set to "capability-probe-json".`,
	))
	if err != nil {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED, "provider rejected structured-output probe"), nil
	}
	payload, ok := responseStructuredJSON(response)
	if !ok {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "structured-output probe returned non-JSON text"), nil
	}
	var decoded struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil || decoded.Answer != "capability-probe-json" {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "structured-output probe returned invalid schema payload"), nil
	}
	return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "structured-output probe returned valid JSON"), nil
}

func probeThinkingCapability(ctx context.Context, backend CapabilityProbeBackend, variant *evalv1.ModelVariant) (*evalv1.ModelCapabilityObservation, error) {
	response, err := backend.Generate(ctx, capabilityProbeRequest(
		variant,
		capabilityProbeAttemptPrefix+"-thinking",
		nil,
		nil,
		nil,
		"Think briefly, then reply with exactly: capability-probe-thinking",
		withThinking(&operatorv1.InferenceThinkingControl{Mode: &operatorv1.InferenceThinkingControl_Enabled{Enabled: true}}),
	))
	if err != nil {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_THINKING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED, "provider rejected thinking probe"), nil
	}
	if responseHasThinking(response) || responseHasText(response, "capability-probe-thinking") {
		detail := "thinking probe returned visible thinking or expected answer text"
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_THINKING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, detail), nil
	}
	return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_THINKING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "thinking probe returned neither thinking nor expected answer"), nil
}

func probeContextLimitCapability(variant *evalv1.ModelVariant) *evalv1.ModelCapabilityObservation {
	if variant.GetContextLimit() > 0 {
		return capabilityObservation(
			evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_CONTEXT_LIMIT,
			evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
			fmt.Sprintf("provider advertised context_limit=%d", variant.GetContextLimit()),
		)
	}
	return capabilityObservation(
		evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_CONTEXT_LIMIT,
		evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE,
		"provider did not advertise context_limit",
	)
}

type capabilityProbeOption func(*models.GenerateRequest)

func withThinking(control *operatorv1.InferenceThinkingControl) capabilityProbeOption {
	return func(req *models.GenerateRequest) {
		req.Thinking = control
	}
}

func capabilityProbeRequest(variant *evalv1.ModelVariant, attemptID string, tools []*operatorv1.InferenceToolDeclaration, toolChoice *operatorv1.InferenceToolChoice, responseFormat *operatorv1.InferenceResponseFormat, prompt string, opts ...capabilityProbeOption) models.GenerateRequest {
	req := models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: variant.GetServedModelTag(),
		Messages: []*operatorv1.InferenceMessage{{
			Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
			Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: prompt}}},
		}},
		Tools:                tools,
		ToolChoice:           toolChoice,
		ResponseFormat:       responseFormat,
		MaxTokens:            128,
		ProviderAttemptID:    attemptID,
		ModelDigest:          variant.GetModelDigest(),
		RequestSchemaVersion: constants.InferenceRequestSchemaVersion,
	}
	for _, opt := range opts {
		opt(&req)
	}
	return req
}

func capabilityObservation(kind evalv1.ModelCapabilityKind, outcome evalv1.EvaluationVerdictStatus, detail string) *evalv1.ModelCapabilityObservation {
	return &evalv1.ModelCapabilityObservation{
		Capability:        kind,
		Outcome:           outcome,
		ObservationDetail: detail,
	}
}

func responseHasText(response *models.GenerateResponse, expected string) bool {
	if response == nil {
		return false
	}
	for _, part := range response.Parts {
		if strings.Contains(part.GetText(), expected) {
			return true
		}
	}
	return false
}

func responseHasToolCall(response *models.GenerateResponse, name string) bool {
	if response == nil {
		return false
	}
	for _, part := range response.Parts {
		if part.GetToolCall() != nil && part.GetToolCall().GetName() == name {
			return true
		}
	}
	return false
}

func responseHasThinking(response *models.GenerateResponse) bool {
	if response == nil {
		return false
	}
	for _, part := range response.Parts {
		if part.GetThinking() != "" {
			return true
		}
	}
	return false
}

func responseStructuredJSON(response *models.GenerateResponse) (string, bool) {
	if response == nil {
		return "", false
	}
	for _, part := range response.Parts {
		if text := strings.TrimSpace(part.GetText()); text != "" {
			if json.Valid([]byte(text)) {
				return text, true
			}
		}
	}
	return "", false
}

// NewOllamaCapabilityProbeBackend wraps an Ollama endpoint for inventory probes.
func NewOllamaCapabilityProbeBackend(endpoint string, logger *slog.Logger) (CapabilityProbeBackend, error) {
	return inference.NewOllamaBackend(endpoint, logger)
}
