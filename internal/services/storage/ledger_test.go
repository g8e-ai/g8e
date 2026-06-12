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

package storage

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// setupTestLedger creates a test environment for GitLedgerService with encryption disabled.
func setupTestLedger(t *testing.T) (*GitLedgerService, string) {
	gitPath := testGitPath(t)
	tempDir := t.TempDir()
	ledgerDir := filepath.Join(tempDir, "ledger")

	// Create vault but do NOT unlock it (encryption disabled)
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))
	testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	ledgerConfig := &LedgerConfig{
		BaseDir:         ledgerDir,
		GitPath:         gitPath,
		EncryptionVault: testVault,
	}
	lms, err := NewGitLedgerService(ledgerConfig, logger)
	require.NoError(t, err)
	require.NotNil(t, lms)

	return lms, tempDir
}

// setupTestLedgerWithEncryption creates a test environment for GitLedgerService with encryption enabled.
func setupTestLedgerWithEncryption(t *testing.T) (*GitLedgerService, string) {
	gitPath := testGitPath(t)
	tempDir := t.TempDir()
	ledgerDir := filepath.Join(tempDir, "ledger")

	// Create vault and unlock it (encryption enabled)
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	vHeader, _, err := vault.NewVaultHeader(privKey)
	require.NoError(t, err)
	require.NoError(t, vHeader.Save(vaultDir))
	testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: testutil.NewTestLogger()})
	require.NoError(t, err)
	require.NoError(t, testVault.Unlock(privKey))
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	ledgerConfig := &LedgerConfig{
		BaseDir:         ledgerDir,
		GitPath:         gitPath,
		EncryptionVault: testVault,
	}
	lms, err := NewGitLedgerService(ledgerConfig, logger)
	require.NoError(t, err)
	require.NotNil(t, lms)

	return lms, tempDir
}

func TestLedgerService_NewService(t *testing.T) {
	t.Parallel()
	lms, _ := setupTestLedger(t)

	assert.NotNil(t, lms)
	assert.NotNil(t, lms.config)
	assert.NotNil(t, lms.logger)
}

func TestLedgerService_NewServiceWithNilConfig(t *testing.T) {
	t.Parallel()
	lms, err := NewGitLedgerService(nil, testutil.NewTestLogger())
	require.Error(t, err)
	assert.Nil(t, lms)
}

func TestLedgerService_MirrorFileWrite_NewFile(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Create a test file path (file doesn't exist yet)
	testFilePath := filepath.Join(tempDir, "test_write.txt")
	operatorSessionID := "test-session-write"

	// Start the mirror operation
	result, err := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, testFilePath, result.FilePath)
	assert.Equal(t, FileMutationWrite, result.Operation)
	assert.True(t, result.Success)
	assert.NotEmpty(t, result.LedgerPath)

	// The mirror path should be within the ledger
	assert.Contains(t, result.LedgerPath, "files")

	// Now create the actual file
	err = os.WriteFile(testFilePath, []byte("Hello, World!"), 0644)
	require.NoError(t, err)

	// Complete the mirror operation
	err = lms.CompleteMirrorWrite(result, operatorSessionID)
	require.NoError(t, err)

	assert.NotEmpty(t, result.LedgerHashAfter)
	assert.NotEmpty(t, result.DiffStat)

	// Verify file was copied to ledger
	mirrorContent, err := os.ReadFile(result.LedgerPath)
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", string(mirrorContent))
}

func TestLedgerService_MirrorFileWrite_ExistingFile(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Create an existing file
	testFilePath := filepath.Join(tempDir, "existing_file.txt")
	err := os.WriteFile(testFilePath, []byte("Original content"), 0644)
	require.NoError(t, err)

	operatorSessionID := "test-session-existing"

	// Start the mirror operation (should backup existing file first)
	result, err := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotEmpty(t, result.LedgerHashBefore)
	assert.True(t, result.Success)

	// Modify the file
	err = os.WriteFile(testFilePath, []byte("Modified content"), 0644)
	require.NoError(t, err)

	// Complete the mirror operation
	err = lms.CompleteMirrorWrite(result, operatorSessionID)
	require.NoError(t, err)

	assert.NotEmpty(t, result.LedgerHashAfter)
	assert.NotEqual(t, result.LedgerHashBefore, result.LedgerHashAfter)

	// Verify mirror reflects updated content
	mirrorContent, err := os.ReadFile(result.LedgerPath)
	require.NoError(t, err)
	assert.Equal(t, "Modified content", string(mirrorContent))
}

func TestLedgerService_MirrorFileWrite_DisabledVault(t *testing.T) {
	t.Parallel()
	lms, _ := NewGitLedgerService(nil, testutil.NewTestLogger())

	result, err := lms.LedgerFileWrite("operator_session", "/some/file")
	require.NoError(t, err) // Graceful degradation: returns nil, nil when git not ready
	assert.Nil(t, result)
}

func TestLedgerService_CompleteMirrorWrite_NilResult(t *testing.T) {
	t.Parallel()
	lms, _ := setupTestLedger(t)

	err := lms.CompleteMirrorWrite(nil, "operator_session")
	require.NoError(t, err)
}

func TestLedgerService_MirrorFileDelete(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Create a file to delete
	testFilePath := filepath.Join(tempDir, "to_delete.txt")
	err := os.WriteFile(testFilePath, []byte("Content to delete"), 0644)
	require.NoError(t, err)

	operatorSessionID := "test-session-delete"

	// Start the delete mirror operation
	result, err := lms.MirrorFileDelete(operatorSessionID, testFilePath)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, testFilePath, result.FilePath)
	assert.Equal(t, FileMutationDelete, result.Operation)
	assert.NotEmpty(t, result.LedgerHashBefore)
	assert.True(t, result.Success)

	// Delete the actual file
	err = os.Remove(testFilePath)
	require.NoError(t, err)

	// Complete the delete mirror operation
	err = lms.CompleteMirrorDelete(result, operatorSessionID)
	require.NoError(t, err)

	assert.NotEmpty(t, result.LedgerHashAfter)
	assert.Equal(t, "file deleted", result.DiffStat)
}

func TestLedgerService_MirrorFileDelete_NonExistentFile(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Try to mirror deletion of non-existent file
	testFilePath := filepath.Join(tempDir, "non_existent.txt")
	operatorSessionID := "test-session-delete-nonexistent"

	result, err := lms.MirrorFileDelete(operatorSessionID, testFilePath)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Success)
}

func TestLedgerService_MirrorFileDelete_DisabledVault(t *testing.T) {
	t.Parallel()
	lms, _ := NewGitLedgerService(nil, testutil.NewTestLogger())

	result, err := lms.MirrorFileDelete("operator_session", "/some/file")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestLedgerService_MirrorFileCreate(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	testFilePath := filepath.Join(tempDir, "new_created_file.txt")
	operatorSessionID := "test-session-create"

	// Start the create mirror operation
	result, err := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, testFilePath, result.FilePath)
	assert.Equal(t, FileMutationCreate, result.Operation)
	assert.NotEmpty(t, result.LedgerHashBefore)
	assert.True(t, result.Success)

	// Create the actual file
	content := "Line 1\nLine 2\nLine 3\n"
	err = os.WriteFile(testFilePath, []byte(content), 0644)
	require.NoError(t, err)

	// Complete the create mirror operation
	err = lms.CompleteMirrorCreate(result, operatorSessionID)
	require.NoError(t, err)

	assert.NotEmpty(t, result.LedgerHashAfter)
	assert.Contains(t, result.DiffStat, "new file")
	assert.Contains(t, result.DiffStat, "lines")

	// Verify file was copied to ledger
	mirrorContent, err := os.ReadFile(result.LedgerPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(mirrorContent))
}

func TestLedgerService_MirrorFileCreate_DisabledVault(t *testing.T) {
	t.Parallel()
	lms, _ := NewGitLedgerService(nil, testutil.NewTestLogger())

	result, err := lms.MirrorFileCreate("operator_session", "/some/file")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestLedgerService_CompleteMirrorCreate_DisabledVault(t *testing.T) {
	t.Parallel()
	lms, _ := NewGitLedgerService(nil, testutil.NewTestLogger())

	err := lms.CompleteMirrorCreate(&LedgerResult{}, "operator_session")
	require.NoError(t, err)
}

func TestLedgerService_GetLedgerPath(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	ledgerDir := filepath.Join(tempDir, "ledger")

	// The mirror path should be within the ledger
	ledgerPath := lms.getLedgerPath(ledgerDir, "/etc/nginx/nginx.conf")
	assert.Contains(t, ledgerPath, "files")
	// On Windows, the path will include the drive letter, so just check for the relative part
	assert.True(t, strings.Contains(ledgerPath, "etc/nginx/nginx.conf") || strings.Contains(ledgerPath, "nginx.conf"), "path should contain nginx.conf")
	assert.NotContains(t, ledgerPath, "//")

	// Test relative path (should be converted to absolute)
	ledgerPath = lms.getLedgerPath(ledgerDir, "relative/path/file.txt")
	assert.Contains(t, ledgerPath, "files")
}

func TestLedgerService_CopyToLedger(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Create source file
	srcPath := filepath.Join(tempDir, "source.txt")
	srcContent := "Source file content with special chars: äöü\n"
	err := os.WriteFile(srcPath, []byte(srcContent), 0644)
	require.NoError(t, err)

	// Copy to ledger
	dstPath := filepath.Join(tempDir, "ledger", "files", "test", "source.txt")
	err = lms.copyToLedger(srcPath, dstPath)
	require.NoError(t, err)

	// Verify copy
	dstContent, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, srcContent, string(dstContent))
}

func TestLedgerService_CopyToLedger_NonExistentSource(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	err := lms.copyToLedger("/nonexistent/file.txt", filepath.Join(tempDir, "dst.txt"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open source file")
}

func TestLedgerService_SnapshotLedger(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	ledgerDir := filepath.Join(tempDir, "ledger")

	// Initialize git repository first
	err := lms.initGitRepo(ledgerDir)
	require.NoError(t, err, "failed to initialize git repo")

	// Take a snapshot
	hash1, err := lms.snapshotLedger(ledgerDir, "Test snapshot 1")
	require.NoError(t, err)
	assert.NotEmpty(t, hash1)
	assert.Len(t, hash1, 40) // Git SHA-1 hash length

	// Make a change and take another snapshot
	testFile := filepath.Join(tempDir, "ledger", "files", "snapshot_test.txt")
	os.MkdirAll(filepath.Dir(testFile), 0755)
	err = os.WriteFile(testFile, []byte("snapshot test"), 0644)
	require.NoError(t, err)

	hash2, err := lms.snapshotLedger(ledgerDir, "Test snapshot 2")
	require.NoError(t, err)
	assert.NotEmpty(t, hash2)
	assert.NotEqual(t, hash1, hash2)
}

func TestLedgerService_CalculateDiffStat(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	ledgerDir := filepath.Join(tempDir, "ledger")

	// Initialize git repo first
	err := lms.initGitRepo(ledgerDir)
	require.NoError(t, err)

	// Take initial snapshot
	hash1, err := lms.snapshotLedger(ledgerDir, "Initial state")
	require.NoError(t, err)

	// Create a file
	testFile := filepath.Join(tempDir, "ledger", "files", "diff_test.txt")
	os.MkdirAll(filepath.Dir(testFile), 0755)
	err = os.WriteFile(testFile, []byte("line 1\nline 2\nline 3\n"), 0644)
	require.NoError(t, err)

	// Take another snapshot
	hash2, err := lms.snapshotLedger(ledgerDir, "After adding file")
	require.NoError(t, err)

	// Calculate diff stat
	diffStat := lms.calculateDiffStat(ledgerDir, hash1, hash2)
	assert.NotEmpty(t, diffStat)
}

func TestLedgerService_CalculateDiffStat_EmptyHashes(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	ledgerDir := filepath.Join(tempDir, "ledger")

	diffStat := lms.calculateDiffStat(ledgerDir, "", "")
	assert.Empty(t, diffStat)

	diffStat = lms.calculateDiffStat(ledgerDir, "abc123", "")
	assert.Empty(t, diffStat)
}

func TestLedgerService_CountLines(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Create test file with multiple lines
	testFile := filepath.Join(tempDir, "lines_test.txt")
	err := os.WriteFile(testFile, []byte("line 1\nline 2\nline 3"), 0644)
	require.NoError(t, err)

	count := lms.countLines(testFile)
	assert.Equal(t, 3, count)

	// Empty file
	emptyFile := filepath.Join(tempDir, "empty.txt")
	err = os.WriteFile(emptyFile, []byte(""), 0644)
	require.NoError(t, err)

	count = lms.countLines(emptyFile)
	assert.Equal(t, 1, count)

	// Non-existent file
	count = lms.countLines("/nonexistent/file.txt")
	assert.Equal(t, 0, count)
}

func TestLedgerService_GetFileHistory(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Create a file and make multiple changes
	testFilePath := filepath.Join(tempDir, "history_test.txt")
	operatorSessionID := "test-session-history"

	// First version
	result1, _ := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Version 1"), 0644)
	lms.CompleteMirrorCreate(result1, operatorSessionID)

	// Second version
	result2, _ := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Version 2"), 0644)
	lms.CompleteMirrorWrite(result2, operatorSessionID)

	// Third version
	result3, _ := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Version 3"), 0644)
	lms.CompleteMirrorWrite(result3, operatorSessionID)

	// Get file history
	history, err := lms.GetFileHistory(testFilePath, 10, operatorSessionID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(history), 2) // At least 2 commits for this file

	// Verify history entries have valid data
	for _, entry := range history {
		assert.NotEmpty(t, entry.CommitHash)
		assert.NotEmpty(t, entry.Message)
		assert.False(t, entry.Timestamp.IsZero())
		assert.Equal(t, testFilePath, entry.FilePath)
	}
}

// Regression: callers (e.g. HistoryHandler) may hold a nil *GitLedgerService when
// the Operator was started without local storage. Public methods must degrade
// to an error instead of panicking on a nil receiver inside gitReady().
func TestLedgerService_GetFileHistory_NilReceiver(t *testing.T) {
	t.Parallel()
	var lms *GitLedgerService

	assert.NotPanics(t, func() {
		history, err := lms.GetFileHistory("/some/file", 10, "session")
		require.Error(t, err)
		assert.Nil(t, history)
		assert.Contains(t, err.Error(), "disabled")
	})
}

func TestLedgerService_GetFileHistory_DisabledVault(t *testing.T) {
	t.Parallel()
	lms, _ := NewGitLedgerService(nil, testutil.NewTestLogger())

	history, err := lms.GetFileHistory("/some/file", 10, "session")
	require.Error(t, err)
	assert.Nil(t, history)
	assert.Contains(t, err.Error(), "disabled")
}

func TestLedgerService_GetFileHistory_DefaultLimit(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Create a file
	testFilePath := filepath.Join(tempDir, "limit_test.txt")
	result, _ := lms.MirrorFileCreate("operator_session", testFilePath)
	os.WriteFile(testFilePath, []byte("content"), 0644)
	lms.CompleteMirrorCreate(result, "operator_session")

	// Get history with zero limit (should default to 50)
	history, err := lms.GetFileHistory(testFilePath, 0, "operator_session")
	// History may be empty if git operations haven't completed
	if err != nil {
		assert.Contains(t, err.Error(), "not found")
	} else {
		assert.NotNil(t, history)
	}
}

func TestLedgerService_GetFileAtCommit(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedgerWithEncryption(t)

	testFilePath := filepath.Join(tempDir, "commit_test.txt")
	operatorSessionID := "test-session-commit"

	// Create initial version
	result1, _ := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Initial content"), 0644)
	lms.CompleteMirrorCreate(result1, operatorSessionID)
	initialHash := result1.LedgerHashAfter

	// Create second version
	result2, _ := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Modified content"), 0644)
	lms.CompleteMirrorWrite(result2, operatorSessionID)

	// Get content at initial commit
	content, err := lms.GetFileAtCommit(testFilePath, initialHash, operatorSessionID)
	require.NoError(t, err)
	assert.Equal(t, "Initial content", content)

	// Current file should be different
	currentContent, _ := os.ReadFile(testFilePath)
	assert.Equal(t, "Modified content", string(currentContent))
}

func TestLedgerService_GetFileAtCommit_DisabledVault(t *testing.T) {
	t.Parallel()
	lms, _ := NewGitLedgerService(nil, testutil.NewTestLogger())

	content, err := lms.GetFileAtCommit("/some/file", "abc123", "session")
	require.Error(t, err)
	assert.Empty(t, content)
	assert.Contains(t, err.Error(), "disabled")
}

func TestLedgerService_RestoreFileFromCommit(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedgerWithEncryption(t)

	testFilePath := filepath.Join(tempDir, "restore_test.txt")
	operatorSessionID := "test-session-restore"

	// Create initial version
	result1, _ := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Original content"), 0644)
	lms.CompleteMirrorCreate(result1, operatorSessionID)
	originalHash := result1.LedgerHashAfter

	// Create second version
	result2, _ := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("New content"), 0644)
	lms.CompleteMirrorWrite(result2, operatorSessionID)

	// Verify current content is "New content"
	currentContent, _ := os.ReadFile(testFilePath)
	assert.Equal(t, "New content", string(currentContent))

	// Restore to original version
	err := lms.RestoreFileFromCommit(testFilePath, originalHash, operatorSessionID)
	require.NoError(t, err)

	// Verify content is restored
	restoredContent, _ := os.ReadFile(testFilePath)
	assert.Equal(t, "Original content", string(restoredContent))
}

func TestLedgerService_RestoreFileFromCommit_DisabledVault(t *testing.T) {
	t.Parallel()
	// Test with config that has no vault (encryption disabled)
	config := &LedgerConfig{
		BaseDir: t.TempDir(),
		GitPath: "/usr/bin/git",
	}
	_, err := NewGitLedgerService(config, testutil.NewTestLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EncryptionVault is required")
}

func TestLedgerService_RestoreFileFromCommit_InvalidCommit(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	testFilePath := filepath.Join(tempDir, "invalid_restore.txt")

	err := lms.RestoreFileFromCommit(testFilePath, "invalidhash123", "operator_session")
	require.Error(t, err)
}

func TestLedgerService_CompleteWorkflow(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedgerWithEncryption(t)

	operatorSessionID := "test-complete-workflow"
	testFilePath := filepath.Join(tempDir, "workflow_test.txt")

	// Step 1: Create file
	createResult, err := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	require.NoError(t, err)

	os.WriteFile(testFilePath, []byte("Step 1: Created"), 0644)
	err = lms.CompleteMirrorCreate(createResult, operatorSessionID)
	require.NoError(t, err)
	hash1 := createResult.LedgerHashAfter

	// Step 2: Modify file
	writeResult, err := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	require.NoError(t, err)

	os.WriteFile(testFilePath, []byte("Step 2: Modified"), 0644)
	err = lms.CompleteMirrorWrite(writeResult, operatorSessionID)
	require.NoError(t, err)
	hash2 := writeResult.LedgerHashAfter

	// Step 3: Modify again
	writeResult2, err := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	require.NoError(t, err)

	os.WriteFile(testFilePath, []byte("Step 3: Modified again"), 0644)
	err = lms.CompleteMirrorWrite(writeResult2, operatorSessionID)
	require.NoError(t, err)

	// Verify history
	history, err := lms.GetFileHistory(testFilePath, 10, operatorSessionID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(history), 2)

	// Restore to Step 1
	err = lms.RestoreFileFromCommit(testFilePath, hash1, operatorSessionID)
	require.NoError(t, err)

	content, _ := os.ReadFile(testFilePath)
	assert.Equal(t, "Step 1: Created", string(content))

	// Restore to Step 2
	err = lms.RestoreFileFromCommit(testFilePath, hash2, operatorSessionID)
	require.NoError(t, err)

	content, _ = os.ReadFile(testFilePath)
	assert.Equal(t, "Step 2: Modified", string(content))
}

func TestLedgerService_MultiSessionConcurrency(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Test that multiple sessions can operate in parallel without blocking each other's git commits
	sessionCount := 5
	done := make(chan bool, sessionCount)

	for i := 0; i < sessionCount; i++ {
		go func(idx int) {
			sessionID := fmt.Sprintf("parallel-session-%d", idx)
			filePath := filepath.Join(tempDir, fmt.Sprintf("parallel_test_%d.txt", idx))

			// Start multi-phase operation
			result, err := lms.MirrorFileCreate(sessionID, filePath)
			assert.NoError(t, err)

			os.WriteFile(filePath, []byte(fmt.Sprintf("content from session %d", idx)), 0644)
			err = lms.CompleteMirrorCreate(result, sessionID)
			assert.NoError(t, err)

			// Verify session-specific ledger directory exists
			sessionLedgerDir := filepath.Join(tempDir, "ledger", "sessions", sessionID)
			assert.DirExists(t, filepath.Join(sessionLedgerDir, ".git"))

			done <- true
		}(i)
	}

	for i := 0; i < sessionCount; i++ {
		select {
		case <-done:
		case <-time.After(time.Second * 10):
			t.Errorf("Timeout waiting for parallel session %d", i)
		}
	}
}

func TestLedgerService_LargeFile(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	testFilePath := filepath.Join(tempDir, "large_file.txt")
	operatorSessionID := "test-large-file"

	// Create a large file (1MB)
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte('A' + (i % 26))
	}

	result, err := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	require.NoError(t, err)

	os.WriteFile(testFilePath, largeContent, 0644)
	err = lms.CompleteMirrorCreate(result, operatorSessionID)
	require.NoError(t, err)

	// Verify file was mirrored correctly
	mirrorContent, err := os.ReadFile(result.LedgerPath)
	require.NoError(t, err)
	assert.Len(t, mirrorContent, len(largeContent))
	assert.Equal(t, largeContent, mirrorContent)
}

func TestLedgerService_SpecialCharactersInPath(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// File with spaces and special chars (that are valid in filenames)
	testFilePath := filepath.Join(tempDir, "special file-name_v1.2.txt")
	operatorSessionID := "test-special-chars"

	result, err := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	require.NoError(t, err)

	os.WriteFile(testFilePath, []byte("special content"), 0644)
	err = lms.CompleteMirrorCreate(result, operatorSessionID)
	require.NoError(t, err)

	// Verify file was copied
	mirrorContent, err := os.ReadFile(result.LedgerPath)
	require.NoError(t, err)
	assert.Equal(t, "special content", string(mirrorContent))
}

func TestLedgerService_DeepNestedPath(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	// Create a deeply nested path
	nestedPath := filepath.Join(tempDir, "level1", "level2", "level3", "level4", "deep_file.txt")
	os.MkdirAll(filepath.Dir(nestedPath), 0755)
	operatorSessionID := "test-deep-path"

	result, err := lms.MirrorFileCreate(operatorSessionID, nestedPath)
	require.NoError(t, err)

	os.WriteFile(nestedPath, []byte("deep content"), 0644)
	err = lms.CompleteMirrorCreate(result, operatorSessionID)
	require.NoError(t, err)

	// Verify nested structure was created in mirror
	mirrorContent, err := os.ReadFile(result.LedgerPath)
	require.NoError(t, err)
	assert.Equal(t, "deep content", string(mirrorContent))
}

func TestLedgerService_NodeBinaryFile(t *testing.T) {
	t.Parallel()
	lms, tempDir := setupTestLedger(t)

	testFilePath := filepath.Join(tempDir, "binary_file.bin")
	operatorSessionID := "test-binary"

	// Create binary content with null bytes
	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}

	result, err := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	require.NoError(t, err)

	os.WriteFile(testFilePath, binaryContent, 0644)
	err = lms.CompleteMirrorCreate(result, operatorSessionID)
	require.NoError(t, err)

	// Verify binary file was copied correctly
	mirrorContent, err := os.ReadFile(result.LedgerPath)
	require.NoError(t, err)
	assert.Equal(t, binaryContent, mirrorContent)
}
