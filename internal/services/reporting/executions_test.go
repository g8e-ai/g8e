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

// setupReportingExecutionVault creates a real ExecutionVaultService backed by a temp
// SQLite database with an unlocked vault, suitable for hermetic unit tests.
func setupReportingExecutionVault(t *testing.T) *storage.ExecutionVaultService {
	t.Helper()

	tempDir := testutil.TempDir(t)
	dbPath := filepath.Join(tempDir, "execution_vault.db")
	vaultDir := filepath.Join(tempDir, "vault")

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(vaultDir, 0700))

	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))

	testVault, err := vault.NewVault(&vault.VaultConfig{
		DataDir: vaultDir,
		Logger:  testutil.NewTestLogger(),
	})
	require.NoError(t, err)
	require.NoError(t, testVault.Unlock(privKey))

	config := &storage.ExecutionVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          1024,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
	}

	ev, err := storage.NewExecutionVaultService(config, testutil.NewTestLogger(), testVault)
	require.NoError(t, err)
	require.NotNil(t, ev)

	t.Cleanup(func() {
		ev.Wait()
		ev.Close()
	})

	return ev
}

// ---------------------------------------------------------------------------
// reportExecutions
// ---------------------------------------------------------------------------

func TestReportExecutions_EmptyStore(t *testing.T) {
	ev := setupReportingExecutionVault(t)
	outDir := testutil.TempDir(t)

	res, err := reportExecutions(context.Background(), outDir, ev)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportExecutionsFilename, res.Filename)
	assert.Equal(t, 0, res.RowCount)
	assert.NotEmpty(t, res.SHA256)

	path := filepath.Join(outDir, constants.ReportExecutionsFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 1, "header only")
	assert.Equal(t, ExecutionRow{}.Columns(), records[0])
}

func TestReportExecutions_WithRecords(t *testing.T) {
	ev := setupReportingExecutionVault(t)
	ctx := context.Background()

	now := time.Now().UTC()

	err := ev.StoreExecution(ctx, &models.ExecutionRecord{
		ID:           "exec-001",
		TimestampUTC: now,
		Command:      "ls -la",
		ExitCode:     0,
		DurationMs:   42,
		StdoutHash:   "abc123",
		StdoutSize:   100,
		StderrHash:   "def456",
		StderrSize:   0,
		OperatorID:   "op-1",
		CaseID:       "case-1",
		TaskID:       "task-1",
	})
	require.NoError(t, err)

	err = ev.StoreExecution(ctx, &models.ExecutionRecord{
		ID:           "exec-002",
		TimestampUTC: now.Add(time.Second),
		Command:      "cat /etc/hosts",
		ExitCode:     1,
		DurationMs:   10,
		StdoutHash:   "out-hash",
		StdoutSize:   50,
		StderrHash:   "err-hash",
		StderrSize:   25,
		OperatorID:   "op-2",
	})
	require.NoError(t, err)

	ev.Wait()

	outDir := testutil.TempDir(t)
	res, err := reportExecutions(ctx, outDir, ev)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RowCount)

	path := filepath.Join(outDir, constants.ReportExecutionsFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3, "header + 2 rows")
	assert.Equal(t, ExecutionRow{}.Columns(), records[0])
	assert.Equal(t, "exec-001", records[1][0])
	assert.Equal(t, "ls -la", records[1][2])
	assert.Equal(t, "0", records[1][3])
	assert.Equal(t, "42", records[1][4])
	assert.Equal(t, "op-1", records[1][11])
	assert.Equal(t, "exec-002", records[2][0])
	assert.Equal(t, "1", records[2][3])
}

func TestReportExecutions_CancelledContext(t *testing.T) {
	ev := setupReportingExecutionVault(t)
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportExecutions(ctx, outDir, ev)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestReportExecutions_NilService(t *testing.T) {
	outDir := testutil.TempDir(t)

	_, err := reportExecutions(context.Background(), outDir, nil)
	require.Error(t, err)
}

func TestReportExecutions_ExitCodeNone(t *testing.T) {
	ev := setupReportingExecutionVault(t)
	ctx := context.Background()

	now := time.Now().UTC()

	err := ev.StoreExecution(ctx, &models.ExecutionRecord{
		ID:           "exec-none",
		TimestampUTC: now,
		Command:      "pending-cmd",
		ExitCode:     constants.ExitCodeNone,
		DurationMs:   0,
	})
	require.NoError(t, err)
	ev.Wait()

	outDir := testutil.TempDir(t)
	res, err := reportExecutions(ctx, outDir, ev)
	require.NoError(t, err)
	assert.Equal(t, 1, res.RowCount)

	path := filepath.Join(outDir, constants.ReportExecutionsFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "", records[1][3], "ExitCodeNone should produce empty string")
}

// ---------------------------------------------------------------------------
// reportFileDiffs
// ---------------------------------------------------------------------------

func TestReportFileDiffs_EmptyStore(t *testing.T) {
	ev := setupReportingExecutionVault(t)
	outDir := testutil.TempDir(t)

	res, err := reportFileDiffs(context.Background(), outDir, ev)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportFileDiffsFilename, res.Filename)
	assert.Equal(t, 0, res.RowCount)
	assert.NotEmpty(t, res.SHA256)

	path := filepath.Join(outDir, constants.ReportFileDiffsFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 1, "header only")
	assert.Equal(t, FileDiffRow{}.Columns(), records[0])
}

func TestReportFileDiffs_WithRecords(t *testing.T) {
	ev := setupReportingExecutionVault(t)
	ctx := context.Background()

	now := time.Now().UTC()

	err := ev.StoreFileDiff(ctx, &models.FileDiffRecord{
		ID:                "diff-001",
		TimestampUTC:      now,
		FilePath:          "/test/file1.txt",
		Operation:         "WRITE",
		LedgerHashBefore:  "hash-before-1",
		LedgerHashAfter:   "hash-after-1",
		DiffStat:          "1 file changed",
		DiffHash:          "diff-hash-1",
		DiffSize:          200,
		OperatorSessionID: "sess-1",
		CaseID:            "case-1",
		OperatorID:        "op-1",
	})
	require.NoError(t, err)

	err = ev.StoreFileDiff(ctx, &models.FileDiffRecord{
		ID:                "diff-002",
		TimestampUTC:      now.Add(time.Second),
		FilePath:          "/test/file2.txt",
		Operation:         "DELETE",
		LedgerHashBefore:  "hash-before-2",
		LedgerHashAfter:   "hash-after-2",
		DiffStat:          "1 file deleted",
		DiffHash:          "diff-hash-2",
		DiffSize:          0,
		OperatorSessionID: "sess-2",
		OperatorID:        "op-2",
	})
	require.NoError(t, err)

	ev.Wait()

	outDir := testutil.TempDir(t)
	res, err := reportFileDiffs(ctx, outDir, ev)
	require.NoError(t, err)
	assert.Equal(t, 2, res.RowCount)

	path := filepath.Join(outDir, constants.ReportFileDiffsFilename)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3, "header + 2 rows")
	assert.Equal(t, FileDiffRow{}.Columns(), records[0])
	assert.Equal(t, "diff-001", records[1][0])
	assert.Equal(t, "/test/file1.txt", records[1][2])
	assert.Equal(t, "WRITE", records[1][3])
	assert.Equal(t, "200", records[1][8])
	assert.Equal(t, "sess-1", records[1][9])
	assert.Equal(t, "op-1", records[1][11])
	assert.Equal(t, "diff-002", records[2][0])
	assert.Equal(t, "DELETE", records[2][3])
	assert.Equal(t, "0", records[2][8])
}

func TestReportFileDiffs_CancelledContext(t *testing.T) {
	ev := setupReportingExecutionVault(t)
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportFileDiffs(ctx, outDir, ev)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestReportFileDiffs_NilService(t *testing.T) {
	outDir := testutil.TempDir(t)

	_, err := reportFileDiffs(context.Background(), outDir, nil)
	require.Error(t, err)
}
