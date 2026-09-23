// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestOpenReadOnlyOperationalEvidence_DoesNotCreateAbsentDatabase(t *testing.T) {
	dbPath := filepath.Join(testutil.TempDir(t), constants.TestReadOnlyDatabaseFilename)
	reader, err := OpenReadOnlyOperationalEvidence(dbPath, testutil.NewTestLogger())
	require.Error(t, err)
	assert.Nil(t, reader)
	_, statErr := os.Stat(dbPath)
	assert.True(t, errors.Is(statErr, os.ErrNotExist))
}

func TestReadOnlyOperationalEvidence_SnapshotReadsBoundedReceiptAndCommitmentBodies(t *testing.T) {
	dbPath := filepath.Join(testutil.TempDir(t), constants.TestReadOnlyDatabaseFilename)
	writer, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), testutil.NewTestLogger())
	require.NoError(t, err)
	_, err = writer.Exec(auditStoreSchema)
	require.NoError(t, err)
	_, err = writer.Exec(`INSERT INTO receipts (transaction_id, transaction_hash, operator_id, action_type, status, executed_at_ms, signer_key_id, signature, receipt_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "tx-1", "hash-1", "operator-1", "FILE_EDIT", "COMPLETED", int64(1_700_000_001_000), "signer-1", "signature-1", `{"transactionId":"tx-1"}`)
	require.NoError(t, err)
	_, err = writer.Exec(`INSERT INTO commitment_ledger (transaction_id, transaction_hash, prior_commitment_hash, committed_at_unix_ms, hash, attestation_json) VALUES (?, ?, ?, ?, ?, ?)`, "tx-1", "hash-1", "", int64(1_700_000_002_000), "commitment-1", []byte(`{"transactionId":"tx-1"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	reader, err := OpenReadOnlyOperationalEvidence(dbPath, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	_, err = reader.db.Exec(`CREATE TABLE should_not_exist (id INTEGER)`)
	require.Error(t, err)

	snapshot, err := reader.Snapshot(context.Background(), OperationalEvidenceQuery{
		WindowStart: time.UnixMilli(1_700_000_000_000).UTC(),
		WindowEnd:   time.UnixMilli(1_700_000_003_000).UTC(),
		MaxRows:     10,
	})
	require.NoError(t, err)
	require.Len(t, snapshot.Receipts, 1)
	require.Len(t, snapshot.Commitments, 1)
	assert.Equal(t, "tx-1", snapshot.Receipts[0].TransactionID)
	assert.Equal(t, []byte(`{"transactionId":"tx-1"}`), snapshot.Receipts[0].Body)
	assert.Equal(t, "tx-1", snapshot.Commitments[0].TransactionID)
	assert.Equal(t, []byte(`{"transactionId":"tx-1"}`), snapshot.Commitments[0].Body)
}

func TestReadOnlyOperationalEvidence_SnapshotFailsInsteadOfTruncatingPopulation(t *testing.T) {
	dbPath := filepath.Join(testutil.TempDir(t), constants.TestReadOnlyDatabaseFilename)
	writer, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), testutil.NewTestLogger())
	require.NoError(t, err)
	_, err = writer.Exec(auditStoreSchema)
	require.NoError(t, err)
	for _, transactionID := range []string{"tx-1", "tx-2"} {
		_, err = writer.Exec(`INSERT INTO receipts (transaction_id, transaction_hash, operator_id, action_type, status, executed_at_ms, signer_key_id, signature) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, transactionID, "hash", "operator-1", "FILE_EDIT", "COMPLETED", int64(1_700_000_001_000), "signer-1", "signature")
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	reader, err := OpenReadOnlyOperationalEvidence(dbPath, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	_, err = reader.Snapshot(context.Background(), OperationalEvidenceQuery{
		WindowStart: time.UnixMilli(1_700_000_000_000).UTC(),
		WindowEnd:   time.UnixMilli(1_700_000_003_000).UTC(),
		MaxRows:     1,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceArtifactTooLarge)
}
