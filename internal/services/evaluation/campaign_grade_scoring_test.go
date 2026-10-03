// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func scoringGrade(id string, basis evalv1.GradeBasis, status evalv1.EvaluationVerdictStatus) *evalv1.DeterministicGrade {
	return newDeterministicGrade("a-1", id, basis, status, "", 0)
}

func completedResult(grades ...*evalv1.DeterministicGrade) *evalv1.EvaluationAssignmentResult {
	return &evalv1.EvaluationAssignmentResult{
		LifecycleStatus:     evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		DeterministicGrades: grades,
	}
}

func TestTallyDeterministicGrades_AMissedContentCheckCountsOnce(t *testing.T) {
	t.Parallel()
	// The worked D1 case: one missing refusal term fails scenario-content, which
	// the responsibility and policy_decision grades restate.
	grades := []*evalv1.DeterministicGrade{
		scoringGrade("scenario-content", basisObservation, verdictFail),
		scoringGrade("trajectory", basisObservation, verdictPass),
		scoringGrade("tool-allowlist", basisObservation, verdictPass),
		scoringGrade("required-evidence:state_observation", basisObservation, verdictPass),
		scoringGrade("primary-responsibility", basisDerived, verdictFail),
		scoringGrade("required-evidence:policy_decision", basisDerived, verdictFail),
		scoringGrade("role-invoked", basisStructural, verdictPass),
		scoringGrade("governed-inference", basisStructural, verdictPass),
	}
	tally, err := tallyDeterministicGrades(grades)
	require.NoError(t, err)
	assert.Equal(t, 4, tally.Counted)
	assert.Equal(t, 3, tally.Passed)
	assert.InDelta(t, 0.75, tally.PassRate(), 1e-9)
	assert.False(t, tally.ObservationsPassed())

	scores, err := deriveScenarioDecomposedScores("a-1", grades)
	require.NoError(t, err)
	assert.InDelta(t, 0.75, findDecomposedScore(scores, "deterministic_pass_rate").GetValue(), 1e-9)
	assert.Equal(t, 0.0, findDecomposedScore(scores, "task_score").GetValue())
	assert.Equal(t, verdictFail, DerivePublicSummaryStatus(completedResult(grades...)))
}

func TestTallyDeterministicGrades_DerivedAndStructuralPassesAddNothing(t *testing.T) {
	t.Parallel()
	tally, err := tallyDeterministicGrades([]*evalv1.DeterministicGrade{
		scoringGrade("trajectory", basisObservation, verdictPass),
		scoringGrade("primary-responsibility", basisDerived, verdictPass),
		scoringGrade("governed-inference", basisStructural, verdictPass),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, tally.Counted)
	assert.Equal(t, 1, tally.Passed)
}

func TestTallyDeterministicGrades_AGradeWithoutABasisIsAConstructionError(t *testing.T) {
	t.Parallel()
	_, err := tallyDeterministicGrades([]*evalv1.DeterministicGrade{
		scoringGrade("trajectory", basisObservation, verdictPass),
		scoringGrade("scenario-content", evalv1.GradeBasis_GRADE_BASIS_UNSPECIFIED, verdictPass),
	})
	require.ErrorIs(t, err, constants.ErrEvaluationGradeBasisMissing)
}

func TestTallyDeterministicGrades_ADerivedFailureAloneIsAGraderBug(t *testing.T) {
	t.Parallel()
	_, err := tallyDeterministicGrades([]*evalv1.DeterministicGrade{
		scoringGrade("trajectory", basisObservation, verdictPass),
		scoringGrade("primary-responsibility", basisDerived, verdictFail),
	})
	require.ErrorIs(t, err, constants.ErrEvaluationDerivedGradeContradiction)
}

func TestDerivePublicSummaryStatus_AgreesWithTheScoresOnEveryFixture(t *testing.T) {
	t.Parallel()
	fixtures := map[string][]*evalv1.DeterministicGrade{
		"all observations pass": {
			scoringGrade("trajectory", basisObservation, verdictPass),
			scoringGrade("scenario-content", basisObservation, verdictPass),
			scoringGrade("primary-responsibility", basisDerived, verdictPass),
		},
		"one observation fails": {
			scoringGrade("trajectory", basisObservation, verdictPass),
			scoringGrade("scenario-content", basisObservation, verdictFail),
			scoringGrade("primary-responsibility", basisDerived, verdictFail),
		},
		"a failing triage is not part of the verdict": {
			scoringGrade("triage", basisObservation, verdictFail),
			scoringGrade("trajectory", basisObservation, verdictPass),
		},
		"a passing triage does not rescue a failing observation": {
			scoringGrade("triage", basisObservation, verdictPass),
			scoringGrade("trajectory", basisObservation, verdictFail),
		},
	}
	for name, grades := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scores, err := deriveScenarioDecomposedScores("a-1", grades)
			require.NoError(t, err)
			taskPassed := findDecomposedScore(scores, "task_score").GetValue() == 1
			verdict := DerivePublicSummaryStatus(completedResult(grades...))
			assert.Equal(t, taskPassed, verdict == verdictPass, "task_score and the verdict decide pass from the same tally")
		})
	}
}

func TestDerivePublicSummaryStatus_AGradeWithoutABasisIsInvalidEvidence(t *testing.T) {
	t.Parallel()
	// There is no legacy reading: a result recorded before grades carried a basis
	// is identified by its release tag, not re-derived under the current rules.
	result := completedResult(&evalv1.DeterministicGrade{CriterionId: "trajectory", Status: verdictPass})
	assert.Equal(t, verdictInvalidEvidence, DerivePublicSummaryStatus(result))
}

func TestDerivePublicSummaryStatus_AFailedStructuralGradeIsInvalidEvidenceAndUnscored(t *testing.T) {
	t.Parallel()
	grades := []*evalv1.DeterministicGrade{
		scoringGrade("trajectory", basisObservation, verdictFail),
		scoringGrade("role-invoked", basisStructural, verdictFail),
	}
	assert.Equal(t, verdictInvalidEvidence, DerivePublicSummaryStatus(completedResult(grades...)))
	scores, err := deriveScenarioDecomposedScores("a-1", grades)
	require.NoError(t, err)
	assert.Nil(t, scores, "a harness failure carries no score for or against the model")
}
