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
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
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
}
