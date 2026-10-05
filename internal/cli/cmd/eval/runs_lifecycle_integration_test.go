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
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func TestRunsCancel_StopsEveryAssignmentOfAnIdleRun(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	out := env.mustRun(t, "runs", "cancel", "run-a-1")
	assert.Contains(t, out, "Cancelled run run-a-1: stopped 41 assignment(s)")

	var show runShowJSON
	require.NoError(t, env.runJSON(t, &show, "runs", "show", "run-a-1"))
	assert.Equal(t, "cancelled", show.Status)
	assert.Equal(t, uint32(41), show.Stopped)
	assert.Zero(t, show.Queued)
}

func TestRunsCancel_JSONReportsWhatItStopped(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	var payload runCancelJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "cancel", "run-a-1"))
	assert.Equal(t, "run-a-1", payload.RunID)
	assert.Equal(t, 41, payload.Stopped)
	assert.False(t, payload.WasRunning)
	assert.False(t, payload.ClearedStale)
}

func TestRunsCancel_ClearsAStaleLeaseAndReportsIt(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.holdRun(t, "run-a-1", otherHostPID)
	env.control.setAlive(otherHostPID, false)

	out := env.mustRun(t, "runs", "cancel", "run-a-1")
	assert.Contains(t, out, "Cleared stale lease held by process 9999 on test-host")

	_, err := env.store(t).LoadRunLease(context.Background(), "run-a-1")
	assert.True(t, isMissingRecord(err))
}

func TestRunsCancel_AsksALiveProcessToStopAndWaitsForItToRelease(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.holdRun(t, "run-a-1", otherHostPID)

	type outcome struct {
		out string
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := env.run(t, "runs", "cancel", "run-a-1")
		done <- outcome{out, err}
	}()

	waitForCancelRequest(t, env, "run-a-1")
	select {
	case <-done:
		t.Fatal("cancel returned while the process still held the run")
	case <-time.After(50 * time.Millisecond):
	}
	env.control.setAlive(otherHostPID, false)

	select {
	case result := <-done:
		require.NoError(t, result.err, result.out)
		assert.Contains(t, result.out, "Waiting for process 9999")
		assert.Contains(t, result.out, "stopped 41 assignment(s)")
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not finish after the process exited")
	}
	assert.Equal(t, []int{otherHostPID}, env.control.interrupted())
	_, err := env.store(t).LoadRunLease(context.Background(), "run-a-1")
	assert.True(t, isMissingRecord(err))
}

func TestRunsCancel_RejectsMissingAndArchivedRuns(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "runs", "cancel", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs cancel")

	env.mustRun(t, "runs", "archive", "run-a-1")
	_, err = env.run(t, "runs", "cancel", "run-a-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationArchived)
}

func TestRunsResume_DoesNotResumeAStoppedRun(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.mustRun(t, "runs", "cancel", "run-a-1")

	store := env.store(t)
	controller := evaluation.NewCampaignController(store, nil, env.deps.now, func(prefix string) string { return prefix + "-id" })
	assignment, ok, err := controller.ResumeNextAssignment(context.Background(), "run-a-1")
	assert.False(t, ok, "cancelled run has no queued assignments to resume")
	assert.Nil(t, assignment)
	assert.NoError(t, err)
}

func TestRunsShow_WatchReturnsForASettledRun(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.mustRun(t, "runs", "cancel", "run-a-1")

	out := env.mustRun(t, "runs", "show", "run-a-1", "--watch")
	assert.Contains(t, out, "Status: cancelled")
}

func TestRunsShow_WatchFollowsARunUntilItSettles(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	done := make(chan string, 1)
	go func() {
		out, _ := env.run(t, "runs", "show", "run-a-1", "--watch")
		done <- out
	}()
	select {
	case <-done:
		t.Fatal("watch returned for a run that has not settled")
	case <-time.After(60 * time.Millisecond):
	}

	env.mustRun(t, "runs", "cancel", "run-a-1")
	select {
	case out := <-done:
		assert.Contains(t, out, "cancelled")
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not return after the run settled")
	}
}

func TestRunsLogs_PrintsTheMostRecentLogOfAFinishedRun(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	logDir := path.Join(constants.EvaluationQueueLogsDirname, "run-a-1")
	env.writeLog(t, path.Join(logDir, "execution-100.txt"), "older execution\n")
	env.writeLog(t, path.Join(logDir, "execution-200.txt"), "newer execution\n")

	out := env.mustRun(t, "runs", "logs", "run-a-1")
	assert.Contains(t, out, "newer execution")
	assert.NotContains(t, out, "older execution")
}

func TestRunsLogs_ReportsARunThatHasNoLog(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "runs", "logs", "run-a-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRunsLogs_RejectsMissingRun(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "runs", "logs", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs logs")
}

func TestRunsLogs_ReadsTheLiveLogOfAHeldRun(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.holdRun(t, "run-a-1", otherHostPID)
	env.writeLog(t, path.Join(constants.EvaluationQueueLogsDirname, "run-a-1", "execution-1.txt"), "live line\n")

	out := env.mustRun(t, "runs", "logs", "run-a-1")
	assert.Contains(t, out, "live line")
}

func TestRunsLogs_FollowPrintsAppendedOutputUntilTheProcessExits(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.holdRun(t, "run-a-1", otherHostPID)
	logPath := path.Join(constants.EvaluationQueueLogsDirname, "run-a-1", "execution-1.txt")
	env.writeLog(t, logPath, "first line\n")

	done := make(chan string, 1)
	go func() {
		out, _ := env.run(t, "runs", "logs", "run-a-1", "--follow")
		done <- out
	}()
	select {
	case <-done:
		t.Fatal("follow returned while the process still held the run")
	case <-time.After(60 * time.Millisecond):
	}

	env.writeLog(t, logPath, "second line\n")
	time.Sleep(30 * time.Millisecond)
	env.writeLog(t, logPath, "last line\n")
	env.control.setAlive(otherHostPID, false)

	select {
	case out := <-done:
		assert.Contains(t, out, "first line")
		assert.Contains(t, out, "second line")
		assert.Contains(t, out, "last line")
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not end after the process exited")
	}
}

func TestRunsArchive_RoundTrip(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	out := env.mustRun(t, "runs", "archive", "run-a-1")
	assert.Contains(t, out, "Archived run run-a-1")
	assert.Contains(t, out, "public record", "archiving must say the public projections remain")

	assert.Contains(t, env.mustRun(t, "runs", "list"), "No runs found")

	var listed runListJSON
	require.NoError(t, env.runJSON(t, &listed, "runs", "list", "--archived"))
	require.Len(t, listed.Runs, 1)
	assert.True(t, listed.Runs[0].Archived)

	var show runShowJSON
	require.NoError(t, env.runJSON(t, &show, "runs", "show", "run-a-1"))
	assert.True(t, show.Archived)
	assert.Equal(t, "scheduled", show.Status)

	out = env.mustRun(t, "runs", "unarchive", "run-a-1")
	assert.Contains(t, out, "Unarchived run run-a-1")
	require.NoError(t, env.runJSON(t, &listed, "runs", "list"))
	require.Len(t, listed.Runs, 1)
	assert.False(t, listed.Runs[0].Archived)
}

func TestRunsArchive_RecordsWhoArchivedAndWhen(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	var payload archiveResultJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "archive", "run-a-1"))
	assert.Equal(t, "run", payload.Kind)
	assert.Equal(t, "run-a-1", payload.ID)
	assert.Equal(t, "eval-a", payload.CampaignID)
	assert.Equal(t, "operator-1", payload.ArchivedBy)
	assert.NotEmpty(t, payload.ArchivedAt)
	assert.True(t, payload.Archived)
}

func TestRunsArchive_ArchivedRunsStayReadable(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.startPrepared(t, "eval-a", "run-a-2")
	env.mustRun(t, "runs", "archive", "run-a-1")

	_, err := env.run(t, "runs", "verify", "run-a-1", "--coverage")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalRunVerificationFailed, "an incomplete archived run is still verifiable, and reports as incomplete")

	env.mustRun(t, "runs", "export", "run-a-1", "--output-dir", "data/eval/exports/run-a-1")

	var compared runCompareJSON
	require.NoError(t, env.runJSON(t, &compared, "runs", "compare", "run-a-1", "run-a-2"))
	assert.Equal(t, 41, compared.Left.Cells)
	assert.Equal(t, 41, compared.Right.Cells)
}

func TestRunsArchive_ArchivedRunsAreRejectedByMutatingCommands(t *testing.T) {
	env := setupRunEnv(t)
	env.enableWitnessGateway(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.mustRun(t, "runs", "archive", "run-a-1")

	for _, args := range [][]string{
		{"runs", "resume", "run-a-1", "--no-auto-bind"},
		{"runs", "publish", "run-a-1"},
		{"runs", "repair", "run-a-1", "--results"},
		{"runs", "repair", "run-a-1", "--trace-digests"},
	} {
		_, err := env.run(t, args...)
		require.Error(t, err, strings.Join(args, " "))
		assert.ErrorIs(t, err, constants.ErrEvaluationArchived, strings.Join(args, " "))
	}
}

func TestRunsArchive_RefusesARunThatIsRunning(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.holdRun(t, "run-a-1", otherHostPID)

	_, err := env.run(t, "runs", "archive", "run-a-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationRunRunning)

	var listed runListJSON
	require.NoError(t, env.runJSON(t, &listed, "runs", "list"))
	require.Len(t, listed.Runs, 1, "a refused archive moves nothing")
}

func TestRunsArchive_ArchivesARunWhoseLeaseIsStale(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.holdRun(t, "run-a-1", otherHostPID)
	env.control.setAlive(otherHostPID, false)

	env.mustRun(t, "runs", "archive", "run-a-1")
}

func TestRunsArchive_RejectsBadTargets(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "runs", "archive", "missing")
	require.Error(t, err)

	env.mustRun(t, "runs", "archive", "run-a-1")
	_, err = env.run(t, "runs", "archive", "run-a-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationArchived)

	env.mustRun(t, "runs", "unarchive", "run-a-1")
	_, err = env.run(t, "runs", "unarchive", "run-a-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationNotArchived)
}

func TestRunsArchive_ARunArchivedAloneStaysListedUnderItsActiveCampaign(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.startPrepared(t, "eval-a", "run-a-2")
	env.mustRun(t, "runs", "archive", "run-a-1")

	var payload campaignShowJSON
	require.NoError(t, env.runJSON(t, &payload, "campaigns", "show", "eval-a"))
	require.Len(t, payload.Runs, 2)
	archived := map[string]bool{}
	for _, row := range payload.Runs {
		archived[row.RunID] = row.Archived
	}
	assert.Equal(t, map[string]bool{"run-a-1": true, "run-a-2": false}, archived)
}

func TestCampaignsArchive_ArchivesTheCampaignAndAllOfItsRuns(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.startPrepared(t, "eval-a", "run-a-2")
	env.prepareRun(t, "eval-b", "run-b-1")

	out := env.mustRun(t, "campaigns", "archive", "eval-a")
	assert.Contains(t, out, "Archived campaign eval-a and 2 run(s)")
	assert.Contains(t, out, "public record")

	var campaigns campaignListJSON
	require.NoError(t, env.runJSON(t, &campaigns, "campaigns", "list"))
	require.Len(t, campaigns.Campaigns, 1)
	assert.Equal(t, "eval-b", campaigns.Campaigns[0].CampaignID)

	require.NoError(t, env.runJSON(t, &campaigns, "campaigns", "list", "--archived"))
	require.Len(t, campaigns.Campaigns, 2)

	var runs runListJSON
	require.NoError(t, env.runJSON(t, &runs, "runs", "list"))
	require.Len(t, runs.Runs, 1)
	assert.Equal(t, "run-b-1", runs.Runs[0].RunID)

	var show campaignShowJSON
	require.NoError(t, env.runJSON(t, &show, "campaigns", "show", "eval-a"))
	assert.True(t, show.Archived)
	require.NotNil(t, show.Archive)
	assert.Equal(t, "operator-1", show.Archive.ArchivedBy)
	assert.Len(t, show.Runs, 2)
}

func TestCampaignsArchive_RoundTrip(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.mustRun(t, "campaigns", "archive", "eval-a")

	out := env.mustRun(t, "campaigns", "unarchive", "eval-a")
	assert.Contains(t, out, "Unarchived campaign eval-a and 1 run(s)")

	var campaigns campaignListJSON
	require.NoError(t, env.runJSON(t, &campaigns, "campaigns", "list"))
	require.Len(t, campaigns.Campaigns, 1)
	assert.False(t, campaigns.Campaigns[0].Archived)
	var runs runListJSON
	require.NoError(t, env.runJSON(t, &runs, "runs", "list"))
	require.Len(t, runs.Runs, 1)
	assert.Equal(t, "run-a-1", runs.Runs[0].RunID)

	_, err := env.run(t, "campaigns", "unarchive", "eval-a")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationNotArchived)
}

func TestCampaignsArchive_MovesNothingWhenARunIsRunning(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.startPrepared(t, "eval-a", "run-a-2")
	env.holdRun(t, "run-a-2", otherHostPID)

	_, err := env.run(t, "campaigns", "archive", "eval-a")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationRunRunning)

	var runs runListJSON
	require.NoError(t, env.runJSON(t, &runs, "runs", "list"))
	assert.Len(t, runs.Runs, 2, "no run moved to the archive")
	var campaigns campaignListJSON
	require.NoError(t, env.runJSON(t, &campaigns, "campaigns", "list"))
	require.Len(t, campaigns.Campaigns, 1)
	assert.False(t, campaigns.Campaigns[0].Archived)
}

func TestCampaignsArchive_ARunUnarchivedAloneNeedsItsCampaignActive(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.mustRun(t, "campaigns", "archive", "eval-a")

	_, err := env.run(t, "runs", "unarchive", "run-a-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationArchived)
	assert.Contains(t, err.Error(), "unarchive the campaign first")
}

func TestCampaignsCreate_RejectsAnArchivedCampaignID(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.mustRun(t, "campaigns", "archive", "eval-a")

	_, err := env.run(t, "campaigns", "create", "eval-a", "qwen3:4b")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationArchived)
}
