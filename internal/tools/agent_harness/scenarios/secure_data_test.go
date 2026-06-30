// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"testing"
)

func TestSecureDataScenarios(t *testing.T) {
	scenarios := secureDataScenarios()

	if len(scenarios) != 3 {
		t.Errorf("secureDataScenarios should return 3 scenarios, got %d", len(scenarios))
	}

	expectedNames := map[string]bool{
		"secure-data-migration":      true,
		"secure-data-bypass-attempt": true,
		"secure-data-cross-tenant":   true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected secure-data scenario name: %q", sc.Name)
		}
		if sc.Run == nil {
			t.Errorf("secure-data scenario %q should have non-nil Run function", sc.Name)
		}
	}
}

func TestSecureDataScenarioTitles(t *testing.T) {
	scenarios := secureDataScenarios()

	expectedTitles := map[string]string{
		"secure-data-migration":      "Governed migration with chain-of-custody receipt",
		"secure-data-bypass-attempt": "Direct transfer without GovernanceEnvelope blocked by doctrine",
		"secure-data-cross-tenant":   "Cross-tenant leak attempt rejected by doctrine",
	}

	for _, sc := range scenarios {
		expected, ok := expectedTitles[sc.Name]
		if !ok {
			t.Errorf("No expected title for scenario %q", sc.Name)
			continue
		}
		if sc.Title != expected {
			t.Errorf("Scenario %q should have title %q, got %q", sc.Name, expected, sc.Title)
		}
	}
}

func TestSecureDataScenarioPersonas(t *testing.T) {
	scenarios := secureDataScenarios()

	for _, sc := range scenarios {
		if sc.Persona.ID != "migration-agent" {
			t.Errorf("secure-data scenario %q should use persona 'migration-agent', got %q", sc.Name, sc.Persona.ID)
		}
		if sc.Persona.UserAgent == "" {
			t.Errorf("secure-data scenario %q should have non-empty UserAgent", sc.Name)
		}
	}
}

func TestSecureDataScenarioPostures(t *testing.T) {
	scenarios := secureDataScenarios()

	expectedPostures := map[string]Posture{
		"secure-data-migration":      Consensus,
		"secure-data-bypass-attempt": Doctrine,
		"secure-data-cross-tenant":   Doctrine,
	}

	for _, sc := range scenarios {
		expected, ok := expectedPostures[sc.Name]
		if !ok {
			t.Errorf("No expected posture for scenario %q", sc.Name)
			continue
		}
		if sc.RequiresPosture != expected {
			t.Errorf("Scenario %q should require %q posture, got %q", sc.Name, expected, sc.RequiresPosture)
		}
	}
}

func TestSecureDataScenariosInRegistry(t *testing.T) {
	sc, ok := Find("secure-data-migration")
	if !ok {
		t.Fatal("Registry should include secure-data-migration scenario")
	}
	if sc.RequiresPosture != Consensus {
		t.Errorf("secure-data-migration should require Consensus posture, got %q", sc.RequiresPosture)
	}

	sc, ok = Find("secure-data-bypass-attempt")
	if !ok {
		t.Fatal("Registry should include secure-data-bypass-attempt scenario")
	}
	if sc.RequiresPosture != Doctrine {
		t.Errorf("secure-data-bypass-attempt should require Doctrine posture, got %q", sc.RequiresPosture)
	}

	sc, ok = Find("secure-data-cross-tenant")
	if !ok {
		t.Fatal("Registry should include secure-data-cross-tenant scenario")
	}
	if sc.RequiresPosture != Doctrine {
		t.Errorf("secure-data-cross-tenant should require Doctrine posture, got %q", sc.RequiresPosture)
	}
}
