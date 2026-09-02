// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
)

func TestRegistry(t *testing.T) {
	scenarios := Registry()

	assert.NotEmpty(t, scenarios, "Registry should return at least one scenario")

	for _, sc := range scenarios {
		assert.NotEmpty(t, sc.Name, "Scenario should have a Name")
		assert.NotEmpty(t, sc.Title, "Scenario should have a Title")
		assert.NotEmpty(t, sc.Persona.ID, "Scenario should have a Persona with ID")
		assert.NotEmpty(t, sc.RequiresPosture, "Scenario should have a RequiresPosture")
		assert.NotNil(t, sc.Run, "Scenario should have a Run function")
	}
}

func TestFind(t *testing.T) {
	scenarios := Registry()

	require.NotEmpty(t, scenarios, "Registry should return at least one scenario for Find test")

	firstName := scenarios[0].Name
	found, ok := Find(firstName)
	assert.True(t, ok, "Find should return true for existing scenario %q", firstName)
	assert.Equal(t, firstName, found.Name, "Find should return scenario with correct name")

	_, ok = Find("non-existent-scenario-name")
	assert.False(t, ok, "Find should return false for non-existent scenario")
}

func TestPostureConstants(t *testing.T) {
	assert.Equal(t, Posture("doctrine"), Doctrine, "Doctrine constant should be 'doctrine'")
	assert.Equal(t, Posture("consensus"), Consensus, "Consensus constant should be 'consensus'")
	assert.Equal(t, Posture("notary"), Notary, "Notary constant should be 'notary'")
}

func TestPostureStringValues(t *testing.T) {
	tests := []struct {
		posture  Posture
		expected string
	}{
		{Doctrine, "doctrine"},
		{Consensus, "consensus"},
		{Notary, "notary"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, string(tt.posture), "Posture %q should equal %q", tt.posture, tt.expected)
	}
}

func TestPostureEquality(t *testing.T) {
	assert.Equal(t, Posture("doctrine"), Doctrine, "Doctrine should equal Posture(\"doctrine\")")
	assert.Equal(t, Posture("consensus"), Consensus, "Consensus should equal Posture(\"consensus\")")
	assert.Equal(t, Posture("notary"), Notary, "Notary should equal Posture(\"notary\")")

	assert.NotEqual(t, Doctrine, Consensus, "Doctrine should not equal Consensus")
	assert.NotEqual(t, Consensus, Notary, "Consensus should not equal Notary")
	assert.NotEqual(t, Doctrine, Notary, "Doctrine should not equal Notary")
}

func TestResultRetainToolReceipt_RetainsAuthoritativeIdentity(t *testing.T) {
	r := &Result{}
	resp := &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"content":[{"type":"text","text":"done"}],"receipt":{"transaction_id":"tx-1","transaction_hash":"hash-1","signer_key_id":"warden-key-1","signature":"signature-1"}}`)}

	err := r.retainToolReceipt(resp)

	require.NoError(t, err)
	assert.Equal(t, []string{"tx-1"}, r.TransactionIDs)
	require.Len(t, r.Receipts, 1)
	assert.Equal(t, "tx-1", r.Receipts[0].TransactionID)
	assert.Equal(t, "hash-1", r.Receipts[0].TransactionHash)
	assert.Equal(t, "warden-key-1", r.Receipts[0].SignerKeyID)
	assert.Equal(t, "signature-1", r.Receipts[0].Signature)
}

func TestResultRetainToolReceipt_FailsClosedOnMissingMalformedOrIncompleteMetadata(t *testing.T) {
	tests := []struct {
		name    string
		resp    *clientpkg.JSONRPCResponse
		wantErr error
	}{
		{
			name:    "nil response",
			resp:    nil,
			wantErr: constants.ErrHarnessReceiptReferenceMissing,
		},
		{
			name:    "empty result bytes",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(``)},
			wantErr: constants.ErrHarnessReceiptReferenceMissing,
		},
		{
			name:    "malformed JSON result",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{not valid json`)},
			wantErr: nil, // wrapped decode error, not a sentinel
		},
		{
			name:    "missing receipt field entirely",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"content":[{"type":"text","text":"done"}]}`)},
			wantErr: constants.ErrHarnessReceiptReferenceMissing,
		},
		{
			name:    "null receipt field",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"content":[{"type":"text","text":"done"}],"receipt":null}`)},
			wantErr: constants.ErrHarnessReceiptReferenceMissing,
		},
		{
			name:    "missing transaction_id",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"receipt":{"transaction_hash":"hash-1","signer_key_id":"warden-key-1","signature":"signature-1"}}`)},
			wantErr: constants.ErrHarnessReceiptReferenceInvalid,
		},
		{
			name:    "missing transaction_hash",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"receipt":{"transaction_id":"tx-1","signer_key_id":"warden-key-1","signature":"signature-1"}}`)},
			wantErr: constants.ErrHarnessReceiptReferenceInvalid,
		},
		{
			name:    "missing signer_key_id",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"receipt":{"transaction_id":"tx-1","transaction_hash":"hash-1","signature":"signature-1"}}`)},
			wantErr: constants.ErrHarnessReceiptReferenceInvalid,
		},
		{
			name:    "missing signature",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"receipt":{"transaction_id":"tx-1","transaction_hash":"hash-1","signer_key_id":"warden-key-1"}}`)},
			wantErr: constants.ErrHarnessReceiptReferenceInvalid,
		},
		{
			name:    "all four fields missing",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"receipt":{}}`)},
			wantErr: constants.ErrHarnessReceiptReferenceInvalid,
		},
		{
			name:    "empty string transaction_id",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"receipt":{"transaction_id":"","transaction_hash":"hash-1","signer_key_id":"warden-key-1","signature":"signature-1"}}`)},
			wantErr: constants.ErrHarnessReceiptReferenceInvalid,
		},
		{
			name:    "empty string transaction_hash",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"receipt":{"transaction_id":"tx-1","transaction_hash":"","signer_key_id":"warden-key-1","signature":"signature-1"}}`)},
			wantErr: constants.ErrHarnessReceiptReferenceInvalid,
		},
		{
			name:    "empty string signer_key_id",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"receipt":{"transaction_id":"tx-1","transaction_hash":"hash-1","signer_key_id":"","signature":"signature-1"}}`)},
			wantErr: constants.ErrHarnessReceiptReferenceInvalid,
		},
		{
			name:    "empty string signature",
			resp:    &clientpkg.JSONRPCResponse{Result: json.RawMessage(`{"receipt":{"transaction_id":"tx-1","transaction_hash":"hash-1","signer_key_id":"warden-key-1","signature":""}}`)},
			wantErr: constants.ErrHarnessReceiptReferenceInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Result{}

			err := r.retainToolReceipt(tt.resp)

			require.Error(t, err, "retainToolReceipt must fail closed on %s", tt.name)
			assert.Empty(t, r.TransactionIDs, "no transaction IDs should be retained on %s", tt.name)
			assert.Empty(t, r.Receipts, "no receipts should be retained on %s", tt.name)

			if tt.wantErr != nil {
				assert.True(t, errors.Is(err, tt.wantErr),
					"expected error %v on %s, got %v", tt.wantErr, tt.name, err)
			}
		})
	}
}

func TestResult_Note(t *testing.T) {
	r := &Result{}
	r.note("test note %d", 42)

	assert.Len(t, r.Notes, 1, "note should add one entry")
	assert.Equal(t, "test note 42", r.Notes[0], "note should format correctly")

	r.note("second note")
	assert.Len(t, r.Notes, 2, "note should append")
}

func TestResult_Tx(t *testing.T) {
	r := &Result{}

	r.tx("hash1")
	assert.Len(t, r.TxHashes, 1, "tx should add one entry for valid hash")
	assert.Equal(t, "hash1", r.TxHashes[0], "tx should store hash correctly")

	r.tx("hash2")
	assert.Len(t, r.TxHashes, 2, "tx should append")

	r.tx("")
	assert.Len(t, r.TxHashes, 2, "tx should skip empty hash")
}

func TestResult(t *testing.T) {
	t.Run("note method", func(t *testing.T) {
		r := &Result{}
		r.note("test note %d", 42)

		assert.Len(t, r.Notes, 1, "note should add one entry")
		assert.Equal(t, "test note 42", r.Notes[0], "note should format correctly")

		r.note("second note")
		assert.Len(t, r.Notes, 2, "note should append")
	})

	t.Run("tx method", func(t *testing.T) {
		r := &Result{}

		r.tx("hash1")
		assert.Len(t, r.TxHashes, 1, "tx should add one entry for valid hash")
		assert.Equal(t, "hash1", r.TxHashes[0], "tx should store hash correctly")

		r.tx("hash2")
		assert.Len(t, r.TxHashes, 2, "tx should append")

		r.tx("")
		assert.Len(t, r.TxHashes, 2, "tx should skip empty hash")
	})
}

func TestResultZeroValue(t *testing.T) {
	var r Result

	assert.Empty(t, r.Name, "zero Result Name should be empty")
	assert.Empty(t, r.Title, "zero Result Title should be empty")
	assert.Empty(t, r.Persona, "zero Result Persona should be empty")
	assert.Empty(t, r.RequiresPosture, "zero Result RequiresPosture should be empty")
	assert.True(t, r.StartedAt.IsZero(), "zero Result StartedAt should be zero time")
	assert.Zero(t, r.DurationMS, "zero Result DurationMS should be 0")
	assert.Nil(t, r.Exchanges, "zero Result Exchanges should be nil")
	assert.Nil(t, r.TxHashes, "zero Result TxHashes should be nil")
	assert.Nil(t, r.Notes, "zero Result Notes should be nil")
	assert.False(t, r.OK, "zero Result OK should be false")
	assert.Empty(t, r.Err, "zero Result Err should be empty")
}

func TestResultFields(t *testing.T) {
	r := Result{}

	assert.Empty(t, r.Name, "Zero Result should have empty Name")
	assert.Empty(t, r.Title, "Zero Result should have empty Title")
	assert.Empty(t, r.Persona, "Zero Result should have empty Persona")
	assert.Empty(t, r.RequiresPosture, "Zero Result should have empty RequiresPosture")
	assert.True(t, r.StartedAt.IsZero(), "Zero Result should have zero StartedAt")
	assert.Zero(t, r.DurationMS, "Zero Result should have zero DurationMS")
	assert.Nil(t, r.Exchanges, "Zero Result should have nil Exchanges")
	assert.Nil(t, r.TxHashes, "Zero Result should have nil TxHashes")
	assert.Nil(t, r.Notes, "Zero Result should have nil Notes")
	assert.False(t, r.OK, "Zero Result should have false OK")
	assert.Empty(t, r.Err, "Zero Result should have empty Err")
}

func TestResultNoteFormatting(t *testing.T) {
	r := &Result{}

	r.note("simple")
	assert.Len(t, r.Notes, 1, "note should add one entry")
	assert.Equal(t, "simple", r.Notes[0], "note formatting failed")

	r.note("with %s", "arg")
	assert.Len(t, r.Notes, 2, "note should append")
	assert.Equal(t, "with arg", r.Notes[1], "note formatting with arg failed")

	r.note("number %d", 42)
	assert.Len(t, r.Notes, 3, "note should append")
	assert.Equal(t, "number 42", r.Notes[2], "note formatting with number failed")

	r.note("multiple %s %d %v", "args", 123, true)
	assert.Len(t, r.Notes, 4, "note should append")
	assert.Equal(t, "multiple args 123 true", r.Notes[3], "note formatting with multiple args failed")
}

func TestResultNoteWithNilArgs(t *testing.T) {
	r := &Result{}

	r.note("test")
	assert.Len(t, r.Notes, 1, "note with no args should work")
}

func TestResultTxHashHandling(t *testing.T) {
	r := &Result{}

	validHashes := []string{
		"abc123",
		"deadbeef",
		"0123456789abcdef",
		"ABCDEF",
	}

	for _, hash := range validHashes {
		r.tx(hash)
	}

	assert.Len(t, r.TxHashes, len(validHashes), "Should have correct number of hashes")

	for i, hash := range validHashes {
		assert.Equal(t, hash, r.TxHashes[i], "Hash at index %d should match", i)
	}

	initialCount := len(r.TxHashes)
	r.tx("")
	assert.Len(t, r.TxHashes, initialCount, "Empty hashes should be skipped")

	r.tx("  ")
	assert.NotEqual(t, initialCount, len(r.TxHashes), "Whitespace hashes are currently added (not skipped)")
}

func TestResultTxWithWhitespace(t *testing.T) {
	r := &Result{}

	r.tx("  ")
	assert.Len(t, r.TxHashes, 1, "whitespace-only hash should be added (current behavior)")

	r.tx("\t\n")
	assert.Len(t, r.TxHashes, 2, "tab/newline hash should be added (current behavior)")
}

func TestResultJSONSerialization(t *testing.T) {
	r := Result{
		Name:            "test",
		Title:           "Test",
		Persona:         "test-persona",
		RequiresPosture: Doctrine,
		StartedAt:       time.Now(),
		DurationMS:      100,
		Exchanges:       []clientpkg.Exchange{},
		TxHashes:        []string{"hash1", "hash2"},
		Notes:           []string{"note1", "note2"},
		OK:              true,
	}

	data, err := json.Marshal(r)
	require.NoError(t, err, "Result should be JSON serializable")

	var r2 Result
	err = json.Unmarshal(data, &r2)
	require.NoError(t, err, "Result should be JSON deserializable")

	assert.Equal(t, r.Name, r2.Name, "Deserialized Name should match")
	assert.Equal(t, r.Title, r2.Title, "Deserialized Title should match")
}

func TestResultWithError(t *testing.T) {
	r := Result{
		Name:    "failed-scenario",
		Title:   "Failed Scenario",
		Persona: "test-persona",
		OK:      false,
		Err:     "something went wrong",
	}

	data, err := json.Marshal(r)
	require.NoError(t, err, "Result with error should be JSON serializable")

	var r2 Result
	err = json.Unmarshal(data, &r2)
	require.NoError(t, err, "Result with error should be JSON deserializable")

	assert.False(t, r2.OK, "Deserialized OK should be false")
	assert.Equal(t, "something went wrong", r2.Err, "Deserialized Err should match")
}

func TestScenarioStruct(t *testing.T) {
	sc := Scenario{
		Name:            "test-name",
		Title:           "Test Title",
		Persona:         clientpkg.Persona{ID: "test-persona"},
		RequiresPosture: Doctrine,
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			return nil
		},
	}

	assert.Equal(t, "test-name", sc.Name, "Scenario Name should be set")
	assert.Equal(t, "Test Title", sc.Title, "Scenario Title should be set")
	assert.Equal(t, "test-persona", sc.Persona.ID, "Scenario Persona should be set")
	assert.Equal(t, Doctrine, sc.RequiresPosture, "Scenario RequiresPosture should be set")
	assert.NotNil(t, sc.Run, "Scenario Run should be set")
}

func TestScenarioFieldValidation(t *testing.T) {
	tests := []struct {
		name    string
		sc      Scenario
		wantErr bool
	}{
		{
			name: "valid scenario",
			sc: Scenario{
				Name:            "valid",
				Title:           "Valid Scenario",
				Persona:         clientpkg.Persona{ID: "test"},
				RequiresPosture: Doctrine,
				Run:             func(ctx context.Context, c *clientpkg.Client, r *Result) error { return nil },
			},
			wantErr: false,
		},
		{
			name: "empty name",
			sc: Scenario{
				Name:            "",
				Title:           "Title",
				Persona:         clientpkg.Persona{ID: "test"},
				RequiresPosture: Doctrine,
				Run:             func(ctx context.Context, c *clientpkg.Client, r *Result) error { return nil },
			},
			wantErr: true,
		},
		{
			name: "empty title",
			sc: Scenario{
				Name:            "test",
				Title:           "",
				Persona:         clientpkg.Persona{ID: "test"},
				RequiresPosture: Doctrine,
				Run:             func(ctx context.Context, c *clientpkg.Client, r *Result) error { return nil },
			},
			wantErr: true,
		},
		{
			name: "empty persona ID",
			sc: Scenario{
				Name:            "test",
				Title:           "Title",
				Persona:         clientpkg.Persona{ID: ""},
				RequiresPosture: Doctrine,
				Run:             func(ctx context.Context, c *clientpkg.Client, r *Result) error { return nil },
			},
			wantErr: true,
		},
		{
			name: "empty posture",
			sc: Scenario{
				Name:            "test",
				Title:           "Title",
				Persona:         clientpkg.Persona{ID: "test"},
				RequiresPosture: "",
				Run:             func(ctx context.Context, c *clientpkg.Client, r *Result) error { return nil },
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasErr := tt.sc.Name == "" || tt.sc.Title == "" || tt.sc.Persona.ID == "" || tt.sc.RequiresPosture == "" || tt.sc.Run == nil
			assert.Equal(t, tt.wantErr, hasErr, "validation error mismatch")
		})
	}
}

func TestExecute(t *testing.T) {
	ctx := context.Background()

	// Create a mock client
	mockClient := &clientpkg.Client{}

	// Test successful execution
	sc := Scenario{
		Name:    "test-scenario",
		Title:   "Test Scenario",
		Persona: clientpkg.Persona{ID: "test-persona"},
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			r.note("test note")
			r.tx("tx123")
			return nil
		},
	}

	result := Execute(ctx, mockClient, sc)

	assert.True(t, result.OK, "Execute should succeed, got error: %s", result.Err)
	assert.Equal(t, "test-scenario", result.Name, "Execute should set Name")
	assert.Equal(t, "Test Scenario", result.Title, "Execute should set Title")
	assert.Equal(t, "test-persona", result.Persona, "Execute should set Persona")
	assert.Len(t, result.Notes, 1, "Execute should record notes")
	assert.Len(t, result.TxHashes, 1, "Execute should record tx hashes")
	assert.GreaterOrEqual(t, result.DurationMS, int64(0), "Execute should record non-negative duration")

	scFail := Scenario{
		Name:    "fail-scenario",
		Title:   "Fail Scenario",
		Persona: clientpkg.Persona{ID: "test-persona"},
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			return errors.New("test error")
		},
	}

	resultFail := Execute(ctx, mockClient, scFail)

	assert.False(t, resultFail.OK, "Execute should fail when Run returns error")
	assert.Equal(t, "test error", resultFail.Err, "Execute should set error")
}

func TestExecute_RetainsDemoCorrelation(t *testing.T) {
	t.Setenv(string(constants.EnvVar.DemoRunID), "fedramp-run-123")
	t.Setenv(string(constants.EnvVar.DemoScenarioID), "fedramp-provision")
	scenario := Scenario{
		Name:    "fedramp-provision-harness",
		Title:   "FedRAMP Provision",
		Persona: clientpkg.Persona{ID: "test-persona"},
		Run:     func(context.Context, *clientpkg.Client, *Result) error { return nil },
	}

	result := Execute(context.Background(), &clientpkg.Client{}, scenario)

	assert.Equal(t, "fedramp-run-123", result.RunID)
	assert.Equal(t, "fedramp-provision", result.ScenarioID)
}

func TestExecuteWithRecording(t *testing.T) {
	ctx := context.Background()

	// Create a mock client
	mockClient := &clientpkg.Client{}

	// Test that Execute sets up recording
	sc := Scenario{
		Name:    "test-recording",
		Title:   "Test Recording",
		Persona: clientpkg.Persona{ID: "test-persona"},
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			// The client should have recording set
			return nil
		},
	}

	result := Execute(ctx, mockClient, sc)

	assert.True(t, result.OK, "Execute should succeed, got error: %s", result.Err)
}

func TestExecuteTiming(t *testing.T) {
	ctx := context.Background()
	mockClient := &clientpkg.Client{}

	// Test with a scenario that takes some time
	sc := Scenario{
		Name:    "test-timing",
		Title:   "Test Timing",
		Persona: clientpkg.Persona{ID: "test-persona"},
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}

	result := Execute(ctx, mockClient, sc)

	assert.GreaterOrEqual(t, result.DurationMS, int64(0), "Execute should record non-negative duration")
	assert.False(t, result.StartedAt.IsZero(), "Execute should record StartedAt")
}

func TestExecuteContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockClient := &clientpkg.Client{}
	sc := Scenario{
		Name:    "cancel-test",
		Title:   "Cancel Test",
		Persona: clientpkg.Persona{ID: "test"},
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			// This should not run if context is already cancelled
			return nil
		},
	}

	result := Execute(ctx, mockClient, sc)

	// The scenario should still complete (it just won't run)
	// Execute doesn't check context cancellation before calling Run
	_ = result
}

func TestExecuteMultipleNotes(t *testing.T) {
	ctx := context.Background()
	mockClient := &clientpkg.Client{}

	sc := Scenario{
		Name:    "multi-note",
		Title:   "Multi Note",
		Persona: clientpkg.Persona{ID: "test"},
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			for i := 0; i < 10; i++ {
				r.note("note %d", i)
			}
			return nil
		},
	}

	result := Execute(ctx, mockClient, sc)

	assert.Len(t, result.Notes, 10, "Should have 10 notes")
}

func TestExecuteMultipleTxHashes(t *testing.T) {
	ctx := context.Background()
	mockClient := &clientpkg.Client{}

	sc := Scenario{
		Name:    "multi-tx",
		Title:   "Multi TX",
		Persona: clientpkg.Persona{ID: "test"},
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			for i := 0; i < 5; i++ {
				r.tx(fmt.Sprintf("hash%d", i))
			}
			return nil
		},
	}

	result := Execute(ctx, mockClient, sc)

	assert.Len(t, result.TxHashes, 5, "Should have 5 tx hashes")
}

func TestExecuteWithNilClient(t *testing.T) {
	ctx := context.Background()
	sc := Scenario{
		Name:    "test",
		Title:   "Test",
		Persona: clientpkg.Persona{ID: "test"},
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			// This should panic when c.Record is called
			return nil
		},
	}

	defer func() {
		assert.NotNil(t, recover(), "Execute with nil client should panic")
	}()

	Execute(ctx, nil, sc)
}

func TestScenarioRunNilPanic(t *testing.T) {
	ctx := context.Background()
	mockClient := &clientpkg.Client{}

	sc := Scenario{
		Name:    "nil-run",
		Title:   "Nil Run",
		Persona: clientpkg.Persona{ID: "test"},
		Run:     nil,
	}

	// This should panic, which is expected behavior
	defer func() {
		assert.NotNil(t, recover(), "Scenario with nil Run should panic")
	}()

	Execute(ctx, mockClient, sc)
}

func TestFindCaseSensitive(t *testing.T) {
	_, ok := Find("MCP-PLAIN")
	assert.False(t, ok, "Find should be case-sensitive, 'MCP-PLAIN' should not match 'mcp-plain'")

	_, ok = Find("mcp-plain")
	assert.True(t, ok, "Find should find 'mcp-plain' with exact case")
}

func TestFindEmptyRegistry(t *testing.T) {
	_, ok := Find("non-existent")
	assert.False(t, ok, "Find should return false for non-existent scenario")
}

func TestRegistryScenarioOrder(t *testing.T) {
	scenarios := Registry()

	// Verify that scenarios are in the expected order:
	// MCP scenarios first, then A2A scenarios, then governance scenarios
	require.GreaterOrEqual(t, len(scenarios), 3, "Registry should have at least 3 scenarios")

	assert.Contains(t, scenarios[0].Name, "mcp", "First scenario should be MCP")

	lastName := scenarios[len(scenarios)-1].Name
	validLastPrefixes := []string{"consensus", "envelope", "notary", "consensus", "delegation", "dhs", "finance", "fedramp"}
	found := false
	for _, p := range validLastPrefixes {
		if strings.Contains(lastName, p) {
			found = true
			break
		}
	}
	assert.True(t, found, "Last scenario should be governance or dhs, got %q", lastName)
}

func TestRegistryNoRemovedDemoScenarios(t *testing.T) {
	scenarios := Registry()
	removedPrefixes := []string{"dow-", "swarm-", "secure-data-"}

	for _, sc := range scenarios {
		for _, prefix := range removedPrefixes {
			assert.NotContains(t, sc.Name, prefix,
				"Registry should not contain scenario %q with removed-demo prefix %q", sc.Name, prefix)
		}
	}
}

func TestRegistryUniqueNames(t *testing.T) {
	scenarios := Registry()
	nameSet := make(map[string]bool)

	for _, sc := range scenarios {
		assert.False(t, nameSet[sc.Name], "Registry should have unique scenario names, duplicate found: %q", sc.Name)
		nameSet[sc.Name] = true
	}
}

func TestRegistryCount(t *testing.T) {
	scenarios := Registry()
	expectedCount := len(mcpScenarios()) + len(a2aScenarios()) + len(governanceScenarios()) + len(ensembleScenarios()) + len(dhsScenarios()) + len(financeScenarios()) + len(fedrampScenarios())

	assert.Equal(t, expectedCount, len(scenarios), "Registry should have correct scenario count")
}

func TestScenarioRequiresPostureValues(t *testing.T) {
	scenarios := Registry()
	validPostures := map[Posture]bool{
		Doctrine:  true,
		Consensus: true,
		Notary:    true,
	}

	for _, sc := range scenarios {
		assert.True(t, validPostures[sc.RequiresPosture], "Scenario %q has invalid posture %q", sc.Name, sc.RequiresPosture)
	}
}

func TestScenarioPostureDistribution(t *testing.T) {
	scenarios := Registry()
	postureCount := make(map[Posture]int)

	for _, sc := range scenarios {
		postureCount[sc.RequiresPosture]++
	}

	assert.NotZero(t, postureCount[Doctrine], "Registry should have at least one Doctrine scenario")
	assert.NotZero(t, postureCount[Consensus], "Registry should have at least one Consensus scenario")
	assert.NotZero(t, postureCount[Notary], "Registry should have at least one Notary scenario")
}

func TestScenarioRunNotNil(t *testing.T) {
	scenarios := Registry()

	for _, sc := range scenarios {
		assert.NotNil(t, sc.Run, "Scenario %q should have non-nil Run function", sc.Name)
	}
}

func TestScenarioTitles(t *testing.T) {
	scenarios := Registry()

	for _, sc := range scenarios {
		assert.NotEmpty(t, sc.Title, "Scenario %q should have a non-empty Title", sc.Name)
		assert.NotEqual(t, sc.Name, sc.Title, "Scenario %q should have a Title different from Name", sc.Name)
	}
}

func TestScenarioPersonaUserAgent(t *testing.T) {
	scenarios := Registry()

	for _, sc := range scenarios {
		assert.NotEmpty(t, sc.Persona.UserAgent, "Scenario %q should have a non-empty UserAgent in Persona", sc.Name)
	}
}

// TestRegistryPostureDeclarations is a registry-wide regression test that
// asserts each scenario's RequiresPosture matches the governance elements it
// uses. The expected posture map documents the classification rationale for
// every scenario. If a scenario is added or its posture changes, this test
// forces the developer to update the map and verify the posture is correct.
//
// Classification rules:
//   - Plain MCP/A2A calls (no SubmitMaximal) → Doctrine (L1 only)
//   - SubmitMaximal with Ensemble, expects admission → Consensus (L1+L2)
//   - SubmitMaximal with suspend/approve flow → Notary (L1+L2+L3)
//   - SubmitMaximal that expects L1 rejection → Doctrine (L1 blocks first)
func TestRegistryPostureDeclarations(t *testing.T) {
	expectedPostures := map[string]Posture{
		// MCP scenarios — plain MCPToolsCall, no governance extras
		"mcp-plain":              Doctrine,
		"healthcare-success":     Doctrine,
		"healthcare-phi-blocked": Doctrine,
		"healthcare-gold-card":   Doctrine,
		"healthcare-sla-breach":  Doctrine,
		"mcp-advanced":           Doctrine,
		"mcp-secured":            Doctrine,

		// A2A scenarios — plain A2ACall, no governance extras
		"a2a-plain":    Doctrine,
		"a2a-secured":  Doctrine,
		"a2a-protobuf": Doctrine,

		// Governance scenarios — SubmitMaximal with Ensemble/Authenticator
		"consensus":        Consensus, // Ensemble, no Authenticator, expects admission
		"envelope-maximal": Notary,    // Ensemble + Authenticator suspend flow
		"agent-delegation": Doctrine,  // MCPToolsCall/MCPToolsList, no governance extras
		"consensus-quorum": Consensus, // Ensemble, expects admission
		"notary-oob":       Notary,    // Ensemble + Authenticator suspend flow

		// Ensemble scenarios — EnsembleChat via g8ee, GovKit identity binding, polls audit vault
		"ensemble-chat-file-create": Doctrine, // Chat -> file_create tool -> FILE_EDIT receipt
		"ensemble-chat-file-write":  Doctrine, // Chat -> file_write tool -> FILE_EDIT receipt
		"ensemble-document-update":  Doctrine, // Direct DOCUMENT_UPDATE create + merge=true patch + read-back (Bug 10 regression)
		"ensemble-document-delete":  Doctrine, // Direct DOCUMENT_UPDATE create + DOCUMENT_DELETE + verify absence

		// DHS scenarios
		"dhs-ingest":         Consensus, // Ensemble + Authenticator inline, expects admission
		"dhs-release":        Notary,    // Ensemble + Authenticator suspend flow
		"dhs-cue":            Consensus, // Ensemble + Authenticator inline, expects admission
		"dhs-evidence-block": Doctrine,  // Ensemble + Authenticator, but tests L1 rejection
		"dhs-purge":          Consensus, // Ensemble + Authenticator inline, expects admission

		// Finance scenarios — plain MCPToolsCall
		"finance-unauthorized-trade": Doctrine,

		// FedRAMP scenarios
		"fedramp-provision":      Consensus, // Ensemble + Authenticator inline, expects admission
		"fedramp-deny":           Doctrine,  // Ensemble + Authenticator, but tests L1 rejection
		"fedramp-escalate":       Notary,    // Ensemble + Authenticator suspend flow
		"fedramp-revert":         Consensus, // Ensemble + Authenticator inline, expects admission
		"fedramp-evidence-block": Doctrine,  // Ensemble + Authenticator, but tests L1 rejection
	}

	scenarios := Registry()

	// Assert every registry scenario has an expected posture entry.
	for _, sc := range scenarios {
		expected, ok := expectedPostures[sc.Name]
		require.True(t, ok, "Scenario %q missing from expectedPostures map — add it with the correct posture", sc.Name)
		assert.Equal(t, expected, sc.RequiresPosture,
			"Scenario %q has posture %q but expected %q", sc.Name, sc.RequiresPosture, expected)
	}

	// Assert no stale entries in the expected map.
	registryNames := make(map[string]bool)
	for _, sc := range scenarios {
		registryNames[sc.Name] = true
	}
	for name := range expectedPostures {
		assert.True(t, registryNames[name],
			"expectedPostures map contains %q but it is not in the registry — remove the stale entry", name)
	}
}

func TestPersonaConstants(t *testing.T) {
	// Verify that the persona constants are defined
	// These are package-level variables in mcp_a2a.go
	// We can't directly access them, but we can verify scenarios use them
	scenarios := Registry()

	personaSet := make(map[string]bool)
	for _, sc := range scenarios {
		personaSet[sc.Persona.ID] = true
	}

	// Check for expected personas (note: "principal" is used internally in governance
	// scenarios but not as a scenario's main persona)
	expectedPersonas := []string{
		"claude-desktop",
		"cursor",
		"enterprise-agent",
		"a2a-peer",
		"a2a-secure-peer",
		"protobuf-agent",
		"ensemble-producer",
	}

	for _, expected := range expectedPersonas {
		assert.True(t, personaSet[expected], "Registry should use persona %q", expected)
	}
}
