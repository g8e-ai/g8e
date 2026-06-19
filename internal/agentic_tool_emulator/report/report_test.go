// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	clientpkg "github.com/g8e-ai/g8e/internal/agentic_tool_emulator/client"
	"github.com/g8e-ai/g8e/internal/agentic_tool_emulator/scenarios"
)

func TestWrite(t *testing.T) {
	tmpDir := t.TempDir()

	rep := Report{
		GeneratedAt:       time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
		Gateway:           "https://localhost:8443",
		OperatorSessionID: "op-session-123",
		Results: []scenarios.Result{
			{
				Name:            "test-scenario",
				Title:           "Test Scenario",
				Persona:         "test-persona",
				RequiresPosture: scenarios.Doctrine,
				StartedAt:       time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
				DurationMS:      100,
				Exchanges: []clientpkg.Exchange{
					{Method: "GET", URL: "/api/test", Status: 200, LatencyMS: 50},
				},
				TxHashes: []string{"tx-hash-123"},
				Notes:    []string{"test note"},
				OK:       true,
			},
		},
		Receipts: []clientpkg.Receipt{
			{
				TransactionHash: "tx-hash-123",
				ActionType:      "fs_list",
				Status:          "committed",
				StateRootBefore: "root-before",
				StateRootAfter:  "root-after",
			},
		},
	}

	jsonPath, mdPath, err := Write(tmpDir, rep)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify JSON file exists and is readable
	if jsonPath == "" {
		t.Error("jsonPath should not be empty")
	}
	if _, err := os.Stat(jsonPath); err != nil {
		t.Errorf("JSON file does not exist: %v", err)
	}

	// Verify MD file exists and is readable
	if mdPath == "" {
		t.Error("mdPath should not be empty")
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Errorf("MD file does not exist: %v", err)
	}

	// Verify paths are in the temp dir
	if filepath.Dir(jsonPath) != tmpDir {
		t.Error("jsonPath should be in temp dir")
	}
	if filepath.Dir(mdPath) != tmpDir {
		t.Error("mdPath should be in temp dir")
	}
}

func TestWrite_NestedDir(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "path")

	rep := Report{
		GeneratedAt: time.Now(),
		Gateway:     "https://localhost:8443",
		Results:     []scenarios.Result{},
		Receipts:    []clientpkg.Receipt{},
	}

	jsonPath, mdPath, err := Write(nestedDir, rep)
	if err != nil {
		t.Fatalf("Write failed with nested dir: %v", err)
	}

	// Verify nested directory was created
	if _, err := os.Stat(nestedDir); err != nil {
		t.Errorf("Nested directory was not created: %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(jsonPath); err != nil {
		t.Errorf("JSON file does not exist in nested dir: %v", err)
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Errorf("MD file does not exist in nested dir: %v", err)
	}
}

func TestMarkdown(t *testing.T) {
	rep := Report{
		GeneratedAt:       time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
		Gateway:           "https://localhost:8443",
		OperatorSessionID: "op-session-123",
		Results: []scenarios.Result{
			{
				Name:            "test-scenario",
				Title:           "Test Scenario",
				Persona:         "test-persona",
				RequiresPosture: scenarios.Doctrine,
				StartedAt:       time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC),
				DurationMS:      100,
				Exchanges: []clientpkg.Exchange{
					{Method: "GET", URL: "/api/test", Status: 200, LatencyMS: 50},
				},
				TxHashes: []string{"tx-hash-123"},
				OK:       true,
			},
		},
		Receipts: []clientpkg.Receipt{
			{
				TransactionHash: "tx-hash-123",
				ActionType:      "fs_list",
				Status:          "committed",
				StateRootBefore: "root-before",
				StateRootAfter:  "root-after",
			},
		},
	}

	md := markdown(rep)
	if md == "" {
		t.Error("markdown should not be empty")
	}

	// Check for expected sections
	expectedStrings := []string{
		"# Agentic Tool Emulator run report",
		"Generated:",
		"Gateway:",
		"Operator session:",
		"## Summary",
		"## Detail",
		"## Real Operator receipts",
		"test-scenario",
		"✅ ok",
	}

	for _, s := range expectedStrings {
		if !contains(md, s) {
			t.Errorf("markdown should contain %q", s)
		}
	}
}

func TestMarkdown_EmptyResults(t *testing.T) {
	rep := Report{
		GeneratedAt:       time.Now(),
		Gateway:           "https://localhost:8443",
		OperatorSessionID: "",
		Results:           []scenarios.Result{},
		Receipts:          []clientpkg.Receipt{},
	}

	md := markdown(rep)
	if md == "" {
		t.Error("markdown should not be empty")
	}

	// Should show "(auto-discover)" for empty operator session
	if !contains(md, "(auto-discover)") {
		t.Error("markdown should show (auto-discover) for empty operator session")
	}
}

func TestMarkdown_NoReceipts(t *testing.T) {
	rep := Report{
		GeneratedAt: time.Now(),
		Gateway:     "https://localhost:8443",
		Results: []scenarios.Result{
			{
				Name:            "test",
				Title:           "Test",
				Persona:         "test",
				RequiresPosture: scenarios.Doctrine,
				StartedAt:       time.Now(),
				DurationMS:      100,
				Exchanges:       []clientpkg.Exchange{},
				OK:              true,
			},
		},
		Receipts: []clientpkg.Receipt{},
	}

	md := markdown(rep)
	if !contains(md, "No receipts returned") {
		t.Error("markdown should show message when no receipts")
	}
}

func TestIndexReceipts(t *testing.T) {
	receipts := []clientpkg.Receipt{
		{TransactionHash: "hash1", ActionType: "action1"},
		{TransactionHash: "hash2", ActionType: "action2"},
		{TransactionHash: "", ActionType: "action3"}, // empty hash should be skipped
	}

	idx := indexReceipts(receipts)

	if len(idx) != 2 {
		t.Errorf("index should have 2 entries, got %d", len(idx))
	}

	if _, ok := idx["hash1"]; !ok {
		t.Error("index should contain hash1")
	}
	if _, ok := idx["hash2"]; !ok {
		t.Error("index should contain hash2")
	}
	if _, ok := idx[""]; ok {
		t.Error("index should not contain empty hash")
	}
}

func TestTxMatch(t *testing.T) {
	idx := map[string]clientpkg.Receipt{
		"hash1": {TransactionHash: "hash1", ActionType: "action1"},
		"hash2": {TransactionHash: "hash2", ActionType: "action2"},
	}

	tests := []struct {
		name     string
		hashes   []string
		contains string
	}{
		{"empty hashes", []string{}, "—"},
		{"all matched", []string{"hash1", "hash2"}, "✓"},
		{"none matched", []string{"hash3"}, "(no receipt)"},
		{"partial match", []string{"hash1", "hash3"}, "✓"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := txMatch(tt.hashes, idx)
			if !contains(result, tt.contains) {
				t.Errorf("txMatch result should contain %q, got %s", tt.contains, result)
			}
		})
	}
}

func TestMark(t *testing.T) {
	if mark(true) != "✅ ok" {
		t.Error("mark(true) should return '✅ ok'")
	}
	if mark(false) != "❌ fail" {
		t.Error("mark(false) should return '❌ fail'")
	}
}

func TestOrNone(t *testing.T) {
	if orNone("") != "(auto-discover)" {
		t.Error("orNone('') should return '(auto-discover)'")
	}
	if orNone("test") != "test" {
		t.Error("orNone('test') should return 'test'")
	}
}

func TestShortHash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "short"},
		{"exactly12", "exactly12"},
		{"exactly12chars", "exactly12cha…"},
		{"thisislongerthan12", "thisislonger…"},
		{"a", "a"},
		{"", ""},
	}

	for _, tt := range tests {
		if result := shortHash(tt.input); result != tt.expected {
			t.Errorf("shortHash(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

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
