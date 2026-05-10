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

func TestNewRawVaultService(t *testing.T) {
	logger := testutil.NewTestLogger()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "raw_vault.db")

	config := &RawVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}

	rv, err := NewRawVaultService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, rv)
	defer rv.Close()

	_, err = os.Stat(dbPath)
	assert.NoError(t, err)

	assert.True(t, rv.IsEnabled())
}

func TestRawVaultService_Disabled(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := &RawVaultConfig{
		Enabled: false,
	}

	rv, err := NewRawVaultService(config, logger)
	assert.NoError(t, err)
	assert.Nil(t, rv)
}

func TestRawVaultService_StoreAndRetrieve(t *testing.T) {
	logger := testutil.NewTestLogger()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "raw_vault_store.db")

	config := &RawVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}

	rv, err := NewRawVaultService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, rv)
	defer rv.Close()

	exitCode := 0
	record := &RawExecutionRecord{
		ID:               "test-raw-exec-123",
		TimestampUTC:     time.Now().UTC(),
		Command:          "cat /etc/passwd",
		ExitCode:         &exitCode,
		DurationMs:       150,
		StdoutCompressed: []byte("root:x:0:0:root:/root:/bin/bash\nuser:x:1000:1000:User:/home/user:/bin/bash"),
		StderrCompressed: []byte(""),
		StdoutHash:       rv.HashString("root:x:0:0:root:/root:/bin/bash\nuser:x:1000:1000:User:/home/user:/bin/bash"),
		StderrHash:       "",
		StdoutSize:       74,
		StderrSize:       0,
		UserID:           "user-456",
		CaseID:           "case-789",
		TaskID:           "task-abc",
		InvestigationID:  "inv-def",
		OperatorID:       "op-ghi",
	}

	err = rv.StoreRawExecution(record)
	require.NoError(t, err)

	retrieved, err := rv.GetRawExecution("test-raw-exec-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, record.ID, retrieved.ID)
	assert.Equal(t, record.Command, retrieved.Command)
	assert.Equal(t, *record.ExitCode, *retrieved.ExitCode)
	assert.Equal(t, record.StdoutSize, retrieved.StdoutSize)
	assert.Equal(t, record.StderrSize, retrieved.StderrSize)
	assert.Equal(t, record.UserID, retrieved.UserID)
	assert.Equal(t, record.CaseID, retrieved.CaseID)
	assert.Equal(t, record.OperatorID, retrieved.OperatorID)
}

func TestRawVaultService_NotFound(t *testing.T) {
	logger := testutil.NewTestLogger()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "raw_vault_notfound.db")

	config := &RawVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}

	rv, err := NewRawVaultService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, rv)
	defer rv.Close()

	retrieved, err := rv.GetRawExecution("nonexistent-id")
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestRawVaultService_HashString(t *testing.T) {
	logger := testutil.NewTestLogger()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "raw_vault_hash.db")

	config := &RawVaultConfig{
		DBPath:               dbPath,
		Enabled:              true,
		PruneIntervalMinutes: 60,
	}

	rv, err := NewRawVaultService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, rv)
	defer rv.Close()

	hash1 := rv.HashString("test data")
	hash2 := rv.HashString("test data")
	hash3 := rv.HashString("different data")

	assert.Equal(t, hash1, hash2, "same input should produce same hash")
	assert.NotEqual(t, hash1, hash3, "different input should produce different hash")
	assert.Len(t, hash1, 64, "SHA256 hex string should be 64 characters")
}

func TestDualVaultIsolation(t *testing.T) {
	logger := testutil.NewTestLogger()

	tmpDir := t.TempDir()
	rawDbPath := filepath.Join(tmpDir, "raw_vault.db")
	scrubbedDbPath := filepath.Join(tmpDir, "scrubbed_vault.db")

	rawConfig := &RawVaultConfig{
		DBPath:               rawDbPath,
		Enabled:              true,
		PruneIntervalMinutes: 60,
	}

	scrubbedConfig := &LocalStoreConfig{
		DBPath:  scrubbedDbPath,
		Enabled: true,
	}

	rawVault, err := NewRawVaultService(rawConfig, logger)
	require.NoError(t, err)
	require.NotNil(t, rawVault)
	defer rawVault.Close()

	scrubbedVault, err := NewLocalStoreService(scrubbedConfig, logger)
	require.NoError(t, err)
	require.NotNil(t, scrubbedVault)
	defer scrubbedVault.Close()

	execID := "dual-vault-test-123"
	exitCode := 0

	rawOutput := "root:x:0:0:root:/root:/bin/bash\nuser:x:1000:1000:User:/home/user:/bin/bash"
	scrubbedOutput := "[SCRUBBED] user list output"

	rawRecord := &RawExecutionRecord{
		ID:               execID,
		TimestampUTC:     time.Now().UTC(),
		Command:          "cat /etc/passwd",
		ExitCode:         &exitCode,
		StdoutCompressed: []byte(rawOutput),
		StdoutSize:       len(rawOutput),
	}
	err = rawVault.StoreRawExecution(rawRecord)
	require.NoError(t, err)

	scrubbedRecord := &ExecutionRecord{
		ID:               execID,
		TimestampUTC:     time.Now().UTC(),
		Command:          "cat /etc/passwd",
		ExitCode:         &exitCode,
		StdoutCompressed: []byte(scrubbedOutput),
		StdoutSize:       len(rawOutput),
	}
	err = scrubbedVault.StoreExecution(scrubbedRecord)
	require.NoError(t, err)

	retrievedRaw, err := rawVault.GetRawExecution(execID)
	require.NoError(t, err)
	require.NotNil(t, retrievedRaw)
	assert.Equal(t, rawOutput, string(retrievedRaw.StdoutCompressed))

	retrievedScrubbed, err := scrubbedVault.GetExecution(execID)
	require.NoError(t, err)
	require.NotNil(t, retrievedScrubbed)
	assert.Equal(t, scrubbedOutput, string(retrievedScrubbed.StdoutCompressed))

	assert.NotEqual(t, string(retrievedRaw.StdoutCompressed), string(retrievedScrubbed.StdoutCompressed),
		"raw vault and scrubbed vault should contain different data for the same execution ID")

	assert.Contains(t, string(retrievedRaw.StdoutCompressed), "root:x:0:0",
		"raw vault should contain unscrubbed data")
	assert.Contains(t, string(retrievedScrubbed.StdoutCompressed), "[SCRUBBED]",
		"scrubbed vault should contain redacted data")
}

// LFAA Dual-Vault File Diff Tests (Raw Vault)

func TestRawVaultService_StoreAndRetrieveFileDiff(t *testing.T) {
	logger := testutil.NewTestLogger()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "raw_vault_file_diff.db")

	config := &RawVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}

	rv, err := NewRawVaultService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, rv)
	defer rv.Close()

	rawDiffContent := "--- a/etc/nginx/nginx.conf\n+++ b/etc/nginx/nginx.conf\n@@ -10,5 +10,8 @@\n-password=secret123\n+password=newpassword456"

	record := &RawFileDiffRecord{
		ID:                "raw-diff-test-123",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/etc/nginx/nginx.conf",
		Operation:         "replace",
		LedgerHashBefore:  "abc123",
		LedgerHashAfter:   "def456",
		DiffStat:          "+1/-1",
		DiffCompressed:    []byte(rawDiffContent),
		DiffHash:          rv.HashString(rawDiffContent),
		DiffSize:          len(rawDiffContent),
		OperatorSessionID: "web_session_abc",
		UserID:            "user-123",
		CaseID:            "case-456",
		OperatorID:        "op-789",
	}

	err = rv.StoreRawFileDiff(record)
	require.NoError(t, err)

	retrieved, err := rv.GetRawFileDiff("raw-diff-test-123")
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
	assert.Equal(t, rawDiffContent, string(retrieved.DiffCompressed))
}

func TestRawVaultService_GetRawFileDiffsBySession(t *testing.T) {
	logger := testutil.NewTestLogger()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "raw_vault_file_diff_session.db")

	config := &RawVaultConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}

	rv, err := NewRawVaultService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, rv)
	defer rv.Close()

	operatorSessionID := "raw_session_test_123"

	for i := 0; i < 5; i++ {
		record := &RawFileDiffRecord{
			ID:                "raw-diff-session-" + string(rune('A'+i)),
			TimestampUTC:      time.Now().UTC().Add(time.Duration(-i) * time.Minute),
			FilePath:          "/path/to/file" + string(rune('A'+i)) + ".txt",
			Operation:         "write",
			DiffStat:          "+5/-0",
			DiffCompressed:    []byte("raw diff content with secrets " + string(rune('A'+i))),
			DiffSize:          30,
			OperatorSessionID: operatorSessionID,
		}
		err = rv.StoreRawFileDiff(record)
		require.NoError(t, err)
	}

	record := &RawFileDiffRecord{
		ID:                "raw-diff-other-session",
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/other/file.txt",
		Operation:         "write",
		DiffCompressed:    []byte("other content"),
		DiffSize:          13,
		OperatorSessionID: "different_session",
	}
	err = rv.StoreRawFileDiff(record)
	require.NoError(t, err)

	records, err := rv.GetRawFileDiffsBySession(operatorSessionID, 10)
	require.NoError(t, err)
	assert.Len(t, records, 5)

	for _, r := range records {
		assert.Equal(t, operatorSessionID, r.OperatorSessionID)
	}
}

func TestRawVaultService_GetRawFileDiff_NotFound(t *testing.T) {
	logger := testutil.NewTestLogger()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "raw_vault_file_diff_notfound.db")

	config := &RawVaultConfig{
		DBPath:               dbPath,
		Enabled:              true,
		PruneIntervalMinutes: 60,
	}

	rv, err := NewRawVaultService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, rv)
	defer rv.Close()

	retrieved, err := rv.GetRawFileDiff("nonexistent-diff-id")
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestDualVaultFileDiffIsolation(t *testing.T) {
	logger := testutil.NewTestLogger()

	tmpDir := t.TempDir()
	rawDbPath := filepath.Join(tmpDir, "raw_vault_diff.db")
	scrubbedDbPath := filepath.Join(tmpDir, "scrubbed_vault_diff.db")

	rawConfig := &RawVaultConfig{
		DBPath:               rawDbPath,
		Enabled:              true,
		PruneIntervalMinutes: 60,
	}

	scrubbedConfig := &LocalStoreConfig{
		DBPath:  scrubbedDbPath,
		Enabled: true,
	}

	rawVault, err := NewRawVaultService(rawConfig, logger)
	require.NoError(t, err)
	require.NotNil(t, rawVault)
	defer rawVault.Close()

	scrubbedVault, err := NewLocalStoreService(scrubbedConfig, logger)
	require.NoError(t, err)
	require.NotNil(t, scrubbedVault)
	defer scrubbedVault.Close()

	diffID := "dual-vault-diff-test-123"
	rawDiff := "--- a/config.yml\n+++ b/config.yml\n@@ -1 +1 @@\n-api_key: sk-secret-12345\n+api_key: sk-secret-67890"
	scrubbedDiff := "--- a/config.yml\n+++ b/config.yml\n@@ -1 +1 @@\n-api_key: [REDACTED]\n+api_key: [REDACTED]"

	rawRecord := &RawFileDiffRecord{
		ID:                diffID,
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/app/config.yml",
		Operation:         "replace",
		DiffCompressed:    []byte(rawDiff),
		DiffSize:          len(rawDiff),
		OperatorSessionID: "session-abc",
	}
	err = rawVault.StoreRawFileDiff(rawRecord)
	require.NoError(t, err)

	scrubbedRecord := &FileDiffRecord{
		ID:                diffID,
		TimestampUTC:      time.Now().UTC(),
		FilePath:          "/app/config.yml",
		Operation:         "replace",
		DiffCompressed:    []byte(scrubbedDiff),
		DiffSize:          len(scrubbedDiff),
		OperatorSessionID: "session-abc",
	}
	err = scrubbedVault.StoreFileDiff(scrubbedRecord)
	require.NoError(t, err)

	retrievedRaw, err := rawVault.GetRawFileDiff(diffID)
	require.NoError(t, err)
	require.NotNil(t, retrievedRaw)
	assert.Equal(t, rawDiff, string(retrievedRaw.DiffCompressed))

	retrievedScrubbed, err := scrubbedVault.GetFileDiff(diffID)
	require.NoError(t, err)
	require.NotNil(t, retrievedScrubbed)
	assert.Equal(t, scrubbedDiff, string(retrievedScrubbed.DiffCompressed))

	assert.NotEqual(t, string(retrievedRaw.DiffCompressed), string(retrievedScrubbed.DiffCompressed),
		"raw vault and scrubbed vault should contain different diff data for the same diff ID")

	assert.Contains(t, string(retrievedRaw.DiffCompressed), "sk-secret-12345",
		"raw vault should contain unscrubbed secrets")
	assert.Contains(t, string(retrievedScrubbed.DiffCompressed), "[REDACTED]",
		"scrubbed vault should contain redacted secrets")
	assert.NotContains(t, string(retrievedScrubbed.DiffCompressed), "sk-secret",
		"scrubbed vault should NOT contain any secrets")
}

func TestRawVaultPrune(t *testing.T) {
	logger := testutil.NewTestLogger()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "raw_prune_test.db")

	config := &RawVaultConfig{
		Enabled:              true,
		DBPath:               dbPath,
		RetentionDays:        7,
		MaxDBSizeMB:          1, // Small limit for testing
		PruneIntervalMinutes: 60,
	}

	rv, err := NewRawVaultService(config, logger)
	require.NoError(t, err)
	defer rv.Close()

	// 1. Insert old and recent raw execution records
	oldTime := time.Now().AddDate(0, 0, -10).UTC()
	recentTime := time.Now().AddDate(0, 0, -2).UTC()

	oldExec := &RawExecutionRecord{
		ID:           "old-raw-exec",
		TimestampUTC: oldTime,
		Command:      "old raw command",
	}
	recentExec := &RawExecutionRecord{
		ID:           "recent-raw-exec",
		TimestampUTC: recentTime,
		Command:      "recent raw command",
	}
	require.NoError(t, rv.StoreRawExecution(oldExec))
	require.NoError(t, rv.StoreRawExecution(recentExec))

	// 2. Insert old and recent raw file diffs
	oldDiff := &RawFileDiffRecord{
		ID:           "old-raw-diff",
		TimestampUTC: oldTime,
		FilePath:     "/tmp/old_raw",
	}
	recentDiff := &RawFileDiffRecord{
		ID:           "recent-raw-diff",
		TimestampUTC: recentTime,
		FilePath:     "/tmp/recent_raw",
	}
	require.NoError(t, rv.StoreRawFileDiff(oldDiff))
	require.NoError(t, rv.StoreRawFileDiff(recentDiff))

	// 3. Run pruning
	pruneFunc := rawVaultPrune(config)
	pruneFunc(rv.db, logger)

	// 4. Verify retention pruning
	retrievedOldExec, err := rv.GetRawExecution("old-raw-exec")
	require.NoError(t, err)
	assert.Nil(t, retrievedOldExec)

	retrievedRecentExec, err := rv.GetRawExecution("recent-raw-exec")
	require.NoError(t, err)
	assert.NotNil(t, retrievedRecentExec)

	retrievedOldDiff, err := rv.GetRawFileDiff("old-raw-diff")
	require.NoError(t, err)
	assert.Nil(t, retrievedOldDiff)

	retrievedRecentDiff, err := rv.GetRawFileDiff("recent-raw-diff")
	require.NoError(t, err)
	assert.NotNil(t, retrievedRecentDiff)

	// 5. Test size-based pruning
	// Create a lot of records to exceed 1MB.
	// 30 * 100KB = 3MB, well over the 1MB limit.
	// We use random data to prevent high compression ratios if any.
	largeData := make([]byte, 1024*100)
	_, _ = rand.Read(largeData)
	for i := 0; i < 30; i++ {
		err = rv.StoreRawExecution(&RawExecutionRecord{
			ID:               fmt.Sprintf("large-raw-exec-%d", i),
			TimestampUTC:     time.Now().UTC().Add(time.Duration(i) * time.Second),
			StdoutCompressed: largeData,
		})
		require.NoError(t, err)
	}

	// Trigger pruning again
	// Force a checkpoint to move data from WAL to main DB file so GetSizeBytes sees it
	_, err = rv.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	require.NoError(t, err)

	pruneFunc(rv.db, logger)

	// Verify some records were deleted (size-based pruning deletes 10%)
	// We started with 1 recent + 30 large = 31 records.
	// 10% of 31 is 3. So 28 should remain if exactly 10% are deleted.
	var count int
	err = rv.db.QueryRow("SELECT COUNT(*) FROM raw_execution_log").Scan(&count)
	require.NoError(t, err)
	assert.Less(t, count, 31, "Size-based pruning should have deleted at least one record")
}

func TestDefaultRawVaultConfig_ReturnsExpectedDefaults(t *testing.T) {
	cfg := DefaultRawVaultConfig()

	require.NotNil(t, cfg)
	assert.Equal(t, "./.g8e/raw_vault.db", cfg.DBPath)
	assert.Equal(t, int64(2048), cfg.MaxDBSizeMB)
	assert.Equal(t, 30, cfg.RetentionDays)
	assert.Equal(t, 60, cfg.PruneIntervalMinutes)
	assert.True(t, cfg.Enabled)
}
