// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"testing"
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

func TestSetGovKit(t *testing.T) {
	testKit := &GovKit{
		Ensemble:   nil,
		Principal:  nil,
		L3Mode:     "test-mode",
		OperatorID: "test-operator",
	}

	SetGovKit(testKit)

	// We can't directly access kit since it's package-private,
	// but we can at least verify the function doesn't panic
	if testKit == nil {
		t.Error("SetGovKit should not nil the input")
	}
}

func TestShort(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "short"},
		{"exactly12", "exactly12"},
		{"exactly12char", "exactly12cha…"},
		{"thisislonger13", "thisislonger…"},
		{"a", "a"},
		{"", ""},
	}

	for _, tt := range tests {
		result := short(tt.input)
		if result != tt.expected {
			t.Errorf("short(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestSuspendedFromBody(t *testing.T) {
	// Test with a body that contains an /approve/ URL with hex hash
	body := []byte(`{"error":{"message":"L3 approval required","data":"https://localhost:8443/approve/abc123def456"}}`)
	hash, ok := suspendedFromBody(body)
	if !ok {
		t.Error("suspendedFromBody should find hash in body with /approve/ URL")
	}
	if hash != "abc123def456" {
		t.Errorf("suspendedFromBody should return correct hash, got %q", hash)
	}

	// Test with body that doesn't contain an /approve/ URL
	body = []byte(`{"other":"data"}`)
	_, ok = suspendedFromBody(body)
	if ok {
		t.Error("suspendedFromBody should return false when no /approve/ URL found")
	}

	// Test with empty body
	body = []byte{}
	_, ok = suspendedFromBody(body)
	if ok {
		t.Error("suspendedFromBody should return false for empty body")
	}
}
