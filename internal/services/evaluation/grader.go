// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type Grader struct {
	now func() time.Time
}

func NewGrader(now func() time.Time) *Grader {
	if now == nil {
		now = time.Now
	}
	return &Grader{now: now}
}

func (g *Grader) Grade(attemptID string, assertion *evalv1.EvaluationAssertion, observations []*evalv1.EvaluationObservation) *evalv1.EvaluationVerdict {
	verdict := &evalv1.EvaluationVerdict{
		VerdictId:    attemptID + ":" + assertion.GetAssertionId(),
		AssertionRef: versioned(assertion.GetAssertionId(), assertion.GetAssertionVersion()),
		GraderRef:    versioned(GraderID, GraderVersion),
		EvaluatedAt:  timestamppb.New(g.now().UTC()),
	}
	if len(assertion.GetRequiredObservationTypes()) != 1 || len(assertion.GetRequiredAuthorities()) != 1 || assertion.GetExpected().GetValue() == nil {
		verdict.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE
		verdict.FailureReason = "assertion is malformed"
		return verdict
	}
	matched, typePresent := matchingObservations(assertion, observations)
	if len(matched) == 0 {
		if typePresent {
			verdict.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE
			verdict.FailureReason = "required observation authority is missing"
		} else {
			verdict.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE
			verdict.FailureReason = "required observation is unavailable"
		}
		return verdict
	}
	for _, observation := range matched {
		verdict.ObservedRefs = append(verdict.ObservedRefs, observation.GetObservationId())
		if len(observation.GetEvidenceRefs()) == 0 {
			verdict.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE
			verdict.FailureReason = "required observation has no resolvable evidence reference"
			return verdict
		}
		verdict.EvidenceRefs = append(verdict.EvidenceRefs, observation.GetEvidenceRefs()...)
	}
	for index := 1; index < len(matched); index++ {
		if !proto.Equal(matched[0].GetValue(), matched[index].GetValue()) {
			verdict.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE
			verdict.FailureReason = "required observations contradict each other"
			return verdict
		}
	}
	passed, supported := compareValues(assertion.GetComparator(), matched[0].GetValue(), assertion.GetExpected())
	if !supported {
		verdict.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED
		verdict.FailureReason = "assertion comparator or value type is unsupported"
		return verdict
	}
	if passed {
		verdict.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
		return verdict
	}
	verdict.Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	verdict.FailureReason = "observed value does not satisfy the assertion"
	return verdict
}

func DeriveRequiredVerdictMetric(verdicts []*evalv1.EvaluationVerdict) *evalv1.EvaluationMetric {
	metric := &evalv1.EvaluationMetric{
		MetricId: MetricRequiredVerdictPassRate, MetricVersion: RegistryVersion,
		Unit:                  evalv1.EvaluationMetricUnit_EVALUATION_METRIC_UNIT_RATIO,
		Direction:             evalv1.EvaluationMetricDirection_EVALUATION_METRIC_DIRECTION_HIGHER_IS_BETTER,
		EligiblePopulationRef: versioned(MetricEligiblePopulation, RegistryVersion),
		MissingDataPolicy:     evalv1.EvaluationMissingDataPolicy_EVALUATION_MISSING_DATA_POLICY_FAIL,
	}
	for _, verdict := range verdicts {
		metric.Denominator++
		metric.SourceVerdictRefs = append(metric.SourceVerdictRefs, verdict.GetVerdictId())
		metric.EvidenceRefs = append(metric.EvidenceRefs, verdict.GetEvidenceRefs()...)
		if verdict.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
			metric.Numerator++
		}
	}
	if metric.Denominator > 0 {
		metric.Value = float64(metric.Numerator) / float64(metric.Denominator)
	}
	return metric
}

func SummaryStatus(verdicts []*evalv1.EvaluationVerdict) evalv1.EvaluationVerdictStatus {
	status := evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	for _, verdict := range verdicts {
		switch verdict.GetStatus() {
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE:
			return verdict.GetStatus()
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE:
			if status != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL && status != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED {
				status = verdict.GetStatus()
			}
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED:
			if status != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL {
				status = verdict.GetStatus()
			}
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL:
			status = verdict.GetStatus()
		case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS:
		default:
			return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE
		}
	}
	return status
}

func matchingObservations(assertion *evalv1.EvaluationAssertion, observations []*evalv1.EvaluationObservation) ([]*evalv1.EvaluationObservation, bool) {
	requiredType := assertion.GetRequiredObservationTypes()[0]
	requiredAuthority := assertion.GetRequiredAuthorities()[0]
	matched := []*evalv1.EvaluationObservation{}
	typePresent := false
	for _, observation := range observations {
		if observation == nil || observation.GetObservationType().GetId() != requiredType.GetId() || observation.GetObservationType().GetVersion() != requiredType.GetVersion() {
			continue
		}
		typePresent = true
		if observation.GetAuthority() == requiredAuthority && observation.GetValue() != nil {
			matched = append(matched, observation)
		}
	}
	return matched, typePresent
}

func compareValues(comparator evalv1.EvaluationComparator, observed, expected *evalv1.EvaluationValue) (bool, bool) {
	if observed == nil || expected == nil {
		return false, false
	}
	switch left := observed.GetValue().(type) {
	case *evalv1.EvaluationValue_BooleanValue:
		right, ok := expected.GetValue().(*evalv1.EvaluationValue_BooleanValue)
		if !ok || (comparator != evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL && comparator != evalv1.EvaluationComparator_EVALUATION_COMPARATOR_NOT_EQUAL) {
			return false, false
		}
		return (left.BooleanValue == right.BooleanValue) == (comparator == evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL), true
	case *evalv1.EvaluationValue_IntegerValue:
		right, ok := expected.GetValue().(*evalv1.EvaluationValue_IntegerValue)
		if !ok {
			return false, false
		}
		return compareIntegers(comparator, left.IntegerValue, right.IntegerValue)
	case *evalv1.EvaluationValue_StringValue:
		right, ok := expected.GetValue().(*evalv1.EvaluationValue_StringValue)
		if !ok || (comparator != evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL && comparator != evalv1.EvaluationComparator_EVALUATION_COMPARATOR_NOT_EQUAL) {
			return false, false
		}
		return (left.StringValue == right.StringValue) == (comparator == evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL), true
	case *evalv1.EvaluationValue_ArtifactReference:
		right, ok := expected.GetValue().(*evalv1.EvaluationValue_ArtifactReference)
		if !ok || comparator != evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL {
			return false, false
		}
		return proto.Equal(left.ArtifactReference, right.ArtifactReference), true
	default:
		return false, false
	}
}

func compareIntegers(comparator evalv1.EvaluationComparator, left, right int64) (bool, bool) {
	switch comparator {
	case evalv1.EvaluationComparator_EVALUATION_COMPARATOR_EQUAL:
		return left == right, true
	case evalv1.EvaluationComparator_EVALUATION_COMPARATOR_NOT_EQUAL:
		return left != right, true
	case evalv1.EvaluationComparator_EVALUATION_COMPARATOR_GREATER_THAN:
		return left > right, true
	case evalv1.EvaluationComparator_EVALUATION_COMPARATOR_GREATER_THAN_OR_EQUAL:
		return left >= right, true
	case evalv1.EvaluationComparator_EVALUATION_COMPARATOR_LESS_THAN:
		return left < right, true
	case evalv1.EvaluationComparator_EVALUATION_COMPARATOR_LESS_THAN_OR_EQUAL:
		return left <= right, true
	default:
		return false, false
	}
}

func VerdictSummary(verdicts []*evalv1.EvaluationVerdict) string {
	passed := 0
	for _, verdict := range verdicts {
		if verdict.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
			passed++
		}
	}
	return fmt.Sprintf("%d/%d required invariants passed", passed, len(verdicts))
}

func cloneReferences(references []*compliancev1.ComplianceEvidenceReference) []*compliancev1.ComplianceEvidenceReference {
	clones := make([]*compliancev1.ComplianceEvidenceReference, 0, len(references))
	for _, reference := range references {
		if reference != nil {
			clones = append(clones, proto.Clone(reference).(*compliancev1.ComplianceEvidenceReference))
		}
	}
	return clones
}
