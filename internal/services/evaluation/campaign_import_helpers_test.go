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

func TestToolOutcomeStatus_OnlyPassAndFailAreVerdicts(t *testing.T) {
	t.Parallel()
	assert.Equal(t, verdictPass, toolOutcomeStatus("pass"))
	assert.Equal(t, verdictFail, toolOutcomeStatus("fail"))
	for _, outcome := range []string{"", "unknown", "PASS", "refused"} {
		assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE, toolOutcomeStatus(outcome), outcome)
	}
}

func TestMergeSemanticGrades_ImportedGradesReplaceTheComputedOnes(t *testing.T) {
	t.Parallel()
	computed := []*evalv1.SemanticGrade{{GradeId: "computed"}}
	imported := []*evalv1.SemanticGrade{{GradeId: "imported"}}

	assert.Equal(t, imported, mergeSemanticGrades(computed, imported))
	assert.Equal(t, computed, mergeSemanticGrades(computed, nil))
	assert.Empty(t, mergeSemanticGrades(nil, nil))
}

func TestGraderCallRecordsFromTrace(t *testing.T) {
	t.Parallel()
	assignment := &evalv1.EvaluationAssignment{AssignmentId: "assignment-1"}
	newID := func(prefix string) string { return prefix + "-generated" }

	t.Run("an absent field yields no records", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, graderCallRecordsFromTrace(assignment, EvaluationTrace{}, newID))
	})

	t.Run("records keep their ids, judge, and inference reference", func(t *testing.T) {
		t.Parallel()
		trace := EvaluationTrace{"grader_calls": []any{
			EvaluationTrace{"grader_call_id": "g1", "judge_variant_id": "judge-1", "provider_attempt_id": "attempt-1"},
			EvaluationTrace{"judge_variant_id": "judge-2"},
			"not an object",
		}}

		records := graderCallRecordsFromTrace(assignment, trace, newID)

		require.Len(t, records, 2, "a non-object entry is skipped")
		assert.Equal(t, "g1", records[0].GetGraderCallId())
		assert.Equal(t, "judge-1", records[0].GetJudgeVariantId())
		assert.Equal(t, "assignment-1", records[0].GetAssignmentId())
		require.NotNil(t, records[0].GetInferenceRecordRef())
		assert.Equal(t, "attempt-1", records[0].GetInferenceRecordRef().GetArtifactId())
		assert.Equal(t, "grader-call-generated", records[1].GetGraderCallId(), "a missing id is generated")
		assert.Nil(t, records[1].GetInferenceRecordRef())
	})
}

func TestPublicFinishState_EveryReasonMapsOrIsRejected(t *testing.T) {
	t.Parallel()
	tests := map[string]evalv1.PublicFinishState{
		"stop":      evalv1.PublicFinishState_PUBLIC_FINISH_STATE_STOP,
		"STOP":      evalv1.PublicFinishState_PUBLIC_FINISH_STATE_STOP,
		"length":    evalv1.PublicFinishState_PUBLIC_FINISH_STATE_LENGTH,
		"tool_call": evalv1.PublicFinishState_PUBLIC_FINISH_STATE_TOOL_CALL,
		"tool-call": evalv1.PublicFinishState_PUBLIC_FINISH_STATE_TOOL_CALL,
		"error":     evalv1.PublicFinishState_PUBLIC_FINISH_STATE_ERROR,
		"":          evalv1.PublicFinishState_PUBLIC_FINISH_STATE_UNAVAILABLE,
	}
	for reason, want := range tests {
		got, err := publicFinishState(reason)
		require.NoError(t, err, reason)
		assert.Equal(t, want, got, reason)
	}
	_, err := publicFinishState("end_turn")
	require.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}

func TestPublicPolicyText_EveryOutcomeMapsOrIsRejected(t *testing.T) {
	t.Parallel()
	tests := map[string]evalv1.EvaluationPolicyDecisionOutcome{
		"allow":   evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_ALLOW,
		"Deny":    evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_DENY,
		"REFUSED": evalv1.EvaluationPolicyDecisionOutcome_EVALUATION_POLICY_DECISION_OUTCOME_REFUSED,
	}
	for text, want := range tests {
		got, err := publicPolicyText(text)
		require.NoError(t, err, text)
		assert.Equal(t, want, got, text)
	}
	for _, text := range []string{"", "pass", "unknown"} {
		_, err := publicPolicyText(text)
		require.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed, text)
	}
}

func TestPublicLoadState_UnspecifiedIsUnavailableAndUnknownIsRejected(t *testing.T) {
	t.Parallel()
	for _, state := range []evalv1.EvaluationLoadState{
		evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_COLD,
		evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_WARM,
		evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_UNAVAILABLE,
	} {
		got, err := publicLoadState(state)
		require.NoError(t, err)
		assert.Equal(t, state, got)
	}
	got, err := publicLoadState(evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_UNSPECIFIED)
	require.NoError(t, err)
	assert.Equal(t, evalv1.EvaluationLoadState_EVALUATION_LOAD_STATE_UNAVAILABLE, got)
	_, err = publicLoadState(evalv1.EvaluationLoadState(99))
	require.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
}
