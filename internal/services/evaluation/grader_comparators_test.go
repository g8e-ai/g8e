// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestSummaryStatus_Precedence(t *testing.T) {
	t.Parallel()
	pass := &evalv1.EvaluationVerdict{Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS}
	fail := &evalv1.EvaluationVerdict{Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL}
	unavailable := &evalv1.EvaluationVerdict{Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE}
	unsupported := &evalv1.EvaluationVerdict{Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED}
	invalid := &evalv1.EvaluationVerdict{Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE}

	tests := []struct {
		name     string
		verdicts []*evalv1.EvaluationVerdict
		want     evalv1.EvaluationVerdictStatus
	}{
		{name: "all pass", verdicts: []*evalv1.EvaluationVerdict{pass, pass}, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS},
		{name: "fail beats pass", verdicts: []*evalv1.EvaluationVerdict{pass, fail}, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL},
		{name: "invalid beats fail", verdicts: []*evalv1.EvaluationVerdict{fail, invalid}, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE},
		{name: "unsupported beats unavailable", verdicts: []*evalv1.EvaluationVerdict{unavailable, unsupported}, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED},
		{name: "fail beats unavailable", verdicts: []*evalv1.EvaluationVerdict{unavailable, fail}, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, SummaryStatus(test.verdicts))
		})
	}
}

func TestDeriveRequiredVerdictMetric_CountsPasses(t *testing.T) {
	t.Parallel()
	verdicts := []*evalv1.EvaluationVerdict{
		{
			VerdictId: "v-1",
			Status:    evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
			EvidenceRefs: []*compliancev1.ComplianceEvidenceReference{
				testEvidenceReference("pass"),
			},
		},
		{
			VerdictId: "v-2",
			Status:    evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
			EvidenceRefs: []*compliancev1.ComplianceEvidenceReference{
				testEvidenceReference("fail"),
			},
		},
	}
	metric := DeriveRequiredVerdictMetric(verdicts)
	assert.Equal(t, int64(1), metric.GetNumerator())
	assert.Equal(t, int64(2), metric.GetDenominator())
	assert.InDelta(t, 0.5, metric.GetValue(), 0.0001)
	assert.Equal(t, []string{"v-1", "v-2"}, metric.GetSourceVerdictRefs())
}

func TestGrader_IntegerComparators(t *testing.T) {
	t.Parallel()
	grader := NewGrader(func() time.Time { return time.Unix(1, 0).UTC() })
	observation := graderIntegerObservation("obs-1", 10)
	tests := []struct {
		name       string
		comparator evalv1.EvaluationComparator
		expected   int64
		want       evalv1.EvaluationVerdictStatus
	}{
		{name: "equal pass", comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL, expected: 10, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS},
		{name: "equal fail", comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL, expected: 9, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL},
		{name: "greater than pass", comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_GREATER_THAN, expected: 9, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS},
		{name: "less than or equal pass", comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_LESS_THAN_OR_EQUAL, expected: 10, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS},
		{name: "not equal pass", comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_NOT_EQUAL, expected: 9, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertion := equalIntegerAssertion("count", test.expected, "target-count", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE)
			assertion.Comparator = test.comparator
			verdict := grader.Grade("attempt-1", assertion, []*evalv1.EvaluationObservation{observation})
			assert.Equal(t, test.want, verdict.Status)
		})
	}
}

func TestBuildRecoveredTerminalAssignmentResult_MaterializesTerminalLifecycle(t *testing.T) {
	t.Parallel()
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId:    "assignment-1",
		RunId:           "run-1",
		CampaignId:      "campaign-1",
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED,
		CompletedAt:     timestamppb.New(time.Unix(1_700_000_100, 0).UTC()),
	}
	result, err := BuildRecoveredTerminalAssignmentResult(assignment, time.Unix(1_700_000_000, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, assignment.GetLifecycleStatus(), result.GetLifecycleStatus())
	assert.Equal(t, assignment.GetCompletedAt().AsTime(), result.GetCompletedAt().AsTime())
	assert.NotEmpty(t, result.GetResultDigest())
	assert.Equal(t, "recovered-terminal", result.GetDeterministicGrades()[0].GetCriterionId())
}

func TestBuildRecoveredTerminalAssignmentResult_RejectsNonTerminalLifecycle(t *testing.T) {
	t.Parallel()
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId:    "assignment-1",
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING,
	}
	_, err := BuildRecoveredTerminalAssignmentResult(assignment, time.Unix(1, 0).UTC())
	require.Error(t, err)
}

func graderIntegerObservation(id string, value int64) *evalv1.EvaluationObservation {
	return &evalv1.EvaluationObservation{
		ObservationId:   id,
		ObservationType: versioned("target-count", RegistryVersion),
		Authority:       evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE,
		Value:           &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: value}},
		EvidenceRefs:    []*compliancev1.ComplianceEvidenceReference{testEvidenceReference(id)},
	}
}
