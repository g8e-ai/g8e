// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type stubFormationProvenancePreflight struct{}

func (stubFormationProvenancePreflight) PreflightSovereignModel(_ context.Context, model FormationModel) (*FormationAttestation, error) {
	return &FormationAttestation{Verified: true, Digest: model.ModelDigest}, nil
}

type stubFormationObservationLoader struct {
	windows map[string]*evalv1.ProviderBoundaryObservationWindow
}

func (s *stubFormationObservationLoader) LoadObservationWindow(_ context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, error) {
	if s == nil || s.windows[providerAttemptID] == nil {
		return nil, constants.ErrNotFound
	}
	return s.windows[providerAttemptID], nil
}

func seedFormationObservationWindow(loader *stubFormationObservationLoader, attemptID string) {
	loader.windows[attemptID] = &evalv1.ProviderBoundaryObservationWindow{
		ProviderAttemptId: attemptID,
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			VramBytesAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			VramUsedBytes:         1024 * 1024 * 1024,
		}},
	}
}

type recordingFormationInferenceDispatcher struct {
	requests []*operatorv1.InferenceDispatchRequest
}

func (d *recordingFormationInferenceDispatcher) DispatchInference(_ context.Context, req *operatorv1.InferenceDispatchRequest) (*operatorv1.InferenceDispatchResponse, error) {
	d.requests = append(d.requests, req)
	attemptID := req.GetProviderAttemptId()
	return &operatorv1.InferenceDispatchResponse{
		Result: &operatorv1.InferenceResult{
			ProviderAttemptId: attemptID,
			RequestedModel:    req.GetModel(),
			ResultDigest:      "digest-" + attemptID,
			Parts: []*operatorv1.InferenceResponsePart{{
				Part: &operatorv1.InferenceResponsePart_Text{Text: "role-output:" + attemptID},
			}},
			CompletionTokens:     12,
			TimeToFirstTokenNs:     ptrInt64(2_000_000),
			GenerationDurationNs: ptrInt64(10_000_000),
			CampaignId:           req.GetCampaignId(),
			ModelRegistryDigest:  req.GetModelRegistryDigest(),
		},
		Receipt: &operatorv1.ActionReceipt{ResultSummary: "digest-" + attemptID},
	}, nil
}

func ptrInt64(value int64) *int64 {
	return &value
}

func ultraLightSpeedsterProductionDeps(t *testing.T, dispatcher *recordingFormationInferenceDispatcher, modelDispatcher *recordingOllamaModelCommandDispatcher) FormationProductionDependencies {
	t.Helper()
	variants := ultraLightSpeedsterRegistryVariants()
	inferenceVariants := InferenceVariantsFromEvalRegistry(variants)
	registryDigest, err := models.ComputeInferenceModelRegistryDigest("formation-campaign", inferenceVariants)
	require.NoError(t, err)
	observer := &stubFormationObservationLoader{windows: map[string]*evalv1.ProviderBoundaryObservationWindow{}}
	for _, attemptID := range []string{
		"ultra-light-speedster-lite-id",
		"ultra-light-speedster-assistant-id",
		"ultra-light-speedster-primary-id",
	} {
		seedFormationObservationWindow(observer, attemptID)
	}
	return FormationProductionDependencies{
		RunContext: FormationRunContext{
			CampaignID:          "formation-campaign",
			RunID:               "formation-run",
			AssignmentID:        "formation-assignment",
			EvaluationAttemptID: "formation-attempt",
			ModelRegistryDigest: registryDigest,
			InferenceSessionID:  "sess-inf-1",
		},
		Variants:               variants,
		ProvenancePreflight:    stubFormationProvenancePreflight{},
		ObservationLoader:      observer,
		InferenceDispatcher:    dispatcher,
		ModelCommandDispatcher: modelDispatcher,
		OllamaEnvironment:      map[string]string{"OLLAMA_HOST": "http://127.0.0.1:11434"},
		NewID:                  func(prefix string) string { return prefix + "-id" },
		Now:                    func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	}
}

func TestRunFormationProduction_ExecutesUltraLightSpeedsterThroughGovernedPath(t *testing.T) {
	dispatcher := &recordingFormationInferenceDispatcher{}
	modelDispatcher := &recordingOllamaModelCommandDispatcher{}
	deps := ultraLightSpeedsterProductionDeps(t, dispatcher, modelDispatcher)

	result, err := RunFormationProduction(context.Background(), ultraLightSpeedsterBindingRequest(t), deps, []byte("initial"))
	require.NoError(t, err)
	require.True(t, result.Passed)
	assert.Equal(t, "ultra-light-speedster", result.FormationID)
	assert.Len(t, result.Roles, 3)
	assert.True(t, result.MutationIntercepted)
	assert.Equal(t, []FormationRole{
		FormationRoleLite,
		FormationRoleAssistant,
		FormationRolePrimary,
	}, []FormationRole{result.Roles[0].Role, result.Roles[1].Role, result.Roles[2].Role})

	assert.Len(t, dispatcher.requests, 6) // 3 warmups + 3 role executions
	for _, dispatchReq := range dispatcher.requests {
		assert.Equal(t, "sess-inf-1", dispatchReq.GetTargetOperatorSessionId())
		assert.Equal(t, "formation-campaign", dispatchReq.GetCampaignId())
		assert.Equal(t, "formation-run", dispatchReq.GetRunId())
		assert.Equal(t, "formation-assignment", dispatchReq.GetAssignmentId())
		assert.Equal(t, deps.RunContext.ModelRegistryDigest, dispatchReq.GetModelRegistryDigest())
		assert.NotEmpty(t, dispatchReq.GetModelDigest())
	}
	assert.Len(t, modelDispatcher.requests, 3)
	assert.Equal(t, "/g8e operator model release qwen2.5:0.5b-instruct-q4_K_M", modelDispatcher.requests[0].Command)
	assert.Equal(t, "/g8e operator model release gemma2:2b-instruct-q4_K_M", modelDispatcher.requests[1].Command)
	assert.Equal(t, "/g8e operator model release phi3.5:3.8b-mini-instruct-q4_K_M", modelDispatcher.requests[2].Command)
}

func TestNewFormationProductionRunner_RequiresGovernedBinding(t *testing.T) {
	_, err := NewFormationProductionRunner(FormationProductionDependencies{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationRunnerDependency)
}
