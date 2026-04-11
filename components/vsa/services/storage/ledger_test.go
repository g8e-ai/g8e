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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/vsa/testutil"
)

// setupTestLedger creates a test environment for LedgerService
func setupTestLedger(t *testing.T) (*LedgerService, *AuditVaultService, string) {
	gitPath := testGitPath(t)
	tempDir := t.TempDir()

	config := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		GitPath:                   gitPath,
	}

	logger := testutil.NewTestLogger()

	avs, err := NewAuditVaultService(config, logger)
	require.NoError(t, err)

	lms := NewLedgerService(avs, nil, logger)
	require.NotNil(t, lms)

	return lms, avs, tempDir
}

func TestLedgerService_NewService(t *testing.T) {
	lms, avs, _ := setupTestLedger(t)
	defer avs.Close()

	assert.NotNil(t, lms)
	assert.NotNil(t, lms.auditVault)
	assert.NotNil(t, lms.logger)
}

func TestLedgerService_NewServiceWithNilAuditVault(t *testing.T) {
	lms := NewLedgerService(nil, nil, testutil.NewTestLogger())
	assert.NotNil(t, lms)
	assert.Nil(t, lms.auditVault)
}

func TestLedgerService_MirrorFileWrite_NewFile(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	assert.True(t, strings.Contains(result.LedgerPath, "ledger/files"))

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
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	lms := NewLedgerService(nil, nil, testutil.NewTestLogger())

	result, err := lms.LedgerFileWrite("operator_session", "/some/file")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestLedgerService_CompleteMirrorWrite_NilResult(t *testing.T) {
	lms, avs, _ := setupTestLedger(t)
	defer avs.Close()

	err := lms.CompleteMirrorWrite(nil, "operator_session")
	assert.NoError(t, err)
}

func TestLedgerService_MirrorFileDelete(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	// Try to mirror deletion of non-existent file
	testFilePath := filepath.Join(tempDir, "non_existent.txt")
	operatorSessionID := "test-session-delete-nonexistent"

	result, err := lms.MirrorFileDelete(operatorSessionID, testFilePath)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Success)
}

func TestLedgerService_MirrorFileDelete_DisabledVault(t *testing.T) {
	lms := NewLedgerService(nil, nil, testutil.NewTestLogger())

	result, err := lms.MirrorFileDelete("operator_session", "/some/file")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestLedgerService_MirrorFileCreate(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	lms := NewLedgerService(nil, nil, testutil.NewTestLogger())

	result, err := lms.MirrorFileCreate("operator_session", "/some/file")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestLedgerService_CompleteMirrorCreate_DisabledVault(t *testing.T) {
	lms := NewLedgerService(nil, nil, testutil.NewTestLogger())

	err := lms.CompleteMirrorCreate(&LedgerResult{}, "operator_session")
	assert.NoError(t, err)
}

func TestLedgerService_GetLedgerPath(t *testing.T) {
	lms, avs, _ := setupTestLedger(t)
	defer avs.Close()

	// Test absolute path
	ledgerPath := lms.getLedgerPath("/etc/nginx/nginx.conf")
	assert.Contains(t, ledgerPath, "ledger/files")
	assert.Contains(t, ledgerPath, "etc/nginx/nginx.conf")
	assert.False(t, strings.Contains(ledgerPath, "//"))

	// Test relative path (should be converted to absolute)
	ledgerPath = lms.getLedgerPath("relative/path/file.txt")
	assert.Contains(t, ledgerPath, "ledger/files")
}

func TestLedgerService_CopyToLedger(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	err := lms.copyToLedger("/nonexistent/file.txt", filepath.Join(tempDir, "dst.txt"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open source file")
}

func TestLedgerService_SnapshotLedger(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	// Take a snapshot
	hash1, err := lms.snapshotLedger("Test snapshot 1")
	require.NoError(t, err)
	assert.NotEmpty(t, hash1)
	assert.Len(t, hash1, 40) // Git SHA-1 hash length

	// Make a change and take another snapshot
	testFile := filepath.Join(tempDir, "ledger", "files", "snapshot_test.txt")
	os.MkdirAll(filepath.Dir(testFile), 0755)
	err = os.WriteFile(testFile, []byte("snapshot test"), 0644)
	require.NoError(t, err)

	hash2, err := lms.snapshotLedger("Test snapshot 2")
	require.NoError(t, err)
	assert.NotEmpty(t, hash2)
	assert.NotEqual(t, hash1, hash2)
}

func TestLedgerService_CalculateDiffStat(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	// Take initial snapshot
	hash1, err := lms.snapshotLedger("Initial state")
	require.NoError(t, err)

	// Create a file
	testFile := filepath.Join(tempDir, "ledger", "files", "diff_test.txt")
	os.MkdirAll(filepath.Dir(testFile), 0755)
	err = os.WriteFile(testFile, []byte("line 1\nline 2\nline 3\n"), 0644)
	require.NoError(t, err)

	// Take another snapshot
	hash2, err := lms.snapshotLedger("After adding file")
	require.NoError(t, err)

	// Calculate diff stat
	diffStat := lms.calculateDiffStat(hash1, hash2)
	assert.NotEmpty(t, diffStat)
}

func TestLedgerService_CalculateDiffStat_EmptyHashes(t *testing.T) {
	lms, avs, _ := setupTestLedger(t)
	defer avs.Close()

	diffStat := lms.calculateDiffStat("", "")
	assert.Empty(t, diffStat)

	diffStat = lms.calculateDiffStat("abc123", "")
	assert.Empty(t, diffStat)
}

func TestLedgerService_CountLines(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	history, err := lms.GetFileHistory(testFilePath, 10)
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

func TestLedgerService_GetFileHistory_DisabledVault(t *testing.T) {
	lms := NewLedgerService(nil, nil, testutil.NewTestLogger())

	history, err := lms.GetFileHistory("/some/file", 10)
	assert.Error(t, err)
	assert.Nil(t, history)
	assert.Contains(t, err.Error(), "disabled")
}

func TestLedgerService_GetFileHistory_DefaultLimit(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	// Create a file
	testFilePath := filepath.Join(tempDir, "limit_test.txt")
	result, _ := lms.MirrorFileCreate("operator_session", testFilePath)
	os.WriteFile(testFilePath, []byte("content"), 0644)
	lms.CompleteMirrorCreate(result, "operator_session")

	// Get history with zero limit (should default to 50)
	history, err := lms.GetFileHistory(testFilePath, 0)
	require.NoError(t, err)
	assert.NotNil(t, history)
}

func TestLedgerService_GetFileAtCommit(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	content, err := lms.GetFileAtCommit(testFilePath, initialHash)
	require.NoError(t, err)
	assert.Equal(t, "Initial content", content)

	// Current file should be different
	currentContent, _ := os.ReadFile(testFilePath)
	assert.Equal(t, "Modified content", string(currentContent))
}

func TestLedgerService_GetFileAtCommit_DisabledVault(t *testing.T) {
	lms := NewLedgerService(nil, nil, testutil.NewTestLogger())

	content, err := lms.GetFileAtCommit("/some/file", "abc123")
	assert.Error(t, err)
	assert.Empty(t, content)
	assert.Contains(t, err.Error(), "disabled")
}

func TestLedgerService_RestoreFileFromCommit(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	lms := NewLedgerService(nil, nil, testutil.NewTestLogger())

	err := lms.RestoreFileFromCommit("/some/file", "abc123", "operator_session")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

func TestLedgerService_RestoreFileFromCommit_InvalidCommit(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	testFilePath := filepath.Join(tempDir, "invalid_restore.txt")

	err := lms.RestoreFileFromCommit(testFilePath, "invalidhash123", "operator_session")
	assert.Error(t, err)
}

func TestLedgerService_CompleteWorkflow(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	history, err := lms.GetFileHistory(testFilePath, 10)
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

func TestLedgerService_ConcurrentMirrorOperations(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

	// Test that mutex properly serializes operations
	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(idx int) {
			filePath := filepath.Join(tempDir, "concurrent_test.txt")
			operatorSessionID := "concurrent-session"

			result, _ := lms.LedgerFileWrite(operatorSessionID, filePath)
			os.WriteFile(filePath, []byte("content"), 0644)
			lms.CompleteMirrorWrite(result, operatorSessionID)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// No panics or race conditions = pass
}

func TestLedgerService_LargeFile(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	assert.Equal(t, len(largeContent), len(mirrorContent))
	assert.Equal(t, largeContent, mirrorContent)
}

func TestLedgerService_SpecialCharactersInPath(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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

func TestLedgerService_BinaryFile(t *testing.T) {
	lms, avs, tempDir := setupTestLedger(t)
	defer avs.Close()

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
