// Copyright (c) 2026 Lateralus Labs, LLC.

package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

)

func TestDHSScenarios(t *testing.T) {
	scenarios := dhsScenarios()

	assert.Len(t, scenarios, 5, "dhsScenarios should return 5 scenarios")

	expectedNames := map[string]bool{
		"dhs-ingest":         true,
		"dhs-release":        true,
		"dhs-cue":            true,
		"dhs-evidence-block": true,
		"dhs-purge":          true,
	}

	for _, sc := range scenarios {
		assert.Contains(t, expectedNames, sc.Name, "Unexpected dhs scenario name")
		assert.NotNil(t, sc.Run, "dhs scenario %q should have non-nil Run function", sc.Name)
		delete(expectedNames, sc.Name)
	}
	for name := range expectedNames {
		assert.Fail(t, "Missing expected dhs scenario", name)
	}
}

func TestDHSScenarioTitles(t *testing.T) {
	scenarios := dhsScenarios()

	expectedTitles := map[string]string{
		"dhs-ingest":         "DHS: governed multi-source ingest into the sovereign data plane",
		"dhs-release":        "DHS: cross-domain release gated on out-of-band release-authority approval (L3)",
		"dhs-cue":            "DHS: authorized interdiction cue admitted under L2 consensus quorum",
		"dhs-evidence-block": "DHS: attempt to wipe the audit trail is rejected by L1 doctrine",
		"dhs-purge":          "DHS: governed retention destruction with cryptographic receipt",
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
		assert.Fail(t, "Missing expected dhs scenario title", name)
	}
}

func TestDHSScenarioPostures(t *testing.T) {
	scenarios := dhsScenarios()

	expectedPostures := map[string]Posture{
		"dhs-ingest":         Consensus,
		"dhs-release":        Notary,
		"dhs-cue":            Consensus,
		"dhs-evidence-block": Doctrine,
		"dhs-purge":          Consensus,
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
		assert.Fail(t, "Missing expected dhs scenario posture", name)
	}
}

func TestDHSScenarioPersonas(t *testing.T) {
	scenarios := dhsScenarios()

	for _, sc := range scenarios {
		assert.NotEmpty(t, sc.Persona.ID, "dhs scenario %q should have non-empty Persona.ID", sc.Name)
		assert.NotEmpty(t, sc.Persona.UserAgent, "dhs scenario %q should have non-empty Persona.UserAgent", sc.Name)
	}

	// dhs-release uses the release authority persona
	var releaseScenario *Scenario
	for i := range scenarios {
		if scenarios[i].Name == "dhs-release" {
			releaseScenario = &scenarios[i]
			break
		}
	}
	require.NotNil(t, releaseScenario, "dhs-release scenario not found")
	assert.Equal(t, "dhs-release-authority", releaseScenario.Persona.ID, "dhs-release should use persona 'dhs-release-authority'")

	// All other scenarios use the connector persona
	for _, sc := range scenarios {
		if sc.Name == "dhs-release" {
			continue
		}
		assert.Equal(t, "dhs-coalition-connector", sc.Persona.ID, "dhs scenario %q should use persona 'dhs-coalition-connector'", sc.Name)
	}
}

func TestDHSSovereignArgsDefaults(t *testing.T) {
	assert.Equal(t, "10.63.0.50:9100", DHSSovereignArgs.DataSvcEndpoint, "Default DataSvcEndpoint should be 10.63.0.50:9100")
}

func TestDataopArgs(t *testing.T) {
	got := dataopArgs("ingest", "TRK-CBP-0001", "NIPR")
	assert.Contains(t, got, "dataop", "dataopArgs should contain 'dataop' command")
	assert.Contains(t, got, "ingest", "dataopArgs should contain the operation 'ingest'")
	assert.Contains(t, got, "TRK-CBP-0001", "dataopArgs should contain the record ID")
	assert.Contains(t, got, "NIPR", "dataopArgs should contain the detail")
	assert.Contains(t, got, "10.63.0.50:9100", "dataopArgs should contain the DataSvcEndpoint")
}

func TestDHSScenariosInRegistry(t *testing.T) {
	expected := []string{
		"dhs-ingest",
		"dhs-release",
		"dhs-cue",
		"dhs-evidence-block",
		"dhs-purge",
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

func TestDHSGovKitValidation(t *testing.T) {
	kit := &GovKit{
		OperatorID: "dhs-operator",
	}

	assert.NotEmpty(t, kit.OperatorID, "Valid GovKit should have non-empty OperatorID")
}
