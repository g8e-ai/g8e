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

func TestDefaultInferenceAcceptanceCases_CoversPhase1AMatrix(t *testing.T) {
	t.Parallel()
	cases := DefaultInferenceAcceptanceCases()
	require.Len(t, cases, 9)
	ids := make([]InferenceAcceptanceCaseID, 0, len(cases))
	for _, acceptanceCase := range cases {
		ids = append(ids, acceptanceCase.ID)
	}
	assert.Contains(t, ids, InferenceAcceptanceCaseStreamingProgress)
	assert.Contains(t, ids, InferenceAcceptanceCaseToolContinuation)
}

func TestBuildInferenceProbeDispatchRequest_StructuredCasePreservesSchema(t *testing.T) {
	t.Parallel()
	base := InferenceProbeRequest{
		ProviderAttemptID:       "attempt-1",
		Role:                    models.InferenceModelRolePrimary,
		Model:                   "probe-model",
		TargetOperatorSessionID: "sess-1",
		Prompt:                  "Return JSON.",
		ResponseFormat:          ProbeStructuredResponseFormat(),
	}
	req, err := BuildInferenceProbeDispatchRequest(base)
	require.NoError(t, err)
	assert.Equal(t, "application/json", req.GetResponseFormat().GetMediaType())
	assert.Contains(t, req.GetResponseFormat().GetJsonSchema(), `"answer"`)
}

func TestBuildInferenceProbeDispatchRequest_ToolContinuationPreservesHistory(t *testing.T) {
	t.Parallel()
	acceptanceCase, err := LookupInferenceAcceptanceCase(InferenceAcceptanceCaseToolContinuation)
	require.NoError(t, err)
	base := InferenceProbeRequest{
		ProviderAttemptID:       "attempt-1",
		Role:                    models.InferenceModelRolePrimary,
		Model:                   "probe-model",
		TargetOperatorSessionID: "sess-1",
	}
	probeReq := acceptanceCase.Apply(base)
	req, err := BuildInferenceProbeDispatchRequest(probeReq)
	require.NoError(t, err)
	require.Len(t, req.GetMessages(), 3)
	assert.Equal(t, operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL, req.GetMessages()[2].GetRole())
	assert.Equal(t, "probe_echo", req.GetTools()[0].GetName())
}

func TestValidateInferenceAcceptanceCase_StructuredJSON(t *testing.T) {
	t.Parallel()
	resp := &operatorv1.InferenceDispatchResponse{
		Result: &operatorv1.InferenceResult{
			Parts: []*operatorv1.InferenceResponsePart{{
				Part: &operatorv1.InferenceResponsePart_Text{Text: `{"answer":"structured-ok"}`},
			}},
		},
		Receipt: &operatorv1.ActionReceipt{ResultSummary: "ignored"},
	}
	require.NoError(t, ValidateInferenceAcceptanceCase(InferenceAcceptanceCaseStructuredJSON, resp))
}

func TestValidateInferenceAcceptanceCase_StructuredJSONAcceptsFencedPayload(t *testing.T) {
	t.Parallel()
	resp := &operatorv1.InferenceDispatchResponse{
		Result: &operatorv1.InferenceResult{
			Parts: []*operatorv1.InferenceResponsePart{{
				Part: &operatorv1.InferenceResponsePart_Text{Text: "```json\n{\"answer\":\"structured-ok\"}\n```"},
			}},
		},
	}
	require.NoError(t, ValidateInferenceAcceptanceCase(InferenceAcceptanceCaseStructuredJSON, resp))
}

func TestStructuredJSONCase_UsesSystemAndUserMessages(t *testing.T) {
	t.Parallel()
	acceptanceCase, err := LookupInferenceAcceptanceCase(InferenceAcceptanceCaseStructuredJSON)
	require.NoError(t, err)
	probeReq := acceptanceCase.Apply(InferenceProbeRequest{
		ProviderAttemptID:       "attempt-1",
		Role:                    models.InferenceModelRolePrimary,
		Model:                   "probe-model",
		TargetOperatorSessionID: "sess-1",
	})
	req, err := BuildInferenceProbeDispatchRequest(probeReq)
	require.NoError(t, err)
	require.Len(t, req.GetMessages(), 2)
	assert.Equal(t, operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_SYSTEM, req.GetMessages()[0].GetRole())
	assert.Equal(t, operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER, req.GetMessages()[1].GetRole())
	assert.Contains(t, req.GetMessages()[1].GetParts()[0].GetText(), "structured-ok")
}

func TestValidateInferenceAcceptanceCase_ToolSelectionRequiresCall(t *testing.T) {
	t.Parallel()
	resp := &operatorv1.InferenceDispatchResponse{
		Result: &operatorv1.InferenceResult{
			Parts: []*operatorv1.InferenceResponsePart{{
				Part: &operatorv1.InferenceResponsePart_Text{Text: "no tool"},
			}},
		},
	}
	err := ValidateInferenceAcceptanceCase(InferenceAcceptanceCaseToolSelection, resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
}

func TestSelectInferenceAcceptanceCases_FiltersRequestedIDs(t *testing.T) {
	t.Parallel()
	selected, err := SelectInferenceAcceptanceCases([]InferenceAcceptanceCaseID{
		InferenceAcceptanceCaseUnaryBasic,
		InferenceAcceptanceCaseStreamingProgress,
	})
	require.NoError(t, err)
	require.Len(t, selected, 2)
	assert.Equal(t, InferenceAcceptanceCaseUnaryBasic, selected[0].ID)
	assert.True(t, selected[1].Stream)
}

func TestValidateInferenceAcceptanceCase_TextMatchIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	resp := &operatorv1.InferenceDispatchResponse{
		Result: &operatorv1.InferenceResult{
			Parts: []*operatorv1.InferenceResponsePart{{
				Part: &operatorv1.InferenceResponsePart_Text{Text: strings.ToUpper("probe-ok")},
			}},
		},
	}
	require.NoError(t, ValidateInferenceAcceptanceCase(InferenceAcceptanceCaseUnaryBasic, resp))
}
