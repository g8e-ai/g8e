// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	gradePass = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	gradeFail = evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL

	basisObservation = evalv1.GradeBasis_GRADE_BASIS_OBSERVATION
	basisDerived     = evalv1.GradeBasis_GRADE_BASIS_DERIVED
	basisStructural  = evalv1.GradeBasis_GRADE_BASIS_STRUCTURAL
)

func cellGrade(criterion string, basis evalv1.GradeBasis, status evalv1.EvaluationVerdictStatus) *evalv1.DeterministicGrade {
	return &evalv1.DeterministicGrade{GradeId: "a:" + criterion, CriterionId: criterion, Basis: basis, Status: status}
}

func cellPassRate(value float64) []*evalv1.DecomposedScoreRecord {
	return []*evalv1.DecomposedScoreRecord{{Dimension: "deterministic_pass_rate", Value: value}}
}

func completedResult(scores []*evalv1.DecomposedScoreRecord, grades ...*evalv1.DeterministicGrade) *evalv1.EvaluationAssignmentResult {
	return &evalv1.EvaluationAssignmentResult{
		LifecycleStatus:     evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		DeterministicGrades: grades,
		DecomposedScores:    scores,
	}
}

func TestCellFromResult_ReadsTheVerdictAndRateTheOtherSurfacesPublish(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		result      *evalv1.EvaluationAssignmentResult
		wantVerdict string
		wantPassed  bool
		wantScore   float64
	}{
		{
			name: "every counted observation passed",
			result: completedResult(cellPassRate(1),
				cellGrade("scenario-content", basisObservation, gradePass),
				cellGrade("trajectory", basisObservation, gradePass),
				cellGrade("primary-responsibility", basisDerived, gradePass)),
			wantVerdict: "pass", wantPassed: true, wantScore: 1,
		},
		{
			// One missed observation also fails the derived grade that restates
			// it; the cell reads one miss, not two.
			name: "one miss fails one observation and its derived restatement",
			result: completedResult(cellPassRate(0.5),
				cellGrade("scenario-content", basisObservation, gradeFail),
				cellGrade("trajectory", basisObservation, gradePass),
				cellGrade("primary-responsibility", basisDerived, gradeFail)),
			wantVerdict: "fail", wantPassed: false, wantScore: 0.5,
		},
		{
			name: "a failed triage never fails the cell",
			result: completedResult(cellPassRate(1),
				cellGrade("triage", basisObservation, gradeFail),
				cellGrade("scenario-content", basisObservation, gradePass)),
			wantVerdict: "pass", wantPassed: true, wantScore: 1,
		},
		{
			name: "a failed structural grade is invalid evidence with no score",
			result: completedResult(nil,
				cellGrade("role-invoked", basisStructural, gradeFail),
				cellGrade("scenario-content", basisObservation, gradeFail)),
			wantVerdict: "invalid_evidence", wantPassed: false, wantScore: 0,
		},
		{
			name: "a grade with no basis is invalid evidence",
			result: completedResult(nil,
				cellGrade("scenario-content", evalv1.GradeBasis_GRADE_BASIS_UNSPECIFIED, gradePass)),
			wantVerdict: "invalid_evidence", wantPassed: false, wantScore: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cell := cellFromResult(tt.result)

			assert.Equal(t, "completed", cell.Status)
			assert.Equal(t, tt.wantVerdict, cell.Verdict)
			assert.Equal(t, tt.wantPassed, cell.Passed)
			assert.InDelta(t, tt.wantScore, cell.Score, 1e-9)
			assert.Equal(t, tt.wantVerdict == "invalid_evidence", cell.invalidEvidence())
		})
	}
}

func TestCompareRunCells_InvalidEvidenceIsNeitherARegressionNorAnImprovement(t *testing.T) {
	t.Parallel()
	pass := runCell{Status: "completed", Verdict: "pass", Passed: true, Score: 1}
	fail := runCell{Status: "completed", Verdict: "fail", Score: 0.5}
	invalid := runCell{Status: "completed", Verdict: invalidEvidenceVerdict}
	left := map[string]runCell{"pass-to-fail": pass, "fail-to-pass": fail, "pass-to-invalid": pass, "invalid-to-pass": invalid, "invalid-both": invalid, "same": pass}
	right := map[string]runCell{"pass-to-fail": fail, "fail-to-pass": pass, "pass-to-invalid": invalid, "invalid-to-pass": pass, "invalid-both": invalid, "same": pass}

	onlyLeft, onlyRight, changes := compareRunCells(left, right)

	assert.Empty(t, onlyLeft)
	assert.Empty(t, onlyRight)
	kinds := make(map[string]string, len(changes))
	for _, change := range changes {
		kinds[change.Cell] = change.Kind
	}
	assert.Equal(t, map[string]string{
		"pass-to-fail":    "regression",
		"fail-to-pass":    "improvement",
		"pass-to-invalid": "changed",
		"invalid-to-pass": "changed",
	}, kinds)
}

func TestSummarizeRunCells_InvalidEvidenceDoesNotLowerTheMean(t *testing.T) {
	t.Parallel()
	cells := map[string]runCell{
		"a": {Status: "completed", Verdict: "pass", Passed: true, Score: 1},
		"b": {Status: "completed", Verdict: "fail", Score: 0.5},
		"c": {Status: "completed", Verdict: invalidEvidenceVerdict},
	}

	side := summarizeRunCells("run-1", "campaign-1", cells)

	assert.Equal(t, 3, side.Cells)
	assert.Equal(t, 1, side.Passed)
	assert.Equal(t, 1, side.InvalidEvidence)
	assert.InDelta(t, 0.75, side.MeanScore, 1e-9, "the mean reads the two cells that measured the model")
}
