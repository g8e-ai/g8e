// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// TestGradeConsumers_AgreeOnEveryFixtureResult is the W5.4 agreement check over
// results the real import path produced from traces: the verdict, the published
// scores, the verifier's recomputation, and the execution-log line all read one
// tally, so none of them can tell a different story about the same result.
func TestGradeConsumers_AgreeOnEveryFixtureResult(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		scenarioID    string
		mutate        func(f *seededImportFixture)
		wantVerdict   evalv1.EvaluationVerdictStatus
		wantScored    bool
		wantTaskScore float64
		wantPassRate  float64
		wantTriageOK  float64
	}{
		{
			name:          "every counted observation passed",
			scenarioID:    "recovery-error-guided-retry",
			wantVerdict:   verdictPass,
			wantScored:    true,
			wantTaskScore: 1,
			wantPassRate:  1,
			wantTriageOK:  1,
		},
		{
			// The content check fails once; the derived `primary-responsibility`
			// restates it and is reported without being counted again. Five
			// observations count, one failed: 4/5.
			name:          "one missed observation counts once",
			scenarioID:    "recovery-error-guided-retry",
			mutate:        func(f *seededImportFixture) { f.trace["designated_role_output"] = "Nothing relevant was found." },
			wantVerdict:   verdictFail,
			wantScored:    true,
			wantTaskScore: 0,
			wantPassRate:  0.8,
			wantTriageOK:  1,
		},
		{
			name:          "a failed triage is reported on its own score and never fails the verdict",
			scenarioID:    "recovery-error-guided-retry",
			mutate:        func(f *seededImportFixture) { f.trace["triage_model_call"] = EvaluationTrace{"succeeded": false} },
			wantVerdict:   verdictPass,
			wantScored:    true,
			wantTaskScore: 1,
			wantPassRate:  1,
			wantTriageOK:  0,
		},
		{
			// The judge returned nothing, so the semantic grade is a failed harness
			// precondition. The failed content check beneath it is not a model result
			// either: the result is shown, carries no scores, and is not counted.
			name:        "a failed harness precondition is invalid evidence with no scores",
			scenarioID:  "final-response-diagnosis",
			wantVerdict: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newSeededImportFixture(t, tt.scenarioID, tt.mutate)
			result := f.importResult(t)
			scores := make(map[string]float64, len(result.GetDecomposedScores()))
			for _, score := range result.GetDecomposedScores() {
				scores[score.GetDimension()] = score.GetValue()
			}

			// Verdict.
			assert.Equal(t, tt.wantVerdict, DerivePublicSummaryStatus(result))

			// Published scores.
			if tt.wantScored {
				assert.InDelta(t, tt.wantTaskScore, scores["task_score"], 1e-9)
				assert.InDelta(t, tt.wantPassRate, scores["deterministic_pass_rate"], 1e-9)
				assert.InDelta(t, tt.wantTriageOK, scores["triage_ok"], 1e-9)
				assert.Equal(t, tt.wantVerdict == verdictPass, scores["task_score"] == 1, "the verdict is PASS exactly when the task score is 1")
				assert.Equal(t, tt.wantVerdict == verdictPass, scores["deterministic_pass_rate"] == 1, "the verdict is PASS exactly when every counted observation passed")
			} else {
				assert.Empty(t, scores, "an unscorable result publishes no scores")
			}

			// Verifier recomputation: grades and scores recompute from the trace.
			report := verifyAssignment(t, f, result, catalogRef(DefaultSuiteID, DefaultSuiteVersion))
			assert.Equal(t, verdictPass, report.GetStatus(), "%v", report.GetFailureReasons())

			// Execution-log line.
			line := AssignmentOutcomeSummary(result)
			require.Contains(t, line, fmt.Sprintf("verdict=%s", verdictLabel(tt.wantVerdict)))
			if tt.wantScored {
				assert.Contains(t, line, fmt.Sprintf("pass_rate=%.1f%%", tt.wantPassRate*100))
			} else {
				assert.NotContains(t, line, "pass_rate=")
			}
		})
	}
}

func verdictLabel(status evalv1.EvaluationVerdictStatus) string {
	return status.String()[len(verdictStatusPrefix):]
}
