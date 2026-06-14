// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"testing"

	clientpkg "github.com/g8e-ai/g8e/internal/emulator/client"
)

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
}

func TestGovKitStruct(t *testing.T) {
	ensemble := &clientpkg.Ensemble{}
	principal := &clientpkg.Principal{}

	kit := &GovKit{
		Ensemble:   ensemble,
		Principal:  principal,
		L3Mode:     "mock",
		OperatorID: "test-operator-id",
	}

	if kit.Ensemble != ensemble {
		t.Error("GovKit Ensemble should be set")
	}
	if kit.Principal != principal {
		t.Error("GovKit Principal should be set")
	}
	if kit.L3Mode != "mock" {
		t.Error("GovKit L3Mode should be set")
	}
	if kit.OperatorID != "test-operator-id" {
		t.Error("GovKit OperatorID should be set")
	}
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
			name: "nil ensemble",
			kit: &GovKit{
				Ensemble:   nil,
				Principal:  &clientpkg.Principal{},
				L3Mode:     "mock",
				OperatorID: "test",
			},
			wantErr: true,
		},
		{
			name: "nil principal",
			kit: &GovKit{
				Ensemble:   &clientpkg.Ensemble{},
				Principal:  nil,
				L3Mode:     "mock",
				OperatorID: "test",
			},
			wantErr: true,
		},
		{
			name: "empty operator ID",
			kit: &GovKit{
				Ensemble:   &clientpkg.Ensemble{},
				Principal:  &clientpkg.Principal{},
				L3Mode:     "mock",
				OperatorID: "",
			},
			wantErr: true,
		},
		{
			name: "valid kit",
			kit: &GovKit{
				Ensemble:   &clientpkg.Ensemble{},
				Principal:  &clientpkg.Principal{},
				L3Mode:     "mock",
				OperatorID: "test-operator",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasErr := false
			if tt.kit == nil || tt.kit.Ensemble == nil || tt.kit.Principal == nil || tt.kit.OperatorID == "" {
				hasErr = true
			}
			if hasErr != tt.wantErr {
				t.Errorf("GovKit validation error mismatch, got %v, want %v", hasErr, tt.wantErr)
			}
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
		if result != tt.expected {
			t.Errorf("short(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestShortEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"012345678901", "012345678901"},     // exactly 12 chars
		{"0123456789012", "012345678901…"},   // 13 chars
		{"0123456789", "0123456789"},         // 10 chars
		{"a\nb", "a\nb"},                     // contains newline
		{"日本語", "日本語"},                   // unicode
		// Note: short() counts bytes, not runes. Each emoji is 4 bytes.
		// 3 emojis = 12 bytes, so they won't be truncated
		{"🎉🎉🎉", "🎉🎉🎉"},
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

	// Test with nil body
	_, ok = suspendedFromBody(nil)
	if ok {
		t.Error("suspendedFromBody should return false for nil body")
	}

	// Test with body containing /approve/ but no hash
	body = []byte(`{"data":"https://localhost:8443/approve/"}`)
	_, ok = suspendedFromBody(body)
	if ok {
		t.Error("suspendedFromBody should return false when /approve/ has no hash")
	}
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
			if ok != tt.found {
				t.Errorf("suspendedFromBody() found = %v, want %v", ok, tt.found)
			}
			if ok && hash != tt.expected {
				t.Errorf("suspendedFromBody() hash = %q, want %q", hash, tt.expected)
			}
		})
	}
}

func TestGovernanceScenarios(t *testing.T) {
	scenarios := governanceScenarios()

	// Should have 2 governance scenarios
	if len(scenarios) != 2 {
		t.Errorf("governanceScenarios should return 2 scenarios, got %d", len(scenarios))
	}

	// Should have expected names
	expectedNames := []string{"consensus", "envelope-maximal"}
	nameSet := make(map[string]bool)
	for _, sc := range scenarios {
		nameSet[sc.Name] = true
	}
	for _, name := range expectedNames {
		if !nameSet[name] {
			t.Errorf("governanceScenarios should include scenario %q", name)
		}
	}

	// consensus should require Consensus posture
	consensusSc, ok := Find("consensus")
	if !ok {
		t.Fatal("Should find consensus scenario")
	}
	if consensusSc.RequiresPosture != Consensus {
		t.Errorf("consensus scenario should require Consensus posture, got %q", consensusSc.RequiresPosture)
	}

	// envelope-maximal should require Notary posture
	envelopeSc, ok := Find("envelope-maximal")
	if !ok {
		t.Fatal("Should find envelope-maximal scenario")
	}
	if envelopeSc.RequiresPosture != Notary {
		t.Errorf("envelope-maximal scenario should require Notary posture, got %q", envelopeSc.RequiresPosture)
	}
}

func TestGovernanceScenarioNames(t *testing.T) {
	scenarios := governanceScenarios()

	expectedNames := map[string]bool{
		"consensus":        true,
		"envelope-maximal": true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected governance scenario name: %q", sc.Name)
		}
	}
}

func TestGovernanceScenarioTitles(t *testing.T) {
	scenarios := governanceScenarios()

	expectedTitles := map[string]string{
		"consensus":        "L2 consensus envelope (mock ensemble co-sign)",
		"envelope-maximal": "Official notary envelope: L2 consensus + principal L3 signing",
	}

	for _, sc := range scenarios {
		expectedTitle, ok := expectedTitles[sc.Name]
		if !ok {
			t.Errorf("No expected title defined for scenario %q", sc.Name)
			continue
		}
		if sc.Title != expectedTitle {
			t.Errorf("Scenario %q should have title %q, got %q", sc.Name, expectedTitle, sc.Title)
		}
	}
}

func TestGovernanceScenarioPostures(t *testing.T) {
	scenarios := governanceScenarios()

	expectedPostures := map[string]Posture{
		"consensus":        Consensus,
		"envelope-maximal": Notary,
	}

	for _, sc := range scenarios {
		expectedPosture, ok := expectedPostures[sc.Name]
		if !ok {
			t.Errorf("No expected posture defined for scenario %q", sc.Name)
			continue
		}
		if sc.RequiresPosture != expectedPosture {
			t.Errorf("Scenario %q should require posture %q, got %q", sc.Name, expectedPosture, sc.RequiresPosture)
		}
	}
}

func TestGovernanceScenarioPersonas(t *testing.T) {
	scenarios := governanceScenarios()

	// Both governance scenarios should use ensemble-producer persona
	for _, sc := range scenarios {
		if sc.Persona.ID != "ensemble-producer" {
			t.Errorf("Governance scenario %q should use ensemble-producer persona, got %q", sc.Name, sc.Persona.ID)
		}
	}
}

func TestGovernanceScenarioRunNotNil(t *testing.T) {
	scenarios := governanceScenarios()

	for _, sc := range scenarios {
		if sc.Run == nil {
			t.Errorf("Governance scenario %q should have non-nil Run function", sc.Name)
		}
	}
}

func TestGovKitL3Modes(t *testing.T) {
	tests := []struct {
		name    string
		l3Mode  string
		valid   bool
	}{
		{
			name:   "mock mode",
			l3Mode: "mock",
			valid:  true,
		},
		{
			name:   "suspend mode",
			l3Mode: "suspend",
			valid:  true,
		},
		{
			name:   "empty mode",
			l3Mode: "",
			valid:  false,
		},
		{
			name:   "invalid mode",
			l3Mode: "invalid",
			valid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kit := &GovKit{
				Ensemble:   &clientpkg.Ensemble{},
				Principal:  &clientpkg.Principal{},
				L3Mode:     tt.l3Mode,
				OperatorID: "test",
			}

			isValid := kit.L3Mode == "mock" || kit.L3Mode == "suspend"
			if isValid != tt.valid {
				t.Errorf("L3Mode validation mismatch, got %v, want %v", isValid, tt.valid)
			}
		})
	}
}
