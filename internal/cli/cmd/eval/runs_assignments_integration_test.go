// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package eval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

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
