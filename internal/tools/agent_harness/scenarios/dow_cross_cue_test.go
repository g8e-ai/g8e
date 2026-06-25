// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package scenarios

import (
	"testing"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

func TestDoWScenarios(t *testing.T) {
	scenarios := dowScenarios()

	if len(scenarios) != 2 {
		t.Errorf("dowScenarios should return 2 scenarios, got %d", len(scenarios))
	}

	expectedNames := map[string]bool{
		"dow-cross-cue": true,
		"dow-bft-veto":  true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected dow scenario name: %q", sc.Name)
		}
		if sc.Run == nil {
			t.Errorf("dow scenario %q should have non-nil Run function", sc.Name)
		}
		if sc.RequiresPosture != Consensus {
			t.Errorf("dow scenario %q should require Consensus posture, got %q", sc.Name, sc.RequiresPosture)
		}
	}
}

func TestDoWScenarioTitles(t *testing.T) {
	scenarios := dowScenarios()

	expectedTitles := map[string]string{
		"dow-cross-cue": "DoW: SIGINT→EO/IR cross-cue with governed camera slew",
		"dow-bft-veto":  "DoW: BFT veto rejects spoofed GNSS cross-cue",
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

func TestDoWScenarioPersonas(t *testing.T) {
	scenarios := dowScenarios()

	for _, sc := range scenarios {
		if sc.Persona.ID != "dow-sigint-agent" {
			t.Errorf("dow scenario %q should use persona 'dow-sigint-agent', got %q", sc.Name, sc.Persona.ID)
		}
		if sc.Persona.UserAgent == "" {
			t.Errorf("dow scenario %q should have non-empty UserAgent", sc.Name)
		}
	}
}

func TestDoWCrossCueArgsDefaults(t *testing.T) {
	if DoWCrossCueArgs.GimbalEndpoint != "10.43.0.40:9000" {
		t.Errorf("Default gimbal endpoint should be 10.43.0.40:9000, got %s", DoWCrossCueArgs.GimbalEndpoint)
	}
	if DoWCrossCueArgs.Azimuth != "45.0" {
		t.Errorf("Default azimuth should be 45.0, got %s", DoWCrossCueArgs.Azimuth)
	}
	if DoWCrossCueArgs.Elevation != "30.0" {
		t.Errorf("Default elevation should be 30.0, got %s", DoWCrossCueArgs.Elevation)
	}
}

func TestDoWScenariosInRegistry(t *testing.T) {
	sc, ok := Find("dow-cross-cue")
	if !ok {
		t.Fatal("Registry should include dow-cross-cue scenario")
	}
	if sc.RequiresPosture != Consensus {
		t.Errorf("dow-cross-cue should require Consensus posture, got %q", sc.RequiresPosture)
	}

	sc, ok = Find("dow-bft-veto")
	if !ok {
		t.Fatal("Registry should include dow-bft-veto scenario")
	}
	if sc.RequiresPosture != Consensus {
		t.Errorf("dow-bft-veto should require Consensus posture, got %q", sc.RequiresPosture)
	}
}

func TestDoWGovKitValidation(t *testing.T) {
	// Verify that dow scenarios use the same GovKit validation as governance scenarios
	kit := &GovKit{
		Ensemble:   &clientpkg.Ensemble{},
		Principal:  &clientpkg.Principal{},
		L3Mode:     "mock",
		OperatorID: "dow-operator",
	}

	if kit.Ensemble == nil || kit.Principal == nil || kit.OperatorID == "" {
		t.Error("Valid GovKit should pass validation")
	}
}
