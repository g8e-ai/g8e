// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package storagetest

import (
	"crypto/ed25519"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLAuditStore_ListEvents_NilStore(t *testing.T) {
	var avs *TestSQLAuditStore
	events, err := avs.ListEvents("", 10, 0)
	require.ErrorIs(t, err, constants.ErrAuditStoreDisabled)
	assert.Nil(t, events)
}

func TestSQLAuditStore_ListEvents_WithSessionFilter(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	sessionA := "list-events-session-a"
	sessionB := "list-events-session-b"
	require.NoError(t, avs.CreateSession(sessionA, "operator", "Session A", "user@test.com"))
	require.NoError(t, avs.CreateSession(sessionB, "operator", "Session B", "user@test.com"))

	baseTime := time.Now().Add(-time.Hour).UTC()
	for i := 0; i < 3; i++ {
		_, err := avs.RecordEvent(&storage.Event{
			OperatorSessionID: sessionA,
			Timestamp:         baseTime.Add(time.Duration(i) * time.Minute),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       fmt.Sprintf("A-event-%d", i),
		})
		require.NoError(t, err)
	}
	for i := 0; i < 2; i++ {
		_, err := avs.RecordEvent(&storage.Event{
			OperatorSessionID: sessionB,
			Timestamp:         baseTime.Add(time.Duration(i) * time.Minute),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       fmt.Sprintf("B-event-%d", i),
		})
		require.NoError(t, err)
	}

	events, err := avs.ListEvents(sessionA, 100, 0)
	require.NoError(t, err)
	require.Len(t, events, 3)
	for _, e := range events {
		assert.Equal(t, sessionA, e.OperatorSessionID)
	}
	assert.Equal(t, "A-event-0", events[0].ContentText)
	assert.Equal(t, "A-event-2", events[2].ContentText)

	eventsB, err := avs.ListEvents(sessionB, 100, 0)
	require.NoError(t, err)
	require.Len(t, eventsB, 2)
	for _, e := range eventsB {
		assert.Equal(t, sessionB, e.OperatorSessionID)
	}
}

func TestSQLAuditStore_ListEvents_NoSessionFilter(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	sessionA := "list-all-session-a"
	sessionB := "list-all-session-b"
	require.NoError(t, avs.CreateSession(sessionA, "operator", "Session A", "user@test.com"))
	require.NoError(t, avs.CreateSession(sessionB, "operator", "Session B", "user@test.com"))

	baseTime := time.Now().Add(-time.Hour).UTC()
	_, err = avs.RecordEvent(&storage.Event{
		OperatorSessionID: sessionA,
		Timestamp:         baseTime,
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       "first",
	})
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	_, err = avs.RecordEvent(&storage.Event{
		OperatorSessionID: sessionB,
		Timestamp:         baseTime.Add(time.Second),
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       "second",
	})
	require.NoError(t, err)

	events, err := avs.ListEvents("", 100, 0)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "first", events[0].ContentText)
	assert.Equal(t, "second", events[1].ContentText)
}

func TestSQLAuditStore_ListEvents_EncryptedContent(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	encVault := CreateTestVault(t, vaultDir, []byte("list-events-encryption-key"))
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

	sessionID := "list-events-encrypted-session"
	require.NoError(t, avs.CreateSession(sessionID, "operator", "Encrypted", "user@test.com"))

	secretContent := "classified output"
	secretStdout := "secret stdout data"
	_, err = avs.RecordEvent(&storage.Event{
		OperatorSessionID: sessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       secretContent,
		CommandStdout:     secretStdout,
		CommandExitCode:   0,
	})
	require.NoError(t, err)

	events, err := avs.ListEvents(sessionID, 100, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, secretContent, events[0].ContentText)
	assert.Equal(t, secretStdout, events[0].CommandStdout)
}

func TestSQLAuditStore_ListEvents_LockedVault(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	encVault := CreateTestVault(t, vaultDir, privKey)
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

	sessionID := "list-events-locked-vault-session"
	require.NoError(t, avs.CreateSession(sessionID, "operator", "Locked Vault", "user@test.com"))

	plaintextContent := "classified secret output"
	plaintextStdout := "secret stdout"
	_, err = avs.RecordEvent(&storage.Event{
		OperatorSessionID: sessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       plaintextContent,
		CommandStdout:     plaintextStdout,
		CommandExitCode:   0,
	})
	require.NoError(t, err)

	// Lock the vault so ListEvents cannot decrypt
	encVault.Lock()
	require.False(t, encVault.IsUnlocked())

	events, err := avs.ListEvents(sessionID, 100, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	// Content should be ciphertext (raw bytes as string), not the plaintext
	assert.NotEqual(t, plaintextContent, events[0].ContentText)
	assert.NotEqual(t, plaintextStdout, events[0].CommandStdout)
	// Ciphertext is non-empty (encrypted data was stored)
	assert.NotEmpty(t, events[0].ContentText)
	assert.NotEmpty(t, events[0].CommandStdout)
}

func TestSQLAuditStore_ListEvents_DefaultLimit(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	sessionID := "list-events-default-limit-session"
	require.NoError(t, avs.CreateSession(sessionID, "operator", "Default Limit", "user@test.com"))

	for i := 0; i < 3; i++ {
		_, err := avs.RecordEvent(&storage.Event{
			OperatorSessionID: sessionID,
			Timestamp:         time.Now().Add(time.Duration(i) * time.Minute).UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       fmt.Sprintf("event-%d", i),
		})
		require.NoError(t, err)
	}

	events, err := avs.ListEvents(sessionID, 0, 0)
	require.NoError(t, err)
	assert.Len(t, events, 3)
}

func TestSQLAuditStore_ListEvents_EmptyResult(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	sessionID := "list-events-empty-session"
	require.NoError(t, avs.CreateSession(sessionID, "operator", "Empty", "user@test.com"))

	events, err := avs.ListEvents(sessionID, 100, 0)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestSQLAuditStore_ListFileMutations_NilStore(t *testing.T) {
	var avs *TestSQLAuditStore
	mutations, err := avs.ListFileMutations(10, 0)
	require.ErrorIs(t, err, constants.ErrAuditStoreDisabled)
	assert.Nil(t, mutations)
}

func TestSQLAuditStore_ListFileMutations_Pagination(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	sessionID := "list-mutations-session"
	require.NoError(t, avs.CreateSession(sessionID, "operator", "Mutations", "user@test.com"))

	eventID, err := avs.RecordEvent(&storage.Event{
		OperatorSessionID: sessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.FileEdit.Completed,
		ContentText:       "batch file ops",
	})
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		err := avs.RecordFileMutation(&storage.FileMutationLog{
			EventID:   eventID,
			Filepath:  fmt.Sprintf("/test/file_%d.txt", i),
			Operation: storage.FileMutationWrite,
		})
		require.NoError(t, err)
	}

	page1, err := avs.ListFileMutations(2, 0)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, "/test/file_0.txt", page1[0].Filepath)
	assert.Equal(t, "/test/file_1.txt", page1[1].Filepath)

	page2, err := avs.ListFileMutations(2, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, "/test/file_2.txt", page2[0].Filepath)
	assert.Equal(t, "/test/file_3.txt", page2[1].Filepath)

	page3, err := avs.ListFileMutations(2, 4)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	assert.Equal(t, "/test/file_4.txt", page3[0].Filepath)

	beyond, err := avs.ListFileMutations(10, 100)
	require.NoError(t, err)
	assert.Empty(t, beyond)
}

func TestSQLAuditStore_ListFileMutations_DefaultLimit(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	sessionID := "list-mutations-default-limit-session"
	require.NoError(t, avs.CreateSession(sessionID, "operator", "Default", "user@test.com"))

	eventID, err := avs.RecordEvent(&storage.Event{
		OperatorSessionID: sessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.FileEdit.Completed,
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err := avs.RecordFileMutation(&storage.FileMutationLog{
			EventID:   eventID,
			Filepath:  fmt.Sprintf("/default/file_%d.txt", i),
			Operation: storage.FileMutationWrite,
		})
		require.NoError(t, err)
	}

	mutations, err := avs.ListFileMutations(0, 0)
	require.NoError(t, err)
	assert.Len(t, mutations, 3)
}

func TestSQLAuditStore_ListFileMutations_EmptyResult(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	mutations, err := avs.ListFileMutations(100, 0)
	require.NoError(t, err)
	assert.Empty(t, mutations)
}
