// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// External test package: cmdtest imports shared, so the evidence-backup tests
// that need cmdtest's hermetic environment cannot live in package shared.
package shared_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

var priorSnapshotTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

var runReportRelPath = path.Join(constants.EvaluationDataPath, "runs", "run-1", "report.json")

type evidenceFixture struct {
	fileSvc fs.RuntimeFileService
	cfg     *config.Config
}

func newEvidenceFixture(t *testing.T) *evidenceFixture {
	t.Helper()
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	return &evidenceFixture{fileSvc: fileSvc, cfg: cfg}
}

func (f *evidenceFixture) writeEvidence(t *testing.T, rel, body string) {
	t.Helper()
	require.NoError(t, f.fileSvc.WriteFile(context.Background(), rel, []byte(body), constants.PermFilePrivate))
}

func (f *evidenceFixture) backup() *shared.PreCleanBackup {
	return shared.EvalEvidenceBackup(cmdtest.ConfigLoaderFor(f.cfg), cmdtest.FileSvcFactoryFor(f.fileSvc))
}

// snapshotDirs lists the snapshot directories under the default backup dir.
func (f *evidenceFixture) snapshotDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(shared.DefaultEvalBackupDir(f.cfg))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	return dirs
}

func TestEvalEvidenceBackup_PromptNamesTheDefaultBackupDirectory(t *testing.T) {
	backup := newEvidenceFixture(t).backup()

	assert.Equal(t, "Back up evaluation evidence to "+constants.EvaluationBackupDefaultDir+" first?", backup.Prompt)
}

func TestEvalEvidenceBackup_RunSnapshotsEvidenceIntoTheDefaultBackupDirectory(t *testing.T) {
	f := newEvidenceFixture(t)
	f.writeEvidence(t, runReportRelPath, `{"run":1}`)

	summary, err := f.backup().Run(context.Background())

	require.NoError(t, err)
	dirs := f.snapshotDirs(t)
	require.Len(t, dirs, 1)
	assert.True(t, strings.HasPrefix(dirs[0], constants.EvaluationBackupDirPrefix), dirs[0])
	assert.Contains(t, summary, "Backed up evaluation evidence (1 file(s), 9 bytes) to ")
	assert.Contains(t, summary, shared.DefaultEvalBackupDir(f.cfg))
}

func TestEvalEvidenceBackup_RunReportsAlreadyBackedUpWhenEvidenceMatchesTheNewestSnapshot(t *testing.T) {
	f := newEvidenceFixture(t)
	f.writeEvidence(t, runReportRelPath, `{"run":1}`)
	prior, err := evaluation.NewEvalBackup(f.fileSvc, func() time.Time { return priorSnapshotTime }).
		Create(context.Background(), shared.DefaultEvalBackupDir(f.cfg))
	require.NoError(t, err)

	summary, err := f.backup().Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Evaluation evidence already backed up in "+prior.SnapshotDir, summary)
	assert.Len(t, f.snapshotDirs(t), 1, "an unchanged run must not leave a second snapshot behind")
}

func TestEvalEvidenceBackup_RunTakesANewSnapshotWhenEvidenceChangedSinceTheLastOne(t *testing.T) {
	f := newEvidenceFixture(t)
	f.writeEvidence(t, runReportRelPath, `{"run":1}`)
	_, err := evaluation.NewEvalBackup(f.fileSvc, func() time.Time { return priorSnapshotTime }).
		Create(context.Background(), shared.DefaultEvalBackupDir(f.cfg))
	require.NoError(t, err)
	f.writeEvidence(t, runReportRelPath, `{"run":1,"status":"completed"}`)

	summary, err := f.backup().Run(context.Background())

	require.NoError(t, err)
	assert.Contains(t, summary, "Backed up evaluation evidence")
	assert.NotContains(t, summary, "already backed up")
	assert.Len(t, f.snapshotDirs(t), 2)
}

func TestEvalEvidenceBackup_RunReturnsSentinelWhenThereIsNoEvidence(t *testing.T) {
	f := newEvidenceFixture(t)

	summary, err := f.backup().Run(context.Background())

	assert.Empty(t, summary)
	require.ErrorIs(t, err, constants.ErrEvaluationBackupEmpty)
	assert.Empty(t, f.snapshotDirs(t))
}

func TestEvalEvidenceBackup_RunAsksTheConfigLoaderForTheDefaultProjectRoot(t *testing.T) {
	f := newEvidenceFixture(t)
	var loaderArgs []string
	backup := shared.EvalEvidenceBackup(
		func(projectRoot string) (*config.Config, error) {
			loaderArgs = append(loaderArgs, projectRoot)
			return f.cfg, nil
		},
		func(root string, _ *slog.Logger) (fs.RuntimeFileService, error) {
			assert.Equal(t, f.cfg.ProjectRoot, root, "the file service must be rooted at the loaded project root")
			return f.fileSvc, nil
		},
	)

	_, err := backup.Run(context.Background())

	require.ErrorIs(t, err, constants.ErrEvaluationBackupEmpty)
	assert.Equal(t, []string{""}, loaderArgs)
}

func TestEvalEvidenceBackup_RunWrapsConfigLoadFailureAndSkipsTheFileService(t *testing.T) {
	cause := errors.New("config unreadable")
	factoryCalled := false
	backup := shared.EvalEvidenceBackup(
		func(string) (*config.Config, error) { return nil, cause },
		func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			factoryCalled = true
			return nil, nil
		},
	)

	summary, err := backup.Run(context.Background())

	assert.Empty(t, summary)
	require.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "load config")
	assert.False(t, factoryCalled)
}

func TestEvalEvidenceBackup_RunWrapsFileServiceInitFailure(t *testing.T) {
	f := newEvidenceFixture(t)
	cause := errors.New("runtime root not writable")
	backup := shared.EvalEvidenceBackup(cmdtest.ConfigLoaderFor(f.cfg), cmdtest.FailingFileSvcFactory(cause))

	summary, err := backup.Run(context.Background())

	assert.Empty(t, summary)
	require.ErrorIs(t, err, constants.ErrFileServiceInit)
	require.ErrorIs(t, err, cause)
}

func TestConfirmDestructive_WithEvidenceBackupReportsNothingToBackUpOnAnEmptyRuntime(t *testing.T) {
	f := newEvidenceFixture(t)
	cmd := &cobra.Command{Use: "clean"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))

	proceed, err := shared.ConfirmDestructive(cmd, shared.DestructiveOptions{AssumeYes: true, Backup: f.backup()})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Contains(t, out.String(), "No evaluation evidence found; nothing to back up.")
}

func TestConfirmDestructive_WithEvidenceBackupPrintsTheSnapshotSummaryBeforeProceeding(t *testing.T) {
	f := newEvidenceFixture(t)
	f.writeEvidence(t, runReportRelPath, `{"run":1}`)
	cmd := &cobra.Command{Use: "clean"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))

	proceed, err := shared.ConfirmDestructive(cmd, shared.DestructiveOptions{AssumeYes: true, Backup: f.backup()})

	require.NoError(t, err)
	assert.True(t, proceed)
	assert.Contains(t, out.String(), "Backed up evaluation evidence (1 file(s), 9 bytes)")
	assert.Len(t, f.snapshotDirs(t), 1)
}
