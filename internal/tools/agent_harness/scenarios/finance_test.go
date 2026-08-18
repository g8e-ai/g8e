// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"testing"
)

func TestFinanceScenarios(t *testing.T) {
	scenarios := financeScenarios()

	if len(scenarios) != 1 {
		t.Errorf("financeScenarios should return 1 scenario, got %d", len(scenarios))
	}

	expectedNames := map[string]bool{
		"finance-unauthorized-trade": true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected finance scenario name: %q", sc.Name)
		}
		if sc.Run == nil {
			t.Errorf("finance scenario %q should have non-nil Run function", sc.Name)
		}
		if sc.RequiresPosture != Doctrine {
			t.Errorf("finance scenario %q should require Doctrine posture, got %q", sc.Name, sc.RequiresPosture)
		}
	}
}

func TestFinanceScenarioTitles(t *testing.T) {
	scenarios := financeScenarios()

	expectedTitles := map[string]string{
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

func TestFinanceScenarioPersonas(t *testing.T) {
	scenarios := financeScenarios()

	expectedPersonas := map[string]string{
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
			t.Errorf("finance scenario %q should have non-empty UserAgent", sc.Name)
		}
	}
}

func TestFinanceScenariosInRegistry(t *testing.T) {
	sc, ok := Find("finance-unauthorized-trade")
	if !ok {
		t.Fatal("Registry should include finance-unauthorized-trade scenario")
	}
	if sc.RequiresPosture != Doctrine {
		t.Errorf("finance-unauthorized-trade should require Doctrine posture, got %q", sc.RequiresPosture)
	}
}
