// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

var assignmentStartedAt = time.Unix(1_700_000_000, 0).UTC()

// saveGradedResult persists a terminal result for one assignment of a prepared
// run, 9 seconds after the assignment started.
func (e *runEnv) saveGradedResult(t *testing.T, assignment *evalv1.EvaluationAssignment, scores []*evalv1.DecomposedScoreRecord, grades ...*evalv1.DeterministicGrade) {
	t.Helper()
	ctx := context.Background()
	store := e.store(t)
	assignment.StartedAt = timestamppb.New(assignmentStartedAt)
	require.NoError(t, store.SaveAssignment(ctx, assignment))
	result := &evalv1.EvaluationAssignmentResult{
		SchemaVersion:       evaluation.CampaignSchemaVersion,
		AssignmentId:        assignment.GetAssignmentId(),
		RunId:               assignment.GetRunId(),
		CampaignId:          assignment.GetCampaignId(),
		Lane:                assignment.GetLane(),
		LifecycleStatus:     evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED,
		DeterministicGrades: grades,
		DecomposedScores:    scores,
		CompletedAt:         timestamppb.New(assignmentStartedAt.Add(9 * time.Second)),
	}
	digest, err := evaluation.ComputeAssignmentResultDigest(result)
	require.NoError(t, err)
	result.ResultDigest = digest
	require.NoError(t, store.SaveAssignmentResult(ctx, result))
}

type gradedRun struct {
	passed, missed, invalid, queued string
}

// seedGradedRun prepares a run and records three terminal results: one pass, one
// model miss (a failed observation and the derived grade that restates it), and
// one failed harness precondition.
func seedGradedRun(t *testing.T, env *runEnv) gradedRun {
	t.Helper()
	env.prepareRun(t, "eval-a", "run-a-1")
	assignments, err := env.store(t).ListAssignments(context.Background(), "run-a-1")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(assignments), 4)

	env.saveGradedResult(t, assignments[0], cellPassRate(1),
		cellGrade("scenario-content", basisObservation, gradePass),
		cellGrade("primary-responsibility", basisDerived, gradePass))
	env.saveGradedResult(t, assignments[1], cellPassRate(0.5),
		cellGrade("scenario-content", basisObservation, gradeFail),
		cellGrade("trajectory", basisObservation, gradePass),
		cellGrade("primary-responsibility", basisDerived, gradeFail))
	invalid := cellGrade("role-invoked", basisStructural, gradeFail)
	invalid.Detail = "designated model role was not invoked"
	env.saveGradedResult(t, assignments[2], nil, invalid, cellGrade("scenario-content", basisObservation, gradeFail))
	return gradedRun{
		passed:  assignments[0].GetAssignmentId(),
		missed:  assignments[1].GetAssignmentId(),
		invalid: assignments[2].GetAssignmentId(),
		queued:  assignments[3].GetAssignmentId(),
	}
}

func TestRunsAssignments_FailedListsEveryAssignmentThatDidNotPassWithItsCause(t *testing.T) {
	env := setupRunEnv(t)
	run := seedGradedRun(t, env)

	out := env.mustRun(t, "runs", "assignments", "run-a-1", "--failed")

	assert.Contains(t, out, run.missed)
	assert.Contains(t, out, "verdict=FAIL pass_rate=50.0%")
	assert.Contains(t, out, "scenario-content [observation]")
	assert.Contains(t, out, run.invalid)
	assert.Contains(t, out, "verdict=INVALID_EVIDENCE")
	assert.NotContains(t, out, "pass_rate=0.0%", "an unscored result states no rate")
	assert.Contains(t, out, "role-invoked [structural] (designated model role was not invoked)")
	assert.Contains(t, out, "9s", "the duration is shown")
	assert.NotContains(t, out, run.passed, "a passing assignment is not a failure")
	assert.NotContains(t, out, run.queued, "an assignment with no result has not failed")
}

func TestRunsAssignments_UnfilteredListsEveryAssignment(t *testing.T) {
	env := setupRunEnv(t)
	run := seedGradedRun(t, env)

	out := env.mustRun(t, "runs", "assignments", "run-a-1")

	for _, id := range []string{run.passed, run.missed, run.invalid, run.queued} {
		assert.Contains(t, out, id)
	}
	assert.Contains(t, out, "verdict=PASS pass_rate=100.0%")
}

func TestRunsAssignments_JSONStatesVerdictAndRateBesideCanonicalProtojson(t *testing.T) {
	env := setupRunEnv(t)
	run := seedGradedRun(t, env)

	var payload runAssignmentsJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "assignments", "run-a-1"))

	byID := make(map[string]runAssignmentJSON, len(payload.Assignments))
	for _, row := range payload.Assignments {
		byID[row.AssignmentID] = row
	}
	assert.Equal(t, "run-a-1", payload.RunID)

	passed := byID[run.passed]
	assert.Equal(t, "pass", passed.Verdict)
	require.NotNil(t, passed.PassRate)
	assert.InDelta(t, 1.0, *passed.PassRate, 1e-9)
	require.NotNil(t, passed.DurationMs)
	assert.Equal(t, int64(9000), *passed.DurationMs)
	assert.Contains(t, string(passed.Result), `"deterministic_grades"`, "the result is canonical protojson")
	assert.Contains(t, string(passed.Assignment), `"assignment_id"`)

	missed := byID[run.missed]
	assert.Equal(t, "fail", missed.Verdict)
	require.NotNil(t, missed.PassRate)
	assert.InDelta(t, 0.5, *missed.PassRate, 1e-9)

	invalid := byID[run.invalid]
	assert.Equal(t, invalidEvidenceVerdict, invalid.Verdict)
	assert.Nil(t, invalid.PassRate, "an unscored result states null, not a rate")

	queued := byID[run.queued]
	assert.Empty(t, queued.Verdict)
	assert.Empty(t, queued.Result)
	assert.Nil(t, queued.PassRate)
	assert.Nil(t, queued.DurationMs)
}

func TestRunsAssignments_AStatedZeroPassRateIsNotNull(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	assignments, err := env.store(t).ListAssignments(context.Background(), "run-a-1")
	require.NoError(t, err)
	env.saveGradedResult(t, assignments[0], cellPassRate(0), cellGrade("scenario-content", basisObservation, gradeFail))

	var payload runAssignmentsJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "assignments", "run-a-1", "--failed"))

	require.Len(t, payload.Assignments, 1)
	require.NotNil(t, payload.Assignments[0].PassRate, "protojson drops a zero double; the CLI view states it")
	assert.Zero(t, *payload.Assignments[0].PassRate)
}

func TestRunsAssignments_OneAssignmentShowsEveryGradeWithItsBasis(t *testing.T) {
	env := setupRunEnv(t)
	run := seedGradedRun(t, env)

	out := env.mustRun(t, "runs", "assignments", "run-a-1", "--assignment", run.missed)

	assert.Contains(t, out, "Grades for "+run.missed)
	for _, want := range []string{"scenario-content", "trajectory", "primary-responsibility", "observation", "derived"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, run.passed)
}

func TestRunsAssignments_UnknownAssignmentIsNotFound(t *testing.T) {
	env := setupRunEnv(t)
	seedGradedRun(t, env)

	_, err := env.run(t, "runs", "assignments", "run-a-1", "--assignment", "no-such-assignment")

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestExecutedAssignmentLine_NamesWhatRanHowLongAndHowItWent(t *testing.T) {
	t.Parallel()
	assignment := &evalv1.EvaluationAssignment{
		AssignmentId: "a-1",
		ScenarioId:   "security-policy-deny-delete",
		Repetition:   2,
		StartedAt:    timestamppb.New(assignmentStartedAt),
		Target: &evalv1.EvaluationAssignment_Homogeneous{Homogeneous: &evalv1.HomogeneousAssignmentTarget{
			CandidateVariant: &evalv1.ModelVariant{ServedModelTag: "gemma4:e4b"},
			DesignatedRole:   evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
		}},
	}
	result := completedResult(cellPassRate(0.5),
		cellGrade("scenario-content", basisObservation, gradeFail),
		cellGrade("trajectory", basisObservation, gradePass))
	result.AssignmentId = "a-1"
	result.CompletedAt = timestamppb.New(assignmentStartedAt.Add(9*time.Second + 40*time.Millisecond))

	line := executedAssignmentLine(assignment, result)

	assert.Equal(t,
		"Executed a-1 [security-policy-deny-delete | gemma4:e4b/primary | rep 2, 9s]: COMPLETED verdict=FAIL pass_rate=50.0% failed=[scenario-content [observation] ()]",
		line)
}
