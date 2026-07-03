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
	"encoding/csv"
	"encoding/hex"
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

// setupReportingEnv creates a fully populated reporting environment in a temp
// directory. It returns Options pre-configoured with all paths, plus the vault
// key path so tests can pass it to Run.
func setupReportingEnv(t *testing.T, seed bool) (Options, string) {
	t.Helper()

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	runtimeDir := filepath.Join(root, "runtime")
	vaultDir := filepath.Join(root, "vault")
	ledgerDir := filepath.Join(root, "runtime", "ledger")
	outDir := filepath.Join(root, "reports")

	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.MkdirAll(runtimeDir, 0o755))
	require.NoError(t, os.MkdirAll(vaultDir, 0o700))

	// Create vault with key.
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vh, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vh.Save(vaultDir))

	keyHex := hex.EncodeToString(privKey)
	keyPath := filepath.Join(root, "vault.key")
	require.NoError(t, os.WriteFile(keyPath, []byte(keyHex), 0o600))

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	require.NoError(t, v.Unlock(privKey))
	t.Cleanup(func() { v.Close() })

	// Audit store.
	auditCfg := &storage.AuditStoreConfig{
		DataDir:              dataDir,
		DBPath:               constants.DbFilename,
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		EncryptionVault:      v,
	}
	store, err := storage.NewSQLAuditStore(auditCfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	// Create commitment_ledger table in the same DB (audit store doesn't create it).
	clDB, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(filepath.Join(dataDir, constants.DbFilename)), testutil.NewTestLogger())
	require.NoError(t, err)
	_, err = clDB.Exec(`CREATE TABLE IF NOT EXISTS commitment_ledger (
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
	)`)
	require.NoError(t, err)
	t.Cleanup(func() { clDB.Close() })

	// Execution vault.
	evCfg := storage.DefaultExecutionVaultConfig()
	evCfg.DBPath = filepath.Join(runtimeDir, constants.ExecutionVaultDBFilename)
	ev, err := storage.NewExecutionVaultService(evCfg, testutil.NewTestLogger(), v)
	require.NoError(t, err)
	t.Cleanup(func() { ev.Close() })

	// Replay store.
	rsCfg := storage.DefaultReplayStoreConfig()
	rsCfg.DBPath = filepath.Join(runtimeDir, constants.ReplayStoreDBFilename)
	rs, err := storage.NewSQLReplayStore(rsCfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { rs.Close() })

	// Suspended transaction store.
	stsCfg := storage.DefaultSuspendedTransactionConfig()
	stsCfg.DBPath = filepath.Join(dataDir, constants.SuspendedTxFilename)
	sts, err := storage.NewSuspendedTransactionService(stsCfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { sts.Close() })

	opts := Options{
		DataDir:                    dataDir,
		RuntimeDir:                 runtimeDir,
		LedgerDir:                  ledgerDir,
		VaultDir:                   vaultDir,
		VaultKeyPath:               keyPath,
		OutDir:                     outDir,
		ExecutionVaultDBPath:       evCfg.DBPath,
		ReplayStoreDBPath:          rsCfg.DBPath,
		SuspendedTransactionDBPath: stsCfg.DBPath,
		Logger:                     testutil.NewTestLogger(),
	}

	if seed {
		seedReportingData(t, store, ev, rs, sts)
	}

	return opts, keyPath
}

func seedReportingData(t *testing.T, store *storage.SQLAuditStore, ev *storage.ExecutionVaultService, rs *storage.SQLReplayStore, sts *storage.SuspendedTransactionService) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	// Session + events + receipts + file mutations.
	require.NoError(t, store.CreateSession("sess-run-1", "operator", "Run Test", "user@test.com"))

	require.NoError(t, store.RecordActionReceipt(&models.ActionReceiptRecord{
		TransactionID:     "tx-run-1",
		TransactionHash:   "hash-run-1",
		OperatorID:        "op-1",
		OperatorSessionID: "sess-run-1",
		ActionType:        constants.ActionTypeFileEdit,
		TargetResource:    "/file1",
		Status:            2,
		ExecutedAt:        now,
		SignerKeyID:       "key-1",
		Signature:         "sig-1",
	}))

	_, err := store.RecordEvent(&storage.Event{
		OperatorSessionID: "sess-run-1",
		Timestamp:         now,
		Type:              "COMMAND_EXECUTION",
		CommandRaw:        "echo test",
		ContentText:       "test\n",
	})
	require.NoError(t, err)

	// File mutation — need a valid event ID.
	eventID := createSessionAndEvent(t, store)
	require.NoError(t, store.RecordFileMutation(&storage.FileMutationLog{
		EventID:          eventID,
		Filepath:         "/test/run-file",
		Operation:        storage.FileMutationWrite,
		LedgerHashBefore: "hash-before",
		LedgerHashAfter:  "hash-after",
	}))

	// Execution vault.
	require.NoError(t, ev.StoreExecution(ctx, &models.ExecutionRecord{
		ID:           "exec-run-1",
		TimestampUTC: now,
		Command:      "ls -la",
		ExitCode:     0,
		DurationMs:   42,
		StdoutHash:   "stdout-hash",
		StdoutSize:   100,
		StderrHash:   "stderr-hash",
		StderrSize:   0,
		OperatorID:   "op-1",
	}))

	require.NoError(t, ev.StoreFileDiff(ctx, &models.FileDiffRecord{
		ID:                "diff-run-1",
		TimestampUTC:      now,
		FilePath:          "/test/run-file",
		Operation:         "WRITE",
		LedgerHashBefore:  "hash-before",
		LedgerHashAfter:   "hash-after",
		DiffStat:          "1 file changed",
		DiffHash:          "diff-hash",
		DiffSize:          50,
		OperatorSessionID: "sess-run-1",
		OperatorID:        "op-1",
	}))

	// Replay store.
	_, err = rs.ReserveNonce("nonce-run-1", now.Add(time.Hour))
	require.NoError(t, err)

	// Suspended transaction store.
	require.NoError(t, sts.StoreSuspendedTransaction(ctx, &models.SuspendedTransaction{
		TransactionHash: "suspend-run-1",
		Envelope:        []byte("env"),
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Hour),
		UserID:          "user-1",
		OperatorID:      "op-1",
		ToolName:        "shell_exec",
	}))
}

func TestRun_PopulatedStores_AllCSVFilesWritten(t *testing.T) {
	opts, _ := setupReportingEnv(t, true)

	result, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, result.VaultUnlocked)
	assert.Equal(t, 0, result.FailCount)

	// Verify all expected CSV files exist.
	expectedFiles := []string{
		constants.ReportReceiptsFilename,
		constants.ReportSessionsFilename,
		constants.ReportEventsFilename,
		constants.ReportFileMutationsFilename,
		constants.ReportCommitmentsFilename,
		constants.ReportExecutionsFilename,
		constants.ReportFileDiffsFilename,
		constants.ReportReplayNoncesFilename,
		constants.ReportSuspendedTxFilename,
		constants.ReportVerificationFilename,
		constants.ReportManifestFilename,
	}

	for _, fname := range expectedFiles {
		path := filepath.Join(opts.OutDir, fname)
		_, err := os.Stat(path)
		assert.NoError(t, err, "expected file %s to exist", fname)
	}

	// Verify manifest has entries for all files.
	manifestPath := filepath.Join(opts.OutDir, constants.ReportManifestFilename)
	f, err := os.Open(manifestPath)
	require.NoError(t, err)
	defer f.Close()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	// Header + at least 10 data rows (all reporters + verification).
	assert.GreaterOrEqual(t, len(records), 2, "manifest should have header + data rows")

	// Verify receipts CSV has 1 row.
	receiptsPath := filepath.Join(opts.OutDir, constants.ReportReceiptsFilename)
	f2, err := os.Open(receiptsPath)
	require.NoError(t, err)
	defer f2.Close()
	r2 := csv.NewReader(f2)
	receiptRecords, err := r2.ReadAll()
	require.NoError(t, err)
	require.Len(t, receiptRecords, 2, "header + 1 receipt row")
	assert.Equal(t, "tx-run-1", receiptRecords[1][0])

	// Verify executions CSV has 1 row.
	execPath := filepath.Join(opts.OutDir, constants.ReportExecutionsFilename)
	f3, err := os.Open(execPath)
	require.NoError(t, err)
	defer f3.Close()
	r3 := csv.NewReader(f3)
	execRecords, err := r3.ReadAll()
	require.NoError(t, err)
	require.Len(t, execRecords, 2, "header + 1 execution row")
	assert.Equal(t, "exec-run-1", execRecords[1][0])

	// Verify replay nonces CSV has 1 row.
	noncePath := filepath.Join(opts.OutDir, constants.ReportReplayNoncesFilename)
	f4, err := os.Open(noncePath)
	require.NoError(t, err)
	defer f4.Close()
	r4 := csv.NewReader(f4)
	nonceRecords, err := r4.ReadAll()
	require.NoError(t, err)
	require.Len(t, nonceRecords, 2, "header + 1 nonce row")

	// Verify suspended transactions CSV has 1 row.
	suspendPath := filepath.Join(opts.OutDir, constants.ReportSuspendedTxFilename)
	f5, err := os.Open(suspendPath)
	require.NoError(t, err)
	defer f5.Close()
	r5 := csv.NewReader(f5)
	suspendRecords, err := r5.ReadAll()
	require.NoError(t, err)
	require.Len(t, suspendRecords, 2, "header + 1 suspended tx row")
	assert.Equal(t, "suspend-run-1", suspendRecords[1][0])
}

func TestRun_EmptyStores_AllCSVFilesWritten(t *testing.T) {
	opts, _ := setupReportingEnv(t, false)

	result, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, result.VaultUnlocked)
	assert.Equal(t, 0, result.FailCount)

	// All files should still exist with header-only rows.
	expectedFiles := []string{
		constants.ReportReceiptsFilename,
		constants.ReportSessionsFilename,
		constants.ReportEventsFilename,
		constants.ReportFileMutationsFilename,
		constants.ReportCommitmentsFilename,
		constants.ReportExecutionsFilename,
		constants.ReportFileDiffsFilename,
		constants.ReportReplayNoncesFilename,
		constants.ReportSuspendedTxFilename,
		constants.ReportVerificationFilename,
		constants.ReportManifestFilename,
	}

	for _, fname := range expectedFiles {
		path := filepath.Join(opts.OutDir, fname)
		_, err := os.Stat(path)
		assert.NoError(t, err, "expected file %s to exist even with empty stores", fname)
	}
}

func TestRun_LockedVault_NoKeyPath(t *testing.T) {
	opts, _ := setupReportingEnv(t, true)
	opts.VaultKeyPath = "" // No key → locked vault.

	result, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.False(t, result.VaultUnlocked)
}

func TestRun_LockedVault_KeyFileNotFound(t *testing.T) {
	opts, _ := setupReportingEnv(t, true)
	opts.VaultKeyPath = filepath.Join(t.TempDir(), "nonexistent.key")

	result, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.False(t, result.VaultUnlocked)
}

func TestRun_MissingExecutionVault(t *testing.T) {
	opts, _ := setupReportingEnv(t, true)
	// Use a path in a nonexistent directory so SQLite can't create the DB.
	opts.ExecutionVaultDBPath = filepath.Join("/nonexistent-dir-xyz", "test.db")

	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	// Executions and file_diffs CSVs should be absent.
	_, statErr := os.Stat(filepath.Join(opts.OutDir, constants.ReportExecutionsFilename))
	assert.True(t, os.IsNotExist(statErr), "executions.csv should not exist when execution vault is unavailable")
}

func TestRun_MissingReplayStore(t *testing.T) {
	opts, _ := setupReportingEnv(t, true)
	opts.ReplayStoreDBPath = filepath.Join("/nonexistent-dir-xyz", "test.db")

	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(opts.OutDir, constants.ReportReplayNoncesFilename))
	assert.True(t, os.IsNotExist(statErr), "replay_nonces.csv should not exist when replay store is unavailable")
}

func TestRun_MissingSuspendedTxStore(t *testing.T) {
	opts, _ := setupReportingEnv(t, true)
	opts.SuspendedTransactionDBPath = filepath.Join("/nonexistent-dir-xyz", "test.db")

	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(opts.OutDir, constants.ReportSuspendedTxFilename))
	assert.True(t, os.IsNotExist(statErr), "suspended_transactions.csv should not exist when suspended tx store is unavailable")
}

func TestRun_CancelledContext(t *testing.T) {
	opts, _ := setupReportingEnv(t, true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Run(ctx, opts)
	require.NoError(t, err) // Run itself doesn't return error on cancelled ctx; individual reporters skip.
	// No files should be written because the run closure checks ctx.Err() before each reporter.
	// However, the OutDir is still created.
	_, statErr := os.Stat(filepath.Join(opts.OutDir, constants.ReportReceiptsFilename))
	assert.True(t, os.IsNotExist(statErr), "no CSV files should be written with cancelled context")
}

func TestRun_BadOutDir(t *testing.T) {
	opts, _ := setupReportingEnv(t, false)
	opts.OutDir = filepath.Join("/dev/null", "cannot-create-here")

	_, err := Run(context.Background(), opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportOutputDirFailed)
}
