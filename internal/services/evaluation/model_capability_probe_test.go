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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type stubGovernedCapabilityProbeDispatcher struct {
	bySuffix map[string]func(InferenceProbeRequest) (*operatorv1.InferenceDispatchResponse, error)
}

func (s *stubGovernedCapabilityProbeDispatcher) DispatchInference(_ context.Context, req *operatorv1.InferenceDispatchRequest) (*operatorv1.InferenceDispatchResponse, error) {
	for suffix, handler := range s.bySuffix {
		if req.GetProviderAttemptId() == capabilityProbeAttemptPrefix+"-"+suffix {
			probeReq := InferenceProbeRequest{
				ProviderAttemptID:       req.GetProviderAttemptId(),
				Model:                   req.GetModel(),
				TargetOperatorSessionID: req.GetTargetOperatorSessionId(),
			}
			return handler(probeReq)
		}
	}
	return nil, fmt.Errorf("unexpected attempt %s", req.GetProviderAttemptId())
}

func TestGovernedCapabilityProbeRunner_RecordsDescriptiveOutcomesWithoutExclusion(t *testing.T) {
	variant := &evalv1.ModelVariant{
		VariantId:      "probe-model",
		ProviderClass:  "ollama",
		ServedModelTag: "probe-model:latest",
		ModelDigest:    repeatHex('d', 64),
		ContextLimit:   4096,
	}
	dispatcher := &stubGovernedCapabilityProbeDispatcher{bySuffix: map[string]func(InferenceProbeRequest) (*operatorv1.InferenceDispatchResponse, error){
		"completion": func(req InferenceProbeRequest) (*operatorv1.InferenceDispatchResponse, error) {
			return probeResponse(req.ProviderAttemptID, "capability-probe-ok"), nil
		},
		"tools": func(InferenceProbeRequest) (*operatorv1.InferenceDispatchResponse, error) {
			return nil, fmt.Errorf("tools unsupported")
		},
		"structured": func(req InferenceProbeRequest) (*operatorv1.InferenceDispatchResponse, error) {
			return probeResponse(req.ProviderAttemptID, "not-json"), nil
		},
		"thinking": func(InferenceProbeRequest) (*operatorv1.InferenceDispatchResponse, error) {
			return nil, fmt.Errorf("thinking unsupported")
		},
	}}
	runner := NewGovernedCapabilityProbeRunner(dispatcher, "infer-session", func(prefix string) string { return prefix })
	observations, err := runner.RunCapabilityProbes(context.Background(), variant)
	require.NoError(t, err)
	require.Len(t, observations, len(RequiredModelCapabilityKinds()))
	outcomes := map[evalv1.ModelCapabilityKind]evalv1.EvaluationVerdictStatus{}
	for _, observation := range observations {
		outcomes[observation.GetCapability()] = observation.GetOutcome()
		assert.NotEmpty(t, observation.GetObservationDetail())
	}
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, outcomes[evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_COMPLETION])
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED, outcomes[evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_TOOL_CALLING])
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, outcomes[evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT])
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED, outcomes[evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_THINKING])
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, outcomes[evalv1.ModelCapabilityKind_MODEL_CAPABILITY_KIND_CONTEXT_LIMIT])
}

func probeResponse(attemptID, text string) *operatorv1.InferenceDispatchResponse {
	result := &operatorv1.InferenceResult{
		ProviderAttemptId: attemptID,
		RequestedModel:    "probe-model:latest",
		ResultDigest:      "digest",
		Parts:             []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: text}}},
	}
	return &operatorv1.InferenceDispatchResponse{
		Result:  result,
		Receipt: &operatorv1.ActionReceipt{ResultSummary: result.GetResultDigest()},
	}
}
