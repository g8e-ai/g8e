// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type runnerTestLane struct {
	calls []ExecutionRequest
}

func (l *runnerTestLane) ResolveTarget(context.Context, string) (Target, error) {
	return Target{OperatorID: "operator-1", SessionID: "session-1"}, nil
}

func (l *runnerTestLane) Posture(context.Context) (evalv1.EvaluationGovernancePosture, error) {
	return evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_DOCTRINE, nil
}

func (l *runnerTestLane) Execute(_ context.Context, request ExecutionRequest) (*LaneOutcome, error) {
	l.calls = append(l.calls, request)
	if request.Prohibited {
		return &LaneOutcome{Rejected: true, GatewayL1Attributed: true, CompletedExecutionCount: 0, EvidenceRefs: []*compliancev1.ComplianceEvidenceReference{testEvidenceReference("gateway-rejection")}}, nil
	}
	return &LaneOutcome{
		TransactionID: "tx-1", ExecutionID: "execution-1", TargetIdentityMatches: true, ReceiptCompleted: true,
		ReceiptDurable: true, ProtocolChainValid: true, CompletedExecutionCount: 1,
		EvidenceRefs: []*compliancev1.ComplianceEvidenceReference{testEvidenceReference("operator-receipt")},
	}, nil
}

type runnerTestObserver struct {
	counts []int64
	index  int
}

func (o *runnerTestObserver) Observe(context.Context, string) (*TargetState, error) {
	count := o.counts[o.index]
	o.index++
	return &TargetState{
		Present:    count > 0,
		Content:    bytes.Repeat([]byte("run-1\n"), int(count)),
		ObservedAt: time.Unix(int64(o.index), 0).UTC(),
	}, nil
}

type runnerTestStore struct {
	report   *evalv1.EvaluationReport
	evidence []*evalv1.EvaluationTargetState
}

func (s *runnerTestStore) SaveReport(_ context.Context, report *evalv1.EvaluationReport) error {
	s.report = proto.Clone(report).(*evalv1.EvaluationReport)
	return nil
}

func (s *runnerTestStore) SaveTargetState(_ context.Context, evidence *evalv1.EvaluationTargetState) (*compliancev1.ComplianceEvidenceReference, error) {
	s.evidence = append(s.evidence, proto.Clone(evidence).(*evalv1.EvaluationTargetState))
	return testEvidenceReference(fmt.Sprintf("target-state-%d", len(s.evidence))), nil
}

func TestRunner_CoreExecutionBoundaryProducesPersistedPassingReport(t *testing.T) {
	lane := &runnerTestLane{}
	observer := &runnerTestObserver{counts: []int64{0, 1, 1}}
	store := &runnerTestStore{}
	now := time.Unix(1_789_473_600, 0).UTC()
	counter := 0
	runner := NewRunner(NewRegistry(), lane, observer, store, func() time.Time { return now }, func(prefix string) string {
		counter++
		return fmt.Sprintf("%s-%d", prefix, counter)
	})

	report, err := runner.Run(context.Background(), RunRequest{
		RunID: "run-1", TargetResource: "evaluation-target.txt", Marker: "run-1",
		Deployment: &evalv1.EvaluationDeploymentIdentity{DeploymentId: "deployment-1", TopologyRef: versioned(TopologyID, TopologyVersion)},
	})

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, report.SummaryStatus)
	assert.Equal(t, report.RequiredVerdictCount, report.PassedVerdictCount)
	assert.Equal(t, uint32(10), report.RequiredVerdictCount)
	require.Len(t, report.Attempts, 2)
	assert.Equal(t, evalv1.EvaluationAttemptStatus_EVALUATION_ATTEMPT_STATUS_COMPLETED, report.Attempts[0].Status)
	assert.Equal(t, evalv1.EvaluationAttemptStatus_EVALUATION_ATTEMPT_STATUS_REJECTED, report.Attempts[1].Status)
	require.Len(t, lane.calls, 2)
	assert.False(t, lane.calls[0].Prohibited)
	assert.True(t, lane.calls[1].Prohibited)
	require.NotNil(t, store.report)
	assert.True(t, proto.Equal(report, store.report))
	require.Len(t, store.evidence, 3)
	assert.Equal(t, AllowedExecutionScenarioID, store.evidence[0].ScenarioId)
	assert.Equal(t, report.Attempts[0].AttemptId, store.evidence[0].AttemptId)
	assert.False(t, store.evidence[0].Present)
	assert.Equal(t, ProhibitedExecutionScenarioID, store.evidence[2].ScenarioId)
	assert.Equal(t, report.Attempts[1].AttemptId, store.evidence[2].AttemptId)
	require.Len(t, report.Metrics, 1)
	assert.Equal(t, int64(10), report.Metrics[0].Numerator)
	assert.Equal(t, int64(10), report.Metrics[0].Denominator)
	assert.Equal(t, 1.0, report.Metrics[0].Value)
}

func TestGrader_FailsClosedForMissingContradictoryAndWrongAuthorityEvidence(t *testing.T) {
	assertion := equalIntegerAssertion("count", 1, "target-count", evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE)
	newObservation := func(id string, count int64, authority evalv1.EvaluationEvidenceAuthority, withEvidence bool) *evalv1.EvaluationObservation {
		observation := &evalv1.EvaluationObservation{ObservationId: id, ObservationType: versioned("target-count", RegistryVersion), Authority: authority, Value: &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: count}}}
		if withEvidence {
			observation.EvidenceRefs = []*compliancev1.ComplianceEvidenceReference{testEvidenceReference(id)}
		}
		return observation
	}
	tests := []struct {
		name         string
		observations []*evalv1.EvaluationObservation
		want         evalv1.EvaluationVerdictStatus
	}{
		{name: "missing observation", want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE},
		{name: "wrong authority", observations: []*evalv1.EvaluationObservation{newObservation("wrong", 1, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_GATEWAY_COORDINATION, true)}, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE},
		{name: "missing evidence reference", observations: []*evalv1.EvaluationObservation{newObservation("unbound", 1, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE, false)}, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE},
		{name: "contradictory observations", observations: []*evalv1.EvaluationObservation{newObservation("one", 1, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE, true), newObservation("two", 2, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE, true)}, want: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE},
	}
	grader := NewGrader(func() time.Time { return time.Unix(1, 0).UTC() })
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verdict := grader.Grade("attempt-1", assertion, test.observations)
			assert.Equal(t, test.want, verdict.Status)
		})
	}
}

func TestGrader_MalformedAssertionIsInvalidEvidenceNotUnavailable(t *testing.T) {
	grader := NewGrader(func() time.Time { return time.Unix(1, 0).UTC() })
	observation := &evalv1.EvaluationObservation{
		ObservationId:   "obs-1",
		ObservationType: versioned("target-count", RegistryVersion),
		Authority:       evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE,
		Value:           &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: 1}},
		EvidenceRefs:    []*compliancev1.ComplianceEvidenceReference{testEvidenceReference("obs-1")},
	}
	tests := []struct {
		name      string
		assertion *evalv1.EvaluationAssertion
	}{
		{name: "no required observation type", assertion: &evalv1.EvaluationAssertion{AssertionId: "a", AssertionVersion: RegistryVersion, Comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL, Expected: &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: 1}}, RequiredAuthorities: []evalv1.EvaluationEvidenceAuthority{evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE}}},
		{name: "no required authority", assertion: &evalv1.EvaluationAssertion{AssertionId: "a", AssertionVersion: RegistryVersion, Comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL, Expected: &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: 1}}, RequiredObservationTypes: []*compliancev1.VersionedReference{versioned("target-count", RegistryVersion)}}},
		{name: "missing expected value", assertion: &evalv1.EvaluationAssertion{AssertionId: "a", AssertionVersion: RegistryVersion, Comparator: evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL, RequiredObservationTypes: []*compliancev1.VersionedReference{versioned("target-count", RegistryVersion)}, RequiredAuthorities: []evalv1.EvaluationEvidenceAuthority{evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE}}},
		{name: "nil assertion", assertion: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verdict := grader.Grade("attempt-1", test.assertion, []*evalv1.EvaluationObservation{observation})
			assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE, verdict.Status)
		})
	}
}

func testEvidenceReference(id string) *compliancev1.ComplianceEvidenceReference {
	return &compliancev1.ComplianceEvidenceReference{ArtifactId: id + ":sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ArtifactType: id, Sha256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
}
