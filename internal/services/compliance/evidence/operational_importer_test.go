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
	"encoding/json"
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

func TestOperationalExportImporter_ReplaysExportedReceiptAndPersistenceBodies(t *testing.T) {
	executedAt := time.UnixMilli(1_700_000_001_000).UTC()
	attestation := &operatorv1.ReceiptPersistenceAttestation{TransactionId: "tx-1", PersistedAtUnixMs: executedAt.Add(time.Second).UnixMilli(), SignerKeyId: "signer-1"}
	receiptBody, err := compliancev1.MarshalCanonical(&operatorv1.ActionReceipt{TransactionId: "tx-1", SignerKeyId: "signer-1", ExecutedAtUnixMs: executedAt.UnixMilli(), FinalPersistenceAttestation: attestation})
	require.NoError(t, err)
	outputDir := t.TempDir()
	_, err = ExportOperationalEvidence(context.Background(), &storage.OperationalEvidenceSnapshot{Receipts: []storage.OperationalReceiptSource{{TransactionID: "tx-1", ExecutedAt: executedAt, Body: receiptBody}}}, OperationalExportRequest{
		ScopeID:              "scope-1",
		OwnerRuntimeBoundary: "operator-1",
		AcquisitionBoundary:  "operator-local-export",
		WindowStart:          executedAt.Add(-time.Minute),
		WindowEnd:            executedAt.Add(time.Minute),
		MaxRows:              10,
		OutputDir:            outputDir,
	})
	require.NoError(t, err)

	files := map[string][]byte{}
	inventoryBody, err := os.ReadFile(filepath.Join(outputDir, constants.ComplianceOperationalInventoryFilename))
	require.NoError(t, err)
	files[constants.ComplianceOperationalInventoryFilename] = inventoryBody
	var inventory OperationalSourceInventory
	require.NoError(t, json.Unmarshal(inventoryBody, &inventory))
	for _, artifact := range inventory.Artifacts {
		body, readErr := os.ReadFile(filepath.Join(outputDir, artifact.RelativePath))
		require.NoError(t, readErr)
		files[artifact.RelativePath] = body
	}

	publicKey := ed25519.PublicKey(make([]byte, ed25519.PublicKeySize))
	importer := NewOperationalExportImporter(&memoryArtifactReader{files: files}, &assessedSignerStub{keys: map[string]ed25519.PublicKey{"signer-1": publicKey}}, constants.ComplianceOperationalInventoryFilename, "scope-1", "run-1", executedAt.Add(time.Minute), func() time.Time { return executedAt.Add(time.Minute) })
	nodes, err := importer.Import(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	assert.Equal(t, ArtifactTypeActionReceipt, nodes[0].ArtifactType)
	assert.Equal(t, ArtifactTypeReceiptPersistence, nodes[1].ArtifactType)
	assert.Equal(t, VerificationStatusFailed, nodes[0].VerificationStatus)
}
