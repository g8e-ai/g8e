// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License 1.1 —
// see LICENSE for details.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestAssignmentOutcomeSummary_NamesEveryFailedGrade(t *testing.T) {
	result := &evalv1.EvaluationAssignmentResult{
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		DeterministicGrades: []*evalv1.DeterministicGrade{
			{CriterionId: "trajectory", Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, Detail: "DIRECT"},
			{CriterionId: "scenario-content", Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, Detail: "missing required term"},
			{CriterionId: "primary-responsibility", Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL, Detail: "designated role did not satisfy: content check"},
		},
		DecomposedScores: []*evalv1.DecomposedScoreRecord{{Dimension: "deterministic_pass_rate", Value: 0.7272727}},
	}

	assert.Equal(t,
		"COMPLETED verdict=FAIL pass_rate=72.7% failed=[scenario-content (missing required term); primary-responsibility (designated role did not satisfy: content check)]",
		AssignmentOutcomeSummary(result))
}

func TestAssignmentOutcomeSummary_PassingResultListsNoFailures(t *testing.T) {
	result := &evalv1.EvaluationAssignmentResult{
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		DeterministicGrades: []*evalv1.DeterministicGrade{
			{CriterionId: "trajectory", Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS},
		},
		DecomposedScores: []*evalv1.DecomposedScoreRecord{{Dimension: "deterministic_pass_rate", Value: 1}},
	}

	assert.Equal(t, "COMPLETED verdict=PASS pass_rate=100.0%", AssignmentOutcomeSummary(result))
}
