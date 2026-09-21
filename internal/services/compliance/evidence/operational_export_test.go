// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestExportOperationalEvidence_PreservesReceiptsPersistenceAndCommitments(t *testing.T) {
	executedAt := time.UnixMilli(1_700_000_001_000).UTC()
	attestation := &operatorv1.ReceiptPersistenceAttestation{TransactionId: "tx-1", PersistedAtUnixMs: executedAt.Add(time.Second).UnixMilli(), SignerKeyId: "signer-1"}
	receiptBody, err := compliancev1.MarshalCanonical(&operatorv1.ActionReceipt{TransactionId: "tx-1", ExecutedAtUnixMs: executedAt.UnixMilli(), FinalPersistenceAttestation: attestation})
	require.NoError(t, err)
	outputDir := t.TempDir()
	inventory, err := ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{
		Receipts:    []storage.OperationalReceiptSource{{TransactionID: "tx-1", ExecutedAt: executedAt, Body: receiptBody}},
		Commitments: []storage.OperationalCommitmentSource{{Sequence: 1, TransactionID: "tx-1", CommittedAt: executedAt.Add(2 * time.Second), Body: []byte(`{"transactionId":"tx-1"}`)}},
	}, OperationalExportRequest{
		ScopeID:              "scope-1",
		OwnerRuntimeBoundary: "operator-1",
		AcquisitionBoundary:  "operator-local-export",
		WindowStart:          executedAt.Add(-time.Minute),
		WindowEnd:            executedAt.Add(time.Minute),
		MaxRows:              10,
		OutputDir:            outputDir,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, inventory.ReceiptCount)
	assert.Equal(t, 1, inventory.PersistenceCount)
	assert.Equal(t, 1, inventory.CommitmentCount)
	assert.Len(t, inventory.Artifacts, 3)
	for _, artifact := range inventory.Artifacts {
		body, readErr := os.ReadFile(filepath.Join(outputDir, artifact.RelativePath))
		require.NoError(t, readErr)
		assert.Equal(t, artifact.SHA256, exportDigestHex(body))
	}
	_, err = os.Stat(filepath.Join(outputDir, constants.ComplianceOperationalInventoryFilename))
	assert.NoError(t, err)
}

func TestExportOperationalEvidence_RecordsUnavailablePersistenceWithoutInventingAttestation(t *testing.T) {
	executedAt := time.UnixMilli(1_700_000_001_000).UTC()
	inventory, err := ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{
		Receipts: []storage.OperationalReceiptSource{{TransactionID: "tx-1", ExecutedAt: executedAt, Body: []byte(`{"transactionId":"tx-1"}`)}},
	}, OperationalExportRequest{
		ScopeID:              "scope-1",
		OwnerRuntimeBoundary: "operator-1",
		AcquisitionBoundary:  "operator-local-export",
		WindowStart:          executedAt.Add(-time.Minute),
		WindowEnd:            executedAt.Add(time.Minute),
		MaxRows:              10,
		OutputDir:            t.TempDir(),
	})
	require.NoError(t, err)
	assert.Zero(t, inventory.PersistenceCount)
	require.Len(t, inventory.Limitations, 1)
	assert.Contains(t, inventory.Limitations[0], "receipt tx-1 could not yield persistence evidence")
}

func exportDigestHex(body []byte) string {
	return ContentAddress(ArtifactTypeActionReceipt, body)[len(string(ArtifactTypeActionReceipt))+len(":sha256:"):]
}
