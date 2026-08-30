// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"crypto/ed25519"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLAuditStore_InitDatabase_CommitmentLedgerWardenIntentColumn verifies that
// initDatabase creates the commitment_ledger table with the protocol-aligned
// warden_intent_signature_digest column directly from the schema DDL.
func TestSQLAuditStore_InitDatabase_CommitmentLedgerWardenIntentColumn(t *testing.T) {
	tempDir := testutil.TempDir(t)

	fileSvc, _ := newTestFileSvc(t, tempDir)

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, filepath.Join(tempDir, constants.VaultDirname), privKey)

	config := &AuditStoreConfig{
		DBPath:               constants.TestCommitmentLedgerDBFilename,
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		EncryptionVault:      testVault,
	}

	ass, err := NewSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { ass.Close() })

	type columnInfo struct {
		CID  int
		Name string
		Type string
	}

	rows, err := ass.db.Query("PRAGMA table_info(commitment_ledger)")
	require.NoError(t, err)
	t.Cleanup(func() { rows.Close() })

	var columns []columnInfo
	for rows.Next() {
		var ci columnInfo
		var notNull, pk int
		var dflt sql.NullString
		require.NoError(t, rows.Scan(&ci.CID, &ci.Name, &ci.Type, &notNull, &dflt, &pk))
		columns = append(columns, ci)
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, columns, "commitment_ledger table should have columns")

	names := make(map[string]bool, len(columns))
	for _, c := range columns {
		names[c.Name] = true
	}

	assert.True(t, names["warden_intent_signature_digest"],
		"commitment_ledger must have warden_intent_signature_digest column; got %v", names)
	assert.False(t, names["actuator_intent_signature_digest"],
		"commitment_ledger must not retain actuator_intent_signature_digest column; got %v", names)
}

func TestMigrateCommitmentColumns_RenamesActuatorIntentWithoutLosingData(t *testing.T) {
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(":memory:"), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`CREATE TABLE commitment_ledger (id INTEGER PRIMARY KEY, actuator_intent_signature_digest TEXT)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO commitment_ledger (id, actuator_intent_signature_digest) VALUES (1, 'digest')`)
	require.NoError(t, err)

	require.NoError(t, migrateCommitmentColumns(db, testutil.NewTestLogger()))
	var digest string
	require.NoError(t, db.QueryRow(`SELECT warden_intent_signature_digest FROM commitment_ledger WHERE id = 1`).Scan(&digest))
	assert.Equal(t, "digest", digest)
	_, err = db.Exec(`SELECT actuator_intent_signature_digest FROM commitment_ledger`)
	assert.Error(t, err)
}

func TestSQLAuditStore_StartupMigratesPopulatedCommitmentLedger(t *testing.T) {
	tempDir := testutil.TempDir(t)
	fileSvc, _ := newTestFileSvc(t, tempDir)
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, filepath.Join(tempDir, constants.VaultDirname), privKey)
	config := &AuditStoreConfig{
		DBPath:               constants.TestCommitmentLedgerDBFilename,
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		EncryptionVault:      testVault,
	}
	store, err := NewSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	_, err = store.db.Exec(`INSERT INTO commitment_ledger (transaction_id, transaction_hash, prior_commitment_hash, warden_intent_signature_digest, committed_at_unix_ms, hash, attestation_json) VALUES ('tx-1', 'tx-hash', '', 'preserved-digest', 1, 'commitment-hash', '{}')`)
	require.NoError(t, err)
	_, err = store.db.Exec(`ALTER TABLE commitment_ledger RENAME COLUMN warden_intent_signature_digest TO actuator_intent_signature_digest`)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	reopened, err := NewSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	rows, err := reopened.CommitmentLedger().ListCommitments()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "preserved-digest", rows[0].WardenIntentSignatureDigest)
}
