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
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLAuditStore_Session(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
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

	// Create a session
	operatorSessionID := "test-session-123"
	err = avs.CreateSession(operatorSessionID, "operator", "Test OperatorSession", "user@example.com")
	require.NoError(t, err)

	// Retrieve the session
	session, err := avs.GetOperatorSession(operatorSessionID)
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Equal(t, operatorSessionID, session.ID)
	assert.Equal(t, "Test OperatorSession", session.Title)
	assert.Equal(t, "user@example.com", session.UserIdentity)
}

func TestSQLAuditStore_MultipleSessions(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
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

	// Create multiple sessions
	sessions := []struct {
		id    string
		title string
		user  string
	}{
		{"session-1", "First OperatorSession", "user1@test.com"},
		{"session-2", "Second OperatorSession", "user2@test.com"},
		{"session-3", "Third OperatorSession", "user1@test.com"},
	}

	for _, s := range sessions {
		err := avs.CreateSession(s.id, "operator", s.title, s.user)
		require.NoError(t, err)
	}

	// Verify each session exists with correct data
	for _, s := range sessions {
		session, err := avs.GetOperatorSession(s.id)
		require.NoError(t, err)
		require.NotNil(t, session)
		assert.Equal(t, s.id, session.ID)
		assert.Equal(t, s.title, session.Title)
		assert.Equal(t, s.user, session.UserIdentity)
	}
}

func TestSQLAuditStore_GetSessionNotFound(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
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

	session, err := avs.GetOperatorSession("non-existent-session")
	require.NoError(t, err)
	assert.Nil(t, session)
}

func TestSQLAuditStore_SessionWithNullFields(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
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

	// Create session with empty optional fields
	err = avs.CreateSession("null-fields-session", "operator", "", "")
	require.NoError(t, err)

	session, err := avs.GetOperatorSession("null-fields-session")
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Equal(t, "null-fields-session", session.ID)
	assert.Empty(t, session.Title)
	assert.Empty(t, session.UserIdentity)
}
