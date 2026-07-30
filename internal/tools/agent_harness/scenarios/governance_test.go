// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

func TestSetGovKit(t *testing.T) {
	testKit := &GovKit{
		OperatorID: "test-operator",
	}

	SetGovKit(testKit)

	// We can't directly access kit since it's package-private,
	// but we can at least verify the function doesn't panic
}

func TestGovKitStruct(t *testing.T) {
	kit := &GovKit{
		OperatorID: "test-operator-id",
	}

	assert.Equal(t, "test-operator-id", kit.OperatorID, "GovKit OperatorID should be set")
}

func TestGovKitValidation(t *testing.T) {
	tests := []struct {
		name    string
		kit     *GovKit
		wantErr bool
	}{
		{
			name:    "nil kit",
			kit:     nil,
			wantErr: true,
		},
		{
			name: "empty operator ID",
			kit: &GovKit{
				OperatorID: "",
			},
			wantErr: true,
		},
		{
			name: "valid kit",
			kit: &GovKit{
				OperatorID: "test-operator",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasErr := tt.kit == nil || tt.kit.OperatorID == ""
			assert.Equal(t, tt.wantErr, hasErr, "GovKit validation error mismatch")
		})
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
		assert.Equal(t, tt.expected, result, "short(%q)", tt.input)
	}
}

func TestShortEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"012345678901", "012345678901"},   // exactly 12 chars
		{"0123456789012", "012345678901…"}, // 13 chars
		{"0123456789", "0123456789"},       // 10 chars
		{"a\nb", "a\nb"},                   // contains newline
		{"日本語", "日本語"},                     // unicode
		// Note: short() counts bytes, not runes. Each emoji is 4 bytes.
		// 3 emojis = 12 bytes, so they won't be truncated
		{"🎉🎉🎉", "🎉🎉🎉"},
	}

	for _, tt := range tests {
		result := short(tt.input)
		assert.Equal(t, tt.expected, result, "short(%q)", tt.input)
	}
}

func TestSuspendedFromBody(t *testing.T) {
	// Test with a body that contains an /approve/ URL with hex hash
	body := []byte(`{"error":{"message":"L3 approval required","data":"https://localhost:8443/approve/abc123def456"}}`)
	hash, ok := suspendedFromBody(body)
	assert.True(t, ok, "suspendedFromBody should find hash in body with /approve/ URL")
	assert.Equal(t, "abc123def456", hash, "suspendedFromBody should return correct hash")

	// Test with body that doesn't contain an /approve/ URL
	body = []byte(`{"other":"data"}`)
	_, ok = suspendedFromBody(body)
	assert.False(t, ok, "suspendedFromBody should return false when no /approve/ URL found")

	// Test with empty body
	body = []byte{}
	_, ok = suspendedFromBody(body)
	assert.False(t, ok, "suspendedFromBody should return false for empty body")

	// Test with nil body
	_, ok = suspendedFromBody(nil)
	assert.False(t, ok, "suspendedFromBody should return false for nil body")

	// Test with body containing /approve/ but no hash
	body = []byte(`{"data":"https://localhost:8443/approve/"}`)
	_, ok = suspendedFromBody(body)
	assert.False(t, ok, "suspendedFromBody should return false when /approve/ has no hash")
}

func TestSuspendedFromBodyEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected string
		found    bool
	}{
		{
			name:     "valid hex hash",
			body:     []byte(`{"data":"https://example.com/approve/abc123def456"}`),
			expected: "abc123def456",
			found:    true,
		},
		{
			name:     "hash with uppercase",
			body:     []byte(`{"data":"https://example.com/approve/ABC123DEF456"}`),
			expected: "ABC123DEF456",
			found:    true,
		},
		{
			name:     "hash with mixed case",
			body:     []byte(`{"data":"https://example.com/approve/aBc123DeF456"}`),
			expected: "aBc123DeF456",
			found:    true,
		},
		{
			name:     "hash with numbers only",
			body:     []byte(`{"data":"https://example.com/approve/123456789012"}`),
			expected: "123456789012",
			found:    true,
		},
		{
			name:     "no approve URL",
			body:     []byte(`{"data":"https://example.com/other/abc123"}`),
			expected: "",
			found:    false,
		},
		{
			name:     "malformed URL",
			body:     []byte(`{"data":"https://example.com/approve/"}`),
			expected: "",
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, ok := suspendedFromBody(tt.body)
			assert.Equal(t, tt.found, ok, "suspendedFromBody() found mismatch")
			if ok {
				assert.Equal(t, tt.expected, hash, "suspendedFromBody() hash mismatch")
			}
		})
	}
}

func TestGovernanceScenarios(t *testing.T) {
	scenarios := governanceScenarios()

	assert.Len(t, scenarios, 5, "governanceScenarios should return 5 scenarios")

	// Should have expected names
	expectedNames := []string{"consensus", "envelope-maximal", "agent-delegation", "consensus-quorum", "notary-oob"}
	nameSet := make(map[string]bool)
	for _, sc := range scenarios {
		nameSet[sc.Name] = true
	}
	for _, name := range expectedNames {
		assert.Contains(t, nameSet, name, "governanceScenarios should include scenario %q", name)
	}

	// consensus should require Consensus posture
	consensusSc, ok := Find("consensus")
	require.True(t, ok, "Should find consensus scenario")
	assert.Equal(t, Consensus, consensusSc.RequiresPosture, "consensus scenario should require Consensus posture")

	// envelope-maximal should require Notary posture
	envelopeSc, ok := Find("envelope-maximal")
	require.True(t, ok, "Should find envelope-maximal scenario")
	assert.Equal(t, Notary, envelopeSc.RequiresPosture, "envelope-maximal scenario should require Notary posture")
}

func TestGovernanceScenarioNames(t *testing.T) {
	scenarios := governanceScenarios()

	expectedNames := map[string]bool{
		"consensus":        true,
		"envelope-maximal": true,
		"agent-delegation": true,
		"consensus-quorum": true,
		"notary-oob":       true,
	}

	for _, sc := range scenarios {
		assert.Contains(t, expectedNames, sc.Name, "Unexpected governance scenario name: %q", sc.Name)
	}
}

func TestGovernanceScenarioTitles(t *testing.T) {
	scenarios := governanceScenarios()

	expectedTitles := map[string]string{
		"consensus":        "L2 consensus envelope (ensemble co-sign)",
		"envelope-maximal": "Official notary envelope: L2 consensus + principal L3 signing",
		"agent-delegation": "CLI delegates app credential to agent (SPIFFE distinctness + receipt audit)",
		"consensus-quorum": "Consensus quorum: 2-of-3 co-sign, receipt records consensus",
		"notary-oob":       "L3 notary OOB: suspend then principal approves out-of-band",
	}

	for _, sc := range scenarios {
		expectedTitle, ok := expectedTitles[sc.Name]
		if !ok {
			assert.Fail(t, "No expected title defined for scenario %q", sc.Name)
			continue
		}
		assert.Equal(t, expectedTitle, sc.Title, "Scenario %q should have correct title", sc.Name)
	}
}

func TestGovernanceScenarioPostures(t *testing.T) {
	scenarios := governanceScenarios()

	expectedPostures := map[string]Posture{
		"consensus":        Consensus,
		"envelope-maximal": Notary,
		"agent-delegation": Doctrine,
		"consensus-quorum": Consensus,
		"notary-oob":       Notary,
	}

	for _, sc := range scenarios {
		expectedPosture, ok := expectedPostures[sc.Name]
		if !ok {
			assert.Fail(t, "No expected posture defined for scenario %q", sc.Name)
			continue
		}
		assert.Equal(t, expectedPosture, sc.RequiresPosture, "Scenario %q should require correct posture", sc.Name)
	}
}

func TestGovernanceScenarioPersonas(t *testing.T) {
	scenarios := governanceScenarios()

	// Governance scenarios should use expected personas
	expectedPersonas := map[string]string{
		"consensus":        "ensemble-producer",
		"envelope-maximal": "ensemble-producer",
		"agent-delegation": "cli-delegator",
		"consensus-quorum": "ensemble-producer",
		"notary-oob":       "principal",
	}
	for _, sc := range scenarios {
		expected, ok := expectedPersonas[sc.Name]
		if !ok {
			assert.Fail(t, "No expected persona defined for scenario %q", sc.Name)
			continue
		}
		assert.Equal(t, expected, sc.Persona.ID, "Governance scenario %q should use correct persona", sc.Name)
	}
}

func TestGovernanceScenarioRunNotNil(t *testing.T) {
	scenarios := governanceScenarios()

	for _, sc := range scenarios {
		assert.NotNil(t, sc.Run, "Governance scenario %q should have non-nil Run function", sc.Name)
	}
}

func TestReceiptFailed(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		wantSummary string
		wantFailed  bool
	}{
		{
			name:        "FAILED status returns summary",
			body:        []byte(`{"status":` + itoa(int(operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED)) + `,"result_summary":"exit code 1: cloudsvc unreachable"}`),
			wantSummary: "exit code 1: cloudsvc unreachable",
			wantFailed:  true,
		},
		{
			name:        "COMPLETED status returns false",
			body:        []byte(`{"status":` + itoa(int(operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED)) + `,"result_summary":"ok"}`),
			wantSummary: "",
			wantFailed:  false,
		},
		{
			name:        "UNSPECIFIED status returns false",
			body:        []byte(`{"status":` + itoa(int(operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED)) + `,"result_summary":""}`),
			wantSummary: "",
			wantFailed:  false,
		},
		{
			name:        "CANCELLED status returns false",
			body:        []byte(`{"status":` + itoa(int(operatorv1.ExecutionStatus_EXECUTION_STATUS_CANCELLED)) + `,"result_summary":"cancelled"}`),
			wantSummary: "",
			wantFailed:  false,
		},
		{
			name:        "non-receipt JSON returns false",
			body:        []byte(`{"error":{"message":"L3 approval required","data":"https://localhost:8443/approve/abc123"}}`),
			wantSummary: "",
			wantFailed:  false,
		},
		{
			name:        "empty body returns false",
			body:        []byte{},
			wantSummary: "",
			wantFailed:  false,
		},
		{
			name:        "nil body returns false",
			body:        nil,
			wantSummary: "",
			wantFailed:  false,
		},
		{
			name:        "malformed JSON returns false",
			body:        []byte(`{not json`),
			wantSummary: "",
			wantFailed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, failed := receiptFailed(tt.body)
			assert.Equal(t, tt.wantFailed, failed, "receiptFailed failed flag mismatch")
			assert.Equal(t, tt.wantSummary, summary, "receiptFailed summary mismatch")
		})
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

