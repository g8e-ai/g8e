// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// scenarioEvidenceTypes is the closed vocabulary of required evidence types.
// Every type in it is derivable from a digest-bound trace, so none can be
// permanently unavailable (INV-EVAL-CAMP-10).
var scenarioEvidenceTypes = map[string]struct{}{
	"model_inference":     {},
	"deterministic_grade": {},
	"semantic_grade":      {},
	"tool_decision":       {},
	"tool_call":           {},
	"governed_action":     {},
	"policy_decision":     {},
	"state_observation":   {},
	"recovery":            {},
	"final_response":      {},
}

func scenarioContractError(blueprint ScenarioBlueprint, format string, args ...any) error {
	return fmt.Errorf("evaluation: scenario %s: %w: %s", blueprint.ScenarioID, constants.ErrEvaluationScenarioContractInvalid, fmt.Sprintf(format, args...))
}

func scenarioUnanswerableError(blueprint ScenarioBlueprint, format string, args ...any) error {
	return fmt.Errorf("evaluation: scenario %s: %w: %s", blueprint.ScenarioID, constants.ErrEvaluationPromptUnanswerable, fmt.Sprintf(format, args...))
}

// validateScenarioContract fails closed on any breach of the scenario
// contract. Both catalog builders call it, so a bad blueprint cannot
// materialize. Checks run in the order the plan lists them.
func validateScenarioContract(blueprint ScenarioBlueprint, registry *AgentToolRegistry) error {
	if blueprint.TrajectoryPolicy == evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_UNSPECIFIED {
		return scenarioContractError(blueprint, "trajectory policy is unspecified")
	}
	if err := validateSeedCaseTitle(blueprint); err != nil {
		return err
	}
	if err := validateScenarioToolNames(blueprint, registry); err != nil {
		return err
	}
	if err := validateScenarioToolSets(blueprint); err != nil {
		return err
	}
	if err := validateTrajectoryPolicyShape(blueprint); err != nil {
		return err
	}
	if blueprint.GradingMethod != evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE && !contentCheckHasConstraint(blueprint.Gold.ContentCheck) {
		return scenarioContractError(blueprint, "scenario has no content check and is not judge-graded, so it cannot fail")
	}
	if err := validateArgumentValidators(blueprint, registry); err != nil {
		return err
	}
	if err := validatePromptHint(blueprint, registry); err != nil {
		return err
	}
	if err := validateWorkspaceFiles(blueprint); err != nil {
		return err
	}
	if err := validatePlayerExpectations(blueprint); err != nil {
		return err
	}
	return validateRequiredEvidenceTypes(blueprint)
}

// validatePlayerExpectations fails closed on player gold that could never
// grade anything or names a label g8ee never emits, so a typo cannot freeze
// into a scenario that silently accepts or rejects every run.
func validatePlayerExpectations(blueprint ScenarioBlueprint) error {
	players := blueprint.Gold.Players
	if players == nil {
		return nil
	}
	if triage := players.Triage; triage != nil {
		if len(triage.Complexity)+len(triage.Intent)+len(triage.Posture) == 0 {
			return scenarioContractError(blueprint, "triage expectation grades no label")
		}
		for _, value := range triage.Complexity {
			if value != constants.TriageComplexitySimple && value != constants.TriageComplexityComplex {
				return scenarioContractError(blueprint, "triage complexity %q is not a g8ee classification", value)
			}
		}
		for _, value := range triage.Intent {
			if value != constants.TriageIntentInformation && value != constants.TriageIntentAction && value != constants.TriageIntentUnknown {
				return scenarioContractError(blueprint, "triage intent %q is not a g8ee classification", value)
			}
		}
		for _, value := range triage.Posture {
			if value != constants.TriagePostureNormal && value != constants.TriagePostureEscalated && value != constants.TriagePostureAdversarial && value != constants.TriagePostureConfused {
				return scenarioContractError(blueprint, "triage posture %q is not a g8ee classification", value)
			}
		}
	}
	if players.Command != nil && !contentCheckHasConstraint(players.Command) {
		return scenarioContractError(blueprint, "command expectation has no constraint")
	}
	if marshal := players.Marshal; marshal != nil {
		if len(marshal.Risk) == 0 {
			return scenarioContractError(blueprint, "marshal expectation accepts no risk level")
		}
		for _, risk := range marshal.Risk {
			if risk != MarshalRiskLow && risk != MarshalRiskMedium && risk != MarshalRiskHigh {
				return scenarioContractError(blueprint, "marshal risk %q is not a g8ee risk level", risk)
			}
		}
	}
	if players.Codex != nil && !contentCheckHasConstraint(players.Codex) {
		return scenarioContractError(blueprint, "codex expectation has no constraint")
	}
	return nil
}

func validateSeedCaseTitle(blueprint ScenarioBlueprint) error {
	title := blueprint.Input.Seed.CaseTitle
	if strings.TrimSpace(title) == "" {
		return scenarioContractError(blueprint, "seed case title is empty")
	}
	lowered := strings.ToLower(title)
	if strings.Contains(lowered, "eval") || strings.Contains(lowered, "{{") {
		return scenarioContractError(blueprint, "seed case title %q must read like a real case and never name the evaluation", title)
	}
	return nil
}

func validateScenarioToolNames(blueprint ScenarioBlueprint, registry *AgentToolRegistry) error {
	names := make([]string, 0, len(blueprint.AllowedTools)+len(blueprint.ExpectedTools)+len(blueprint.ForbiddenTools))
	names = append(names, blueprint.AllowedTools...)
	names = append(names, blueprint.ExpectedTools...)
	names = append(names, blueprint.ForbiddenTools...)
	if blueprint.Gold.PromptHint != nil {
		names = append(names, blueprint.Gold.PromptHint.HintedTools...)
	}
	for _, validator := range blueprint.Gold.ArgumentValidators {
		names = append(names, validator.ToolName)
	}
	for _, name := range names {
		if _, ok := registry.Tool(name); !ok {
			return scenarioContractError(blueprint, "tool %q is not in the agent tool registry", name)
		}
	}
	return nil
}

func validateScenarioToolSets(blueprint ScenarioBlueprint) error {
	allowed := stringSet(blueprint.AllowedTools)
	for _, name := range blueprint.ExpectedTools {
		if _, ok := allowed[name]; !ok {
			return scenarioContractError(blueprint, "expected tool %q is not in AllowedTools", name)
		}
	}
	for _, name := range blueprint.ForbiddenTools {
		if _, ok := allowed[name]; ok {
			return scenarioContractError(blueprint, "tool %q is both allowed and forbidden", name)
		}
	}
	return nil
}

func validateTrajectoryPolicyShape(blueprint ScenarioBlueprint) error {
	hint := blueprint.Gold.PromptHint
	switch blueprint.TrajectoryPolicy {
	case evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_ANSWER:
		if len(blueprint.ExpectedTools) > 0 || len(blueprint.AllowedTools) > 0 || hint != nil {
			return scenarioContractError(blueprint, "ANSWER scenarios declare no expected tools, allowed tools, or prompt hint")
		}
	case evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_FIRST_CHOICE, evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GUIDED:
		if len(blueprint.ExpectedTools) == 0 {
			return scenarioContractError(blueprint, "%s scenarios need expected tools", blueprint.TrajectoryPolicy)
		}
		if hint == nil {
			return scenarioContractError(blueprint, "%s scenarios need a prompt hint", blueprint.TrajectoryPolicy)
		}
		expected := stringSet(blueprint.ExpectedTools)
		for _, name := range hint.HintedTools {
			if _, ok := expected[name]; !ok {
				return scenarioContractError(blueprint, "hinted tool %q is not an expected tool", name)
			}
		}
	case evalv1.EvaluationTrajectoryPolicy_EVALUATION_TRAJECTORY_POLICY_GOVERNED:
		if len(blueprint.ForbiddenTools) == 0 || len(blueprint.ExpectedTools) > 0 {
			return scenarioContractError(blueprint, "GOVERNED scenarios need forbidden tools and no expected tools")
		}
	default:
		return scenarioContractError(blueprint, "unknown trajectory policy %s", blueprint.TrajectoryPolicy)
	}
	return nil
}

func contentCheckHasConstraint(check *ScenarioContentCheck) bool {
	if check == nil {
		return false
	}
	return check.ExactToken != "" || len(check.ExactLabels) > 0 || check.LeadingLabel != "" || check.WordCount > 0 ||
		check.MaxSentences > 0 || len(check.RequiredTerms) > 0 || len(check.ForbiddenTerms) > 0 ||
		len(check.JSONStringFields) > 0 || len(check.JSONIntegerFields) > 0 || check.Interrogation
}

func validateArgumentValidators(blueprint ScenarioBlueprint, registry *AgentToolRegistry) error {
	for _, validator := range blueprint.Gold.ArgumentValidators {
		tool, _ := registry.Tool(validator.ToolName)
		known := stringSet(tool.Arguments)
		for _, constraint := range validator.Arguments {
			if _, ok := known[constraint.Name]; !ok {
				return scenarioContractError(blueprint, "validator for %q constrains unknown argument %q", validator.ToolName, constraint.Name)
			}
			hasRegex := len(constraint.RegexMatches) > 0 || len(constraint.RegexRejects) > 0
			if hasRegex && (len(constraint.RegexMatches) == 0 || len(constraint.RegexRejects) == 0) {
				return scenarioContractError(blueprint, "regex constraint on %q.%q needs both match and reject samples", validator.ToolName, constraint.Name)
			}
		}
	}
	return nil
}

// validatePromptHint enforces INV-EVAL-CAMP-11: every required argument of
// every hinted tool is derivable from where the hint says it is.
func validatePromptHint(blueprint ScenarioBlueprint, registry *AgentToolRegistry) error {
	hint := blueprint.Gold.PromptHint
	if hint == nil {
		return nil
	}
	hinted := stringSet(hint.HintedTools)
	for _, argument := range hint.Arguments {
		if _, ok := hinted[argument.ToolName]; !ok {
			return scenarioContractError(blueprint, "hint argument %q names tool %q that is not hinted", argument.Name, argument.ToolName)
		}
	}
	for _, toolName := range hint.HintedTools {
		tool, _ := registry.Tool(toolName)
		for _, required := range tool.RequiredArguments {
			matches := hintArgumentsFor(hint, toolName, required)
			if len(matches) != 1 {
				return scenarioUnanswerableError(blueprint, "required argument %q of %q has %d hint sources, want exactly 1", required, toolName, len(matches))
			}
			if err := validateHintArgumentSource(blueprint, matches[0]); err != nil {
				return err
			}
		}
	}
	return nil
}

func hintArgumentsFor(hint *ScenarioPromptHint, toolName, argumentName string) []ScenarioHintArgument {
	var matches []ScenarioHintArgument
	for _, argument := range hint.Arguments {
		if argument.ToolName == toolName && argument.Name == argumentName {
			matches = append(matches, argument)
		}
	}
	return matches
}

func validateHintArgumentSource(blueprint ScenarioBlueprint, argument ScenarioHintArgument) error {
	switch argument.Source {
	case evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_PROMPT:
		if argument.Value == "" || !strings.Contains(blueprint.Input.UserPrompt, argument.Value) {
			return scenarioUnanswerableError(blueprint, "%q.%q is sourced from the prompt but the prompt does not contain %q", argument.ToolName, argument.Name, argument.Value)
		}
	case evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_SEED:
		if argument.Value == "" || !seedContains(blueprint.Input.Seed, argument.Value) {
			return scenarioUnanswerableError(blueprint, "%q.%q is sourced from the seed but the seed does not contain %q", argument.ToolName, argument.Name, argument.Value)
		}
	case evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_WORKSPACE:
		if !strings.Contains(blueprint.Input.UserPrompt, ScenarioWorkspaceToken) {
			return scenarioUnanswerableError(blueprint, "%q.%q is sourced from the workspace but the prompt never names it", argument.ToolName, argument.Name)
		}
		if argument.Value != "" && !strings.HasPrefix(argument.Value, ScenarioWorkspaceToken) {
			return scenarioUnanswerableError(blueprint, "%q.%q workspace value %q is not under the workspace", argument.ToolName, argument.Name, argument.Value)
		}
	case evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_OPERATOR_CONTEXT:
		if argument.Name != "target_operators" {
			return scenarioUnanswerableError(blueprint, "%q.%q cannot come from operator context", argument.ToolName, argument.Name)
		}
	case evalv1.EvaluationHintArgumentSource_EVALUATION_HINT_ARGUMENT_SOURCE_MODEL_AUTHORED:
		if argument.Name != "justification" && argument.Name != "request" {
			return scenarioUnanswerableError(blueprint, "%q.%q cannot be authored by the model", argument.ToolName, argument.Name)
		}
	default:
		return scenarioUnanswerableError(blueprint, "%q.%q has an unspecified hint source", argument.ToolName, argument.Name)
	}
	return nil
}

func seedContains(seed InvestigationSeed, value string) bool {
	for _, turn := range seed.Turns {
		if strings.Contains(turn.Content, value) {
			return true
		}
	}
	for _, event := range seed.HistoryEvents {
		if strings.Contains(event.Summary, value) || strings.Contains(event.ArgumentsJSON, value) {
			return true
		}
	}
	return false
}

func validateWorkspaceFiles(blueprint ScenarioBlueprint) error {
	files := blueprint.Input.WorkspaceFiles
	seen := make(map[string]struct{}, len(files))
	real := 0
	for _, file := range files {
		if err := validateWorkspaceRelPath(file.RelPath); err != nil {
			return scenarioContractError(blueprint, "%v", err)
		}
		if _, dup := seen[file.RelPath]; dup {
			return scenarioContractError(blueprint, "duplicate workspace file %q", file.RelPath)
		}
		seen[file.RelPath] = struct{}{}
		if !file.Decoy {
			real++
		}
	}
	if real > 0 || blueprint.ScenarioID == "recovery-tool-failure" {
		return nil
	}
	if strings.Contains(blueprint.Input.UserPrompt, ScenarioWorkspaceToken) || validatorsUseWorkspace(blueprint.Gold.ArgumentValidators) {
		return scenarioContractError(blueprint, "the prompt or validators use the workspace but it has no non-decoy file")
	}
	return nil
}

func validatorsUseWorkspace(validators []ToolArgumentValidator) bool {
	for _, validator := range validators {
		for _, constraint := range validator.Arguments {
			if strings.Contains(constraint.PathEquals, ScenarioWorkspaceToken) || strings.Contains(constraint.PathUnder, ScenarioWorkspaceToken) || strings.Contains(constraint.Equals, ScenarioWorkspaceToken) {
				return true
			}
		}
	}
	return false
}

func validateRequiredEvidenceTypes(blueprint ScenarioBlueprint) error {
	for _, evidenceType := range blueprint.Gold.RequiredEvidenceTypes {
		if _, ok := scenarioEvidenceTypes[evidenceType]; !ok {
			return scenarioContractError(blueprint, "required evidence type %q is not in the closed vocabulary", evidenceType)
		}
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
