// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reporting

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// setupTestAuditStore creates a real SQLAuditStore with a temp directory and
// unlocked vault for encryption. Cleanup is registered via t.Cleanup.
func setupTestAuditStore(t *testing.T) *storage.SQLAuditStore {
	t.Helper()
	tempDir := t.TempDir()

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	testVault := createTestVault(t, vaultDir, privKey)

	cfg := &storage.AuditStoreConfig{
		DataDir:              tempDir,
		DBPath:               "test_audit.db",
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		EncryptionVault:      testVault,
	}
	store, err := storage.NewSQLAuditStore(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

// createTestVault creates an unlocked vault in the given directory.
func createTestVault(t *testing.T, dataDir string, privateKey []byte) *vault.Vault {
	t.Helper()
	require.NoError(t, os.MkdirAll(dataDir, 0700))
	header, _, err := vault.NewVaultHeader(privateKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(dataDir))
	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dataDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	require.NoError(t, v.Unlock(privateKey))
	t.Cleanup(func() { v.Close() })
	return v
}

// setupTestCommitmentLedger creates a commitment ledger backed by an in-memory SQLite DB.
// Returns both the CommitmentLedger and the underlying DB for direct SQL access.
func setupTestCommitmentLedger(t *testing.T) (*storage.CommitmentLedger, *sqliteutil.DB) {
	t.Helper()
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(":memory:"), testutil.NewTestLogger())
	require.NoError(t, err)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS commitment_ledger (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			transaction_id TEXT NOT NULL,
			transaction_hash TEXT NOT NULL,
			prior_commitment_hash TEXT NOT NULL,
			state_root_at_commit TEXT,
			l2_signature_digest TEXT,
			Actuator_intent_signature_digest TEXT,
			human_signature_digest TEXT,
			action_type TEXT,
			target_resource TEXT,
			committed_at_unix_ms INTEGER NOT NULL,
			auditor_key_id TEXT,
			signature TEXT,
			hash TEXT NOT NULL,
			attestation_json TEXT NOT NULL,
			UNIQUE(hash)
		)
	`)
	require.NoError(t, err)
	cl := storage.NewCommitmentLedger(db, testutil.NewTestLogger())
	t.Cleanup(func() { db.Close() })
	return cl, db
}

// insertCommitment inserts a commitment row directly via SQL for testing.
func insertCommitment(t *testing.T, cl *storage.CommitmentLedger, txID, txHash, priorHash, hash, actionType, targetResource string) {
	t.Helper()
	attestation := map[string]any{
		"transaction_id":                   txID,
		"transaction_hash":                 txHash,
		"prior_commitment_hash":            priorHash,
		"hash":                             hash,
		"action_type":                      actionType,
		"target_resource":                  targetResource,
		"committed_at_unix_ms":             time.Now().UnixMilli(),
		"auditor_key_id":                   "test-auditor",
		"signature":                        "test-sig",
		"state_root_at_commit":             "",
		"l2_signature_digest":              "",
		"Actuator_intent_signature_digest": "",
		"human_signature_digest":           "",
	}
	attJSON, err := json.Marshal(attestation)
	require.NoError(t, err)
	err = cl.AppendCommitmentJSON(attJSON, priorHash, hash)
	require.NoError(t, err)
}

// insertCommitmentDirect inserts a commitment row directly via SQL, bypassing
// chain integrity checks. Used for testing broken chain scenarios.
func insertCommitmentDirect(t *testing.T, db *sqliteutil.DB, txID, txHash, priorHash, hash string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO commitment_ledger (
			transaction_id, transaction_hash, prior_commitment_hash, hash,
			attestation_json, committed_at_unix_ms
		) VALUES (?, ?, ?, ?, ?, ?)
	`, txID, txHash, priorHash, hash, `{}`, time.Now().UnixMilli())
	require.NoError(t, err)
}

// createSessionAndEvent creates a session and an event, returning the event ID.
// Needed because file_mutations and receipts have FK constraints on events/sessions.
func createSessionAndEvent(t *testing.T, store *storage.SQLAuditStore) int64 {
	t.Helper()
	sessionID := "test-session-verify"
	err := store.CreateSession(sessionID, "operator", "Test Session", "user@test.com")
	require.NoError(t, err)
	eventID, err := store.RecordEvent(&storage.Event{
		OperatorSessionID: sessionID,
		Timestamp:         time.Now().UTC(),
		Type:              "COMMAND_EXECUTION",
		CommandRaw:        "echo test",
	})
	require.NoError(t, err)
	return eventID
}

// findRow finds a verification row by check name.
func findRow(vr VerificationResult, check string) *VerificationRow {
	for i := range vr.Rows {
		if vr.Rows[i].Check == check {
			return &vr.Rows[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Check 1: Commitment chain integrity
// ---------------------------------------------------------------------------

func TestReportVerification_EmptyLedger_AllPass(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)

	fileRes, vr, err := reportVerification(context.Background(), outDir, nil, cl, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, vr.FailCount)
	assert.True(t, fileRes.RowCount > 0)

	chainRow := findRow(vr, "commitment_chain")
	require.NotNil(t, chainRow)
	assert.Equal(t, verifyResultPass, chainRow.Result)
	assert.Contains(t, chainRow.Detail, "ledger empty")
}

func TestReportVerification_ValidCommitmentChain_AllPass(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)

	insertCommitment(t, cl, "tx-1", "hash-1", "", "commit-hash-1", "FS_WRITE", "/file1")
	insertCommitment(t, cl, "tx-2", "hash-2", "commit-hash-1", "commit-hash-2", "FS_WRITE", "/file2")

	_, vr, err := reportVerification(context.Background(), outDir, nil, cl, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, vr.FailCount)

	chainRow := findRow(vr, "commitment_chain")
	require.NotNil(t, chainRow)
	assert.Equal(t, verifyResultPass, chainRow.Result)
	assert.Contains(t, chainRow.Detail, "2 commitments verified")
}

func TestReportVerification_FirstCommitmentHasNonEmptyPriorHash_Fails(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)

	insertCommitment(t, cl, "tx-1", "hash-1", "should-be-empty", "commit-hash-1", "FS_WRITE", "/file1")

	_, vr, err := reportVerification(context.Background(), outDir, nil, cl, nil)
	require.NoError(t, err)
	assert.Greater(t, vr.FailCount, 0)

	chainRow := findRow(vr, "commitment_chain")
	require.NotNil(t, chainRow)
	assert.Equal(t, verifyResultFail, chainRow.Result)
	assert.Contains(t, chainRow.Detail, "non-empty prior_commitment_hash")
}

func TestReportVerification_BrokenChainSecondCommit_Fails(t *testing.T) {
	outDir := t.TempDir()
	cl, db := setupTestCommitmentLedger(t)

	insertCommitment(t, cl, "tx-1", "hash-1", "", "commit-hash-1", "FS_WRITE", "/file1")
	// Insert directly to bypass chain integrity enforcement
	insertCommitmentDirect(t, db, "tx-2", "hash-2", "wrong-prior-hash", "commit-hash-2")

	_, vr, err := reportVerification(context.Background(), outDir, nil, cl, nil)
	require.NoError(t, err)
	assert.Greater(t, vr.FailCount, 0)

	chainRow := findRow(vr, "commitment_chain")
	require.NotNil(t, chainRow)
	assert.Equal(t, verifyResultFail, chainRow.Result)
	assert.Contains(t, chainRow.Detail, "prior_commitment_hash")
}

// ---------------------------------------------------------------------------
// Check 3: Git merkle root cross-check
// ---------------------------------------------------------------------------

func TestReportVerification_NilLedger_MerkleRootSkipped(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)

	_, vr, err := reportVerification(context.Background(), outDir, nil, cl, nil)
	require.NoError(t, err)

	merkleRow := findRow(vr, "git_merkle_root")
	require.NotNil(t, merkleRow)
	assert.Equal(t, verifyResultSkipped, merkleRow.Result)
	assert.Contains(t, merkleRow.Detail, "ledger not configured")
}

// ---------------------------------------------------------------------------
// Check 4: File mutation linkage
// ---------------------------------------------------------------------------

func TestReportVerification_NilAuditStore_MutationLinkageSkipped(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)

	_, vr, err := reportVerification(context.Background(), outDir, nil, cl, nil)
	require.NoError(t, err)

	mutationRow := findRow(vr, "file_mutation_linkage")
	require.NotNil(t, mutationRow)
	assert.Equal(t, verifyResultSkipped, mutationRow.Result)
	assert.Contains(t, mutationRow.Detail, "audit store not configured")
}

func TestReportVerification_MutationsAllHaveHashes_Pass(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)
	auditStore := setupTestAuditStore(t)
	eventID := createSessionAndEvent(t, auditStore)

	err := auditStore.RecordFileMutation(&storage.FileMutationLog{
		EventID:          eventID,
		Filepath:         "/test/file1",
		Operation:        storage.FileMutationWrite,
		LedgerHashBefore: "hash-before-1",
		LedgerHashAfter:  "hash-after-1",
	})
	require.NoError(t, err)

	err = auditStore.RecordFileMutation(&storage.FileMutationLog{
		EventID:          eventID,
		Filepath:         "/test/file2",
		Operation:        storage.FileMutationCreate,
		LedgerHashBefore: "hash-before-2",
		LedgerHashAfter:  "hash-after-2",
	})
	require.NoError(t, err)

	_, vr, err := reportVerification(context.Background(), outDir, auditStore, cl, nil)
	require.NoError(t, err)

	mutationRow := findRow(vr, "file_mutation_linkage")
	require.NotNil(t, mutationRow)
	assert.Equal(t, verifyResultPass, mutationRow.Result)
	assert.Contains(t, mutationRow.Detail, "2 write/create ops with hashes")
}

func TestReportVerification_MutationMissingLedgerHashAfter_Fails(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)
	auditStore := setupTestAuditStore(t)
	eventID := createSessionAndEvent(t, auditStore)

	err := auditStore.RecordFileMutation(&storage.FileMutationLog{
		EventID:          eventID,
		Filepath:         "/test/file1",
		Operation:        storage.FileMutationWrite,
		LedgerHashBefore: "hash-before-1",
		LedgerHashAfter:  "",
	})
	require.NoError(t, err)

	_, vr, err := reportVerification(context.Background(), outDir, auditStore, cl, nil)
	require.NoError(t, err)
	assert.Greater(t, vr.FailCount, 0)

	mutationRow := findRow(vr, "file_mutation_linkage")
	require.NotNil(t, mutationRow)
	assert.Equal(t, verifyResultFail, mutationRow.Result)
	assert.Contains(t, mutationRow.Detail, "missing ledger_hash_after")
}

func TestReportVerification_ReadOperationDoesNotRequireHash(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)
	auditStore := setupTestAuditStore(t)
	eventID := createSessionAndEvent(t, auditStore)

	err := auditStore.RecordFileMutation(&storage.FileMutationLog{
		EventID:          eventID,
		Filepath:         "/test/file1",
		Operation:        "READ",
		LedgerHashBefore: "hash-before-1",
		LedgerHashAfter:  "",
	})
	require.NoError(t, err)

	_, vr, err := reportVerification(context.Background(), outDir, auditStore, cl, nil)
	require.NoError(t, err)

	mutationRow := findRow(vr, "file_mutation_linkage")
	require.NotNil(t, mutationRow)
	assert.Equal(t, verifyResultPass, mutationRow.Result)
}

// ---------------------------------------------------------------------------
// Check 5: Receipt/commitment cross-link
// ---------------------------------------------------------------------------

func TestReportVerification_AllReceiptsPresent_Pass(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)
	auditStore := setupTestAuditStore(t)
	sessionID := "test-session-receipts"
	err := auditStore.CreateSession(sessionID, "operator", "Test Session", "user@test.com")
	require.NoError(t, err)

	insertCommitment(t, cl, "tx-1", "hash-1", "", "commit-hash-1", "FS_WRITE", "/file1")

	err = auditStore.RecordActionReceipt(&models.ActionReceiptRecord{
		TransactionID:     "tx-1",
		TransactionHash:   "hash-1",
		OperatorID:        "op-1",
		OperatorSessionID: sessionID,
		ActionType:        constants.ActionTypeFileEdit,
		TargetResource:    "/file1",
		ExecutedAt:        time.Now().UTC(),
		SignerKeyID:       "key-1",
		Signature:         "sig-1",
	})
	require.NoError(t, err)

	_, vr, err := reportVerification(context.Background(), outDir, auditStore, cl, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, vr.FailCount)

	crossRow := findRow(vr, "receipt_commitment_crosslink")
	require.NotNil(t, crossRow)
	assert.Equal(t, verifyResultPass, crossRow.Result)
	assert.Contains(t, crossRow.Detail, "all 1 commitments have matching receipts")
}

func TestReportVerification_MissingReceipt_Fails(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)
	auditStore := setupTestAuditStore(t)
	sessionID := "test-session-missing-receipt"
	err := auditStore.CreateSession(sessionID, "operator", "Test Session", "user@test.com")
	require.NoError(t, err)

	insertCommitment(t, cl, "tx-1", "hash-1", "", "commit-hash-1", "FS_WRITE", "/file1")
	insertCommitment(t, cl, "tx-2", "hash-2", "commit-hash-1", "commit-hash-2", "FS_WRITE", "/file2")

	err = auditStore.RecordActionReceipt(&models.ActionReceiptRecord{
		TransactionID:     "tx-1",
		TransactionHash:   "hash-1",
		OperatorID:        "op-1",
		OperatorSessionID: sessionID,
		ActionType:        constants.ActionTypeFileEdit,
		TargetResource:    "/file1",
		ExecutedAt:        time.Now().UTC(),
		SignerKeyID:       "key-1",
		Signature:         "sig-1",
	})
	require.NoError(t, err)

	_, vr, err := reportVerification(context.Background(), outDir, auditStore, cl, nil)
	require.NoError(t, err)
	assert.Greater(t, vr.FailCount, 0)

	crossRow := findRow(vr, "receipt_commitment_crosslink")
	require.NotNil(t, crossRow)
	assert.Equal(t, verifyResultFail, crossRow.Result)
	assert.Contains(t, crossRow.Detail, "1 committed transaction_ids have no matching receipt")
}

func TestReportVerification_EmptyCommitments_NoCrossLinkNeeded(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)
	auditStore := setupTestAuditStore(t)

	_, vr, err := reportVerification(context.Background(), outDir, auditStore, cl, nil)
	require.NoError(t, err)

	// When the ledger is empty, ListCommitments returns nil, so the
	// receipt_commitment_crosslink check is skipped (auditStore != nil but commitments == nil).
	// The check only runs when both are non-nil.
	crossRow := findRow(vr, "receipt_commitment_crosslink")
	assert.Nil(t, crossRow, "cross-link check should be skipped when commitments is nil")
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestReportVerification_ContextCancelled_ReturnsErr(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := reportVerification(ctx, outDir, nil, cl, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// CSV output verification
// ---------------------------------------------------------------------------

func TestReportVerification_CSVFileWritten(t *testing.T) {
	outDir := t.TempDir()
	cl, _ := setupTestCommitmentLedger(t)

	fileRes, vr, err := reportVerification(context.Background(), outDir, nil, cl, nil)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportVerificationFilename, fileRes.Filename)
	assert.NotEmpty(t, fileRes.SHA256)
	assert.Equal(t, len(vr.Rows), fileRes.RowCount)

	csvPath := filepath.Join(outDir, constants.ReportVerificationFilename)
	info, err := os.Stat(csvPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// ---------------------------------------------------------------------------
// Commitment ledger read error
// ---------------------------------------------------------------------------

func TestReportVerification_CommitmentLedgerReadError_Skipped(t *testing.T) {
	outDir := t.TempDir()

	// Create a commitment ledger with a nil db to trigger read error
	cl := storage.NewCommitmentLedger(nil, testutil.NewTestLogger())

	_, vr, err := reportVerification(context.Background(), outDir, nil, cl, nil)
	require.NoError(t, err)

	chainRow := findRow(vr, "commitment_chain")
	require.NotNil(t, chainRow)
	assert.Equal(t, verifyResultSkipped, chainRow.Result)
	assert.Contains(t, chainRow.Detail, "could not read commitments")
}
