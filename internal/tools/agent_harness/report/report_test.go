// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/scenarios"
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

	jsonPath := filepath.Join(tmpDir, "report.json")
	mdPath := filepath.Join(tmpDir, "report.md")
	_, _, err := Write(jsonPath, mdPath, rep)
	require.NoError(t, err)

	// Verify JSON file exists and is readable
	assert.NotEmpty(t, jsonPath)
	_, err = os.Stat(jsonPath)
	assert.NoError(t, err)

	// Verify MD file exists and is readable
	assert.NotEmpty(t, mdPath)
	_, err = os.Stat(mdPath)
	assert.NoError(t, err)

	// Verify paths are in the temp dir
	assert.Equal(t, tmpDir, filepath.Dir(jsonPath))
	assert.Equal(t, tmpDir, filepath.Dir(mdPath))
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

	jsonPath := filepath.Join(nestedDir, "report.json")
	mdPath := filepath.Join(nestedDir, "report.md")
	_, _, err := Write(jsonPath, mdPath, rep)
	require.NoError(t, err)

	// Verify nested directory was created
	_, err = os.Stat(nestedDir)
	assert.NoError(t, err)

	// Verify files exist
	_, err = os.Stat(jsonPath)
	assert.NoError(t, err)
	_, err = os.Stat(mdPath)
	assert.NoError(t, err)
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
	assert.NotEmpty(t, md)

	// Check for expected sections
	expectedStrings := []string{
		"# Agent Harness run report",
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
		assert.True(t, strings.Contains(md, s), "markdown should contain %q", s)
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
	assert.NotEmpty(t, md)
	assert.True(t, strings.Contains(md, "(auto-discover)"), "markdown should show (auto-discover) for empty operator session")
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
	assert.True(t, strings.Contains(md, "No receipts returned"), "markdown should show message when no receipts")
}

func TestIndexReceipts(t *testing.T) {
	receipts := []clientpkg.Receipt{
		{TransactionHash: "hash1", ActionType: "action1"},
		{TransactionHash: "hash2", ActionType: "action2"},
		{TransactionHash: "", ActionType: "action3"}, // empty hash should be skipped
	}

	idx := indexReceipts(receipts)

	assert.Len(t, idx, 2)
	assert.Contains(t, idx, "hash1")
	assert.Contains(t, idx, "hash2")
	assert.NotContains(t, idx, "")
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
			assert.True(t, strings.Contains(result, tt.contains), "txMatch result should contain %q, got %s", tt.contains, result)
		})
	}
}

func TestMark(t *testing.T) {
	assert.Equal(t, "✅ ok", mark(true))
	assert.Equal(t, "❌ fail", mark(false))
}

func TestOrNone(t *testing.T) {
	assert.Equal(t, "(auto-discover)", orNone(""))
	assert.Equal(t, "test", orNone("test"))
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
		assert.Equal(t, tt.expected, shortHash(tt.input), "shortHash(%q)", tt.input)
	}
}
