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
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/g8eo/testutil"
)

func TestNewLocalStoreService(t *testing.T) {
	logger := testutil.NewTestLogger()

	dbPath := filepath.Join(t.TempDir(), "test_local_state.db")
	config := DefaultLocalStoreConfig()
	config.DBPath = dbPath

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ls)
	defer ls.Close()

	_, err = os.Stat(dbPath)
	assert.NoError(t, err)
	assert.True(t, ls.IsEnabled())
}

func TestLocalStoreService_Disabled(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := &LocalStoreConfig{
		Enabled: false,
	}

	ls, err := NewLocalStoreService(config, logger)
	assert.NoError(t, err)
	assert.Nil(t, ls)
}

func TestLocalStoreService_StoreAndRetrieve(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := DefaultLocalStoreConfig()
	config.DBPath = filepath.Join(t.TempDir(), "test_store.db")

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ls)
	defer ls.Close()

	exitCode := 0
	record := &ExecutionRecord{
		ID:               "test-exec-123",
		TimestampUTC:     time.Now().UTC(),
		Command:          "echo 'Hello, World!'",
		ExitCode:         &exitCode,
		DurationMs:       150,
		StdoutCompressed: []byte("Hello, World!\n"),
		StderrCompressed: []byte(""),
		StdoutSize:       14,
		StderrSize:       0,
		UserID:           "user-456",
		CaseID:           "case-789",
		TaskID:           "task-abc",
		InvestigationID:  "inv-def",
		OperatorID:       "op-ghi",
	}

	err = ls.StoreExecution(record)
	require.NoError(t, err)

	retrieved, err := ls.GetExecution("test-exec-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, record.ID, retrieved.ID)
	assert.Equal(t, record.Command, retrieved.Command)
	assert.Equal(t, *record.ExitCode, *retrieved.ExitCode)
	assert.Equal(t, record.DurationMs, retrieved.DurationMs)
	assert.Equal(t, record.CaseID, retrieved.CaseID)
	assert.Equal(t, record.TaskID, retrieved.TaskID)
	assert.Equal(t, record.StdoutCompressed, retrieved.StdoutCompressed)
}

func TestLocalStoreService_HashConsistency(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := DefaultLocalStoreConfig()
	config.DBPath = filepath.Join(t.TempDir(), "test_hash.db")

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ls)
	defer ls.Close()

	testData := "Hello, World!"

	hash1 := ls.HashString(testData)
	hash2 := ls.HashString(testData)
	assert.Equal(t, hash1, hash2)
	assert.Len(t, hash1, 64)

	hash3 := ls.HashString("Different data")
	assert.NotEqual(t, hash1, hash3)
}

func TestLocalStoreService_UpsertBehavior(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := DefaultLocalStoreConfig()
	config.DBPath = filepath.Join(t.TempDir(), "test_upsert.db")

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ls)
	defer ls.Close()

	exitCode := 0
	record := &ExecutionRecord{
		ID:               "test-upsert-123",
		TimestampUTC:     time.Now().UTC(),
		Command:          "initial command",
		ExitCode:         &exitCode,
		DurationMs:       100,
		StdoutCompressed: []byte("initial output"),
		StdoutSize:       14,
		CaseID:           "case-upsert",
	}
	err = ls.StoreExecution(record)
	require.NoError(t, err)

	exitCode2 := 1
	record.Command = "updated command"
	record.ExitCode = &exitCode2
	record.StdoutCompressed = []byte("updated output")
	record.StdoutSize = 14

	err = ls.StoreExecution(record)
	require.NoError(t, err)

	retrieved, err := ls.GetExecution("test-upsert-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, "updated command", retrieved.Command)
	assert.Equal(t, 1, *retrieved.ExitCode)
}

func TestLocalStoreService_NonExistentRecord(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := DefaultLocalStoreConfig()
	config.DBPath = filepath.Join(t.TempDir(), "test_nonexistent.db")

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ls)
	defer ls.Close()

	record, err := ls.GetExecution("does-not-exist")
	assert.NoError(t, err)
	assert.Nil(t, record)
}

func TestLocalStoreService_NilSafety(t *testing.T) {
	var ls *LocalStoreService

	assert.False(t, ls.IsEnabled())

	err := ls.StoreExecution(&ExecutionRecord{ID: "test"})
	assert.NoError(t, err)

	err = ls.Close()
	assert.NoError(t, err)
}

func TestDefaultLocalStoreConfig(t *testing.T) {
	t.Run("returns valid default config", func(t *testing.T) {
		cfg := DefaultLocalStoreConfig()

		assert.NotNil(t, cfg)
		assert.True(t, cfg.Enabled)
		assert.NotEmpty(t, cfg.DBPath)
		assert.Greater(t, cfg.MaxDBSizeMB, int64(0))
		assert.Greater(t, cfg.RetentionDays, 0)
		assert.Greater(t, cfg.PruneIntervalMinutes, 0)
	})
}

func TestLocalStoreService_StoreAndRetrieveFileDiff(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := DefaultLocalStoreConfig()
	config.DBPath = filepath.Join(t.TempDir(), "test_file_diff.db")

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ls)
	defer ls.Close()

	diffContent := "--- a/etc/nginx/nginx.conf\n+++ b/etc/nginx/nginx.conf\n@@ -10,5 +10,8 @@\n-old line\n+new line"

	record := &FileDiffRecord{
		ID:                "diff-test-123",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/etc/nginx/nginx.conf",
		Operation:         "replace",
		LedgerHashBefore:  "abc123",
		LedgerHashAfter:   "def456",
		DiffStat:          "+1/-1",
		DiffCompressed:    []byte(diffContent),
		DiffSize:          len(diffContent),
		OperatorSessionID: "web_session_abc",
		UserID:            "user-123",
		CaseID:            "case-456",
		OperatorID:        "op-789",
	}

	err = ls.StoreFileDiff(record)
	require.NoError(t, err)

	retrieved, err := ls.GetFileDiff("diff-test-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, record.ID, retrieved.ID)
	assert.Equal(t, record.FilePath, retrieved.FilePath)
	assert.Equal(t, record.Operation, retrieved.Operation)
	assert.Equal(t, record.LedgerHashBefore, retrieved.LedgerHashBefore)
	assert.Equal(t, record.LedgerHashAfter, retrieved.LedgerHashAfter)
	assert.Equal(t, record.DiffStat, retrieved.DiffStat)
	assert.Equal(t, record.DiffSize, retrieved.DiffSize)
	assert.Equal(t, record.OperatorSessionID, retrieved.OperatorSessionID)
	assert.Equal(t, record.UserID, retrieved.UserID)
	assert.Equal(t, record.CaseID, retrieved.CaseID)
	assert.Equal(t, record.OperatorID, retrieved.OperatorID)
	assert.Equal(t, diffContent, string(retrieved.DiffCompressed))
}

func TestLocalStoreService_GetFileDiffsBySession(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := DefaultLocalStoreConfig()
	config.DBPath = filepath.Join(t.TempDir(), "test_file_diff_session.db")

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ls)
	defer ls.Close()

	operatorSessionID := "web_session_test_123"

	for i := 0; i < 5; i++ {
		record := &FileDiffRecord{
			ID:                "diff-session-" + string(rune('A'+i)),
			TimestampUTC:      time.Now().UTC().Add(time.Duration(-i) * time.Minute),
			FilePath:          "/path/to/file" + string(rune('A'+i)) + ".txt",
			Operation:         "write",
			DiffStat:          "+5/-0",
			DiffCompressed:    []byte("diff content " + string(rune('A'+i))),
			DiffSize:          15,
			OperatorSessionID: operatorSessionID,
		}
		err = ls.StoreFileDiff(record)
		require.NoError(t, err)
	}

	record := &FileDiffRecord{
		ID:                "diff-other-session",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/other/file.txt",
		Operation:         "write",
		DiffCompressed:    []byte("other content"),
		DiffSize:          13,
		OperatorSessionID: "different_session",
	}
	err = ls.StoreFileDiff(record)
	require.NoError(t, err)

	records, err := ls.GetFileDiffsBySession(operatorSessionID, 10)
	require.NoError(t, err)
	assert.Len(t, records, 5)

	for _, r := range records {
		assert.Equal(t, operatorSessionID, r.OperatorSessionID)
	}
}

func TestLocalStoreService_GetFileDiff_NotFound(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := DefaultLocalStoreConfig()
	config.DBPath = filepath.Join(t.TempDir(), "test_file_diff_notfound.db")

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ls)
	defer ls.Close()

	retrieved, err := ls.GetFileDiff("nonexistent-diff-id")
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestLocalStorePrune(t *testing.T) {
	logger := testutil.NewTestLogger()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "prune_test.db")

	config := &LocalStoreConfig{
		Enabled:              true,
		DBPath:               dbPath,
		RetentionDays:        7,
		MaxDBSizeMB:          1, // Small limit for testing
		PruneIntervalMinutes: 60,
	}

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	defer ls.Close()

	// 1. Insert old and recent execution records
	oldTime := time.Now().AddDate(0, 0, -10).UTC()
	recentTime := time.Now().AddDate(0, 0, -2).UTC()

	oldExec := &ExecutionRecord{
		ID:           "old-exec",
		TimestampUTC: oldTime,
		Command:      "old command",
	}
	recentExec := &ExecutionRecord{
		ID:           "recent-exec",
		TimestampUTC: recentTime,
		Command:      "recent command",
	}
	require.NoError(t, ls.StoreExecution(oldExec))
	require.NoError(t, ls.StoreExecution(recentExec))

	// 2. Insert old and recent file diffs
	oldDiff := &FileDiffRecord{
		ID:           "old-diff",
		TimestampUTC: oldTime,
		FilePath:     "/tmp/old",
	}
	recentDiff := &FileDiffRecord{
		ID:           "recent-diff",
		TimestampUTC: recentTime,
		FilePath:     "/tmp/recent",
	}
	require.NoError(t, ls.StoreFileDiff(oldDiff))
	require.NoError(t, ls.StoreFileDiff(recentDiff))

	// 3. Run pruning
	pruneFunc := localStorePrune(config)
	pruneFunc(ls.db, logger)

	// 4. Verify retention pruning
	retrievedOldExec, err := ls.GetExecution("old-exec")
	require.NoError(t, err)
	assert.Nil(t, retrievedOldExec)

	retrievedRecentExec, err := ls.GetExecution("recent-exec")
	require.NoError(t, err)
	assert.NotNil(t, retrievedRecentExec)

	retrievedOldDiff, err := ls.GetFileDiff("old-diff")
	require.NoError(t, err)
	assert.Nil(t, retrievedOldDiff)

	retrievedRecentDiff, err := ls.GetFileDiff("recent-diff")
	require.NoError(t, err)
	assert.NotNil(t, retrievedRecentDiff)

	// 5. Test size-based pruning
	// Create a lot of records to exceed 1MB.
	// 30 * 100KB = 3MB, well over the 1MB limit.
	// We use random data to prevent high compression ratios if any.
	largeData := make([]byte, 1024*100)
	_, _ = rand.Read(largeData)
	for i := 0; i < 30; i++ {
		err = ls.StoreExecution(&ExecutionRecord{
			ID:               fmt.Sprintf("large-exec-%d", i),
			TimestampUTC:     time.Now().UTC().Add(time.Duration(i) * time.Second),
			StdoutCompressed: largeData,
		})
		require.NoError(t, err)
	}

	// Trigger pruning again
	// Force a checkpoint to move data from WAL to main DB file so GetSizeBytes sees it
	_, err = ls.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	require.NoError(t, err)

	pruneFunc(ls.db, logger)

	// Verify some records were deleted (size-based pruning deletes 10%)
	// We started with 1 recent + 30 large = 31 records.
	// 10% of 31 is 3. So 28 should remain if exactly 10% are deleted.
	var count int
	err = ls.db.QueryRow("SELECT COUNT(*) FROM execution_log").Scan(&count)
	require.NoError(t, err)
	assert.Less(t, count, 31, "Size-based pruning should have deleted at least one record")
}

func TestLocalStoreService_FileDiffUpsert(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := DefaultLocalStoreConfig()
	config.DBPath = filepath.Join(t.TempDir(), "test_file_diff_upsert.db")

	ls, err := NewLocalStoreService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ls)
	defer ls.Close()

	record := &FileDiffRecord{
		ID:                "diff-upsert-123",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/etc/config.yml",
		Operation:         "write",
		DiffCompressed:    []byte("initial diff content"),
		DiffSize:          20,
		OperatorSessionID: "session-abc",
	}
	err = ls.StoreFileDiff(record)
	require.NoError(t, err)

	record.DiffCompressed = []byte("updated diff content")
	record.DiffSize = 20
	err = ls.StoreFileDiff(record)
	require.NoError(t, err)

	retrieved, err := ls.GetFileDiff("diff-upsert-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "updated diff content", string(retrieved.DiffCompressed))
}
