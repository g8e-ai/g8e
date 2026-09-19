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
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type Verifier struct {
	reader   complianceevidence.ArtifactReader
	registry *Registry
	now      func() time.Time
}

type verifiedArtifact struct {
	body []byte
}

type verificationState struct {
	report    *evalv1.EvaluationReport
	artifacts map[string]verifiedArtifact
}

func NewVerifier(reader complianceevidence.ArtifactReader, registry *Registry, now func() time.Time) *Verifier {
	if now == nil {
		now = time.Now
	}
	return &Verifier{reader: reader, registry: registry, now: now}
}

func (v *Verifier) Verify(ctx context.Context, runID string) (*compliancev1.ComplianceVerificationReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	verifiedAt := time.Time{}
	if v != nil && v.now != nil {
		verifiedAt = v.now().UTC()
	}
	result := &compliancev1.ComplianceVerificationReport{ReportId: runID, VerifiedAt: timestamppb.New(verifiedAt), VerifierId: constants.EvalRunVerifierID, VerifierVersion: constants.EvalRunVerifierVersion}
	fail := func(code error, subject, reason string) {
		result.Failures = append(result.Failures, &compliancev1.VerificationFailure{Code: code.Error(), SubjectRef: subject, Reason: reason})
	}
	if v == nil || v.reader == nil || v.registry == nil || !complianceevidence.ValidPathElement(runID) || verifiedAt.IsZero() {
		fail(constants.ErrInvalidEvidenceGraph, runID, "reader, registry, canonical run ID, and verification time are required")
		return finalizeVerification(result), nil
	}
	state := v.load(ctx, runID, fail)
	if state != nil {
		v.verifyStructure(state, fail)
		v.verifyArtifacts(ctx, state, fail)
		v.verifySemantics(state, fail)
		v.verifyDirectory(ctx, runID, state, fail)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return finalizeVerification(result), nil
}

func finalizeVerification(result *compliancev1.ComplianceVerificationReport) *compliancev1.ComplianceVerificationReport {
	sort.Slice(result.Failures, func(i, j int) bool {
		left := result.Failures[i].GetSubjectRef() + result.Failures[i].GetCode() + result.Failures[i].GetReason()
		right := result.Failures[j].GetSubjectRef() + result.Failures[j].GetCode() + result.Failures[j].GetReason()
		return left < right
	})
	result.Valid = len(result.Failures) == 0
	result.Checks = []*compliancev1.VerificationCheckResult{complianceevidence.NewVerificationCheckResult(constants.EvalRunVerificationCheck, constants.EvalRunVerifierID, constants.EvalRunVerifierVersion, []string{result.GetReportId()}, result.GetFailures())}
	return result
}

func (v *Verifier) load(ctx context.Context, runID string, fail func(error, string, string)) *verificationState {
	path := filepath.Join(evaluationRunDir(runID), constants.EvaluationReportFilename)
	body, err := v.reader.ReadFile(ctx, path)
	if err != nil {
		fail(complianceevidence.ClassifyReadError(err), path, err.Error())
		return nil
	}
	report := &evalv1.EvaluationReport{}
	if err := evalv1.UnmarshalCanonical(body, report); err != nil {
		fail(constants.ErrEvidenceArtifactMalformed, path, err.Error())
		return nil
	}
	if report.GetRun().GetRunId() != runID {
		fail(constants.ErrEvidenceScopeMismatch, path, "report run ID does not match selected run")
	}
	return &verificationState{report: report, artifacts: make(map[string]verifiedArtifact)}
}

func (v *Verifier) verifyStructure(state *verificationState, fail func(error, string, string)) {
	report := state.report
	run := report.GetRun()
	if report.GetSchemaVersion() != RegistryVersion || run == nil || run.GetSchemaVersion() != RegistryVersion || run.GetSuiteRef().GetId() != CoreExecutionBoundarySuiteID || run.GetSuiteRef().GetVersion() != CoreExecutionBoundarySuiteVersion {
		fail(constants.ErrEvidenceArtifactMalformed, constants.EvaluationReportFilename, "native evaluation schema or suite identity is unsupported")
		return
	}
	suite, err := v.registry.Lookup(run.GetSuiteRef().GetId(), run.GetSuiteRef().GetVersion())
	if err != nil {
		fail(constants.ErrEvaluationSuiteUnsupported, run.GetSuiteRef().GetId(), err.Error())
		return
	}
	if run.GetActivePosture() != suite.RequiredPosture || run.GetLane() != suite.Lane || run.GetTargetOperatorId() == "" || run.GetTargetOperatorSessionId() == "" || run.GetDeployment() == nil || run.GetDeployment().GetTopologyRef().GetId() != TopologyID || run.GetDeployment().GetTopologyRef().GetVersion() != TopologyVersion || run.GetDeployment().GetControlledTarget() == "" || run.GetDeployment().GetIndependentObserver() == "" {
		fail(constants.ErrEvidenceScopeMismatch, run.GetRunId(), "run posture, lane, target, topology, or runtime boundary is incomplete")
	}
	if !validTimestamp(run.GetStartedAt()) || !validTimestamp(run.GetCompletedAt()) || run.GetCompletedAt().AsTime().Before(run.GetStartedAt().AsTime()) {
		fail(constants.ErrStaleEvidence, run.GetRunId(), "run time window is invalid")
	}
	if len(report.GetAttempts()) != len(suite.Scenarios) || len(run.GetAttemptRefs()) != len(report.GetAttempts()) {
		fail(constants.ErrInvalidEvidenceGraph, run.GetRunId(), "report must contain exactly one attempt for each registered scenario")
	}
	attempts := make(map[string]*evalv1.EvaluationAttempt)
	for index, attempt := range report.GetAttempts() {
		if attempt == nil || !complianceevidence.ValidPathElement(attempt.GetAttemptId()) || attempt.GetRunId() != run.GetRunId() || index >= len(suite.Scenarios) || !proto.Equal(attempt.GetScenarioRef(), suite.Scenarios[index].Reference) || run.GetAttemptRefs()[index] != attempt.GetAttemptId() || !validTimestamp(attempt.GetStartedAt()) || !validTimestamp(attempt.GetCompletedAt()) || attempt.GetCompletedAt().AsTime().Before(attempt.GetStartedAt().AsTime()) {
			fail(constants.ErrEvidenceScopeMismatch, run.GetRunId(), fmt.Sprintf("attempt at index %d is malformed or misbound", index))
			continue
		}
		if _, exists := attempts[attempt.GetAttemptId()]; exists {
			fail(constants.ErrEvidenceDuplicateID, attempt.GetAttemptId(), "duplicate attempt ID")
		}
		attempts[attempt.GetAttemptId()] = attempt
	}
	v.verifyRegisteredRecords(report, suite, attempts, fail)
}

func (v *Verifier) verifyRegisteredRecords(report *evalv1.EvaluationReport, suite *SuiteDefinition, attempts map[string]*evalv1.EvaluationAttempt, fail func(error, string, string)) {
	observations := make(map[string]*evalv1.EvaluationObservation)
	for _, observation := range report.GetObservations() {
		if observation == nil || observation.GetObservationId() == "" || observation.GetRunId() != report.GetRun().GetRunId() || attempts[observation.GetAttemptId()] == nil || observation.GetScenarioId() != attempts[observation.GetAttemptId()].GetScenarioRef().GetId() || !validTimestamp(observation.GetObservedAt()) {
			fail(constants.ErrEvidenceScopeMismatch, report.GetRun().GetRunId(), "observation is incomplete or misbound")
			continue
		}
		if _, exists := observations[observation.GetObservationId()]; exists {
			fail(constants.ErrEvidenceDuplicateID, observation.GetObservationId(), "duplicate observation ID")
		}
		observations[observation.GetObservationId()] = observation
	}
	assertions := make(map[string]*evalv1.EvaluationAssertion)
	for _, scenario := range suite.Scenarios {
		for _, assertion := range scenario.Assertions {
			assertions[assertion.GetAssertionId()] = assertion
		}
	}
	if len(report.GetAssertions()) != len(assertions) {
		fail(constants.ErrInvalidEvidenceGraph, report.GetRun().GetRunId(), "assertion set does not match the registered suite")
	}
	for _, assertion := range report.GetAssertions() {
		canonical := assertions[assertion.GetAssertionId()]
		if canonical == nil || !proto.Equal(canonical, assertion) {
			fail(constants.ErrEvidenceScopeMismatch, assertion.GetAssertionId(), "assertion does not match the registered suite")
		}
	}
	verdicts := make(map[string]*evalv1.EvaluationVerdict)
	for _, verdict := range report.GetVerdicts() {
		if verdict == nil || verdict.GetVerdictId() == "" {
			fail(constants.ErrEvidenceArtifactMalformed, report.GetRun().GetRunId(), "verdict is incomplete")
			continue
		}
		if _, exists := verdicts[verdict.GetVerdictId()]; exists {
			fail(constants.ErrEvidenceDuplicateID, verdict.GetVerdictId(), "duplicate verdict ID")
		}
		verdicts[verdict.GetVerdictId()] = verdict
	}
	for _, attempt := range attempts {
		for _, ref := range attempt.GetObservationRefs() {
			if observations[ref] == nil || observations[ref].GetAttemptId() != attempt.GetAttemptId() {
				fail(constants.ErrUnresolvedReference, ref, "attempt observation reference is unresolved or misbound")
			}
		}
		for _, ref := range attempt.GetAssertionRefs() {
			if assertions[ref] == nil {
				fail(constants.ErrUnresolvedReference, ref, "attempt assertion reference is unresolved")
			}
		}
		for _, ref := range attempt.GetVerdictRefs() {
			if verdicts[ref] == nil {
				fail(constants.ErrUnresolvedReference, ref, "attempt verdict reference is unresolved")
			}
		}
	}
}

func (v *Verifier) verifyArtifacts(ctx context.Context, state *verificationState, fail func(error, string, string)) {
	unique := make(map[string]*compliancev1.ComplianceEvidenceReference)
	for _, reference := range state.report.GetEvidenceRefs() {
		if reference != nil {
			if existing := unique[reference.GetArtifactId()]; existing != nil && (existing.GetArtifactType() != reference.GetArtifactType() || existing.GetSha256() != reference.GetSha256() || existing.GetMediaType() != reference.GetMediaType()) {
				fail(constants.ErrEvidenceScopeMismatch, reference.GetArtifactId(), "duplicate evidence reference has conflicting content metadata")
			}
			unique[reference.GetArtifactId()] = reference
		}
	}
	for _, observation := range state.report.GetObservations() {
		for _, reference := range observation.GetEvidenceRefs() {
			if reference == nil {
				continue
			}
			if reference.GetRunId() != state.report.GetRun().GetRunId() || reference.GetScenarioId() != observation.GetScenarioId() || reference.GetAttemptId() != observation.GetAttemptId() {
				fail(constants.ErrEvidenceScopeMismatch, reference.GetArtifactId(), "evidence reference does not match observation scope")
			}
			if existing := unique[reference.GetArtifactId()]; existing != nil && (existing.GetArtifactType() != reference.GetArtifactType() || existing.GetSha256() != reference.GetSha256() || existing.GetMediaType() != reference.GetMediaType()) {
				fail(constants.ErrEvidenceScopeMismatch, reference.GetArtifactId(), "duplicate evidence reference has conflicting content metadata")
			}
			unique[reference.GetArtifactId()] = reference
		}
	}
	for id, reference := range unique {
		artifactType, digest, ok := complianceevidence.ParseContentAddress(id)
		if !ok || string(artifactType) != reference.GetArtifactType() || digest != reference.GetSha256() || reference.GetMediaType() != constants.MediaTypeJSON {
			fail(constants.ErrEvidenceArtifactMalformed, id, "content address and declared metadata disagree")
			continue
		}
		path := filepath.Join(evaluationEvidenceDir(state.report.GetRun().GetRunId()), digest+constants.FileExtJSON)
		body, err := v.reader.ReadFile(ctx, path)
		if err != nil {
			fail(complianceevidence.ClassifyReadError(err), id, err.Error())
			continue
		}
		if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
			fail(constants.ErrEvidenceArtifactMalformed, id, err.Error())
			continue
		}
		if complianceevidence.ContentAddress(artifactType, body) != id {
			fail(constants.ErrChecksumMismatch, id, "artifact body does not match its content address")
			continue
		}
		state.artifacts[id] = verifiedArtifact{body: body}
	}
	if len(unique) != len(state.artifacts) {
		fail(constants.ErrUnresolvedReference, state.report.GetRun().GetRunId(), "one or more declared evidence artifacts could not be resolved")
	}
	v.verifyStoredVerification(ctx, state, fail)
}

func (v *Verifier) verifyStoredVerification(ctx context.Context, state *verificationState, fail func(error, string, string)) {
	reference := state.report.GetRun().GetFinalVerificationReportRef()
	if reference == nil {
		return
	}
	if reference.GetRunId() != state.report.GetRun().GetRunId() || reference.GetArtifactType() != "evaluation-verification" || reference.GetMediaType() != constants.MediaTypeJSON {
		fail(constants.ErrEvidenceScopeMismatch, reference.GetArtifactId(), "stored verification reference is misbound")
		return
	}
	body, err := v.reader.ReadFile(ctx, filepath.Join(evaluationRunDir(state.report.GetRun().GetRunId()), constants.EvaluationVerificationFilename))
	if err != nil {
		fail(complianceevidence.ClassifyReadError(err), reference.GetArtifactId(), err.Error())
		return
	}
	if complianceevidence.ContentReferenceForBody("evaluation-verification", body) != reference.GetArtifactId() {
		fail(constants.ErrChecksumMismatch, reference.GetArtifactId(), "stored verification report does not match its content address")
		return
	}
	stored := &compliancev1.ComplianceVerificationReport{}
	if err := compliancev1.UnmarshalCanonical(body, stored); err != nil || stored.GetReportId() != state.report.GetRun().GetRunId() {
		fail(constants.ErrEvidenceArtifactMalformed, reference.GetArtifactId(), "stored verification report is noncanonical or misbound")
	}
}

func (v *Verifier) verifySemantics(state *verificationState, fail func(error, string, string)) {
	report := state.report
	if len(report.GetAttempts()) != 2 {
		return
	}
	allowed := report.GetAttempts()[0]
	prohibited := report.GetAttempts()[1]
	observationByType := make(map[string]*evalv1.EvaluationObservation)
	for _, observation := range report.GetObservations() {
		key := observation.GetAttemptId() + ":" + observation.GetObservationType().GetId()
		if observationByType[key] != nil {
			fail(constants.ErrEvidenceDuplicateID, key, "duplicate observation type within attempt")
		}
		observationByType[key] = observation
	}
	initialState := v.targetState(observationByType[allowed.GetAttemptId()+":target-marker-count-before-allowed"], state, fail)
	allowedState := v.targetState(observationByType[allowed.GetAttemptId()+":target-marker-count-after-allowed"], state, fail)
	finalState := v.targetState(observationByType[prohibited.GetAttemptId()+":target-marker-count-after-prohibited"], state, fail)
	if initialState != nil && allowedState != nil && finalState != nil {
		initialCount := int64(0)
		if initialState.GetPresent() && len(initialState.GetContent()) > 0 {
			initialCount = int64(bytes.Count(initialState.GetContent(), allowedState.GetContent()))
		}
		v.requireInteger(observationByType[allowed.GetAttemptId()+":target-marker-count-before-allowed"], initialCount, fail)
		v.requireInteger(observationByType[allowed.GetAttemptId()+":target-marker-count-after-allowed"], boolCount(allowedState.GetPresent() && len(allowedState.GetContent()) > 0), fail)
		v.requireInteger(observationByType[prohibited.GetAttemptId()+":target-marker-count-after-prohibited"], boolCount(finalState.GetPresent() && bytes.Equal(finalState.GetContent(), allowedState.GetContent()) && len(finalState.GetContent()) > 0), fail)
	}
	receipt, auditRecords, exchanges, signerKey := v.allowedEvidence(allowed, state, fail)
	if receipt != nil {
		completed := receipt.GetStatus() == operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED
		v.requireBoolean(observationByType[allowed.GetAttemptId()+":target-identity-matches"], true, fail)
		v.requireBoolean(observationByType[allowed.GetAttemptId()+":terminal-receipt-completed"], completed, fail)
		v.requireBoolean(observationByType[allowed.GetAttemptId()+":receipt-durable"], receipt.GetFinalPersistenceAttestation() != nil, fail)
		chainValid := complianceevidence.VerifyReceiptEvidenceSignatures(receipt, signerKey) == nil
		v.requireBoolean(observationByType[allowed.GetAttemptId()+":protocol-chain-valid"], chainValid, fail)
		if !chainValid {
			fail(constants.ErrInvalidEvidenceGraph, receipt.GetTransactionId(), "receipt or persistence signature is invalid")
		}
		v.verifyReceiptBinding(report, allowed, receipt, auditRecords, fail)
	}
	prohibitedRecords, prohibitedExchanges := v.prohibitedEvidence(prohibited, state, fail)
	rejected, attributed := rejectionOutcome(prohibitedExchanges, prohibited.GetAttemptId())
	count, err := countCompletedAttemptReceipts(prohibitedRecords, prohibited.GetAttemptId())
	if err != nil {
		fail(constants.ErrInvalidEvidenceGraph, prohibited.GetAttemptId(), err.Error())
	}
	v.requireBoolean(observationByType[prohibited.GetAttemptId()+":gateway-request-rejected"], rejected, fail)
	v.requireBoolean(observationByType[prohibited.GetAttemptId()+":gateway-l1-rejection-attributed"], attributed, fail)
	v.requireInteger(observationByType[prohibited.GetAttemptId()+":completed-execution-count"], count, fail)
	_ = exchanges
	v.regrade(report, fail)
}

func (v *Verifier) targetState(observation *evalv1.EvaluationObservation, state *verificationState, fail func(error, string, string)) *evalv1.EvaluationTargetState {
	if observation == nil || len(observation.GetEvidenceRefs()) != 1 {
		fail(constants.ErrInvalidEvidenceGraph, state.report.GetRun().GetRunId(), "target observation must carry exactly one evidence reference")
		return nil
	}
	artifact := state.artifacts[observation.GetEvidenceRefs()[0].GetArtifactId()]
	target := &evalv1.EvaluationTargetState{}
	if err := evalv1.UnmarshalCanonical(artifact.body, target); err != nil {
		fail(constants.ErrEvidenceArtifactMalformed, observation.GetObservationId(), err.Error())
		return nil
	}
	if target.GetSchemaVersion() != RegistryVersion || target.GetRunId() != observation.GetRunId() || target.GetScenarioId() != observation.GetScenarioId() || target.GetAttemptId() != observation.GetAttemptId() || target.GetTargetResource() != state.report.GetRun().GetDeployment().GetControlledTarget() || !validTimestamp(target.GetObservedAt()) || !target.GetObservedAt().AsTime().Equal(observation.GetObservedAt().AsTime()) || (!target.GetPresent() && len(target.GetContent()) != 0) {
		fail(constants.ErrEvidenceScopeMismatch, observation.GetObservationId(), "target-state body is incomplete or misbound")
		return nil
	}
	return target
}

func (v *Verifier) allowedEvidence(attempt *evalv1.EvaluationAttempt, state *verificationState, fail func(error, string, string)) (*operatorv1.ActionReceipt, []*models.ActionReceiptRecord, []client.Exchange, ed25519.PublicKey) {
	var receipt *operatorv1.ActionReceipt
	var records []*models.ActionReceiptRecord
	var exchanges []client.Exchange
	var key ed25519.PublicKey
	for _, reference := range attemptEvidenceReferences(state.report, attempt.GetAttemptId()) {
		artifact := state.artifacts[reference.GetArtifactId()]
		switch reference.GetArtifactType() {
		case string(complianceevidence.ArtifactTypeActionReceipt):
			receipt = &operatorv1.ActionReceipt{}
			if err := strictProtoJSON(artifact.body, receipt); err != nil {
				fail(constants.ErrEvidenceArtifactMalformed, reference.GetArtifactId(), err.Error())
				receipt = nil
			}
		case string(complianceevidence.ArtifactTypeAuditRecord):
			response := &models.AuditReceiptsResponse{}
			if err := json.Unmarshal(artifact.body, response); err != nil || !response.Success {
				fail(constants.ErrEvidenceArtifactMalformed, reference.GetArtifactId(), "audit mirror response is invalid")
			} else {
				records = response.Receipts
			}
		case string(complianceevidence.ArtifactTypeEvalExchange):
			if err := json.Unmarshal(artifact.body, &exchanges); err != nil {
				fail(constants.ErrEvidenceArtifactMalformed, reference.GetArtifactId(), err.Error())
			}
		}
	}
	if receipt == nil || len(records) == 0 || len(exchanges) == 0 {
		fail(constants.ErrUnresolvedReference, attempt.GetAttemptId(), "allowed attempt receipt, audit mirror, or exchange evidence is missing")
		return receipt, records, exchanges, key
	}
	key = signerKeyFromExchanges(exchanges, receipt.GetSignerKeyId())
	if len(key) != ed25519.PublicKeySize {
		fail(constants.ErrEvidenceTrustNotAssessed, receipt.GetSignerKeyId(), "trusted signer response is missing from the captured exchange")
	}
	return receipt, records, exchanges, key
}

func (v *Verifier) verifyReceiptBinding(report *evalv1.EvaluationReport, attempt *evalv1.EvaluationAttempt, receipt *operatorv1.ActionReceipt, records []*models.ActionReceiptRecord, fail func(error, string, string)) {
	if attempt.GetExecutionId() != attempt.GetAttemptId() {
		fail(constants.ErrEvidenceScopeMismatch, attempt.GetAttemptId(), "attempt execution ID does not match attempt ID")
		return
	}
	record, err := exactReceiptRecord(records, attempt.GetTransactionId())
	if err != nil {
		fail(constants.ErrEvaluationReceiptUnavailable, attempt.GetAttemptId(), err.Error())
		return
	}
	binding := complianceevidence.ReceiptBinding{RunID: report.GetRun().GetRunId(), ScenarioID: attempt.GetScenarioRef().GetId(), AttemptID: attempt.GetAttemptId(), ExecutionID: attempt.GetExecutionId(), InvestigationID: attempt.GetScenarioRef().GetId(), TransactionID: attempt.GetTransactionId(), TargetOperatorID: report.GetRun().GetTargetOperatorId(), TargetOperatorSessionID: report.GetRun().GetTargetOperatorSessionId(), ActionType: string(constants.ActionTypeFileEdit)}
	projection := complianceevidence.ReceiptProjection{ExecutionID: attempt.GetExecutionId(), TransactionID: record.TransactionID, TransactionHash: record.TransactionHash, InvestigationID: record.InvestigationID, OperatorID: record.OperatorID, OperatorSessionID: record.OperatorSessionID, ActionType: string(record.ActionType), SignerKeyID: record.SignerKeyID, Signature: record.Signature}
	if _, err := complianceevidence.BuildVerifiedReceiptEvidence(binding, projection, receipt); err != nil {
		fail(constants.ErrEvidenceScopeMismatch, attempt.GetAttemptId(), err.Error())
	}
}

func (v *Verifier) prohibitedEvidence(attempt *evalv1.EvaluationAttempt, state *verificationState, fail func(error, string, string)) ([]*models.ActionReceiptRecord, []client.Exchange) {
	var records []*models.ActionReceiptRecord
	var exchanges []client.Exchange
	for _, reference := range attemptEvidenceReferences(state.report, attempt.GetAttemptId()) {
		artifact := state.artifacts[reference.GetArtifactId()]
		switch reference.GetArtifactType() {
		case string(complianceevidence.ArtifactTypeAuditRecord):
			response := &models.AuditReceiptsResponse{}
			if err := json.Unmarshal(artifact.body, response); err != nil || !response.Success {
				fail(constants.ErrEvidenceArtifactMalformed, reference.GetArtifactId(), "prohibited audit response is invalid")
			} else {
				records = response.Receipts
			}
		case string(complianceevidence.ArtifactTypeEvalExchange):
			if err := json.Unmarshal(artifact.body, &exchanges); err != nil {
				fail(constants.ErrEvidenceArtifactMalformed, reference.GetArtifactId(), err.Error())
			}
		}
	}
	if records == nil || len(exchanges) == 0 {
		fail(constants.ErrUnresolvedReference, attempt.GetAttemptId(), "prohibited audit or exchange evidence is missing")
	}
	return records, exchanges
}

func (v *Verifier) regrade(report *evalv1.EvaluationReport, fail func(error, string, string)) {
	observations := make(map[string][]*evalv1.EvaluationObservation)
	for _, observation := range report.GetObservations() {
		observations[observation.GetAttemptId()] = append(observations[observation.GetAttemptId()], observation)
	}
	stored := make(map[string]*evalv1.EvaluationVerdict)
	for _, verdict := range report.GetVerdicts() {
		stored[verdict.GetVerdictId()] = verdict
	}
	grader := NewGrader(func() time.Time { return v.now().UTC() })
	recomputed := make([]*evalv1.EvaluationVerdict, 0)
	for _, attempt := range report.GetAttempts() {
		for _, assertionRef := range attempt.GetAssertionRefs() {
			var assertion *evalv1.EvaluationAssertion
			for _, candidate := range report.GetAssertions() {
				if candidate.GetAssertionId() == assertionRef {
					assertion = candidate
					break
				}
			}
			if assertion == nil {
				continue
			}
			actual := grader.Grade(attempt.GetAttemptId(), assertion, observations[attempt.GetAttemptId()])
			recomputed = append(recomputed, actual)
			expected := stored[actual.GetVerdictId()]
			if expected == nil || expected.GetStatus() != actual.GetStatus() || expected.GetFailureReason() != actual.GetFailureReason() || !proto.Equal(expected.GetAssertionRef(), actual.GetAssertionRef()) || !proto.Equal(expected.GetGraderRef(), actual.GetGraderRef()) || !equalStrings(expected.GetObservedRefs(), actual.GetObservedRefs()) || !equalReferences(expected.GetEvidenceRefs(), actual.GetEvidenceRefs()) {
				fail(constants.ErrInvalidEvidenceGraph, actual.GetVerdictId(), "stored verdict does not match deterministic recomputation")
			}
		}
	}
	metric := DeriveRequiredVerdictMetric(recomputed)
	if len(report.GetMetrics()) != 1 || !proto.Equal(report.GetMetrics()[0], metric) {
		fail(constants.ErrInvalidEvidenceGraph, report.GetRun().GetRunId(), "stored metric does not match deterministic recomputation")
	}
	if report.GetSummaryStatus() != SummaryStatus(recomputed) || report.GetRequiredVerdictCount() != uint32(len(recomputed)) || report.GetPassedVerdictCount() != uint32(metric.GetNumerator()) || report.GetSummary() != VerdictSummary(recomputed) {
		fail(constants.ErrInvalidEvidenceGraph, report.GetRun().GetRunId(), "stored summary does not match deterministic recomputation")
	}
}

func (v *Verifier) verifyDirectory(ctx context.Context, runID string, state *verificationState, fail func(error, string, string)) {
	runDir := evaluationRunDir(runID)
	entries, err := v.reader.ReadDir(ctx, runDir)
	if err != nil {
		fail(complianceevidence.ClassifyReadError(err), runDir, err.Error())
		return
	}
	for _, entry := range entries {
		if entry.Name() != constants.EvaluationReportFilename && entry.Name() != constants.EvaluationVerificationFilename && entry.Name() != constants.EvaluationEvidenceDirname {
			fail(constants.ErrUnexpectedEvidenceArtifact, entry.Name(), "undeclared file in evaluation run directory")
		}
	}
	evidenceEntries, err := v.reader.ReadDir(ctx, evaluationEvidenceDir(runID))
	if err != nil {
		fail(complianceevidence.ClassifyReadError(err), evaluationEvidenceDir(runID), err.Error())
		return
	}
	declared := make(map[string]bool)
	for artifactID := range state.artifacts {
		_, digest, _ := complianceevidence.ParseContentAddress(artifactID)
		declared[digest+constants.FileExtJSON] = true
	}
	for _, entry := range evidenceEntries {
		if entry.IsDir() || !declared[entry.Name()] {
			fail(constants.ErrUnexpectedEvidenceArtifact, entry.Name(), "undeclared artifact in evaluation evidence directory")
		}
	}
	if len(evidenceEntries) != len(declared) {
		fail(constants.ErrUnresolvedReference, runID, "declared evidence file count does not match directory contents")
	}
}

func (v *Verifier) requireBoolean(observation *evalv1.EvaluationObservation, expected bool, fail func(error, string, string)) {
	if observation == nil {
		fail(constants.ErrInvalidEvidenceGraph, "observation", "required boolean observation is missing")
		return
	}
	value, ok := observation.GetValue().GetValue().(*evalv1.EvaluationValue_BooleanValue)
	if !ok || value.BooleanValue != expected {
		fail(constants.ErrInvalidEvidenceGraph, observation.GetObservationId(), "stored boolean observation does not match evidence")
	}
}

func (v *Verifier) requireInteger(observation *evalv1.EvaluationObservation, expected int64, fail func(error, string, string)) {
	if observation == nil {
		fail(constants.ErrInvalidEvidenceGraph, "observation", "required integer observation is missing")
		return
	}
	value, ok := observation.GetValue().GetValue().(*evalv1.EvaluationValue_IntegerValue)
	if !ok || value.IntegerValue != expected {
		fail(constants.ErrInvalidEvidenceGraph, observation.GetObservationId(), "stored integer observation does not match evidence")
	}
}

func strictProtoJSON(body []byte, message proto.Message) error {
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return err
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(body, message); err != nil {
		return err
	}
	canonical, err := complianceevidence.MarshalCanonicalProto(message)
	if err != nil {
		return err
	}
	if !bytes.Equal(body, canonical) {
		return constants.ErrEvidenceArtifactMalformed
	}
	return nil
}

func signerKeyFromExchanges(exchanges []client.Exchange, signerID string) ed25519.PublicKey {
	for _, exchange := range exchanges {
		if exchange.Status < 200 || exchange.Status >= 300 || !strings.Contains(exchange.URL, constants.APIPaths.GovernanceSignersByID) {
			continue
		}
		var response struct {
			ID        string `json:"id"`
			PublicKey string `json:"public_key_hex"`
			Enabled   bool   `json:"enabled"`
		}
		if json.Unmarshal(exchange.RespBody, &response) != nil || response.ID != signerID || !response.Enabled {
			continue
		}
		decoded, err := hex.DecodeString(response.PublicKey)
		if err == nil && len(decoded) == ed25519.PublicKeySize {
			return ed25519.PublicKey(decoded)
		}
	}
	return nil
}

func rejectionOutcome(exchanges []client.Exchange, attemptID string) (bool, bool) {
	for _, exchange := range exchanges {
		if exchange.Method != "POST" || !strings.Contains(exchange.URL, constants.APIPaths.OperatorsCommands) {
			continue
		}
		request := &client.DispatchCommandRequest{}
		if json.Unmarshal(exchange.ReqBody, request) != nil || request.TaskID != attemptID {
			continue
		}
		rejected := exchange.Status >= 400
		return rejected, rejected && bytes.Contains(exchange.RespBody, []byte(constants.ErrTxL1ValidationFailed.Error()))
	}
	return false, false
}

func attemptEvidenceReferences(report *evalv1.EvaluationReport, attemptID string) []*compliancev1.ComplianceEvidenceReference {
	unique := make(map[string]*compliancev1.ComplianceEvidenceReference)
	for _, observation := range report.GetObservations() {
		if observation.GetAttemptId() != attemptID {
			continue
		}
		for _, reference := range observation.GetEvidenceRefs() {
			if reference != nil {
				unique[reference.GetArtifactId()] = reference
			}
		}
	}
	result := make([]*compliancev1.ComplianceEvidenceReference, 0, len(unique))
	for _, reference := range unique {
		result = append(result, reference)
	}
	return result
}

func validTimestamp(timestamp *timestamppb.Timestamp) bool {
	return timestamp != nil && timestamp.CheckValid() == nil
}

func boolCount(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalReferences(left, right []*compliancev1.ComplianceEvidenceReference) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !proto.Equal(left[index], right[index]) {
			return false
		}
	}
	return true
}
