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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/services/sqliteutil"
	"github.com/g8e-ai/g8e/components/vsa/services/vault"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditVaultConfig_Default(t *testing.T) {
	// Ensure no environment variable override is set
	os.Unsetenv("G8E_DATA_DIR")

	config := DefaultAuditVaultConfig()

	// Default DataDir must be CWD-relative - Operator operates where user deploys it
	assert.Equal(t, "./.g8e/data", config.DataDir)
	assert.Equal(t, "g8e.db", config.DBPath)
	assert.Equal(t, "ledger", config.LedgerDir)
	assert.Equal(t, int64(2048), config.MaxDBSizeMB)
	assert.Equal(t, 90, config.RetentionDays)
	assert.Equal(t, 102400, config.OutputTruncationThreshold)
	assert.Equal(t, 51200, config.HeadTailSize)
	assert.True(t, config.Enabled)
}

func TestAuditVaultService_Bootstrap(t *testing.T) {
	gitPath := testGitPath(t)

	// Create temporary directory for test
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

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NotNil(t, avs)
	defer avs.Close()

	// Verify directory structure was created
	assert.DirExists(t, tempDir)
	assert.DirExists(t, filepath.Join(tempDir, "ledger"))
	assert.DirExists(t, filepath.Join(tempDir, "ledger", "files"))

	// Verify database was created
	assert.FileExists(t, filepath.Join(tempDir, "test.db"))

	// Verify git was initialized
	assert.DirExists(t, filepath.Join(tempDir, "ledger", ".git"))

	// Verify service is enabled
	assert.True(t, avs.IsEnabled())
	assert.Equal(t, tempDir, avs.GetDataDir())
}

func TestAuditVaultService_Session(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create a session
	operatorSessionID := "test-session-123"
	err = avs.CreateSession(operatorSessionID, "Test OperatorSession", "user@example.com")
	require.NoError(t, err)

	// Retrieve the session
	session, err := avs.GetSession(operatorSessionID)
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Equal(t, operatorSessionID, session.ID)
	assert.Equal(t, "Test OperatorSession", session.Title)
	assert.Equal(t, "user@example.com", session.UserIdentity)
}

func TestAuditVaultService_Event(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create a session first
	operatorSessionID := "test-session-456"
	err = avs.CreateSession(operatorSessionID, "Test OperatorSession", "user@example.com")
	require.NoError(t, err)

	// Record a command execution event
	exitCode := 0
	event := &Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                EventTypeCmdExec,
		ContentText:         "List files",
		CommandRaw:          "ls -la",
		CommandExitCode:     &exitCode,
		CommandStdout:       "file1.txt\nfile2.txt",
		CommandStderr:       "",
		ExecutionDurationMs: 150,
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	assert.Greater(t, eventID, int64(0))

	// Retrieve events for the session
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	retrievedEvent := events[0]
	assert.Equal(t, operatorSessionID, retrievedEvent.OperatorSessionID)
	assert.Equal(t, EventTypeCmdExec, retrievedEvent.Type)
	assert.Equal(t, "ls -la", retrievedEvent.CommandRaw)
	assert.Equal(t, "file1.txt\nfile2.txt", retrievedEvent.CommandStdout)
}

func TestAuditVaultService_RecordEvent_AutoCreatesSession(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Do NOT call CreateSession — simulates direct terminal / anchored terminal commands
	// where no session is explicitly created before events are recorded.
	operatorSessionID := "session_1771888262981_ffafe0f4-9c9e-439c-8a97-89e5a9f04c1e"
	exitCode := 0
	event := &Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                EventTypeCmdExec,
		ContentText:         "direct terminal command",
		CommandRaw:          "uptime",
		CommandExitCode:     &exitCode,
		CommandStdout:       " 15:27:00 up 1 day,  3:42,  1 user,  load average: 0.10, 0.08, 0.06",
		CommandStderr:       "",
		ExecutionDurationMs: 10,
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err, "RecordEvent must not fail when session was not pre-created")
	assert.Greater(t, eventID, int64(0))

	// OperatorSession should have been auto-created
	session, err := avs.GetSession(operatorSessionID)
	require.NoError(t, err)
	require.NotNil(t, session, "session should be auto-created by RecordEvent")
	assert.Equal(t, operatorSessionID, session.ID)

	// Event should be retrievable
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "uptime", events[0].CommandRaw)
}

func TestAuditVaultService_OutputTruncation(t *testing.T) {
	tempDir := t.TempDir()

	config := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 100,
		HeadTailSize:              30,
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create a session
	operatorSessionID := "test-session-truncation"
	err = avs.CreateSession(operatorSessionID, "Truncation Test", "user@example.com")
	require.NoError(t, err)

	// Create large output that exceeds threshold
	largeOutput := make([]byte, 200)
	for i := range largeOutput {
		largeOutput[i] = byte('A' + (i % 26))
	}

	exitCode := 0
	event := &Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                EventTypeCmdExec,
		ContentText:         "Large output test",
		CommandRaw:          "echo large",
		CommandExitCode:     &exitCode,
		CommandStdout:       string(largeOutput),
		CommandStderr:       "",
		ExecutionDurationMs: 50,
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	assert.Greater(t, eventID, int64(0))

	// Retrieve and verify truncation
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	retrievedEvent := events[0]
	assert.True(t, retrievedEvent.StdoutTruncated)
	assert.Contains(t, retrievedEvent.CommandStdout, "[TRUNCATED:")
}

func TestAuditVaultService_FileMutation(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create a session
	operatorSessionID := "test-session-mutation"
	err = avs.CreateSession(operatorSessionID, "Mutation Test", "user@example.com")
	require.NoError(t, err)

	// Record a file mutation event
	exitCode := 0
	event := &Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                EventTypeFileMutation,
		ContentText:         "Write config file",
		CommandRaw:          "file_write /etc/nginx/nginx.conf",
		CommandExitCode:     &exitCode,
		ExecutionDurationMs: 25,
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)

	// Record file mutation log
	mutation := &FileMutationLog{
		EventID:          eventID,
		Filepath:         "/etc/nginx/nginx.conf",
		Operation:        FileMutationWrite,
		LedgerHashBefore: "abc123",
		LedgerHashAfter:  "def456",
		DiffStat:         "+5 lines, -2 lines",
	}

	err = avs.RecordFileMutation(mutation)
	require.NoError(t, err)

	// Retrieve file mutations
	mutations, err := avs.GetFileMutations(eventID)
	require.NoError(t, err)
	require.Len(t, mutations, 1)

	retrievedMutation := mutations[0]
	assert.Equal(t, "/etc/nginx/nginx.conf", retrievedMutation.Filepath)
	assert.Equal(t, FileMutationWrite, retrievedMutation.Operation)
	assert.Equal(t, "abc123", retrievedMutation.LedgerHashBefore)
	assert.Equal(t, "def456", retrievedMutation.LedgerHashAfter)
}

func TestAuditVaultService_Disabled(t *testing.T) {
	config := &AuditVaultConfig{
		Enabled: false,
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	assert.Nil(t, avs)
}

func TestAuditVaultService_MultipleSessions(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
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
		err := avs.CreateSession(s.id, s.title, s.user)
		require.NoError(t, err)
	}

	// Verify each session exists with correct data
	for _, s := range sessions {
		session, err := avs.GetSession(s.id)
		require.NoError(t, err)
		require.NotNil(t, session)
		assert.Equal(t, s.id, session.ID)
		assert.Equal(t, s.title, session.Title)
		assert.Equal(t, s.user, session.UserIdentity)
	}
}

func TestAuditVaultService_EventPagination(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-pagination-session"
	err = avs.CreateSession(operatorSessionID, "Pagination Test", "user@test.com")
	require.NoError(t, err)

	// Create 25 events
	exitCode := 0
	for i := 0; i < 25; i++ {
		event := &Event{
			OperatorSessionID:   operatorSessionID,
			Timestamp:           time.Now().UTC(),
			Type:                EventTypeCmdExec,
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
	assert.Len(t, events, 0)

	// Test default limit (0 should default to 50)
	events, err = avs.GetEvents(operatorSessionID, 0, 0)
	require.NoError(t, err)
	assert.Len(t, events, 25) // All events since we have less than 50
}

func TestAuditVaultService_EventOrdering(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-ordering-session"
	err = avs.CreateSession(operatorSessionID, "Ordering Test", "user@test.com")
	require.NoError(t, err)

	// Create events with increasing timestamps
	baseTime := time.Now().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		event := &Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         baseTime.Add(time.Duration(i) * time.Minute),
			Type:              EventTypeCmdExec,
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

func TestAuditVaultService_MultipleFileMutationsPerEvent(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-multi-mutation-session"
	err = avs.CreateSession(operatorSessionID, "Multi-Mutation Test", "user@test.com")
	require.NoError(t, err)

	// Create an event
	exitCode := 0
	event := &Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeFileMutation,
		ContentText:       "Batch file operation",
		CommandExitCode:   &exitCode,
	}
	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)

	// Record multiple file mutations for the same event
	files := []string{"/etc/nginx/nginx.conf", "/etc/hosts", "/var/log/app.log"}
	for i, file := range files {
		mutation := &FileMutationLog{
			EventID:          eventID,
			Filepath:         file,
			Operation:        FileMutationWrite,
			LedgerHashBefore: fmt.Sprintf("before_%d", i),
			LedgerHashAfter:  fmt.Sprintf("after_%d", i),
			DiffStat:         fmt.Sprintf("+%d lines", i+1),
		}
		err = avs.RecordFileMutation(mutation)
		require.NoError(t, err)
	}

	// Retrieve all mutations for the event
	mutations, err := avs.GetFileMutations(eventID)
	require.NoError(t, err)
	assert.Len(t, mutations, 3)

	// Verify each mutation
	for i, m := range mutations {
		assert.Equal(t, files[i], m.Filepath)
		assert.Equal(t, FileMutationWrite, m.Operation)
	}
}

func TestAuditVaultService_NullExitCode(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-null-exit-session"
	err = avs.CreateSession(operatorSessionID, "Null Exit Test", "user@test.com")
	require.NoError(t, err)

	// Create event with nil exit code
	event := &Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeUserMsg,
		ContentText:       "User message without exit code",
		CommandExitCode:   nil, // No exit code for user messages
	}
	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	assert.Greater(t, eventID, int64(0))

	// Retrieve and verify nil exit code
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Nil(t, events[0].CommandExitCode)
}

func TestAuditVaultService_DifferentEventTypes(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-event-types-session"
	err = avs.CreateSession(operatorSessionID, "Event Types Test", "user@test.com")
	require.NoError(t, err)

	eventTypes := []EventType{
		EventTypeUserMsg,
		EventTypeAIMsg,
		EventTypeCmdExec,
		EventTypeFileMutation,
	}

	for _, eventType := range eventTypes {
		event := &Event{
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
	types := make(map[EventType]bool)
	for _, e := range events {
		types[e.Type] = true
	}
	for _, et := range eventTypes {
		assert.True(t, types[et], "Missing event type: %s", et)
	}
}

func TestAuditVaultService_StderrTruncation(t *testing.T) {
	tempDir := t.TempDir()

	config := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 100,
		HeadTailSize:              30,
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-stderr-truncation"
	err = avs.CreateSession(operatorSessionID, "Stderr Truncation Test", "user@test.com")
	require.NoError(t, err)

	// Large stderr output
	largeStderr := make([]byte, 200)
	for i := range largeStderr {
		largeStderr[i] = byte('E' + (i % 26))
	}

	exitCode := 1
	event := &Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeCmdExec,
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

func TestAuditVaultService_GetSessionNotFound(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	session, err := avs.GetSession("non-existent-session")
	require.NoError(t, err)
	assert.Nil(t, session)
}

func TestAuditVaultService_GetEventsEmptySession(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "empty-session"
	err = avs.CreateSession(operatorSessionID, "Empty OperatorSession", "user@test.com")
	require.NoError(t, err)

	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, events, 0)
}

func TestAuditVaultService_GetFileMutationsNoMutations(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Non-existent event ID
	mutations, err := avs.GetFileMutations(99999)
	require.NoError(t, err)
	assert.Len(t, mutations, 0)
}

func TestAuditVaultService_WALMode(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// WAL mode should create additional files
	dbPath := filepath.Join(tempDir, "test.db")
	assert.FileExists(t, dbPath)

	// WAL file should be created after some activity
	err = avs.CreateSession("wal-test", "WAL Test", "user@test.com")
	require.NoError(t, err)

	// The -wal and -shm files may or may not exist depending on activity
	// Just verify the main db exists and works
	assert.FileExists(t, dbPath)
}

func TestAuditVaultService_IsEnabled(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	assert.True(t, avs.IsEnabled())

	// Nil service
	var nilService *AuditVaultService
	assert.False(t, nilService.IsEnabled())
}

func TestAuditVaultService_GetDataDir(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	assert.Equal(t, tempDir, avs.GetDataDir())

	// Nil service
	var nilService *AuditVaultService
	assert.Empty(t, nilService.GetDataDir())
}

func TestAuditVaultService_GetLedgerPath(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	ledgerPath := avs.GetLedgerPath()
	assert.Contains(t, ledgerPath, "ledger")
	assert.DirExists(t, ledgerPath)

	// Nil service
	var nilService *AuditVaultService
	assert.Empty(t, nilService.GetLedgerPath())
}

func TestAuditVaultService_NilServiceMethods(t *testing.T) {
	var avs *AuditVaultService

	// These should not panic and return gracefully
	err := avs.CreateSession("id", "title", "user")
	assert.NoError(t, err)

	eventID, err := avs.RecordEvent(&Event{})
	assert.NoError(t, err)
	assert.Equal(t, int64(0), eventID)

	err = avs.RecordFileMutation(&FileMutationLog{})
	assert.NoError(t, err)

	err = avs.Close()
	assert.NoError(t, err)
}

func TestAuditVaultService_DefaultConfig(t *testing.T) {
	// Verify default config uses hardcoded values (no env var overrides)
	// VSA uses CLI flags only, not environment variables for configuration
	config := DefaultAuditVaultConfig()
	assert.Equal(t, "./.g8e/data", config.DataDir)
	assert.Equal(t, "g8e.db", config.DBPath)
	assert.Equal(t, "ledger", config.LedgerDir)
	assert.Equal(t, int64(2048), config.MaxDBSizeMB)
	assert.Equal(t, 90, config.RetentionDays)
}

func TestAuditVaultService_GetEncryptionVault(t *testing.T) {
	tempDir := t.TempDir()
	logger := testutil.NewTestLogger()

	// 1. Without encryption vault
	config1 := &AuditVaultConfig{
		DataDir: tempDir,
		DBPath:  "test1.db",
		Enabled: true,
	}
	avs1, err := NewAuditVaultService(config1, logger)
	require.NoError(t, err)
	defer avs1.Close()
	assert.Nil(t, avs1.GetEncryptionVault())

	// 2. With encryption vault
	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: filepath.Join(tempDir, "vault"),
		Logger:  logger,
	})
	require.NoError(t, err)
	defer v.Close()

	config2 := &AuditVaultConfig{
		DataDir:         tempDir,
		DBPath:          "test2.db",
		Enabled:         true,
		EncryptionVault: v,
	}
	avs2, err := NewAuditVaultService(config2, logger)
	require.NoError(t, err)
	defer avs2.Close()
	assert.Equal(t, v, avs2.GetEncryptionVault())

	// 3. Nil service
	var nilService *AuditVaultService
	assert.Nil(t, nilService.GetEncryptionVault())
}

func TestAuditVaultPrune(t *testing.T) {
	tempDir := t.TempDir()
	logger := testutil.NewTestLogger()
	config := &AuditVaultConfig{
		DataDir:       tempDir,
		DBPath:        "prune_test.db",
		Enabled:       true,
		RetentionDays: 7,
	}

	avs, err := NewAuditVaultService(config, logger)
	require.NoError(t, err)
	defer avs.Close()

	// 1. Insert sessions first to satisfy FK constraints
	_, err = avs.db.Exec("INSERT INTO sessions (id, title) VALUES (?, ?)", "old-session", "op-1")
	require.NoError(t, err)
	_, err = avs.db.Exec("INSERT INTO sessions (id, title) VALUES (?, ?)", "recent-session", "op-1")
	require.NoError(t, err)

	// 2. Insert events
	oldTime := time.Now().AddDate(0, 0, -10)
	oldTimestamp := sqliteutil.FormatTimestamp(oldTime)
	_, err = avs.db.Exec("INSERT INTO events (id, timestamp, type, operator_session_id) VALUES (?, ?, ?, ?)",
		1, oldTimestamp, "test.event", "old-session")
	require.NoError(t, err)

	// Insert a recent event
	recentTime := time.Now().AddDate(0, 0, -2)
	recentTimestamp := sqliteutil.FormatTimestamp(recentTime)
	_, err = avs.db.Exec("INSERT INTO events (id, timestamp, type, operator_session_id) VALUES (?, ?, ?, ?)",
		2, recentTimestamp, "test.event", "recent-session")
	require.NoError(t, err)

	// 3. Insert file mutations
	_, err = avs.db.Exec("INSERT INTO file_mutation_log (event_id, filepath, operation) VALUES (?, ?, ?)",
		1, "/tmp/old", "create")
	require.NoError(t, err)
	_, err = avs.db.Exec("INSERT INTO file_mutation_log (event_id, filepath, operation) VALUES (?, ?, ?)",
		2, "/tmp/recent", "create")
	require.NoError(t, err)

	// 4. Run pruning
	pruneFunc := auditVaultPrune(config)
	pruneFunc(avs.db, logger)

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

func TestAuditVaultService_LongContentFields(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-long-content"
	err = avs.CreateSession(operatorSessionID, "Long Content Test", "user@test.com")
	require.NoError(t, err)

	// Create event with long content (below truncation threshold)
	longContent := make([]byte, 50000)
	for i := range longContent {
		longContent[i] = byte('A' + (i % 26))
	}

	exitCode := 0
	event := &Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeCmdExec,
		ContentText:       string(longContent),
		CommandRaw:        "cat large_file",
		CommandExitCode:   &exitCode,
		CommandStdout:     string(longContent),
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	assert.Greater(t, eventID, int64(0))

	// Retrieve and verify
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.Equal(t, string(longContent), events[0].ContentText)
	assert.False(t, events[0].StdoutTruncated) // Below threshold
}

func TestAuditVaultService_FileMutationOperationTypes(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "test-mutation-types"
	err = avs.CreateSession(operatorSessionID, "Mutation Types Test", "user@test.com")
	require.NoError(t, err)

	operations := []FileMutationOperation{
		FileMutationWrite,
		FileMutationDelete,
		FileMutationCreate,
	}

	for _, op := range operations {
		event := &Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              EventTypeFileMutation,
		}
		eventID, err := avs.RecordEvent(event)
		require.NoError(t, err)

		mutation := &FileMutationLog{
			EventID:   eventID,
			Filepath:  fmt.Sprintf("/test/%s_file.txt", op),
			Operation: op,
		}
		err = avs.RecordFileMutation(mutation)
		require.NoError(t, err)

		// Verify operation type is stored correctly
		mutations, err := avs.GetFileMutations(eventID)
		require.NoError(t, err)
		require.Len(t, mutations, 1)
		assert.Equal(t, op, mutations[0].Operation)
	}
}

func TestAuditVaultService_SessionWithNullFields(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create session with empty optional fields
	err = avs.CreateSession("null-fields-session", "", "")
	require.NoError(t, err)

	session, err := avs.GetSession("null-fields-session")
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Equal(t, "null-fields-session", session.ID)
	assert.Empty(t, session.Title)
	assert.Empty(t, session.UserIdentity)
}

func TestAuditVaultService_CloseIdempotent(t *testing.T) {
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
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)

	// Close multiple times should not panic
	err = avs.Close()
	assert.NoError(t, err)

	// Second close might error (db already closed) but shouldn't panic
	_ = avs.Close()
}

// ============================================================================
// Encryption Integration Tests
// ============================================================================

func TestAuditVaultService_WithEncryption(t *testing.T) {
	tempDir := t.TempDir()

	// Create and initialize vault for encryption
	vaultDataDir := filepath.Join(tempDir, "vault")
	encVault := newTestVault(t, vaultDataDir, "test-api-key-for-encryption")
	defer encVault.Close()
	require.True(t, encVault.IsUnlocked())

	// Create audit vault with encryption enabled
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
		EncryptionVault:           encVault,
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Verify encryption is enabled
	assert.True(t, avs.IsEncryptionEnabled())

	// Create session
	err = avs.CreateSession("encrypted-session-1", "Encrypted Test", "test-user")
	require.NoError(t, err)

	// Record event with sensitive content
	sensitiveContent := "This is highly confidential command output with secrets"
	exitCode := 0
	event := &Event{
		OperatorSessionID: "encrypted-session-1",
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeCmdExec,
		ContentText:       "User message about secrets",
		CommandRaw:        "echo secret",
		CommandExitCode:   &exitCode,
		CommandStdout:     sensitiveContent,
		CommandStderr:     "Some error output",
	}

	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)
	require.Greater(t, eventID, int64(0))

	// Retrieve and verify decryption works
	events, err := avs.GetEvents("encrypted-session-1", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	retrievedEvent := events[0]
	assert.Equal(t, "User message about secrets", retrievedEvent.ContentText)
	assert.Equal(t, sensitiveContent, retrievedEvent.CommandStdout)
	assert.Equal(t, "Some error output", retrievedEvent.CommandStderr)
}

func TestAuditVaultService_EncryptedDataUnreadableWithoutKey(t *testing.T) {
	tempDir := t.TempDir()

	apiKey := "test-api-key-for-locking-test"

	// Create and initialize vault
	vaultDataDir := filepath.Join(tempDir, "vault")
	vault1 := newTestVault(t, vaultDataDir, apiKey)

	// Create audit vault with encryption
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
		EncryptionVault:           vault1,
	}

	avs1, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)

	// Write encrypted data
	err = avs1.CreateSession("locked-test-session", "Locked Test", "test-user")
	require.NoError(t, err)

	secretData := "TOP SECRET: Password is hunter2"
	exitCode := 0
	_, err = avs1.RecordEvent(&Event{
		OperatorSessionID: "locked-test-session",
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeCmdExec,
		CommandRaw:        "cat /etc/passwd",
		CommandExitCode:   &exitCode,
		CommandStdout:     secretData,
	})
	require.NoError(t, err)

	avs1.Close()
	vault1.Close()

	// Reopen database WITHOUT encryption vault (simulating access without the key)
	config2 := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           nil, // No vault = no decryption
	}

	avs2, err := NewAuditVaultService(config2, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs2.Close()

	// Encryption should be disabled
	assert.False(t, avs2.IsEncryptionEnabled())

	// Read events - they should be encrypted (gibberish)
	events, err := avs2.GetEvents("locked-test-session", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	// The stdout should NOT equal the original secret (it's encrypted binary)
	assert.NotEqual(t, secretData, events[0].CommandStdout)
	// It should contain binary data that doesn't match plaintext
	assert.NotContains(t, events[0].CommandStdout, "hunter2")
}

func TestAuditVaultService_EncryptionWithRekey(t *testing.T) {
	tempDir := t.TempDir()

	oldAPIKey := "old-api-key-before-refresh"
	newAPIKey := "new-api-key-after-refresh"

	// Initialize with old key
	vaultDataDir := filepath.Join(tempDir, "vault")
	vaultSvc := newTestVault(t, vaultDataDir, oldAPIKey)

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
		EncryptionVault:           vaultSvc,
	}

	avs, err := NewAuditVaultService(config, testutil.NewTestLogger())
	require.NoError(t, err)

	// Write data with old key
	err = avs.CreateSession("rekey-session", "Rekey Test", "test-user")
	require.NoError(t, err)

	originalData := "Data encrypted with old key"
	exitCode := 0
	_, err = avs.RecordEvent(&Event{
		OperatorSessionID: "rekey-session",
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeCmdExec,
		CommandRaw:        "echo test",
		CommandExitCode:   &exitCode,
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
	config2 := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           vault2,
	}

	avs2, err := NewAuditVaultService(config2, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs2.Close()
	defer vault2.Close()

	// Verify we can still read the data with the new key
	events, err := avs2.GetEvents("rekey-session", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, originalData, events[0].CommandStdout)
}

func TestAuditVaultService_MixedEncryptedUnencrypted(t *testing.T) {
	tempDir := t.TempDir()

	config1 := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           nil, // No encryption
	}

	avs1, err := NewAuditVaultService(config1, testutil.NewTestLogger())
	require.NoError(t, err)

	err = avs1.CreateSession("mixed-session", "Mixed Test", "test-user")
	require.NoError(t, err)

	unencryptedData := "This is plaintext data"
	exitCode := 0
	_, err = avs1.RecordEvent(&Event{
		OperatorSessionID: "mixed-session",
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeCmdExec,
		CommandRaw:        "echo plaintext",
		CommandExitCode:   &exitCode,
		CommandStdout:     unencryptedData,
	})
	require.NoError(t, err)

	avs1.Close()

	// Now create vault and write encrypted data
	vaultDataDir := filepath.Join(tempDir, "vault")
	encVault := newTestVault(t, vaultDataDir, "mixed-test-api-key")

	config2 := &AuditVaultConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           encVault,
	}

	avs2, err := NewAuditVaultService(config2, testutil.NewTestLogger())
	require.NoError(t, err)

	encryptedData := "This is encrypted data"
	_, err = avs2.RecordEvent(&Event{
		OperatorSessionID: "mixed-session",
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeCmdExec,
		CommandRaw:        "echo encrypted",
		CommandExitCode:   &exitCode,
		CommandStdout:     encryptedData,
	})
	require.NoError(t, err)

	// Read all events - should handle both encrypted and unencrypted
	events, err := avs2.GetEvents("mixed-session", 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 2)

	// Most recent first (encrypted)
	assert.Equal(t, encryptedData, events[0].CommandStdout)
	// Older one (unencrypted)
	assert.Equal(t, unencryptedData, events[1].CommandStdout)

	avs2.Close()
	encVault.Close()
}
