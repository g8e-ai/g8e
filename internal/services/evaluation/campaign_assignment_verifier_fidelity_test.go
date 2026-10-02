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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func catalogRef(id, version string) *compliancev1.VersionedReference {
	return &compliancev1.VersionedReference{Id: id, Version: version}
}

func verifyAssignment(t *testing.T, f seededImportFixture, result *evalv1.EvaluationAssignmentResult, ref *compliancev1.VersionedReference) *evalv1.EvaluationVerificationReport {
	t.Helper()
	report, err := NewCampaignAssignmentVerifier(func() time.Time { return importNow.Add(time.Minute) }).Verify(context.Background(), CampaignAssignmentVerificationRequest{
		Assignment:    f.req.Assignment,
		Result:        result,
		ScenarioInput: f.req.ScenarioInput,
		ScenarioGold:  f.req.ScenarioGold,
		ScenarioTools: f.req.ScenarioTools,
		GradingMethod: f.req.GradingMethod,
		Trace:         f.trace,
		CatalogRef:    ref,
	})
	require.NoError(t, err)
	return report
}

func redigest(t *testing.T, result *evalv1.EvaluationAssignmentResult) {
	t.Helper()
	digest, err := ComputeAssignmentResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = digest
}

func TestCampaignAssignmentVerifier_RecomputesTheTrajectoryAndFailureReasonsItStored(t *testing.T) {
	t.Parallel()
	current := catalogRef(DefaultSuiteID, DefaultSuiteVersion)

	t.Run("an honest result verifies", func(t *testing.T) {
		t.Parallel()
		f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
		report := verifyAssignment(t, f, f.importResult(t), current)
		assert.Equal(t, verdictPass, report.GetStatus(), "%v", report.GetFailureReasons())
	})

	tamper := map[string]func(result *evalv1.EvaluationAssignmentResult){
		"the trajectory outcome": func(result *evalv1.EvaluationAssignmentResult) {
			result.TrajectoryOutcome = evalv1.EvaluationTrajectoryOutcome_EVALUATION_TRAJECTORY_OUTCOME_DIRECT
		},
		"the guided retry count":   func(result *evalv1.EvaluationAssignmentResult) { result.GuidedRetryCount = 5 },
		"the private failure text": func(result *evalv1.EvaluationAssignmentResult) { result.FailureReason = "made up" },
		"the public failure text": func(result *evalv1.EvaluationAssignmentResult) {
			result.PublicFailureReason = "The model made no tool call."
		},
	}
	for name, mutate := range tamper {
		t.Run("a re-digested result with a rewritten "+name+" fails verification", func(t *testing.T) {
			t.Parallel()
			f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
			result := f.importResult(t)
			mutate(result)
			redigest(t, result)

			report := verifyAssignment(t, f, result, current)

			assert.Equal(t, verdictFail, report.GetStatus())
			assert.Contains(t, report.GetFailureReasons(), "stored trajectory result does not match recomputation")
		})
	}

	t.Run("a legacy catalog result is not regraded, so its trajectory fields are not compared", func(t *testing.T) {
		t.Parallel()
		f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
		result := f.importResult(t)
		result.PublicFailureReason = "from an older grader"
		redigest(t, result)

		report := verifyAssignment(t, f, result, catalogRef(LegacyDefaultSuiteID, "1.0.0"))

		assert.Equal(t, verdictPass, report.GetStatus(), "%v", report.GetFailureReasons())
	})
}

// The scores are published and drive every pass rate, so the verifier recomputes
// them from the grades it just verified instead of trusting a resealed digest.
func TestCampaignAssignmentVerifier_RecomputesTheScoresItStored(t *testing.T) {
	t.Parallel()
	current := catalogRef(DefaultSuiteID, DefaultSuiteVersion)
	const wantFailure = "stored decomposed scores do not match recomputation"

	t.Run("the honest result carries scores and verifies", func(t *testing.T) {
		t.Parallel()
		f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
		result := f.importResult(t)
		require.NotEmpty(t, result.GetDecomposedScores(), "a result with no scores would verify vacuously")

		report := verifyAssignment(t, f, result, current)

		assert.Equal(t, verdictPass, report.GetStatus(), "%v", report.GetFailureReasons())
	})

	tamper := map[string]func(result *evalv1.EvaluationAssignmentResult){
		"a task score": func(result *evalv1.EvaluationAssignmentResult) {
			for _, score := range result.GetDecomposedScores() {
				if score.GetDimension() == "task_score" {
					score.Value = 1 - score.GetValue()
				}
			}
		},
		"a pass rate": func(result *evalv1.EvaluationAssignmentResult) {
			for _, score := range result.GetDecomposedScores() {
				if score.GetDimension() == "deterministic_pass_rate" {
					score.Value = 0.123
				}
			}
		},
		"every score dropped": func(result *evalv1.EvaluationAssignmentResult) { result.DecomposedScores = nil },
		"an invented score": func(result *evalv1.EvaluationAssignmentResult) {
			result.DecomposedScores = append(result.DecomposedScores, &evalv1.DecomposedScoreRecord{ScoreId: "invented", Dimension: "invented", Value: 1})
		},
	}
	for name, mutate := range tamper {
		t.Run("a re-digested result with "+name+" fails verification", func(t *testing.T) {
			t.Parallel()
			f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
			result := f.importResult(t)
			mutate(result)
			redigest(t, result)

			report := verifyAssignment(t, f, result, current)

			assert.Equal(t, verdictFail, report.GetStatus())
			assert.Contains(t, report.GetFailureReasons(), wantFailure)
			assert.NotContains(t, report.GetFailureReasons(), "stored deterministic grades do not match recomputation", "the grades are honest; only the scores drifted")
		})
	}

	t.Run("a legacy catalog result is not regraded, so its scores are not compared", func(t *testing.T) {
		t.Parallel()
		f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
		result := f.importResult(t)
		result.DecomposedScores = nil
		redigest(t, result)

		report := verifyAssignment(t, f, result, catalogRef(LegacyDefaultSuiteID, "1.0.0"))

		assert.Equal(t, verdictPass, report.GetStatus(), "%v", report.GetFailureReasons())
	})
}

// TestCampaignAssignmentVerifier_OldCatalogRunsKeepEveryIntegrityCheck is R8:
// a run frozen from an older built-in catalog skips only the regrading, never
// the digest and evidence checks.
func TestCampaignAssignmentVerifier_OldCatalogRunsKeepEveryIntegrityCheck(t *testing.T) {
	t.Parallel()
	oldRefs := map[string]*compliancev1.VersionedReference{
		"the pre-rename default suite id": catalogRef(LegacyDefaultSuiteID, "1.0.0"),
		"an older default suite version":  catalogRef(DefaultSuiteID, "1.0.0"),
	}
	for name, ref := range oldRefs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)

			t.Run("stored grades that differ from today's grader are accepted", func(t *testing.T) {
				result := f.importResult(t)
				result.DeterministicGrades = append(result.DeterministicGrades, &evalv1.DeterministicGrade{
					GradeId: "assignment-1:from-an-older-grader", CriterionId: "from-an-older-grader", Status: verdictFail, Detail: "produced by catalog 1.0.0",
				})
				redigest(t, result)
				report := verifyAssignment(t, f, result, ref)
				assert.Equal(t, verdictPass, report.GetStatus(), "%v", report.GetFailureReasons())
			})
			t.Run("the same drift under the current catalog fails", func(t *testing.T) {
				result := f.importResult(t)
				result.DeterministicGrades = append(result.DeterministicGrades, &evalv1.DeterministicGrade{
					GradeId: "assignment-1:from-an-older-grader", CriterionId: "from-an-older-grader", Status: verdictFail, Detail: "produced by catalog 1.0.0",
				})
				redigest(t, result)
				report := verifyAssignment(t, f, result, catalogRef(DefaultSuiteID, DefaultSuiteVersion))
				assert.Equal(t, verdictFail, report.GetStatus())
				assert.Contains(t, report.GetFailureReasons(), "stored deterministic grades do not match recomputation")
			})
			t.Run("an edited result without a fresh digest still fails", func(t *testing.T) {
				result := f.importResult(t)
				result.FailureReason = "edited after sealing"
				report := verifyAssignment(t, f, result, ref)
				assert.Equal(t, verdictFail, report.GetStatus())
				require.NotEmpty(t, report.GetFailureReasons())
				assert.Contains(t, report.GetFailureReasons()[0], "result digest validation failed")
			})
			t.Run("evidence that no longer matches the trace still fails", func(t *testing.T) {
				result := f.importResult(t)
				result.ModelInferences[0].PromptTokens = 99
				redigest(t, result)
				report := verifyAssignment(t, f, result, ref)
				assert.Equal(t, verdictFail, report.GetStatus())
				assert.Contains(t, report.GetFailureReasons(), "model inference 0 mismatch: "+constants.ErrEvaluationEvidenceTraceMismatch.Error())
			})
			t.Run("a tool call record rewritten after the fact still fails", func(t *testing.T) {
				result := f.importResult(t)
				result.ToolCalls[0].ErrorType = "forged"
				result.ToolCalls[0].GuidanceShown = true
				redigest(t, result)
				report := verifyAssignment(t, f, result, ref)
				assert.Equal(t, verdictFail, report.GetStatus())
				assert.Contains(t, report.GetFailureReasons(), "tool call record 0 mismatch: "+constants.ErrEvaluationEvidenceTraceMismatch.Error())
			})
			t.Run("a tool call record dropped from the result still fails", func(t *testing.T) {
				result := f.importResult(t)
				result.ToolCalls = nil
				redigest(t, result)
				report := verifyAssignment(t, f, result, ref)
				assert.Equal(t, verdictFail, report.GetStatus())
				assert.Contains(t, report.GetFailureReasons(), "tool call records mismatch: "+constants.ErrEvaluationEvidenceTraceMismatch.Error())
			})
			t.Run("a trace that no longer matches its digest still fails", func(t *testing.T) {
				result := f.importResult(t)
				tampered := f
				tampered.trace = EvaluationTrace{}
				for k, v := range f.trace {
					tampered.trace[k] = v
				}
				tampered.trace["designated_role_output"] = "edited after the digest"
				report := verifyAssignment(t, tampered, result, ref)
				assert.Equal(t, verdictFail, report.GetStatus())
				assert.Contains(t, report.GetFailureReasons()[0], "trace digest validation failed")
			})
		})
	}
}

func TestCampaignAssignmentVerifier_CurrentCatalogsAreAlwaysRegraded(t *testing.T) {
	t.Parallel()
	refs := map[string]*compliancev1.VersionedReference{
		"no catalog reference":   nil,
		"an empty version":       catalogRef(DefaultSuiteID, ""),
		"the current default":    catalogRef(DefaultSuiteID, DefaultSuiteVersion),
		"a custom suite":         catalogRef("my-suite", "3.1.4"),
		"a custom older version": catalogRef("my-suite", "0.0.1"),
	}
	for name, ref := range refs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
			result := f.importResult(t)
			result.DeterministicGrades[0].Detail = "forged detail"
			redigest(t, result)

			report := verifyAssignment(t, f, result, ref)

			assert.Equal(t, verdictFail, report.GetStatus())
			assert.Contains(t, report.GetFailureReasons(), "stored deterministic grades do not match recomputation")
		})
	}
}

func TestCampaignAssignmentVerifier_RejectsIncompleteRequests(t *testing.T) {
	t.Parallel()
	f := newSeededImportFixture(t, "recovery-error-guided-retry", nil)
	result := f.importResult(t)
	verifier := NewCampaignAssignmentVerifier(func() time.Time { return importNow })

	t.Run("no assignment", func(t *testing.T) {
		t.Parallel()
		report, err := verifier.Verify(context.Background(), CampaignAssignmentVerificationRequest{Result: result})
		require.NoError(t, err)
		assert.Equal(t, verdictFail, report.GetStatus())
		assert.Equal(t, []string{"assignment and result are required"}, report.GetFailureReasons())
	})
	t.Run("no result", func(t *testing.T) {
		t.Parallel()
		report, err := verifier.Verify(context.Background(), CampaignAssignmentVerificationRequest{Assignment: f.req.Assignment})
		require.NoError(t, err)
		assert.Equal(t, verdictFail, report.GetStatus())
	})
	t.Run("a result for another assignment", func(t *testing.T) {
		t.Parallel()
		other := proto.Clone(f.req.Assignment).(*evalv1.EvaluationAssignment)
		other.AssignmentId = "assignment-2"
		report, err := verifier.Verify(context.Background(), CampaignAssignmentVerificationRequest{Assignment: other, Result: result, Trace: f.trace})
		require.NoError(t, err)
		assert.Equal(t, verdictFail, report.GetStatus())
		assert.Contains(t, report.GetFailureReasons(), "assignment and result binding mismatch")
	})
	t.Run("no trace", func(t *testing.T) {
		t.Parallel()
		report, err := verifier.Verify(context.Background(), CampaignAssignmentVerificationRequest{Assignment: f.req.Assignment, Result: result})
		require.NoError(t, err)
		assert.Equal(t, verdictFail, report.GetStatus())
		assert.Contains(t, report.GetFailureReasons(), "imported assignment trace is required for verification")
	})
	t.Run("a cancelled context is an error, not a verdict", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		report, err := verifier.Verify(ctx, CampaignAssignmentVerificationRequest{Assignment: f.req.Assignment, Result: result, Trace: f.trace})
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, report)
	})
	t.Run("a nil context and clock default sensibly", func(t *testing.T) {
		t.Parallel()
		//nolint:staticcheck // the verifier documents a nil context as acceptable
		report, err := NewCampaignAssignmentVerifier(nil).Verify(nil, CampaignAssignmentVerificationRequest{Assignment: f.req.Assignment, Result: result, Trace: f.trace, ScenarioInput: f.req.ScenarioInput, ScenarioGold: f.req.ScenarioGold, ScenarioTools: f.req.ScenarioTools, GradingMethod: f.req.GradingMethod, CatalogRef: catalogRef(DefaultSuiteID, DefaultSuiteVersion)})
		require.NoError(t, err)
		assert.Equal(t, verdictPass, report.GetStatus(), "%v", report.GetFailureReasons())
		assert.False(t, report.GetVerifiedAt().AsTime().IsZero())
	})
	t.Run("a non-homogeneous target cannot be regraded", func(t *testing.T) {
		t.Parallel()
		noTarget := proto.Clone(f.req.Assignment).(*evalv1.EvaluationAssignment)
		noTarget.Target = nil
		report, err := verifier.Verify(context.Background(), CampaignAssignmentVerificationRequest{Assignment: noTarget, Result: result, Trace: f.trace})
		require.NoError(t, err)
		assert.Equal(t, verdictFail, report.GetStatus())
		// Failure reasons are persisted strings, so the typed sentinel is matched by its text.
		assert.Contains(t, strings.Join(report.GetFailureReasons(), "\n"), constants.ErrEvaluationTargetKindMismatch.Error())
	})
}

func TestVerifyImportedEvidence_CapturePresenceMustMatchTheTrace(t *testing.T) {
	t.Parallel()
	for _, field := range []struct {
		name   string
		mutate func(result *evalv1.EvaluationAssignmentResult)
		want   string
	}{
		{name: "tool decisions", mutate: func(result *evalv1.EvaluationAssignmentResult) {
			result.ToolDecisionsCaptured = !result.ToolDecisionsCaptured
		}, want: "tool decisions capture presence mismatch"},
		{name: "tool calls", mutate: func(result *evalv1.EvaluationAssignmentResult) { result.ToolCallsCaptured = !result.ToolCallsCaptured }, want: "tool calls capture presence mismatch"},
		{name: "governed actions", mutate: func(result *evalv1.EvaluationAssignmentResult) {
			result.GovernedActionsCaptured = !result.GovernedActionsCaptured
		}, want: "governed actions capture presence mismatch"},
		{name: "policy decisions", mutate: func(result *evalv1.EvaluationAssignmentResult) {
			result.PolicyDecisionsCaptured = !result.PolicyDecisionsCaptured
		}, want: "policy decisions capture presence mismatch"},
		{name: "scored span", mutate: func(result *evalv1.EvaluationAssignmentResult) {
			span := uint64(7)
			result.ScoredInferenceSpanNanos = &span
		}, want: "scored inference span mismatch"},
		{name: "policy decision dropped", mutate: func(result *evalv1.EvaluationAssignmentResult) { result.PolicyDecisions = nil }, want: "policy decision records mismatch"},
	} {
		t.Run(field.name, func(t *testing.T) {
			t.Parallel()
			f := newSeededImportFixture(t, "recovery-error-guided-retry", func(f *seededImportFixture) {
				f.trace["policy_decisions"] = []any{map[string]any{"decision_id": "d1", "tool_name": toolRun, "outcome": "deny", "detail": "blocked"}}
			})
			result := f.importResult(t)
			field.mutate(result)
			redigest(t, result)

			err := verifyImportedEvidence(f.req.Assignment, result, f.trace)

			require.Error(t, err)
			assert.Contains(t, err.Error(), field.want)
		})
	}
}
