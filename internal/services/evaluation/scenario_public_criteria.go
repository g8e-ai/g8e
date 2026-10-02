// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// Public criterion IDs are the stable grade criterion IDs a scenario's grades
// carry (docs/architecture/evals.md, Grades). A public criterion defines one of
// them for a reader of the public projection.
const (
	publicCriterionRoleInvoked     = "role-invoked"
	publicCriterionGovernedInfer   = "governed-inference"
	publicCriterionTrajectory      = "trajectory"
	publicCriterionScenarioContent = "scenario-content"
	publicCriterionToolAllowlist   = "tool-allowlist"
	publicCriterionSemanticJudge   = "semantic-judge"
)

// publicCriterionText is the public label and description of one criterion.
// Public text names what is checked. It never states the expected answer, a
// value, or the direction of a policy outcome: that is the private gold
// (INV-EVAL-EVID-04), so no text here may be copied from a gold description.
type publicCriterionText struct {
	Label       string
	Description string
}

var (
	publicRoleInvokedText = publicCriterionText{
		Label:       "Role invoked",
		Description: "The designated role handled the scored model call.",
	}
	publicGovernedInferenceText = publicCriterionText{
		Label:       "Governed inference",
		Description: "The scored model calls went through governed inference.",
	}
	publicToolAllowlistText = publicCriterionText{
		Label:       "Declared tools",
		Description: "Every tool call is checked against the scenario's declared tool set.",
	}
	publicSemanticJudgeText = publicCriterionText{
		Label:       "Judged answer quality",
		Description: "A judge model rates the answer against the scenario's public description.",
	}
)

// publicTrajectoryText keys the trajectory criterion by the scenario's
// trajectory policy, which is already public on the scenario summary.
var publicTrajectoryText = map[evalv1.EvaluationTrajectoryPolicy]publicCriterionText{
	policyAnswer: {
		Label:       "Tool trajectory",
		Description: "Tool use is checked against the answer-only trajectory policy.",
	},
	policyFirstChoice: {
		Label:       "Tool trajectory",
		Description: "The first tool call is checked against the scenario's expected tool and argument rules.",
	},
	policyGuided: {
		Label:       "Tool trajectory",
		Description: "Tool calls, including any retry after a platform error, are checked against the scenario's expected tool and argument rules.",
	},
	policyGoverned: {
		Label:       "Tool trajectory",
		Description: "Tool calls are checked against the scenario's forbidden tools and any policy denial the role meets.",
	},
}

// publicContentText keys the scenario-content criterion by scenario category.
var publicContentText = map[evalv1.EvaluationScenarioCategory]publicCriterionText{
	evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE: {
		Label:       "Answer content",
		Description: "The answer is checked against the format and length instructions in the prompt.",
	},
	evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION: {
		Label:       "Answer content",
		Description: "The answer is checked for the information the tool result makes available.",
	},
	evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT: {
		Label:       "Answer content",
		Description: "The answer is checked for the information the tool result makes available.",
	},
	evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_TECHNICAL_ANALYSIS: {
		Label:       "Answer content",
		Description: "The answer is checked against the evidence supplied in the prompt.",
	},
	evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_ROUTING_DELEGATION: {
		Label:       "Answer content",
		Description: "The answer is checked for how the role handles the work routed to it.",
	},
	evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_VERIFICATION: {
		Label:       "Answer content",
		Description: "The answer is checked for its verdict on the supplied evidence.",
	},
	evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_SECURITY_POLICY: {
		Label:       "Answer content",
		Description: "The answer is checked for how it responds to the policy constraint.",
	},
	evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_RECOVERY: {
		Label:       "Answer content",
		Description: "The answer is checked for how it reports the outcome after a tool or resource problem.",
	},
	evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_FINAL_RESPONSE: {
		Label:       "Answer content",
		Description: "The answer is checked for the structure and evidence of a final response.",
	},
}

// publicCriteriaForBlueprint derives the public criteria a scenario carries
// from the blueprint's typed shape, so a criterion can never claim a check the
// scenario does not run: the role, governed-inference, and trajectory checks
// run for every scenario; the content check when the gold carries one or the
// scenario is not judge-graded; the tool allowlist when tools are declared; and
// the judge when the scenario is judge-graded. Built-in and custom suites share
// this path, so both publish a scenario summary.
func publicCriteriaForBlueprint(blueprint ScenarioBlueprint) ([]*evalv1.PublicScenarioCriterion, error) {
	trajectory, ok := publicTrajectoryText[blueprint.TrajectoryPolicy]
	if !ok {
		return nil, scenarioContractError(blueprint, "no public trajectory criterion for policy %s", blueprint.TrajectoryPolicy)
	}
	criteria := []*evalv1.PublicScenarioCriterion{
		publicCriterion(publicCriterionRoleInvoked, publicRoleInvokedText, gradingDeterministic),
		publicCriterion(publicCriterionGovernedInfer, publicGovernedInferenceText, gradingDeterministic),
		publicCriterion(publicCriterionTrajectory, trajectory, gradingDeterministic),
	}
	judged := blueprint.GradingMethod == gradingSemanticJudge
	if blueprint.Gold.ContentCheck != nil || !judged {
		content, ok := publicContentText[blueprint.Category]
		if !ok {
			return nil, scenarioContractError(blueprint, "no public content criterion for category %s", blueprint.Category)
		}
		criteria = append(criteria, publicCriterion(publicCriterionScenarioContent, content, gradingDeterministic))
	}
	if len(blueprint.AllowedTools) > 0 {
		criteria = append(criteria, publicCriterion(publicCriterionToolAllowlist, publicToolAllowlistText, gradingDeterministic))
	}
	if judged {
		criteria = append(criteria, publicCriterion(publicCriterionSemanticJudge, publicSemanticJudgeText, gradingSemanticJudge))
	}
	return criteria, nil
}

func publicCriterion(id string, text publicCriterionText, method evalv1.EvaluationGradingMethod) *evalv1.PublicScenarioCriterion {
	return &evalv1.PublicScenarioCriterion{
		CriterionId:       id,
		PublicLabel:       text.Label,
		PublicDescription: text.Description,
		GradingMethod:     method,
		Required:          true,
	}
}

// publicToolScoreDimensionsForBlueprint derives which tool-scorecard
// dimensions the scenario exercises. A dimension left out publishes as
// `scenario_not_applicable`; one listed publishes its grade when one maps to it
// and `source_not_captured` otherwise. Dimensions follow the scenario's typed
// shape: expected tools, argument validators, a governed policy, and a
// tool-using recovery. The order is the enum order.
func publicToolScoreDimensionsForBlueprint(blueprint ScenarioBlueprint) []*evalv1.PublicToolScoreDimensionRequirement {
	applies := make(map[evalv1.PublicToolScoreDimension]bool)
	if len(blueprint.ExpectedTools) > 0 {
		applies[evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_RECOGNITION] = true
		applies[evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_SELECTION] = true
	}
	if len(blueprint.Gold.ArgumentValidators) > 0 {
		applies[evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SCHEMA] = true
		applies[evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SEMANTICS] = true
	}
	if blueprint.TrajectoryPolicy == policyGoverned {
		applies[evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_PERMISSION_COMPLIANCE] = true
	}
	if blueprint.Gold.RecoveryExpectation.Expected && len(blueprint.ExpectedTools) > 0 {
		applies[evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_RESULT_INTERPRETATION] = true
		applies[evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_FOLLOW_UP_DECISION] = true
		applies[evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_RECOVERY] = true
	}
	var dimensions []*evalv1.PublicToolScoreDimensionRequirement
	for _, dimension := range publicToolScoreDimensionOrder {
		if applies[dimension] {
			dimensions = append(dimensions, &evalv1.PublicToolScoreDimensionRequirement{Dimension: dimension, Required: true})
		}
	}
	return dimensions
}

// publicToolScoreDimensionOrder is every tool-scorecard dimension in enum
// order, so a scenario's derived dimensions are deterministic.
var publicToolScoreDimensionOrder = []evalv1.PublicToolScoreDimension{
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_RECOGNITION,
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_SELECTION,
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SCHEMA,
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SEMANTICS,
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_PERMISSION_COMPLIANCE,
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_RESULT_INTERPRETATION,
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_FOLLOW_UP_DECISION,
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_UNNECESSARY_TOOL_CALLS,
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_LOOPING,
	evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_RECOVERY,
}

// validatePublicScenarioCriteria fails closed on a scenario whose public
// projection could not be resolved: no criteria, a criterion without an ID,
// label, description, or grading method, a repeated criterion, or an
// unspecified or repeated tool dimension.
func validatePublicScenarioCriteria(scenario *evalv1.EvaluationScenarioDefinition) error {
	if len(scenario.GetPublicCriteria()) == 0 {
		return fmt.Errorf("scenario %s has no public criteria", scenario.GetScenarioId())
	}
	seen := make(map[string]struct{}, len(scenario.GetPublicCriteria()))
	for _, criterion := range scenario.GetPublicCriteria() {
		if criterion.GetCriterionId() == "" || criterion.GetPublicLabel() == "" || criterion.GetPublicDescription() == "" {
			return fmt.Errorf("scenario %s has a public criterion missing its id, label, or description", scenario.GetScenarioId())
		}
		if criterion.GetGradingMethod() == evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_UNSPECIFIED {
			return fmt.Errorf("scenario %s public criterion %s has an unspecified grading method", scenario.GetScenarioId(), criterion.GetCriterionId())
		}
		if _, dup := seen[criterion.GetCriterionId()]; dup {
			return fmt.Errorf("scenario %s repeats public criterion %s", scenario.GetScenarioId(), criterion.GetCriterionId())
		}
		seen[criterion.GetCriterionId()] = struct{}{}
	}
	dimensions := make(map[evalv1.PublicToolScoreDimension]struct{}, len(scenario.GetPublicToolScoreDimensions()))
	for _, requirement := range scenario.GetPublicToolScoreDimensions() {
		dimension := requirement.GetDimension()
		if dimension == evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_UNSPECIFIED {
			return fmt.Errorf("scenario %s has an unspecified public tool score dimension", scenario.GetScenarioId())
		}
		if _, dup := dimensions[dimension]; dup {
			return fmt.Errorf("scenario %s repeats public tool score dimension %s", scenario.GetScenarioId(), dimension)
		}
		dimensions[dimension] = struct{}{}
	}
	return nil
}
