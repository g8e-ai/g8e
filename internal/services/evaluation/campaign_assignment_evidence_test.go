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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestBuildPublicAssignmentEvidence(t *testing.T) {
	result := &evalv1.EvaluationAssignmentResult{
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			ModelRole:         evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
			AgentPersona:      "primary",
			ModelVariant:      &evalv1.ModelVariant{VariantId: "variant-a"},
			UsageAvailability: evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED,
			FinishReason:      "tool_call",
		}},
		ToolDecisionsCaptured:   true,
		ToolDecisions:           []*evalv1.ToolDecisionRecord{{ToolName: "read_file", Outcome: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS}},
		ToolCallsCaptured:       true,
		ToolCalls:               []*evalv1.ToolCallRecord{{ToolName: "read_file", SchemaOutcome: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, SemanticOutcome: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE}},
		PolicyDecisionsCaptured: true,
		PolicyDecisions:         []*evalv1.PolicyDecisionRecord{{ToolName: "", Outcome: evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_DENY}},
		GovernedActionsCaptured: true,
		GovernedActions:         []*evalv1.GovernedActionBinding{{PolicyDecision: "deny", TransactionId: "private-transaction"}},
	}
	activity, bindings, err := BuildPublicAssignmentEvidence(PublicAssignmentEvidenceInput{
		Result:       result,
		PublicProofs: []*evalv1.PublicEvidenceBinding{{Sha256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SchemaRef: "g8e.eval.v1.PublicProjection", Kind: "evaluation_projection"}},
	})
	require.NoError(t, err)
	require.NotNil(t, activity)
	require.NotNil(t, activity.Summary)
	assert.Len(t, activity.Summary.ModelActivity.Records, 1)
	assert.Equal(t, "variant-a", activity.Summary.ModelActivity.Records[0].GetVariantId())
	assert.Equal(t, "governed action", activity.Summary.GovernedActions.Records[0].GetActionLabel())
	assert.Equal(t, evalv1.PublicReceiptStatus_PUBLIC_RECEIPT_STATUS_UNAVAILABLE, activity.Summary.GovernedActions.Records[0].GetReceiptStatus())
	assert.Len(t, bindings, 1)
	assert.Equal(t, "primary", activity.Summary.ModelActivity.Records[0].GetAgentPersona())
}

func TestBuildPublicAssignmentEvidence_MissingCaptureAndEmptyObservedList(t *testing.T) {
	activity, _, err := BuildPublicAssignmentEvidence(PublicAssignmentEvidenceInput{Result: &evalv1.EvaluationAssignmentResult{ToolDecisionsCaptured: true}})
	require.NoError(t, err)
	assert.Equal(t, evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED, activity.Summary.ToolDecisions.Availability)
	assert.Empty(t, activity.Summary.ToolDecisions.Records)
	assert.Equal(t, evalv1.PublicActivityAvailability_PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE, activity.Summary.ToolCalls.Availability)
	assert.Equal(t, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_NOT_CAPTURED, activity.Summary.ToolCalls.UnavailableReason)
}

func TestBuildPublicAssignmentEvidence_RejectsPrivateOrMalformedProof(t *testing.T) {
	base := PublicAssignmentEvidenceInput{Result: &evalv1.EvaluationAssignmentResult{}}
	for _, test := range []struct {
		name    string
		binding *evalv1.PublicEvidenceBinding
	}{
		{name: "uppercase hash", binding: &evalv1.PublicEvidenceBinding{Sha256: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", SchemaRef: "schema", Kind: "evaluation_projection"}},
		{name: "private kind", binding: &evalv1.PublicEvidenceBinding{Sha256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SchemaRef: "schema", Kind: "trace"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.PublicProofs = []*evalv1.PublicEvidenceBinding{test.binding}
			_, _, err := BuildPublicAssignmentEvidence(input)
			assert.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
		})
	}
}

func TestBuildPublicAssignmentEvidence_RejectsUnknownActivityEnum(t *testing.T) {
	_, _, err := BuildPublicAssignmentEvidence(PublicAssignmentEvidenceInput{Result: &evalv1.EvaluationAssignmentResult{
		ToolCallsCaptured: true,
		ToolCalls:         []*evalv1.ToolCallRecord{{ToolName: "read_file", SchemaOutcome: evalv1.EvaluationVerdictStatus(99)}},
	}})
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}
