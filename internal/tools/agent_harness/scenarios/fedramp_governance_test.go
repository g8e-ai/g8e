// Copyright (c) 2026 Lateralus Labs, LLC.

package scenarios

import (
	"strings"
	"testing"
)

func TestFedRAMPScenarios(t *testing.T) {
	scenarios := fedrampScenarios()

	if len(scenarios) != 5 {
		t.Errorf("fedrampScenarios should return 5 scenarios, got %d", len(scenarios))
	}

	expectedNames := map[string]bool{
		"fedramp-provision":      true,
		"fedramp-deny":           true,
		"fedramp-escalate":       true,
		"fedramp-revert":         true,
		"fedramp-evidence-block": true,
	}

	for _, sc := range scenarios {
		if !expectedNames[sc.Name] {
			t.Errorf("Unexpected fedramp scenario name: %q", sc.Name)
		}
		if sc.Run == nil {
			t.Errorf("fedramp scenario %q should have non-nil Run function", sc.Name)
		}
	}
}

func TestFedRAMPScenarioTitles(t *testing.T) {
	scenarios := fedrampScenarios()

	expectedTitles := map[string]string{
		"fedramp-provision":      "FedRAMP: governed cloud resource provisioning with L2 consensus",
		"fedramp-deny":           "FedRAMP: unauthorized audit trail destruction blocked by L1 doctrine",
		"fedramp-escalate":       "FedRAMP: resource destruction gated on authorizing official approval (L3)",
		"fedramp-revert":         "FedRAMP: governed configuration revert under L2 consensus quorum",
		"fedramp-evidence-block": "FedRAMP: attempt to wipe the gateway audit vault is rejected by L1 doctrine",
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

func TestFedRAMPScenarioPostures(t *testing.T) {
	scenarios := fedrampScenarios()

	expectedPostures := map[string]Posture{
		"fedramp-provision":      Consensus,
		"fedramp-deny":           Doctrine,
		"fedramp-escalate":       Notary,
		"fedramp-revert":         Consensus,
		"fedramp-evidence-block": Doctrine,
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

func TestFedRAMPScenarioPersonas(t *testing.T) {
	scenarios := fedrampScenarios()

	for _, sc := range scenarios {
		if sc.Persona.ID == "" {
			t.Errorf("fedramp scenario %q should have non-empty Persona.ID", sc.Name)
		}
		if sc.Persona.UserAgent == "" {
			t.Errorf("fedramp scenario %q should have non-empty Persona.UserAgent", sc.Name)
		}
	}

	var escalateScenario *Scenario
	for i := range scenarios {
		if scenarios[i].Name == "fedramp-escalate" {
			escalateScenario = &scenarios[i]
			break
		}
	}
	if escalateScenario == nil {
		t.Fatal("fedramp-escalate scenario not found")
		return
	}
	if escalateScenario.Persona.ID != "fedramp-authorizing-official" {
		t.Errorf("fedramp-escalate should use persona 'fedramp-authorizing-official', got %q", escalateScenario.Persona.ID)
	}

	for _, sc := range scenarios {
		if sc.Name == "fedramp-escalate" {
			continue
		}
		if sc.Persona.ID != "fedramp-cloud-operator" {
			t.Errorf("fedramp scenario %q should use persona 'fedramp-cloud-operator', got %q", sc.Name, sc.Persona.ID)
		}
	}
}

func TestFedRAMPArgsDefaults(t *testing.T) {
	if FedRAMPArgs.CloudSvcEndpoint != "10.73.0.50:9100" {
		t.Errorf("Default CloudSvcEndpoint should be 10.73.0.50:9100, got %s", FedRAMPArgs.CloudSvcEndpoint)
	}
}

func TestCloudopArgs(t *testing.T) {
	got := cloudopArgs("provision", "fedramp-vm-prod-01", "FIPS-199-MODERATE")
	if !strings.Contains(got, "cloudop") {
		t.Errorf("cloudopArgs should contain 'cloudop' command, got %s", got)
	}
	if !strings.Contains(got, "provision") {
		t.Errorf("cloudopArgs should contain the action 'provision', got %s", got)
	}
	if !strings.Contains(got, "fedramp-vm-prod-01") {
		t.Errorf("cloudopArgs should contain the resource ID, got %s", got)
	}
	if !strings.Contains(got, "FIPS-199-MODERATE") {
		t.Errorf("cloudopArgs should contain the detail, got %s", got)
	}
	if !strings.Contains(got, "10.73.0.50:9100") {
		t.Errorf("cloudopArgs should contain the CloudSvcEndpoint, got %s", got)
	}
}

func TestFedRAMPScenariosInRegistry(t *testing.T) {
	expected := []string{
		"fedramp-provision",
		"fedramp-deny",
		"fedramp-escalate",
		"fedramp-revert",
		"fedramp-evidence-block",
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

func TestFedRAMPScenarioPrefix(t *testing.T) {
	scenarios := fedrampScenarios()
	for _, sc := range scenarios {
		if !strings.HasPrefix(sc.Name, FedRAMPScenarioPrefix) {
			t.Errorf("fedramp scenario %q should start with %q prefix", sc.Name, FedRAMPScenarioPrefix)
		}
	}
}

func TestFedRAMPProvisionRequiresConsensus(t *testing.T) {
	sc, ok := Find("fedramp-provision")
	if !ok {
		t.Fatal("fedramp-provision scenario not found in registry")
	}
	if sc.RequiresPosture != Consensus {
		t.Errorf("fedramp-provision should require Consensus posture, got %s", sc.RequiresPosture)
	}
}

func TestFedRAMPArgsStructFields(t *testing.T) {
	if FedRAMPArgs.CloudSvcEndpoint == "" {
		t.Error("FedRAMPArgs.CloudSvcEndpoint should not be empty")
	}
	if !strings.Contains(FedRAMPArgs.CloudSvcEndpoint, ":") {
		t.Errorf("FedRAMPArgs.CloudSvcEndpoint should contain host:port, got %q", FedRAMPArgs.CloudSvcEndpoint)
	}
}
