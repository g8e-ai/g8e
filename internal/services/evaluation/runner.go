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
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type Target struct {
	OperatorID string
	SessionID  string
}

type ExecutionRequest struct {
	RunID          string
	ScenarioID     string
	AttemptID      string
	Target         Target
	TargetResource string
	Marker         string
	Prohibited     bool
}

type LaneOutcome struct {
	TransactionID           string
	ExecutionID             string
	TargetIdentityMatches   bool
	ReceiptCompleted        bool
	ReceiptDurable          bool
	ProtocolChainValid      bool
	Rejected                bool
	GatewayL1Attributed     bool
	CompletedExecutionCount int64
	EvidenceRefs            []*compliancev1.ComplianceEvidenceReference
}

type PlatformLane interface {
	ResolveTarget(ctx context.Context, pinnedSessionID string) (Target, error)
	Posture(ctx context.Context) (evalv1.EvaluationGovernancePosture, error)
	Execute(ctx context.Context, request ExecutionRequest) (*LaneOutcome, error)
}

type TargetObserver interface {
	Observe(ctx context.Context, targetResource string) (*TargetState, error)
}

type RunRequest struct {
	RunID                   string
	PinnedOperatorSessionID string
	TargetResource          string
	Marker                  string
	Deployment              *evalv1.EvaluationDeploymentIdentity
}

type ReportStore interface {
	SaveReport(ctx context.Context, report *evalv1.EvaluationReport) error
	SaveTargetState(ctx context.Context, evidence *evalv1.EvaluationTargetState) (*compliancev1.ComplianceEvidenceReference, error)
}

type Runner struct {
	registry *Registry
	lane     PlatformLane
	observer TargetObserver
	grader   *Grader
	store    ReportStore
	now      func() time.Time
	newID    func(string) string
}

func NewRunner(registry *Registry, lane PlatformLane, observer TargetObserver, store ReportStore, now func() time.Time, newID func(string) string) *Runner {
	if now == nil {
		now = time.Now
	}
	return &Runner{registry: registry, lane: lane, observer: observer, grader: NewGrader(now), store: store, now: now, newID: newID}
}

func (r *Runner) Run(ctx context.Context, request RunRequest) (*evalv1.EvaluationReport, error) {
	if r == nil || r.registry == nil || r.lane == nil || r.observer == nil || r.store == nil || r.newID == nil || request.RunID == "" || request.TargetResource == "" || request.Marker == "" || request.Deployment == nil {
		return nil, fmt.Errorf("%w: runner dependencies and run request are required", constants.ErrMissingRequiredField)
	}
	suite, err := r.registry.Lookup(CoreExecutionBoundarySuiteID, CoreExecutionBoundarySuiteVersion)
	if err != nil {
		return nil, err
	}
	startedAt := r.now().UTC()
	target, targetErr := r.lane.ResolveTarget(ctx, request.PinnedOperatorSessionID)
	posture, postureErr := r.lane.Posture(ctx)
	report := &evalv1.EvaluationReport{
		SchemaVersion: RegistryVersion,
		Run: &evalv1.EvaluationRun{
			SchemaVersion: RegistryVersion, RunId: request.RunID, SuiteRef: proto.Clone(suite.Reference).(*compliancev1.VersionedReference),
			Deployment: proto.Clone(request.Deployment).(*evalv1.EvaluationDeploymentIdentity), ActivePosture: posture, Lane: suite.Lane,
			TargetOperatorId: target.OperatorID, TargetOperatorSessionId: target.SessionID, StartedAt: timestamppb.New(startedAt),
		},
	}
	if targetErr != nil || postureErr != nil || posture != suite.RequiredPosture {
		detail := firstFailure(targetErr, postureErr)
		if detail == "" {
			detail = constants.ErrEvaluationPostureUnsupported.Error()
		}
		r.buildUnavailableAttempts(report, suite, detail)
		r.finalize(report)
		persistErr := r.store.SaveReport(ctx, report)
		if persistErr != nil {
			return report, persistErr
		}
		return report, fmt.Errorf("evaluation: topology preflight: %s", detail)
	}

	allowedAttemptID := r.newID("allowed-attempt")
	initialCount, initialRef, initialObservedAt, initialErr := r.observeCount(ctx, request.RunID, AllowedExecutionScenarioID, allowedAttemptID, request.TargetResource, request.Marker)
	allowedStarted := r.now().UTC()
	allowedOutcome, allowedErr := r.lane.Execute(ctx, ExecutionRequest{
		RunID: request.RunID, ScenarioID: AllowedExecutionScenarioID, AttemptID: allowedAttemptID,
		Target: target, TargetResource: request.TargetResource, Marker: request.Marker,
	})
	allowedCount, allowedObservationRef, allowedObservedAt, allowedObservationErr := r.observeCount(ctx, request.RunID, AllowedExecutionScenarioID, allowedAttemptID, request.TargetResource, request.Marker)
	allowedAttempt := &evalv1.EvaluationAttempt{
		AttemptId: allowedAttemptID, RunId: request.RunID, ScenarioRef: versioned(AllowedExecutionScenarioID, CoreExecutionBoundarySuiteVersion),
		Status: attemptStatus(allowedErr), StartedAt: timestamppb.New(allowedStarted), CompletedAt: timestamppb.New(r.now().UTC()), FailureDetail: errorDetail(allowedErr),
	}
	if allowedOutcome != nil {
		allowedAttempt.TransactionId = allowedOutcome.TransactionID
		allowedAttempt.ExecutionId = allowedOutcome.ExecutionID
	}
	allowedObservations := []*evalv1.EvaluationObservation{
		integerObservation(r.newID("observation"), "target-marker-count-before-allowed", request.RunID, AllowedExecutionScenarioID, allowedAttemptID, initialCount, initialRef, initialErr, initialObservedAt),
		integerObservation(r.newID("observation"), "target-marker-count-after-allowed", request.RunID, AllowedExecutionScenarioID, allowedAttemptID, allowedCount, allowedObservationRef, allowedObservationErr, allowedObservedAt),
		booleanOutcomeObservation(r.newID("observation"), "target-identity-matches", request.RunID, AllowedExecutionScenarioID, allowedAttemptID, allowedOutcome, func(outcome *LaneOutcome) bool { return outcome.TargetIdentityMatches }, r.now()),
		booleanOutcomeObservation(r.newID("observation"), "terminal-receipt-completed", request.RunID, AllowedExecutionScenarioID, allowedAttemptID, allowedOutcome, func(outcome *LaneOutcome) bool { return outcome.ReceiptCompleted }, r.now()),
		booleanOutcomeObservation(r.newID("observation"), "receipt-durable", request.RunID, AllowedExecutionScenarioID, allowedAttemptID, allowedOutcome, func(outcome *LaneOutcome) bool { return outcome.ReceiptDurable }, r.now()),
		booleanOutcomeObservation(r.newID("observation"), "protocol-chain-valid", request.RunID, AllowedExecutionScenarioID, allowedAttemptID, allowedOutcome, func(outcome *LaneOutcome) bool { return outcome.ProtocolChainValid }, r.now()),
	}
	r.appendAttempt(report, allowedAttempt, suite.Scenarios[0], allowedObservations)

	prohibitedAttemptID := r.newID("prohibited-attempt")
	prohibitedStarted := r.now().UTC()
	prohibitedOutcome, prohibitedErr := r.lane.Execute(ctx, ExecutionRequest{
		RunID: request.RunID, ScenarioID: ProhibitedExecutionScenarioID, AttemptID: prohibitedAttemptID,
		Target: target, TargetResource: request.TargetResource, Marker: request.Marker, Prohibited: true,
	})
	finalCount, finalObservationRef, finalObservedAt, finalObservationErr := r.observeCount(ctx, request.RunID, ProhibitedExecutionScenarioID, prohibitedAttemptID, request.TargetResource, request.Marker)
	prohibitedAttempt := &evalv1.EvaluationAttempt{
		AttemptId: prohibitedAttemptID, RunId: request.RunID, ScenarioRef: versioned(ProhibitedExecutionScenarioID, CoreExecutionBoundarySuiteVersion),
		Status: prohibitedAttemptStatus(prohibitedOutcome, prohibitedErr), StartedAt: timestamppb.New(prohibitedStarted), CompletedAt: timestamppb.New(r.now().UTC()), FailureDetail: errorDetail(prohibitedErr),
	}
	if prohibitedOutcome != nil {
		prohibitedAttempt.TransactionId = prohibitedOutcome.TransactionID
		prohibitedAttempt.ExecutionId = prohibitedOutcome.ExecutionID
	}
	prohibitedObservations := []*evalv1.EvaluationObservation{
		booleanGatewayObservation(r.newID("observation"), "gateway-request-rejected", request.RunID, ProhibitedExecutionScenarioID, prohibitedAttemptID, prohibitedOutcome, func(outcome *LaneOutcome) bool { return outcome.Rejected }, r.now()),
		integerObservation(r.newID("observation"), "target-marker-count-after-prohibited", request.RunID, ProhibitedExecutionScenarioID, prohibitedAttemptID, finalCount, finalObservationRef, finalObservationErr, finalObservedAt),
		integerOutcomeObservation(r.newID("observation"), "completed-execution-count", request.RunID, ProhibitedExecutionScenarioID, prohibitedAttemptID, prohibitedOutcome, r.now()),
		booleanGatewayObservation(r.newID("observation"), "gateway-l1-rejection-attributed", request.RunID, ProhibitedExecutionScenarioID, prohibitedAttemptID, prohibitedOutcome, func(outcome *LaneOutcome) bool { return outcome.GatewayL1Attributed }, r.now()),
	}
	r.appendAttempt(report, prohibitedAttempt, suite.Scenarios[1], prohibitedObservations)
	r.finalize(report)
	if err := r.store.SaveReport(ctx, report); err != nil {
		return report, err
	}
	if allowedErr != nil || prohibitedErr != nil || initialErr != nil || allowedObservationErr != nil || finalObservationErr != nil || report.GetSummaryStatus() != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		return report, fmt.Errorf("evaluation: %s", report.GetSummary())
	}
	return report, nil
}

func (r *Runner) observeCount(ctx context.Context, runID, scenarioID, attemptID, targetResource, marker string) (int64, *compliancev1.ComplianceEvidenceReference, time.Time, error) {
	state, err := r.observer.Observe(ctx, targetResource)
	if err != nil {
		return 0, nil, r.now().UTC(), err
	}
	if state == nil || state.ObservedAt.IsZero() {
		return 0, nil, r.now().UTC(), fmt.Errorf("%w: observer returned incomplete target state", constants.ErrEvaluationObservationUnavailable)
	}
	count := int64(0)
	if state.Present {
		count = int64(bytes.Count(state.Content, []byte(marker)))
	}
	reference, err := r.store.SaveTargetState(ctx, &evalv1.EvaluationTargetState{
		SchemaVersion:  RegistryVersion,
		RunId:          runID,
		ScenarioId:     scenarioID,
		AttemptId:      attemptID,
		TargetResource: targetResource,
		ObservedAt:     timestamppb.New(state.ObservedAt.UTC()),
		Present:        state.Present,
		Content:        append([]byte(nil), state.Content...),
	})
	if err != nil {
		return count, nil, state.ObservedAt.UTC(), err
	}
	return count, reference, state.ObservedAt.UTC(), nil
}

func (r *Runner) appendAttempt(report *evalv1.EvaluationReport, attempt *evalv1.EvaluationAttempt, scenario ScenarioDefinition, observations []*evalv1.EvaluationObservation) {
	report.Attempts = append(report.Attempts, attempt)
	report.Run.AttemptRefs = append(report.Run.AttemptRefs, attempt.GetAttemptId())
	for _, observation := range observations {
		report.Observations = append(report.Observations, observation)
		attempt.ObservationRefs = append(attempt.ObservationRefs, observation.GetObservationId())
	}
	for _, assertion := range scenario.Assertions {
		assertionCopy := proto.Clone(assertion).(*evalv1.EvaluationAssertion)
		report.Assertions = append(report.Assertions, assertionCopy)
		attempt.AssertionRefs = append(attempt.AssertionRefs, assertionCopy.GetAssertionId())
		verdict := r.grader.Grade(attempt.GetAttemptId(), assertionCopy, observations)
		report.Verdicts = append(report.Verdicts, verdict)
		attempt.VerdictRefs = append(attempt.VerdictRefs, verdict.GetVerdictId())
	}
}

func (r *Runner) finalize(report *evalv1.EvaluationReport) {
	report.Run.CompletedAt = timestamppb.New(r.now().UTC())
	report.SummaryStatus = SummaryStatus(report.Verdicts)
	report.RequiredVerdictCount = uint32(len(report.Verdicts))
	for _, verdict := range report.Verdicts {
		if verdict.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
			report.PassedVerdictCount++
		}
	}
	report.Metrics = []*evalv1.EvaluationMetric{DeriveRequiredVerdictMetric(report.Verdicts)}
	report.Summary = VerdictSummary(report.Verdicts)
	for _, observation := range report.Observations {
		for _, reference := range observation.GetEvidenceRefs() {
			report.EvidenceRefs = appendUniqueReference(report.EvidenceRefs, reference)
		}
	}
}

func (r *Runner) buildUnavailableAttempts(report *evalv1.EvaluationReport, suite *SuiteDefinition, detail string) {
	for _, scenario := range suite.Scenarios {
		attempt := &evalv1.EvaluationAttempt{AttemptId: r.newID("attempt"), RunId: report.Run.RunId, ScenarioRef: proto.Clone(scenario.Reference).(*compliancev1.VersionedReference), Status: evalv1.EvaluationAttemptStatus_EVALUATION_ATTEMPT_STATUS_UNAVAILABLE, StartedAt: timestamppb.New(r.now().UTC()), CompletedAt: timestamppb.New(r.now().UTC()), FailureDetail: detail}
		r.appendAttempt(report, attempt, scenario, nil)
	}
}

func integerObservation(id, observationType, runID, scenarioID, attemptID string, count int64, reference *compliancev1.ComplianceEvidenceReference, observationErr error, observedAt time.Time) *evalv1.EvaluationObservation {
	observation := observation(id, observationType, runID, scenarioID, attemptID, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_TARGET_OBSERVER, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE, observedAt)
	if observationErr == nil {
		observation.Value = &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: count}}
	}
	if reference != nil {
		observation.EvidenceRefs = []*compliancev1.ComplianceEvidenceReference{reference}
	}
	return observation
}

func booleanOutcomeObservation(id, observationType, runID, scenarioID, attemptID string, outcome *LaneOutcome, value func(*LaneOutcome) bool, observedAt time.Time) *evalv1.EvaluationObservation {
	observation := observation(id, observationType, runID, scenarioID, attemptID, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE, observedAt)
	if outcome != nil {
		observation.Value = &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_BooleanValue{BooleanValue: value(outcome)}}
		observation.EvidenceRefs = cloneReferences(outcome.EvidenceRefs)
	}
	return observation
}

func booleanGatewayObservation(id, observationType, runID, scenarioID, attemptID string, outcome *LaneOutcome, value func(*LaneOutcome) bool, observedAt time.Time) *evalv1.EvaluationObservation {
	observation := observation(id, observationType, runID, scenarioID, attemptID, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_GATEWAY_ADMISSION, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_GATEWAY_COORDINATION, observedAt)
	if outcome != nil {
		observation.Value = &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_BooleanValue{BooleanValue: value(outcome)}}
		observation.EvidenceRefs = cloneReferences(outcome.EvidenceRefs)
	}
	return observation
}

func integerOutcomeObservation(id, observationType, runID, scenarioID, attemptID string, outcome *LaneOutcome, observedAt time.Time) *evalv1.EvaluationObservation {
	observation := observation(id, observationType, runID, scenarioID, attemptID, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE, observedAt)
	if outcome != nil {
		observation.Value = &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: outcome.CompletedExecutionCount}}
		observation.EvidenceRefs = cloneReferences(outcome.EvidenceRefs)
	}
	return observation
}

func observation(id, observationType, runID, scenarioID, attemptID string, source evalv1.EvaluationObservationSource, authority evalv1.EvaluationEvidenceAuthority, observedAt time.Time) *evalv1.EvaluationObservation {
	return &evalv1.EvaluationObservation{ObservationId: id, ObservationType: versioned(observationType, RegistryVersion), Source: source, ObservedAt: timestamppb.New(observedAt.UTC()), RunId: runID, ScenarioId: scenarioID, AttemptId: attemptID, Authority: authority}
}

func attemptStatus(err error) evalv1.EvaluationAttemptStatus {
	if err != nil {
		return evalv1.EvaluationAttemptStatus_EVALUATION_ATTEMPT_STATUS_FAILED
	}
	return evalv1.EvaluationAttemptStatus_EVALUATION_ATTEMPT_STATUS_COMPLETED
}

func prohibitedAttemptStatus(outcome *LaneOutcome, err error) evalv1.EvaluationAttemptStatus {
	if outcome != nil && outcome.Rejected {
		return evalv1.EvaluationAttemptStatus_EVALUATION_ATTEMPT_STATUS_REJECTED
	}
	return attemptStatus(err)
}

func firstFailure(errors ...error) string {
	for _, err := range errors {
		if err != nil {
			return err.Error()
		}
	}
	return ""
}

func errorDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
