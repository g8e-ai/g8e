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
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHistoryHandler(t *testing.T) (*HistoryHandler, *AuditVaultService, string) {
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
	hh := NewHistoryHandler(avs, lms, logger)

	return hh, avs, tempDir
}

func TestHistoryHandler_FetchHistory(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	// Create test data
	operatorSessionID := "test-session-history"
	err := avs.CreateSession(operatorSessionID, "Test History OperatorSession", "user@test.com")
	require.NoError(t, err)

	// Add some events
	exitCode := 0
	for i := 0; i < 5; i++ {
		event := &Event{
			OperatorSessionID:   operatorSessionID,
			Timestamp:           time.Now().UTC(),
			Type:                EventTypeCmdExec,
			ContentText:         "Test command",
			CommandRaw:          "echo test",
			CommandExitCode:     &exitCode,
			CommandStdout:       "test output",
			ExecutionDurationMs: 100,
		}
		_, err := avs.RecordEvent(event)
		require.NoError(t, err)
	}

	// Fetch history
	request := fetchHistoryRequest{
		OperatorSessionID: operatorSessionID,
		Limit:             10,
		Offset:            0,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, operatorSessionID, response.OperatorSessionID)
	assert.Equal(t, 5, len(response.Events))
	assert.NotNil(t, response.WebSession)
	assert.Equal(t, "Test History OperatorSession", response.WebSession.Title)
}

func TestHistoryHandler_FetchHistoryMissingSession(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	// Fetch history for non-existent session
	request := fetchHistoryRequest{
		OperatorSessionID: "non-existent-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success) // Request succeeds but returns empty
	assert.Nil(t, response.WebSession)
	assert.Empty(t, response.Events)
}

func TestHistoryHandler_FetchHistoryInvalidRequest(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	// Empty operator_session_id
	request := fetchHistoryRequest{
		OperatorSessionID: "",
		Limit:             10,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "operator_session_id is required")
}

func TestHistoryHandler_IsEnabled(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	assert.True(t, hh.IsEnabled())

	// Test with nil handler
	var nilHandler *HistoryHandler
	assert.False(t, nilHandler.IsEnabled())
}

func TestHistoryHandler_FetchHistoryWithFileMutations(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	// Create test data
	operatorSessionID := "test-session-mutations"
	err := avs.CreateSession(operatorSessionID, "Mutation Test OperatorSession", "user@test.com")
	require.NoError(t, err)

	// Add file mutation event
	exitCode := 0
	event := &Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                EventTypeFileMutation,
		ContentText:         "Write config",
		CommandRaw:          "file_write /etc/config.yml",
		CommandExitCode:     &exitCode,
		ExecutionDurationMs: 50,
	}
	eventID, err := avs.RecordEvent(event)
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
	err = avs.RecordFileMutation(mutation)
	require.NoError(t, err)

	// Fetch history
	request := fetchHistoryRequest{
		OperatorSessionID: operatorSessionID,
		Limit:             10,
		Offset:            0,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Len(t, response.Events, 1)

	historyEvent := response.Events[0]
	assert.Equal(t, "FILE_MUTATION", historyEvent.Type)
	assert.Len(t, historyEvent.FileMutations, 1)
	assert.Equal(t, "/etc/config.yml", historyEvent.FileMutations[0].Filepath)
	assert.Equal(t, "WRITE", historyEvent.FileMutations[0].Operation)
}

func TestHistoryHandler_FetchHistoryPagination(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	operatorSessionID := "test-pagination-session"
	err := avs.CreateSession(operatorSessionID, "Pagination Test", "user@test.com")
	require.NoError(t, err)

	// Create 15 events
	exitCode := 0
	for i := 0; i < 15; i++ {
		event := &Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              EventTypeCmdExec,
			ContentText:       "Test command",
			CommandRaw:        "echo test",
			CommandExitCode:   &exitCode,
		}
		_, err := avs.RecordEvent(event)
		require.NoError(t, err)
	}

	// Test first page
	request := fetchHistoryRequest{
		OperatorSessionID: operatorSessionID,
		Limit:             10,
		Offset:            0,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Len(t, response.Events, 10)
	assert.Equal(t, 10, response.Limit)
	assert.Equal(t, 0, response.Offset)

	// Test second page with offset
	request.Offset = 10
	requestJSON, _ = json.Marshal(request)

	response, err = hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Len(t, response.Events, 5) // Remaining 5 events
	assert.Equal(t, 10, response.Offset)
}

func TestHistoryHandler_FetchHistoryDefaultLimit(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	operatorSessionID := "test-default-limit"
	err := avs.CreateSession(operatorSessionID, "Default Limit Test", "user@test.com")
	require.NoError(t, err)

	// Create 5 events
	for i := 0; i < 5; i++ {
		event := &Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              EventTypeCmdExec,
		}
		_, err := avs.RecordEvent(event)
		require.NoError(t, err)
	}

	// Request with limit=0 (should default to 50)
	request := fetchHistoryRequest{
		OperatorSessionID: operatorSessionID,
		Limit:             0,
		Offset:            0,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Len(t, response.Events, 5)
	assert.Equal(t, 50, response.Limit) // Default limit
}

func TestHistoryHandler_FetchHistoryInvalidJSON(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	// Invalid JSON
	response, err := hh.HandleFetchHistory([]byte("invalid json"))
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "invalid request format")
}

func TestHistoryHandler_FetchFileHistory(t *testing.T) {
	hh, avs, tempDir := setupTestHistoryHandler(t)
	defer avs.Close()

	// Create a file and track it through multiple versions
	testFilePath := tempDir + "/test_file_history.txt"
	operatorSessionID := "test-file-history-session"

	// Create the file
	err := os.WriteFile(testFilePath, []byte("Version 1"), 0644)
	require.NoError(t, err)

	// Mirror the file creation
	lms := hh.ledger
	result, _ := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	lms.CompleteMirrorCreate(result, operatorSessionID)

	// Modify the file
	os.WriteFile(testFilePath, []byte("Version 2"), 0644)
	result2, _ := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	lms.CompleteMirrorWrite(result2, operatorSessionID)

	// Fetch file history
	request := fetchFileHistoryRequest{
		FilePath: testFilePath,
		Limit:    10,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchFileHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, testFilePath, response.FilePath)
	assert.GreaterOrEqual(t, len(response.History), 1)
}

func TestHistoryHandler_FetchFileHistoryMissingFilePath(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	request := fetchFileHistoryRequest{
		FilePath: "",
		Limit:    10,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchFileHistory(requestJSON)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "file_path is required")
}

func TestHistoryHandler_FetchFileHistoryDefaultLimit(t *testing.T) {
	hh, avs, tempDir := setupTestHistoryHandler(t)
	defer avs.Close()

	testFilePath := tempDir + "/default_limit_file.txt"
	err := os.WriteFile(testFilePath, []byte("content"), 0644)
	require.NoError(t, err)

	lms := hh.ledger
	result, _ := lms.MirrorFileCreate("operator_session", testFilePath)
	lms.CompleteMirrorCreate(result, "operator_session")

	// Request with limit=0
	request := fetchFileHistoryRequest{
		FilePath: testFilePath,
		Limit:    0,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchFileHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
}

func TestHistoryHandler_FetchFileHistoryInvalidJSON(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	response, err := hh.HandleFetchFileHistory([]byte("invalid json"))
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "invalid request format")
}

func TestHistoryHandler_RestoreFile(t *testing.T) {
	hh, avs, tempDir := setupTestHistoryHandler(t)
	defer avs.Close()

	testFilePath := tempDir + "/restore_test.txt"
	operatorSessionID := "test-restore-session"

	// Create initial file
	err := os.WriteFile(testFilePath, []byte("Original content"), 0644)
	require.NoError(t, err)

	lms := hh.ledger
	result1, _ := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	lms.CompleteMirrorCreate(result1, operatorSessionID)
	originalHash := result1.LedgerHashAfter

	// Modify file
	os.WriteFile(testFilePath, []byte("Modified content"), 0644)
	result2, _ := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	lms.CompleteMirrorWrite(result2, operatorSessionID)

	// Verify current content
	content, _ := os.ReadFile(testFilePath)
	assert.Equal(t, "Modified content", string(content))

	// Restore to original
	request := restoreFileRequest{
		FilePath:          testFilePath,
		CommitHash:        originalHash,
		OperatorSessionID: operatorSessionID,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, testFilePath, response.FilePath)
	assert.Equal(t, originalHash, response.CommitHash)

	// Verify content was restored
	content, _ = os.ReadFile(testFilePath)
	assert.Equal(t, "Original content", string(content))
}

func TestHistoryHandler_RestoreFileMissingFilePath(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	request := restoreFileRequest{
		FilePath:          "",
		CommitHash:        "abc123",
		OperatorSessionID: "operator_session",
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "file_path is required")
}

func TestHistoryHandler_RestoreFileMissingCommitHash(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	request := restoreFileRequest{
		FilePath:          "/some/file",
		CommitHash:        "",
		OperatorSessionID: "operator_session",
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "commit_hash is required")
}

func TestHistoryHandler_RestoreFileMissingSessionID(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	request := restoreFileRequest{
		FilePath:          "/some/file",
		CommitHash:        "abc123",
		OperatorSessionID: "",
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "operator_session_id is required")
}

func TestHistoryHandler_RestoreFileInvalidJSON(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	response, err := hh.HandleRestoreFile([]byte("invalid json"))
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "invalid request format")
}

func TestHistoryHandler_RestoreFileInvalidCommit(t *testing.T) {
	hh, avs, tempDir := setupTestHistoryHandler(t)
	defer avs.Close()

	testFilePath := tempDir + "/invalid_restore.txt"
	os.WriteFile(testFilePath, []byte("content"), 0644)

	request := restoreFileRequest{
		FilePath:          testFilePath,
		CommitHash:        "invalidhash123456789",
		OperatorSessionID: "operator_session",
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.False(t, response.Success)
	assert.Contains(t, response.Error, "failed to restore file")
}

func TestHistoryHandler_GetFileAtCommit(t *testing.T) {
	hh, avs, tempDir := setupTestHistoryHandler(t)
	defer avs.Close()

	testFilePath := tempDir + "/get_at_commit.txt"
	operatorSessionID := "test-get-at-commit"

	// Create file
	os.WriteFile(testFilePath, []byte("Initial"), 0644)
	lms := hh.ledger
	result, _ := lms.MirrorFileCreate(operatorSessionID, testFilePath)
	lms.CompleteMirrorCreate(result, operatorSessionID)
	initialHash := result.LedgerHashAfter

	// Modify
	os.WriteFile(testFilePath, []byte("Modified"), 0644)
	result2, _ := lms.LedgerFileWrite(operatorSessionID, testFilePath)
	lms.CompleteMirrorWrite(result2, operatorSessionID)

	// Get content at initial commit
	content, err := hh.GetFileAtCommit(testFilePath, initialHash)
	require.NoError(t, err)
	assert.Equal(t, "Initial", content)
}

func TestHistoryHandler_NilHandler(t *testing.T) {
	var hh *HistoryHandler
	assert.False(t, hh.IsEnabled())
}

func TestHistoryHandler_NilAuditVault(t *testing.T) {
	logger := testutil.NewTestLogger()
	hh := NewHistoryHandler(nil, nil, logger)
	assert.False(t, hh.IsEnabled())
}

func TestHistoryHandler_AllEventTypes(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	operatorSessionID := "test-all-event-types"
	err := avs.CreateSession(operatorSessionID, "All Event Types", "user@test.com")
	require.NoError(t, err)

	// Create events of all types
	eventTypes := []EventType{
		EventTypeUserMsg,
		EventTypeAIMsg,
		EventTypeCmdExec,
		EventTypeFileMutation,
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
		_, err := avs.RecordEvent(event)
		require.NoError(t, err)
	}

	// Fetch history
	request := fetchHistoryRequest{
		OperatorSessionID: operatorSessionID,
		Limit:             10,
	}
	requestJSON, _ := json.Marshal(request)

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

func TestHistoryHandler_EventWithTruncatedOutput(t *testing.T) {
	tempDir := t.TempDir()

	// Small truncation threshold
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

	logger := testutil.NewTestLogger()
	avs, err := NewAuditVaultService(config, logger)
	require.NoError(t, err)
	defer avs.Close()

	lms := NewLedgerService(avs, nil, logger)
	hh := NewHistoryHandler(avs, lms, logger)

	operatorSessionID := "test-truncated-output"
	err = avs.CreateSession(operatorSessionID, "Truncated Output", "user@test.com")
	require.NoError(t, err)

	// Create event with large output
	largeOutput := make([]byte, 200)
	for i := range largeOutput {
		largeOutput[i] = byte('A' + (i % 26))
	}

	exitCode := 0
	event := &Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeCmdExec,
		CommandRaw:        "large_output_cmd",
		CommandExitCode:   &exitCode,
		CommandStdout:     string(largeOutput),
	}
	_, err = avs.RecordEvent(event)
	require.NoError(t, err)

	// Fetch history
	request := fetchHistoryRequest{
		OperatorSessionID: operatorSessionID,
		Limit:             10,
	}
	requestJSON, _ := json.Marshal(request)

	response, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, response.Success)
	require.Len(t, response.Events, 1)
	assert.True(t, response.Events[0].StdoutTruncated)
	assert.Contains(t, response.Events[0].CommandStdout, "[TRUNCATED:")
}

func TestHistoryHandler_MultipleFileMutationsInHistory(t *testing.T) {
	hh, avs, _ := setupTestHistoryHandler(t)
	defer avs.Close()

	operatorSessionID := "test-multi-mutations"
	err := avs.CreateSession(operatorSessionID, "Multi Mutations", "user@test.com")
	require.NoError(t, err)

	// Create file mutation event with multiple files
	exitCode := 0
	event := &Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              EventTypeFileMutation,
		ContentText:       "Batch file update",
		CommandExitCode:   &exitCode,
	}
	eventID, err := avs.RecordEvent(event)
	require.NoError(t, err)

	// Record multiple file mutations
	files := []string{"/etc/config1.yml", "/etc/config2.yml", "/etc/config3.yml"}
	for _, f := range files {
		mutation := &FileMutationLog{
			EventID:   eventID,
			Filepath:  f,
			Operation: FileMutationWrite,
		}
		err = avs.RecordFileMutation(mutation)
		require.NoError(t, err)
	}

	// Fetch history
	request := fetchHistoryRequest{
		OperatorSessionID: operatorSessionID,
		Limit:             10,
	}
	requestJSON, _ := json.Marshal(request)

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
