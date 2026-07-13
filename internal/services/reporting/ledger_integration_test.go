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
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestGitLedger creates a real GitLedgerService with a temp git repo
// and an unlocked vault. The caller can optionally seed commits.
func setupTestGitLedger(t *testing.T) *storage.GitLedgerService {
	t.Helper()

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	vaultDir := filepath.Join(testutil.TempDir(t), "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0o700))
	v := createTestVault(t, vaultDir, privKey)

	ledgerDir := filepath.Join(testutil.TempDir(t), "ledger")
	ledger, err := storage.NewGitLedgerService(&storage.LedgerConfig{
		BaseDir:         ledgerDir,
		GitPath:         "git",
		EncryptionVault: v,
	}, testutil.NewTestLogger())
	require.NoError(t, err)

	return ledger
}

// seedLedgerCommit writes a file through the two-phase ledger commit flow,
// producing at least one git commit in the files/ repo.
func seedLedgerCommit(t *testing.T, ledger *storage.GitLedgerService, sessionID, filePath, content string) {
	t.Helper()

	// Write the source file.
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	// Phase 1: LedgerFileWrite (pre-mutation snapshot).
	result, err := ledger.LedgerFileWrite(sessionID, filePath)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Phase 2: CompleteMirrorWrite (post-mutation commit).
	require.NoError(t, ledger.CompleteMirrorWrite(result, sessionID))
}

func TestReportLedgerMerkleRoot_BootstrapOnly(t *testing.T) {
	ledger := setupTestGitLedger(t)
	outDir := testutil.TempDir(t)

	res, err := reportLedgerMerkleRoot(context.Background(), outDir, ledger)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportLedgerMerkleRootFilename, res.Filename)
	// Bootstrap creates an initial .gitignore commit, so merkle root exists.
	assert.Equal(t, 1, res.RowCount)

	// CSV should exist with header + 1 row.
	f, err := os.Open(filepath.Join(outDir, constants.ReportLedgerMerkleRootFilename))
	require.NoError(t, err)
	defer f.Close()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2, "header + 1 row")
	assert.NotEmpty(t, records[1][0], "merkle root should be non-empty")
}

func TestReportLedgerMerkleRoot_WithCommits(t *testing.T) {
	ledger := setupTestGitLedger(t)

	// Seed a commit.
	srcFile := filepath.Join(testutil.TempDir(t), "test-file.txt")
	seedLedgerCommit(t, ledger, "", srcFile, "hello world")

	outDir := testutil.TempDir(t)
	res, err := reportLedgerMerkleRoot(context.Background(), outDir, ledger)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportLedgerMerkleRootFilename, res.Filename)
	assert.Equal(t, 1, res.RowCount, "should have 1 row with the merkle root")

	// Verify CSV content.
	f, err := os.Open(filepath.Join(outDir, constants.ReportLedgerMerkleRootFilename))
	require.NoError(t, err)
	defer f.Close()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2, "header + 1 row")
	assert.NotEmpty(t, records[1][0], "merkle root should be non-empty")
}

func TestReportLedgerCommits_BootstrapOnly(t *testing.T) {
	ledger := setupTestGitLedger(t)
	outDir := testutil.TempDir(t)

	res, err := reportLedgerCommits(context.Background(), outDir, ledger)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportLedgerCommitsFilename, res.Filename)
	// Bootstrap creates an initial .gitignore commit.
	assert.Equal(t, 1, res.RowCount)

	// CSV should exist with header + 1 row.
	f, err := os.Open(filepath.Join(outDir, constants.ReportLedgerCommitsFilename))
	require.NoError(t, err)
	defer f.Close()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2, "header + 1 row")
	assert.NotEmpty(t, records[1][0], "commit hash should be non-empty")
}

func TestReportLedgerCommits_WithCommits(t *testing.T) {
	ledger := setupTestGitLedger(t)

	// Seed two commits with different files.
	srcFile1 := filepath.Join(testutil.TempDir(t), "file1.txt")
	seedLedgerCommit(t, ledger, "", srcFile1, "content 1")

	srcFile2 := filepath.Join(testutil.TempDir(t), "file2.txt")
	seedLedgerCommit(t, ledger, "", srcFile2, "content 2")

	outDir := testutil.TempDir(t)
	res, err := reportLedgerCommits(context.Background(), outDir, ledger)
	require.NoError(t, err)
	assert.Equal(t, constants.ReportLedgerCommitsFilename, res.Filename)
	assert.GreaterOrEqual(t, res.RowCount, 2, "should have at least 2 commits")

	// Verify CSV content.
	f, err := os.Open(filepath.Join(outDir, constants.ReportLedgerCommitsFilename))
	require.NoError(t, err)
	defer f.Close()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(records), 3, "header + at least 2 commit rows")

	// Each commit row should have a non-empty commit hash (first column).
	for i := 1; i < len(records); i++ {
		assert.NotEmpty(t, records[i][0], "commit hash should be non-empty (row %d)", i)
	}
}

func TestReportLedgerMerkleRoot_CancelledContext(t *testing.T) {
	ledger := setupTestGitLedger(t)
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportLedgerMerkleRoot(ctx, outDir, ledger)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestReportLedgerCommits_CancelledContext(t *testing.T) {
	ledger := setupTestGitLedger(t)
	outDir := testutil.TempDir(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reportLedgerCommits(ctx, outDir, ledger)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
