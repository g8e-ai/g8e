// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// TestResolvePublicScenarioContext_SucceedsForEveryBuiltInScenario is the
// regression for the default suite publishing no scenario summary: the catalog
// carried no public criteria, so the resolver rejected every scenario and
// publication degraded to the no-scenario-context path.
func TestResolvePublicScenarioContext_SucceedsForEveryBuiltInScenario(t *testing.T) {
	catalog, artifacts, err := LoadScenarioCatalog()
	require.NoError(t, err)
	require.Len(t, catalog.GetScenarios(), DefaultSuiteScenarioCount)

	run := &evalv1.EvaluationRun{
		RunId: "run-1",
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId: "campaign-1", CatalogDigest: catalog.GetCatalogDigest(), CatalogRef: catalog.GetCatalogRef(),
		},
	}
	for _, scenario := range catalog.GetScenarios() {
		assignment := &evalv1.EvaluationAssignment{
			AssignmentId: "assignment-1", RunId: "run-1", CampaignId: "campaign-1", ScenarioId: scenario.GetScenarioId(),
			ScenarioRef: &compliancev1.VersionedReference{Id: scenario.GetScenarioId(), Version: scenario.GetScenarioVersion()},
		}

		resolved, err := ResolvePublicScenarioContext(context.Background(), nil, run, catalog, assignment, artifacts)
		require.NoError(t, err, scenario.GetScenarioId())
		assert.NotEmpty(t, resolved.Criteria, scenario.GetScenarioId())
		assert.Equal(t, scenario.GetTrajectoryPolicy(), resolved.TrajectoryPolicy, scenario.GetScenarioId())

		summary, err := BuildPublicScenarioSummary(resolved)
		require.NoError(t, err, scenario.GetScenarioId())
		assert.Equal(t, scenario.GetPromptHint() != nil, summary.GetPromptHint() != nil, scenario.GetScenarioId())
	}
}

func TestBuildScenarioCatalog_EveryScenarioCarriesValidPublicCriteria(t *testing.T) {
	catalog, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	for _, scenario := range catalog.GetScenarios() {
		require.NoError(t, validatePublicScenarioCriteria(scenario), scenario.GetScenarioId())
		ids := make(map[string]*evalv1.PublicScenarioCriterion, len(scenario.GetPublicCriteria()))
		for _, criterion := range scenario.GetPublicCriteria() {
			ids[criterion.GetCriterionId()] = criterion
			assert.True(t, criterion.GetRequired(), "%s %s", scenario.GetScenarioId(), criterion.GetCriterionId())
		}
		for _, id := range []string{publicCriterionRoleInvoked, publicCriterionGovernedInfer, publicCriterionTrajectory} {
			assert.Contains(t, ids, id, scenario.GetScenarioId())
		}
		assert.Equal(t, len(scenario.GetAllowedTools()) > 0, ids[publicCriterionToolAllowlist] != nil, scenario.GetScenarioId())
		judged := scenario.GetGradingMethod() == gradingSemanticJudge
		assert.Equal(t, judged, ids[publicCriterionSemanticJudge] != nil, scenario.GetScenarioId())
		if judged {
			assert.Equal(t, gradingSemanticJudge, ids[publicCriterionSemanticJudge].GetGradingMethod(), scenario.GetScenarioId())
		}
	}
}

// TestBuildScenarioCatalog_PublicCriteriaNeverReusePrivateGoldText guards
// INV-EVAL-EVID-04: the gold descriptions state expected behavior and stay
// private, so no public label or description may equal or quote one.
func TestBuildScenarioCatalog_PublicCriteriaNeverReusePrivateGoldText(t *testing.T) {
	catalog, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	byID := make(map[string]*evalv1.EvaluationScenarioDefinition, len(catalog.GetScenarios()))
	for _, scenario := range catalog.GetScenarios() {
		byID[scenario.GetScenarioId()] = scenario
	}
	for _, blueprint := range scenarioBlueprints() {
		scenario := byID[blueprint.ScenarioID]
		require.NotNil(t, scenario, blueprint.ScenarioID)
		private := privateGoldDescriptions(blueprint.Gold)
		require.NotEmpty(t, private, blueprint.ScenarioID)
		for _, criterion := range scenario.GetPublicCriteria() {
			for _, public := range []string{criterion.GetPublicLabel(), criterion.GetPublicDescription()} {
				for _, gold := range private {
					assert.NotContains(t, strings.ToLower(public), strings.ToLower(gold), "scenario %s criterion %s quotes gold text", blueprint.ScenarioID, criterion.GetCriterionId())
					assert.NotEqual(t, strings.ToLower(gold), strings.ToLower(public), "scenario %s criterion %s equals gold text", blueprint.ScenarioID, criterion.GetCriterionId())
				}
			}
		}
	}
}

func privateGoldDescriptions(gold ScenarioGoldCriteria) []string {
	descriptions := []string{gold.ExpectedBehavior, gold.PolicyExpectation.Detail, gold.EscalationExpectation.Detail, gold.RecoveryExpectation.Detail}
	for _, role := range gold.RoleCriteria {
		for _, criterion := range role.Criteria {
			descriptions = append(descriptions, criterion.Description)
		}
	}
	for _, pipeline := range gold.PipelineCriteria {
		for _, criterion := range pipeline.Criteria {
			descriptions = append(descriptions, criterion.Description)
		}
	}
	kept := descriptions[:0]
	for _, description := range descriptions {
		if strings.TrimSpace(description) != "" {
			kept = append(kept, description)
		}
	}
	return kept
}

func TestBuildScenarioCatalog_PublicToolScoreDimensionsFollowScenarioShape(t *testing.T) {
	const (
		recognition    = evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_RECOGNITION
		selection      = evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_SELECTION
		schema         = evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SCHEMA
		semantics      = evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SEMANTICS
		permission     = evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_PERMISSION_COMPLIANCE
		interpretation = evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_RESULT_INTERPRETATION
		followUp       = evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_FOLLOW_UP_DECISION
		recovery       = evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_RECOVERY
	)
	catalog, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	got := make(map[string][]evalv1.PublicToolScoreDimension, len(catalog.GetScenarios()))
	for _, scenario := range catalog.GetScenarios() {
		for _, requirement := range scenario.GetPublicToolScoreDimensions() {
			assert.True(t, requirement.GetRequired(), scenario.GetScenarioId())
			got[scenario.GetScenarioId()] = append(got[scenario.GetScenarioId()], requirement.GetDimension())
		}
	}

	assert.Empty(t, got["instruction-exact-format"], "an answer-only scenario exercises no tool dimension")
	assert.Empty(t, got["recovery-malformed-resource"], "an answer-only recovery has no tool result to interpret")
	assert.Equal(t, []evalv1.PublicToolScoreDimension{recognition, selection}, got["tool-select-constraints"])
	assert.Equal(t, []evalv1.PublicToolScoreDimension{recognition, selection, schema, semantics}, got["tool-select-file-read"])
	assert.Equal(t, []evalv1.PublicToolScoreDimension{permission}, got["security-policy-deny-delete"])
	assert.Equal(t, []evalv1.PublicToolScoreDimension{recognition, selection, schema, semantics, interpretation, followUp, recovery}, got["recovery-error-guided-retry"])
}

// TestPublicCriteria_BuiltInSuiteRoundTripsThroughSuiteAuthoring proves a
// custom suite exported from the built-in suite publishes the same public
// criteria, so custom suites never fall back to the no-scenario-context path.
func TestPublicCriteria_BuiltInSuiteRoundTripsThroughSuiteAuthoring(t *testing.T) {
	builtin, _, err := BuildScenarioCatalog()
	require.NoError(t, err)
	custom, _, err := MaterializeSuite(DefaultScenarioSuite())
	require.NoError(t, err)

	require.Len(t, custom.GetScenarios(), len(builtin.GetScenarios()))
	for index, scenario := range builtin.GetScenarios() {
		exported := custom.GetScenarios()[index]
		require.Equal(t, scenario.GetScenarioId(), exported.GetScenarioId())
		require.Len(t, exported.GetPublicCriteria(), len(scenario.GetPublicCriteria()), scenario.GetScenarioId())
		for i := range scenario.GetPublicCriteria() {
			assert.True(t, proto.Equal(scenario.GetPublicCriteria()[i], exported.GetPublicCriteria()[i]), scenario.GetScenarioId())
		}
	}
	assert.Equal(t, builtin.GetCatalogDigest(), custom.GetCatalogDigest())
}

func TestPublicCriteria_ValidationRejectsUnresolvableCriteria(t *testing.T) {
	valid := func() *evalv1.EvaluationScenarioDefinition {
		return &evalv1.EvaluationScenarioDefinition{
			ScenarioId: "scenario-1",
			PublicCriteria: []*evalv1.PublicScenarioCriterion{
				{CriterionId: "a", PublicLabel: "A", PublicDescription: "Checks A", GradingMethod: gradingDeterministic, Required: true},
			},
		}
	}
	require.NoError(t, validatePublicScenarioCriteria(valid()))

	tests := []struct {
		name   string
		mutate func(*evalv1.EvaluationScenarioDefinition)
	}{
		{"no criteria", func(s *evalv1.EvaluationScenarioDefinition) { s.PublicCriteria = nil }},
		{"missing id", func(s *evalv1.EvaluationScenarioDefinition) { s.PublicCriteria[0].CriterionId = "" }},
		{"missing label", func(s *evalv1.EvaluationScenarioDefinition) { s.PublicCriteria[0].PublicLabel = "" }},
		{"missing description", func(s *evalv1.EvaluationScenarioDefinition) { s.PublicCriteria[0].PublicDescription = "" }},
		{"unspecified grading method", func(s *evalv1.EvaluationScenarioDefinition) {
			s.PublicCriteria[0].GradingMethod = evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_UNSPECIFIED
		}},
		{"repeated criterion", func(s *evalv1.EvaluationScenarioDefinition) {
			s.PublicCriteria = append(s.PublicCriteria, proto.Clone(s.PublicCriteria[0]).(*evalv1.PublicScenarioCriterion))
		}},
		{"unspecified dimension", func(s *evalv1.EvaluationScenarioDefinition) {
			s.PublicToolScoreDimensions = []*evalv1.PublicToolScoreDimensionRequirement{{Required: true}}
		}},
		{"repeated dimension", func(s *evalv1.EvaluationScenarioDefinition) {
			dimension := &evalv1.PublicToolScoreDimensionRequirement{Dimension: evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_RECOVERY, Required: true}
			s.PublicToolScoreDimensions = []*evalv1.PublicToolScoreDimensionRequirement{dimension, proto.Clone(dimension).(*evalv1.PublicToolScoreDimensionRequirement)}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scenario := valid()
			tt.mutate(scenario)
			assert.Error(t, validatePublicScenarioCriteria(scenario))
		})
	}
}
