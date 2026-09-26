// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type operationalCommitmentSigner struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
	keyID      string
}

func newOperationalCommitmentSigner(t *testing.T) *operationalCommitmentSigner {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	return &operationalCommitmentSigner{publicKey: publicKey, privateKey: privateKey, keyID: hex.EncodeToString(publicKey)}
}

func (s *operationalCommitmentSigner) commitment(t *testing.T, transactionID, priorHash string, committedAt time.Time) *operatorv1.CommitmentAttestation {
	t.Helper()
	attestation := &operatorv1.CommitmentAttestation{
		TransactionId:               transactionID,
		TransactionHash:             transactionID + "-hash",
		PriorCommitmentHash:         priorHash,
		StateRootAtCommit:           "state-" + transactionID,
		WardenIntentSignatureDigest: "warden-" + transactionID,
		ActionType:                  "FILE_EDIT",
		TargetResource:              "/tmp/" + transactionID,
		CommittedAtUnixMs:           committedAt.UnixMilli(),
		AuditorKeyId:                s.keyID,
	}
	payload, err := governance.CanonicalizeCommitmentAttestation(attestation)
	require.NoError(t, err)
	digest := sha256.Sum256(payload)
	attestation.Hash = hex.EncodeToString(digest[:])
	attestation.Signature = hex.EncodeToString(ed25519.Sign(s.privateKey, payload))
	return attestation
}

func canonicalOperationalCommitment(t *testing.T, attestation *operatorv1.CommitmentAttestation) []byte {
	t.Helper()
	body, err := compliancev1.MarshalCanonical(attestation)
	require.NoError(t, err)
	return body
}

func operationalExportRequestForTest(outputDir string, at time.Time) OperationalExportRequest {
	return OperationalExportRequest{
		ScopeID:              "scope-1",
		AdmissionID:          "source-1",
		SourceKind:           "operator-audit",
		SourceVersion:        "1.0.0",
		SourceScopeID:        "operator-scope-1",
		OwnerRuntimeBoundary: "operator-1",
		AcquisitionBoundary:  "operator-local-export",
		RunID:                "run-1",
		VerifierID:           "operational-export",
		VerifierVersion:      "1.0.0",
		WindowStart:          at.Add(-time.Minute),
		WindowEnd:            at.Add(time.Minute),
		MaxRows:              10,
		OutputDir:            outputDir,
	}
}

func operationalAdmissionForTest() *compliancev1.AssessmentSourceAdmission {
	return &compliancev1.AssessmentSourceAdmission{
		AdmissionId:          "source-1",
		SourceKind:           "operator-audit",
		SourceVersion:        "1.0.0",
		SourceScopeId:        "operator-scope-1",
		OwnerRuntimeBoundary: "operator-1",
		AcquisitionBoundary:  "operator-local-export",
		RunId:                "run-1",
		VerifierRef:          &compliancev1.VersionedReference{Id: "operational-export", Version: "1.0.0"},
	}
}

func operationalExportFiles(t *testing.T, outputDir string, inventory *OperationalSourceInventory, admissionID string) map[string][]byte {
	t.Helper()
	sourceRoot := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, admissionID)
	inventoryBody, err := os.ReadFile(filepath.Join(outputDir, constants.ComplianceOperationalInventoryFilename))
	require.NoError(t, err)
	files := map[string][]byte{path.Join(sourceRoot, constants.ComplianceOperationalInventoryFilename): inventoryBody}
	for _, artifact := range inventory.Artifacts {
		body, err := os.ReadFile(filepath.Join(outputDir, artifact.RelativePath))
		require.NoError(t, err)
		files[path.Join(sourceRoot, artifact.RelativePath)] = body
	}
	return files
}

func newOperationalTrust(signer *operationalCommitmentSigner) *assessedSignerStub {
	return &assessedSignerStub{keys: map[string]ed25519.PublicKey{signer.keyID: signer.publicKey}}
}

func TestOperationalExportImporter_RejectsBrokenCommitmentSegment(t *testing.T) {
	executedAt := time.UnixMilli(1_700_000_001_000).UTC()
	signer := newOperationalCommitmentSigner(t)
	first := signer.commitment(t, "tx-1", "", executedAt)
	second := signer.commitment(t, "tx-2", "wrong-prior", executedAt.Add(time.Second))
	outputDir := t.TempDir()
	inventory, err := ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{
		Commitments: []storage.OperationalCommitmentSource{
			{Sequence: 10, TransactionID: first.GetTransactionId(), CommittedAt: executedAt, Body: canonicalOperationalCommitment(t, first)},
			{Sequence: 11, TransactionID: second.GetTransactionId(), CommittedAt: executedAt.Add(time.Second), Body: canonicalOperationalCommitment(t, second)},
		},
	}, operationalExportRequestForTest(outputDir, executedAt))
	require.NoError(t, err)
	assert.Equal(t, int64(10), inventory.CommitmentFirstSequence)
	assert.Equal(t, int64(11), inventory.CommitmentLastSequence)

	files := operationalExportFiles(t, outputDir, inventory, "source-1")
	admission := operationalAdmissionForTest()
	importer := NewOperationalExportImporter(&memoryArtifactReader{files: files}, newOperationalTrust(signer), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1", constants.ComplianceOperationalInventoryFilename), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1"), "scope-1", admission, executedAt.Add(time.Minute), func() time.Time { return executedAt.Add(time.Minute) })
	_, err = importer.Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestOperationalExportImporter_AcceptsBoundedCommitmentSegmentWithoutWholeLedgerClaim(t *testing.T) {
	executedAt := time.UnixMilli(1_700_000_001_000).UTC()
	signer := newOperationalCommitmentSigner(t)
	commitment := signer.commitment(t, "tx-1", "external-prior", executedAt)
	body := canonicalOperationalCommitment(t, commitment)
	outputDir := t.TempDir()
	inventory, err := ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{
		Commitments: []storage.OperationalCommitmentSource{{Sequence: 10, TransactionID: commitment.GetTransactionId(), CommittedAt: executedAt, Body: body}},
	}, operationalExportRequestForTest(outputDir, executedAt))
	require.NoError(t, err)
	assert.Equal(t, "external-prior", inventory.CommitmentBoundaryPriorHash)
	assert.Equal(t, commitment.GetHash(), inventory.CommitmentHeadHash)
	files := operationalExportFiles(t, outputDir, inventory, "source-1")
	admission := operationalAdmissionForTest()
	importer := NewOperationalExportImporter(&memoryArtifactReader{files: files}, newOperationalTrust(signer), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1", constants.ComplianceOperationalInventoryFilename), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1"), "scope-1", admission, executedAt.Add(time.Minute), func() time.Time { return executedAt.Add(time.Minute) })
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, VerificationStatusVerified, nodes[0].VerificationStatus)
}

func TestOperationalExportImporter_RejectsCommitmentSegmentSequenceGap(t *testing.T) {
	executedAt := time.UnixMilli(1_700_000_001_000).UTC()
	signer := newOperationalCommitmentSigner(t)
	first := signer.commitment(t, "tx-1", "external-prior", executedAt)
	second := signer.commitment(t, "tx-2", first.GetHash(), executedAt.Add(time.Second))
	outputDir := t.TempDir()
	inventory, err := ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{
		Commitments: []storage.OperationalCommitmentSource{
			{Sequence: 10, TransactionID: first.GetTransactionId(), CommittedAt: executedAt, Body: canonicalOperationalCommitment(t, first)},
			{Sequence: 12, TransactionID: second.GetTransactionId(), CommittedAt: executedAt.Add(time.Second), Body: canonicalOperationalCommitment(t, second)},
		},
	}, operationalExportRequestForTest(outputDir, executedAt))
	require.NoError(t, err)
	assert.False(t, inventory.CommitmentSequenceContiguous)

	files := operationalExportFiles(t, outputDir, inventory, "source-1")
	admission := operationalAdmissionForTest()
	importer := NewOperationalExportImporter(&memoryArtifactReader{files: files}, newOperationalTrust(signer), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1", constants.ComplianceOperationalInventoryFilename), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1"), "scope-1", admission, executedAt.Add(time.Minute), func() time.Time { return executedAt.Add(time.Minute) })
	_, err = importer.Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestOperationalExportImporter_RejectsRetainedReceiptAndStageSubstitutions(t *testing.T) {
	executedAt := time.UnixMilli(1_700_000_001_000).UTC()
	tests := []struct {
		name   string
		mutate func(*operatorv1.ActionReceipt)
	}{
		{name: "receipt transaction hash", mutate: func(receipt *operatorv1.ActionReceipt) {
			receipt.TransactionHash = "substituted-transaction-hash"
			for _, stage := range receipt.DeterministicStageEvidence {
				stage.TransactionHash = receipt.TransactionHash
			}
		}},
		{name: "commitment stage signer", mutate: func(receipt *operatorv1.ActionReceipt) {
			for _, stage := range receipt.DeterministicStageEvidence {
				if stage.GetKind() == operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND {
					stage.SignerKeyId = "substituted-auditor"
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signer := newOperationalCommitmentSigner(t)
			commitment := signer.commitment(t, "tx-1", "external-prior", executedAt)
			receipt := newEvalVerifiedChainReceipt(signer.keyID)
			receipt.TransactionHash = commitment.GetTransactionHash()
			for _, stage := range receipt.DeterministicStageEvidence {
				stage.TransactionHash = receipt.GetTransactionHash()
				stage.ActionType = commitment.GetActionType()
				if stage.GetKind() == operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND {
					stage.CommitmentHash = commitment.GetHash()
					stage.PriorCommitmentHash = commitment.GetPriorCommitmentHash()
					stage.SignerKeyId = commitment.GetAuditorKeyId()
					stage.L2SignatureDigest = commitment.GetL2SignatureDigest()
					stage.L3SignatureDigest = commitment.GetHumanSignatureDigest()
				}
			}
			test.mutate(receipt)
			receiptPayload, err := governance.CanonicalizeActionReceipt(receipt)
			require.NoError(t, err)
			receipt.Signature = hex.EncodeToString(ed25519.Sign(signer.privateKey, receiptPayload))
			receiptBody, err := compliancev1.MarshalCanonical(receipt)
			require.NoError(t, err)
			outputDir := t.TempDir()
			inventory, err := ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{
				Receipts:    []storage.OperationalReceiptSource{{TransactionID: receipt.GetTransactionId(), ExecutedAt: executedAt, Body: receiptBody}},
				Commitments: []storage.OperationalCommitmentSource{{Sequence: 10, TransactionID: commitment.GetTransactionId(), CommittedAt: executedAt, Body: canonicalOperationalCommitment(t, commitment)}},
			}, operationalExportRequestForTest(outputDir, executedAt))
			require.NoError(t, err)
			files := operationalExportFiles(t, outputDir, inventory, "source-1")
			admission := operationalAdmissionForTest()
			importer := NewOperationalExportImporter(&memoryArtifactReader{files: files}, newOperationalTrust(signer), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1", constants.ComplianceOperationalInventoryFilename), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1"), "scope-1", admission, executedAt.Add(time.Minute), func() time.Time { return executedAt.Add(time.Minute) })
			_, err = importer.Import(context.Background())
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
		})
	}
}

func TestOperationalExportImporter_ReplaysExportedReceiptAndPersistenceBodies(t *testing.T) {
	executedAt := time.UnixMilli(1_700_000_001_000).UTC()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signerKeyID := hex.EncodeToString(publicKey)
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    "tx-1",
		TransactionHash:  "hash-tx-1",
		SignerKeyId:      signerKeyID,
		ExecutedAtUnixMs: executedAt.UnixMilli(),
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
		L2Status:         operatorv1.L2Status_L2_STATUS_NOT_REQUIRED,
		L3Status:         operatorv1.L3Status_L3_STATUS_NOT_REQUIRED,
		DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{
			{StageId: "tx-1:L1", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED, TransactionId: "tx-1", TransactionHash: "hash-tx-1", InvestigationId: "investigation-1", ParentStageId: "tx-1:L4", ActionType: "FILE_EDIT"},
			{StageId: "tx-1:L4", Kind: operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, Outcome: operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_FAILED, TransactionId: "tx-1", TransactionHash: "hash-tx-1", InvestigationId: "investigation-1", ActionType: "FILE_EDIT"},
		},
	}
	receiptPayload, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(privateKey, receiptPayload))
	attestation := &operatorv1.ReceiptPersistenceAttestation{TransactionId: "tx-1", ReceiptSignatureDigest: governance.SignatureDigest([]string{receipt.Signature}), PersistedAtUnixMs: executedAt.Add(time.Second).UnixMilli(), AuditRecordId: "tx-1", SignerKeyId: signerKeyID}
	attestationPayload, err := governance.CanonicalizeReceiptPersistenceAttestation(attestation)
	require.NoError(t, err)
	attestation.Signature = hex.EncodeToString(ed25519.Sign(privateKey, attestationPayload))
	receipt.FinalPersistenceAttestation = attestation
	receiptBody, err := compliancev1.MarshalCanonical(receipt)
	require.NoError(t, err)
	outputDir := t.TempDir()
	_, err = ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{Receipts: []storage.OperationalReceiptSource{{TransactionID: "tx-1", ExecutedAt: executedAt, Body: receiptBody}}}, OperationalExportRequest{
		ScopeID:              "scope-1",
		AdmissionID:          "source-1",
		SourceKind:           "operator-audit",
		SourceVersion:        "1.0.0",
		SourceScopeID:        "operator-scope-1",
		OwnerRuntimeBoundary: "operator-1",
		RunID:                "run-1",
		VerifierID:           "operational-export",
		VerifierVersion:      "1.0.0",
		AcquisitionBoundary:  "operator-local-export",
		WindowStart:          executedAt.Add(-time.Minute),
		WindowEnd:            executedAt.Add(time.Minute),
		MaxRows:              10,
		OutputDir:            outputDir,
	})
	require.NoError(t, err)

	sourceRoot := path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1")
	files := map[string][]byte{}
	inventoryBody, err := os.ReadFile(filepath.Join(outputDir, constants.ComplianceOperationalInventoryFilename))
	require.NoError(t, err)
	files[path.Join(sourceRoot, constants.ComplianceOperationalInventoryFilename)] = inventoryBody
	var inventory OperationalSourceInventory
	require.NoError(t, json.Unmarshal(inventoryBody, &inventory))
	for _, artifact := range inventory.Artifacts {
		body, readErr := os.ReadFile(filepath.Join(outputDir, artifact.RelativePath))
		require.NoError(t, readErr)
		files[path.Join(sourceRoot, artifact.RelativePath)] = body
	}

	admission := &compliancev1.AssessmentSourceAdmission{AdmissionId: "source-1", SourceKind: "operator-audit", SourceVersion: "1.0.0", SourceScopeId: "operator-scope-1", OwnerRuntimeBoundary: "operator-1", AcquisitionBoundary: "operator-local-export", RunId: "run-1", VerifierRef: &compliancev1.VersionedReference{Id: "operational-export", Version: "1.0.0"}}
	importer := NewOperationalExportImporter(&memoryArtifactReader{files: files}, &assessedSignerStub{keys: map[string]ed25519.PublicKey{signerKeyID: publicKey}}, path.Join(sourceRoot, constants.ComplianceOperationalInventoryFilename), sourceRoot, "scope-1", admission, executedAt.Add(time.Minute), func() time.Time { return executedAt.Add(time.Minute) })
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 3)
	assert.Equal(t, ArtifactTypeActionReceipt, nodes[0].ArtifactType)
	assert.Equal(t, ArtifactTypeProtocolChain, nodes[1].ArtifactType)
	assert.Equal(t, ArtifactTypeReceiptPersistence, nodes[2].ArtifactType)
	assert.Equal(t, VerificationStatusVerified, nodes[0].VerificationStatus)
	assert.Equal(t, VerificationStatusVerified, nodes[1].VerificationStatus)
	assert.Equal(t, []string{nodes[0].ArtifactID}, nodes[1].References)
	assert.True(t, len(nodes[0].BundlePath) > len(sourceRoot))
	assert.Equal(t, sourceRoot, nodes[0].BundlePath[:len(sourceRoot)])

	inventory.SourceScopeID = "other-operator-scope"
	files[path.Join(sourceRoot, constants.ComplianceOperationalInventoryFilename)], err = json.Marshal(&inventory)
	require.NoError(t, err)
	_, err = importer.Import(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
	assert.Equal(t, "operational-export", importer.SourceID())
}

func TestOperationalExportImporter_VerifiesAuditChainAndCrossLinksReceipts(t *testing.T) {
	executedAt := time.UnixMilli(1_700_000_001_000).UTC()
	signer := newOperationalCommitmentSigner(t)
	commitment := signer.commitment(t, "tx-1", "external-prior", executedAt)
	receipt := newEvalVerifiedChainReceipt(signer.keyID)
	receipt.TransactionHash = commitment.GetTransactionHash()
	for _, stage := range receipt.DeterministicStageEvidence {
		stage.TransactionHash = receipt.GetTransactionHash()
		if stage.GetKind() == operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND {
			stage.CommitmentHash = commitment.GetHash()
			stage.PriorCommitmentHash = commitment.GetPriorCommitmentHash()
			stage.SignerKeyId = commitment.GetAuditorKeyId()
			stage.L2SignatureDigest = commitment.GetL2SignatureDigest()
			stage.L3SignatureDigest = commitment.GetHumanSignatureDigest()
		}
	}
	receiptPayload, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	receipt.Signature = hex.EncodeToString(ed25519.Sign(signer.privateKey, receiptPayload))
	receiptBody, err := compliancev1.MarshalCanonical(receipt)
	require.NoError(t, err)

	timestamp := executedAt.UTC().Format(time.RFC3339Nano)
	digest, err := storage.ComputeAuditEventContentDigest(&storage.Event{ContentText: string(receiptBody)})
	require.NoError(t, err)
	prevHash := strings.Repeat("0", 64)
	hash := storage.AuditEventChainHash(7, prevHash, string(constants.EventOperatorReceiptRecorded), "session-1", timestamp, digest, receipt.GetTransactionId())

	outputDir := t.TempDir()
	inventory, err := ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{
		Receipts: []storage.OperationalReceiptSource{{TransactionID: receipt.GetTransactionId(), ExecutedAt: executedAt, Body: receiptBody}},
		Commitments: []storage.OperationalCommitmentSource{{Sequence: 10, TransactionID: commitment.GetTransactionId(), CommittedAt: executedAt, Body: canonicalOperationalCommitment(t, commitment)}},
		AuditChain: []storage.OperationalAuditChainSource{{
			Seq:               7,
			PrevHash:          prevHash,
			Hash:              hash,
			EventType:         string(constants.EventOperatorReceiptRecorded),
			OperatorSessionID: "session-1",
			Timestamp:         executedAt,
			ContentDigest:     digest,
			TransactionID:     receipt.GetTransactionId(),
			ContentText:       string(receiptBody),
		}},
	}, operationalExportRequestForTest(outputDir, executedAt))
	require.NoError(t, err)
	assert.Equal(t, 1, inventory.AuditChainCount)

	files := operationalExportFiles(t, outputDir, inventory, "source-1")
	admission := operationalAdmissionForTest()
	importer := NewOperationalExportImporter(&memoryArtifactReader{files: files}, newOperationalTrust(signer), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1", constants.ComplianceOperationalInventoryFilename), path.Join(constants.ComplianceBundleSourcesDirname, constants.ComplianceOperationalExportDirname, "source-1"), "scope-1", admission, executedAt.Add(time.Minute), func() time.Time { return executedAt.Add(time.Minute) })
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)

	var receiptNode, chainNode, commitmentNode *EvidenceNode
	for index := range nodes {
		switch nodes[index].ArtifactType {
		case ArtifactTypeActionReceipt:
			receiptNode = &nodes[index]
		case ArtifactTypeAuditChainEntry:
			chainNode = &nodes[index]
		case ArtifactTypeCommitment:
			commitmentNode = &nodes[index]
		}
	}
	require.NotNil(t, receiptNode)
	require.NotNil(t, chainNode)
	require.NotNil(t, commitmentNode)
	assert.Contains(t, receiptNode.References, chainNode.ArtifactID)
	assert.Contains(t, chainNode.References, receiptNode.ArtifactID)
	assert.Contains(t, commitmentNode.References, receiptNode.ArtifactID)
}
