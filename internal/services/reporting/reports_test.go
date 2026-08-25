// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package reporting

import (
	"context"
	"crypto/ed25519"
	"encoding/csv"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// reportReceipts
// ---------------------------------------------------------------------------

func TestReportReceipts_EmptyStore(t *testing.T) {
	store := setupTestAuditStore(t)
	outDir := testutil.TempDir(t)

	res, err := reportReceipts(context.Background(), outDir, store)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportReceiptsFilename, res.Filename)
	assert.Equal(t, 0, res.RowCount)
	assert.NotEmpty(t, res.SHA256)

	csvPath := filepath.Join(outDir, constants.ReportReceiptsFilename)
	f, err := os.Open(csvPath)
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 1, "header only")
	assert.Equal(t, ReceiptRow{}.Columns(), records[0])
}

func TestReportReceipts_WithRecords(t *testing.T) {
	store := setupTestAuditStore(t)
	sessionID := "test-session-receipts"
	require.NoError(t, store.CreateSession(sessionID, "operator", "Test", "user@test.com"))

	now := time.Now().UTC()
	require.NoError(t, store.RecordActionReceipt(&models.ActionReceiptRecord{
		TransactionID:     "tx-1",
		TransactionHash:   "hash-1",
		OperatorID:        "op-1",
		OperatorSessionID: sessionID,
		ActionType:        constants.ActionTypeFileEdit,
		TargetResource:    "/file1",
		Status:            2,
		ExecutedAt:        now,
		SignerKeyID:       "key-1",
		Signature:         "sig-1",
	}))
	require.NoError(t, store.RecordActionReceipt(&models.ActionReceiptRecord{
		TransactionID:     "tx-2",
		TransactionHash:   "hash-2",
		OperatorID:        "op-2",
		OperatorSessionID: sessionID,
		ActionType:        constants.ActionTypeFsRead,
		TargetResource:    "/file2",
		Status:            2,
		ExecutedAt:        now.Add(time.Second),
		SignerKeyID:       "key-2",
		Signature:         "sig-2",
	}))

	outDir := testutil.TempDir(t)
	res, err := reportReceipts(context.Background(), outDir, store)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RowCount)

	f, err := os.Open(filepath.Join(outDir, constants.ReportReceiptsFilename))
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3, "header + 2 rows")
	txIDs := []string{records[1][0], records[2][0]}
	assert.ElementsMatch(t, []string{"tx-1", "tx-2"}, txIDs)
}

func TestReportReceipts_CancelledContext(t *testing.T) {
	store := setupTestAuditStore(t)
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportReceipts(ctx, outDir, store)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// reportSessions
// ---------------------------------------------------------------------------

func TestReportSessions_EmptyStore(t *testing.T) {
	store := setupTestAuditStore(t)
	outDir := testutil.TempDir(t)

	res, err := reportSessions(context.Background(), outDir, store)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportSessionsFilename, res.Filename)
	assert.Equal(t, 0, res.RowCount)
}

func TestReportSessions_WithRecords(t *testing.T) {
	store := setupTestAuditStore(t)
	require.NoError(t, store.CreateSession("sess-1", "operator", "Session 1", "user1@test.com"))
	require.NoError(t, store.CreateSession("sess-2", "operator", "Session 2", "user2@test.com"))

	outDir := testutil.TempDir(t)
	res, err := reportSessions(context.Background(), outDir, store)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RowCount)

	f, err := os.Open(filepath.Join(outDir, constants.ReportSessionsFilename))
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3, "header + 2 rows")
	assert.Equal(t, "sess-1", records[1][0])
	assert.Equal(t, "sess-2", records[2][0])
}

func TestReportSessions_CancelledContext(t *testing.T) {
	store := setupTestAuditStore(t)
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportSessions(ctx, outDir, store)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// reportEvents
// ---------------------------------------------------------------------------

func TestReportEvents_EmptyStore(t *testing.T) {
	store := setupTestAuditStore(t)
	outDir := testutil.TempDir(t)

	res, err := reportEvents(context.Background(), outDir, store)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportEventsFilename, res.Filename)
	assert.Equal(t, 0, res.RowCount)
}

func TestReportEvents_WithRecords(t *testing.T) {
	store := setupTestAuditStore(t)
	sessionID := "test-session-events"
	require.NoError(t, store.CreateSession(sessionID, "operator", "Test", "user@test.com"))

	_, err := store.RecordEvent(&storage.Event{
		OperatorSessionID: sessionID,
		Timestamp:         time.Now().UTC(),
		Type:              "COMMAND_EXECUTION",
		CommandRaw:        "echo hello",
		ContentText:       "hello\n",
	})
	require.NoError(t, err)

	_, err = store.RecordEvent(&storage.Event{
		OperatorSessionID: sessionID,
		Timestamp:         time.Now().UTC(),
		Type:              "COMMAND_EXECUTION",
		CommandRaw:        "ls -la",
		ContentText:       "",
	})
	require.NoError(t, err)

	outDir := testutil.TempDir(t)
	res, err := reportEvents(context.Background(), outDir, store)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RowCount)

	f, err := os.Open(filepath.Join(outDir, constants.ReportEventsFilename))
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3, "header + 2 rows")
}

func TestReportEvents_CancelledContext(t *testing.T) {
	store := setupTestAuditStore(t)
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportEvents(ctx, outDir, store)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// reportFileMutations
// ---------------------------------------------------------------------------

func TestReportFileMutations_EmptyStore(t *testing.T) {
	store := setupTestAuditStore(t)
	outDir := testutil.TempDir(t)

	res, err := reportFileMutations(context.Background(), outDir, store)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportFileMutationsFilename, res.Filename)
	assert.Equal(t, 0, res.RowCount)
}

func TestReportFileMutations_WithRecords(t *testing.T) {
	store := setupTestAuditStore(t)
	eventID := createSessionAndEvent(t, store)

	require.NoError(t, store.RecordFileMutation(&storage.FileMutationLog{
		EventID:          eventID,
		Filepath:         "/test/file1",
		Operation:        storage.FileMutationWrite,
		LedgerHashBefore: "hash-before-1",
		LedgerHashAfter:  "hash-after-1",
	}))
	require.NoError(t, store.RecordFileMutation(&storage.FileMutationLog{
		EventID:          eventID,
		Filepath:         "/test/file2",
		Operation:        storage.FileMutationCreate,
		LedgerHashBefore: "",
		LedgerHashAfter:  "hash-after-2",
	}))

	outDir := testutil.TempDir(t)
	res, err := reportFileMutations(context.Background(), outDir, store)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RowCount)

	f, err := os.Open(filepath.Join(outDir, constants.ReportFileMutationsFilename))
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3, "header + 2 rows")
	assert.Equal(t, "/test/file1", records[1][2])
	assert.Equal(t, "/test/file2", records[2][2])
}

func TestReportFileMutations_CancelledContext(t *testing.T) {
	store := setupTestAuditStore(t)
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportFileMutations(ctx, outDir, store)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// reportCommitments
// ---------------------------------------------------------------------------

func TestReportCommitments_EmptyLedger(t *testing.T) {
	cl, _ := setupTestCommitmentLedger(t)
	outDir := testutil.TempDir(t)

	res, err := reportCommitments(context.Background(), outDir, cl)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportCommitmentsFilename, res.Filename)
	assert.Equal(t, 0, res.RowCount)
}

func TestReportCommitments_WithRecords(t *testing.T) {
	cl, _ := setupTestCommitmentLedger(t)
	insertCommitment(t, cl, "tx-1", "hash-1", "", "commit-hash-1", "FS_WRITE", "/file1")
	insertCommitment(t, cl, "tx-2", "hash-2", "commit-hash-1", "commit-hash-2", "FS_WRITE", "/file2")

	outDir := testutil.TempDir(t)
	res, err := reportCommitments(context.Background(), outDir, cl)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RowCount)

	f, err := os.Open(filepath.Join(outDir, constants.ReportCommitmentsFilename))
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3, "header + 2 rows")
	txIDs := []string{records[1][2], records[2][2]}
	assert.ElementsMatch(t, []string{"tx-1", "tx-2"}, txIDs)
}

func TestReportCommitments_CancelledContext(t *testing.T) {
	cl, _ := setupTestCommitmentLedger(t)
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportCommitments(ctx, outDir, cl)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// reportReplayNonces
// ---------------------------------------------------------------------------

func TestReportReplayNonces_EmptyStore(t *testing.T) {
	tempDir := testutil.TempDir(t)
	dbPath := filepath.Join(tempDir, "test_replay.db")
	rs, err := storage.NewSQLReplayStore(&storage.ReplayStoreConfig{DBPath: dbPath}, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { rs.Close() })

	outDir := testutil.TempDir(t)
	res, err := reportReplayNonces(context.Background(), outDir, rs)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportReplayNoncesFilename, res.Filename)
	assert.Equal(t, 0, res.RowCount)
}

func TestReportReplayNonces_WithRecords(t *testing.T) {
	tempDir := testutil.TempDir(t)
	dbPath := filepath.Join(tempDir, "test_replay.db")
	rs, err := storage.NewSQLReplayStore(&storage.ReplayStoreConfig{DBPath: dbPath}, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { rs.Close() })

	expiresAt := time.Now().UTC().Add(time.Hour)
	_, err = rs.ReserveNonce("nonce-1", expiresAt)
	require.NoError(t, err)
	_, err = rs.ReserveNonce("nonce-2", expiresAt)
	require.NoError(t, err)

	outDir := testutil.TempDir(t)
	res, err := reportReplayNonces(context.Background(), outDir, rs)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RowCount)

	f, err := os.Open(filepath.Join(outDir, constants.ReportReplayNoncesFilename))
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3, "header + 2 rows")
}

func TestReportReplayNonces_CancelledContext(t *testing.T) {
	tempDir := testutil.TempDir(t)
	dbPath := filepath.Join(tempDir, "test_replay.db")
	rs, err := storage.NewSQLReplayStore(&storage.ReplayStoreConfig{DBPath: dbPath}, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { rs.Close() })

	outDir := testutil.TempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = reportReplayNonces(ctx, outDir, rs)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// reportSuspendedTransactions
// ---------------------------------------------------------------------------

func TestReportSuspendedTransactions_EmptyStore(t *testing.T) {
	tempDir := testutil.TempDir(t)
	dbPath := filepath.Join(tempDir, "test_suspended.db")
	sts, err := storage.NewSuspendedTransactionService(&storage.SuspendedTransactionConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { sts.Close() })

	outDir := testutil.TempDir(t)
	res, err := reportSuspendedTransactions(context.Background(), outDir, sts)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportSuspendedTxFilename, res.Filename)
	assert.Equal(t, 0, res.RowCount)
}

func TestReportSuspendedTransactions_WithRecords(t *testing.T) {
	tempDir := testutil.TempDir(t)
	dbPath := filepath.Join(tempDir, "test_suspended.db")
	sts, err := storage.NewSuspendedTransactionService(&storage.SuspendedTransactionConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { sts.Close() })

	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, sts.StoreSuspendedTransaction(ctx, &models.SuspendedTransaction{
		TransactionHash: "suspend-hash-1",
		Envelope:        []byte("env-1"),
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Hour),
		UserID:          "user-1",
		OperatorID:      "op-1",
		ToolName:        "shell_exec",
	}))

	require.NoError(t, sts.StoreSuspendedTransaction(ctx, &models.SuspendedTransaction{
		TransactionHash:   "suspend-hash-2",
		Envelope:          []byte("env-2"),
		CreatedAt:         now,
		ExpiresAt:         now.Add(time.Hour),
		UserID:            "user-2",
		OperatorID:        "op-1",
		ToolName:          "fs_write",
		Approved:          true,
		ApprovedBy:        "approver-1",
		ApprovalSignature: "sig-1",
	}))

	outDir := testutil.TempDir(t)
	res, err := reportSuspendedTransactions(ctx, outDir, sts)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RowCount)

	f, err := os.Open(filepath.Join(outDir, constants.ReportSuspendedTxFilename))
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3, "header + 2 rows")
	txHashes := []string{records[1][0], records[2][0]}
	assert.ElementsMatch(t, []string{"suspend-hash-1", "suspend-hash-2"}, txHashes)
	statuses := []string{records[1][4], records[2][4]}
	assert.ElementsMatch(t, []string{"pending", "approved"}, statuses)
}

func TestReportSuspendedTransactions_CancelledContext(t *testing.T) {
	tempDir := testutil.TempDir(t)
	dbPath := filepath.Join(tempDir, "test_suspended.db")
	sts, err := storage.NewSuspendedTransactionService(&storage.SuspendedTransactionConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { sts.Close() })

	outDir := testutil.TempDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = reportSuspendedTransactions(ctx, outDir, sts)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// reportLedgerMerkleRoot & reportLedgerCommits
// ---------------------------------------------------------------------------

func TestReportLedgerMerkleRoot_NilLedger(t *testing.T) {
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportLedgerMerkleRoot(ctx, outDir, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestReportLedgerCommits_NilLedger(t *testing.T) {
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportLedgerCommits(ctx, outDir, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// openVault
// ---------------------------------------------------------------------------

func TestOpenVault_NoKeyPath_ReturnsLockedVault(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	header, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(vaultDir))

	v, unlocked := openVault(vaultDir, "", testutil.NewTestLogger())
	assert.NotNil(t, v)
	assert.False(t, unlocked)
	t.Cleanup(func() { v.Close() })
}

func TestOpenVault_KeyFileNotFound(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	header, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(vaultDir))

	v, unlocked := openVault(vaultDir, filepath.Join(tempDir, "nonexistent.key"), testutil.NewTestLogger())
	assert.NotNil(t, v)
	assert.False(t, unlocked)
	t.Cleanup(func() { v.Close() })
}

func TestOpenVault_InvalidKeyEncoding(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	header, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(vaultDir))

	keyPath := filepath.Join(tempDir, "bad.key")
	require.NoError(t, os.WriteFile(keyPath, []byte("not-valid-hex\n"), 0600))

	v, unlocked := openVault(vaultDir, keyPath, testutil.NewTestLogger())
	assert.NotNil(t, v)
	assert.False(t, unlocked)
	t.Cleanup(func() { v.Close() })
}

func TestOpenVault_ValidKey(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	header, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, header.Save(vaultDir))

	keyHex := hex.EncodeToString(privKey)
	keyPath := filepath.Join(tempDir, "vault.key")
	require.NoError(t, os.WriteFile(keyPath, []byte(keyHex), 0600))

	v, unlocked := openVault(vaultDir, keyPath, testutil.NewTestLogger())
	assert.NotNil(t, v)
	assert.True(t, unlocked)
	t.Cleanup(func() { v.Close() })
}

// ---------------------------------------------------------------------------
// recordTypeForFile (additional edge cases)
// ---------------------------------------------------------------------------

func TestRecordTypeForFile_EdgeCases(t *testing.T) {
	assert.Equal(t, "manifest", recordTypeForFile("manifest.csv"))
	assert.Equal(t, "suspended_transactions", recordTypeForFile("/a/b/suspended_transactions.csv"))
}
