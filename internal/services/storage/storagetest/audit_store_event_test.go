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

//go:build integration

package storagetest

import (
	"crypto/ed25519"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLAuditStore_Event(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create a session first
	operatorSessionID := "test-session-456"
	err = avs.CreateSession(operatorSessionID, "operator", "Test OperatorSession", "user@example.com")
	require.NoError(t, err)

	// Record a command execution event
	exitCode := 0
	event := &storage.Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                constants.Event.Operator.Audit.Command,
		ContentText:         "List files",
		CommandRaw:          "ls -la",
		CommandExitCode:     &exitCode,
		CommandStdout:       "file1.txt\nfile2.txt",
		CommandStderr:       "",
		ExecutionDurationMs: 150,
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	assert.Positive(t, eventID)

	// Retrieve events for the session
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	retrievedEvent := events[0]
	assert.Equal(t, operatorSessionID, retrievedEvent.OperatorSessionID)
	assert.Equal(t, constants.Event.Operator.Audit.Command, retrievedEvent.Type)
	assert.Equal(t, "ls -la", retrievedEvent.CommandRaw)
	assert.Equal(t, "file1.txt\nfile2.txt", retrievedEvent.CommandStdout)
}

func TestSQLAuditStore_RecordEvent_RejectsUnknownSession(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "session_1771888262981_ffafe0f4-9c9e-439c-8a97-89e5a9f04c1e"
	exitCode := 0
	event := &storage.Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                constants.Event.Operator.Audit.Command,
		ContentText:         "direct terminal command",
		CommandRaw:          "uptime",
		CommandExitCode:     &exitCode,
		CommandStdout:       " 15:27:00 up 1 day,  3:42,  1 user,  load average: 0.10, 0.08, 0.06",
		CommandStderr:       "",
		ExecutionDurationMs: 10,
	}

	eventID, err := avs.RecordEvent(event)
	require.ErrorIs(t, err, storage.ErrAuditSessionUnknown)
	assert.Equal(t, int64(0), eventID)

	session, err := avs.GetOperatorSession(operatorSessionID)
	require.NoError(t, err)
	assert.Nil(t, session)
}

func TestSQLAuditStore_RecordEvent_RejectsMissingSession(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	eventID, err := avs.RecordEvent(&storage.Event{
		Timestamp:   time.Now().UTC(),
		Type:        constants.Event.Operator.Audit.Command,
		ContentText: "missing session",
		CommandRaw:  "uptime",
	})
	require.ErrorIs(t, err, storage.ErrAuditSessionMissing)
	assert.Equal(t, int64(0), eventID)
}

func TestSQLAuditStore_RecordEvents_RollsBackUnknownSession(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "batch-valid-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Batch Session", "user@example.com")
	require.NoError(t, err)

	err = avs.RecordEvents([]*storage.Event{
		{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       "valid event",
			CommandRaw:        "uptime",
		},
		{
			OperatorSessionID: "unknown-batch-session",
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       "invalid event",
			CommandRaw:        "id",
		},
	})
	require.ErrorIs(t, err, storage.ErrAuditSessionUnknown)

	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestSQLAuditStore_RecordEvents_SucceedsWithExistingSessions(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "batch-existing-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Batch Session", "user@example.com")
	require.NoError(t, err)

	err = avs.RecordEvents([]*storage.Event{
		{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.UserMsg,
			ContentText:       "hello",
		},
		{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.AIMsg,
			ContentText:       "world",
		},
	})
	require.NoError(t, err)

	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestSQLAuditStore_OutputTruncation(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 100,
		HeadTailSize:              30,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create a session
	operatorSessionID := "test-session-truncation"
	err = avs.CreateSession(operatorSessionID, "operator", "Truncation Test", "user@example.com")
	require.NoError(t, err)

	// Create large output that exceeds threshold
	largeOutput := make([]byte, 200)
	for i := range largeOutput {
		largeOutput[i] = byte('A' + (i % 26))
	}

	exitCode := 0
	event := &storage.Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                constants.Event.Operator.Audit.Command,
		ContentText:         "Large output test",
		CommandRaw:          "echo large",
		CommandExitCode:     &exitCode,
		CommandStdout:       string(largeOutput),
		CommandStderr:       "",
		ExecutionDurationMs: 50,
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	assert.Positive(t, eventID)

	// Retrieve and verify truncation
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	retrievedEvent := events[0]
	assert.True(t, retrievedEvent.StdoutTruncated)
	assert.Contains(t, retrievedEvent.CommandStdout, "[TRUNCATED:")
}

func TestSQLAuditStore_StderrTruncation(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 100,
		HeadTailSize:              30,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-stderr-truncation"
	err = avs.CreateSession(operatorSessionID, "operator", "Stderr Truncation Test", "user@test.com")
	require.NoError(t, err)

	// Large stderr output
	largeStderr := make([]byte, 200)
	for i := range largeStderr {
		largeStderr[i] = byte('E' + (i % 26))
	}

	exitCode := 1
	event := &storage.Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		CommandRaw:        "failing_command",
		CommandExitCode:   &exitCode,
		CommandStdout:     "small stdout",
		CommandStderr:     string(largeStderr),
	}

	_, err = avs.RecordEvent(event)
	require.NoError(t, err)

	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.True(t, events[0].StderrTruncated)
	assert.False(t, events[0].StdoutTruncated)
	assert.Contains(t, events[0].CommandStderr, "[TRUNCATED:")
}

func TestSQLAuditStore_EventPagination(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-pagination-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Pagination Test", "user@test.com")
	require.NoError(t, err)

	// Create 25 events
	exitCode := 0
	for i := 0; i < 25; i++ {
		event := &storage.Event{
			OperatorSessionID:   operatorSessionID,
			Timestamp:           time.Now().UTC(),
			Type:                constants.Event.Operator.Audit.Command,
			ContentText:         fmt.Sprintf("Event %d", i),
			CommandRaw:          fmt.Sprintf("echo %d", i),
			CommandExitCode:     &exitCode,
			CommandStdout:       fmt.Sprintf("Output %d", i),
			ExecutionDurationMs: int64(i * 10),
		}
		_, err := avs.RecordEvent(event)
		require.NoError(t, err)
	}

	// Test first page
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, events, 10)

	// Test second page
	events, err = avs.GetEvents(operatorSessionID, 10, 10)
	require.NoError(t, err)
	assert.Len(t, events, 10)

	// Test third page (partial)
	events, err = avs.GetEvents(operatorSessionID, 10, 20)
	require.NoError(t, err)
	assert.Len(t, events, 5)

	// Test offset beyond total
	events, err = avs.GetEvents(operatorSessionID, 10, 100)
	require.NoError(t, err)
	assert.Empty(t, events)

	// Test default limit (0 should default to 50)
	events, err = avs.GetEvents(operatorSessionID, 0, 0)
	require.NoError(t, err)
	assert.Len(t, events, 25) // All events since we have less than 50
}

func TestSQLAuditStore_EventOrdering(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-ordering-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Ordering Test", "user@test.com")
	require.NoError(t, err)

	// Create events with increasing timestamps
	baseTime := time.Now().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		event := &storage.Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         baseTime.Add(time.Duration(i) * time.Minute),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       fmt.Sprintf("Event %d", i),
		}
		_, err := avs.RecordEvent(event)
		require.NoError(t, err)
	}

	// Events should be returned in descending order (newest first)
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 5)

	// Verify descending order
	for i := 0; i < len(events)-1; i++ {
		assert.True(t, events[i].Timestamp.After(events[i+1].Timestamp) ||
			events[i].Timestamp.Equal(events[i+1].Timestamp))
	}
}

func TestSQLAuditStore_NullExitCode(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-null-exit-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Null Exit Test", "user@test.com")
	require.NoError(t, err)

	// Create event with nil exit code
	event := &storage.Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.UserMsg,
		ContentText:       "User message without exit code",
		CommandExitCode:   nil, // No exit code for user messages
	}
	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	assert.Positive(t, eventID)

	// Retrieve and verify nil exit code
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Nil(t, events[0].CommandExitCode)
}

func TestSQLAuditStore_DifferentEventTypes(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-event-types-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Event Types Test", "user@test.com")
	require.NoError(t, err)

	eventTypes := []constants.EventType{
		constants.Event.Operator.Audit.UserMsg,
		constants.Event.Operator.Audit.AIMsg,
		constants.Event.Operator.Audit.Command,
		constants.Event.Operator.FileEdit.Completed,
	}

	for _, eventType := range eventTypes {
		event := &storage.Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              eventType,
			ContentText:       fmt.Sprintf("Test %s event", eventType),
		}
		_, err := avs.RecordEvent(event)
		require.NoError(t, err)
	}

	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, events, 4)

	// Verify all event types are present
	types := make(map[constants.EventType]bool)
	for _, e := range events {
		types[e.Type] = true
	}
	for _, et := range eventTypes {
		assert.True(t, types[et], "Missing event type: %s", et)
	}
}

func TestSQLAuditStore_GetEventsEmptySession(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "empty-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Empty OperatorSession", "user@test.com")
	require.NoError(t, err)

	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestSQLAuditStore_LongContentFields(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-long-content"
	err = avs.CreateSession(operatorSessionID, "operator", "Long Content Test", "user@test.com")
	require.NoError(t, err)

	// Create event with long content (below truncation threshold)
	longContent := make([]byte, 50000)
	for i := range longContent {
		longContent[i] = byte('A' + (i % 26))
	}

	exitCode := 0
	event := &storage.Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       string(longContent),
		CommandRaw:        "cat large_file",
		CommandExitCode:   &exitCode,
		CommandStdout:     string(longContent),
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	assert.Positive(t, eventID)

	// Retrieve and verify
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.Equal(t, string(longContent), events[0].ContentText)
	assert.False(t, events[0].StdoutTruncated) // Below threshold
}
