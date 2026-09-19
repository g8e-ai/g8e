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

	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type stubCapabilityProbeBackend struct {
	byAttempt map[string]func(models.GenerateRequest) (*models.GenerateResponse, error)
}

func (s *stubCapabilityProbeBackend) Generate(_ context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
	handler, ok := s.byAttempt[req.ProviderAttemptID]
	if !ok {
		return nil, fmt.Errorf("unexpected attempt %s", req.ProviderAttemptID)
	}
	return handler(req)
}

func TestRunModelCapabilityProbes_RecordsDescriptiveOutcomesWithoutExclusion(t *testing.T) {
	variant := &evalv1.ModelVariant{
		VariantId:      "probe-model",
		ProviderClass:  "ollama",
		ServedModelTag: "probe-model:latest",
		ModelDigest:    repeatHex('d', 64),
		ContextLimit:   4096,
	}
	backend := &stubCapabilityProbeBackend{byAttempt: map[string]func(models.GenerateRequest) (*models.GenerateResponse, error){
		capabilityProbeAttemptPrefix + "-completion": func(models.GenerateRequest) (*models.GenerateResponse, error) {
			return &models.GenerateResponse{Parts: []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "capability-probe-ok"}}}}, nil
		},
		capabilityProbeAttemptPrefix + "-tools": func(models.GenerateRequest) (*models.GenerateResponse, error) {
			return nil, fmt.Errorf("tools unsupported")
		},
		capabilityProbeAttemptPrefix + "-structured": func(models.GenerateRequest) (*models.GenerateResponse, error) {
			return &models.GenerateResponse{Parts: []*operatorv1.InferenceResponsePart{{Part: &operatorv1.InferenceResponsePart_Text{Text: "not-json"}}}}, nil
		},
		capabilityProbeAttemptPrefix + "-thinking": func(models.GenerateRequest) (*models.GenerateResponse, error) {
			return nil, fmt.Errorf("thinking unsupported")
		},
	}}
	observations, err := RunModelCapabilityProbes(context.Background(), backend, variant)
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
