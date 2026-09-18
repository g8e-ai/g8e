// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"sort"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ScenarioBlueprint is the authoring record for one frozen scenario.
type ScenarioBlueprint struct {
	ScenarioID                  string
	ScenarioVersion             string
	Category                    evalv1.EvaluationScenarioCategory
	PublicDescription           string
	GradingMethod               evalv1.EvaluationGradingMethod
	AllowedTools                []string
	ExpectedTools               []string
	ForbiddenTools              []string
	RequiredConcepts            []string
	TinyTask                    bool
	RequiresToolDecision        bool
	RequiresGovernedAction      bool
	ExpectsFailureOrUnavailable bool
	Input                       ScenarioInputFixture
	Gold                        ScenarioGoldCriteria
}

// ScenarioArtifacts holds the canonical fixture bodies for one scenario.
type ScenarioArtifacts struct {
	Input ScenarioArtifactPair
	Gold  ScenarioArtifactPair
}

// BuildScenarioCatalog materializes the frozen 25-scenario catalog and
// its content-addressed fixture artifacts.
func BuildScenarioCatalog() (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts, error) {
	blueprints := scenarioBlueprints()
	artifacts := make(map[string]ScenarioArtifacts, len(blueprints))
	scenarios := make([]*evalv1.EvaluationScenarioDefinition, 0, len(blueprints))
	for _, blueprint := range blueprints {
		scenario, pair, err := materializeScenarioBlueprint(blueprint)
		if err != nil {
			return nil, nil, fmt.Errorf("evaluation: build scenario catalog: scenario %s: %w", blueprint.ScenarioID, err)
		}
		artifacts[blueprint.ScenarioID] = pair
		scenarios = append(scenarios, scenario)
	}
	sort.Slice(scenarios, func(i, j int) bool {
		return scenarios[i].GetScenarioId() < scenarios[j].GetScenarioId()
	})
	catalog := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: CampaignSchemaVersion,
		CatalogRef: &compliancev1.VersionedReference{
			Id:      StandardCatalogID,
			Version: StandardCatalogVersion,
		},
		Scenarios: scenarios,
	}
	digest, err := ComputeScenarioCatalogDigest(catalog)
	if err != nil {
		return nil, nil, err
	}
	catalog.CatalogDigest = digest
	return catalog, artifacts, nil
}

// LoadScenarioCatalog returns the validated frozen scenario catalog.
func LoadScenarioCatalog() (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts, error) {
	catalog, artifacts, err := BuildScenarioCatalog()
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateScenarioCatalog(catalog, artifacts); err != nil {
		return nil, nil, err
	}
	return catalog, artifacts, nil
}

// ValidateScenarioCatalog verifies the frozen catalog gate for Phase 2.
func ValidateScenarioCatalog(catalog *evalv1.EvaluationScenarioCatalog, artifacts map[string]ScenarioArtifacts) error {
	if catalog == nil || catalog.GetCatalogRef() == nil {
		return fmt.Errorf("evaluation: validate scenario catalog: %w", constants.ErrMissingRequiredField)
	}
	if catalog.GetCatalogRef().GetId() != StandardCatalogID || catalog.GetCatalogRef().GetVersion() != StandardCatalogVersion {
		return fmt.Errorf("evaluation: validate scenario catalog: catalog identity mismatch")
	}
	if catalog.GetSchemaVersion() != CampaignSchemaVersion {
		return fmt.Errorf("evaluation: validate scenario catalog: unsupported schema version")
	}
	if err := ValidateScenarioCatalogDigest(catalog); err != nil {
		return err
	}
	if len(catalog.GetScenarios()) != 25 {
		return fmt.Errorf("evaluation: validate scenario catalog: expected 25 scenarios, got %d", len(catalog.GetScenarios()))
	}
	seen := make(map[string]struct{}, len(catalog.GetScenarios()))
	for _, scenario := range catalog.GetScenarios() {
		if scenario == nil || scenario.GetScenarioId() == "" || scenario.GetScenarioVersion() == "" {
			return fmt.Errorf("evaluation: validate scenario catalog: %w", constants.ErrMissingRequiredField)
		}
		key := scenario.GetScenarioId() + "@" + scenario.GetScenarioVersion()
		if _, exists := seen[key]; exists {
			return fmt.Errorf("evaluation: validate scenario catalog: duplicate scenario %s", key)
		}
		seen[key] = struct{}{}
		if scenario.GetCategory() == evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_UNSPECIFIED {
			return fmt.Errorf("evaluation: validate scenario catalog: scenario %s has unspecified category", scenario.GetScenarioId())
		}
		if scenario.GetPublicDescription() == "" {
			return fmt.Errorf("evaluation: validate scenario catalog: scenario %s missing public description", scenario.GetScenarioId())
		}
		if scenario.GetGradingMethod() == evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_UNSPECIFIED {
			return fmt.Errorf("evaluation: validate scenario catalog: scenario %s has unspecified grading method", scenario.GetScenarioId())
		}
		if scenario.GetInputFixtureRef() == nil || scenario.GetGoldCriteriaRef() == nil {
			return fmt.Errorf("evaluation: validate scenario catalog: scenario %s missing fixture references", scenario.GetScenarioId())
		}
		pair, ok := artifacts[scenario.GetScenarioId()]
		if !ok {
			return fmt.Errorf("evaluation: validate scenario catalog: missing artifacts for scenario %s", scenario.GetScenarioId())
		}
		if err := validateScenarioArtifactBinding(scenario.GetInputFixtureRef(), pair.Input); err != nil {
			return fmt.Errorf("evaluation: validate scenario catalog: scenario %s input fixture: %w", scenario.GetScenarioId(), err)
		}
		if err := validateScenarioArtifactBinding(scenario.GetGoldCriteriaRef(), pair.Gold); err != nil {
			return fmt.Errorf("evaluation: validate scenario catalog: scenario %s gold criteria: %w", scenario.GetScenarioId(), err)
		}
	}
	return validateCategoryCounts(catalog.GetScenarios())
}

func materializeScenarioBlueprint(blueprint ScenarioBlueprint) (*evalv1.EvaluationScenarioDefinition, ScenarioArtifacts, error) {
	input := blueprint.Input
	input.ScenarioID = blueprint.ScenarioID
	input.SyntheticLabel = "SYNTHETIC - controlled evaluation fixture"
	inputArtifact, err := buildScenarioInputArtifact(input)
	if err != nil {
		return nil, ScenarioArtifacts{}, err
	}
	gold := blueprint.Gold
	gold.ScenarioID = blueprint.ScenarioID
	goldArtifact, err := buildScenarioGoldArtifact(gold)
	if err != nil {
		return nil, ScenarioArtifacts{}, err
	}
	return &evalv1.EvaluationScenarioDefinition{
		ScenarioId:        blueprint.ScenarioID,
		ScenarioVersion:   blueprint.ScenarioVersion,
		Category:          blueprint.Category,
		PublicDescription: blueprint.PublicDescription,
		GradingMethod:     blueprint.GradingMethod,
		AllowedTools:      append([]string(nil), blueprint.AllowedTools...),
		ExpectedTools:     append([]string(nil), blueprint.ExpectedTools...),
		ForbiddenTools:    append([]string(nil), blueprint.ForbiddenTools...),
		RequiredConcepts:  append([]string(nil), blueprint.RequiredConcepts...),
		InputFixtureRef:   inputArtifact.Reference,
		GoldCriteriaRef:   goldArtifact.Reference,
	}, ScenarioArtifacts{Input: inputArtifact, Gold: goldArtifact}, nil
}

func validateScenarioArtifactBinding(reference *compliancev1.ComplianceEvidenceReference, artifact ScenarioArtifactPair) error {
	if reference == nil || artifact.Reference == nil {
		return fmt.Errorf("%w: missing reference", constants.ErrMissingRequiredField)
	}
	if reference.GetArtifactId() != artifact.Reference.GetArtifactId() {
		return fmt.Errorf("artifact ID mismatch")
	}
	if reference.GetSha256() != artifact.Reference.GetSha256() {
		return fmt.Errorf("artifact digest mismatch")
	}
	return nil
}

func validateCategoryCounts(scenarios []*evalv1.EvaluationScenarioDefinition) error {
	expected := map[evalv1.EvaluationScenarioCategory]int{
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE: 4,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION:        4,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT:         3,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TECHNICAL_ANALYSIS:    4,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_ROUTING_DELEGATION:    3,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_VERIFICATION:          2,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_SECURITY_POLICY:       2,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_RECOVERY:              2,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_FINAL_RESPONSE:        1,
	}
	counts := make(map[evalv1.EvaluationScenarioCategory]int, len(expected))
	for _, scenario := range scenarios {
		counts[scenario.GetCategory()]++
	}
	for category, want := range expected {
		if counts[category] != want {
			return fmt.Errorf("evaluation: validate scenario catalog: category %s expected %d scenarios, got %d", category.String(), want, counts[category])
		}
	}
	return nil
}
