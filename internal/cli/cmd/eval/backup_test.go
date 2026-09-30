// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestBackupRestore_RoundTripAfterWipe(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	fileSvc := env.fileSvc(t)
	ctx := context.Background()
	backupRoot := filepath.Join(testutil.TempDir(t), "my-eval-backups")

	var created evalBackupJSON
	require.NoError(t, env.runJSON(t, &created, "backup", "--output-dir", backupRoot))
	require.NotEmpty(t, created.Files)
	assert.Equal(t, len(created.Files), created.FileCount)
	assert.Equal(t, backupRoot, filepath.Dir(created.SnapshotDir))

	reportRel := filepath.ToSlash(filepath.Join(constants.EvaluationDataPath, constants.EvaluationRunsDirname, "run-a-1", constants.EvaluationReportFilename))
	want, err := fileSvc.ReadFile(ctx, reportRel)
	if err != nil {
		reportRel = created.Files[0].Path
		want, err = fileSvc.ReadFile(ctx, reportRel)
	}
	require.NoError(t, err)

	require.NoError(t, fileSvc.RemoveAll(ctx, constants.EvaluationDataPath))
	require.NoError(t, fileSvc.RemoveAll(ctx, constants.EvaluationDirname))

	var restored evalRestoreJSON
	require.NoError(t, env.runJSON(t, &restored, "restore", created.SnapshotDir))
	assert.Len(t, restored.Restored, created.FileCount)
	assert.Empty(t, restored.Unchanged)

	got, err := fileSvc.ReadFile(ctx, reportRel)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestBackup_RejectsOutputDirInsideRuntime(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "backup", "--output-dir", env.fileSvc(t).Resolve("eval-backups"))
	assert.ErrorIs(t, err, constants.ErrEvaluationBackupDestinationInvalid)
}

func TestBackupRestore_DefaultsToProjectEvalBackups(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	fileSvc := env.fileSvc(t)
	ctx := context.Background()

	var created evalBackupJSON
	require.NoError(t, env.runJSON(t, &created, "backup"))
	assert.Equal(t, filepath.Join(env.root, "eval", "backups"), filepath.Dir(created.SnapshotDir))

	require.NoError(t, fileSvc.RemoveAll(ctx, constants.EvaluationDataPath))
	require.NoError(t, fileSvc.RemoveAll(ctx, constants.EvaluationDirname))

	var restored evalRestoreJSON
	require.NoError(t, env.runJSON(t, &restored, "restore"))
	assert.Equal(t, created.SnapshotDir, restored.SnapshotDir)
	assert.Len(t, restored.Restored, created.FileCount)
}

func TestRestore_WithoutSnapshotAndNoDefaultBackupsFails(t *testing.T) {
	env := setupRunEnv(t)

	_, err := env.run(t, "restore")

	assert.ErrorIs(t, err, constants.ErrEvaluationBackupNone)
}

func TestAutoBackupEval_SnapshotsOnceThenSkipsUnchangedEvidence(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	var out bytes.Buffer
	env.cmd.SetOut(&out)
	backupDir := filepath.Join(env.root, "eval", "backups")

	autoBackupEval(env.cmd, env.deps, false)
	assert.Contains(t, out.String(), "Backed up evaluation evidence")
	snapshots, err := os.ReadDir(backupDir)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)

	out.Reset()
	env.deps.now = func() time.Time { return time.Unix(1789657337, 0).UTC().Add(time.Minute) }
	autoBackupEval(env.cmd, env.deps, false)
	assert.Contains(t, out.String(), "already backed up")
	snapshots, err = os.ReadDir(backupDir)
	require.NoError(t, err)
	assert.Len(t, snapshots, 1)
}

func TestAutoBackupEval_JSONModeStaysSilentOnStdout(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	var out bytes.Buffer
	env.cmd.SetOut(&out)

	autoBackupEval(env.cmd, env.deps, true)

	assert.Empty(t, out.String())
	assert.DirExists(t, filepath.Join(env.root, "eval", "backups"))
}

func TestAutoBackupEval_WarnsWithoutFailingWhenBackupIsImpossible(t *testing.T) {
	env := setupRunEnv(t)
	// A file where the backup directory belongs makes the snapshot impossible.
	require.NoError(t, os.MkdirAll(filepath.Join(env.root, "eval"), constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(filepath.Join(env.root, "eval", "backups"), []byte("in the way"), constants.PermFilePrivate))
	var stdout, stderr bytes.Buffer
	env.cmd.SetOut(&stdout)
	env.cmd.SetErr(&stderr)

	autoBackupEval(env.cmd, env.deps, false)

	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "warning: evaluation: automatic backup failed")
}

func TestRunsResume_BacksUpEvidenceWhenItFinishes(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	env := setupRunEnv(t)
	env.withExecuteGateway(t)
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	ensemble, _ := env.firstAssignmentServer(t, runID)

	out := env.mustRun(t, "runs", "resume", runID, "--ensemble-url", ensemble.URL)

	assert.Contains(t, out, "Backed up evaluation evidence")
	latest, err := evaluation.LatestEvalBackupSnapshot(filepath.Join(env.root, "eval", "backups"))
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(latest, constants.EvaluationDataPath, constants.EvaluationRunsDirname, runID))
}

func TestRunsResume_NoBackupFlagSkipsTheBackup(t *testing.T) {
	gwremote.WithGatewayHealthCheck(t, true)
	env := setupRunEnv(t)
	env.withExecuteGateway(t)
	runID := env.prepareRun(t, "eval-a", "run-a-1")
	ensemble, _ := env.firstAssignmentServer(t, runID)

	out := env.mustRun(t, "runs", "resume", runID, "--ensemble-url", ensemble.URL, "--no-backup")

	assert.NotContains(t, out, "Backed up")
	assert.NoDirExists(t, filepath.Join(env.root, "eval", "backups"))
}

func TestRestore_RejectsConflictingEvidenceWithoutOverwrite(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")
	backupRoot := filepath.Join(testutil.TempDir(t), "backups")
	var created evalBackupJSON
	require.NoError(t, env.runJSON(t, &created, "backup", "--output-dir", backupRoot))
	require.NoError(t, env.fileSvc(t).WriteFile(context.Background(), created.Files[0].Path, []byte("diverged"), constants.PermFilePrivate))

	_, err := env.run(t, "restore", created.SnapshotDir)
	require.ErrorIs(t, err, constants.ErrEvaluationBackupConflict)

	_, err = env.run(t, "restore", created.SnapshotDir, "--overwrite")
	require.NoError(t, err)
}
