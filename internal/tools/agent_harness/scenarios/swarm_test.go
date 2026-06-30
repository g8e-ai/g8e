// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"testing"
)

func TestSwarmScenarios(t *testing.T) {
	scenarios := swarmScenarios()

	if len(scenarios) != 3 {
		t.Errorf("swarmScenarios should return 3 scenarios, got %d", len(scenarios))
	}

	expectedNames := map[string]bool{
		"swarm-recon-mission":             true,
		"swarm-weapon-release-block":      true,
		"swarm-restricted-airspace-block": true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected swarm scenario name: %q", sc.Name)
		}
		if sc.Run == nil {
			t.Errorf("swarm scenario %q should have non-nil Run function", sc.Name)
		}
	}
}

func TestSwarmScenarioTitles(t *testing.T) {
	scenarios := swarmScenarios()

	expectedTitles := map[string]string{
		"swarm-recon-mission":             "Swarm: authorized recon mission with governed drone deployment",
		"swarm-weapon-release-block":      "Swarm: unauthorized weapon release blocked by L1 doctrine",
		"swarm-restricted-airspace-block": "Swarm: restricted airspace navigation blocked by L1 doctrine",
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

func TestSwarmScenarioPersonas(t *testing.T) {
	scenarios := swarmScenarios()

	for _, sc := range scenarios {
		if sc.Persona.ID != "swarm-command-agent" {
			t.Errorf("swarm scenario %q should use persona 'swarm-command-agent', got %q", sc.Name, sc.Persona.ID)
		}
		if sc.Persona.UserAgent == "" {
			t.Errorf("swarm scenario %q should have non-empty UserAgent", sc.Name)
		}
	}
}

func TestSwarmScenarioPostures(t *testing.T) {
	scenarios := swarmScenarios()

	expectedPostures := map[string]Posture{
		"swarm-recon-mission":             Consensus,
		"swarm-weapon-release-block":      Doctrine,
		"swarm-restricted-airspace-block": Doctrine,
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

func TestSwarmScenariosInRegistry(t *testing.T) {
	sc, ok := Find("swarm-recon-mission")
	if !ok {
		t.Fatal("Registry should include swarm-recon-mission scenario")
	}
	if sc.RequiresPosture != Consensus {
		t.Errorf("swarm-recon-mission should require Consensus posture, got %q", sc.RequiresPosture)
	}

	sc, ok = Find("swarm-weapon-release-block")
	if !ok {
		t.Fatal("Registry should include swarm-weapon-release-block scenario")
	}
	if sc.RequiresPosture != Doctrine {
		t.Errorf("swarm-weapon-release-block should require Doctrine posture, got %q", sc.RequiresPosture)
	}

	sc, ok = Find("swarm-restricted-airspace-block")
	if !ok {
		t.Fatal("Registry should include swarm-restricted-airspace-block scenario")
	}
	if sc.RequiresPosture != Doctrine {
		t.Errorf("swarm-restricted-airspace-block should require Doctrine posture, got %q", sc.RequiresPosture)
	}
}
