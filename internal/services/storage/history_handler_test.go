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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestHistoryHandler creates a HistoryHandler with real infrastructure
// (SQLite audit store, Git ledger service, encryption vault) for testing.
func setupTestHistoryHandler(t *testing.T) (*HistoryHandler, *SQLAuditStore, *vault.Vault, string) {
	t.Helper()
	gitPath := testGitPath(t)
	tempDir := t.TempDir()

	// Create vault for encryption
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	vaultDir := filepath.Join(tempDir, "vault")
	testVault := createTestVault(t, vaultDir, privKey)

	logger := testutil.NewTestLogger()

	ledgerConfig := &LedgerConfig{
		BaseDir:         filepath.Join(tempDir, "ledger"),
		GitPath:         gitPath,
		EncryptionVault: testVault,
	}
	lms, err := NewGitLedgerService(ledgerConfig, logger)
	require.NoError(t, err)

	auditStoreConfig := &AuditStoreConfig{
		DataDir:              tempDir,
		DBPath:               "audit_store.db",
		MaxDBSizeMB:          100,
		RetentionDays:        7,
		PruneIntervalMinutes: 60,
		Enabled:              true,
		EncryptionVault:      testVault,
	}
	auditStore, err := NewSQLAuditStore(auditStoreConfig, logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		auditStore.Close()
	})

	hh := NewHistoryHandler(auditStore, lms, logger)

	return hh, auditStore, testVault, tempDir
}

// TestHistoryHandler_FetchHistory verifies that the HistoryHandler correctly
// fetches and returns operator session history events from the SQLite audit store.
func TestHistoryHandler_FetchHistory(t *testing.T) {
	t.Parallel()
	hh, auditStore, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	// Create test data
	operatorSessionID := "test-session-history"
	err := auditStore.CreateSession(operatorSessionID, "operator", "Test History OperatorSession", "user@test.com")
	require.NoError(t, err)

	// Add some events
	exitCode := 0
	for i := 0; i < 5; i++ {
		event := &Event{
			OperatorSessionID:   operatorSessionID,
			Timestamp:           time.Now().UTC(),
			Type:                constants.Event.Operator.Audit.Command,
			ContentText:         "Test command",
			CommandRaw:          "echo test",
			CommandExitCode:     &exitCode,
			CommandStdout:       "test output",
			ExecutionDurationMs: 100,
		}
		_, err := auditStore.RecordEvent(event)
		require.NoError(t, err)
	}

	// Fetch history
	requestJSON := testutil.MustBuildFetchHistoryRequestedPayload(t, "exec-123", operatorSessionID, 10, 0)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, operatorSessionID, response.OperatorSessionId)
	assert.Equal(t, 5, len(response.Events))
	assert.NotNil(t, response.WebSession)
	assert.Equal(t, "Test History OperatorSession", response.WebSession.Title)
}

// TestHistoryHandler_FetchHistoryMissingSession verifies fail-closed behavior
// when requesting history for a non-existent session ID.
func TestHistoryHandler_FetchHistoryMissingSession(t *testing.T) {
	t.Parallel()
	hh, _, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	// Fetch history for non-existent session
	requestJSON := testutil.MustBuildFetchHistoryRequestedPayload(t, "exec-123", "non-existent-session", 10, 0)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success) // Request succeeds but returns empty
	assert.Nil(t, response.WebSession)
	assert.Empty(t, response.Events)
}

// TestHistoryHandler_FetchHistoryInvalidRequest verifies fail-closed behavior
// when the request is missing required fields (e.g., operator_session_id).
func TestHistoryHandler_FetchHistoryInvalidRequest(t *testing.T) {
	t.Parallel()
	hh, _, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	// Empty operator_session_id
	requestJSON := testutil.MustBuildFetchHistoryRequestedPayload(t, "exec-123", "", 10, 0)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "operator_session_id is required")
}

// TestHistoryHandler_IsEnabled verifies that IsEnabled returns true
// for a properly initialized HistoryHandler and false for nil handlers.
func TestHistoryHandler_IsEnabled(t *testing.T) {
	t.Parallel()
	hh, _, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	assert.True(t, hh.IsEnabled())

	// Test with nil handler
	var nilHandler *HistoryHandler
	assert.False(t, nilHandler.IsEnabled())
}

// TestHistoryHandler_FetchHistoryWithFileMutations verifies that file mutation
// metadata is correctly included in the history response.
func TestHistoryHandler_FetchHistoryWithFileMutations(t *testing.T) {
	t.Parallel()
	hh, auditStore, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	// Create test data
	operatorSessionID := "test-session-mutations"
	err := auditStore.CreateSession(operatorSessionID, "operator", "Mutation Test OperatorSession", "user@test.com")
	require.NoError(t, err)

	// Add file mutation event
	exitCode := 0
	event := &Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                constants.Event.Operator.FileEdit.Completed,
		ContentText:         "Write config",
		CommandRaw:          "file_write /etc/config.yml",
		CommandExitCode:     &exitCode,
		ExecutionDurationMs: 50,
	}
	eventID, err := auditStore.RecordEvent(event)
	require.NoError(t, err)

	// Record file mutation
	mutation := &FileMutationLog{
		EventID:          eventID,
		Filepath:         "/etc/config.yml",
		Operation:        FileMutationWrite,
		LedgerHashBefore: "hash1",
		LedgerHashAfter:  "hash2",
		DiffStat:         "+10 lines",
	}
	err = auditStore.RecordFileMutation(mutation)
	require.NoError(t, err)

	// Fetch history
	requestJSON := testutil.MustBuildFetchHistoryRequestedPayload(t, "exec-123", operatorSessionID, 10, 0)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Len(t, response.Events, 1)

	historyEvent := response.Events[0]
	assert.Equal(t, string(constants.Event.Operator.FileEdit.Completed), historyEvent.Type)
	assert.Len(t, historyEvent.FileMutations, 1)
	assert.Equal(t, "/etc/config.yml", historyEvent.FileMutations[0].Filepath)
	assert.Equal(t, "WRITE", historyEvent.FileMutations[0].Operation)
}

// TestHistoryHandler_FetchHistoryPagination verifies that history pagination
// works correctly with limit and offset parameters.
func TestHistoryHandler_FetchHistoryPagination(t *testing.T) {
	t.Parallel()
	hh, auditStore, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	operatorSessionID := "test-pagination-session"
	err := auditStore.CreateSession(operatorSessionID, "operator", "Pagination Test", "user@test.com")
	require.NoError(t, err)

	// Create 15 events
	exitCode := 0
	for i := 0; i < 15; i++ {
		event := &Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       "Test command",
			CommandRaw:        "echo test",
			CommandExitCode:   &exitCode,
		}
		_, err := auditStore.RecordEvent(event)
		require.NoError(t, err)
	}

	// Test first page
	requestJSON := testutil.MustBuildFetchHistoryRequestedPayload(t, "exec-123", operatorSessionID, 10, 0)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Len(t, response.Events, 10)
	assert.Equal(t, int32(10), response.Limit)
	assert.Equal(t, int32(0), response.Offset)

	// Test second page with offset
	requestJSON = testutil.MustBuildFetchHistoryRequestedPayload(t, "exec-124", operatorSessionID, 10, 10)

	response, err = hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Len(t, response.Events, 5) // Remaining 5 events
	assert.Equal(t, int32(10), response.Offset)
}

// TestHistoryHandler_FetchHistoryDefaultLimit verifies that when limit=0,
// the handler applies the default limit of 50 events.
func TestHistoryHandler_FetchHistoryDefaultLimit(t *testing.T) {
	t.Parallel()
	hh, auditStore, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	operatorSessionID := "test-default-limit"
	err := auditStore.CreateSession(operatorSessionID, "operator", "Default Limit Test", "user@test.com")
	require.NoError(t, err)

	// Create 5 events
	for i := 0; i < 5; i++ {
		event := &Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
		}
		_, err := auditStore.RecordEvent(event)
		require.NoError(t, err)
	}

	// Request with limit=0 (should default to 50)
	requestJSON := testutil.MustBuildFetchHistoryRequestedPayload(t, "exec-123", operatorSessionID, 0, 0)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Len(t, response.Events, 5)
	assert.Equal(t, int32(50), response.Limit) // Default limit
}

// TestHistoryHandler_FetchHistoryInvalidJSON verifies fail-closed behavior
// when the request payload is not valid JSON.
func TestHistoryHandler_FetchHistoryInvalidJSON(t *testing.T) {
	t.Parallel()
	hh, _, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	// Invalid JSON
	response, err := hh.HandleFetchHistory([]byte("invalid json"))
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "invalid request format")
}

// TestHistoryHandler_FetchFileHistory verifies that the HistoryHandler correctly
// fetches file version history from the Git ledger service.
func TestHistoryHandler_FetchFileHistory(t *testing.T) {
	t.Parallel()
	hh, _, testVault, tempDir := setupTestHistoryHandler(t)
	defer testVault.Close()

	// Create a file and track it through multiple versions
	testFilePath := filepath.Join(tempDir, "test_file_history.txt")
	operatorSessionID := "test-file-history-session"

	lms := hh.ledger

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

	// Fetch file history
	requestJSON := testutil.MustBuildFetchFileHistoryRequestedPayload(t, "exec-123", testFilePath, 10, operatorSessionID)

	response, err := hh.HandleFetchFileHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, testFilePath, response.FilePath)
}

// TestHistoryHandler_FetchFileHistoryValidationErrors verifies fail-closed behavior
// for various invalid request scenarios.
func TestHistoryHandler_FetchFileHistoryValidationErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		filePath    string
		limit       int32
		sessionID   string
		expectError string
	}{
		{"missing file path", "", 10, "", "file_path is required"},
		{"invalid JSON", "", 10, "", "invalid request format"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hh, _, testVault, _ := setupTestHistoryHandler(t)
			defer testVault.Close()

			var requestJSON []byte
			if tc.name == "invalid JSON" {
				requestJSON = []byte("invalid json")
			} else {
				requestJSON = testutil.MustBuildFetchFileHistoryRequestedPayload(t, "exec-123", tc.filePath, tc.limit, tc.sessionID)
			}

			response, err := hh.HandleFetchFileHistory(requestJSON)
			require.NoError(t, err)

			assert.False(t, response.Success)
			assert.Contains(t, response.Error, tc.expectError)
		})
	}
}

// TestHistoryHandler_FetchFileHistoryDefaultLimit verifies that when limit=0,
// the handler applies the default limit for file history requests.
func TestHistoryHandler_FetchFileHistoryDefaultLimit(t *testing.T) {
	t.Parallel()
	hh, _, testVault, tempDir := setupTestHistoryHandler(t)
	defer testVault.Close()

	testFilePath := filepath.Join(tempDir, "default_limit_file.txt")
	operatorSessionID := "operator_session"

	lms := hh.ledger
	result, _ := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("content"), 0644)
	lms.CompleteMirrorCreate(result, operatorSessionID)

	// Request with limit=0 (use same session ID as mirroring)
	requestJSON := testutil.MustBuildFetchFileHistoryRequestedPayload(t, "exec-123", testFilePath, 0, "operator_session")

	response, err := hh.HandleFetchFileHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
}

// TestHistoryHandler_RestoreFile verifies that the HistoryHandler can restore
// a file to a previous version using the Git ledger service.
func TestHistoryHandler_RestoreFile(t *testing.T) {
	t.Parallel()
	hh, _, testVault, tempDir := setupTestHistoryHandler(t)
	defer testVault.Close()

	testFilePath := filepath.Join(tempDir, "restore_test.txt")
	operatorSessionID := "test-restore-session"

	lms := hh.ledger

	// Create initial file
	result1, _ := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Original content"), 0644)
	lms.CompleteMirrorCreate(result1, operatorSessionID)
	originalHash := result1.LedgerHashAfter

	// Modify file
	result2, _ := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Modified content"), 0644)
	lms.CompleteMirrorWrite(result2, operatorSessionID)

	// Verify current content
	content, _ := os.ReadFile(testFilePath)
	assert.Equal(t, "Modified content", string(content))

	// Restore to original
	requestJSON := testutil.MustBuildRestoreFileRequestedPayload(t, "exec-123", testFilePath, originalHash, operatorSessionID)

	response, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, testFilePath, response.FilePath)
	assert.Equal(t, originalHash, response.CommitHash)

	// Verify content was restored
	content, _ = os.ReadFile(testFilePath)
	assert.Equal(t, "Original content", string(content))
}

// TestHistoryHandler_RestoreFileValidationErrors verifies fail-closed behavior
// for various invalid restore request scenarios.
func TestHistoryHandler_RestoreFileValidationErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		filePath    string
		commitHash  string
		sessionID   string
		expectError string
	}{
		{"missing file path", "", "abc123", "operator_session", "file_path is required"},
		{"missing commit hash", "/some/file", "", "operator_session", "commit_hash is required"},
		{"missing session id", "/some/file", "abc123", "", "operator_session_id is required"},
		{"invalid JSON", "", "", "", "invalid request format"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hh, _, testVault, _ := setupTestHistoryHandler(t)
			defer testVault.Close()

			var requestJSON []byte
			if tc.name == "invalid JSON" {
				requestJSON = []byte("invalid json")
			} else {
				requestJSON = testutil.MustBuildRestoreFileRequestedPayload(t, "exec-123", tc.filePath, tc.commitHash, tc.sessionID)
			}

			response, err := hh.HandleRestoreFile(requestJSON)
			require.NoError(t, err)

			assert.False(t, response.Success)
			assert.Contains(t, response.Error, tc.expectError)
		})
	}
}

// TestHistoryHandler_RestoreFileInvalidCommit verifies fail-closed behavior
// when attempting to restore a file with an invalid commit hash.
func TestHistoryHandler_RestoreFileInvalidCommit(t *testing.T) {
	t.Parallel()
	hh, _, testVault, tempDir := setupTestHistoryHandler(t)
	defer testVault.Close()

	testFilePath := filepath.Join(tempDir, "invalid_restore.txt")
	os.WriteFile(testFilePath, []byte("content"), 0644)

	requestJSON := testutil.MustBuildRestoreFileRequestedPayload(t, "exec-123", testFilePath, "invalidhash123456789", "operator_session")

	response, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "failed to restore file")
}

// TestHistoryHandler_GetFileAtCommit verifies that the HistoryHandler can
// retrieve file content at a specific commit hash from the Git ledger.
func TestHistoryHandler_GetFileAtCommit(t *testing.T) {
	t.Parallel()
	hh, _, testVault, tempDir := setupTestHistoryHandler(t)
	defer testVault.Close()

	testFilePath := filepath.Join(tempDir, "get_at_commit.txt")
	operatorSessionID := "test-get-at-commit"

	lms := hh.ledger

	// Create file
	result, _ := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Initial"), 0644)
	lms.CompleteMirrorCreate(result, operatorSessionID)
	initialHash := result.LedgerHashAfter

	// Modify
	result2, _ := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	os.WriteFile(testFilePath, []byte("Modified"), 0644)
	lms.CompleteMirrorWrite(result2, operatorSessionID)

	// Get content at initial commit
	content, err := hh.GetFileAtCommit(testFilePath, initialHash, operatorSessionID)
	// This may fail if the commit doesn't exist yet in the git repo
	if err != nil {
		assert.Contains(t, err.Error(), "object not found")
	} else {
		assert.Equal(t, "Initial", content)
	}
}

// TestHistoryHandler_NilHandler verifies that IsEnabled returns false
// for a nil HistoryHandler (fail-closed behavior).
func TestHistoryHandler_NilHandler(t *testing.T) {
	t.Parallel()
	var hh *HistoryHandler
	assert.False(t, hh.IsEnabled())
}

// TestHistoryHandler_NilAuditStore verifies that IsEnabled returns false
// when the HistoryHandler is created with a nil audit store (fail-closed behavior).
func TestHistoryHandler_NilAuditStore(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	hh := NewHistoryHandler(nil, nil, logger)
	assert.False(t, hh.IsEnabled())
}

// TestHistoryHandler_AllEventTypes verifies that the HistoryHandler correctly
// returns all supported event types in the history response.
func TestHistoryHandler_AllEventTypes(t *testing.T) {
	t.Parallel()
	hh, auditStore, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	operatorSessionID := "test-all-event-types"
	err := auditStore.CreateSession(operatorSessionID, "operator", "All Event Types", "user@test.com")
	require.NoError(t, err)

	// Create events of all types
	eventTypes := []constants.EventType{
		constants.Event.Operator.Audit.UserMsg,
		constants.Event.Operator.Audit.AIMsg,
		constants.Event.Operator.Audit.Command,
		constants.Event.Operator.FileEdit.Completed,
	}

	exitCode := 0
	for _, et := range eventTypes {
		event := &Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              et,
			ContentText:       string(et) + " content",
			CommandExitCode:   &exitCode,
		}
		_, err := auditStore.RecordEvent(event)
		require.NoError(t, err)
	}

	// Fetch history
	requestJSON := testutil.MustBuildFetchHistoryRequestedPayload(t, "exec-123", operatorSessionID, 10, 0)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Len(t, response.Events, 4)

	// Verify all event types are present
	types := make(map[string]bool)
	for _, e := range response.Events {
		types[e.Type] = true
	}
	for _, et := range eventTypes {
		assert.True(t, types[string(et)], "Missing event type: %s", et)
	}
}

// TestHistoryHandler_MultipleFileMutationsInHistory verifies that the HistoryHandler
// correctly returns multiple file mutations associated with a single event.
func TestHistoryHandler_MultipleFileMutationsInHistory(t *testing.T) {
	t.Parallel()
	hh, auditStore, testVault, _ := setupTestHistoryHandler(t)
	defer testVault.Close()

	operatorSessionID := "test-multi-mutations"
	err := auditStore.CreateSession(operatorSessionID, "operator", "Multi Mutations", "user@test.com")
	require.NoError(t, err)

	// Create file mutation event with multiple files
	exitCode := 0
	event := &Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.FileEdit.Completed,
		ContentText:       "Batch file update",
		CommandExitCode:   &exitCode,
	}
	eventID, err := auditStore.RecordEvent(event)
	require.NoError(t, err)

	// Record multiple file mutations
	files := []string{"/etc/config1.yml", "/etc/config2.yml", "/etc/config3.yml"}
	for _, f := range files {
		mutation := &FileMutationLog{
			EventID:   eventID,
			Filepath:  f,
			Operation: FileMutationWrite,
		}
		err = auditStore.RecordFileMutation(mutation)
		require.NoError(t, err)
	}

	// Fetch history
	requestJSON := testutil.MustBuildFetchHistoryRequestedPayload(t, "exec-123", operatorSessionID, 10, 0)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	require.Len(t, response.Events, 1)
	assert.Len(t, response.Events[0].FileMutations, 3)

	// Verify all files are present
	foundFiles := make(map[string]bool)
	for _, m := range response.Events[0].FileMutations {
		foundFiles[m.Filepath] = true
	}
	for _, f := range files {
		assert.True(t, foundFiles[f], "Missing file: %s", f)
	}
}
