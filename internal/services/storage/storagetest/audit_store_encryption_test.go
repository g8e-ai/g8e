// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package storagetest

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/internal/timesvc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Encryption Integration Tests
// ============================================================================

func TestSQLAuditStore_WithEncryption(t *testing.T) {
	tempDir := testutil.TempDir(t)

	// Create and initialize vault for encryption
	vaultDataDir := filepath.Join(tempDir, "vault")
	encVault := CreateTestVault(t, vaultDataDir, []byte("test-api-key-for-encryption"))
	defer encVault.Close()
	require.True(t, encVault.IsUnlocked())

	fileSvc := NewTestFileSvc(t, tempDir)

	// Create audit vault with encryption enabled
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           encVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	// Create session
	err = avs.CreateSession("encrypted-session-1", "operator", "Encrypted Test", "test-user")
	require.NoError(t, err)

	// Record event with sensitive content
	sensitiveContent := "This is highly confidential command output with secrets"
	exitCode := 0
	event := &storage.Event{
		OperatorSessionID: "encrypted-session-1",
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       "User message about secrets",
		CommandRaw:        "echo secret",
		CommandExitCode:   exitCode,
		CommandStdout:     sensitiveContent,
		CommandStderr:     "Some error output",
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	require.Positive(t, eventID)

	// Retrieve and verify decryption works
	events, err := avs.GetEvents("encrypted-session-1", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	retrievedEvent := events[0]
	assert.Equal(t, "User message about secrets", retrievedEvent.ContentText)
	assert.Equal(t, sensitiveContent, retrievedEvent.CommandStdout)
	assert.Equal(t, "Some error output", retrievedEvent.CommandStderr)
}

func TestSQLAuditStore_EncryptedDataUnreadableWithoutKey(t *testing.T) {
	tempDir := testutil.TempDir(t)

	apiKey := []byte("test-api-key-for-locking-test")

	// Create and initialize vault
	vaultDataDir := filepath.Join(tempDir, "vault")
	vault1 := CreateTestVault(t, vaultDataDir, apiKey)
	fileSvc := NewTestFileSvc(t, tempDir)

	// Create audit vault with encryption
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           vault1,
	}

	avs1, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)

	// Write encrypted data
	err = avs1.CreateSession("locked-test-session", "operator", "Locked Test", "test-user")
	require.NoError(t, err)

	secretData := "TOP SECRET: Password is hunter2"
	exitCode := 0
	_, err = avs1.RecordEvent(&storage.Event{
		OperatorSessionID: "locked-test-session",
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		CommandRaw:        "cat /etc/passwd",
		CommandExitCode:   exitCode,
		CommandStdout:     secretData,
	})
	require.NoError(t, err)

	avs1.Close()
	vault1.Close()

	// Attempt to reopen database WITHOUT encryption vault should fail
	// This is the new fail-closed behavior: service cannot be opened without vault
	config2 := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           nil, // No vault = service fails to initialize
	}

	avs2, err := NewTestSQLAuditStore(config2, testutil.NewTestLogger(), fileSvc)
	require.Error(t, err)
	require.Nil(t, avs2)
	assert.Contains(t, err.Error(), "EncryptionVault is required")

	// Verify data can still be read with the correct vault
	vault3, err := vault.NewVault(&vault.VaultConfig{
		DataDir: vaultDataDir,
		Logger:  testutil.NewTestLogger(),
	})
	require.NoError(t, err)
	err = vault3.Unlock(apiKey)
	require.NoError(t, err)
	defer vault3.Close()

	config3 := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           vault3,
	}

	avs3, err := NewTestSQLAuditStore(config3, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs3.Close()

	// Read events with correct vault - should decrypt successfully
	events, err := avs3.GetEvents("locked-test-session", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	// The stdout should equal the original secret (decrypted correctly)
	assert.Equal(t, secretData, events[0].CommandStdout)
}

func TestSQLAuditStore_EncryptionWithRekey(t *testing.T) {
	tempDir := testutil.TempDir(t)

	oldAPIKey := []byte("old-api-key-before-refresh")
	newAPIKey := []byte("new-api-key-after-refresh")

	// Initialize with old key
	vaultDataDir := filepath.Join(tempDir, "vault")
	vaultSvc := CreateTestVault(t, vaultDataDir, oldAPIKey)
	fileSvc := NewTestFileSvc(t, tempDir)

	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           vaultSvc,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)

	// Write data with old key
	err = avs.CreateSession("rekey-session", "operator", "Rekey Test", "test-user")
	require.NoError(t, err)

	originalData := "Data encrypted with old key"
	exitCode := 0
	_, err = avs.RecordEvent(&storage.Event{
		OperatorSessionID: "rekey-session",
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		CommandRaw:        "echo test",
		CommandExitCode:   exitCode,
		CommandStdout:     originalData,
	})
	require.NoError(t, err)

	avs.Close()
	vaultSvc.Close()

	// Rekey: open a locked vault instance, rekey it, then unlock with the new key
	vault2, err := vault.NewVault(&vault.VaultConfig{
		DataDir: vaultDataDir,
		Logger:  testutil.NewTestLogger(),
	})
	require.NoError(t, err)
	err = vault2.Rekey(oldAPIKey, newAPIKey)
	require.NoError(t, err)
	err = vault2.Unlock(newAPIKey)
	require.NoError(t, err)

	// Reopen audit vault with rekeyed vault
	config2 := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           vault2,
	}

	avs2, err := NewTestSQLAuditStore(config2, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs2.Close()
	defer vault2.Close()

	// Verify we can still read the data with the new key
	events, err := avs2.GetEvents("rekey-session", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, originalData, events[0].CommandStdout)
}

func TestSQLAuditStore_MixedEncryptedUnencrypted(t *testing.T) {
	tempDir := testutil.TempDir(t)

	// With the new fail-closed behavior, vault is mandatory
	// This test verifies that encryption is consistently applied
	vaultDataDir := filepath.Join(tempDir, "vault")
	encVault := CreateTestVault(t, vaultDataDir, []byte("mixed-test-api-key"))
	defer encVault.Close()
	fileSvc := NewTestFileSvc(t, tempDir)

	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           encVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	err = avs.CreateSession("mixed-session", "operator", "Mixed Test", "test-user")
	require.NoError(t, err)

	// Write multiple events - all should be encrypted
	exitCode := 0
	data1 := "First encrypted data"
	_, err = avs.RecordEvent(&storage.Event{
		OperatorSessionID: "mixed-session",
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		CommandRaw:        "echo first",
		CommandExitCode:   exitCode,
		CommandStdout:     data1,
	})
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)

	data2 := "Second encrypted data"
	_, err = avs.RecordEvent(&storage.Event{
		OperatorSessionID: "mixed-session",
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		CommandRaw:        "echo second",
		CommandExitCode:   exitCode,
		CommandStdout:     data2,
	})
	require.NoError(t, err)

	// Read all events - all should be decrypted correctly
	events, err := avs.GetEvents("mixed-session", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 2)

	// Events are returned in descending timestamp order (newest first)
	assert.Equal(t, data2, events[0].CommandStdout)
	assert.Equal(t, data1, events[1].CommandStdout)
}

func TestAuditVaultPrune(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")
	logger := testutil.NewTestLogger()

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)

	config := &TestSQLAuditStoreConfig{
		DBPath:          "prune_test.db",
		RetentionDays:   7,
		EncryptionVault: testVault,
	}

	avs, err := NewTestSQLAuditStore(config, logger, fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	// 1. Insert sessions first to satisfy FK constraints
	_, err = avs.db.Exec("INSERT INTO sessions (id, title) VALUES (?, ?)", "old-session", "op-1")
	require.NoError(t, err)
	_, err = avs.db.Exec("INSERT INTO sessions (id, title) VALUES (?, ?)", "recent-session", "op-1")
	require.NoError(t, err)

	// 2. Insert events
	oldTime := time.Now().AddDate(0, 0, -10)
	oldTimestamp := timesvc.FormatTimestamp(oldTime)
	_, err = avs.db.Exec("INSERT INTO events (id, timestamp, type, operator_session_id) VALUES (?, ?, ?, ?)",
		1, oldTimestamp, "test.event", "old-session")
	require.NoError(t, err)

	// Insert a recent event
	recentTime := time.Now().AddDate(0, 0, -2)
	recentTimestamp := timesvc.FormatTimestamp(recentTime)
	_, err = avs.db.Exec("INSERT INTO events (id, timestamp, type, operator_session_id) VALUES (?, ?, ?, ?)",
		2, recentTimestamp, "test.event", "recent-session")
	require.NoError(t, err)

	// 3. Insert file mutations
	tmpDir := testutil.TempDir(t)
	_, err = avs.db.Exec("INSERT INTO file_mutation_log (event_id, filepath, operation) VALUES (?, ?, ?)",
		1, filepath.Join(tmpDir, "old"), "create")
	require.NoError(t, err)
	_, err = avs.db.Exec("INSERT INTO file_mutation_log (event_id, filepath, operation) VALUES (?, ?, ?)",
		2, filepath.Join(tmpDir, "recent"), "create")
	require.NoError(t, err)

	// 4. Run pruning
	pruneFunc := auditVaultPrune(config)
	err = pruneFunc(context.Background(), avs.db, logger)
	require.NoError(t, err)

	// 3. Verify results
	var count int
	// Old event should be gone
	err = avs.db.QueryRow("SELECT COUNT(*) FROM events WHERE id = 1").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Recent event should remain
	err = avs.db.QueryRow("SELECT COUNT(*) FROM events WHERE id = 2").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Old session (now orphaned) should be gone
	err = avs.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = 'old-session'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Recent session should remain
	err = avs.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE id = 'recent-session'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Old file mutation should be gone
	err = avs.db.QueryRow("SELECT COUNT(*) FROM file_mutation_log WHERE event_id = 1").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Recent file mutation should remain
	err = avs.db.QueryRow("SELECT COUNT(*) FROM file_mutation_log WHERE event_id = 2").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
