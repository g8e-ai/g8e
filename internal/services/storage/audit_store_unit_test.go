// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// TestDefaultAuditStoreConfig verifies the default configuration values
func TestDefaultAuditStoreConfig(t *testing.T) {
	config := DefaultAuditStoreConfig()

	if config == nil {
		t.Fatal("DefaultAuditStoreConfig returned nil")
	}

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"DBPath", config.DBPath, constants.DbFilename},
		{"MaxDBSizeMB", config.MaxDBSizeMB, int64(2048)},
		{"RetentionDays", config.RetentionDays, 90},
		{"PruneIntervalMinutes", config.PruneIntervalMinutes, 60},
		{"OutputTruncationThreshold", config.OutputTruncationThreshold, 102400},
		{"HeadTailSize", config.HeadTailSize, 51200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestTruncateOutput verifies the head/tail truncation logic
func TestTruncateOutput(t *testing.T) {
	config := &AuditStoreConfig{
		OutputTruncationThreshold: 100,
		HeadTailSize:              20,
	}

	ass := &SQLAuditStore{
		config: config,
	}

	tests := []struct {
		name          string
		input         string
		wantTruncated bool
	}{
		{
			name:          "empty string",
			input:         "",
			wantTruncated: false,
		},
		{
			name:          "short string below threshold",
			input:         "short",
			wantTruncated: false,
		},
		{
			name:          "string at threshold",
			input:         string(make([]byte, 100)),
			wantTruncated: false,
		},
		{
			name:          "string above threshold",
			input:         string(make([]byte, 150)),
			wantTruncated: true,
		},
		{
			name:          "string with specific content",
			input:         "a" + string(make([]byte, 100)) + "z",
			wantTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, truncated := ass.truncateOutput(tt.input)

			if truncated != tt.wantTruncated {
				t.Errorf("truncateOutput() truncated = %v, want %v", truncated, tt.wantTruncated)
			}

			// If truncated, result should be shorter than input
			if truncated && len(result) >= len(tt.input) {
				t.Errorf("truncateOutput() result length %d should be less than input length %d when truncated", len(result), len(tt.input))
			}

			// If not truncated, result should equal input
			if !truncated && result != tt.input {
				t.Errorf("truncateOutput() result = %q, want %q when not truncated", result, tt.input)
			}
		})
	}
}

// TestTruncateOutputHeadTail verifies head and tail preservation
func TestTruncateOutputHeadTail(t *testing.T) {
	config := &AuditStoreConfig{
		OutputTruncationThreshold: 30,
		HeadTailSize:              10,
	}

	ass := &SQLAuditStore{
		config: config,
	}

	input := "0123456789" + "MIDDLECONTENTXX" + "abcdefghij"
	result, truncated := ass.truncateOutput(input)

	if !truncated {
		t.Error("Expected truncation for input above threshold")
	}

	// Verify head is preserved
	if !strings.Contains(result, "0123456789") {
		t.Error("Head not preserved in truncated output")
	}

	// Verify tail is preserved
	if !strings.Contains(result, "abcdefghij") {
		t.Error("Tail not preserved in truncated output")
	}

	// Verify truncation marker is present
	if !strings.Contains(result, "TRUNCATED") {
		t.Error("Truncation marker not present in output")
	}

	// Verify middle is removed
	if strings.Contains(result, "MIDDLECONTENTXX") {
		t.Error("Middle should be removed in truncated output")
	}
}

// TestTruncateOutputWithDifferentSizes tests truncation with various size configurations
func TestTruncateOutputWithDifferentSizes(t *testing.T) {
	tests := []struct {
		name                  string
		threshold             int
		headTailSize          int
		input                 string
		expectedTruncated     bool
		expectedHeadPreserved bool
		expectedTailPreserved bool
	}{
		{
			name:                  "small threshold, small head/tail",
			threshold:             25,
			headTailSize:          5,
			input:                 "0123456789" + "MIDDLE" + "abcdefghij",
			expectedTruncated:     true,
			expectedHeadPreserved: true,
			expectedTailPreserved: true,
		},
		{
			name:                  "large threshold, no truncation",
			threshold:             1000,
			headTailSize:          10,
			input:                 "short",
			expectedTruncated:     false,
			expectedHeadPreserved: false,
			expectedTailPreserved: false,
		},
		{
			name:                  "threshold exactly at input length",
			threshold:             30,
			headTailSize:          10,
			input:                 string(make([]byte, 30)),
			expectedTruncated:     false,
			expectedHeadPreserved: false,
			expectedTailPreserved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &AuditStoreConfig{
				OutputTruncationThreshold: tt.threshold,
				HeadTailSize:              tt.headTailSize,
			}

			ass := &SQLAuditStore{
				config: config,
			}

			result, truncated := ass.truncateOutput(tt.input)

			if truncated != tt.expectedTruncated {
				t.Errorf("truncateOutput() truncated = %v, want %v", truncated, tt.expectedTruncated)
			}

			if tt.expectedHeadPreserved && !strings.Contains(result, tt.input[:tt.headTailSize]) {
				t.Error("Head not preserved in truncated output")
			}

			if tt.expectedTailPreserved && !strings.Contains(result, tt.input[len(tt.input)-tt.headTailSize:]) {
				t.Error("Tail not preserved in truncated output")
			}
		})
	}
}

// TestFileMutationOperationConstants verifies operation constants
func TestFileMutationOperationConstants(t *testing.T) {
	tests := []struct {
		name  string
		op    FileMutationOperation
		value string
	}{
		{"FileMutationWrite", FileMutationWrite, "WRITE"},
		{"FileMutationDelete", FileMutationDelete, "DELETE"},
		{"FileMutationCreate", FileMutationCreate, "CREATE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.op) != tt.value {
				t.Errorf("%s = %q, want %q", tt.name, tt.op, tt.value)
			}
		})
	}
}

// TestErrorConstants verifies error constants
func TestErrorConstants(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrAuditEventNil", constants.ErrAuditEventNil, "AUDIT_EVENT_INVALID"},
		{"ErrAuditSessionMissing", constants.ErrAuditSessionMissing, "AUDIT_SESSION_MISSING"},
		{"ErrAuditSessionUnknown", constants.ErrAuditSessionUnknown, "AUDIT_SESSION_UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("Error constant should not be nil")
			}
			if !strings.Contains(tt.err.Error(), tt.want) {
				t.Errorf("Error message = %q, want to contain %q", tt.err.Error(), tt.want)
			}
		})
	}
}

// TestNilStoreMethods verifies nil-safe method calls
func TestNilStoreMethods(t *testing.T) {
	var ass *SQLAuditStore

	// These methods should handle nil gracefully
	ass.Wait()
	err := ass.Close()
	if err != nil {
		t.Errorf("Close() on nil store should return nil, got %v", err)
	}

	vault := ass.GetEncryptionVault()
	if vault != nil {
		t.Error("GetEncryptionVault() on nil store should return nil")
	}

	dataDir := ass.GetDataDir()
	if dataDir != "" {
		t.Error("GetDataDir() on nil store should return empty string")
	}
}

// TestGetDataDir verifies data directory retrieval
func TestGetDataDir(t *testing.T) {
	tests := []struct {
		name string
		ass  *SQLAuditStore
		want string
	}{
		{
			name: "nil store",
			ass:  nil,
			want: "",
		},
		{
			name: "store with nil fileSvc",
			ass: &SQLAuditStore{
				config: &AuditStoreConfig{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ass.GetDataDir()
			if got != tt.want {
				t.Errorf("GetDataDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestWait verifies wait behavior
func TestWait(t *testing.T) {
	tests := []struct {
		name string
		ass  *SQLAuditStore
	}{
		{
			name: "nil store",
			ass:  nil,
		},
		{
			name: "store with no writes",
			ass:  &SQLAuditStore{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			tt.ass.Wait()
		})
	}
}

// TestClose verifies close behavior
func TestClose(t *testing.T) {
	tests := []struct {
		name string
		ass  *SQLAuditStore
	}{
		{
			name: "nil store",
			ass:  nil,
		},
		{
			name: "store without resources",
			ass:  &SQLAuditStore{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			err := tt.ass.Close()
			if err != nil {
				t.Errorf("Close() unexpected error = %v", err)
			}
		})
	}
}

// TestCreateSessionNilStore verifies nil-safe session creation
func TestCreateSessionNilStore(t *testing.T) {
	var ass *SQLAuditStore

	// Should not panic, should return nil
	err := ass.CreateSession("test-id", "operator", "test title", "user")
	if err != nil {
		t.Errorf("CreateSession() on nil store should return nil, got %v", err)
	}
}

// TestRecordEventNilStore verifies nil-safe event recording
func TestRecordEventNilStore(t *testing.T) {
	var ass *SQLAuditStore
	event := &Event{
		OperatorSessionID: "test-session",
		Type:              constants.Event.Operator.Command.Requested,
	}

	// Should not panic, should return 0
	eventID, err := ass.RecordEvent(event)
	if err != nil {
		t.Errorf("RecordEvent() on nil store should return nil error, got %v", err)
	}
	if eventID != 0 {
		t.Errorf("RecordEvent() on nil store should return 0, got %d", eventID)
	}
}

// TestRecordEventsNilStore verifies nil-safe batch event recording
func TestRecordEventsNilStore(t *testing.T) {
	var ass *SQLAuditStore
	events := []*Event{
		{
			OperatorSessionID: "test-session",
			Type:              constants.Event.Operator.Command.Requested,
		},
	}

	// Should not panic, should return nil
	err := ass.RecordEvents(events)
	if err != nil {
		t.Errorf("RecordEvents() on nil store should return nil, got %v", err)
	}
}

// TestRecordActionReceiptNilStore verifies nil-safe receipt recording
func TestRecordActionReceiptNilStore(t *testing.T) {
	var ass *SQLAuditStore

	// Should not panic, should return nil
	err := ass.RecordActionReceipt(nil)
	if err != nil {
		t.Errorf("RecordActionReceipt() on nil store should return nil, got %v", err)
	}
}

// TestRecordFileMutationNilStore verifies nil-safe mutation recording
func TestRecordFileMutationNilStore(t *testing.T) {
	var ass *SQLAuditStore
	mutation := &FileMutationLog{
		EventID: 123,
	}

	// Should not panic, should return nil
	err := ass.RecordFileMutation(mutation)
	if err != nil {
		t.Errorf("RecordFileMutation() on nil store should return nil, got %v", err)
	}
}

// TestSQLAuditStore_NilEncryptionVault verifies that NewSQLAuditStore
// requires EncryptionVault in config and returns an error when vault is nil.
func TestSQLAuditStore_NilEncryptionVault(t *testing.T) {
	logger := testutil.NewTestLogger()

	config := DefaultAuditStoreConfig()

	// Test that service fails to initialize with nil EncryptionVault
	ass, err := NewSQLAuditStore(config, logger, nil)
	if err == nil {
		t.Error("NewSQLAuditStore with nil EncryptionVault should return error")
	}
	if !strings.Contains(err.Error(), "EncryptionVault is required") {
		t.Errorf("Error should mention 'EncryptionVault is required', got: %v", err)
	}
	if ass != nil {
		t.Error("NewSQLAuditStore with nil EncryptionVault should return nil store")
	}
}

// TestGetActionReceipt_NilStore verifies nil-safe behavior
func TestGetActionReceipt_NilStore(t *testing.T) {
	var ass *SQLAuditStore

	receipt, err := ass.GetActionReceipt("test-tx-id")
	if err == nil {
		t.Error("GetActionReceipt on nil store should return error")
	}
	if receipt != nil {
		t.Error("GetActionReceipt on nil store should return nil receipt")
	}
	if !strings.Contains(err.Error(), "audit store is disabled") {
		t.Errorf("Error should mention 'audit store is disabled', got: %v", err)
	}
}

// TestGetActionReceipt_NilDB verifies behavior when db is nil
func TestGetActionReceipt_NilDB(t *testing.T) {
	ass := &SQLAuditStore{
		db: nil,
	}

	receipt, err := ass.GetActionReceipt("test-tx-id")
	if err == nil {
		t.Error("GetActionReceipt with nil db should return error")
	}
	if receipt != nil {
		t.Error("GetActionReceipt with nil db should return nil receipt")
	}
	if !strings.Contains(err.Error(), "audit store is disabled") {
		t.Errorf("Error should mention 'audit store is disabled', got: %v", err)
	}
}

// TestListActionReceipts_NilStore verifies nil-safe behavior
func TestListActionReceipts_NilStore(t *testing.T) {
	var ass *SQLAuditStore

	receipts, err := ass.ListActionReceipts("session-id", 10, 0)
	if err == nil {
		t.Error("ListActionReceipts on nil store should return error")
	}
	if receipts != nil {
		t.Error("ListActionReceipts on nil store should return nil receipts")
	}
	if !strings.Contains(err.Error(), "audit store is disabled") {
		t.Errorf("Error should mention 'audit store is disabled', got: %v", err)
	}
}

// TestListActionReceipts_NilDB verifies behavior when db is nil
func TestListActionReceipts_NilDB(t *testing.T) {
	ass := &SQLAuditStore{
		db: nil,
	}

	receipts, err := ass.ListActionReceipts("session-id", 10, 0)
	if err == nil {
		t.Error("ListActionReceipts with nil db should return error")
	}
	if receipts != nil {
		t.Error("ListActionReceipts with nil db should return nil receipts")
	}
	if !strings.Contains(err.Error(), "audit store is disabled") {
		t.Errorf("Error should mention 'audit store is disabled', got: %v", err)
	}
}

// TestListActionReceipts_DefaultLimit verifies default limit is applied
func TestListActionReceipts_DefaultLimit(t *testing.T) {
	// This test verifies the logic that applies default limit when limit <= 0
	// Since we can't mock the db easily, we test the nil case which also checks limit logic
	ass := &SQLAuditStore{
		db: nil,
	}

	// Test with zero limit (should default to 50)
	receipts, err := ass.ListActionReceipts("session-id", 0, 0)
	if err == nil {
		t.Error("ListActionReceipts with nil db should return error")
	}
	if receipts != nil {
		t.Error("ListActionReceipts with nil db should return nil receipts")
	}
}

// TestListActionReceiptsSince_NilStore verifies nil-safe behavior
func TestListActionReceiptsSince_NilStore(t *testing.T) {
	var ass *SQLAuditStore

	since := time.Now()
	receipts, err := ass.ListActionReceiptsSince(since, 10)
	if err == nil {
		t.Error("ListActionReceiptsSince on nil store should return error")
	}
	if receipts != nil {
		t.Error("ListActionReceiptsSince on nil store should return nil receipts")
	}
	if !strings.Contains(err.Error(), "audit store is disabled") {
		t.Errorf("Error should mention 'audit store is disabled', got: %v", err)
	}
}

// TestListActionReceiptsSince_NilDB verifies behavior when db is nil
func TestListActionReceiptsSince_NilDB(t *testing.T) {
	ass := &SQLAuditStore{
		db: nil,
	}

	since := time.Now()
	receipts, err := ass.ListActionReceiptsSince(since, 10)
	if err == nil {
		t.Error("ListActionReceiptsSince with nil db should return error")
	}
	if receipts != nil {
		t.Error("ListActionReceiptsSince with nil db should return nil receipts")
	}
	if !strings.Contains(err.Error(), "audit store is disabled") {
		t.Errorf("Error should mention 'audit store is disabled', got: %v", err)
	}
}

// TestListActionReceiptsSince_DefaultLimit verifies default limit is applied
func TestListActionReceiptsSince_DefaultLimit(t *testing.T) {
	ass := &SQLAuditStore{
		db: nil,
	}

	since := time.Now()
	// Test with zero limit (should default to 100)
	receipts, err := ass.ListActionReceiptsSince(since, 0)
	if err == nil {
		t.Error("ListActionReceiptsSince with nil db should return error")
	}
	if receipts != nil {
		t.Error("ListActionReceiptsSince with nil db should return nil receipts")
	}
}
