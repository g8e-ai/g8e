// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// SuiteSchemaVersion is the schema version of the suite authoring file.
const SuiteSchemaVersion = "1.0.0"

// suiteIDPattern is the spelling of a suite ID. A suite ID is a file name, a
// CLI argument, and the scope of its fixture artifacts.
var suiteIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

const (
	categoryEnumPrefix         = "EVALUATION_SCENARIO_CATEGORY_"
	gradingMethodEnumPrefix    = "EVALUATION_GRADING_METHOD_"
	trajectoryPolicyEnumPrefix = "EVALUATION_TRAJECTORY_POLICY_"
	hintSourceEnumPrefix       = "EVALUATION_HINT_ARGUMENT_SOURCE_"
	roleEnumPrefix             = "MODEL_CAMPAIGN_ROLE_"
)

// ScenarioSuite is the authoring record of one evaluation suite: a named,
// versioned set of scenarios a campaign can freeze. It is the file format
// `g8e eval suites` reads and writes. Enum-valued fields are lowercase names
// (for example "tool_selection"), never numbers. A campaign freezes the
// materialized catalog and fixture artifacts at creation, so editing or
// deleting a suite never changes a campaign that already used it.
type ScenarioSuite struct {
	SchemaVersion string          `json:"schema_version"`
	ID            string          `json:"id"`
	Version       string          `json:"version"`
	Description   string          `json:"description,omitempty"`
	Scenarios     []SuiteScenario `json:"scenarios"`
}

// SuiteScenario is one scenario of a suite: its public metadata, the private
// prompt fixture, and the private gold criteria.
type SuiteScenario struct {
	ScenarioID        string               `json:"scenario_id"`
	ScenarioVersion   string               `json:"scenario_version"`
	Category          string               `json:"category"`
	PublicDescription string               `json:"public_description"`
	GradingMethod     string               `json:"grading_method"`
	TrajectoryPolicy  string               `json:"trajectory_policy"`
	EligibleRoles     []string             `json:"eligible_roles"`
	AllowedTools      []string             `json:"allowed_tools,omitempty"`
	ExpectedTools     []string             `json:"expected_tools,omitempty"`
	ForbiddenTools    []string             `json:"forbidden_tools,omitempty"`
	RequiredConcepts  []string             `json:"required_concepts,omitempty"`
	Input             ScenarioInputFixture `json:"input"`
	Gold              SuiteGold            `json:"gold"`
}

// SuiteGold is ScenarioGoldCriteria with the prompt hint's argument source
// spelled as a name. The shallower PromptHint field shadows the embedded one
// in encoding/json, so every other gold field keeps its artifact spelling.
type SuiteGold struct {
	ScenarioGoldCriteria
	PromptHint *SuitePromptHint `json:"prompt_hint,omitempty"`
}

// SuitePromptHint is ScenarioPromptHint with named argument sources.
type SuitePromptHint struct {
	HintedTools []string                `json:"hinted_tools"`
	Arguments   []SuiteHintArgumentSpec `json:"arguments,omitempty"`
}

// SuiteHintArgumentSpec is ScenarioHintArgument with a named source such as
// "prompt", "seed", "workspace", "operator_context", or "model_authored".
type SuiteHintArgumentSpec struct {
	ToolName string `json:"tool_name"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	Value    string `json:"value,omitempty"`
}

// SuiteSummary is the identity and shape of one suite for listings.
type SuiteSummary struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	Description   string `json:"description,omitempty"`
	Builtin       bool   `json:"builtin"`
	ScenarioCount int    `json:"scenario_count"`
}

// DefaultScenarioSuite returns the built-in default suite as an authoring
// record, so it can be exported and used as a template for a custom suite.
func DefaultScenarioSuite() ScenarioSuite {
	return suiteDefinitionFromBlueprints(DefaultSuiteID, DefaultSuiteVersion,
		"The built-in suite: every scenario the platform scores by default, across instruction adherence, tool use, analysis, routing, verification, security, recovery, and final response.",
		scenarioBlueprints())
}

// SmokeScenarioSuite returns the built-in smoke suite as an authoring record.
func SmokeScenarioSuite() ScenarioSuite {
	return suiteDefinitionFromBlueprints(SmokeSuiteID, DefaultSuiteVersion,
		"The built-in screening subset of the default suite: five high-discrimination scenarios for a fast pre-qualification pass.",
		smokeSuiteBlueprints())
}

// BuiltinScenarioSuite returns the built-in suite with the given ID.
func BuiltinScenarioSuite(id string) (ScenarioSuite, bool) {
	switch id {
	case DefaultSuiteID:
		return DefaultScenarioSuite(), true
	case SmokeSuiteID:
		return SmokeScenarioSuite(), true
	default:
		return ScenarioSuite{}, false
	}
}

// BuiltinSuiteIDs lists the built-in suite IDs in display order.
func BuiltinSuiteIDs() []string {
	return []string{DefaultSuiteID, SmokeSuiteID}
}

// CatalogRecomputesGrades reports whether verification regrades the stored
// results of a campaign frozen from this catalog. Every suite grades from the
// fixtures frozen with its campaign, so a custom suite is always regraded. Only
// a built-in default suite at a version other than the current one is not: its
// stored grades were produced against fixtures this build no longer carries.
func CatalogRecomputesGrades(ref *compliancev1.VersionedReference) bool {
	if ref == nil || ref.GetVersion() == "" {
		return true
	}
	return ref.GetId() != DefaultSuiteID || ref.GetVersion() == DefaultSuiteVersion
}

// Summary projects a definition to its listing form.
func (d ScenarioSuite) Summary(builtin bool) SuiteSummary {
	return SuiteSummary{ID: d.ID, Version: d.Version, Description: d.Description, Builtin: builtin, ScenarioCount: len(d.Scenarios)}
}

// DecodeScenarioSuite parses one suite authoring file. Unknown fields are
// rejected so a misspelled criterion cannot be silently dropped from scoring.
func DecodeScenarioSuite(body []byte) (ScenarioSuite, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var def ScenarioSuite
	if err := decoder.Decode(&def); err != nil {
		return ScenarioSuite{}, fmt.Errorf("evaluation: decode suite definition: %w: %w", constants.ErrEvaluationSuiteInvalid, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ScenarioSuite{}, fmt.Errorf("evaluation: decode suite definition: %w: trailing content after the definition", constants.ErrEvaluationSuiteInvalid)
	}
	return def, nil
}

// EncodeScenarioSuite renders one suite authoring file.
func EncodeScenarioSuite(def ScenarioSuite) ([]byte, error) {
	body, err := json.MarshalIndent(def, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("evaluation: encode suite definition: %w", err)
	}
	return append(body, '\n'), nil
}

// ValidateSuiteIdentity checks the parts of a definition that do not need the
// scenario contract: schema, identity, and scenario identity uniqueness.
func ValidateSuiteIdentity(def ScenarioSuite) error {
	if def.SchemaVersion != SuiteSchemaVersion {
		return suiteInvalid("schema_version must be %q, got %q", SuiteSchemaVersion, def.SchemaVersion)
	}
	if !suiteIDPattern.MatchString(def.ID) {
		return suiteInvalid("id %q must be 1-64 characters of lowercase letters, digits, '-', '_', or '.', starting with a letter or digit", def.ID)
	}
	if strings.TrimSpace(def.Version) == "" {
		return suiteInvalid("version is required")
	}
	if len(def.Scenarios) == 0 {
		return suiteInvalid("a suite needs at least one scenario")
	}
	seen := make(map[string]struct{}, len(def.Scenarios))
	for i, scenario := range def.Scenarios {
		if !complianceevidence.ValidPathElement(scenario.ScenarioID) {
			return suiteInvalid("scenario %d: scenario_id %q must be a single path element", i, scenario.ScenarioID)
		}
		if strings.TrimSpace(scenario.ScenarioVersion) == "" {
			return suiteInvalid("scenario %s: scenario_version is required", scenario.ScenarioID)
		}
		if _, dup := seen[scenario.ScenarioID]; dup {
			return suiteInvalid("scenario_id %q appears more than once", scenario.ScenarioID)
		}
		seen[scenario.ScenarioID] = struct{}{}
	}
	return nil
}

// MaterializeSuite validates a definition against the full scenario contract
// (tool registry, trajectory policy shape, argument validators, prompt hints,
// workspace fixtures) and materializes its catalog and fixture artifacts. A
// definition that fails any check cannot be stored or frozen.
func MaterializeSuite(def ScenarioSuite) (*evalv1.EvaluationScenarioCatalog, map[string]ScenarioArtifacts, error) {
	if err := ValidateSuiteIdentity(def); err != nil {
		return nil, nil, err
	}
	blueprints := make([]ScenarioBlueprint, 0, len(def.Scenarios))
	for _, scenario := range def.Scenarios {
		blueprint, err := scenario.blueprint()
		if err != nil {
			return nil, nil, fmt.Errorf("evaluation: materialize suite %s: scenario %s: %w", def.ID, scenario.ScenarioID, err)
		}
		blueprints = append(blueprints, blueprint)
	}
	catalog, artifacts, err := buildSuiteCatalog(&compliancev1.VersionedReference{Id: def.ID, Version: def.Version}, blueprints)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateScenarioCatalog(catalog, artifacts); err != nil {
		return nil, nil, err
	}
	return catalog, artifacts, nil
}

func suiteInvalid(format string, args ...any) error {
	return fmt.Errorf("evaluation: suite definition: %w: %s", constants.ErrEvaluationSuiteInvalid, fmt.Sprintf(format, args...))
}

func suiteDefinitionFromBlueprints(id, version, description string, blueprints []ScenarioBlueprint) ScenarioSuite {
	def := ScenarioSuite{SchemaVersion: SuiteSchemaVersion, ID: id, Version: version, Description: description, Scenarios: make([]SuiteScenario, 0, len(blueprints))}
	for _, blueprint := range blueprints {
		def.Scenarios = append(def.Scenarios, suiteScenarioFromBlueprint(blueprint))
	}
	return def
}

func suiteScenarioFromBlueprint(blueprint ScenarioBlueprint) SuiteScenario {
	roles := make([]string, 0, len(blueprint.EligibleRoles))
	for _, role := range blueprint.EligibleRoles {
		roles = append(roles, enumName(role.String(), roleEnumPrefix))
	}
	input := blueprint.Input
	input.SchemaVersion = scenarioInputSchemaVersion
	input.ScenarioID = blueprint.ScenarioID
	input.SyntheticLabel = scenarioSyntheticLabel
	gold := blueprint.Gold
	gold.SchemaVersion = scenarioGoldSchemaVersion
	gold.ScenarioID = blueprint.ScenarioID
	suiteGold := SuiteGold{ScenarioGoldCriteria: gold}
	if hint := gold.PromptHint; hint != nil {
		published := &SuitePromptHint{HintedTools: append([]string(nil), hint.HintedTools...)}
		for _, argument := range hint.Arguments {
			published.Arguments = append(published.Arguments, SuiteHintArgumentSpec{
				ToolName: argument.ToolName, Name: argument.Name, Value: argument.Value,
				Source: enumName(argument.Source.String(), hintSourceEnumPrefix),
			})
		}
		suiteGold.PromptHint = published
	}
	suiteGold.ScenarioGoldCriteria.PromptHint = nil
	return SuiteScenario{
		ScenarioID:        blueprint.ScenarioID,
		ScenarioVersion:   blueprint.ScenarioVersion,
		Category:          enumName(blueprint.Category.String(), categoryEnumPrefix),
		PublicDescription: blueprint.PublicDescription,
		GradingMethod:     enumName(blueprint.GradingMethod.String(), gradingMethodEnumPrefix),
		TrajectoryPolicy:  enumName(blueprint.TrajectoryPolicy.String(), trajectoryPolicyEnumPrefix),
		EligibleRoles:     roles,
		AllowedTools:      append([]string(nil), blueprint.AllowedTools...),
		ExpectedTools:     append([]string(nil), blueprint.ExpectedTools...),
		ForbiddenTools:    append([]string(nil), blueprint.ForbiddenTools...),
		RequiredConcepts:  append([]string(nil), blueprint.RequiredConcepts...),
		Input:             input,
		Gold:              suiteGold,
	}
}

// blueprint converts one authored scenario to the blueprint the catalog
// builder validates and materializes.
func (s SuiteScenario) blueprint() (ScenarioBlueprint, error) {
	category, err := parseSuiteEnum[evalv1.EvaluationScenarioCategory]("category", categoryEnumPrefix, evalv1.EvaluationScenarioCategory_value, s.Category)
	if err != nil {
		return ScenarioBlueprint{}, err
	}
	grading, err := parseSuiteEnum[evalv1.EvaluationGradingMethod]("grading_method", gradingMethodEnumPrefix, evalv1.EvaluationGradingMethod_value, s.GradingMethod)
	if err != nil {
		return ScenarioBlueprint{}, err
	}
	policy, err := parseSuiteEnum[evalv1.EvaluationTrajectoryPolicy]("trajectory_policy", trajectoryPolicyEnumPrefix, evalv1.EvaluationTrajectoryPolicy_value, s.TrajectoryPolicy)
	if err != nil {
		return ScenarioBlueprint{}, err
	}
	roles := make([]evalv1.ModelCampaignRole, 0, len(s.EligibleRoles))
	for _, name := range s.EligibleRoles {
		role, err := parseSuiteEnum[evalv1.ModelCampaignRole]("eligible_roles entry", roleEnumPrefix, evalv1.ModelCampaignRole_value, name)
		if err != nil {
			return ScenarioBlueprint{}, err
		}
		roles = append(roles, role)
	}
	gold := s.Gold.ScenarioGoldCriteria
	gold.PromptHint = nil
	if hint := s.Gold.PromptHint; hint != nil {
		private := &ScenarioPromptHint{HintedTools: append([]string(nil), hint.HintedTools...)}
		for _, argument := range hint.Arguments {
			source, err := parseSuiteEnum[evalv1.EvaluationHintArgumentSource]("prompt_hint argument source", hintSourceEnumPrefix, evalv1.EvaluationHintArgumentSource_value, argument.Source)
			if err != nil {
				return ScenarioBlueprint{}, err
			}
			private.Arguments = append(private.Arguments, ScenarioHintArgument{ToolName: argument.ToolName, Name: argument.Name, Source: source, Value: argument.Value})
		}
		gold.PromptHint = private
	}
	return ScenarioBlueprint{
		ScenarioID:        s.ScenarioID,
		ScenarioVersion:   s.ScenarioVersion,
		Category:          category,
		PublicDescription: s.PublicDescription,
		GradingMethod:     grading,
		AllowedTools:      append([]string(nil), s.AllowedTools...),
		ExpectedTools:     append([]string(nil), s.ExpectedTools...),
		ForbiddenTools:    append([]string(nil), s.ForbiddenTools...),
		RequiredConcepts:  append([]string(nil), s.RequiredConcepts...),
		EligibleRoles:     roles,
		TrajectoryPolicy:  policy,
		Input:             s.Input,
		Gold:              gold,
	}, nil
}

// parseSuiteEnum resolves a lowercase enum name to its proto value and rejects
// both unknown names and the UNSPECIFIED zero value.
func parseSuiteEnum[E ~int32](field, prefix string, values map[string]int32, text string) (E, error) {
	value, ok := values[prefix+strings.ToUpper(strings.TrimSpace(text))]
	if !ok || value == 0 {
		return 0, fmt.Errorf("%w: %s %q is not one of %s", constants.ErrEvaluationSuiteInvalid, field, text, strings.Join(suiteEnumNames(prefix, values), ", "))
	}
	return E(value), nil
}

func suiteEnumNames(prefix string, values map[string]int32) []string {
	names := make([]string, 0, len(values))
	for name, value := range values {
		if value == 0 {
			continue
		}
		names = append(names, strings.ToLower(strings.TrimPrefix(name, prefix)))
	}
	sort.Strings(names)
	return names
}

func enumName(value, prefix string) string {
	return strings.ToLower(strings.TrimPrefix(value, prefix))
}
