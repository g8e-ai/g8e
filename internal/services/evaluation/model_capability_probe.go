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
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const capabilityProbeAttemptPrefix = "capability-probe"

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

type governedCapabilityProbeRunner struct {
	dispatcher FormationInferenceDispatcher
	sessionID  string
	newID      func(string) string
}

// NewGovernedCapabilityProbeRunner executes bounded inventory capability probes
// through the exact Inference Operator session.
func NewGovernedCapabilityProbeRunner(dispatcher FormationInferenceDispatcher, inferenceSessionID string, newID func(string) string) GovernedCapabilityProbeRunner {
	return &governedCapabilityProbeRunner{
		dispatcher: dispatcher,
		sessionID:  inferenceSessionID,
		newID:      newID,
	}
}

func (r *governedCapabilityProbeRunner) RunCapabilityProbes(ctx context.Context, variant *evalv1.ModelVariant) ([]*evalv1.ModelCapabilityObservation, error) {
	if r == nil || r.dispatcher == nil || r.sessionID == "" || r.newID == nil || variant == nil || variant.GetServedModelTag() == "" || variant.GetModelDigest() == "" {
		return nil, fmt.Errorf("evaluation: run model capability probes: %w", constants.ErrMissingRequiredField)
	}
	observations := make([]*evalv1.ModelCapabilityObservation, 0, len(RequiredModelCapabilityKinds()))
	for _, kind := range RequiredModelCapabilityKinds() {
		observation, err := r.probeModelCapability(ctx, variant, kind)
		if err != nil {
			return nil, fmt.Errorf("evaluation: run model capability probes: variant %s capability %s: %w", variant.GetVariantId(), kind.String(), err)
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

func (r *governedCapabilityProbeRunner) probeModelCapability(ctx context.Context, variant *evalv1.ModelVariant, kind evalv1.ModelCapabilityKind) (*evalv1.ModelCapabilityObservation, error) {
	switch kind {
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION:
		return r.probeCompletionCapability(ctx, variant)
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING:
		return r.probeToolCallingCapability(ctx, variant)
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT:
		return r.probeStructuredOutputCapability(ctx, variant)
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_THINKING:
		return r.probeThinkingCapability(ctx, variant)
	case evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_CONTEXT_LIMIT:
		return probeContextLimitCapability(variant), nil
	default:
		return nil, fmt.Errorf("evaluation: probe model capability: unsupported kind %s", kind.String())
	}
}

func (r *governedCapabilityProbeRunner) dispatchCapabilityProbe(ctx context.Context, req InferenceProbeRequest) (*operatorv1.InferenceDispatchResponse, error) {
	dispatchReq, err := BuildInferenceProbeDispatchRequest(req)
	if err != nil {
		return nil, err
	}
	resp, err := r.dispatcher.DispatchInference(ctx, dispatchReq)
	if err != nil {
		return nil, err
	}
	if err := ValidateInferenceProbeResponse(req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (r *governedCapabilityProbeRunner) baseProbeRequest(variant *evalv1.ModelVariant, attemptSuffix string) InferenceProbeRequest {
	return InferenceProbeRequest{
		ProviderAttemptID:       r.newID(capabilityProbeAttemptPrefix + "-" + attemptSuffix),
		Role:                    models.InferenceModelRolePrimary,
		Model:                   variant.GetServedModelTag(),
		ModelDigest:             variant.GetModelDigest(),
		TargetOperatorSessionID: r.sessionID,
	}
}

func (r *governedCapabilityProbeRunner) probeCompletionCapability(ctx context.Context, variant *evalv1.ModelVariant) (*evalv1.ModelCapabilityObservation, error) {
	req := r.baseProbeRequest(variant, "completion")
	req.Prompt = "Reply with exactly: capability-probe-ok"
	resp, err := r.dispatchCapabilityProbe(ctx, req)
	if err != nil {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "provider completion probe failed"), nil
	}
	if inferenceResultHasText(resp.GetResult(), "capability-probe-ok") {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "completion probe returned expected text"), nil
	}
	return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "completion probe returned unexpected output"), nil
}

func (r *governedCapabilityProbeRunner) probeToolCallingCapability(ctx context.Context, variant *evalv1.ModelVariant) (*evalv1.ModelCapabilityObservation, error) {
	req := r.baseProbeRequest(variant, "tools")
	req.Prompt = "Call probe_echo with message capability-probe-tool"
	req.Tools = []*operatorv1.InferenceToolDeclaration{ProbeEchoToolDeclaration()}
	req.ToolChoice = &operatorv1.InferenceToolChoice{Mode: operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_REQUIRED}
	resp, err := r.dispatchCapabilityProbe(ctx, req)
	if err != nil {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED, "provider rejected tool-call probe"), nil
	}
	if inferenceResultHasToolCall(resp.GetResult(), "probe_echo") {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "tool-call probe returned probe_echo"), nil
	}
	return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, "tool-call probe did not return probe_echo"), nil
}

func (r *governedCapabilityProbeRunner) probeStructuredOutputCapability(ctx context.Context, variant *evalv1.ModelVariant) (*evalv1.ModelCapabilityObservation, error) {
	req := r.baseProbeRequest(variant, "structured")
	req.Prompt = `Return JSON with answer set to "capability-probe-json".`
	req.ResponseFormat = ProbeStructuredResponseFormat()
	resp, err := r.dispatchCapabilityProbe(ctx, req)
	if err != nil {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED, "provider rejected structured-output probe"), nil
	}
	payload, ok := inferenceResultStructuredJSON(resp.GetResult())
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

func (r *governedCapabilityProbeRunner) probeThinkingCapability(ctx context.Context, variant *evalv1.ModelVariant) (*evalv1.ModelCapabilityObservation, error) {
	req := r.baseProbeRequest(variant, "thinking")
	req.Prompt = "Think briefly, then reply with exactly: capability-probe-thinking"
	req.Thinking = &operatorv1.InferenceThinkingControl{Mode: &operatorv1.InferenceThinkingControl_Enabled{Enabled: true}}
	resp, err := r.dispatchCapabilityProbe(ctx, req)
	if err != nil {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_THINKING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED, "provider rejected thinking probe"), nil
	}
	if inferenceResultHasThinking(resp.GetResult()) || inferenceResultHasText(resp.GetResult(), "capability-probe-thinking") {
		return capabilityObservation(evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_THINKING, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, "thinking probe returned visible thinking or expected answer text"), nil
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

func capabilityObservation(kind evalv1.ModelCapabilityKind, outcome evalv1.EvaluationVerdictStatus, detail string) *evalv1.ModelCapabilityObservation {
	return &evalv1.ModelCapabilityObservation{
		Capability:        kind,
		Outcome:           outcome,
		ObservationDetail: detail,
	}
}

func inferenceResultHasText(result *operatorv1.InferenceResult, expected string) bool {
	if result == nil {
		return false
	}
	text := strings.ToLower(collectResponseText(result))
	return strings.Contains(text, strings.ToLower(expected))
}

func inferenceResultHasToolCall(result *operatorv1.InferenceResult, name string) bool {
	if result == nil {
		return false
	}
	for _, part := range result.GetParts() {
		if part.GetToolCall() != nil && part.GetToolCall().GetName() == name {
			return true
		}
	}
	return false
}

func inferenceResultHasThinking(result *operatorv1.InferenceResult) bool {
	if result == nil {
		return false
	}
	for _, part := range result.GetParts() {
		if part.GetThinking() != "" {
			return true
		}
	}
	return false
}

func inferenceResultStructuredJSON(result *operatorv1.InferenceResult) (string, bool) {
	if result == nil {
		return "", false
	}
	text := extractStructuredJSONPayload(collectResponseText(result))
	if text == "" {
		return "", false
	}
	if json.Valid([]byte(text)) {
		return text, true
	}
	return "", false
}
