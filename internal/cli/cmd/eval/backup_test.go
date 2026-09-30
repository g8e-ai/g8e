// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
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

func TestBackup_RequiresOutputDirOutsideRuntime(t *testing.T) {
	env := setupRunEnv(t)
	env.prepareRun(t, "eval-a", "run-a-1")

	_, err := env.run(t, "backup")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output-dir")

	_, err = env.run(t, "backup", "--output-dir", env.fileSvc(t).Resolve("eval-backups"))
	assert.ErrorIs(t, err, constants.ErrEvaluationBackupDestinationInvalid)
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
