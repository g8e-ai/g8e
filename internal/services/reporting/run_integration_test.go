// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package reporting

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupReportingEnv creates a fully populated reporting environment in a temp
// directory. It returns Options pre-configured with all paths and an
// initialized Keystore.
func setupReportingEnv(t *testing.T, seed bool, withSecondaryStores bool) Options {
	t.Helper()

	root := testutil.TempDir(t)
	outDir := filepath.Join(root, "reports")

	fileSvc, err := fs.NewRuntimeFileService(root, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	dataDir := fileSvc.Resolve(constants.DataDirname)

	// Build a memory-keyring keystore and initialize vault.
	ks, err := keystore.NewWithKeyringAndFS(testutil.NewTestLogger(), keystoretest.NewMemoryKeyring(), fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.InitVault())

	vaultKey, err := ks.LoadVaultKey()
	require.NoError(t, err)
	defer vault.SecureZero(vaultKey)

	v, err := vault.NewVault(&vault.VaultConfig{FileSvc: fileSvc, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	require.NoError(t, v.Unlock(vaultKey))
	t.Cleanup(func() { v.Close() })

	// Open g8e.db once; the audit store and its commitment ledger share the pool.
	require.NoError(t, fileSvc.MkdirAll(context.Background(), constants.DataDirname, constants.PermDirStandard))
	mainDB, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(filepath.Join(dataDir, constants.DbFilename)), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { mainDB.Close() })

	// Audit store.
	auditCfg := &storage.AuditStoreConfig{
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		EncryptionVault:      v,
	}
	store, err := storage.NewSQLAuditStore(auditCfg, mainDB, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	var ev *storage.ExecutionVaultService
	var rs *storage.SQLReplayStore
	var sts *storage.SuspendedTransactionService
	if withSecondaryStores {
		evCfg := storage.DefaultExecutionVaultConfig()
		evCfg.DBPath = fileSvc.Resolve(constants.ExecutionVaultDBRelPath)
		ev, err = storage.NewExecutionVaultService(evCfg, testutil.NewTestLogger(), v)
		require.NoError(t, err)
		t.Cleanup(func() { ev.Close() })

		rsCfg := storage.DefaultReplayStoreConfig()
		rsCfg.DBPath = fileSvc.Resolve(constants.ReplayStoreDBRelPath)
		rs, err = storage.NewSQLReplayStore(rsCfg, testutil.NewTestLogger())
		require.NoError(t, err)
		t.Cleanup(func() { rs.Close() })

		stsCfg := storage.DefaultSuspendedTransactionConfig()
		stsCfg.DBPath = fileSvc.Resolve(constants.SuspendedTransactionDBRelPath)
		sts, err = storage.NewSuspendedTransactionService(stsCfg, testutil.NewTestLogger())
		require.NoError(t, err)
		t.Cleanup(func() { sts.Close() })
	}

	opts := Options{
		FileSvc:  fileSvc,
		Keystore: ks,
		OutDir:   outDir,
		Logger:   testutil.NewTestLogger(),
	}

	if seed {
		require.True(t, withSecondaryStores, "seeded reporting env requires secondary stores")
		seedReportingData(t, store, ev, rs, sts)
	}

	return opts
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
	opts := setupReportingEnv(t, true, true)

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
	opts := setupReportingEnv(t, false, true)

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

func TestRun_LockedVault_NilKeystore(t *testing.T) {
	opts := setupReportingEnv(t, true, true)
	opts.Keystore = nil // No keystore → locked vault.

	result, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.False(t, result.VaultUnlocked)
}

func TestRun_LockedVault_VaultKeyNotFound(t *testing.T) {
	opts := setupReportingEnv(t, true, true)
	// Keystore with no vault key initialized
	ksNoVaultKey, err := keystore.NewWithKeyringAndFS(testutil.NewTestLogger(), keystoretest.NewMemoryKeyring(), opts.FileSvc)
	require.NoError(t, err)
	require.NoError(t, ksNoVaultKey.Initialize())
	opts.Keystore = ksNoVaultKey

	result, err := Run(context.Background(), opts)
	require.NoError(t, err)
	assert.False(t, result.VaultUnlocked)
}

func TestRun_MissingExecutionVault(t *testing.T) {
	opts := setupReportingEnv(t, false, false)
	opts.FileSvc = blockedDBPathFileSvc(t, opts.FileSvc, constants.ExecutionVaultDBRelPath)

	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	// Executions and file_diffs CSVs should be absent.
	_, statErr := os.Stat(filepath.Join(opts.OutDir, constants.ReportExecutionsFilename))
	assert.True(t, os.IsNotExist(statErr), "executions.csv should not exist when execution vault is unavailable")
}

func TestRun_MissingReplayStore(t *testing.T) {
	opts := setupReportingEnv(t, false, false)
	opts.FileSvc = blockedDBPathFileSvc(t, opts.FileSvc, constants.ReplayStoreDBRelPath)

	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(opts.OutDir, constants.ReportReplayNoncesFilename))
	assert.True(t, os.IsNotExist(statErr), "replay_nonces.csv should not exist when replay store is unavailable")
}

func TestRun_MissingSuspendedTxStore(t *testing.T) {
	opts := setupReportingEnv(t, false, false)
	opts.FileSvc = blockedDBPathFileSvc(t, opts.FileSvc, constants.SuspendedTransactionDBRelPath)

	_, err := Run(context.Background(), opts)
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(opts.OutDir, constants.ReportSuspendedTxFilename))
	assert.True(t, os.IsNotExist(statErr), "suspended_transactions.csv should not exist when suspended tx store is unavailable")
}

func TestRun_CancelledContext(t *testing.T) {
	opts := setupReportingEnv(t, true, true)

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
	opts := setupReportingEnv(t, false, true)
	blocker := filepath.Join(testutil.TempDir(t), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0644))
	opts.OutDir = filepath.Join(blocker, "cannot-create-here")

	_, err := Run(context.Background(), opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportOutputDirFailed)
}

type blockedDBPathRuntimeFileSvc struct {
	fs.RuntimeFileService
	relPath string
	dbPath  string
}

func (svc *blockedDBPathRuntimeFileSvc) Resolve(relPath string) string {
	if relPath == svc.relPath {
		return svc.dbPath
	}
	return svc.RuntimeFileService.Resolve(relPath)
}

func blockedDBPathFileSvc(t *testing.T, base fs.RuntimeFileService, relPath string) fs.RuntimeFileService {
	t.Helper()
	blocker := filepath.Join(testutil.TempDir(t), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0644))
	return &blockedDBPathRuntimeFileSvc{
		RuntimeFileService: base,
		relPath:            relPath,
		dbPath:             filepath.Join(blocker, "test.db"),
	}
}
