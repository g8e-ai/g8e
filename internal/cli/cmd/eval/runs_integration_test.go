// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package eval

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestRunsList_EmptyProject(t *testing.T) {
	env := setupRunEnv(t)

	assert.Contains(t, env.mustRun(t, "runs", "list"), "No runs found")
}

func TestRunsList_AfterPrepare(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	out := env.mustRun(t, "runs", "list")
	assert.Contains(t, out, "run-a-1")
	assert.Contains(t, out, "eval-a")
	assert.Contains(t, out, "scheduled")

	var payload runListJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "list"))
	require.Len(t, payload.Runs, 1)
	assert.Equal(t, "run-a-1", payload.Runs[0].RunID)
	assert.Equal(t, "eval-a", payload.Runs[0].CampaignID)
	assert.Equal(t, "scheduled", payload.Runs[0].Status)
	assert.Equal(t, uint64(41), payload.Runs[0].ExpectedAssignments)
	assert.False(t, payload.Runs[0].Archived)
}

func TestRunsList_FiltersByCampaignAndStatus(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.prepareRun(t, "eval-b", "run-b-1")

	var payload runListJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "list", "--campaign", "eval-b"))
	require.Len(t, payload.Runs, 1)
	assert.Equal(t, "run-b-1", payload.Runs[0].RunID)

	require.NoError(t, env.runJSON(t, &payload, "runs", "list", "--status", "verified"))
	assert.Empty(t, payload.Runs)

	require.NoError(t, env.runJSON(t, &payload, "runs", "list", "--status", "scheduled"))
	assert.Len(t, payload.Runs, 2)
}

func TestRunsShow_AfterPrepare(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	out := env.mustRun(t, "runs", "show", "run-a-1")
	assert.Contains(t, out, "Run: run-a-1")
	assert.Contains(t, out, "Campaign: eval-a")
	assert.Contains(t, out, "Status: scheduled")
	assert.Contains(t, out, "Expected assignments: 41")

	var payload runShowJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "show", "run-a-1"))
	assert.Equal(t, "run-a-1", payload.RunID)
	assert.Equal(t, "eval-a", payload.CampaignID)
	assert.Equal(t, testInferenceSession, payload.InferenceSession)
	assert.Equal(t, testDataSession, payload.DataSession)
	assert.Equal(t, uint32(41), payload.Queued)
	assert.Nil(t, payload.Holder)
}

func TestRunsShow_RejectsMissingRun(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "runs", "show", "missing-run-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs show")
}

func TestRunsShow_ReportsInterruptedWhenNoProcessHoldsARunningAssignment(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	markFirstAssignmentRunning(t, env)

	var payload runShowJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "show", "run-a-1"))
	assert.Equal(t, "interrupted", payload.Status)
	assert.Equal(t, uint32(1), payload.Running)
}

func TestRunsShow_ReportsRunningOnlyWhileALiveProcessHoldsTheLease(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	markFirstAssignmentRunning(t, env)
	_, err := env.store(t).AcquireRunLease(context.Background(), evaluation.RunLease{RunID: "run-a-1", PID: testPID, Host: testHost, StartedAt: env.deps.now(), LogPath: "eval/logs/run-a-1/execution-1.txt"}, nil)
	require.NoError(t, err)

	var payload runShowJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "show", "run-a-1"))
	assert.Equal(t, "running", payload.Status)
	require.NotNil(t, payload.Holder)
	assert.Equal(t, testPID, payload.Holder.PID)

	out := env.mustRun(t, "runs", "show", "run-a-1")
	assert.Contains(t, out, "Held by: process 4242 on test-host")

	env.control.setAlive(testPID, false)
	var stalePayload runShowJSON
	require.NoError(t, env.runJSON(t, &stalePayload, "runs", "show", "run-a-1"))
	assert.Equal(t, "interrupted", stalePayload.Status, "a lease whose process is gone no longer counts as running")
	assert.Nil(t, stalePayload.Holder)
}

func TestRunsVerify_CoverageReportsIncompleteScheduledRun(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	out, err := env.run(t, "runs", "verify", "run-a-1", "--coverage")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalRunVerificationFailed)
	assert.Contains(t, out, "incomplete")

	var payload runCoverageJSON
	err = env.runJSON(t, &payload, "runs", "verify", "run-a-1", "--coverage")
	require.Error(t, err)
	assert.False(t, payload.Complete)
	assert.Equal(t, uint64(41), payload.ExpectedCells)
}

func TestRunsVerify_CoverageCannotCombineWithWitnessRequirements(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "runs", "verify", "run-a-1", "--coverage", "--require-observation")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationFlagsInvalid)
}

func TestRunsVerify_RejectsMissingRun(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "runs", "verify", "missing-run-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs verify")
}

func TestRunsExport_WritesArtifacts(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	outputRelDir := filepath.Join("data", "eval", "runs", "run-a-1", "export")

	out := env.mustRun(t, "runs", "export", "run-a-1", "--output-dir", outputRelDir)
	assert.Contains(t, out, "run-a-1")

	exists, err := env.fileSvc(t).FileExists(context.Background(), filepath.Join(outputRelDir, constants.EvaluationRunSummaryFilename))
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRunsExport_RejectsExternalOutputDir(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "runs", "export", "run-a-1", "--output-dir", filepath.Join(env.root, "exports", "run-a-1"))
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationExportDirNotRelative)
}

func TestRunsExport_RequiresOutputDir(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "runs", "export", "run-a-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output-dir")
}

func TestRunsRepair_TraceDigests(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	assert.Contains(t, env.mustRun(t, "runs", "repair", "run-a-1", "--trace-digests"), "Repaired")

	var payload runRepairJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "repair", "run-a-1", "--trace-digests"))
	assert.Equal(t, "run-a-1", payload.RunID)
}

func TestRunsRepair_Results(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	assert.Contains(t, env.mustRun(t, "runs", "repair", "run-a-1", "--results"), "Repaired")
}

func TestRunsRepair_RequiresExactlyOneMode(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	for _, args := range [][]string{
		{"runs", "repair", "run-a-1"},
		{"runs", "repair", "run-a-1", "--results", "--trace-digests"},
	} {
		_, err := env.run(t, args...)
		require.Error(t, err, strings.Join(args, " "))
		assert.ErrorIs(t, err, constants.ErrEvaluationFlagsInvalid, strings.Join(args, " "))
	}
}

func TestRunsStart_DryRunPrintsPlanWithoutWriting(t *testing.T) {
	env := setupRunEnv(t)
	env.createCampaign(t, "eval-a")
	env.deps.authLoader = func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
		return nil, errors.New("CLI auth should not be loaded for a dry run")
	}

	out := env.mustRun(t, "runs", "start", "eval-a", "--dry-run")
	assert.Contains(t, out, "Run start plan")
	assert.Contains(t, out, "eval-a")
	assert.Contains(t, out, "Sessions resolved: false")

	runIDs, err := env.store(t).ListRunIDs(context.Background())
	require.NoError(t, err)
	assert.Empty(t, runIDs)
}

func TestRunsStart_DryRunJSON(t *testing.T) {
	env := setupRunEnv(t)
	env.createCampaign(t, "eval-a")

	var payload runStartPlanJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "start", "eval-a", "--dry-run"))
	assert.Equal(t, "eval-a", payload.CampaignID)
	assert.Equal(t, uint64(41), payload.CellCount)
	assert.False(t, payload.SessionsResolved)
	assert.Empty(t, payload.InferenceSession)
	assert.Empty(t, payload.DataSession)
	assert.Equal(t, []string{"qwen3:4b"}, payload.ModelTags)
}

func TestRunsStart_PrepareOnlyPersistsRunAndSchedulesAssignments(t *testing.T) {
	env := setupRunEnv(t)
	env.createCampaign(t, "eval-a")

	out := env.mustRun(t, "runs", "start", "eval-a", "--prepare-only", "--publish=false", "--daemon=false")
	assert.Contains(t, out, "Started run")
	assert.Contains(t, out, "Scheduled 41 assignments")
	assert.Contains(t, out, "g8e eval runs resume")

	runIDs, err := env.store(t).ListRunIDs(context.Background())
	require.NoError(t, err)
	require.Len(t, runIDs, 1)
	assert.True(t, strings.HasPrefix(runIDs[0], "eval-a-"), runIDs[0])

	assignments, err := env.store(t).ListAssignments(context.Background(), runIDs[0])
	require.NoError(t, err)
	assert.Len(t, assignments, 41)
}

func TestRunsStart_PrepareOnlyBindsTheOperatorSessions(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	run, err := env.store(t).LoadRun(context.Background(), "run-a-1")
	require.NoError(t, err)
	assert.Equal(t, testInferenceSession, run.GetCampaignBinding().GetInferenceOperatorSessionId())
	assert.Equal(t, testDataSession, run.GetCampaignBinding().GetDataOperatorSessionId())
}

func TestRunsStart_RejectsUnknownCampaign(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "runs", "start", "missing", "--dry-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs start")
}

func TestRunsStart_RequiresACampaignArgument(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "runs", "start")
	require.Error(t, err)
}

func TestRunsStart_ExecutesAndVerifiesWithoutPublication(t *testing.T) {
	env := setupRunEnv(t)
	env.enableWitnessGateway(t)
	publication := env.recordPublication()
	ensemble := newTestEnsembleServer(t, env.traceForAnyRun)
	env.createCampaign(t, "eval-a")

	var out bytes.Buffer
	env.cmd.SetOut(&out)
	result, err := runStartFlow(env.cmd, env.deps, runStartFlowOptions{
		CampaignID:  "eval-a",
		RunID:       "run-a-1",
		EnsembleURL: ensemble.URL,
		NoAutoBind:  true,
		Verify:      true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Executed)
	require.NotNil(t, result.Report)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, result.Report.GetStatus())
	assertPopulationBoundCampaignReport(t, result.Report)
	assert.Contains(t, out.String(), "verification")

	loaded, err := env.store(t).LoadCampaignVerification(context.Background(), "run-a-1")
	require.NoError(t, err)
	assertPopulationBoundCampaignReport(t, loaded)
	assert.Empty(t, publication.completionRunIDs)
	assert.Empty(t, publication.reports)
}

func TestRunsStart_ReleasesTheLeaseAndKeepsTheLogAfterExecution(t *testing.T) {
	env := setupRunEnv(t)
	env.enableWitnessGateway(t)
	ensemble := newTestEnsembleServer(t, env.traceForAnyRun)
	env.createCampaign(t, "eval-a")

	env.cmd.SetOut(&bytes.Buffer{})
	_, err := runStartFlow(env.cmd, env.deps, runStartFlowOptions{CampaignID: "eval-a", RunID: "run-a-1", EnsembleURL: ensemble.URL, NoAutoBind: true})
	require.NoError(t, err)

	_, err = env.store(t).LoadRunLease(context.Background(), "run-a-1")
	require.Error(t, err)
	assert.True(t, isMissingRecord(err), "the lease is released when execution ends")

	logPath, err := latestRunLogPath(context.Background(), env.fileSvc(t), "run-a-1")
	require.NoError(t, err)
	require.NotEmpty(t, logPath)
	body, err := env.fileSvc(t).ReadFile(context.Background(), logPath)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Executed")
}

func TestRunsStart_RejectsAnArchivedCampaign(t *testing.T) {
	env := setupRunEnv(t)
	env.createCampaign(t, "eval-a")
	env.mustRun(t, "campaigns", "archive", "eval-a")

	_, err := env.run(t, "runs", "start", "eval-a", "--dry-run")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationArchived)
}

func TestRunsResume_ExecutesOneAssignment(t *testing.T) {
	env := setupRunEnv(t)
	env.enableWitnessGateway(t)
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	ensemble, assignment := env.firstAssignmentServer(t, runID)

	var out bytes.Buffer
	env.cmd.SetOut(&out)
	executed, err := executeRun(env.cmd, env.deps, runExecuteOptions{
		RunID:       runID,
		Limit:       1,
		EnsembleURL: ensemble.URL,
		NoAutoBind:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, executed)
	assert.Contains(t, out.String(), assignment.GetAssignmentId())
}

func TestRunsResume_DefaultsLimitToOne(t *testing.T) {
	env := setupRunEnv(t)
	env.enableWitnessGateway(t)
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	ensemble, _ := env.firstAssignmentServer(t, runID)

	executed, err := executeRun(env.cmd, env.deps, runExecuteOptions{RunID: runID, EnsembleURL: ensemble.URL, NoAutoBind: true})
	require.NoError(t, err)
	assert.Equal(t, 1, executed)
}

func TestRunsResume_RejectsMissingRunWithoutLeavingALease(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "runs", "resume", "missing-run-id")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)

	runIDs, listErr := env.store(t).ListRunIDs(context.Background())
	require.NoError(t, listErr)
	assert.Empty(t, runIDs, "a failed resume must not create a run directory")
	assert.Contains(t, env.mustRun(t, "runs", "list"), "No runs found")
}

func TestRunsResume_RejectsPreflightWhenGatewayUnhealthy(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, false)
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "runs", "resume", "run-a-1", "--no-auto-bind")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)

	_, leaseErr := env.store(t).LoadRunLease(context.Background(), "run-a-1")
	assert.True(t, isMissingRecord(leaseErr), "a failed resume releases its lease")
}

func TestRunsResume_ExecutesOneAssignmentViaCLI(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	env := setupRunEnv(t)
	env.withExecuteGateway(t)
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	ensemble, assignment := env.firstAssignmentServer(t, runID)

	out := env.mustRun(t, "runs", "resume", runID, "--ensemble-url", ensemble.URL)
	assert.Contains(t, out, assignment.GetAssignmentId())
	assert.Contains(t, out, "Executed 1 assignment(s) for run run-a-1")
}

func TestRunsResume_JSONOutput(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	env := setupRunEnv(t)
	env.withExecuteGateway(t)
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	ensemble, _ := env.firstAssignmentServer(t, runID)

	var payload runResumeJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "resume", runID, "--ensemble-url", ensemble.URL))
	assert.Equal(t, runID, payload.RunID)
	assert.Equal(t, 1, payload.Executed)
	assert.NotEmpty(t, payload.Results)
	assert.Equal(t, int64(40), payload.Remaining)
}

func TestRunsResume_WithPublishFlag(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	env := setupRunEnv(t)
	env.withExecuteGateway(t)
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	ensemble, assignment := env.firstAssignmentServer(t, runID)

	out := env.mustRun(t, "runs", "resume", runID, "--ensemble-url", ensemble.URL, "--publish")
	assert.Contains(t, out, assignment.GetAssignmentId())
}

func TestRunsResume_RefusesARunHeldByALiveProcess(t *testing.T) {
	env := setupRunEnv(t)
	env.enableWitnessGateway(t)
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	_, err := env.store(t).AcquireRunLease(context.Background(), evaluation.RunLease{RunID: runID, PID: 9999, Host: testHost, StartedAt: env.deps.now()}, nil)
	require.NoError(t, err)
	env.control.setAlive(9999, true)

	executed, err := executeRun(env.cmd, env.deps, runExecuteOptions{RunID: runID, Limit: 1, NoAutoBind: true})
	require.Error(t, err)
	assert.Zero(t, executed)
	assert.ErrorIs(t, err, constants.ErrEvaluationRunLeaseHeld)
}

func TestRunsResume_ReplacesAStaleLeaseAndReportsIt(t *testing.T) {
	env := setupRunEnv(t)
	env.enableWitnessGateway(t)
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	ensemble, _ := env.firstAssignmentServer(t, runID)
	_, err := env.store(t).AcquireRunLease(context.Background(), evaluation.RunLease{RunID: runID, PID: 9999, Host: testHost, StartedAt: env.deps.now()}, nil)
	require.NoError(t, err)

	var out bytes.Buffer
	env.cmd.SetOut(&out)
	executed, err := executeRun(env.cmd, env.deps, runExecuteOptions{RunID: runID, Limit: 1, EnsembleURL: ensemble.URL, NoAutoBind: true})
	require.NoError(t, err)
	assert.Equal(t, 1, executed)
	assert.Contains(t, out.String(), "Cleared stale lease held by process 9999 on test-host")
}

func TestVerifyRun_PersistsAndPublishesPopulationBoundReport(t *testing.T) {
	env := setupRunEnv(t)
	env.enableWitnessGateway(t)
	publication := env.recordPublication()
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	ensemble, _ := env.firstAssignmentServer(t, runID)
	executed, err := executeRun(env.cmd, env.deps, runExecuteOptions{RunID: runID, Limit: 1, EnsembleURL: ensemble.URL, NoAutoBind: true})
	require.NoError(t, err)
	require.Equal(t, 1, executed)

	store := env.store(t)
	spec, err := store.LoadCampaignSpec(context.Background(), "eval-a")
	require.NoError(t, err)
	var out bytes.Buffer
	env.cmd.SetOut(&out)
	report, err := verifyRun(env.cmd, env.deps, store, runID, false, false, true, false)
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS, report.GetStatus())

	loaded, err := store.LoadCampaignVerification(context.Background(), runID)
	require.NoError(t, err)
	assert.Equal(t, report.GetStatus(), loaded.GetStatus())
	assertPopulationBoundCampaignReport(t, loaded)
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM, loaded.GetProviderObservationPolicy())
	assert.Equal(t, evalv1.EvaluationWitnessPolicy_EVALUATION_WITNESS_POLICY_INTERIM, loaded.GetModelProvenancePolicy())
	assert.Equal(t, spec.GetCampaignDigest(), loaded.GetCampaignDigest())
	assert.Equal(t, spec.GetCatalogDigest(), loaded.GetCatalogDigest())
	assert.Equal(t, spec.GetModelRegistryDigest(), loaded.GetModelRegistryDigest())
	assert.Equal(t, []string{runID}, publication.completionRunIDs)
	require.Len(t, publication.reports, 1)
	assertPopulationBoundCampaignReport(t, publication.reports[0])
	assert.Equal(t, loaded.GetReportDigestRef().GetSha256(), publication.reports[0].GetReportDigestRef().GetSha256())
}

func TestVerifyRun_RejectsMissingRun(t *testing.T) {
	env := setupRunEnv(t)

	report, err := verifyRun(env.cmd, env.deps, env.store(t), "missing-run-id", false, false, false, false)
	require.Error(t, err)
	assert.Nil(t, report)
	assert.Contains(t, err.Error(), "runs verify")
}

func TestRunsVerify_ViaCLIPersistsAndPublishesPopulationBoundReport(t *testing.T) {
	env := setupRunEnv(t)
	env.enableWitnessGateway(t)
	publication := env.recordPublication()
	ensemble := newTestEnsembleServer(t, env.traceForAnyRun)
	env.createCampaign(t, "eval-a")
	env.cmd.SetOut(&bytes.Buffer{})
	_, err := runStartFlow(env.cmd, env.deps, runStartFlowOptions{CampaignID: "eval-a", RunID: "run-a-1", EnsembleURL: ensemble.URL, NoAutoBind: true})
	require.NoError(t, err)

	out := env.mustRun(t, "runs", "verify", "run-a-1")
	assert.Contains(t, out, "PASS")

	loaded, err := env.store(t).LoadCampaignVerification(context.Background(), "run-a-1")
	require.NoError(t, err)
	assertPopulationBoundCampaignReport(t, loaded)
	assert.Equal(t, []string{"run-a-1"}, publication.completionRunIDs)
	require.Len(t, publication.reports, 1)
	assert.Equal(t, loaded.GetReportDigestRef().GetSha256(), publication.reports[0].GetReportDigestRef().GetSha256())

	var payload runVerifyJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "verify", "run-a-1"))
	assert.Equal(t, "run-a-1", payload.RunID)
	assert.Equal(t, evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS.String(), payload.Status)
}

func TestRunsPublish_PublishesScheduledRun(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	env := setupRunEnv(t)
	env.withPublishGateway(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	assert.Contains(t, env.mustRun(t, "runs", "publish", "run-a-1"), "Published")
}

func TestRunsPublish_ForceRepublish(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	env := setupRunEnv(t)
	env.withPublishGateway(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	assert.Contains(t, env.mustRun(t, "runs", "publish", "run-a-1", "--force"), "forced republish")
}

func TestRunsPublish_IgnoresLegacyFailedReport(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	env := setupRunEnv(t)
	env.withPublishGateway(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	require.NoError(t, env.store(t).SaveCampaignVerification(context.Background(), "run-a-1", &evalv1.EvaluationVerificationReport{
		SchemaVersion: evaluation.CampaignSchemaVersion,
		ReportId:      "run-a-1",
		RunId:         "run-a-1",
		Status:        evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		VerifiedAt:    timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
	}))

	env.mustRun(t, "runs", "publish", "run-a-1")
}

func TestRunsPublish_RejectsPersistedPassingReportThatDoesNotApply(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	env := setupRunEnv(t)
	env.withPublishGateway(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	require.NoError(t, env.store(t).SaveCampaignVerification(context.Background(), "run-a-1", &evalv1.EvaluationVerificationReport{
		SchemaVersion:            constants.CampaignVerifierVersion,
		ReportId:                 "run-a-1",
		RunId:                    "run-a-1",
		Status:                   evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		VerifiedAt:               timestamppb.New(time.Unix(1_700_000_200, 0).UTC()),
		VerifiedPopulationDigest: strings.Repeat("9", 64),
	}))

	_, err := env.run(t, "runs", "publish", "run-a-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
}

func TestRunsPublish_RejectsWhenGatewayUnhealthy(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, false)
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "runs", "publish", "run-a-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs publish")
}

func TestRunsCompare_ReportsSharedCellsAndDifferences(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.startPrepared(t, "eval-a", "run-a-2")

	var payload runCompareJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "compare", "run-a-1", "run-a-2"))
	assert.Equal(t, "run-a-1", payload.Left.RunID)
	assert.Equal(t, "run-a-2", payload.Right.RunID)
	assert.Equal(t, 41, payload.Left.Cells)
	assert.Equal(t, 41, payload.Right.Cells)
	assert.Empty(t, payload.OnlyInLeft)
	assert.Empty(t, payload.OnlyInRight)
	assert.Zero(t, payload.Regressions)
	assert.Zero(t, payload.Improvements)

	out := env.mustRun(t, "runs", "compare", "run-a-1", "run-a-2")
	assert.Contains(t, out, "run-a-1 (eval-a): 41 cells")
	assert.Contains(t, out, "Regressions: 0  Improvements: 0")
}

func TestRunsCompare_FlagsCellsOnlyInOneRun(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	env.createCampaignOver(t, "eval-b", &evalv1.ModelVariant{VariantId: "gemma3-4b", ServedModelTag: "gemma3:4b", ModelDigest: repeatTestHex('d'), ProviderClass: "ollama"})
	env.startPrepared(t, "eval-b", "run-b-1")

	var payload runCompareJSON
	require.NoError(t, env.runJSON(t, &payload, "runs", "compare", "run-a-1", "run-b-1"))
	assert.Len(t, payload.OnlyInLeft, 41, "no cell of a different model is shared")
	assert.Len(t, payload.OnlyInRight, 41)
}

func TestRunsCompare_RequiresTwoRuns(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "runs", "compare", "only-one")
	require.Error(t, err)
}

func TestPublicRestore_RequiresTarget(t *testing.T) {
	env := setupRunEnv(t)

	command := PublicRestoreCmdWithConfig(env.deps.configLoader, env.deps.fileSvcFactory)
	command.SetArgs([]string{"--project-root", env.root})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationFlagsInvalid)
}

func TestPublicRestore_RejectsBothTargets(t *testing.T) {
	env := setupRunEnv(t)

	command := PublicRestoreCmdWithConfig(env.deps.configLoader, env.deps.fileSvcFactory)
	command.SetArgs([]string{"--project-root", env.root, "--queue", "--run-id", "run-1"})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationFlagsInvalid)
}
