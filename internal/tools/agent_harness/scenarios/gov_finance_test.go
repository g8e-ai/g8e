// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"testing"
)

func TestGovFinanceScenarios(t *testing.T) {
	scenarios := govFinanceScenarios()

	if len(scenarios) != 2 {
		t.Errorf("govFinanceScenarios should return 2 scenarios, got %d", len(scenarios))
	}

	expectedNames := map[string]bool{
		"gov-cui-exfil-block":       true,
		"finance-unauthorized-trade": true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected gov/finance scenario name: %q", sc.Name)
		}
		if sc.Run == nil {
			t.Errorf("gov/finance scenario %q should have non-nil Run function", sc.Name)
		}
		if sc.RequiresPosture != Doctrine {
			t.Errorf("gov/finance scenario %q should require Doctrine posture, got %q", sc.Name, sc.RequiresPosture)
		}
	}
}

func TestGovFinanceScenarioTitles(t *testing.T) {
	scenarios := govFinanceScenarios()

	expectedTitles := map[string]string{
		"gov-cui-exfil-block":       "CUI exfiltration blocked by L1 doctrine",
		"finance-unauthorized-trade": "Unauthorized trade blocked by L1 doctrine",
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

func TestGovFinanceScenarioPersonas(t *testing.T) {
	scenarios := govFinanceScenarios()

	expectedPersonas := map[string]string{
		"gov-cui-exfil-block":       "gov-agent",
		"finance-unauthorized-trade": "finance-agent",
	}

	for _, sc := range scenarios {
		expected, ok := expectedPersonas[sc.Name]
		if !ok {
			t.Errorf("No expected persona for scenario %q", sc.Name)
			continue
		}
		if sc.Persona.ID != expected {
			t.Errorf("Scenario %q should have persona %q, got %q", sc.Name, expected, sc.Persona.ID)
		}
		if sc.Persona.UserAgent == "" {
			t.Errorf("gov/finance scenario %q should have non-empty UserAgent", sc.Name)
		}
	}
}

func TestGovFinanceScenariosInRegistry(t *testing.T) {
	sc, ok := Find("gov-cui-exfil-block")
	if !ok {
		t.Fatal("Registry should include gov-cui-exfil-block scenario")
	}
	if sc.RequiresPosture != Doctrine {
		t.Errorf("gov-cui-exfil-block should require Doctrine posture, got %q", sc.RequiresPosture)
	}

	sc, ok = Find("finance-unauthorized-trade")
	if !ok {
		t.Fatal("Registry should include finance-unauthorized-trade scenario")
	}
	if sc.RequiresPosture != Doctrine {
		t.Errorf("finance-unauthorized-trade should require Doctrine posture, got %q", sc.RequiresPosture)
	}
}
