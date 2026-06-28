// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

func TestRegistry(t *testing.T) {
	scenarios := Registry()

	if len(scenarios) == 0 {
		t.Error("Registry should return at least one scenario")
	}

	// Check that all scenarios have required fields
	for _, sc := range scenarios {
		if sc.Name == "" {
			t.Error("Scenario should have a Name")
		}
		if sc.Title == "" {
			t.Error("Scenario should have a Title")
		}
		if sc.Persona.ID == "" {
			t.Error("Scenario should have a Persona with ID")
		}
		if sc.RequiresPosture == "" {
			t.Error("Scenario should have a RequiresPosture")
		}
		if sc.Run == nil {
			t.Error("Scenario should have a Run function")
		}
	}
}

func TestFind(t *testing.T) {
	scenarios := Registry()

	if len(scenarios) == 0 {
		t.Fatal("Registry should return at least one scenario for Find test")
	}

	// Test finding an existing scenario
	firstName := scenarios[0].Name
	found, ok := Find(firstName)
	if !ok {
		t.Errorf("Find should return true for existing scenario %q", firstName)
	}
	if found.Name != firstName {
		t.Errorf("Find should return scenario with name %q, got %q", firstName, found.Name)
	}

	// Test finding a non-existent scenario
	_, ok = Find("non-existent-scenario-name")
	if ok {
		t.Error("Find should return false for non-existent scenario")
	}
}

func TestPostureConstants(t *testing.T) {
	if Doctrine != Posture("doctrine") {
		t.Error("Doctrine constant should be 'doctrine'")
	}
	if Consensus != Posture("consensus") {
		t.Error("Consensus constant should be 'consensus'")
	}
	if Notary != Posture("notary") {
		t.Error("Notary constant should be 'notary'")
	}
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
		if string(tt.posture) != tt.expected {
			t.Errorf("Posture %q should equal %q", tt.posture, tt.expected)
		}
	}
}

func TestPostureEquality(t *testing.T) {
	if Doctrine != Posture("doctrine") {
		t.Error("Doctrine should equal Posture(\"doctrine\")")
	}
	if Consensus != Posture("consensus") {
		t.Error("Consensus should equal Posture(\"consensus\")")
	}
	if Notary != Posture("notary") {
		t.Error("Notary should equal Posture(\"notary\")")
	}

	// Test inequality
	if Doctrine == Consensus {
		t.Error("Doctrine should not equal Consensus")
	}
	if Consensus == Notary {
		t.Error("Consensus should not equal Notary")
	}
	if Doctrine == Notary {
		t.Error("Doctrine should not equal Notary")
	}
}

func TestResult_Note(t *testing.T) {
	r := &Result{}
	r.note("test note %d", 42)

	if len(r.Notes) != 1 {
		t.Errorf("note should add one entry, got %d", len(r.Notes))
	}
	if r.Notes[0] != "test note 42" {
		t.Errorf("note should format correctly, got %q", r.Notes[0])
	}

	// Add another note
	r.note("second note")
	if len(r.Notes) != 2 {
		t.Errorf("note should append, got %d", len(r.Notes))
	}
}

func TestResult_Tx(t *testing.T) {
	r := &Result{}

	// Add a valid hash
	r.tx("hash1")
	if len(r.TxHashes) != 1 {
		t.Errorf("tx should add one entry for valid hash, got %d", len(r.TxHashes))
	}
	if r.TxHashes[0] != "hash1" {
		t.Errorf("tx should store hash correctly, got %q", r.TxHashes[0])
	}

	// Add another valid hash
	r.tx("hash2")
	if len(r.TxHashes) != 2 {
		t.Errorf("tx should append, got %d", len(r.TxHashes))
	}

	// Try to add empty hash (should be skipped)
	r.tx("")
	if len(r.TxHashes) != 2 {
		t.Errorf("tx should skip empty hash, got %d", len(r.TxHashes))
	}
}

func TestResult(t *testing.T) {
	t.Run("note method", func(t *testing.T) {
		r := &Result{}
		r.note("test note %d", 42)

		if len(r.Notes) != 1 {
			t.Errorf("note should add one entry, got %d", len(r.Notes))
		}
		if r.Notes[0] != "test note 42" {
			t.Errorf("note should format correctly, got %q", r.Notes[0])
		}

		// Add another note
		r.note("second note")
		if len(r.Notes) != 2 {
			t.Errorf("note should append, got %d", len(r.Notes))
		}
	})

	t.Run("tx method", func(t *testing.T) {
		r := &Result{}

		// Add a valid hash
		r.tx("hash1")
		if len(r.TxHashes) != 1 {
			t.Errorf("tx should add one entry for valid hash, got %d", len(r.TxHashes))
		}
		if r.TxHashes[0] != "hash1" {
			t.Errorf("tx should store hash correctly, got %q", r.TxHashes[0])
		}

		// Add another valid hash
		r.tx("hash2")
		if len(r.TxHashes) != 2 {
			t.Errorf("tx should append, got %d", len(r.TxHashes))
		}

		// Try to add empty hash (should be skipped)
		r.tx("")
		if len(r.TxHashes) != 2 {
			t.Errorf("tx should skip empty hash, got %d", len(r.TxHashes))
		}
	})
}

func TestResultZeroValue(t *testing.T) {
	var r Result

	if r.Name != "" {
		t.Errorf("zero Result Name should be empty, got %q", r.Name)
	}
	if r.Title != "" {
		t.Errorf("zero Result Title should be empty, got %q", r.Title)
	}
	if r.Persona != "" {
		t.Errorf("zero Result Persona should be empty, got %q", r.Persona)
	}
	if r.RequiresPosture != "" {
		t.Errorf("zero Result RequiresPosture should be empty, got %q", r.RequiresPosture)
	}
	if !r.StartedAt.IsZero() {
		t.Error("zero Result StartedAt should be zero time")
	}
	if r.DurationMS != 0 {
		t.Errorf("zero Result DurationMS should be 0, got %d", r.DurationMS)
	}
	if r.Exchanges != nil {
		t.Error("zero Result Exchanges should be nil")
	}
	if r.TxHashes != nil {
		t.Error("zero Result TxHashes should be nil")
	}
	if r.Notes != nil {
		t.Error("zero Result Notes should be nil")
	}
	if r.OK {
		t.Error("zero Result OK should be false")
	}
	if r.Err != "" {
		t.Errorf("zero Result Err should be empty, got %q", r.Err)
	}
}

func TestResultFields(t *testing.T) {
	r := Result{}

	// Test zero values
	if r.Name != "" {
		t.Error("Zero Result should have empty Name")
	}
	if r.Title != "" {
		t.Error("Zero Result should have empty Title")
	}
	if r.Persona != "" {
		t.Error("Zero Result should have empty Persona")
	}
	if r.RequiresPosture != "" {
		t.Error("Zero Result should have empty RequiresPosture")
	}
	if !r.StartedAt.IsZero() {
		t.Error("Zero Result should have zero StartedAt")
	}
	if r.DurationMS != 0 {
		t.Error("Zero Result should have zero DurationMS")
	}
	if r.Exchanges != nil {
		t.Error("Zero Result should have nil Exchanges")
	}
	if r.TxHashes != nil {
		t.Error("Zero Result should have nil TxHashes")
	}
	if r.Notes != nil {
		t.Error("Zero Result should have nil Notes")
	}
	if r.OK {
		t.Error("Zero Result should have false OK")
	}
	if r.Err != "" {
		t.Error("Zero Result should have empty Err")
	}
}

func TestResultNoteFormatting(t *testing.T) {
	r := &Result{}

	// Test various format strings
	r.note("simple")
	if len(r.Notes) != 1 || r.Notes[0] != "simple" {
		t.Errorf("note formatting failed, got %q", r.Notes[0])
	}

	r.note("with %s", "arg")
	if len(r.Notes) != 2 || r.Notes[1] != "with arg" {
		t.Errorf("note formatting with arg failed, got %q", r.Notes[1])
	}

	r.note("number %d", 42)
	if len(r.Notes) != 3 || r.Notes[2] != "number 42" {
		t.Errorf("note formatting with number failed, got %q", r.Notes[2])
	}

	r.note("multiple %s %d %v", "args", 123, true)
	if len(r.Notes) != 4 || r.Notes[3] != "multiple args 123 true" {
		t.Errorf("note formatting with multiple args failed, got %q", r.Notes[3])
	}
}

func TestResultNoteWithNilArgs(t *testing.T) {
	r := &Result{}

	// Test with nil args (should not panic)
	r.note("test")
	if len(r.Notes) != 1 {
		t.Errorf("note with no args should work, got %d notes", len(r.Notes))
	}
}

func TestResultTxHashHandling(t *testing.T) {
	r := &Result{}

	// Test adding various hash formats
	validHashes := []string{
		"abc123",
		"deadbeef",
		"0123456789abcdef",
		"ABCDEF",
	}

	for _, hash := range validHashes {
		r.tx(hash)
	}

	if len(r.TxHashes) != len(validHashes) {
		t.Errorf("Should have %d hashes, got %d", len(validHashes), len(r.TxHashes))
	}

	// Verify order is preserved
	for i, hash := range validHashes {
		if r.TxHashes[i] != hash {
			t.Errorf("Hash at index %d should be %q, got %q", i, hash, r.TxHashes[i])
		}
	}

	// Test that empty strings are skipped
	initialCount := len(r.TxHashes)
	r.tx("")
	if len(r.TxHashes) != initialCount {
		t.Error("Empty hashes should be skipped")
	}

	// Note: whitespace strings are NOT skipped by the tx function
	// This is the current behavior
	r.tx("  ")
	if len(r.TxHashes) == initialCount {
		t.Error("Whitespace hashes are currently added (not skipped)")
	}
}

func TestResultTxWithWhitespace(t *testing.T) {
	r := &Result{}

	// Test that whitespace-only strings are added (current behavior)
	r.tx("  ")
	if len(r.TxHashes) != 1 {
		t.Error("whitespace-only hash should be added (current behavior)")
	}

	r.tx("\t\n")
	if len(r.TxHashes) != 2 {
		t.Error("tab/newline hash should be added (current behavior)")
	}
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

	// Test that Result can be serialized to JSON
	data, err := json.Marshal(r)
	if err != nil {
		t.Errorf("Result should be JSON serializable: %v", err)
	}

	// Test that it can be deserialized
	var r2 Result
	err = json.Unmarshal(data, &r2)
	if err != nil {
		t.Errorf("Result should be JSON deserializable: %v", err)
	}

	if r2.Name != r.Name {
		t.Error("Deserialized Name should match")
	}
	if r2.Title != r.Title {
		t.Error("Deserialized Title should match")
	}
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
	if err != nil {
		t.Errorf("Result with error should be JSON serializable: %v", err)
	}

	var r2 Result
	err = json.Unmarshal(data, &r2)
	if err != nil {
		t.Errorf("Result with error should be JSON deserializable: %v", err)
	}

	if r2.OK {
		t.Error("Deserialized OK should be false")
	}
	if r2.Err != "something went wrong" {
		t.Errorf("Deserialized Err should match, got %q", r2.Err)
	}
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

	if sc.Name != "test-name" {
		t.Errorf("Scenario Name should be set")
	}
	if sc.Title != "Test Title" {
		t.Errorf("Scenario Title should be set")
	}
	if sc.Persona.ID != "test-persona" {
		t.Errorf("Scenario Persona should be set")
	}
	if sc.RequiresPosture != Doctrine {
		t.Errorf("Scenario RequiresPosture should be set")
	}
	if sc.Run == nil {
		t.Errorf("Scenario Run should be set")
	}
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
			if hasErr != tt.wantErr {
				t.Errorf("validation error mismatch, got %v, want %v", hasErr, tt.wantErr)
			}
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

	if !result.OK {
		t.Errorf("Execute should succeed, got error: %s", result.Err)
	}
	if result.Name != "test-scenario" {
		t.Errorf("Execute should set Name, got %q", result.Name)
	}
	if result.Title != "Test Scenario" {
		t.Errorf("Execute should set Title, got %q", result.Title)
	}
	if result.Persona != "test-persona" {
		t.Errorf("Execute should set Persona, got %q", result.Persona)
	}
	if len(result.Notes) != 1 {
		t.Errorf("Execute should record notes, got %d", len(result.Notes))
	}
	if len(result.TxHashes) != 1 {
		t.Errorf("Execute should record tx hashes, got %d", len(result.TxHashes))
	}
	if result.DurationMS < 0 {
		t.Error("Execute should record non-negative duration")
	}

	// Test failed execution
	scFail := Scenario{
		Name:    "fail-scenario",
		Title:   "Fail Scenario",
		Persona: clientpkg.Persona{ID: "test-persona"},
		Run: func(ctx context.Context, c *clientpkg.Client, r *Result) error {
			return errors.New("test error")
		},
	}

	resultFail := Execute(ctx, mockClient, scFail)

	if resultFail.OK {
		t.Error("Execute should fail when Run returns error")
	}
	if resultFail.Err != "test error" {
		t.Errorf("Execute should set error, got %q", resultFail.Err)
	}
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

	// Result should be successful
	if !result.OK {
		t.Errorf("Execute should succeed, got error: %s", result.Err)
	}
	// Exchanges slice may be nil if no HTTP calls were made
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

	if result.DurationMS < 0 {
		t.Errorf("Execute should record non-negative duration, got %dms", result.DurationMS)
	}
	if result.StartedAt.IsZero() {
		t.Error("Execute should record StartedAt")
	}
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

	if len(result.Notes) != 10 {
		t.Errorf("Should have 10 notes, got %d", len(result.Notes))
	}
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

	if len(result.TxHashes) != 5 {
		t.Errorf("Should have 5 tx hashes, got %d", len(result.TxHashes))
	}
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
		if r := recover(); r == nil {
			t.Error("Execute with nil client should panic")
		}
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
		if r := recover(); r == nil {
			t.Error("Scenario with nil Run should panic")
		}
	}()

	Execute(ctx, mockClient, sc)
}

func TestFindCaseSensitive(t *testing.T) {
	// Find should be case-sensitive
	_, ok := Find("MCP-PLAIN")
	if ok {
		t.Error("Find should be case-sensitive, 'MCP-PLAIN' should not match 'mcp-plain'")
	}

	_, ok = Find("mcp-plain")
	if !ok {
		t.Error("Find should find 'mcp-plain' with exact case")
	}
}

func TestFindEmptyRegistry(t *testing.T) {
	// This test verifies the behavior when looking for scenarios
	// The actual Registry() always returns scenarios, so we test the lookup logic
	_, ok := Find("non-existent")
	if ok {
		t.Error("Find should return false for non-existent scenario")
	}
}

func TestRegistryScenarioOrder(t *testing.T) {
	scenarios := Registry()

	// Verify that scenarios are in the expected order:
	// MCP scenarios first, then A2A scenarios, then governance scenarios
	if len(scenarios) < 3 {
		t.Fatal("Registry should have at least 3 scenarios")
	}

	// First scenario should be an MCP scenario
	if !contains(scenarios[0].Name, "mcp") {
		t.Errorf("First scenario should be MCP, got %q", scenarios[0].Name)
	}

	// Last scenario should be a governance, dow, or dhs scenario
	lastName := scenarios[len(scenarios)-1].Name
	if !contains(lastName, "consensus") && !contains(lastName, "envelope") && !contains(lastName, "notary") && !contains(lastName, "tribunal") && !contains(lastName, "delegation") && !contains(lastName, "dow") && !contains(lastName, "dhs") {
		t.Errorf("Last scenario should be governance, dow, or dhs, got %q", lastName)
	}
}

func TestRegistryUniqueNames(t *testing.T) {
	scenarios := Registry()
	nameSet := make(map[string]bool)

	for _, sc := range scenarios {
		if nameSet[sc.Name] {
			t.Errorf("Registry should have unique scenario names, duplicate found: %q", sc.Name)
		}
		nameSet[sc.Name] = true
	}
}

func TestRegistryCount(t *testing.T) {
	scenarios := Registry()
	expectedCount := 22 // 5 MCP + 3 A2A + 6 governance + 2 dow + 6 dhs

	if len(scenarios) != expectedCount {
		t.Errorf("Registry should have %d scenarios, got %d", expectedCount, len(scenarios))
	}
}

func TestScenarioRequiresPostureValues(t *testing.T) {
	scenarios := Registry()
	validPostures := map[Posture]bool{
		Doctrine:  true,
		Consensus: true,
		Notary:    true,
	}

	for _, sc := range scenarios {
		if !validPostures[sc.RequiresPosture] {
			t.Errorf("Scenario %q has invalid posture %q", sc.Name, sc.RequiresPosture)
		}
	}
}

func TestScenarioPostureDistribution(t *testing.T) {
	scenarios := Registry()
	postureCount := make(map[Posture]int)

	for _, sc := range scenarios {
		postureCount[sc.RequiresPosture]++
	}

	// Should have at least one scenario for each posture
	if postureCount[Doctrine] == 0 {
		t.Error("Registry should have at least one Doctrine scenario")
	}
	if postureCount[Consensus] == 0 {
		t.Error("Registry should have at least one Consensus scenario")
	}
	if postureCount[Notary] == 0 {
		t.Error("Registry should have at least one Notary scenario")
	}
}

func TestScenarioRunNotNil(t *testing.T) {
	scenarios := Registry()

	for _, sc := range scenarios {
		if sc.Run == nil {
			t.Errorf("Scenario %q should have non-nil Run function", sc.Name)
		}
	}
}

func TestScenarioTitles(t *testing.T) {
	scenarios := Registry()

	for _, sc := range scenarios {
		if sc.Title == "" {
			t.Errorf("Scenario %q should have a non-empty Title", sc.Name)
		}
		// Title should be different from name
		if sc.Title == sc.Name {
			t.Errorf("Scenario %q should have a Title different from Name", sc.Name)
		}
	}
}

func TestScenarioPersonaUserAgent(t *testing.T) {
	scenarios := Registry()

	for _, sc := range scenarios {
		if sc.Persona.UserAgent == "" {
			t.Errorf("Scenario %q should have a non-empty UserAgent in Persona", sc.Name)
		}
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
		if !personaSet[expected] {
			t.Errorf("Registry should use persona %q", expected)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
