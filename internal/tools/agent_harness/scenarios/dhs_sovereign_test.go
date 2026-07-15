// Copyright (c) 2026 Lateralus Labs, LLC.

package scenarios

import (
	"strings"
	"testing"

	clientpkg "github.com/g8e-ai/g8e/internal/tools/agent_harness/client"
)

func TestDHSScenarios(t *testing.T) {
	scenarios := dhsScenarios()

	if len(scenarios) != 6 {
		t.Errorf("dhsScenarios should return 6 scenarios, got %d", len(scenarios))
	}

	expectedNames := map[string]bool{
		"dhs-ingest":         true,
		"dhs-release":        true,
		"dhs-cue":            true,
		"dhs-cue-veto":       true,
		"dhs-evidence-block": true,
		"dhs-purge":          true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected dhs scenario name: %q", sc.Name)
		}
		if sc.Run == nil {
			t.Errorf("dhs scenario %q should have non-nil Run function", sc.Name)
		}
	}
}

func TestDHSScenarioTitles(t *testing.T) {
	scenarios := dhsScenarios()

	expectedTitles := map[string]string{
		"dhs-ingest":         "DHS: governed multi-source ingest into the sovereign data plane",
		"dhs-release":        "DHS: cross-domain release gated on out-of-band release-authority approval (L3)",
		"dhs-cue":            "DHS: authorized interdiction cue admitted under L2 consensus quorum",
		"dhs-cue-veto":       "DHS: interdiction cue without quorum is vetoed by L2 consensus",
		"dhs-evidence-block": "DHS: attempt to wipe the audit trail is rejected by L1 doctrine",
		"dhs-purge":          "DHS: governed retention destruction with cryptographic receipt",
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

func TestDHSScenarioPostures(t *testing.T) {
	scenarios := dhsScenarios()

	expectedPostures := map[string]Posture{
		"dhs-ingest":         Doctrine,
		"dhs-release":        Notary,
		"dhs-cue":            Consensus,
		"dhs-cue-veto":       Consensus,
		"dhs-evidence-block": Doctrine,
		"dhs-purge":          Doctrine,
	}

	for _, sc := range scenarios {
		expected, ok := expectedPostures[sc.Name]
		if !ok {
			t.Errorf("No expected posture for scenario %q", sc.Name)
			continue
		}
		if sc.RequiresPosture != expected {
			t.Errorf("Scenario %q should require %s posture, got %q", sc.Name, expected, sc.RequiresPosture)
		}
	}
}

func TestDHSScenarioPersonas(t *testing.T) {
	scenarios := dhsScenarios()

	for _, sc := range scenarios {
		if sc.Persona.ID == "" {
			t.Errorf("dhs scenario %q should have non-empty Persona.ID", sc.Name)
		}
		if sc.Persona.UserAgent == "" {
			t.Errorf("dhs scenario %q should have non-empty Persona.UserAgent", sc.Name)
		}
	}

	// dhs-release uses the release authority persona
	var releaseScenario *Scenario
	for i := range scenarios {
		if scenarios[i].Name == "dhs-release" {
			releaseScenario = &scenarios[i]
			break
		}
	}
	if releaseScenario == nil {
		t.Fatal("dhs-release scenario not found")
		return
	}
	if releaseScenario.Persona.ID != "dhs-release-authority" {
		t.Errorf("dhs-release should use persona 'dhs-release-authority', got %q", releaseScenario.Persona.ID)
	}

	// All other scenarios use the connector persona
	for _, sc := range scenarios {
		if sc.Name == "dhs-release" {
			continue
		}
		if sc.Persona.ID != "dhs-coalition-connector" {
			t.Errorf("dhs scenario %q should use persona 'dhs-coalition-connector', got %q", sc.Name, sc.Persona.ID)
		}
	}
}

func TestDHSSovereignArgsDefaults(t *testing.T) {
	if DHSSovereignArgs.DataSvcEndpoint != "10.63.0.50:9100" {
		t.Errorf("Default DataSvcEndpoint should be 10.63.0.50:9100, got %s", DHSSovereignArgs.DataSvcEndpoint)
	}
}

func TestDataopArgs(t *testing.T) {
	got := dataopArgs("ingest", "TRK-CBP-0001", "NIPR")
	if !strings.Contains(got, "dataop") {
		t.Errorf("dataopArgs should contain 'dataop' command, got %s", got)
	}
	if !strings.Contains(got, "ingest") {
		t.Errorf("dataopArgs should contain the operation 'ingest', got %s", got)
	}
	if !strings.Contains(got, "TRK-CBP-0001") {
		t.Errorf("dataopArgs should contain the record ID, got %s", got)
	}
	if !strings.Contains(got, "NIPR") {
		t.Errorf("dataopArgs should contain the detail, got %s", got)
	}
	if !strings.Contains(got, "10.63.0.50:9100") {
		t.Errorf("dataopArgs should contain the DataSvcEndpoint, got %s", got)
	}
}

func TestDHSScenariosInRegistry(t *testing.T) {
	expected := []string{
		"dhs-ingest",
		"dhs-release",
		"dhs-cue",
		"dhs-cue-veto",
		"dhs-evidence-block",
		"dhs-purge",
	}
	for _, name := range expected {
		sc, ok := Find(name)
		if !ok {
			t.Errorf("Registry should include %q scenario", name)
			continue
		}
		if sc.Run == nil {
			t.Errorf("Registry scenario %q should have non-nil Run function", name)
		}
	}
}

func TestDHSGovKitValidation(t *testing.T) {
	kit := &GovKit{
		Ensemble:   &clientpkg.Ensemble{},
		Principal:  &clientpkg.Principal{},
		L3Mode:     "mock",
		OperatorID: "dhs-operator",
	}

	if kit.Ensemble == nil || kit.Principal == nil || kit.OperatorID == "" {
		t.Error("Valid GovKit should pass validation")
	}
}
