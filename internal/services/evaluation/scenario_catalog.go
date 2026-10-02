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
	ScenarioID        string
	ScenarioVersion   string
	Category          evalv1.EvaluationScenarioCategory
	PublicDescription string
	GradingMethod     evalv1.EvaluationGradingMethod
	AllowedTools      []string
	ExpectedTools     []string
	ForbiddenTools    []string
	RequiredConcepts  []string
	EligibleRoles     []evalv1.ModelCampaignRole
	TrajectoryPolicy  evalv1.EvaluationTrajectoryPolicy
	Input             ScenarioInputFixture
	Gold              ScenarioGoldCriteria
}

// ScenarioArtifacts holds the canonical fixture bodies for one scenario.
type ScenarioArtifacts struct {
	Input ScenarioArtifactPair
	Gold  ScenarioArtifactPair
}

// DefaultSuiteScenarioCount is the number of scenarios in the built-in default
// suite.
const DefaultSuiteScenarioCount = 27

// buildSuiteCatalog materializes one suite's blueprints into a catalog and its
// content-addressed fixture artifacts. Every blueprint is validated against the
// agent tool registry first, so a blueprint that breaks the scenario contract
// cannot materialize. The catalog's identity is ref; the same blueprints under
// another ref yield another catalog digest.
func buildSuiteCatalog(ref *compliancev1.VersionedReference, blueprints []ScenarioBlueprint) (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts, error) {
	registry, err := LoadAgentToolRegistry()
	if err != nil {
		return nil, nil, fmt.Errorf("evaluation: build suite %s catalog: %w", ref.GetId(), err)
	}
	artifacts := make(map[string]ScenarioArtifacts, len(blueprints))
	scenarios := make([]*evalv1.EvaluationScenarioDefinition, 0, len(blueprints))
	for _, blueprint := range blueprints {
		scenario, pair, err := materializeScenarioBlueprint(ref.GetId(), blueprint, registry)
		if err != nil {
			return nil, nil, fmt.Errorf("evaluation: build suite %s catalog: scenario %s: %w", ref.GetId(), blueprint.ScenarioID, err)
		}
		artifacts[blueprint.ScenarioID] = pair
		scenarios = append(scenarios, scenario)
	}
	sort.Slice(scenarios, func(i, j int) bool {
		return scenarios[i].GetScenarioId() < scenarios[j].GetScenarioId()
	})
	catalog := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: CampaignSchemaVersion,
		CatalogRef:    ref,
		Scenarios:     scenarios,
	}
	digest, err := ComputeScenarioCatalogDigest(catalog)
	if err != nil {
		return nil, nil, err
	}
	catalog.CatalogDigest = digest
	return catalog, artifacts, nil
}

// BuildScenarioCatalog materializes the built-in default suite's catalog and
// its fixture artifacts.
func BuildScenarioCatalog() (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts, error) {
	return buildSuiteCatalog(&compliancev1.VersionedReference{Id: DefaultSuiteID, Version: DefaultSuiteVersion}, scenarioBlueprints())
}

// LoadScenarioCatalog returns the validated built-in default suite catalog.
func LoadScenarioCatalog() (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts, error) {
	catalog, artifacts, err := BuildScenarioCatalog()
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateDefaultSuiteCatalog(catalog, artifacts); err != nil {
		return nil, nil, err
	}
	return catalog, artifacts, nil
}

// SmokeSuiteID identifies the built-in smoke suite: the discriminative subset
// of the default suite used for Tier 1 screening.
const SmokeSuiteID = "smoke-suite"

// SmokeGateScenarioCount is the number of discriminative scenarios in Tier 1 screening.
const SmokeGateScenarioCount = 5

// SmokeGateScenarioIDs lists the 5 high-discriminative scenarios for Tier 1 screening.
var SmokeGateScenarioIDs = []string{
	"instruction-exact-format",
	"tool-select-file-read",
	"tech-error-diagnosis",
	"security-policy-deny-delete",
	"recovery-tool-failure",
}

// smokeSuiteBlueprints selects the smoke scenarios from the default suite.
func smokeSuiteBlueprints() []ScenarioBlueprint {
	smokeIDs := make(map[string]struct{}, len(SmokeGateScenarioIDs))
	for _, id := range SmokeGateScenarioIDs {
		smokeIDs[id] = struct{}{}
	}
	blueprints := make([]ScenarioBlueprint, 0, len(SmokeGateScenarioIDs))
	for _, blueprint := range scenarioBlueprints() {
		if _, ok := smokeIDs[blueprint.ScenarioID]; ok {
			blueprints = append(blueprints, blueprint)
		}
	}
	return blueprints
}

// BuildSmokeGateScenarioCatalog materializes the 5-scenario screening catalog
// and its content-addressed fixture artifacts.
func BuildSmokeGateScenarioCatalog() (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts, error) {
	return buildSuiteCatalog(&compliancev1.VersionedReference{Id: SmokeSuiteID, Version: DefaultSuiteVersion}, smokeSuiteBlueprints())
}

// LoadSmokeGateScenarioCatalog returns the validated built-in smoke suite catalog.
func LoadSmokeGateScenarioCatalog() (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts, error) {
	catalog, artifacts, err := BuildSmokeGateScenarioCatalog()
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateSmokeGateScenarioCatalog(catalog, artifacts); err != nil {
		return nil, nil, err
	}
	return catalog, artifacts, nil
}

// ValidateSmokeGateScenarioCatalog verifies the built-in 5-scenario smoke suite.
func ValidateSmokeGateScenarioCatalog(catalog *evalv1.EvaluationScenarioCatalog, artifacts map[string]ScenarioArtifacts) error {
	if catalog == nil || catalog.GetCatalogRef() == nil {
		return fmt.Errorf("evaluation: validate smoke scenario catalog: %w", constants.ErrMissingRequiredField)
	}
	if catalog.GetCatalogRef().GetId() != SmokeSuiteID || catalog.GetCatalogRef().GetVersion() != DefaultSuiteVersion {
		return fmt.Errorf("evaluation: validate smoke scenario catalog: catalog identity mismatch")
	}
	if len(catalog.GetScenarios()) != SmokeGateScenarioCount {
		return fmt.Errorf("evaluation: validate smoke scenario catalog: expected %d scenarios, got %d", SmokeGateScenarioCount, len(catalog.GetScenarios()))
	}
	return ValidateScenarioCatalog(catalog, artifacts)
}

// ValidateDefaultSuiteCatalog verifies the built-in default suite: its
// identity, its scenario count, and its per-category population.
func ValidateDefaultSuiteCatalog(catalog *evalv1.EvaluationScenarioCatalog, artifacts map[string]ScenarioArtifacts) error {
	if catalog == nil || catalog.GetCatalogRef() == nil {
		return fmt.Errorf("evaluation: validate scenario catalog: %w", constants.ErrMissingRequiredField)
	}
	if catalog.GetCatalogRef().GetId() != DefaultSuiteID || catalog.GetCatalogRef().GetVersion() != DefaultSuiteVersion {
		return fmt.Errorf("evaluation: validate scenario catalog: catalog identity mismatch")
	}
	if len(catalog.GetScenarios()) != DefaultSuiteScenarioCount {
		return fmt.Errorf("evaluation: validate scenario catalog: %w: expected %d scenarios, got %d", constants.ErrEvaluationScenarioContractInvalid, DefaultSuiteScenarioCount, len(catalog.GetScenarios()))
	}
	if err := ValidateScenarioCatalog(catalog, artifacts); err != nil {
		return err
	}
	return validateDefaultSuiteCategoryCounts(catalog.GetScenarios())
}

// ValidateScenarioCatalog verifies the structure of any suite's frozen
// catalog: identity, schema, digest, and every scenario's fixture binding. It
// puts no constraint on how many scenarios a suite has or how they are spread
// across categories; the built-in suites add those gates themselves.
func ValidateScenarioCatalog(catalog *evalv1.EvaluationScenarioCatalog, artifacts map[string]ScenarioArtifacts) error {
	if catalog == nil || catalog.GetCatalogRef() == nil {
		return fmt.Errorf("evaluation: validate scenario catalog: %w", constants.ErrMissingRequiredField)
	}
	if catalog.GetCatalogRef().GetId() == "" || catalog.GetCatalogRef().GetVersion() == "" {
		return fmt.Errorf("evaluation: validate scenario catalog: %w: catalog identity", constants.ErrMissingRequiredField)
	}
	if catalog.GetSchemaVersion() != CampaignSchemaVersion {
		return fmt.Errorf("evaluation: validate scenario catalog: unsupported schema version")
	}
	if err := ValidateScenarioCatalogDigest(catalog); err != nil {
		return err
	}
	if len(catalog.GetScenarios()) == 0 {
		return fmt.Errorf("evaluation: validate scenario catalog: %w: a suite needs at least one scenario", constants.ErrEvaluationScenarioContractInvalid)
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
		if _, err := scenarioEligibleRoles(scenario); err != nil {
			return fmt.Errorf("evaluation: validate scenario catalog: %w", err)
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
	return nil
}

func materializeScenarioBlueprint(suiteID string, blueprint ScenarioBlueprint, registry *AgentToolRegistry) (*evalv1.EvaluationScenarioDefinition, ScenarioArtifacts, error) {
	if err := validateScenarioContract(blueprint, registry); err != nil {
		return nil, ScenarioArtifacts{}, err
	}
	input := blueprint.Input
	seed, err := resolveSeedGuidance(input.Seed, registry)
	if err != nil {
		return nil, ScenarioArtifacts{}, err
	}
	input.Seed = seed
	input.ScenarioID = blueprint.ScenarioID
	input.SyntheticLabel = scenarioSyntheticLabel
	inputArtifact, err := buildScenarioInputArtifact(suiteID, input)
	if err != nil {
		return nil, ScenarioArtifacts{}, err
	}
	gold := blueprint.Gold
	gold.ScenarioID = blueprint.ScenarioID
	goldArtifact, err := buildScenarioGoldArtifact(suiteID, gold)
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
		EligibleRoles:     append([]evalv1.ModelCampaignRole(nil), blueprint.EligibleRoles...),
		InputFixtureRef:   inputArtifact.Reference,
		GoldCriteriaRef:   goldArtifact.Reference,
		TrajectoryPolicy:  blueprint.TrajectoryPolicy,
		PromptHint:        publicPromptHint(blueprint.Gold.PromptHint),
	}, ScenarioArtifacts{Input: inputArtifact, Gold: goldArtifact}, nil
}

// publicPromptHint projects the private hint to its public form. Values are
// never copied: they stay in the private gold artifact (INV-EVAL-EVID-04).
func publicPromptHint(private *ScenarioPromptHint) *evalv1.PromptHint {
	if private == nil {
		return nil
	}
	public := &evalv1.PromptHint{HintedTools: append([]string(nil), private.HintedTools...)}
	for _, argument := range private.Arguments {
		public.Arguments = append(public.Arguments, &evalv1.PromptHintArgument{
			ToolName:     argument.ToolName,
			ArgumentName: argument.Name,
			Source:       argument.Source,
		})
	}
	return public
}

// resolveSeedGuidance returns a copy of the seed with every guidance vector
// reference replaced by the registry's real tool call and model-visible error.
// The registry is the only source of that text: it is produced by running the
// real g8ee handler, so a seeded failure is never hand-written prose.
func resolveSeedGuidance(seed InvestigationSeed, registry *AgentToolRegistry) (InvestigationSeed, error) {
	resolved := seed
	resolved.Turns = append([]InvestigationSeedTurn(nil), seed.Turns...)
	for i := range resolved.Turns {
		vectorID := resolved.Turns[i].GuidanceVectorID
		if vectorID == "" {
			continue
		}
		vector, ok := registry.GuidanceVector(vectorID)
		if !ok {
			return InvestigationSeed{}, fmt.Errorf("%w: unknown guidance vector %q on seed turn %d", constants.ErrEvaluationScenarioContractInvalid, vectorID, i)
		}
		resolved.Turns[i].Content += "\n\nTool result:\n" + vector.Error
		resolved.Turns[i].GuidanceVectorID = ""
	}
	resolved.HistoryEvents = append([]InvestigationSeedHistoryEvent(nil), seed.HistoryEvents...)
	for i := range resolved.HistoryEvents {
		vectorID := resolved.HistoryEvents[i].GuidanceVectorID
		if vectorID == "" {
			continue
		}
		vector, ok := registry.GuidanceVector(vectorID)
		if !ok {
			return InvestigationSeed{}, fmt.Errorf("%w: unknown guidance vector %q on seed history event %d", constants.ErrEvaluationScenarioContractInvalid, vectorID, i)
		}
		event := &resolved.HistoryEvents[i]
		event.ToolName = vector.ToolName
		event.ExecutionID = vector.ExecutionID
		event.ArgumentsJSON = vector.ArgumentsJSON
		event.ErrorType = vector.ErrorType
		event.Error = vector.Error
		event.GuidanceVectorID = ""
	}
	return resolved, nil
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
	if !contentMatchesReference(artifact.Body, artifact.Reference) {
		return fmt.Errorf("artifact body does not match its digest")
	}
	return nil
}

func validateDefaultSuiteCategoryCounts(scenarios []*evalv1.EvaluationScenarioDefinition) error {
	expected := map[evalv1.EvaluationScenarioCategory]int{
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE: 4,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION:        4,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT:         3,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TECHNICAL_ANALYSIS:    4,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_ROUTING_DELEGATION:    3,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_VERIFICATION:          2,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_SECURITY_POLICY:       3,
		evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_RECOVERY:              3,
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
