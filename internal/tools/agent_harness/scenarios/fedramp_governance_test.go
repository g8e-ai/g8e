// Copyright (c) 2026 Lateralus Labs, LLC.

package scenarios

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFedRAMPScenarios(t *testing.T) {
	scenarios := fedrampScenarios()

	assert.Len(t, scenarios, 5, "fedrampScenarios should return 5 scenarios")

	expectedNames := map[string]bool{
		"fedramp-provision":      true,
		"fedramp-deny":           true,
		"fedramp-escalate":       true,
		"fedramp-revert":         true,
		"fedramp-evidence-block": true,
	}

	for _, sc := range scenarios {
		assert.Contains(t, expectedNames, sc.Name, "Unexpected fedramp scenario name")
		assert.NotNil(t, sc.Run, "fedramp scenario %q should have non-nil Run function", sc.Name)
		delete(expectedNames, sc.Name)
	}
	for name := range expectedNames {
		assert.Fail(t, "Missing expected fedramp scenario", name)
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
			assert.Fail(t, "No expected title for scenario", sc.Name)
			continue
		}
		assert.Equal(t, expected, sc.Title, "Scenario %q should have correct title", sc.Name)
		delete(expectedTitles, sc.Name)
	}
	for name := range expectedTitles {
		assert.Fail(t, "Missing expected fedramp scenario title", name)
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
			assert.Fail(t, "No expected posture for scenario", sc.Name)
			continue
		}
		assert.Equal(t, expected, sc.RequiresPosture, "Scenario %q should require correct posture", sc.Name)
		delete(expectedPostures, sc.Name)
	}
	for name := range expectedPostures {
		assert.Fail(t, "Missing expected fedramp scenario posture", name)
	}
}

func TestFedRAMPScenarioPersonas(t *testing.T) {
	scenarios := fedrampScenarios()

	for _, sc := range scenarios {
		assert.NotEmpty(t, sc.Persona.ID, "fedramp scenario %q should have non-empty Persona.ID", sc.Name)
		assert.NotEmpty(t, sc.Persona.UserAgent, "fedramp scenario %q should have non-empty Persona.UserAgent", sc.Name)
	}

	var escalateScenario *Scenario
	for i := range scenarios {
		if scenarios[i].Name == "fedramp-escalate" {
			escalateScenario = &scenarios[i]
			break
		}
	}
	require.NotNil(t, escalateScenario, "fedramp-escalate scenario not found")
	assert.Equal(t, "fedramp-authorizing-official", escalateScenario.Persona.ID, "fedramp-escalate should use persona 'fedramp-authorizing-official'")

	for _, sc := range scenarios {
		if sc.Name == "fedramp-escalate" {
			continue
		}
		assert.Equal(t, "fedramp-cloud-operator", sc.Persona.ID, "fedramp scenario %q should use persona 'fedramp-cloud-operator'", sc.Name)
	}
}

func TestFedRAMPArgsDefaults(t *testing.T) {
	assert.Equal(t, "10.73.0.50:9100", FedRAMPArgs.CloudSvcEndpoint, "Default CloudSvcEndpoint should be 10.73.0.50:9100")
}

func TestCloudopArgs(t *testing.T) {
	got := cloudopArgs("provision", "fedramp-vm-prod-01", "FIPS-199-MODERATE")
	assert.Contains(t, got, "cloudop", "cloudopArgs should contain 'cloudop' command")
	assert.Contains(t, got, "provision", "cloudopArgs should contain the action 'provision'")
	assert.Contains(t, got, "fedramp-vm-prod-01", "cloudopArgs should contain the resource ID")
	assert.Contains(t, got, "FIPS-199-MODERATE", "cloudopArgs should contain the detail")
	assert.Contains(t, got, "10.73.0.50:9100", "cloudopArgs should contain the CloudSvcEndpoint")
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
			assert.Fail(t, "Registry should include scenario", name)
			continue
		}
		assert.NotNil(t, sc.Run, "Registry scenario %q should have non-nil Run function", name)
	}
}

func TestFedRAMPScenarioPrefix(t *testing.T) {
	scenarios := fedrampScenarios()
	for _, sc := range scenarios {
		assert.True(t, strings.HasPrefix(sc.Name, FedRAMPScenarioPrefix), "fedramp scenario %q should start with %q prefix", sc.Name, FedRAMPScenarioPrefix)
	}
}

func TestFedRAMPProvisionRequiresConsensus(t *testing.T) {
	sc, ok := Find("fedramp-provision")
	require.True(t, ok, "fedramp-provision scenario not found in registry")
	assert.Equal(t, Consensus, sc.RequiresPosture, "fedramp-provision should require Consensus posture")
}

func TestFedRAMPArgsStructFields(t *testing.T) {
	assert.NotEmpty(t, FedRAMPArgs.CloudSvcEndpoint, "FedRAMPArgs.CloudSvcEndpoint should not be empty")
	assert.Contains(t, FedRAMPArgs.CloudSvcEndpoint, ":", "FedRAMPArgs.CloudSvcEndpoint should contain host:port")
}
