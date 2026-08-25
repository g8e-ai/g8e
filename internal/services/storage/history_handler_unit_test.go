// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// mockAuditStore is a mock implementation of auditStoreInterface for unit testing
type mockAuditStore struct {
	getOperatorSessionFunc   func(sessionID string) (*OperatorSession, error)
	getEventsFunc            func(sessionID string, limit, offset int) ([]*Event, error)
	getFileMutationsFunc     func(eventID int64) ([]*FileMutationLog, error)
	getOperatorSessionCalled int
	getEventsCalled          int
	getFileMutationsCalled   int
	lastSessionID            string
	lastEventID              int64
}

func (m *mockAuditStore) GetOperatorSession(sessionID string) (*OperatorSession, error) {
	m.getOperatorSessionCalled++
	m.lastSessionID = sessionID
	if m.getOperatorSessionFunc != nil {
		return m.getOperatorSessionFunc(sessionID)
	}
	return nil, nil
}

func (m *mockAuditStore) GetEvents(sessionID string, limit, offset int) ([]*Event, error) {
	m.getEventsCalled++
	m.lastSessionID = sessionID
	if m.getEventsFunc != nil {
		return m.getEventsFunc(sessionID, limit, offset)
	}
	return nil, nil
}

func (m *mockAuditStore) GetFileMutations(eventID int64) ([]*FileMutationLog, error) {
	m.getFileMutationsCalled++
	m.lastEventID = eventID
	if m.getFileMutationsFunc != nil {
		return m.getFileMutationsFunc(eventID)
	}
	return nil, nil
}

// mockLedger is a mock implementation of ledgerInterface for unit testing
type mockLedger struct {
	getFileHistoryFunc         func(filePath string, limit int, sessionID string) ([]FileHistoryEntry, error)
	restoreFileFromCommitFunc  func(filePath, commitHash, sessionID string) error
	getFileAtCommitFunc        func(filePath, commitHash, sessionID string) (string, error)
	mirrorFileCreateFunc       func(operatorSessionID, filePath string) (*LedgerResult, error)
	completeMirrorCreateFunc   func(result *LedgerResult, operatorSessionID string) error
	ledgerFileWriteFunc        func(operatorSessionID, filePath string) (*LedgerResult, error)
	completeMirrorWriteFunc    func(result *LedgerResult, operatorSessionID string) error
	getFileHistoryCalled       int
	restoreFileCalled          int
	getFileAtCommitCalled      int
	mirrorFileCreateCalled     int
	completeMirrorCreateCalled int
	ledgerFileWriteCalled      int
	completeMirrorWriteCalled  int
	lastFilePath               string
	lastCommitHash             string
	lastSessionID              string
}

func (m *mockLedger) GetFileHistory(filePath string, limit int, sessionID string) ([]FileHistoryEntry, error) {
	m.getFileHistoryCalled++
	m.lastFilePath = filePath
	m.lastSessionID = sessionID
	if m.getFileHistoryFunc != nil {
		return m.getFileHistoryFunc(filePath, limit, sessionID)
	}
	return nil, nil
}

func (m *mockLedger) RestoreFileFromCommit(filePath, commitHash, sessionID string) error {
	m.restoreFileCalled++
	m.lastFilePath = filePath
	m.lastCommitHash = commitHash
	m.lastSessionID = sessionID
	if m.restoreFileFromCommitFunc != nil {
		return m.restoreFileFromCommitFunc(filePath, commitHash, sessionID)
	}
	return nil
}

func (m *mockLedger) GetFileAtCommit(filePath, commitHash, sessionID string) (string, error) {
	m.getFileAtCommitCalled++
	m.lastFilePath = filePath
	m.lastCommitHash = commitHash
	m.lastSessionID = sessionID
	if m.getFileAtCommitFunc != nil {
		return m.getFileAtCommitFunc(filePath, commitHash, sessionID)
	}
	return "", nil
}

func (m *mockLedger) MirrorFileCreate(operatorSessionID, filePath string) (*LedgerResult, error) {
	m.mirrorFileCreateCalled++
	m.lastSessionID = operatorSessionID
	m.lastFilePath = filePath
	if m.mirrorFileCreateFunc != nil {
		return m.mirrorFileCreateFunc(operatorSessionID, filePath)
	}
	return &LedgerResult{}, nil
}

func (m *mockLedger) CompleteMirrorCreate(result *LedgerResult, operatorSessionID string) error {
	m.completeMirrorCreateCalled++
	m.lastSessionID = operatorSessionID
	if m.completeMirrorCreateFunc != nil {
		return m.completeMirrorCreateFunc(result, operatorSessionID)
	}
	return nil
}

func (m *mockLedger) LedgerFileWrite(operatorSessionID, filePath string) (*LedgerResult, error) {
	m.ledgerFileWriteCalled++
	m.lastSessionID = operatorSessionID
	m.lastFilePath = filePath
	if m.ledgerFileWriteFunc != nil {
		return m.ledgerFileWriteFunc(operatorSessionID, filePath)
	}
	return &LedgerResult{}, nil
}

func (m *mockLedger) CompleteMirrorWrite(result *LedgerResult, operatorSessionID string) error {
	m.completeMirrorWriteCalled++
	m.lastSessionID = operatorSessionID
	if m.completeMirrorWriteFunc != nil {
		return m.completeMirrorWriteFunc(result, operatorSessionID)
	}
	return nil
}

// mockLogger is a mock implementation of loggerInterface for unit testing
type mockLogger struct {
	infoCalled int
	warnCalled int
	lastMsg    string
	lastArgs   []interface{}
}

func (m *mockLogger) Info(msg string, args ...interface{}) {
	m.infoCalled++
	m.lastMsg = msg
	m.lastArgs = args
}

func (m *mockLogger) Warn(msg string, args ...interface{}) {
	m.warnCalled++
	m.lastMsg = msg
	m.lastArgs = args
}

// TestNewHistoryHandler verifies constructor behavior
func TestNewHistoryHandler(t *testing.T) {
	tests := []struct {
		name       string
		auditStore *mockAuditStore
		ledger     *mockLedger
		logger     *mockLogger
		wantNil    bool
	}{
		{
			name:       "valid dependencies",
			auditStore: &mockAuditStore{},
			ledger:     &mockLedger{},
			logger:     &mockLogger{},
			wantNil:    false,
		},
		{
			name:       "nil audit store",
			auditStore: nil,
			ledger:     &mockLedger{},
			logger:     &mockLogger{},
			wantNil:    false,
		},
		{
			name:       "nil ledger",
			auditStore: &mockAuditStore{},
			ledger:     nil,
			logger:     &mockLogger{},
			wantNil:    false,
		},
		{
			name:       "nil logger",
			auditStore: &mockAuditStore{},
			ledger:     &mockLedger{},
			logger:     nil,
			wantNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hh := NewHistoryHandler(tt.auditStore, tt.ledger, tt.logger)

			if tt.wantNil {
				assert.Nil(t, hh)
			} else {
				assert.NotNil(t, hh)
				assert.Equal(t, tt.auditStore, hh.auditStore)
				assert.Equal(t, tt.ledger, hh.ledger)
				assert.Equal(t, tt.logger, hh.logger)
			}
		})
	}
}

// TestHandleFetchHistory_InvalidProtobuf verifies protobuf unmarshaling error handling
func TestHandleFetchHistory_InvalidProtobuf(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	invalidJSON := []byte("not a valid protobuf")

	result, err := hh.HandleFetchHistory(invalidJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "invalid request format")
}

// TestHandleFetchHistory_MissingSessionID verifies session ID validation
func TestHandleFetchHistory_MissingSessionID(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "operator_session_id is required")
}

// TestHandleFetchHistory_DefaultLimit verifies default limit is applied when limit <= 0
func TestHandleFetchHistory_DefaultLimit(t *testing.T) {
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return &OperatorSession{ID: sessionID, Title: "Test Session"}, nil
		},
		getEventsFunc: func(sessionID string, limit, offset int) ([]*Event, error) {
			assert.Equal(t, 50, limit, "should use default limit of 50")
			return []*Event{}, nil
		},
	}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             0,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Equal(t, int32(50), result.Limit)
}

// TestHandleFetchHistory_GetSessionError verifies error handling when session lookup fails
func TestHandleFetchHistory_GetSessionError(t *testing.T) {
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return nil, errors.New("database connection failed")
		},
	}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "failed to get session")
}

// TestHandleFetchHistory_GetEventsError verifies error handling when event lookup fails
func TestHandleFetchHistory_GetEventsError(t *testing.T) {
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return &OperatorSession{ID: sessionID}, nil
		},
		getEventsFunc: func(sessionID string, limit, offset int) ([]*Event, error) {
			return nil, errors.New("query failed")
		},
	}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "failed to get events")
}

// TestHandleFetchHistory_Success verifies successful history fetch with session and events
func TestHandleFetchHistory_Success(t *testing.T) {
	exitCode := 0
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return &OperatorSession{
				ID:           sessionID,
				Title:        "Test Session",
				CreatedAt:    time.Now().UTC(),
				UserIdentity: "user@test.com",
			}, nil
		},
		getEventsFunc: func(sessionID string, limit, offset int) ([]*Event, error) {
			return []*Event{
				{
					ID:                1,
					OperatorSessionID: sessionID,
					Timestamp:         time.Now().UTC(),
					Type:              constants.Event.Operator.Audit.Command,
					ContentText:       "test command",
					CommandRaw:        "echo test",
					CommandExitCode:   exitCode,
					CommandStdout:     "output",
					CommandStderr:     "",
					StoredLocally:     true,
					StdoutTruncated:   false,
					StderrTruncated:   false,
				},
			}, nil
		},
	}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Equal(t, "test-session", result.OperatorSessionId)
	assert.NotNil(t, result.WebSession)
	assert.Equal(t, "Test Session", result.WebSession.Title)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, int32(10), result.Limit)
	assert.Equal(t, int32(0), result.Offset)
	assert.Equal(t, int32(1), result.Total)
}

// TestHandleFetchHistory_NilExitCode verifies handling of nil exit code
func TestHandleFetchHistory_NilExitCode(t *testing.T) {
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return &OperatorSession{ID: sessionID}, nil
		},
		getEventsFunc: func(sessionID string, limit, offset int) ([]*Event, error) {
			return []*Event{
				{
					ID:                1,
					OperatorSessionID: sessionID,
					Timestamp:         time.Now().UTC(),
					Type:              constants.Event.Operator.Audit.Command,
					CommandExitCode:   constants.ExitCodeNone, // no exit code
				},
			}, nil
		},
	}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, int32(0), result.Events[0].CommandExitCode, "ExitCodeNone should default to 0 in protobuf")
}

// TestHandleFetchHistory_WithFileMutations verifies file mutation inclusion for file edit events
func TestHandleFetchHistory_WithFileMutations(t *testing.T) {
	exitCode := 0
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return &OperatorSession{ID: sessionID}, nil
		},
		getEventsFunc: func(sessionID string, limit, offset int) ([]*Event, error) {
			return []*Event{
				{
					ID:                1,
					OperatorSessionID: sessionID,
					Timestamp:         time.Now().UTC(),
					Type:              constants.Event.Operator.FileEdit.Completed,
					CommandExitCode:   exitCode,
				},
			}, nil
		},
		getFileMutationsFunc: func(eventID int64) ([]*FileMutationLog, error) {
			return []*FileMutationLog{
				{
					ID:               1,
					EventID:          eventID,
					Filepath:         "/etc/config.yml",
					Operation:        FileMutationWrite,
					LedgerHashBefore: "hash1",
					LedgerHashAfter:  "hash2",
					DiffStat:         "+10 lines",
				},
			}, nil
		},
	}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Len(t, result.Events, 1)
	assert.Len(t, result.Events[0].FileMutations, 1)
	assert.Equal(t, "/etc/config.yml", result.Events[0].FileMutations[0].Filepath)
	assert.Equal(t, "WRITE", result.Events[0].FileMutations[0].Operation)
}

// TestHandleFetchHistory_FileMutationError verifies graceful handling of file mutation lookup errors
func TestHandleFetchHistory_FileMutationError(t *testing.T) {
	exitCode := 0
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return &OperatorSession{ID: sessionID}, nil
		},
		getEventsFunc: func(sessionID string, limit, offset int) ([]*Event, error) {
			return []*Event{
				{
					ID:                1,
					OperatorSessionID: sessionID,
					Timestamp:         time.Now().UTC(),
					Type:              constants.Event.Operator.FileEdit.Completed,
					CommandExitCode:   exitCode,
				},
			}, nil
		},
		getFileMutationsFunc: func(eventID int64) ([]*FileMutationLog, error) {
			return nil, errors.New("mutation lookup failed")
		},
	}
	mockLogger := &mockLogger{}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, mockLogger)

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success, "should succeed despite mutation lookup error")
	assert.Len(t, result.Events, 1)
	assert.Len(t, result.Events[0].FileMutations, 0, "mutations should be empty on error")
	assert.Equal(t, 1, mockLogger.warnCalled, "should log warning on mutation error")
}

// TestHandleFetchHistory_NonFileEditEvent verifies mutations are not fetched for non-file-edit events
func TestHandleFetchHistory_NonFileEditEvent(t *testing.T) {
	exitCode := 0
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return &OperatorSession{ID: sessionID}, nil
		},
		getEventsFunc: func(sessionID string, limit, offset int) ([]*Event, error) {
			return []*Event{
				{
					ID:                1,
					OperatorSessionID: sessionID,
					Timestamp:         time.Now().UTC(),
					Type:              constants.Event.Operator.Audit.Command,
					CommandExitCode:   exitCode,
				},
			}, nil
		},
		getFileMutationsFunc: func(eventID int64) ([]*FileMutationLog, error) {
			t.Error("getFileMutations should not be called for non-file-edit events")
			return nil, nil
		},
	}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Len(t, result.Events, 1)
	assert.Equal(t, 0, mockAudit.getFileMutationsCalled)
}

// TestHandleFetchFileHistory_InvalidProtobuf verifies protobuf unmarshaling error handling
func TestHandleFetchFileHistory_InvalidProtobuf(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	invalidJSON := []byte("not a valid protobuf")

	result, err := hh.HandleFetchFileHistory(invalidJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "invalid request format")
}

// TestHandleFetchFileHistory_MissingFilePath verifies file path validation
func TestHandleFetchFileHistory_MissingFilePath(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchFileHistoryRequested{
		FilePath:          "",
		Limit:             10,
		OperatorSessionId: "test-session",
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchFileHistory(requestJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "file_path is required")
}

// TestHandleFetchFileHistory_DefaultLimit verifies default limit is applied when limit <= 0
func TestHandleFetchFileHistory_DefaultLimit(t *testing.T) {
	mockLedger := &mockLedger{
		getFileHistoryFunc: func(filePath string, limit int, sessionID string) ([]FileHistoryEntry, error) {
			assert.Equal(t, 50, limit, "should use default limit of 50")
			return []FileHistoryEntry{}, nil
		},
	}
	hh := NewHistoryHandler(&mockAuditStore{}, mockLedger, &mockLogger{})

	request := &operatorv1.FetchFileHistoryRequested{
		FilePath:          "/test/file.txt",
		Limit:             0,
		OperatorSessionId: "test-session",
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchFileHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
}

// TestHandleFetchFileHistory_GetHistoryError verifies error handling when ledger lookup fails
func TestHandleFetchFileHistory_GetHistoryError(t *testing.T) {
	mockLedger := &mockLedger{
		getFileHistoryFunc: func(filePath string, limit int, sessionID string) ([]FileHistoryEntry, error) {
			return nil, errors.New("git operation failed")
		},
	}
	hh := NewHistoryHandler(&mockAuditStore{}, mockLedger, &mockLogger{})

	request := &operatorv1.FetchFileHistoryRequested{
		FilePath:          "/test/file.txt",
		Limit:             10,
		OperatorSessionId: "test-session",
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchFileHistory(requestJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "failed to get file history")
}

// TestHandleFetchFileHistory_Success verifies successful file history fetch
func TestHandleFetchFileHistory_Success(t *testing.T) {
	mockLedger := &mockLedger{
		getFileHistoryFunc: func(filePath string, limit int, sessionID string) ([]FileHistoryEntry, error) {
			return []FileHistoryEntry{
				{
					CommitHash: "abc123",
					Timestamp:  time.Now().UTC(),
					Message:    "Initial commit",
					FilePath:   filePath,
				},
				{
					CommitHash: "def456",
					Timestamp:  time.Now().UTC(),
					Message:    "Update",
					FilePath:   filePath,
				},
			}, nil
		},
	}
	hh := NewHistoryHandler(&mockAuditStore{}, mockLedger, &mockLogger{})

	request := &operatorv1.FetchFileHistoryRequested{
		FilePath:          "/test/file.txt",
		Limit:             10,
		OperatorSessionId: "test-session",
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchFileHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Equal(t, "/test/file.txt", result.FilePath)
	assert.Len(t, result.History, 2)
	assert.Equal(t, "abc123", result.History[0].CommitHash)
	assert.Equal(t, "def456", result.History[1].CommitHash)
}

// TestHandleRestoreFile_InvalidProtobuf verifies protobuf unmarshaling error handling
func TestHandleRestoreFile_InvalidProtobuf(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	invalidJSON := []byte("not a valid protobuf")

	result, err := hh.HandleRestoreFile(invalidJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "invalid request format")
}

// TestHandleRestoreFile_MissingFilePath verifies file path validation
func TestHandleRestoreFile_MissingFilePath(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	request := &operatorv1.RestoreFileRequested{
		FilePath:          "",
		CommitHash:        "abc123",
		OperatorSessionId: "test-session",
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "file_path is required")
}

// TestHandleRestoreFile_MissingCommitHash verifies commit hash validation
func TestHandleRestoreFile_MissingCommitHash(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	request := &operatorv1.RestoreFileRequested{
		FilePath:          "/test/file.txt",
		CommitHash:        "",
		OperatorSessionId: "test-session",
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "commit_hash is required")
}

// TestHandleRestoreFile_MissingSessionID verifies session ID validation
func TestHandleRestoreFile_MissingSessionID(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	request := &operatorv1.RestoreFileRequested{
		FilePath:          "/test/file.txt",
		CommitHash:        "abc123",
		OperatorSessionId: "",
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "operator_session_id is required")
}

// TestHandleRestoreFile_RestoreError verifies error handling when restore fails
func TestHandleRestoreFile_RestoreError(t *testing.T) {
	mockLedger := &mockLedger{
		restoreFileFromCommitFunc: func(filePath, commitHash, sessionID string) error {
			return errors.New("git checkout failed")
		},
	}
	hh := NewHistoryHandler(&mockAuditStore{}, mockLedger, &mockLogger{})

	request := &operatorv1.RestoreFileRequested{
		FilePath:          "/test/file.txt",
		CommitHash:        "abc123",
		OperatorSessionId: "test-session",
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "failed to restore file")
}

// TestHandleRestoreFile_Success verifies successful file restore
func TestHandleRestoreFile_Success(t *testing.T) {
	mockLedger := &mockLedger{
		restoreFileFromCommitFunc: func(filePath, commitHash, sessionID string) error {
			return nil
		},
	}
	hh := NewHistoryHandler(&mockAuditStore{}, mockLedger, &mockLogger{})

	request := &operatorv1.RestoreFileRequested{
		FilePath:          "/test/file.txt",
		CommitHash:        "abc123",
		OperatorSessionId: "test-session",
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleRestoreFile(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Equal(t, "/test/file.txt", result.FilePath)
	assert.Equal(t, "abc123", result.CommitHash)
}

// TestGetFileAtCommit_Success verifies successful file content retrieval at commit
func TestGetFileAtCommit_Success(t *testing.T) {
	mockLedger := &mockLedger{
		getFileAtCommitFunc: func(filePath, commitHash, sessionID string) (string, error) {
			return "file content at commit", nil
		},
	}
	hh := NewHistoryHandler(&mockAuditStore{}, mockLedger, &mockLogger{})

	content, err := hh.GetFileAtCommit("/test/file.txt", "abc123", "test-session")
	require.NoError(t, err)

	assert.Equal(t, "file content at commit", content)
	assert.Equal(t, 1, mockLedger.getFileAtCommitCalled)
	assert.Equal(t, "/test/file.txt", mockLedger.lastFilePath)
	assert.Equal(t, "abc123", mockLedger.lastCommitHash)
	assert.Equal(t, "test-session", mockLedger.lastSessionID)
}

// TestGetFileAtCommit_Error verifies error handling when file retrieval fails
func TestGetFileAtCommit_Error(t *testing.T) {
	mockLedger := &mockLedger{
		getFileAtCommitFunc: func(filePath, commitHash, sessionID string) (string, error) {
			return "", errors.New("git show failed")
		},
	}
	hh := NewHistoryHandler(&mockAuditStore{}, mockLedger, &mockLogger{})

	content, err := hh.GetFileAtCommit("/test/file.txt", "abc123", "test-session")
	require.Error(t, err)

	assert.Empty(t, content)
	assert.Contains(t, err.Error(), "git show failed")
}

// TestGetFileAtCommit_LedgerError verifies error handling when ledger returns an error
func TestGetFileAtCommit_LedgerError(t *testing.T) {
	mockLedger := &mockLedger{
		getFileAtCommitFunc: func(filePath, commitHash, sessionID string) (string, error) {
			return "", errors.New("ledger error")
		},
	}
	hh := NewHistoryHandler(&mockAuditStore{}, mockLedger, &mockLogger{})

	content, err := hh.GetFileAtCommit("/test/file.txt", "abc123", "test-session")
	require.Error(t, err)

	assert.Empty(t, content)
	assert.Contains(t, err.Error(), "ledger error")
}

// TestFetchHistoryError verifies error response construction
func TestFetchHistoryError(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	result := hh.fetchHistoryError("test error message")

	assert.False(t, result.Success)
	assert.Equal(t, "test error message", result.Error)
}

// TestFetchFileHistoryError verifies error response construction
func TestFetchFileHistoryError(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	result := hh.fetchFileHistoryError("test error message")

	assert.False(t, result.Success)
	assert.Equal(t, "test error message", result.Error)
}

// TestRestoreFileError verifies error response construction
func TestRestoreFileError(t *testing.T) {
	hh := NewHistoryHandler(&mockAuditStore{}, &mockLedger{}, &mockLogger{})

	result := hh.restoreFileError("test error message")

	assert.False(t, result.Success)
	assert.Equal(t, "test error message", result.Error)
}

// TestHandleFetchHistory_NilSession verifies handling when session is nil
func TestHandleFetchHistory_NilSession(t *testing.T) {
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return nil, nil // session not found
		},
		getEventsFunc: func(sessionID string, limit, offset int) ([]*Event, error) {
			return []*Event{}, nil
		},
	}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Nil(t, result.WebSession, "WebSession should be nil when session not found")
}

// TestHandleFetchHistory_EmptyEvents verifies handling when no events exist
func TestHandleFetchHistory_EmptyEvents(t *testing.T) {
	mockAudit := &mockAuditStore{
		getOperatorSessionFunc: func(sessionID string) (*OperatorSession, error) {
			return &OperatorSession{ID: sessionID}, nil
		},
		getEventsFunc: func(sessionID string, limit, offset int) ([]*Event, error) {
			return []*Event{}, nil
		},
	}
	hh := NewHistoryHandler(mockAudit, &mockLedger{}, &mockLogger{})

	request := &operatorv1.FetchHistoryRequested{
		OperatorSessionId: "test-session",
		Limit:             10,
		Offset:            0,
	}
	requestJSON, err := proto.Marshal(request)
	require.NoError(t, err)

	result, err := hh.HandleFetchHistory(requestJSON)
	require.NoError(t, err)

	assert.True(t, result.Success)
	assert.Len(t, result.Events, 0)
	assert.Equal(t, int32(0), result.Total)
}
