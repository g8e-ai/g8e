// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// --- In-memory artifact reader for verifier tests ---

type verifierArtifactReader struct {
	files map[string][]byte
}

func (r *verifierArtifactReader) ReadFile(_ context.Context, path string) ([]byte, error) {
	body, ok := r.files[path]
	if !ok {
		return nil, constants.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func (r *verifierArtifactReader) ReadDir(_ context.Context, path string) ([]os.DirEntry, error) {
	prefix := path + string(os.PathSeparator)
	names := make(map[string]bool)
	for candidate := range r.files {
		if !strings.HasPrefix(candidate, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(candidate, prefix)
		parts := strings.SplitN(remainder, string(os.PathSeparator), 2)
		names[parts[0]] = len(parts) == 2
	}
	if len(names) == 0 {
		return nil, constants.ErrNotFound
	}
	entries := make([]os.DirEntry, 0, len(names))
	for name, directory := range names {
		entries = append(entries, verifierDirEntry{name: name, directory: directory})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

type verifierDirEntry struct {
	name      string
	directory bool
}

func (e verifierDirEntry) Name() string               { return e.name }
func (e verifierDirEntry) IsDir() bool                { return e.directory }
func (e verifierDirEntry) Type() os.FileMode          { return 0 }
func (e verifierDirEntry) Info() (os.FileInfo, error) { return nil, nil }

// --- Fixture data ---

const (
	vRunID              = "run-1"
	vAllowedAttemptID   = "allowed-attempt-1"
	vProhibitedAttemptID = "prohibited-attempt-2"
	vTransactionID      = "tx-1"
	vOperatorID         = "operator-1"
	vSessionID          = "session-1"
	vTargetResource     = "/tmp/g8e-eval-run-1.txt"
	vMarker             = "run-1"
)

// verifierFixture holds all the pieces needed to build a complete valid
// evaluation run in the in-memory reader.
type verifierFixture struct {
	reader      *verifierArtifactReader
	now         func() time.Time
	publicKey   ed25519.PublicKey
	privateKey  ed25519.PrivateKey
	signerKeyID string
	receipt     *operatorv1.ActionReceipt
	// artifact bodies keyed by artifact ID
	bodies map[string][]byte
	// evidence references
	targetState1Ref     *compliancev1.ComplianceEvidenceReference
	targetState2Ref     *compliancev1.ComplianceEvidenceReference
	targetState3Ref     *compliancev1.ComplianceEvidenceReference
	receiptRef          *compliancev1.ComplianceEvidenceReference
	persistenceRef      *compliancev1.ComplianceEvidenceReference
	allowedAuditRef     *compliancev1.ComplianceEvidenceReference
	allowedExchangeRef  *compliancev1.ComplianceEvidenceReference
	prohibitedAuditRef  *compliancev1.ComplianceEvidenceReference
	prohibitedExchangeRef *compliancev1.ComplianceEvidenceReference
	verificationRef     *compliancev1.ComplianceEvidenceReference
	verificationBody    []byte
}

func buildValidVerifierFixture(t *testing.T) *verifierFixture {
	t.Helper()
	now := func() time.Time { return time.Unix(1_700_000_200, 0).UTC() }
	fix := &verifierFixture{
		reader:  &verifierArtifactReader{files: map[string][]byte{}},
		now:     now,
		bodies:  map[string][]byte{},
	}

	// Generate signing key pair
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	fix.publicKey = publicKey
	fix.privateKey = privateKey
	fix.signerKeyID = hex.EncodeToString(publicKey)

	// Build the signed receipt
	fix.receipt = buildVerifierSignedReceipt(t, fix.privateKey, fix.signerKeyID)

	// Marshal receipt and persistence artifacts
	receiptBody, err := complianceevidence.MarshalCanonicalProto(fix.receipt)
	require.NoError(t, err)
	receiptID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeActionReceipt, receiptBody)
	fix.bodies[receiptID] = receiptBody
	fix.receiptRef = verifierEvidenceRef(receiptID, complianceevidence.ArtifactTypeActionReceipt, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, vTransactionID)

	persistenceBody, err := complianceevidence.MarshalCanonicalProto(fix.receipt.GetFinalPersistenceAttestation())
	require.NoError(t, err)
	persistenceID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeReceiptPersistence, persistenceBody)
	fix.bodies[persistenceID] = persistenceBody
	fix.persistenceRef = verifierEvidenceRef(persistenceID, complianceevidence.ArtifactTypeReceiptPersistence, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, vTransactionID)

	// Build allowed audit record
	record := &models.ActionReceiptRecord{
		TransactionID:       fix.receipt.TransactionId,
		TransactionHash:     fix.receipt.TransactionHash,
		InvestigationID:     AllowedExecutionScenarioID,
		OperatorID:          vOperatorID,
		OperatorSessionID:   vSessionID,
		ActionType:          constants.ActionTypeFileEdit,
		Status:              operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		SignerKeyID:         fix.receipt.SignerKeyId,
		Signature:           fix.receipt.Signature,
		ActionReceipt:       fix.receipt,
	}
	allowedAuditResponse := &models.AuditReceiptsResponse{Success: true, Receipts: []*models.ActionReceiptRecord{record}}
	allowedAuditBody, err := json.Marshal(allowedAuditResponse)
	require.NoError(t, err)
	allowedAuditID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeAuditRecord, allowedAuditBody)
	fix.bodies[allowedAuditID] = allowedAuditBody
	fix.allowedAuditRef = verifierEvidenceRef(allowedAuditID, complianceevidence.ArtifactTypeAuditRecord, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, vTransactionID)

	// Build allowed exchanges (dispatch + signer lookup)
	dispatchReq := client.DispatchCommandRequest{
		TargetOperatorSessionID: vSessionID,
		ActionType:              string(constants.ActionTypeFileEdit),
		CaseID:                  vRunID,
		InvestigationID:         AllowedExecutionScenarioID,
		TaskID:                  vAllowedAttemptID,
		CLISessionID:            "cli-session-1",
	}
	dispatchReqBody, err := json.Marshal(dispatchReq)
	require.NoError(t, err)
	dispatchRespBody, err := json.Marshal(client.DispatchCommandResponse{Success: true, TransactionID: fix.receipt.TransactionId})
	require.NoError(t, err)
	signerRespBody, err := json.Marshal(map[string]any{
		"id":            fix.signerKeyID,
		"public_key_hex": fix.signerKeyID,
		"enabled":       true,
	})
	require.NoError(t, err)
	allowedExchanges := []client.Exchange{
		{Method: "POST", URL: "https://localhost:8443" + constants.APIPaths.OperatorsCommands, ReqBody: dispatchReqBody, Status: 200, RespBody: dispatchRespBody},
		{Method: "GET", URL: "https://localhost:8443" + constants.APIPaths.GovernanceSignersByID + fix.signerKeyID, Status: 200, RespBody: signerRespBody},
	}
	allowedExchangeBody, err := json.Marshal(allowedExchanges)
	require.NoError(t, err)
	allowedExchangeID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalExchange, allowedExchangeBody)
	fix.bodies[allowedExchangeID] = allowedExchangeBody
	fix.allowedExchangeRef = verifierEvidenceRef(allowedExchangeID, complianceevidence.ArtifactTypeEvalExchange, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, "")

	// Build prohibited audit (empty receipts)
	prohibitedAuditResponse := &models.AuditReceiptsResponse{Success: true, Receipts: []*models.ActionReceiptRecord{}}
	prohibitedAuditBody, err := json.Marshal(prohibitedAuditResponse)
	require.NoError(t, err)
	prohibitedAuditID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeAuditRecord, prohibitedAuditBody)
	fix.bodies[prohibitedAuditID] = prohibitedAuditBody
	fix.prohibitedAuditRef = verifierEvidenceRef(prohibitedAuditID, complianceevidence.ArtifactTypeAuditRecord, vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, "")

	// Build prohibited exchanges (dispatch with L1 rejection)
	prohibDispatchReq := client.DispatchCommandRequest{
		TargetOperatorSessionID: vSessionID,
		ActionType:              string(constants.ActionTypeFileEdit),
		CaseID:                  vRunID,
		InvestigationID:         ProhibitedExecutionScenarioID,
		TaskID:                  vProhibitedAttemptID,
		CLISessionID:            "cli-session-1",
	}
	prohibDispatchReqBody, err := json.Marshal(prohibDispatchReq)
	require.NoError(t, err)
	prohibDispatchRespBody := []byte(`{"error":"` + constants.ErrTxL1ValidationFailed.Error() + `"}`)
	prohibitedExchanges := []client.Exchange{
		{Method: "POST", URL: "https://localhost:8443" + constants.APIPaths.OperatorsCommands, ReqBody: prohibDispatchReqBody, Status: 400, RespBody: prohibDispatchRespBody},
	}
	prohibitedExchangeBody, err := json.Marshal(prohibitedExchanges)
	require.NoError(t, err)
	prohibitedExchangeID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalExchange, prohibitedExchangeBody)
	fix.bodies[prohibitedExchangeID] = prohibitedExchangeBody
	fix.prohibitedExchangeRef = verifierEvidenceRef(prohibitedExchangeID, complianceevidence.ArtifactTypeEvalExchange, vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, "")

	// Build target states
	observedAt1 := time.Unix(1_700_000_010, 0).UTC()
	targetState1 := &evalv1.EvaluationTargetState{
		SchemaVersion: RegistryVersion, RunId: vRunID, ScenarioId: AllowedExecutionScenarioID, AttemptId: vAllowedAttemptID,
		TargetResource: vTargetResource, ObservedAt: timestamppb.New(observedAt1), Present: false, Content: nil,
	}
	targetState1Body, err := complianceevidence.MarshalCanonicalProto(targetState1)
	require.NoError(t, err)
	targetState1ID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalObservation, targetState1Body)
	fix.bodies[targetState1ID] = targetState1Body
	fix.targetState1Ref = verifierEvidenceRef(targetState1ID, complianceevidence.ArtifactTypeEvalObservation, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, "")

	observedAt2 := time.Unix(1_700_000_020, 0).UTC()
	targetState2 := &evalv1.EvaluationTargetState{
		SchemaVersion: RegistryVersion, RunId: vRunID, ScenarioId: AllowedExecutionScenarioID, AttemptId: vAllowedAttemptID,
		TargetResource: vTargetResource, ObservedAt: timestamppb.New(observedAt2), Present: true, Content: []byte(vMarker + "\n"),
	}
	targetState2Body, err := complianceevidence.MarshalCanonicalProto(targetState2)
	require.NoError(t, err)
	targetState2ID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalObservation, targetState2Body)
	fix.bodies[targetState2ID] = targetState2Body
	fix.targetState2Ref = verifierEvidenceRef(targetState2ID, complianceevidence.ArtifactTypeEvalObservation, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, "")

	observedAt3 := time.Unix(1_700_000_030, 0).UTC()
	targetState3 := &evalv1.EvaluationTargetState{
		SchemaVersion: RegistryVersion, RunId: vRunID, ScenarioId: ProhibitedExecutionScenarioID, AttemptId: vProhibitedAttemptID,
		TargetResource: vTargetResource, ObservedAt: timestamppb.New(observedAt3), Present: true, Content: []byte(vMarker + "\n"),
	}
	targetState3Body, err := complianceevidence.MarshalCanonicalProto(targetState3)
	require.NoError(t, err)
	targetState3ID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalObservation, targetState3Body)
	fix.bodies[targetState3ID] = targetState3Body
	fix.targetState3Ref = verifierEvidenceRef(targetState3ID, complianceevidence.ArtifactTypeEvalObservation, vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, "")

	// Build the report
	report := fix.buildReport(t)

	// Marshal and write report
	reportBody, err := evalv1.MarshalCanonical(report)
	require.NoError(t, err)
	fix.reader.files[filepath.Join(evaluationRunDir(vRunID), constants.EvaluationReportFilename)] = reportBody

	// Write evidence artifacts
	for artifactID, body := range fix.bodies {
		_, digest, _ := complianceevidence.ParseContentAddress(artifactID)
		fix.reader.files[filepath.Join(evaluationEvidenceDir(vRunID), digest+constants.FileExtJSON)] = body
	}

	// Build verification report by running the verifier
	verifier := NewVerifier(fix.reader, NewRegistry(), fix.now)
	verification, err := verifier.Verify(context.Background(), vRunID)
	require.NoError(t, err)
	require.True(t, verification.GetValid(), "fixture must verify as valid, got failures: %v", verification.GetFailures())

	// Marshal and write verification report
	verificationBody, err := compliancev1.MarshalCanonical(verification)
	require.NoError(t, err)
	fix.verificationBody = verificationBody
	fix.reader.files[filepath.Join(evaluationRunDir(vRunID), constants.EvaluationVerificationFilename)] = verificationBody
	verificationID := complianceevidence.ContentReferenceForBody("evaluation-verification", verificationBody)
	_, verificationDigest, _ := complianceevidence.ParseContentReference(verificationID)
	fix.verificationRef = &compliancev1.ComplianceEvidenceReference{
		ArtifactId: verificationID, ArtifactType: "evaluation-verification", Sha256: verificationDigest, MediaType: constants.MediaTypeJSON, RunId: vRunID,
	}

	// Update report with FinalVerificationReportRef and re-write
	report.Run.FinalVerificationReportRef = fix.verificationRef
	reportBody, err = evalv1.MarshalCanonical(report)
	require.NoError(t, err)
	fix.reader.files[filepath.Join(evaluationRunDir(vRunID), constants.EvaluationReportFilename)] = reportBody

	// Verify the updated fixture passes
	verification2, err := NewVerifier(fix.reader, NewRegistry(), fix.now).Verify(context.Background(), vRunID)
	require.NoError(t, err)
	require.True(t, verification2.GetValid(), "updated fixture must verify as valid, got failures: %v", verification2.GetFailures())

	return fix
}

func (f *verifierFixture) buildReport(t *testing.T) *evalv1.EvaluationReport {
	t.Helper()
	suite, err := NewRegistry().Lookup(CoreExecutionBoundarySuiteID, CoreExecutionBoundarySuiteVersion)
	require.NoError(t, err)

	startedAt := time.Unix(1_700_000_000, 0).UTC()
	completedAt := time.Unix(1_700_000_100, 0).UTC()

	allowedEvidenceRefs := []*compliancev1.ComplianceEvidenceReference{f.receiptRef, f.persistenceRef, f.allowedAuditRef, f.allowedExchangeRef}
	prohibitedEvidenceRefs := []*compliancev1.ComplianceEvidenceReference{f.prohibitedAuditRef, f.prohibitedExchangeRef}

	observedAt1 := time.Unix(1_700_000_010, 0).UTC()
	observedAt2 := time.Unix(1_700_000_020, 0).UTC()
	observedAt3 := time.Unix(1_700_000_030, 0).UTC()
	now := f.now()

	observations := []*evalv1.EvaluationObservation{
		verifierIntObs("obs-1", "target-marker-count-before-allowed", vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, 0, f.targetState1Ref, observedAt1),
		verifierIntObs("obs-2", "target-marker-count-after-allowed", vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, 1, f.targetState2Ref, observedAt2),
		verifierBoolObs("obs-3", "target-identity-matches", vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, true, allowedEvidenceRefs, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT, now),
		verifierBoolObs("obs-4", "terminal-receipt-completed", vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, true, allowedEvidenceRefs, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT, now),
		verifierBoolObs("obs-5", "receipt-durable", vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, true, allowedEvidenceRefs, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT, now),
		verifierBoolObs("obs-6", "protocol-chain-valid", vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, true, allowedEvidenceRefs, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT, now),
		verifierBoolObs("obs-7", "gateway-request-rejected", vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, true, prohibitedEvidenceRefs, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_GATEWAY_COORDINATION, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_GATEWAY_ADMISSION, now),
		verifierIntObs("obs-8", "target-marker-count-after-prohibited", vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, 1, f.targetState3Ref, observedAt3),
		verifierIntOutcomeObs("obs-9", "completed-execution-count", vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, 0, prohibitedEvidenceRefs, now),
		verifierBoolObs("obs-10", "gateway-l1-rejection-attributed", vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, true, prohibitedEvidenceRefs, evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_GATEWAY_COORDINATION, evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_GATEWAY_ADMISSION, now),
	}

	allowedAttempt := &evalv1.EvaluationAttempt{
		AttemptId: vAllowedAttemptID, RunId: vRunID, ScenarioRef: versioned(AllowedExecutionScenarioID, CoreExecutionBoundarySuiteVersion),
		Status: evalv1.EvaluationAttemptStatus_EVALUATION_ATTEMPT_STATUS_COMPLETED,
		StartedAt: timestamppb.New(startedAt), CompletedAt: timestamppb.New(completedAt),
		TransactionId: f.receipt.TransactionId, ExecutionId: vAllowedAttemptID,
	}
	prohibitedAttempt := &evalv1.EvaluationAttempt{
		AttemptId: vProhibitedAttemptID, RunId: vRunID, ScenarioRef: versioned(ProhibitedExecutionScenarioID, CoreExecutionBoundarySuiteVersion),
		Status: evalv1.EvaluationAttemptStatus_EVALUATION_ATTEMPT_STATUS_REJECTED,
		StartedAt: timestamppb.New(startedAt), CompletedAt: timestamppb.New(completedAt),
	}

	// Build assertions, verdicts, and attempt refs using the grader
	grader := NewGrader(f.now)
	allAssertions := []*evalv1.EvaluationAssertion{}
	allVerdicts := []*evalv1.EvaluationVerdict{}

	appendAttemptAndGrade := func(attempt *evalv1.EvaluationAttempt, scenario ScenarioDefinition, attemptObs []*evalv1.EvaluationObservation) {
		report := &evalv1.EvaluationReport{Attempts: []*evalv1.EvaluationAttempt{attempt}, Observations: attemptObs}
		_ = report
		for _, obs := range attemptObs {
			attempt.ObservationRefs = append(attempt.ObservationRefs, obs.GetObservationId())
		}
		for _, assertion := range scenario.Assertions {
			assertionCopy := proto.Clone(assertion).(*evalv1.EvaluationAssertion)
			allAssertions = append(allAssertions, assertionCopy)
			attempt.AssertionRefs = append(attempt.AssertionRefs, assertionCopy.GetAssertionId())
			verdict := grader.Grade(attempt.GetAttemptId(), assertionCopy, attemptObs)
			allVerdicts = append(allVerdicts, verdict)
			attempt.VerdictRefs = append(attempt.VerdictRefs, verdict.GetVerdictId())
		}
	}

	allowedObs := observations[:6]
	prohibitedObs := observations[6:]
	appendAttemptAndGrade(allowedAttempt, suite.Scenarios[0], allowedObs)
	appendAttemptAndGrade(prohibitedAttempt, suite.Scenarios[1], prohibitedObs)

	// Build the report
	report := &evalv1.EvaluationReport{
		SchemaVersion: RegistryVersion,
		Run: &evalv1.EvaluationRun{
			SchemaVersion: RegistryVersion, RunId: vRunID,
			SuiteRef: versioned(CoreExecutionBoundarySuiteID, CoreExecutionBoundarySuiteVersion),
			Deployment: &evalv1.EvaluationDeploymentIdentity{
				DeploymentId: vRunID,
				TopologyRef:  versioned(TopologyID, TopologyVersion),
				ControlledTarget: vTargetResource,
				IndependentObserver: constants.DockerEvaluationObserverService,
				RuntimeBoundaries: []*evalv1.EvaluationRuntimeBoundary{
					{Component: evalv1.EvaluationRuntimeComponent_EVALUATION_RUNTIME_COMPONENT_EVALUATOR, ProcessIdentity: "host-side g8e eval process", RuntimeNamespace: "Docker host workspace", Endpoint: "https://localhost:8443", AuthenticatedIdentity: "user-1"},
					{Component: evalv1.EvaluationRuntimeComponent_EVALUATION_RUNTIME_COMPONENT_GATEWAY, ProcessIdentity: constants.DockerGatewayContainer, RuntimeNamespace: "Gateway container", PersistentStore: "Gateway runtime volume", Endpoint: "https://localhost:8443", AuthenticatedIdentity: "cli-session-1"},
					{Component: evalv1.EvaluationRuntimeComponent_EVALUATION_RUNTIME_COMPONENT_OPERATOR, ProcessIdentity: constants.DockerOperatorContainer, RuntimeNamespace: "Operator container", MountedFilesystems: []string{"Operator runtime volume", "shared controlled fixture volume"}, PersistentStore: "Operator runtime volume"},
					{Component: evalv1.EvaluationRuntimeComponent_EVALUATION_RUNTIME_COMPONENT_CONTROLLED_TARGET, ProcessIdentity: vTargetResource, RuntimeNamespace: "shared controlled fixture volume", MountedFilesystems: []string{"shared controlled fixture volume"}},
				},
			},
			ActivePosture: evalv1.EvaluationGovernancePosture_EVALUATION_GOVERNANCE_POSTURE_DOCTRINE,
			Lane:          evalv1.EvaluationLane_EVALUATION_LANE_PLATFORM,
			TargetOperatorId: vOperatorID, TargetOperatorSessionId: vSessionID,
			StartedAt: timestamppb.New(startedAt), CompletedAt: timestamppb.New(completedAt),
			AttemptRefs: []string{vAllowedAttemptID, vProhibitedAttemptID},
		},
		Attempts:    []*evalv1.EvaluationAttempt{allowedAttempt, prohibitedAttempt},
		Observations: observations,
		Assertions:   allAssertions,
		Verdicts:     allVerdicts,
	}
	report.SummaryStatus = SummaryStatus(allVerdicts)
	report.RequiredVerdictCount = uint32(len(allVerdicts))
	for _, verdict := range allVerdicts {
		if verdict.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
			report.PassedVerdictCount++
		}
	}
	report.Metrics = []*evalv1.EvaluationMetric{DeriveRequiredVerdictMetric(allVerdicts)}
	report.Summary = VerdictSummary(allVerdicts)
	for _, observation := range observations {
		for _, reference := range observation.GetEvidenceRefs() {
			report.EvidenceRefs = appendUniqueReference(report.EvidenceRefs, reference)
		}
	}
	return report
}

// --- Helpers ---

func buildVerifierSignedReceipt(t *testing.T, privateKey ed25519.PrivateKey, signerKeyID string) *operatorv1.ActionReceipt {
	t.Helper()
	receipt := &operatorv1.ActionReceipt{
		TransactionId: vTransactionID, TransactionHash: "tx-hash", Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		StateRootBefore: "root-before", StateRootAfter: "root-after", SignerKeyId: signerKeyID,
		ExecutedAtUnixMs: 1_700_000_001_000,
		L2Status: operatorv1.L2Status_L2_STATUS_REQUIRED_VALID, L3Status: operatorv1.L3Status_L3_STATUS_NOT_REQUIRED,
	}
	l4ID := receipt.TransactionId + ":L4"
	l5ID := receipt.TransactionId + ":L5"
	receipt.DeterministicStageEvidence = []*operatorv1.DeterministicStageEvidence{
		{StageId: receipt.TransactionId + ":L1", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, ParentStageId: l4ID},
		{StageId: receipt.TransactionId + ":L2", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, ParentStageId: l4ID},
		{StageId: receipt.TransactionId + ":L3", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED, ParentStageId: l4ID},
		{StageId: l4ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, ParentStageId: l5ID},
		{StageId: receipt.TransactionId + ":PERSIST", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, ParentStageId: l5ID},
		{StageId: receipt.TransactionId + ":COMMIT", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, ParentStageId: l5ID},
		{StageId: l5ID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L5_EXECUTION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_COMPLETED, StateRootBefore: receipt.StateRootBefore, StateRootAfter: receipt.StateRootAfter},
	}
	for _, stage := range receipt.DeterministicStageEvidence {
		stage.TransactionId = receipt.TransactionId
		stage.TransactionHash = receipt.TransactionHash
		stage.ActionType = string(constants.ActionTypeFileEdit)
		stage.OperatorId = vOperatorID
		stage.OperatorSessionId = vSessionID
		stage.CaseId = vRunID
		stage.InvestigationId = AllowedExecutionScenarioID
		stage.TaskId = vAllowedAttemptID
	}
	payload, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(privateKey, payload))
	attestation := &operatorv1.ReceiptPersistenceAttestation{
		TransactionId: receipt.TransactionId, ReceiptSignatureDigest: governance.SignatureDigest([]string{receipt.Signature}),
		PersistedAtUnixMs: 1_700_000_002_000, AuditRecordId: receipt.TransactionId, SignerKeyId: signerKeyID,
	}
	attestationPayload, err := governance.CanonicalizeReceiptPersistenceAttestation(attestation)
	require.NoError(t, err)
	attestation.Signature = hex.EncodeToString(ed25519.Sign(privateKey, attestationPayload))
	receipt.FinalPersistenceAttestation = attestation
	return receipt
}

func verifierEvidenceRef(artifactID string, artifactType complianceevidence.ArtifactType, runID, scenarioID, attemptID, transactionID string) *compliancev1.ComplianceEvidenceReference {
	_, digest, _ := complianceevidence.ParseContentAddress(artifactID)
	return &compliancev1.ComplianceEvidenceReference{
		ArtifactId: artifactID, ArtifactType: string(artifactType), Sha256: digest, MediaType: constants.MediaTypeJSON,
		RunId: runID, ScenarioId: scenarioID, AttemptId: attemptID, TransactionId: transactionID,
	}
}

func verifierIntObs(id, obsType, runID, scenarioID, attemptID string, value int64, evidenceRef *compliancev1.ComplianceEvidenceReference, observedAt time.Time) *evalv1.EvaluationObservation {
	return &evalv1.EvaluationObservation{
		ObservationId: id, ObservationType: versioned(obsType, RegistryVersion),
		Source: evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_TARGET_OBSERVER,
		Authority: evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE,
		ObservedAt: timestamppb.New(observedAt), RunId: runID, ScenarioId: scenarioID, AttemptId: attemptID,
		Value:       &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: value}},
		EvidenceRefs: []*compliancev1.ComplianceEvidenceReference{evidenceRef},
	}
}

func verifierBoolObs(id, obsType, runID, scenarioID, attemptID string, value bool, evidenceRefs []*compliancev1.ComplianceEvidenceReference, authority evalv1.EvaluationEvidenceAuthority, source evalv1.EvaluationObservationSource, observedAt time.Time) *evalv1.EvaluationObservation {
	refs := make([]*compliancev1.ComplianceEvidenceReference, len(evidenceRefs))
	for i, ref := range evidenceRefs {
		refs[i] = proto.Clone(ref).(*compliancev1.ComplianceEvidenceReference)
	}
	return &evalv1.EvaluationObservation{
		ObservationId: id, ObservationType: versioned(obsType, RegistryVersion),
		Source: source, Authority: authority, ObservedAt: timestamppb.New(observedAt),
		RunId: runID, ScenarioId: scenarioID, AttemptId: attemptID,
		Value:       &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_BooleanValue{BooleanValue: value}},
		EvidenceRefs: refs,
	}
}

func verifierIntOutcomeObs(id, obsType, runID, scenarioID, attemptID string, value int64, evidenceRefs []*compliancev1.ComplianceEvidenceReference, observedAt time.Time) *evalv1.EvaluationObservation {
	refs := make([]*compliancev1.ComplianceEvidenceReference, len(evidenceRefs))
	for i, ref := range evidenceRefs {
		refs[i] = proto.Clone(ref).(*compliancev1.ComplianceEvidenceReference)
	}
	return &evalv1.EvaluationObservation{
		ObservationId: id, ObservationType: versioned(obsType, RegistryVersion),
		Source: evalv1.EvaluationObservationSource_EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT,
		Authority: evalv1.EvaluationEvidenceAuthority_EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE,
		ObservedAt: timestamppb.New(observedAt), RunId: runID, ScenarioId: scenarioID, AttemptId: attemptID,
		Value:       &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: value}},
		EvidenceRefs: refs,
	}
}

// readReport reads and unmarshals the report from the reader.
func readReport(t *testing.T, reader *verifierArtifactReader, runID string) *evalv1.EvaluationReport {
	t.Helper()
	body, ok := reader.files[filepath.Join(evaluationRunDir(runID), constants.EvaluationReportFilename)]
	if !ok {
		return nil
	}
	report := &evalv1.EvaluationReport{}
	if err := evalv1.UnmarshalCanonical(body, report); err != nil {
		return nil
	}
	return report
}

// writeReport marshals and writes the report to the reader.
func writeReport(t *testing.T, reader *verifierArtifactReader, runID string, report *evalv1.EvaluationReport) {
	t.Helper()
	body, err := evalv1.MarshalCanonical(report)
	require.NoError(t, err)
	reader.files[filepath.Join(evaluationRunDir(runID), constants.EvaluationReportFilename)] = body
}

// writeEvidence writes an evidence artifact body to the reader.
func writeEvidence(reader *verifierArtifactReader, runID, artifactID string, body []byte) {
	_, digest, _ := complianceevidence.ParseContentAddress(artifactID)
	reader.files[filepath.Join(evaluationEvidenceDir(runID), digest+constants.FileExtJSON)] = body
}

// removeEvidence removes an evidence artifact from the reader.
func removeEvidence(reader *verifierArtifactReader, runID, artifactID string) {
	_, digest, _ := complianceevidence.ParseContentAddress(artifactID)
	delete(reader.files, filepath.Join(evaluationEvidenceDir(runID), digest+constants.FileExtJSON))
}

// --- Mutation matrix tests ---

func TestVerifier_MutationMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fix *verifierFixture)
	}{
		// 1. valid complete run
		{name: "valid complete run", mutate: func(_ *testing.T, _ *verifierFixture) {}},

		// 2. missing report
		{name: "missing report", mutate: func(_ *testing.T, fix *verifierFixture) {
			delete(fix.reader.files, filepath.Join(evaluationRunDir(vRunID), constants.EvaluationReportFilename))
		}},

		// 3. noncanonical report
		{name: "noncanonical report", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			body, err := evalv1.MarshalCanonical(report)
			require.NoError(t, err)
			nonCanonical := append(body, ' ')
			fix.reader.files[filepath.Join(evaluationRunDir(vRunID), constants.EvaluationReportFilename)] = nonCanonical
		}},

		// 4. report run-ID mismatch
		{name: "report run ID mismatch", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			report.Run.RunId = "other-run"
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 5. missing evidence file
		{name: "missing evidence file", mutate: func(_ *testing.T, fix *verifierFixture) {
			removeEvidence(fix.reader, vRunID, fix.targetState1Ref.GetArtifactId())
		}},

		// 6. digest substitution
		{name: "digest substitution", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			// Replace one evidence ref's sha256 with a different digest
			for _, obs := range report.Observations {
				for _, ref := range obs.EvidenceRefs {
					if ref.GetArtifactId() == fix.targetState1Ref.GetArtifactId() {
						ref.Sha256 = strings.Repeat("b", 64)
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.targetState1Ref.GetArtifactId() {
					ref.Sha256 = strings.Repeat("b", 64)
				}
			}
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 7. content-address type mismatch
		{name: "content address type mismatch", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			for _, obs := range report.Observations {
				for _, ref := range obs.EvidenceRefs {
					if ref.GetArtifactId() == fix.targetState1Ref.GetArtifactId() {
						ref.ArtifactType = "action-receipt"
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.targetState1Ref.GetArtifactId() {
					ref.ArtifactType = "action-receipt"
				}
			}
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 8. reference scope mismatch
		{name: "reference scope mismatch", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			for _, obs := range report.Observations {
				if obs.GetObservationId() == "obs-1" {
					obs.EvidenceRefs[0].AttemptId = vProhibitedAttemptID
				}
			}
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 9. duplicate conflicting observation ID
		{name: "duplicate observation ID", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			dup := proto.Clone(report.Observations[0]).(*evalv1.EvaluationObservation)
			report.Observations = append(report.Observations, dup)
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 10. target-state run/scenario/attempt mismatch
		{name: "target state run mismatch", mutate: func(t *testing.T, fix *verifierFixture) {
			targetState := &evalv1.EvaluationTargetState{
				SchemaVersion: RegistryVersion, RunId: "other-run", ScenarioId: AllowedExecutionScenarioID, AttemptId: vAllowedAttemptID,
				TargetResource: vTargetResource, ObservedAt: timestamppb.New(time.Unix(1_700_000_010, 0).UTC()), Present: false,
			}
			body, err := complianceevidence.MarshalCanonicalProto(targetState)
			require.NoError(t, err)
			newID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalObservation, body)
			fix.bodies[newID] = body
			fix.bodies[fix.targetState1Ref.GetArtifactId()] = body
			writeEvidence(fix.reader, vRunID, fix.targetState1Ref.GetArtifactId(), body)
		}},

		// 11. target-state timestamp mismatch
		{name: "target state timestamp mismatch", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			for _, obs := range report.Observations {
				if obs.GetObservationId() == "obs-1" {
					obs.ObservedAt = timestamppb.New(time.Unix(1_700_000_999, 0).UTC())
				}
			}
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 12. allowed target content absent or empty
		{name: "allowed target content absent", mutate: func(t *testing.T, fix *verifierFixture) {
			targetState := &evalv1.EvaluationTargetState{
				SchemaVersion: RegistryVersion, RunId: vRunID, ScenarioId: AllowedExecutionScenarioID, AttemptId: vAllowedAttemptID,
				TargetResource: vTargetResource, ObservedAt: timestamppb.New(time.Unix(1_700_000_020, 0).UTC()), Present: false,
			}
			body, err := complianceevidence.MarshalCanonicalProto(targetState)
			require.NoError(t, err)
			fix.bodies[fix.targetState2Ref.GetArtifactId()] = body
			writeEvidence(fix.reader, vRunID, fix.targetState2Ref.GetArtifactId(), body)
		}},

		// 13. prohibited target content differs from allowed final content
		{name: "prohibited target content differs", mutate: func(t *testing.T, fix *verifierFixture) {
			targetState := &evalv1.EvaluationTargetState{
				SchemaVersion: RegistryVersion, RunId: vRunID, ScenarioId: ProhibitedExecutionScenarioID, AttemptId: vProhibitedAttemptID,
				TargetResource: vTargetResource, ObservedAt: timestamppb.New(time.Unix(1_700_000_030, 0).UTC()), Present: true, Content: []byte("different\n"),
			}
			body, err := complianceevidence.MarshalCanonicalProto(targetState)
			require.NoError(t, err)
			fix.bodies[fix.targetState3Ref.GetArtifactId()] = body
			writeEvidence(fix.reader, vRunID, fix.targetState3Ref.GetArtifactId(), body)
		}},

		// 14. receipt substitution
		{name: "receipt substitution", mutate: func(t *testing.T, fix *verifierFixture) {
			fakeReceipt := &operatorv1.ActionReceipt{
				TransactionId: vTransactionID, TransactionHash: "different-hash", Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
				SignerKeyId: fix.signerKeyID, Signature: "invalid",
			}
			body, err := complianceevidence.MarshalCanonicalProto(fakeReceipt)
			require.NoError(t, err)
			fix.bodies[fix.receiptRef.GetArtifactId()] = body
			writeEvidence(fix.reader, vRunID, fix.receiptRef.GetArtifactId(), body)
		}},

		// 15. persistence attestation substitution
		{name: "persistence attestation substitution", mutate: func(t *testing.T, fix *verifierFixture) {
			fakeAttestation := &operatorv1.ReceiptPersistenceAttestation{
				TransactionId: vTransactionID, ReceiptSignatureDigest: "wrong", PersistedAtUnixMs: 1, AuditRecordId: vTransactionID, SignerKeyId: fix.signerKeyID, Signature: "invalid",
			}
			body, err := complianceevidence.MarshalCanonicalProto(fakeAttestation)
			require.NoError(t, err)
			fix.bodies[fix.persistenceRef.GetArtifactId()] = body
			writeEvidence(fix.reader, vRunID, fix.persistenceRef.GetArtifactId(), body)
		}},

		// 16. signer response missing or disabled
		{name: "signer response disabled", mutate: func(t *testing.T, fix *verifierFixture) {
			signerRespBody, err := json.Marshal(map[string]any{
				"id":            fix.signerKeyID,
				"public_key_hex": fix.signerKeyID,
				"enabled":       false,
			})
			require.NoError(t, err)
			allowedExchanges := []client.Exchange{
				{Method: "POST", URL: "https://localhost:8443" + constants.APIPaths.OperatorsCommands, Status: 200, RespBody: []byte(`{"success":true}`)},
				{Method: "GET", URL: "https://localhost:8443" + constants.APIPaths.GovernanceSignersByID + fix.signerKeyID, Status: 200, RespBody: signerRespBody},
			}
			body, err := json.Marshal(allowedExchanges)
			require.NoError(t, err)
			newID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalExchange, body)
			fix.bodies[newID] = body
			// Update the report's references to point to the new exchange
			report := readReport(t, fix.reader, vRunID)
			newRef := verifierEvidenceRef(newID, complianceevidence.ArtifactTypeEvalExchange, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, "")
			for _, obs := range report.Observations {
				if obs.GetAttemptId() == vAllowedAttemptID {
					for _, ref := range obs.EvidenceRefs {
						if ref.GetArtifactId() == fix.allowedExchangeRef.GetArtifactId() {
							ref.ArtifactId = newRef.ArtifactId
							ref.Sha256 = newRef.Sha256
						}
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.allowedExchangeRef.GetArtifactId() {
					ref.ArtifactId = newRef.ArtifactId
					ref.Sha256 = newRef.Sha256
				}
			}
			writeReport(t, fix.reader, vRunID, report)
			writeEvidence(fix.reader, vRunID, newID, body)
			removeEvidence(fix.reader, vRunID, fix.allowedExchangeRef.GetArtifactId())
			fix.allowedExchangeRef = newRef
		}},

		// 17. receipt signature invalid
		{name: "receipt signature invalid", mutate: func(t *testing.T, fix *verifierFixture) {
			receipt := proto.Clone(fix.receipt).(*operatorv1.ActionReceipt)
			receipt.Signature = hex.EncodeToString(ed25519.Sign(fix.privateKey, []byte("wrong payload")))
			body, err := complianceevidence.MarshalCanonicalProto(receipt)
			require.NoError(t, err)
			newID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeActionReceipt, body)
			fix.bodies[newID] = body
			report := readReport(t, fix.reader, vRunID)
			newRef := verifierEvidenceRef(newID, complianceevidence.ArtifactTypeActionReceipt, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, vTransactionID)
			for _, obs := range report.Observations {
				if obs.GetAttemptId() == vAllowedAttemptID {
					for _, ref := range obs.EvidenceRefs {
						if ref.GetArtifactId() == fix.receiptRef.GetArtifactId() {
							ref.ArtifactId = newRef.ArtifactId
							ref.Sha256 = newRef.Sha256
						}
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.receiptRef.GetArtifactId() {
					ref.ArtifactId = newRef.ArtifactId
					ref.Sha256 = newRef.Sha256
				}
			}
			writeReport(t, fix.reader, vRunID, report)
			writeEvidence(fix.reader, vRunID, newID, body)
			removeEvidence(fix.reader, vRunID, fix.receiptRef.GetArtifactId())
		}},

		// 18. persistence signature invalid
		{name: "persistence signature invalid", mutate: func(t *testing.T, fix *verifierFixture) {
			receipt := proto.Clone(fix.receipt).(*operatorv1.ActionReceipt)
			receipt.FinalPersistenceAttestation.Signature = hex.EncodeToString(ed25519.Sign(fix.privateKey, []byte("wrong attestation")))
			receiptBody, err := complianceevidence.MarshalCanonicalProto(receipt)
			require.NoError(t, err)
			newReceiptID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeActionReceipt, receiptBody)
			fix.bodies[newReceiptID] = receiptBody
			persistenceBody, err := complianceevidence.MarshalCanonicalProto(receipt.GetFinalPersistenceAttestation())
			require.NoError(t, err)
			newPersistenceID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeReceiptPersistence, persistenceBody)
			fix.bodies[newPersistenceID] = persistenceBody
			report := readReport(t, fix.reader, vRunID)
			newReceiptRef := verifierEvidenceRef(newReceiptID, complianceevidence.ArtifactTypeActionReceipt, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, vTransactionID)
			newPersistenceRef := verifierEvidenceRef(newPersistenceID, complianceevidence.ArtifactTypeReceiptPersistence, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, vTransactionID)
			for _, obs := range report.Observations {
				if obs.GetAttemptId() == vAllowedAttemptID {
					for _, ref := range obs.EvidenceRefs {
						if ref.GetArtifactId() == fix.receiptRef.GetArtifactId() {
							ref.ArtifactId = newReceiptRef.ArtifactId
							ref.Sha256 = newReceiptRef.Sha256
						}
						if ref.GetArtifactId() == fix.persistenceRef.GetArtifactId() {
							ref.ArtifactId = newPersistenceRef.ArtifactId
							ref.Sha256 = newPersistenceRef.Sha256
						}
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.receiptRef.GetArtifactId() {
					ref.ArtifactId = newReceiptRef.ArtifactId
					ref.Sha256 = newReceiptRef.Sha256
				}
				if ref.GetArtifactId() == fix.persistenceRef.GetArtifactId() {
					ref.ArtifactId = newPersistenceRef.ArtifactId
					ref.Sha256 = newPersistenceRef.Sha256
				}
			}
			// Also update the audit record to point to the new receipt
			record := &models.ActionReceiptRecord{
				TransactionID: receipt.TransactionId, TransactionHash: receipt.TransactionHash,
				InvestigationID: AllowedExecutionScenarioID, OperatorID: vOperatorID, OperatorSessionID: vSessionID,
				ActionType: constants.ActionTypeFileEdit, Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
				SignerKeyID: receipt.SignerKeyId, Signature: receipt.Signature, ActionReceipt: receipt,
			}
			auditResponse := &models.AuditReceiptsResponse{Success: true, Receipts: []*models.ActionReceiptRecord{record}}
			auditBody, err := json.Marshal(auditResponse)
			require.NoError(t, err)
			newAuditID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeAuditRecord, auditBody)
			fix.bodies[newAuditID] = auditBody
			newAuditRef := verifierEvidenceRef(newAuditID, complianceevidence.ArtifactTypeAuditRecord, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, vTransactionID)
			for _, obs := range report.Observations {
				if obs.GetAttemptId() == vAllowedAttemptID {
					for _, ref := range obs.EvidenceRefs {
						if ref.GetArtifactId() == fix.allowedAuditRef.GetArtifactId() {
							ref.ArtifactId = newAuditRef.ArtifactId
							ref.Sha256 = newAuditRef.Sha256
						}
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.allowedAuditRef.GetArtifactId() {
					ref.ArtifactId = newAuditRef.ArtifactId
					ref.Sha256 = newAuditRef.Sha256
				}
			}
			writeReport(t, fix.reader, vRunID, report)
			writeEvidence(fix.reader, vRunID, newReceiptID, receiptBody)
			writeEvidence(fix.reader, vRunID, newPersistenceID, persistenceBody)
			writeEvidence(fix.reader, vRunID, newAuditID, auditBody)
			removeEvidence(fix.reader, vRunID, fix.receiptRef.GetArtifactId())
			removeEvidence(fix.reader, vRunID, fix.persistenceRef.GetArtifactId())
			removeEvidence(fix.reader, vRunID, fix.allowedAuditRef.GetArtifactId())
		}},

		// 19. deterministic protocol-chain mutation
		{name: "deterministic protocol chain mutation", mutate: func(t *testing.T, fix *verifierFixture) {
			receipt := proto.Clone(fix.receipt).(*operatorv1.ActionReceipt)
			receipt.DeterministicStageEvidence[0].Outcome = operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED
			receiptBody, err := complianceevidence.MarshalCanonicalProto(receipt)
			require.NoError(t, err)
			newID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeActionReceipt, receiptBody)
			fix.bodies[newID] = receiptBody
			report := readReport(t, fix.reader, vRunID)
			newRef := verifierEvidenceRef(newID, complianceevidence.ArtifactTypeActionReceipt, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, vTransactionID)
			for _, obs := range report.Observations {
				if obs.GetAttemptId() == vAllowedAttemptID {
					for _, ref := range obs.EvidenceRefs {
						if ref.GetArtifactId() == fix.receiptRef.GetArtifactId() {
							ref.ArtifactId = newRef.ArtifactId
							ref.Sha256 = newRef.Sha256
						}
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.receiptRef.GetArtifactId() {
					ref.ArtifactId = newRef.ArtifactId
					ref.Sha256 = newRef.Sha256
				}
			}
			record := &models.ActionReceiptRecord{
				TransactionID: receipt.TransactionId, TransactionHash: receipt.TransactionHash,
				InvestigationID: AllowedExecutionScenarioID, OperatorID: vOperatorID, OperatorSessionID: vSessionID,
				ActionType: constants.ActionTypeFileEdit, Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
				SignerKeyID: receipt.SignerKeyId, Signature: receipt.Signature, ActionReceipt: receipt,
			}
			auditResponse := &models.AuditReceiptsResponse{Success: true, Receipts: []*models.ActionReceiptRecord{record}}
			auditBody, err := json.Marshal(auditResponse)
			require.NoError(t, err)
			newAuditID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeAuditRecord, auditBody)
			fix.bodies[newAuditID] = auditBody
			newAuditRef := verifierEvidenceRef(newAuditID, complianceevidence.ArtifactTypeAuditRecord, vRunID, AllowedExecutionScenarioID, vAllowedAttemptID, vTransactionID)
			for _, obs := range report.Observations {
				if obs.GetAttemptId() == vAllowedAttemptID {
					for _, ref := range obs.EvidenceRefs {
						if ref.GetArtifactId() == fix.allowedAuditRef.GetArtifactId() {
							ref.ArtifactId = newAuditRef.ArtifactId
							ref.Sha256 = newAuditRef.Sha256
						}
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.allowedAuditRef.GetArtifactId() {
					ref.ArtifactId = newAuditRef.ArtifactId
					ref.Sha256 = newAuditRef.Sha256
				}
			}
			writeReport(t, fix.reader, vRunID, report)
			writeEvidence(fix.reader, vRunID, newID, receiptBody)
			writeEvidence(fix.reader, vRunID, newAuditID, auditBody)
			removeEvidence(fix.reader, vRunID, fix.receiptRef.GetArtifactId())
			removeEvidence(fix.reader, vRunID, fix.allowedAuditRef.GetArtifactId())
		}},

		// 20. target Operator mismatch
		{name: "target operator mismatch", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			report.Run.TargetOperatorId = "other-operator"
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 21. target Operator session mismatch
		{name: "target operator session mismatch", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			report.Run.TargetOperatorSessionId = "other-session"
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 22. transaction mismatch
		{name: "transaction mismatch", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			report.Attempts[0].TransactionId = "other-tx"
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 23. execution/attempt mismatch
		{name: "execution attempt mismatch", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			report.Attempts[0].ExecutionId = "other-execution"
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 24. Gateway rejection exchange not correlated to the prohibited attempt
		{name: "gateway rejection not correlated", mutate: func(t *testing.T, fix *verifierFixture) {
			prohibDispatchReq := client.DispatchCommandRequest{
				TargetOperatorSessionID: vSessionID,
				ActionType:              string(constants.ActionTypeFileEdit),
				CaseID:                  vRunID,
				InvestigationID:         ProhibitedExecutionScenarioID,
				TaskID:                  "wrong-attempt-id",
				CLISessionID:            "cli-session-1",
			}
			prohibDispatchReqBody, err := json.Marshal(prohibDispatchReq)
			require.NoError(t, err)
			prohibDispatchRespBody := []byte(`{"error":"` + constants.ErrTxL1ValidationFailed.Error() + `"}`)
			prohibitedExchanges := []client.Exchange{
				{Method: "POST", URL: "https://localhost:8443" + constants.APIPaths.OperatorsCommands, ReqBody: prohibDispatchReqBody, Status: 400, RespBody: prohibDispatchRespBody},
			}
			body, err := json.Marshal(prohibitedExchanges)
			require.NoError(t, err)
			newID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalExchange, body)
			fix.bodies[newID] = body
			report := readReport(t, fix.reader, vRunID)
			newRef := verifierEvidenceRef(newID, complianceevidence.ArtifactTypeEvalExchange, vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, "")
			for _, obs := range report.Observations {
				if obs.GetAttemptId() == vProhibitedAttemptID {
					for _, ref := range obs.EvidenceRefs {
						if ref.GetArtifactId() == fix.prohibitedExchangeRef.GetArtifactId() {
							ref.ArtifactId = newRef.ArtifactId
							ref.Sha256 = newRef.Sha256
						}
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.prohibitedExchangeRef.GetArtifactId() {
					ref.ArtifactId = newRef.ArtifactId
					ref.Sha256 = newRef.Sha256
				}
			}
			writeReport(t, fix.reader, vRunID, report)
			writeEvidence(fix.reader, vRunID, newID, body)
			removeEvidence(fix.reader, vRunID, fix.prohibitedExchangeRef.GetArtifactId())
		}},

		// 25. L1 attribution marker missing
		{name: "L1 attribution marker missing", mutate: func(t *testing.T, fix *verifierFixture) {
			prohibDispatchReq := client.DispatchCommandRequest{
				TargetOperatorSessionID: vSessionID,
				ActionType:              string(constants.ActionTypeFileEdit),
				CaseID:                  vRunID,
				InvestigationID:         ProhibitedExecutionScenarioID,
				TaskID:                  vProhibitedAttemptID,
				CLISessionID:            "cli-session-1",
			}
			prohibDispatchReqBody, err := json.Marshal(prohibDispatchReq)
			require.NoError(t, err)
			// Rejection without L1 marker
			prohibDispatchRespBody := []byte(`{"error":"some other error"}`)
			prohibitedExchanges := []client.Exchange{
				{Method: "POST", URL: "https://localhost:8443" + constants.APIPaths.OperatorsCommands, ReqBody: prohibDispatchReqBody, Status: 400, RespBody: prohibDispatchRespBody},
			}
			body, err := json.Marshal(prohibitedExchanges)
			require.NoError(t, err)
			newID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeEvalExchange, body)
			fix.bodies[newID] = body
			report := readReport(t, fix.reader, vRunID)
			newRef := verifierEvidenceRef(newID, complianceevidence.ArtifactTypeEvalExchange, vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, "")
			for _, obs := range report.Observations {
				if obs.GetAttemptId() == vProhibitedAttemptID {
					for _, ref := range obs.EvidenceRefs {
						if ref.GetArtifactId() == fix.prohibitedExchangeRef.GetArtifactId() {
							ref.ArtifactId = newRef.ArtifactId
							ref.Sha256 = newRef.Sha256
						}
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.prohibitedExchangeRef.GetArtifactId() {
					ref.ArtifactId = newRef.ArtifactId
					ref.Sha256 = newRef.Sha256
				}
			}
			writeReport(t, fix.reader, vRunID, report)
			writeEvidence(fix.reader, vRunID, newID, body)
			removeEvidence(fix.reader, vRunID, fix.prohibitedExchangeRef.GetArtifactId())
		}},

		// 26. completed prohibited receipt present
		{name: "completed prohibited receipt present", mutate: func(t *testing.T, fix *verifierFixture) {
			// Add a completed receipt for the prohibited attempt
			completedReceipt := &operatorv1.ActionReceipt{
				TransactionId: "prohibited-tx", TransactionHash: "prohibited-hash",
				Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, SignerKeyId: fix.signerKeyID,
				DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{
					{TaskId: vProhibitedAttemptID, TransactionId: "prohibited-tx", TransactionHash: "prohibited-hash"},
				},
			}
			completedRecord := &models.ActionReceiptRecord{
				TransactionID: "prohibited-tx", TransactionHash: "prohibited-hash",
				InvestigationID: ProhibitedExecutionScenarioID, OperatorID: vOperatorID, OperatorSessionID: vSessionID,
				ActionType: constants.ActionTypeFileEdit, Status: operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
				SignerKeyID: fix.signerKeyID, ActionReceipt: completedReceipt,
			}
			auditResponse := &models.AuditReceiptsResponse{Success: true, Receipts: []*models.ActionReceiptRecord{completedRecord}}
			auditBody, err := json.Marshal(auditResponse)
			require.NoError(t, err)
			newID := complianceevidence.ContentAddress(complianceevidence.ArtifactTypeAuditRecord, auditBody)
			fix.bodies[newID] = auditBody
			report := readReport(t, fix.reader, vRunID)
			newRef := verifierEvidenceRef(newID, complianceevidence.ArtifactTypeAuditRecord, vRunID, ProhibitedExecutionScenarioID, vProhibitedAttemptID, "")
			for _, obs := range report.Observations {
				if obs.GetAttemptId() == vProhibitedAttemptID {
					for _, ref := range obs.EvidenceRefs {
						if ref.GetArtifactId() == fix.prohibitedAuditRef.GetArtifactId() {
							ref.ArtifactId = newRef.ArtifactId
							ref.Sha256 = newRef.Sha256
						}
					}
				}
			}
			for _, ref := range report.EvidenceRefs {
				if ref.GetArtifactId() == fix.prohibitedAuditRef.GetArtifactId() {
					ref.ArtifactId = newRef.ArtifactId
					ref.Sha256 = newRef.Sha256
				}
			}
			writeReport(t, fix.reader, vRunID, report)
			writeEvidence(fix.reader, vRunID, newID, auditBody)
			removeEvidence(fix.reader, vRunID, fix.prohibitedAuditRef.GetArtifactId())
		}},

		// 27. stored observation value differs from reconstructed evidence
		{name: "stored observation value differs", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			for _, obs := range report.Observations {
				if obs.GetObservationId() == "obs-2" {
					obs.Value = &evalv1.EvaluationValue{Value: &evalv1.EvaluationValue_IntegerValue{IntegerValue: 99}}
				}
			}
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 28. stored verdict differs from deterministic grading
		{name: "stored verdict differs", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			report.Verdicts[0].Status = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 29. stored metric differs from verdict derivation
		{name: "stored metric differs", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			report.Metrics[0].Numerator = 5
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 30. stored summary differs from verdict derivation
		{name: "stored summary differs", mutate: func(t *testing.T, fix *verifierFixture) {
			report := readReport(t, fix.reader, vRunID)
			report.Summary = "wrong summary"
			writeReport(t, fix.reader, vRunID, report)
		}},

		// 31. undeclared evidence file
		{name: "undeclared evidence file", mutate: func(_ *testing.T, fix *verifierFixture) {
			fix.reader.files[filepath.Join(evaluationEvidenceDir(vRunID), "undeclared"+constants.FileExtJSON)] = []byte(`{}`)
		}},

		// 32. substituted verification.json or mismatched final verification reference
		{name: "substituted verification json", mutate: func(t *testing.T, fix *verifierFixture) {
			fakeVerification := &compliancev1.ComplianceVerificationReport{
				ReportId: vRunID, Valid: true, VerifierId: constants.EvalRunVerifierID, VerifierVersion: constants.EvalRunVerifierVersion,
				VerifiedAt: timestamppb.New(fix.now()),
			}
			body, err := compliancev1.MarshalCanonical(fakeVerification)
			require.NoError(t, err)
			fix.reader.files[filepath.Join(evaluationRunDir(vRunID), constants.EvaluationVerificationFilename)] = body
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fix := buildValidVerifierFixture(t)
			test.mutate(t, fix)
			verifier := NewVerifier(fix.reader, NewRegistry(), fix.now)
			verification, err := verifier.Verify(context.Background(), vRunID)
			require.NoError(t, err)
			if test.name == "valid complete run" {
				assert.True(t, verification.GetValid(), "valid fixture should pass, got failures: %v", verification.GetFailures())
				assert.Empty(t, verification.GetFailures())
			} else {
				assert.False(t, verification.GetValid(), "mutated fixture should fail, but verification passed")
				assert.NotEmpty(t, verification.GetFailures(), "expected at least one failure")
			}
		})
	}
}

// TestVerifier_NilAndIncompleteInputs verifies fail-closed behavior for nil
// and incomplete verifier inputs.
func TestVerifier_NilAndIncompleteInputs(t *testing.T) {
	now := func() time.Time { return time.Unix(1_700_000_200, 0).UTC() }

	t.Run("nil verifier returns invalid", func(t *testing.T) {
		var v *Verifier
		result, err := v.Verify(context.Background(), "run-1")
		require.NoError(t, err)
		assert.False(t, result.GetValid())
	})

	t.Run("nil reader returns invalid", func(t *testing.T) {
		v := NewVerifier(nil, NewRegistry(), now)
		result, err := v.Verify(context.Background(), "run-1")
		require.NoError(t, err)
		assert.False(t, result.GetValid())
	})

	t.Run("empty run ID returns invalid", func(t *testing.T) {
		reader := &verifierArtifactReader{files: map[string][]byte{}}
		v := NewVerifier(reader, NewRegistry(), now)
		result, err := v.Verify(context.Background(), "")
		require.NoError(t, err)
		assert.False(t, result.GetValid())
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		fix := buildValidVerifierFixture(t)
		v := NewVerifier(fix.reader, NewRegistry(), fix.now)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := v.Verify(ctx, vRunID)
		require.Error(t, err)
	})

	t.Run("nil registry returns invalid", func(t *testing.T) {
		fix := buildValidVerifierFixture(t)
		v := NewVerifier(fix.reader, nil, fix.now)
		result, err := v.Verify(context.Background(), vRunID)
		require.NoError(t, err)
		assert.False(t, result.GetValid())
	})
}

// TestVerifier_ImportedEvidenceUsesNativeArtifacts verifies the native
// EvidenceImporter produces evidence nodes from a valid fixture.
func TestVerifier_ImportedEvidenceUsesNativeArtifacts(t *testing.T) {
	fix := buildValidVerifierFixture(t)
	importer := NewEvidenceImporter(fix.reader, vRunID, fix.now)
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, nodes)
	assert.Equal(t, "native-evaluation", importer.SourceID())
	assert.Equal(t, vRunID, importer.RunID())
	for _, node := range nodes {
		assert.Equal(t, complianceevidence.VerificationStatusVerified, node.VerificationStatus)
		assert.Equal(t, constants.EvalRunVerifierID, node.VerifierID)
	}
}

// TestVerifier_ImportFailsForInvalidRun verifies the importer rejects an
// invalid verification.
func TestVerifier_ImportFailsForInvalidRun(t *testing.T) {
	fix := buildValidVerifierFixture(t)
	// Corrupt the report to make verification fail
	report := readReport(t, fix.reader, vRunID)
	report.Run.TargetOperatorId = "wrong"
	writeReport(t, fix.reader, vRunID, report)
	importer := NewEvidenceImporter(fix.reader, vRunID, fix.now)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrEvalRunVerificationFailed), "expected ErrEvalRunVerificationFailed, got: %v", err)
}

// TestVerifier_ImportFailsForMissingReport verifies the importer rejects a
// missing report.
func TestVerifier_ImportFailsForMissingReport(t *testing.T) {
	fix := buildValidVerifierFixture(t)
	delete(fix.reader.files, filepath.Join(evaluationRunDir(vRunID), constants.EvaluationReportFilename))
	importer := NewEvidenceImporter(fix.reader, vRunID, fix.now)
	_, err := importer.Import(context.Background())
	require.Error(t, err)
}

// TestVerifier_ImportFailsForIncompleteImporter verifies the importer rejects
// nil reader or invalid run ID.
func TestVerifier_ImportFailsForIncompleteImporter(t *testing.T) {
	now := func() time.Time { return time.Unix(1_700_000_200, 0).UTC() }
	_, err := NewEvidenceImporter(nil, vRunID, now).Import(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
	_, err = NewEvidenceImporter(&verifierArtifactReader{files: map[string][]byte{}}, "", now).Import(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
}


