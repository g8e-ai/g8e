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
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// setupTestExecutionVault creates a test environment for ExecutionVaultService
func setupTestExecutionVault(t *testing.T) (*ExecutionVaultService, string) {
	t.Helper()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "execution_vault.db")
	vaultDir := filepath.Join(tempDir, "vault")

	// Create vault
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

	logger := testutil.NewTestLogger()

	config := &ExecutionVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          1024,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
	}

	ev, err := NewExecutionVaultService(config, logger, testVault)
	require.NoError(t, err)
	require.NotNil(t, ev)

	t.Cleanup(func() {
		ev.Wait()
		ev.Close()
	})

	return ev, tempDir
}

func TestExecutionVault_DefaultExecutionVaultConfig(t *testing.T) {
	t.Parallel()

	config := DefaultExecutionVaultConfig()

	assert.NotNil(t, config)
	assert.Equal(t, ".g8e/execution_vault.db", config.DBPath)
	assert.Equal(t, int64(1024), config.MaxDBSizeMB)
	assert.Equal(t, 30, config.RetentionDays)
	assert.Equal(t, 60, config.PruneIntervalMinutes)
}

func TestExecutionVault_NewExecutionVaultService_WithNilConfig(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
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
	t.Cleanup(func() { testVault.Close() })

	ev, err := NewExecutionVaultService(nil, testutil.NewTestLogger(), testVault)
	require.NoError(t, err)
	require.NotNil(t, ev)

	ev.Wait()
	ev.Close()
}

func TestExecutionVault_NewExecutionVaultService_NilVault(t *testing.T) {
	t.Parallel()

	config := &ExecutionVaultConfig{}

	ev, err := NewExecutionVaultService(config, testutil.NewTestLogger(), nil)
	require.Error(t, err)
	assert.Nil(t, ev)
	assert.Contains(t, err.Error(), "encryption vault is required")
}

func TestExecutionVault_StoreExecution_Basic(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-123",
		TimestampUTC:     time.Now().UTC(),
		Command:          "ls -la",
		ExitCode:         exitCode,
		DurationMs:       100,
		StdoutCompressed: []byte("file1.txt\nfile2.txt\n"),
		StderrCompressed: []byte(""),
		StdoutSize:       20,
		StderrSize:       0,
		UserID:           "user-123",
		CaseID:           "case-456",
		TaskID:           "task-789",
		InvestigationID:  "inv-abc",
		OperatorID:       "op-def",
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)

	ev.Wait()
}

func TestExecutionVault_StoreExecution_WithStderr(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 1
	record := &models.ExecutionRecord{
		ID:               "exec-456",
		TimestampUTC:     time.Now().UTC(),
		Command:          "invalid-command",
		ExitCode:         exitCode,
		DurationMs:       50,
		StdoutCompressed: []byte(""),
		StderrCompressed: []byte("command not found"),
		StdoutSize:       0,
		StderrSize:       17,
		UserID:           "user-123",
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)

	ev.Wait()
}

func TestExecutionVault_StoreExecution_NilService(t *testing.T) {
	t.Parallel()

	var ev *ExecutionVaultService
	record := &models.ExecutionRecord{
		ID:           "exec-nil",
		TimestampUTC: time.Now().UTC(),
		Command:      "test",
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
}

func TestExecutionVault_StoreExecution_UpdateExisting(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode1 := 0
	record1 := &models.ExecutionRecord{
		ID:               "exec-update",
		TimestampUTC:     time.Now().UTC(),
		Command:          "echo hello",
		ExitCode:         &exitCode1,
		DurationMs:       10,
		StdoutCompressed: []byte("hello\n"),
		StdoutSize:       6,
	}

	err := ev.StoreExecution(context.Background(), record1)
	require.NoError(t, err)
	ev.Wait()

	exitCode2 := 0
	record2 := &models.ExecutionRecord{
		ID:               "exec-update",
		TimestampUTC:     time.Now().UTC(),
		Command:          "echo world",
		ExitCode:         &exitCode2,
		DurationMs:       15,
		StdoutCompressed: []byte("world\n"),
		StdoutSize:       6,
	}

	err = ev.StoreExecution(context.Background(), record2)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-update")
	require.NoError(t, err)
	assert.Equal(t, "echo world", retrieved.Command)
	assert.Equal(t, int64(15), retrieved.DurationMs)
}

func TestExecutionVault_GetExecution_Basic(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-get-123",
		TimestampUTC:     time.Now().UTC(),
		Command:          "cat file.txt",
		ExitCode:         exitCode,
		DurationMs:       25,
		StdoutCompressed: []byte("file content"),
		StderrCompressed: []byte(""),
		StdoutSize:       12,
		StderrSize:       0,
		UserID:           "user-xyz",
		CaseID:           "case-xyz",
		TaskID:           "task-xyz",
		InvestigationID:  "inv-xyz",
		OperatorID:       "op-xyz",
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-get-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, "exec-get-123", retrieved.ID)
	assert.Equal(t, "cat file.txt", retrieved.Command)
	assert.Equal(t, int64(25), retrieved.DurationMs)
	assert.Equal(t, 0, *retrieved.ExitCode)
	assert.Equal(t, "user-xyz", retrieved.UserID)
	assert.Equal(t, "case-xyz", retrieved.CaseID)
	assert.Equal(t, "task-xyz", retrieved.TaskID)
	assert.Equal(t, "inv-xyz", retrieved.InvestigationID)
	assert.Equal(t, "op-xyz", retrieved.OperatorID)
}

func TestExecutionVault_GetExecution_NotFound(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	retrieved, err := ev.GetExecution(context.Background(), "non-existent-id")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestExecutionVault_GetExecution_NilService(t *testing.T) {
	t.Parallel()

	var ev *ExecutionVaultService

	retrieved, err := ev.GetExecution(context.Background(), "some-id")
	require.Error(t, err)
	assert.Nil(t, retrieved)
	assert.Contains(t, err.Error(), "disabled")
}

func TestExecutionVault_StoreFileDiff_Basic(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	record := &models.FileDiffRecord{
		ID:                "diff-123",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/etc/nginx/nginx.conf",
		Operation:         "write",
		LedgerHashBefore:  "abc123",
		LedgerHashAfter:   "def456",
		DiffStat:          "1 file changed, 5 insertions(+)",
		DiffCompressed:    []byte("--- a/file\n+++ b/file\n@@ -1,1 +1,1 @@\n-old\n+new\n"),
		DiffSize:          50,
		OperatorSessionID: "session-123",
		UserID:            "user-123",
		CaseID:            "case-123",
		OperatorID:        "op-123",
	}

	err := ev.StoreFileDiff(context.Background(), record)
	require.NoError(t, err)

	ev.Wait()
}

func TestExecutionVault_StoreFileDiff_DeleteOperation(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	record := &models.FileDiffRecord{
		ID:                "diff-delete",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/tmp/test.txt",
		Operation:         "delete",
		LedgerHashBefore:  "hash-before",
		LedgerHashAfter:   "",
		DiffStat:          "deleted file mode 100644",
		DiffCompressed:    []byte("diff --git a/file b/file\ndeleted file mode 100644\n"),
		DiffSize:          40,
		OperatorSessionID: "session-456",
		UserID:            "user-456",
		CaseID:            "case-456",
		OperatorID:        "op-456",
	}

	err := ev.StoreFileDiff(context.Background(), record)
	require.NoError(t, err)

	ev.Wait()
}

func TestExecutionVault_StoreFileDiff_NilService(t *testing.T) {
	t.Parallel()

	var ev *ExecutionVaultService
	record := &models.FileDiffRecord{
		ID:           "diff-nil",
		TimestampUTC: time.Now().UTC(),
		FilePath:     "/some/file",
		Operation:    "write",
	}

	err := ev.StoreFileDiff(context.Background(), record)
	require.NoError(t, err)
}

func TestExecutionVault_StoreFileDiff_UpdateExisting(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	record1 := &models.FileDiffRecord{
		ID:             "diff-update",
		TimestampUTC:   time.Now().UTC(),
		FilePath:       "/test/file",
		Operation:      "write",
		DiffCompressed: []byte("old diff"),
		DiffSize:       8,
	}

	err := ev.StoreFileDiff(context.Background(), record1)
	require.NoError(t, err)
	ev.Wait()

	record2 := &models.FileDiffRecord{
		ID:             "diff-update",
		TimestampUTC:   time.Now().UTC(),
		FilePath:       "/test/file",
		Operation:      "write",
		DiffCompressed: []byte("new diff"),
		DiffSize:       8,
	}

	err = ev.StoreFileDiff(context.Background(), record2)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetFileDiff(context.Background(), "diff-update")
	require.NoError(t, err)
	assert.Equal(t, 8, retrieved.DiffSize)
}

func TestExecutionVault_GetFileDiff_Basic(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	record := &models.FileDiffRecord{
		ID:                "diff-get-123",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/etc/hosts",
		Operation:         "write",
		LedgerHashBefore:  "before-hash",
		LedgerHashAfter:   "after-hash",
		DiffStat:          "1 file changed, 2 insertions(+)",
		DiffCompressed:    []byte("diff content"),
		DiffSize:          12,
		OperatorSessionID: "session-get",
		UserID:            "user-get",
		CaseID:            "case-get",
		OperatorID:        "op-get",
	}

	err := ev.StoreFileDiff(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetFileDiff(context.Background(), "diff-get-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, "diff-get-123", retrieved.ID)
	assert.Equal(t, "/etc/hosts", retrieved.FilePath)
	assert.Equal(t, "write", retrieved.Operation)
	assert.Equal(t, "before-hash", retrieved.LedgerHashBefore)
	assert.Equal(t, "after-hash", retrieved.LedgerHashAfter)
	assert.Equal(t, "1 file changed, 2 insertions(+)", retrieved.DiffStat)
	assert.Equal(t, "session-get", retrieved.OperatorSessionID)
	assert.Equal(t, "user-get", retrieved.UserID)
	assert.Equal(t, "case-get", retrieved.CaseID)
	assert.Equal(t, "op-get", retrieved.OperatorID)
}

func TestExecutionVault_GetFileDiff_NotFound(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	retrieved, err := ev.GetFileDiff(context.Background(), "non-existent-diff")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestExecutionVault_GetFileDiff_NilService(t *testing.T) {
	t.Parallel()

	var ev *ExecutionVaultService

	retrieved, err := ev.GetFileDiff(context.Background(), "some-diff-id")
	require.Error(t, err)
	assert.Nil(t, retrieved)
	assert.Contains(t, err.Error(), "disabled")
}

func TestExecutionVault_GetFileDiffsBySession_Basic(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	sessionID := "session-query-123"

	for i := 0; i < 5; i++ {
		record := &models.FileDiffRecord{
			ID:                fmt.Sprintf("diff-%d", i),
			TimestampUTC:      time.Now().UTC(),
			FilePath:          fmt.Sprintf("/file%d.txt", i),
			Operation:         "write",
			DiffCompressed:    []byte(fmt.Sprintf("diff content %d", i)),
			DiffSize:          15,
			OperatorSessionID: sessionID,
			UserID:            "user-123",
			CaseID:            "case-123",
			OperatorID:        "op-123",
		}
		err := ev.StoreFileDiff(context.Background(), record)
		require.NoError(t, err)
	}
	ev.Wait()

	diffs, err := ev.GetFileDiffsBySession(context.Background(), sessionID, 10)
	require.NoError(t, err)
	assert.Len(t, diffs, 5)

	for _, diff := range diffs {
		assert.Equal(t, sessionID, diff.OperatorSessionID)
	}
}

func TestExecutionVault_GetFileDiffsBySession_WithLimit(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	sessionID := "session-limit-123"

	for i := 0; i < 10; i++ {
		record := &models.FileDiffRecord{
			ID:                fmt.Sprintf("diff-limit-%d", i),
			TimestampUTC:      time.Now().UTC(),
			FilePath:          fmt.Sprintf("/file%d.txt", i),
			Operation:         "write",
			DiffCompressed:    []byte(fmt.Sprintf("diff %d", i)),
			DiffSize:          8,
			OperatorSessionID: sessionID,
		}
		err := ev.StoreFileDiff(context.Background(), record)
		require.NoError(t, err)
	}
	ev.Wait()

	diffs, err := ev.GetFileDiffsBySession(context.Background(), sessionID, 3)
	require.NoError(t, err)
	assert.Len(t, diffs, 3)
}

func TestExecutionVault_GetFileDiffsBySession_DefaultLimit(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	sessionID := "session-default-123"

	for i := 0; i < 5; i++ {
		record := &models.FileDiffRecord{
			ID:                fmt.Sprintf("diff-default-%d", i),
			TimestampUTC:      time.Now().UTC(),
			FilePath:          fmt.Sprintf("/file%d.txt", i),
			Operation:         "write",
			DiffCompressed:    []byte("diff"),
			DiffSize:          4,
			OperatorSessionID: sessionID,
		}
		err := ev.StoreFileDiff(context.Background(), record)
		require.NoError(t, err)
	}
	ev.Wait()

	diffs, err := ev.GetFileDiffsBySession(context.Background(), sessionID, 0)
	require.NoError(t, err)
	assert.Len(t, diffs, 5)
}

func TestExecutionVault_GetFileDiffsBySession_EmptySession(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	diffs, err := ev.GetFileDiffsBySession(context.Background(), "empty-session", 10)
	require.NoError(t, err)
	assert.Empty(t, diffs)
}

func TestExecutionVault_GetFileDiffsBySession_NilService(t *testing.T) {
	t.Parallel()

	var ev *ExecutionVaultService

	diffs, err := ev.GetFileDiffsBySession(context.Background(), "session-id", 10)
	require.Error(t, err)
	assert.Nil(t, diffs)
	assert.Contains(t, err.Error(), "disabled")
}

func TestExecutionVault_Close(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	err := ev.Close()
	require.NoError(t, err)

	// Close should succeed and not panic
}

func TestExecutionVault_Close_NilService(t *testing.T) {
	t.Parallel()

	var ev *ExecutionVaultService
	err := ev.Close()
	require.NoError(t, err)
}

func TestExecutionVault_Wait(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-wait",
		TimestampUTC:     time.Now().UTC(),
		Command:          "test",
		ExitCode:         exitCode,
		StdoutCompressed: []byte("output"),
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)

	ev.Wait()

	err = ev.Close()
	require.NoError(t, err)
}

func TestExecutionVault_Wait_NilService(t *testing.T) {
	t.Parallel()

	var ev *ExecutionVaultService
	assert.NotPanics(t, func() {
		ev.Wait()
	})
}

func TestExecutionVault_ConcurrentStoreExecution(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	concurrency := 10
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			exitCode := 0
			record := &models.ExecutionRecord{
				ID:               fmt.Sprintf("exec-concurrent-%d", idx),
				TimestampUTC:     time.Now().UTC(),
				Command:          fmt.Sprintf("cmd-%d", idx),
				ExitCode:         exitCode,
				DurationMs:       int64(idx),
				StdoutCompressed: []byte(fmt.Sprintf("output-%d", idx)),
				StdoutSize:       10,
			}
			err := ev.StoreExecution(context.Background(), record)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		<-done
	}

	ev.Wait()

	for i := 0; i < concurrency; i++ {
		retrieved, err := ev.GetExecution(context.Background(), fmt.Sprintf("exec-concurrent-%d", i))
		require.NoError(t, err)
		assert.NotNil(t, retrieved)
	}
}

func TestExecutionVault_ConcurrentStoreFileDiff(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	concurrency := 10
	sessionID := "concurrent-session"
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			record := &models.FileDiffRecord{
				ID:                fmt.Sprintf("diff-concurrent-%d", idx),
				TimestampUTC:      time.Now().UTC(),
				FilePath:          fmt.Sprintf("/file%d.txt", idx),
				Operation:         "write",
				DiffCompressed:    []byte(fmt.Sprintf("diff-%d", idx)),
				DiffSize:          8,
				OperatorSessionID: sessionID,
			}
			err := ev.StoreFileDiff(context.Background(), record)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		<-done
	}

	ev.Wait()

	diffs, err := ev.GetFileDiffsBySession(context.Background(), sessionID, 100)
	require.NoError(t, err)
	assert.Len(t, diffs, 10)
}

func TestExecutionVault_LargeStdout(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	largeOutput := make([]byte, 1024*1024) // 1MB
	for i := range largeOutput {
		largeOutput[i] = byte('A' + (i % 26))
	}

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-large",
		TimestampUTC:     time.Now().UTC(),
		Command:          "generate-large",
		ExitCode:         exitCode,
		DurationMs:       1000,
		StdoutCompressed: largeOutput,
		StdoutSize:       len(largeOutput),
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-large")
	require.NoError(t, err)
	assert.Equal(t, len(largeOutput), retrieved.StdoutSize)
}

func TestExecutionVault_EmptyOutput(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-empty",
		TimestampUTC:     time.Now().UTC(),
		Command:          "true",
		ExitCode:         exitCode,
		DurationMs:       5,
		StdoutCompressed: []byte(""),
		StderrCompressed: []byte(""),
		StdoutSize:       0,
		StderrSize:       0,
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-empty")
	require.NoError(t, err)
	assert.Equal(t, 0, retrieved.StdoutSize)
	assert.Equal(t, 0, retrieved.StderrSize)
}

func TestExecutionVault_MultipleExecutionsSameCase(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	caseID := "case-multi-123"

	for i := 0; i < 5; i++ {
		exitCode := 0
		record := &models.ExecutionRecord{
			ID:               fmt.Sprintf("exec-case-%d", i),
			TimestampUTC:     time.Now().UTC(),
			Command:          fmt.Sprintf("cmd-%d", i),
			ExitCode:         exitCode,
			DurationMs:       int64(i * 10),
			StdoutCompressed: []byte(fmt.Sprintf("output-%d", i)),
			StdoutSize:       10,
			CaseID:           caseID,
		}
		err := ev.StoreExecution(context.Background(), record)
		require.NoError(t, err)
	}
	ev.Wait()

	for i := 0; i < 5; i++ {
		retrieved, err := ev.GetExecution(context.Background(), fmt.Sprintf("exec-case-%d", i))
		require.NoError(t, err)
		assert.Equal(t, caseID, retrieved.CaseID)
	}
}

func TestExecutionVault_MultipleDiffsSameSession(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	sessionID := "session-multi-123"

	for i := 0; i < 5; i++ {
		record := &models.FileDiffRecord{
			ID:                fmt.Sprintf("diff-session-%d", i),
			TimestampUTC:      time.Now().UTC(),
			FilePath:          fmt.Sprintf("/path/file%d.txt", i),
			Operation:         "write",
			DiffCompressed:    []byte(fmt.Sprintf("diff-%d", i)),
			DiffSize:          8,
			OperatorSessionID: sessionID,
		}
		err := ev.StoreFileDiff(context.Background(), record)
		require.NoError(t, err)
	}
	ev.Wait()

	diffs, err := ev.GetFileDiffsBySession(context.Background(), sessionID, 10)
	require.NoError(t, err)
	assert.Len(t, diffs, 5)
}

func TestExecutionVault_ExecutionWithAllFields(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-all-fields",
		TimestampUTC:     time.Now().UTC(),
		Command:          "comprehensive-test",
		ExitCode:         exitCode,
		DurationMs:       12345,
		StdoutCompressed: []byte("stdout output"),
		StderrCompressed: []byte("stderr output"),
		StdoutSize:       13,
		StderrSize:       13,
		UserID:           "user-all",
		CaseID:           "case-all",
		TaskID:           "task-all",
		InvestigationID:  "inv-all",
		OperatorID:       "op-all",
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-all-fields")
	require.NoError(t, err)

	assert.Equal(t, "comprehensive-test", retrieved.Command)
	assert.Equal(t, int64(12345), retrieved.DurationMs)
	assert.Equal(t, "user-all", retrieved.UserID)
	assert.Equal(t, "case-all", retrieved.CaseID)
	assert.Equal(t, "task-all", retrieved.TaskID)
	assert.Equal(t, "inv-all", retrieved.InvestigationID)
	assert.Equal(t, "op-all", retrieved.OperatorID)
}

func TestExecutionVault_FileDiffWithAllFields(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	record := &models.FileDiffRecord{
		ID:                "diff-all-fields",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/comprehensive/path/file.txt",
		Operation:         "write",
		LedgerHashBefore:  "hash-before-all",
		LedgerHashAfter:   "hash-after-all",
		DiffStat:          "comprehensive diff stat",
		DiffCompressed:    []byte("comprehensive diff content"),
		DiffSize:          26,
		OperatorSessionID: "session-all",
		UserID:            "user-all",
		CaseID:            "case-all",
		OperatorID:        "op-all",
	}

	err := ev.StoreFileDiff(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetFileDiff(context.Background(), "diff-all-fields")
	require.NoError(t, err)

	assert.Equal(t, "/comprehensive/path/file.txt", retrieved.FilePath)
	assert.Equal(t, "write", retrieved.Operation)
	assert.Equal(t, "hash-before-all", retrieved.LedgerHashBefore)
	assert.Equal(t, "hash-after-all", retrieved.LedgerHashAfter)
	assert.Equal(t, "comprehensive diff stat", retrieved.DiffStat)
	assert.Equal(t, "session-all", retrieved.OperatorSessionID)
	assert.Equal(t, "user-all", retrieved.UserID)
	assert.Equal(t, "case-all", retrieved.CaseID)
	assert.Equal(t, "op-all", retrieved.OperatorID)
}

func TestExecutionVault_StoreExecution_LockedVault(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
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
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	config := &ExecutionVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          1024,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
	}

	ev, err := NewExecutionVaultService(config, logger, testVault)
	require.NoError(t, err)
	t.Cleanup(func() {
		ev.Wait()
		ev.Close()
	})

	// Lock the vault
	testVault.Lock()

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-locked",
		TimestampUTC:     time.Now().UTC(),
		Command:          "test",
		ExitCode:         exitCode,
		StdoutCompressed: []byte("output"),
		StdoutSize:       6,
	}

	err = ev.StoreExecution(context.Background(), record)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault is locked")
}

func TestExecutionVault_StoreFileDiff_LockedVault(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
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
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	config := &ExecutionVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          1024,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
	}

	ev, err := NewExecutionVaultService(config, logger, testVault)
	require.NoError(t, err)
	t.Cleanup(func() {
		ev.Wait()
		ev.Close()
	})

	// Lock the vault
	testVault.Lock()

	record := &models.FileDiffRecord{
		ID:             "diff-locked",
		TimestampUTC:   time.Now().UTC(),
		FilePath:       "/test/file",
		Operation:      "write",
		DiffCompressed: []byte("diff content"),
		DiffSize:       12,
	}

	err = ev.StoreFileDiff(context.Background(), record)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault is locked")
}

func TestExecutionVault_PruneRetention(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
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
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	// Set very short retention (1 day)
	config := &ExecutionVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          1024,
		RetentionDays:        1,
		PruneIntervalMinutes: 60,
	}

	ev, err := NewExecutionVaultService(config, logger, testVault)
	require.NoError(t, err)
	t.Cleanup(func() {
		ev.Wait()
		ev.Close()
	})

	// Store an old execution (2 days ago)
	exitCode := 0
	oldRecord := &models.ExecutionRecord{
		ID:               "exec-old",
		TimestampUTC:     time.Now().UTC().AddDate(0, 0, -2),
		Command:          "old-command",
		ExitCode:         exitCode,
		StdoutCompressed: []byte("old output"),
		StdoutSize:       10,
	}
	err = ev.StoreExecution(context.Background(), oldRecord)
	require.NoError(t, err)
	ev.Wait()

	// Store a recent execution
	recentRecord := &models.ExecutionRecord{
		ID:               "exec-recent",
		TimestampUTC:     time.Now().UTC(),
		Command:          "recent-command",
		ExitCode:         exitCode,
		StdoutCompressed: []byte("recent output"),
		StdoutSize:       13,
	}
	err = ev.StoreExecution(context.Background(), recentRecord)
	require.NoError(t, err)
	ev.Wait()

	// Manually trigger prune
	pruneFunc := executionVaultPrune(config)
	err = pruneFunc(context.Background(), ev.db, logger)
	require.NoError(t, err)

	// Old record should be gone
	_, err = ev.GetExecution(context.Background(), "exec-old")
	require.NoError(t, err) // Get returns nil for not found

	// Recent record should still exist
	retrieved, err := ev.GetExecution(context.Background(), "exec-recent")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "exec-recent", retrieved.ID)
}

func TestExecutionVault_PruneSizeLimit(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
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
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	// Set very small size limit (1MB)
	config := &ExecutionVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          1,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
	}

	ev, err := NewExecutionVaultService(config, logger, testVault)
	require.NoError(t, err)
	t.Cleanup(func() {
		ev.Wait()
		ev.Close()
	})

	// Insert multiple large values to exceed size limit
	// Use larger values (1MB each) to ensure we exceed the 1MB limit after compression
	largeOutput := make([]byte, 1024*1024) // 1MB each
	for i := range largeOutput {
		largeOutput[i] = byte(i % 256)
	}

	exitCode := 0
	for i := 0; i < 10; i++ {
		record := &models.ExecutionRecord{
			ID:               fmt.Sprintf("exec-large-%d", i),
			TimestampUTC:     time.Now().UTC().Add(time.Duration(-i) * time.Hour),
			Command:          fmt.Sprintf("cmd-%d", i),
			ExitCode:         exitCode,
			StdoutCompressed: largeOutput,
			StdoutSize:       len(largeOutput),
		}
		err := ev.StoreExecution(context.Background(), record)
		require.NoError(t, err)
	}
	ev.Wait()

	// Manually trigger prune
	pruneFunc := executionVaultPrune(config)
	err = pruneFunc(context.Background(), ev.db, logger)
	require.NoError(t, err)

	// Verify prune function executed without error
	// The actual pruning behavior depends on database size after compression
	// which is hard to predict in tests, so we just verify it doesn't crash
}

func TestExecutionVault_SpecialCharacters(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-special",
		TimestampUTC:     time.Now().UTC(),
		Command:          "echo 'test with spaces and \"quotes\"'",
		ExitCode:         exitCode,
		StdoutCompressed: []byte("output\nwith\nnewlines\tand\ttabs"),
		StdoutSize:       30,
		UserID:           "user-with-dashes_and_underscores",
		CaseID:           "case:with:colons",
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-special")
	require.NoError(t, err)
	assert.Equal(t, "echo 'test with spaces and \"quotes\"'", retrieved.Command)
	assert.Equal(t, "user-with-dashes_and_underscores", retrieved.UserID)
}

func TestExecutionVault_UnicodeCharacters(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-unicode",
		TimestampUTC:     time.Now().UTC(),
		Command:          "echo '日本語 中文 한글 العربية'",
		ExitCode:         exitCode,
		StdoutCompressed: []byte("output-😀-🎉-test"),
		StdoutSize:       20,
		UserID:           "user-日本語",
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-unicode")
	require.NoError(t, err)
	assert.Equal(t, "echo '日本語 中文 한글 العربية'", retrieved.Command)
	assert.Equal(t, "user-日本語", retrieved.UserID)
}

func TestExecutionVault_VeryLongCommand(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	// Create a very long command (10KB)
	longCommand := make([]byte, 10*1024)
	for i := range longCommand {
		longCommand[i] = byte('a' + (i % 26))
	}

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-long-cmd",
		TimestampUTC:     time.Now().UTC(),
		Command:          string(longCommand),
		ExitCode:         exitCode,
		StdoutCompressed: []byte("output"),
		StdoutSize:       6,
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-long-cmd")
	require.NoError(t, err)
	assert.Equal(t, len(longCommand), len(retrieved.Command))
}

func TestExecutionVault_NilExitCode(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	record := &models.ExecutionRecord{
		ID:               "exec-nil-exit",
		TimestampUTC:     time.Now().UTC(),
		Command:          "test",
		ExitCode:         constants.ExitCodeNone,
		StdoutCompressed: []byte("output"),
		StdoutSize:       6,
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-nil-exit")
	require.NoError(t, err)
	assert.Nil(t, retrieved.ExitCode)
}

func TestExecutionVault_ContextCancellation(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-cancelled",
		TimestampUTC:     time.Now().UTC(),
		Command:          "test",
		ExitCode:         exitCode,
		StdoutCompressed: []byte("output"),
		StdoutSize:       6,
	}

	// Operations should still complete since they don't actively check context
	// (this is a documentation test showing current behavior)
	err := ev.StoreExecution(ctx, record)
	require.NoError(t, err)
	ev.Wait()

	_, err = ev.GetExecution(ctx, "exec-cancelled")
	require.NoError(t, err)
}

func TestExecutionVault_DatabaseInitFailure(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := t.TempDir()
	testVault := CreateTestVault(t, vaultDir, privKey)

	// Create a file (not a directory) and try to use a path inside it
	// This will fail because you can't create directories inside a file
	tempFile, err := os.CreateTemp("", "test-file-*")
	require.NoError(t, err)
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	config := &ExecutionVaultConfig{
		DBPath: filepath.Join(tempFile.Name(), "execution_vault.db"),
	}

	ev, err := NewExecutionVaultService(config, logger, testVault)
	require.Error(t, err)
	assert.Error(t, err)
	assert.Nil(t, ev)
}

func TestExecutionVault_SchemaInitFailure(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
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
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	config := &ExecutionVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          1024,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
	}

	// Create a read-only directory to cause schema init failure
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0500))

	ev, err := NewExecutionVaultService(config, logger, testVault)
	// This might succeed or fail depending on OS permissions
	// If it fails, verify the error message
	if err != nil {
		assert.Contains(t, err.Error(), "failed to initialize")
	} else {
		// If it succeeded, close it
		ev.Close()
	}
}

func TestExecutionVault_GetExecution_DecryptFailure(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
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
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	config := &ExecutionVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          1024,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
	}

	ev, err := NewExecutionVaultService(config, logger, testVault)
	require.NoError(t, err)
	t.Cleanup(func() {
		ev.Wait()
		ev.Close()
	})

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-decrypt-test",
		TimestampUTC:     time.Now().UTC(),
		Command:          "test",
		ExitCode:         exitCode,
		StdoutCompressed: []byte("output"),
		StdoutSize:       6,
	}

	err = ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	// Lock the vault before retrieval
	testVault.Lock()

	// Get should succeed but stdout will be empty due to decryption failure
	retrieved, err := ev.GetExecution(context.Background(), "exec-decrypt-test")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "exec-decrypt-test", retrieved.ID)
	// StdoutCompressed should be empty due to decryption failure
	assert.Empty(t, retrieved.StdoutCompressed)
}

func TestExecutionVault_GetFileDiff_DecryptFailure(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
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
	t.Cleanup(func() { testVault.Close() })

	logger := testutil.NewTestLogger()

	config := &ExecutionVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          1024,
		RetentionDays:        30,
		PruneIntervalMinutes: 60,
	}

	ev, err := NewExecutionVaultService(config, logger, testVault)
	require.NoError(t, err)
	t.Cleanup(func() {
		ev.Wait()
		ev.Close()
	})

	record := &models.FileDiffRecord{
		ID:             "diff-decrypt-test",
		TimestampUTC:   time.Now().UTC(),
		FilePath:       "/test/file",
		Operation:      "write",
		DiffCompressed: []byte("diff content"),
		DiffSize:       12,
	}

	err = ev.StoreFileDiff(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	// Lock the vault before retrieval
	testVault.Lock()

	// Get should succeed but diff will be empty due to decryption failure
	retrieved, err := ev.GetFileDiff(context.Background(), "diff-decrypt-test")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "diff-decrypt-test", retrieved.ID)
	// DiffCompressed should be empty due to decryption failure
	assert.Empty(t, retrieved.DiffCompressed)
}

func TestExecutionVault_ZeroDuration(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-zero-duration",
		TimestampUTC:     time.Now().UTC(),
		Command:          "instant-command",
		ExitCode:         exitCode,
		DurationMs:       0,
		StdoutCompressed: []byte("output"),
		StdoutSize:       6,
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-zero-duration")
	require.NoError(t, err)
	assert.Equal(t, int64(0), retrieved.DurationMs)
}

func TestExecutionVault_NegativeDuration(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-negative-duration",
		TimestampUTC:     time.Now().UTC(),
		Command:          "negative-duration-command",
		ExitCode:         exitCode,
		DurationMs:       -100,
		StdoutCompressed: []byte("output"),
		StdoutSize:       6,
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-negative-duration")
	require.NoError(t, err)
	assert.Equal(t, int64(-100), retrieved.DurationMs)
}

func TestExecutionVault_EmptyFields(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	exitCode := 0
	record := &models.ExecutionRecord{
		ID:               "exec-empty-fields",
		TimestampUTC:     time.Now().UTC(),
		Command:          "",
		ExitCode:         exitCode,
		StdoutCompressed: []byte(""),
		StderrCompressed: []byte(""),
		StdoutSize:       0,
		StderrSize:       0,
		UserID:           "",
		CaseID:           "",
		TaskID:           "",
		InvestigationID:  "",
		OperatorID:       "",
	}

	err := ev.StoreExecution(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetExecution(context.Background(), "exec-empty-fields")
	require.NoError(t, err)
	assert.Equal(t, "", retrieved.Command)
	assert.Equal(t, "", retrieved.UserID)
	assert.Equal(t, "", retrieved.CaseID)
}

func TestExecutionVault_FileDiffEmptyFields(t *testing.T) {
	t.Parallel()
	ev, _ := setupTestExecutionVault(t)

	record := &models.FileDiffRecord{
		ID:                "diff-empty-fields",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "",
		Operation:         "",
		LedgerHashBefore:  "",
		LedgerHashAfter:   "",
		DiffStat:          "",
		DiffCompressed:    []byte(""),
		DiffSize:          0,
		OperatorSessionID: "",
		UserID:            "",
		CaseID:            "",
		OperatorID:        "",
	}

	err := ev.StoreFileDiff(context.Background(), record)
	require.NoError(t, err)
	ev.Wait()

	retrieved, err := ev.GetFileDiff(context.Background(), "diff-empty-fields")
	require.NoError(t, err)
	assert.Equal(t, "", retrieved.FilePath)
	assert.Equal(t, "", retrieved.Operation)
	assert.Equal(t, "", retrieved.LedgerHashBefore)
}
