// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestBuildInferenceProbeDispatchRequest_CampaignModeRequiresCompleteBinding(t *testing.T) {
	t.Parallel()
	modelDigest := strings.Repeat("a", 64)
	variants := []*operatorv1.InferenceModelVariant{{Model: "probe-model", Digest: modelDigest}}
	registryDigest, err := models.ComputeInferenceModelRegistryDigest("campaign-1", variants)
	require.NoError(t, err)
	req, err := BuildInferenceProbeDispatchRequest(InferenceProbeRequest{
		ProviderAttemptID:       "attempt-1",
		Role:                    models.InferenceModelRolePrimary,
		Model:                   "probe-model",
		ModelDigest:             modelDigest,
		TargetOperatorSessionID: "sess-inf-1",
		CampaignID:              "campaign-1",
		ModelRegistryDigest:     registryDigest,
		ModelRegistry:           variants,
	})
	require.NoError(t, err)
	assert.Equal(t, "campaign-1", req.GetCampaignId())
	assert.Equal(t, modelDigest, req.GetModelDigest())
	assert.Equal(t, registryDigest, req.GetModelRegistryDigest())
}

func TestBuildInferenceProbeDispatchRequest_RejectsIncompleteCampaignBinding(t *testing.T) {
	t.Parallel()
	_, err := BuildInferenceProbeDispatchRequest(InferenceProbeRequest{
		ProviderAttemptID:       "attempt-1",
		Role:                    models.InferenceModelRolePrimary,
		Model:                   "probe-model",
		TargetOperatorSessionID: "sess-inf-1",
		CampaignID:              "campaign-1",
	})
	require.ErrorIs(t, err, constants.ErrInferenceModelRegistryInvalid)
}

func TestBuildInferenceProbeDispatchRequest_UsesCustomMessagesWhenProvided(t *testing.T) {
	t.Parallel()
	req, err := BuildInferenceProbeDispatchRequest(InferenceProbeRequest{
		ProviderAttemptID:       "attempt-1",
		Role:                    models.InferenceModelRolePrimary,
		Model:                   "probe-model",
		TargetOperatorSessionID: "sess-inf-1",
		Messages: []*operatorv1.InferenceMessage{{
			Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
			Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: "custom"}}},
		}},
		Stream: true,
	})
	require.NoError(t, err)
	assert.True(t, req.GetStream())
	assert.Equal(t, "custom", req.GetMessages()[0].GetParts()[0].GetText())
}

func TestValidateInferenceProbeResponse_BindsIdentityAndDigest(t *testing.T) {
	t.Parallel()
	req := InferenceProbeRequest{
		ProviderAttemptID: "attempt-1",
		Model:             "probe-model",
	}
	resp := &operatorv1.InferenceDispatchResponse{
		Result: &operatorv1.InferenceResult{
			ProviderAttemptId: "attempt-1",
			RequestedModel:    "probe-model",
			ResultDigest:      "digest-1",
			Parts: []*operatorv1.InferenceResponsePart{{
				Part: &operatorv1.InferenceResponsePart_Text{Text: "ok"},
			}},
		},
		Receipt: &operatorv1.ActionReceipt{ResultSummary: "digest-1"},
	}
	require.NoError(t, ValidateInferenceProbeResponse(req, resp))
}

func TestValidateInferenceProbeResponse_RejectsMismatchedBindings(t *testing.T) {
	t.Parallel()
	req := InferenceProbeRequest{
		ProviderAttemptID: "attempt-1",
		Model:             "probe-model",
	}
	tests := []struct {
		name string
		resp *operatorv1.InferenceDispatchResponse
		want error
	}{
		{
			name: "missing result",
			resp: &operatorv1.InferenceDispatchResponse{Receipt: &operatorv1.ActionReceipt{}},
			want: constants.ErrMissingRequiredField,
		},
		{
			name: "provider attempt mismatch",
			resp: &operatorv1.InferenceDispatchResponse{
				Result: &operatorv1.InferenceResult{
					ProviderAttemptId: "other",
					RequestedModel:    "probe-model",
					ResultDigest:      "digest-1",
					Parts: []*operatorv1.InferenceResponsePart{{
						Part: &operatorv1.InferenceResponsePart_Text{Text: "ok"},
					}},
				},
				Receipt: &operatorv1.ActionReceipt{ResultSummary: "digest-1"},
			},
			want: constants.ErrIdentityBindingFailed,
		},
		{
			name: "digest mismatch",
			resp: &operatorv1.InferenceDispatchResponse{
				Result: &operatorv1.InferenceResult{
					ProviderAttemptId: "attempt-1",
					RequestedModel:    "probe-model",
					ResultDigest:      "digest-1",
					Parts: []*operatorv1.InferenceResponsePart{{
						Part: &operatorv1.InferenceResponsePart_Text{Text: "ok"},
					}},
				},
				Receipt: &operatorv1.ActionReceipt{ResultSummary: "digest-2"},
			},
			want: constants.ErrInferenceResultDigest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateInferenceProbeResponse(req, test.resp)
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestValidateInferenceProbeStream_RequiresProgressEvents(t *testing.T) {
	t.Parallel()
	req := InferenceProbeRequest{
		ProviderAttemptID: "attempt-1",
		Model:             "probe-model",
		Stream:            true,
	}
	resp := &operatorv1.InferenceDispatchResponse{
		Result: &operatorv1.InferenceResult{
			ProviderAttemptId: "attempt-1",
			RequestedModel:    "probe-model",
			ResultDigest:      "digest-1",
			Parts: []*operatorv1.InferenceResponsePart{{
				Part: &operatorv1.InferenceResponsePart_Text{Text: "ok"},
			}},
		},
		Receipt: &operatorv1.ActionReceipt{ResultSummary: "digest-1"},
	}
	err := ValidateInferenceProbeStream(req, nil, resp)
	require.ErrorIs(t, err, constants.ErrInferenceProgressHashMismatch)
}
