// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestDefaultChatAcceptanceCases_CoversPhase1AMatrix(t *testing.T) {
	t.Parallel()
	cases := DefaultChatAcceptanceCases()
	require.Len(t, cases, 5)
	ids := make([]ChatAcceptanceCaseID, 0, len(cases))
	for _, acceptanceCase := range cases {
		ids = append(ids, acceptanceCase.ID)
	}
	assert.Contains(t, ids, ChatAcceptanceCaseRolePrimary)
	assert.Contains(t, ids, ChatAcceptanceCaseBackgroundBarrier)
}

func TestBuildChatProbeRequest_ModelRoleLaneRequiresDesignatedRole(t *testing.T) {
	t.Parallel()
	_, err := BuildChatProbeRequest(ChatProbeRequest{
		AssignmentID:            "assignment-1",
		EvaluationAttemptID:     "attempt-1",
		CampaignID:              "campaign-1",
		RunID:                   "run-1",
		ScenarioID:              "scenario-1",
		Model:                   "model-a",
		ModelDigest:             "a" + repeatHex('a', 63),
		TargetOperatorSessionID: "session-1",
		ModelRegistryDigest:     "d" + repeatHex('d', 63),
		ModelRegistry:           []*operatorv1.InferenceModelVariant{{Model: "model-a", Digest: "a" + repeatHex('a', 63)}},
		EvaluationLane:          "model_role",
	}, "data-op-1", "data-session-1")
	require.Error(t, err)
}

func TestBuildChatProbeRequest_ModelRoleLanePreservesContext(t *testing.T) {
	t.Parallel()
	req, err := BuildChatProbeRequest(ChatProbeRequest{
		AssignmentID:            "assignment-1",
		EvaluationAttemptID:     "attempt-1",
		CampaignID:              "campaign-1",
		RunID:                   "run-1",
		ScenarioID:              "scenario-1",
		Model:                   "model-a",
		ModelDigest:             "a" + repeatHex('a', 63),
		TargetOperatorSessionID: "session-1",
		ModelRegistryDigest:     "d" + repeatHex('d', 63),
		ModelRegistry:           []*operatorv1.InferenceModelVariant{{Model: "model-a", Digest: "a" + repeatHex('a', 63)}},
		EvaluationLane:          "model_role",
		DesignatedModelRole:     "assistant",
		Message:                 "Reply with exactly: chat-role-assistant-ok",
	}, "data-op-1", "data-session-1")
	require.NoError(t, err)
	require.NotNil(t, req.EvaluationContext)
	assert.Equal(t, "model_role", req.EvaluationContext.EvaluationLane)
	assert.Equal(t, "assistant", req.EvaluationContext.DesignatedModelRole)
	assert.Equal(t, "g8e", req.LLMPrimaryProvider)
}

func TestValidateChatAcceptanceCase_RolePrimaryRequiresInvokedOutcome(t *testing.T) {
	t.Parallel()
	trace := EvaluationTrace{
		"controlled_role_assignment": EvaluationTrace{
			"designated_model_role": "primary",
		},
		"role_outcome": "role_not_invoked",
		"model_calls": []any{
			EvaluationTrace{
				"agent_role": "sage",
				"model_role": "assistant",
			},
		},
	}
	err := ValidateChatAcceptanceCase(ChatAcceptanceCaseRolePrimary, ChatProbeRequest{DesignatedModelRole: "primary"}, trace)
	require.Error(t, err)
}

func TestValidateChatAcceptanceCase_BackgroundBarrierRequiresCodex(t *testing.T) {
	t.Parallel()
	trace := EvaluationTrace{
		"model_calls": []any{
			EvaluationTrace{"agent_role": "sage"},
		},
	}
	err := ValidateChatAcceptanceCase(ChatAcceptanceCaseBackgroundBarrier, ChatProbeRequest{}, trace)
	require.Error(t, err)
}

func TestSelectChatAcceptanceCases_FiltersRequestedIDs(t *testing.T) {
	t.Parallel()
	selected, err := SelectChatAcceptanceCases([]ChatAcceptanceCaseID{
		ChatAcceptanceCaseSystemBasic,
		ChatAcceptanceCaseRoleLite,
	})
	require.NoError(t, err)
	require.Len(t, selected, 2)
	assert.Equal(t, ChatAcceptanceCaseSystemBasic, selected[0].ID)
}

func repeatHex(ch byte, count int) string {
	out := make([]byte, count)
	for i := range out {
		out[i] = ch
	}
	return string(out)
}
