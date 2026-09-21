// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package report

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type operationalAcceptanceHandler struct{}

func (operationalAcceptanceHandler) ExecuteVerifiedTransaction(context.Context, constants.EventType, governance.CommandMessage) (string, error) {
	return "operator fixture completed", nil
}

func TestOperationalOperatorEvidence_GeneratesNativeAssuranceAndVerifiesOffline(t *testing.T) {
	ctx := context.Background()
	tempDir := testutil.TempDir(t)
	fileSvc := storagetest.NewTestFileSvc(t, tempDir)
	_, vaultKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := storagetest.CreateTestVault(t, fileSvc.Resolve(constants.VaultDirname), vaultKey)

	auditConfig := storage.DefaultAuditStoreConfig()
	auditConfig.EncryptionVault = testVault
	auditStore, err := storage.NewSQLAuditStore(auditConfig, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	auditStoreClosed := false
	t.Cleanup(func() {
		if !auditStoreClosed {
			require.NoError(t, auditStore.Close())
		}
	})
	require.NoError(t, auditStore.CreateSession("operator-session-1", constants.SessionTypeOperator, "operator fixture", "test-user"))

	receiptPublicKey, receiptPrivateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	auditorPublicKey, auditorPrivateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	receiptKeyID := hex.EncodeToString(receiptPublicKey)
	auditorKeyID := hex.EncodeToString(auditorPublicKey)
	stateRoot := testutil.NewMockStateRootProvider("operator-state-root")
	actuator := &governance.L5Actuator{
		Logger:            testutil.NewTestLogger(),
		SQLAuditStore:     auditStore,
		ConsoleAuditStore: testutil.NewConfigurableMockAuditStore(nil),
		StateRootProvider: stateRoot,
		ExecutionHandler:  operationalAcceptanceHandler{},
		SigningKey:        receiptPrivateKey,
		KeyID:             receiptKeyID,
		AuditorSigningKey: auditorPrivateKey,
		AuditorKeyID:      auditorKeyID,
	}

	envelope := &govtypes.GovernanceEnvelope{
		Id:                uuid.NewString(),
		TransactionHash:   "operator-transaction-hash",
		OperatorId:        "operator-1",
		OperatorSessionId: "operator-session-1",
		RequestorUserId:   "test-user",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "operator-fixture-target",
		CaseId:            "operator-run-1",
		InvestigationId:   "operator-investigation-1",
		TaskId:            "operator-attempt-1",
	}
	l4StageID := envelope.Id + ":l4"
	vt := &governance.VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
		DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{
			{StageId: envelope.Id + ":l1", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, TransactionId: envelope.Id, TransactionHash: envelope.TransactionHash, ActionType: envelope.ActionType, OperatorId: envelope.OperatorId, OperatorSessionId: envelope.OperatorSessionId, CaseId: envelope.CaseId, InvestigationId: envelope.InvestigationId, TaskId: envelope.TaskId, ParentStageId: l4StageID},
			{StageId: envelope.Id + ":l2", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_PROTOCOL_L2, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED, TransactionId: envelope.Id, TransactionHash: envelope.TransactionHash, ActionType: envelope.ActionType, OperatorId: envelope.OperatorId, OperatorSessionId: envelope.OperatorSessionId, CaseId: envelope.CaseId, InvestigationId: envelope.InvestigationId, TaskId: envelope.TaskId, ParentStageId: l4StageID},
			{StageId: envelope.Id + ":l3", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L3_NOTARY, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED, TransactionId: envelope.Id, TransactionHash: envelope.TransactionHash, ActionType: envelope.ActionType, OperatorId: envelope.OperatorId, OperatorSessionId: envelope.OperatorSessionId, CaseId: envelope.CaseId, InvestigationId: envelope.InvestigationId, TaskId: envelope.TaskId, ParentStageId: l4StageID},
			{StageId: l4StageID, Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED, TransactionId: envelope.Id, TransactionHash: envelope.TransactionHash, ActionType: envelope.ActionType, OperatorId: envelope.OperatorId, OperatorSessionId: envelope.OperatorSessionId, CaseId: envelope.CaseId, InvestigationId: envelope.InvestigationId, TaskId: envelope.TaskId},
		},
	}
	receipt, err := actuator.Execute(ctx, vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	now := time.Now().UTC()
	windowStart := now.Add(-time.Minute)
	windowEnd := now.Add(time.Minute)
	dbPath := filepath.Join(fileSvc.Resolve(constants.DataDirname), constants.DbFilename)
	reader, err := storage.OpenReadOnlyOperationalEvidence(dbPath, testutil.NewTestLogger())
	require.NoError(t, err)
	snapshot, err := reader.Snapshot(ctx, storage.OperationalEvidenceQuery{WindowStart: windowStart, WindowEnd: windowEnd, MaxRows: 10})
	require.NoError(t, err)
	require.Len(t, snapshot.Receipts, 1)
	require.NotEmpty(t, snapshot.Receipts[0].Body)
	require.Len(t, snapshot.Commitments, 1)
	require.NoError(t, reader.Close())
	require.NoError(t, auditStore.Close())
	auditStoreClosed = true

	assessmentAsOf := now
	scope := validGenerationScope(windowStart, windowEnd)
	scope.AssessmentAsOf = timestamppb.New(assessmentAsOf)
	admission := scope.SourceAdmissions[0]
	admission.SourceKind = "operator-audit"
	admission.SourceVersion = "1.0.0"
	admission.SourceScopeId = "operator-scope-1"
	admission.OwnerRuntimeBoundary = "operator-1"
	admission.AcquisitionBoundary = "operator-local-export"
	admission.RunId = "operator-run-1"
	admission.VerifierRef = &compliancev1.VersionedReference{Id: "operational-export", Version: "1.0.0"}
	admission.DisclosureClassification = constants.ComplianceBundleProfileRestricted

	sourceDir := t.TempDir()
	_, err = evidence.ExportOperationalEvidence(ctx, snapshot, evidence.OperationalExportRequest{
		ScopeID:              scope.GetScopeId(),
		AdmissionID:          admission.GetAdmissionId(),
		SourceKind:           admission.GetSourceKind(),
		SourceVersion:        admission.GetSourceVersion(),
		SourceScopeID:        admission.GetSourceScopeId(),
		OwnerRuntimeBoundary: admission.GetOwnerRuntimeBoundary(),
		AcquisitionBoundary:  admission.GetAcquisitionBoundary(),
		RunID:                admission.GetRunId(),
		VerifierID:           admission.GetVerifierRef().GetId(),
		VerifierVersion:      admission.GetVerifierRef().GetVersion(),
		WindowStart:          windowStart,
		WindowEnd:            windowEnd,
		MaxRows:              10,
		OutputDir:            sourceDir,
	})
	require.NoError(t, err)

	sourceRoot := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, admission.GetAdmissionId())
	inventoryBody, err := os.ReadFile(filepath.Join(sourceDir, constants.ComplianceOperationalInventoryFilename))
	require.NoError(t, err)
	inventory := &evidence.OperationalSourceInventory{}
	require.NoError(t, decodeJSON(inventoryBody, inventory))
	sourceBodies := map[string][]byte{path.Join(sourceRoot, constants.ComplianceOperationalInventoryFilename): inventoryBody}
	for _, artifact := range inventory.Artifacts {
		body, readErr := os.ReadFile(filepath.Join(sourceDir, artifact.RelativePath))
		require.NoError(t, readErr)
		sourceBodies[path.Join(sourceRoot, artifact.RelativePath)] = body
	}

	trust := &assessedEvidenceSignerStub{keys: map[string]ed25519.PublicKey{receiptKeyID: receiptPublicKey, auditorKeyID: auditorPublicKey}}
	importer := evidence.NewOperationalExportImporter(&bundledSourceArtifactReader{bodies: sourceBodies}, trust, path.Join(sourceRoot, constants.ComplianceOperationalInventoryFilename), sourceRoot, scope.GetScopeId(), admission, assessmentAsOf, func() time.Time { return assessmentAsOf })
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	identity := bundleSigningIdentityFixture(t)
	generatedAt := assessmentAsOf.Add(time.Second)
	generation := GenerationRequest{
		Scope:      scope,
		Sources:    []GenerationSource{{AdmissionID: admission.GetAdmissionId(), Importer: importer}},
		Assertions: assertions,
		Frameworks: frameworks,
		Crosswalks: crosswalks,
	}
	generated, err := GenerateComplianceAnalysis(ctx, generation)
	graphReport := ""
	if generated != nil && generated.GraphReport != nil {
		graphReport = generated.GraphReport.String()
	}
	require.NoError(t, err, graphReport)
	assembly, err := GenerateSignedComplianceBundle(ctx, SignedBundleGenerationRequest{
		Generation:      generation,
		Profile:         ProfileRestricted,
		ReportID:        "operator-assurance-1",
		GeneratedAt:     generatedAt,
		SigningIdentity: identity,
		SourceArtifacts: operationalSourceArtifacts(sourceBodies),
	})
	require.NoError(t, err)
	require.NotEmpty(t, assembly.Bundle.GetAnalysis().GetEvidenceResources())

	bundleBodies := make(map[string][]byte, len(assembly.ArtifactBodies))
	for _, artifact := range assembly.ArtifactBodies {
		bundleBodies[artifact.BundlePath] = artifact.Body
	}
	reportPublicKey := identity.privateKey.Public().(ed25519.PublicKey)
	reportPolicy := &compliancev1.ComplianceReportTrustPolicy{
		PolicyId:      "operator-report-policy",
		PolicyVersion: "1.0.0",
		TrustedKeys: []*compliancev1.ComplianceReportTrustedKey{{
			Metadata:         proto.Clone(identity.metadata).(*compliancev1.ComplianceReportSigningKeyMetadata),
			PublicKey:        hex.EncodeToString(reportPublicKey),
			AssessmentId:     "operator-assessment-1",
			AssessorIdentity: "operator-assessor",
			AssessedAt:       timestamppb.New(assessmentAsOf),
			AllowedScopeRefs: []string{scope.GetScopeId()},
		}},
	}
	verification, err := VerifyComplianceReportBundle(ctx, BundleVerificationRequest{Bundle: assembly.Bundle, Reader: &bundleArtifactReaderStub{bodies: bundleBodies}, TrustPolicy: reportPolicy, EvidenceTrust: trust, VerifiedAt: generatedAt.Add(time.Minute)})
	require.NoError(t, err)
	require.True(t, verification.GetValid(), verification.GetFailures())
}

func decodeJSON(body []byte, target any) error {
	return json.Unmarshal(body, target)
}

func operationalSourceArtifacts(sourceBodies map[string][]byte) []SourceArtifact {
	artifacts := make([]SourceArtifact, 0, len(sourceBodies))
	for bundlePath, body := range sourceBodies {
		artifacts = append(artifacts, SourceArtifact{BundlePath: bundlePath, Body: body, MediaType: constants.MediaTypeJSON})
	}
	sort.Slice(artifacts, func(left, right int) bool { return artifacts[left].BundlePath < artifacts[right].BundlePath })
	return artifacts
}
