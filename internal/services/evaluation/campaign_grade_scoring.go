// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// triageGradeCriterionID is the Triage call grade. It is reported on its own
// `triage_ok` score and lowers the lite tier through `player:triage`, so it
// never counts toward the verdict or the deterministic pass rate.
const triageGradeCriterionID = "triage"

// gradeTally is the one reading of a result's deterministic grades. The
// verdict (DerivePublicSummaryStatus), the `task_score` and
// `deterministic_pass_rate` scores, the verifier's recomputation, and
// AssignmentOutcomeSummary all read it, so they cannot disagree.
type gradeTally struct {
	// Counted is the number of observation grades that passed or failed, and
	// Passed the number that passed.
	Counted int
	Passed  int
	// StructuralFailed reports a failed harness precondition (INVALID_EVIDENCE).
	StructuralFailed bool
	// TriageOK reports that the Triage call grade passed.
	TriageOK bool
}

// ObservationsPassed reports whether every counted observation passed.
func (t gradeTally) ObservationsPassed() bool { return t.Passed == t.Counted }

// Scorable reports whether the grades measure the model. A failed structural
// grade means the harness failed, so the assignment is INVALID_EVIDENCE: it is
// shown, carries no scores, and is not counted for or against the model.
func (t gradeTally) Scorable() bool { return !t.StructuralFailed }

// PassRate is the share of counted observations that passed.
func (t gradeTally) PassRate() float64 {
	if t.Counted == 0 {
		return 0
	}
	return float64(t.Passed) / float64(t.Counted)
}

// tallyDeterministicGrades counts each independent observation once. Derived
// grades restate other grades and structural grades are harness preconditions,
// so neither changes the counts. A grade with no basis is a construction error,
// and a derived grade that fails while every observation passed is a grader bug.
func tallyDeterministicGrades(grades []*evalv1.DeterministicGrade) (gradeTally, error) {
	var tally gradeTally
	derivedFailed := ""
	for _, grade := range grades {
		if grade == nil {
			continue
		}
		failed := grade.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
		judged := failed || grade.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
		switch grade.GetBasis() {
		case evalv1.GradeBasis_GRADE_BASIS_OBSERVATION:
			if grade.GetCriterionId() == triageGradeCriterionID {
				tally.TriageOK = judged && !failed
				continue
			}
			if judged {
				tally.Counted++
				if !failed {
					tally.Passed++
				}
			}
		case evalv1.GradeBasis_GRADE_BASIS_DERIVED:
			if failed && derivedFailed == "" {
				derivedFailed = grade.GetCriterionId()
			}
		case evalv1.GradeBasis_GRADE_BASIS_STRUCTURAL:
			if failed {
				tally.StructuralFailed = true
			}
		default:
			return gradeTally{}, fmt.Errorf("evaluation: tally grades: grade %q: %w", grade.GetCriterionId(), constants.ErrEvaluationGradeBasisMissing)
		}
	}
	if derivedFailed != "" && tally.ObservationsPassed() && !tally.StructuralFailed {
		return gradeTally{}, fmt.Errorf("evaluation: tally grades: derived grade %q: %w", derivedFailed, constants.ErrEvaluationDerivedGradeContradiction)
	}
	return tally, nil
}

// verdictFromGrades is the verdict of a completed assignment. A failed harness
// precondition or a malformed grade set is INVALID_EVIDENCE: it appears in the
// result and is never counted against the model. Otherwise the verdict is PASS
// when every counted observation passed and FAIL when one did not.
func verdictFromGrades(grades []*evalv1.DeterministicGrade) evalv1.EvaluationVerdictStatus {
	tally, err := tallyDeterministicGrades(grades)
	switch {
	case err != nil || tally.StructuralFailed:
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE
	case tally.ObservationsPassed():
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	default:
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	}
}

// storedScoresMatchGrades reports whether a result's decomposed scores are the
// ones the grader derives from its deterministic grades. The verifier calls it
// after the grades themselves verified against the trace, so the verdict, the
// scores, and the trace cannot drift apart without a digest-resealed forgery
// being caught.
func storedScoresMatchGrades(result *evalv1.EvaluationAssignmentResult) (bool, error) {
	derived, err := deriveScenarioDecomposedScores(result.GetAssignmentId(), result.GetDeterministicGrades())
	if err != nil {
		return false, err
	}
	stored := result.GetDecomposedScores()
	if len(stored) != len(derived) {
		return false, nil
	}
	for i := range derived {
		if !proto.Equal(stored[i], derived[i]) {
			return false, nil
		}
	}
	return true, nil
}

// DeterministicPassRate is the published share of counted observations that
// passed. It reads the score the grader derived from the same tally as the
// verdict, and reports false when the result carries none (INVALID_EVIDENCE, or
// no counted observation).
func DeterministicPassRate(result *evalv1.EvaluationAssignmentResult) (float64, bool) {
	for _, score := range result.GetDecomposedScores() {
		if score.GetDimension() == deterministicPassRateID {
			return score.GetValue(), true
		}
	}
	return 0, false
}

// GradeBasisLabel is the basis name an operator reads next to a grade.
func GradeBasisLabel(basis evalv1.GradeBasis) string {
	switch basis {
	case evalv1.GradeBasis_GRADE_BASIS_OBSERVATION:
		return "observation"
	case evalv1.GradeBasis_GRADE_BASIS_DERIVED:
		return "derived"
	case evalv1.GradeBasis_GRADE_BASIS_STRUCTURAL:
		return "structural"
	default:
		return "unspecified"
	}
}
