// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

var evalBackupTestNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

const (
	backupRunReportPath  = "data/eval/runs/run-1/report.json"
	backupRunLeasePath   = "data/eval/runs/run-1/lease.json"
	backupActiveRunPath  = "data/eval/active-run.json"
	backupCampaignPath   = "data/eval/campaigns/camp-1/campaign-spec.json"
	backupInventoryPath  = "eval/model-inventory.json"
	backupQueueLogPath   = "eval/logs/queue.log"
	backupUnrelatedPath  = "data/g8e.db"
	backupSnapshotPrefix = "eval-backup-20260930T120000Z"
)

type evalBackupFixture struct {
	files   fs.RuntimeFileService
	backup  *EvalBackup
	outRoot string
}

func newEvalBackupFixture(t *testing.T) *evalBackupFixture {
	t.Helper()
	files, err := fs.NewRuntimeFileService(testutil.TempDir(t), nil)
	require.NoError(t, err)
	require.NoError(t, files.CreateRuntimeTree(context.Background()))
	f := &evalBackupFixture{
		files:   files,
		backup:  NewEvalBackup(files, func() time.Time { return evalBackupTestNow }),
		outRoot: filepath.Join(testutil.TempDir(t), "backups"),
	}
	f.write(t, backupRunReportPath, `{"run":1}`)
	f.write(t, backupRunLeasePath, `{"pid":1}`)
	f.write(t, backupActiveRunPath, `{"run":"run-1"}`)
	f.write(t, backupCampaignPath, `{"campaign":1}`)
	f.write(t, backupInventoryPath, `{"variants":[]}`)
	f.write(t, backupQueueLogPath, "queued\n")
	f.write(t, backupUnrelatedPath, "not evaluation data")
	return f
}

func (f *evalBackupFixture) write(t *testing.T, rel, body string) {
	t.Helper()
	require.NoError(t, f.files.WriteFile(context.Background(), rel, []byte(body), constants.PermFilePrivate))
}

func (f *evalBackupFixture) read(t *testing.T, rel string) string {
	t.Helper()
	data, err := f.files.ReadFile(context.Background(), rel)
	require.NoError(t, err)
	return string(data)
}

func (f *evalBackupFixture) create(t *testing.T) *EvalBackupReport {
	t.Helper()
	report, err := f.backup.Create(context.Background(), f.outRoot)
	require.NoError(t, err)
	return report
}

func reportPaths(files []EvalBackupFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func TestEvalBackup_CreateCopiesEvidenceOnly(t *testing.T) {
	f := newEvalBackupFixture(t)

	report := f.create(t)

	assert.Equal(t, filepath.Join(f.outRoot, backupSnapshotPrefix), report.SnapshotDir)
	assert.Equal(t, []string{backupCampaignPath, backupRunReportPath, backupQueueLogPath, backupInventoryPath}, reportPaths(report.Files))
	for _, file := range report.Files {
		copied, err := os.ReadFile(filepath.Join(report.SnapshotDir, filepath.FromSlash(file.Path)))
		require.NoError(t, err)
		assert.Equal(t, f.read(t, file.Path), string(copied))
		assert.Equal(t, evalBackupDigest(copied), file.SHA256)
	}
	for _, excluded := range []string{backupRunLeasePath, backupActiveRunPath, backupUnrelatedPath} {
		_, err := os.Stat(filepath.Join(report.SnapshotDir, filepath.FromSlash(excluded)))
		assert.ErrorIs(t, err, os.ErrNotExist, excluded)
	}
	var manifest EvalBackupManifest
	raw, err := os.ReadFile(filepath.Join(report.SnapshotDir, constants.EvaluationBackupManifestFilename))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &manifest))
	assert.Equal(t, report.Files, manifest.Files)
}

func TestEvalBackup_CreateRejectsDestinationInsideRuntime(t *testing.T) {
	f := newEvalBackupFixture(t)

	for _, dest := range []string{
		f.files.Resolve(""),
		f.files.Resolve("data/eval/backups"),
		filepath.Join(f.files.Resolve("data"), "..", constants.RuntimeDirname, "x"),
	} {
		_, err := f.backup.Create(context.Background(), dest)
		assert.ErrorIs(t, err, constants.ErrEvaluationBackupDestinationInvalid, dest)
	}
	_, err := f.backup.Create(context.Background(), " ")
	assert.ErrorIs(t, err, constants.ErrEvaluationBackupDestinationInvalid)
}

func TestEvalBackup_CreateRejectsSymlinkedDestinationIntoRuntime(t *testing.T) {
	f := newEvalBackupFixture(t)
	link := filepath.Join(testutil.TempDir(t), "link")
	testutil.Symlink(t, f.files.Resolve(""), link)

	_, err := f.backup.Create(context.Background(), filepath.Join(link, "backups"))

	assert.ErrorIs(t, err, constants.ErrEvaluationBackupDestinationInvalid)
}

func TestEvalBackup_CreateEmptyRuntimeFails(t *testing.T) {
	files, err := fs.NewRuntimeFileService(testutil.TempDir(t), nil)
	require.NoError(t, err)
	require.NoError(t, files.CreateRuntimeTree(context.Background()))
	backup := NewEvalBackup(files, func() time.Time { return evalBackupTestNow })
	out := filepath.Join(testutil.TempDir(t), "backups")

	_, err = backup.Create(context.Background(), out)

	assert.ErrorIs(t, err, constants.ErrEvaluationBackupEmpty)
	_, statErr := os.Stat(out)
	assert.ErrorIs(t, statErr, os.ErrNotExist, "a failed backup must not leave a destination behind")
}

func TestEvalBackup_CreateSameSecondPreservesSnapshots(t *testing.T) {
	f := newEvalBackupFixture(t)
	first := f.create(t)
	f.write(t, backupRunReportPath, `{"run":2}`)

	second := f.create(t)
	third := f.create(t)

	assert.Equal(t, first.SnapshotDir+"-000001", second.SnapshotDir)
	assert.Equal(t, first.SnapshotDir+"-000002", third.SnapshotDir)
	data, err := os.ReadFile(filepath.Join(first.SnapshotDir, backupRunReportPath))
	require.NoError(t, err)
	assert.JSONEq(t, `{"run":1}`, string(data))
	data, err = os.ReadFile(filepath.Join(second.SnapshotDir, backupRunReportPath))
	require.NoError(t, err)
	assert.JSONEq(t, `{"run":2}`, string(data))
	latest, err := LatestEvalBackupSnapshot(f.outRoot)
	require.NoError(t, err)
	assert.Equal(t, third.SnapshotDir, latest)
}

func TestEvalBackup_CreateConcurrentSnapshots(t *testing.T) {
	f := newEvalBackupFixture(t)
	type outcome struct {
		report *EvalBackupReport
		err    error
	}
	const count = 4
	results := make(chan outcome, count)
	for i := 0; i < count; i++ {
		go func() {
			report, err := f.backup.Create(context.Background(), f.outRoot)
			results <- outcome{report, err}
		}()
	}
	directories := make(map[string]bool)
	for i := 0; i < count; i++ {
		result := <-results
		require.NoError(t, result.err)
		assert.False(t, directories[result.report.SnapshotDir])
		directories[result.report.SnapshotDir] = true
		assert.FileExists(t, filepath.Join(result.report.SnapshotDir, constants.EvaluationBackupManifestFilename))
	}
	assert.Len(t, directories, count)
}

func TestEvalBackup_RoundTripAfterWipe(t *testing.T) {
	f := newEvalBackupFixture(t)
	report := f.create(t)
	ctx := context.Background()
	require.NoError(t, f.files.RemoveAll(ctx, constants.EvaluationDataPath))
	require.NoError(t, f.files.RemoveAll(ctx, constants.EvaluationDirname))

	restored, err := f.backup.Restore(ctx, report.SnapshotDir, false)

	require.NoError(t, err)
	assert.Equal(t, report.Files, restored.Restored)
	assert.Empty(t, restored.Unchanged)
	assert.Equal(t, `{"run":1}`, f.read(t, backupRunReportPath))
	assert.Equal(t, "queued\n", f.read(t, backupQueueLogPath))
	leaseExists, err := f.files.FileExists(ctx, backupRunLeasePath)
	require.NoError(t, err)
	assert.False(t, leaseExists, "lease state must not be resurrected")
}

func TestEvalBackup_RestoreIsIdempotent(t *testing.T) {
	f := newEvalBackupFixture(t)
	report := f.create(t)

	restored, err := f.backup.Restore(context.Background(), report.SnapshotDir, false)

	require.NoError(t, err)
	assert.Empty(t, restored.Restored)
	assert.Equal(t, report.Files, restored.Unchanged)
}

func TestEvalBackup_RestoreConflictWritesNothingUnlessOverwrite(t *testing.T) {
	f := newEvalBackupFixture(t)
	report := f.create(t)
	ctx := context.Background()
	f.write(t, backupRunReportPath, `{"run":"changed"}`)
	require.NoError(t, f.files.Remove(ctx, backupCampaignPath))

	_, err := f.backup.Restore(ctx, report.SnapshotDir, false)

	require.ErrorIs(t, err, constants.ErrEvaluationBackupConflict)
	assert.Contains(t, err.Error(), backupRunReportPath)
	assert.Equal(t, `{"run":"changed"}`, f.read(t, backupRunReportPath))
	campaignExists, err := f.files.FileExists(ctx, backupCampaignPath)
	require.NoError(t, err)
	assert.False(t, campaignExists, "a refused restore must not write the files that were not in conflict")

	restored, err := f.backup.Restore(ctx, report.SnapshotDir, true)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{backupRunReportPath, backupCampaignPath}, reportPaths(restored.Restored))
	assert.Equal(t, `{"run":1}`, f.read(t, backupRunReportPath))
}

func TestEvalBackup_RestoreRejectsTamperedFile(t *testing.T) {
	f := newEvalBackupFixture(t)
	report := f.create(t)
	ctx := context.Background()
	require.NoError(t, os.WriteFile(filepath.Join(report.SnapshotDir, filepath.FromSlash(backupInventoryPath)), []byte("tampered"), constants.PermFilePrivate))
	require.NoError(t, f.files.RemoveAll(ctx, constants.EvaluationDataPath))

	_, err := f.backup.Restore(ctx, report.SnapshotDir, false)

	require.ErrorIs(t, err, constants.ErrEvaluationBackupIntegrity)
	exists, err := f.files.FileExists(ctx, backupRunReportPath)
	require.NoError(t, err)
	assert.False(t, exists, "integrity is verified for every file before any write")
}

func TestEvalBackup_RestoreRejectsMissingOrIncompleteManifest(t *testing.T) {
	f := newEvalBackupFixture(t)

	_, err := f.backup.Restore(context.Background(), filepath.Join(testutil.TempDir(t), "nothing"), false)
	assert.ErrorIs(t, err, constants.ErrEvaluationBackupManifestInvalid)

	report := f.create(t)
	require.NoError(t, os.Remove(filepath.Join(report.SnapshotDir, constants.EvaluationBackupManifestFilename)))
	_, err = f.backup.Restore(context.Background(), report.SnapshotDir, false)
	assert.ErrorIs(t, err, constants.ErrEvaluationBackupManifestInvalid)
}

func TestEvalBackup_RestoreRejectsManifestPathsOutsideEvidenceTrees(t *testing.T) {
	f := newEvalBackupFixture(t)
	digest := evalBackupDigest([]byte("x"))
	for _, bad := range []string{
		"pki/operator.key",
		"data/g8e.db",
		"../escape",
		"/etc/passwd",
		"data/eval/../../pki/ca.key",
		"data/eval/runs/run-1/lease.json",
		"eval",
	} {
		t.Run(bad, func(t *testing.T) {
			snapshot := t.TempDir()
			raw, err := json.Marshal(EvalBackupManifest{
				SchemaVersion: evalBackupManifestVersion,
				Files:         []EvalBackupFile{{Path: bad, Size: 1, SHA256: digest}},
			})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(snapshot, constants.EvaluationBackupManifestFilename), raw, constants.PermFilePrivate))

			_, err = f.backup.Restore(context.Background(), snapshot, true)

			assert.ErrorIs(t, err, constants.ErrEvaluationBackupManifestInvalid)
		})
	}
}

func (f *evalBackupFixture) laterBackup(after time.Duration) *EvalBackup {
	return NewEvalBackup(f.files, func() time.Time { return evalBackupTestNow.Add(after) })
}

func TestEvalBackup_CreateIfChangedSkipsIdenticalEvidence(t *testing.T) {
	f := newEvalBackupFixture(t)
	first := f.create(t)

	report, err := f.backup.CreateIfChanged(context.Background(), f.outRoot)

	require.NoError(t, err)
	assert.True(t, report.Unchanged)
	assert.Equal(t, first.SnapshotDir, report.SnapshotDir)
	assert.Equal(t, first.Files, report.Files)
	entries, err := os.ReadDir(f.outRoot)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "an unchanged backup must not leave a second snapshot")
}

func TestEvalBackup_CreateIfChangedKeepsSnapshotWhenEvidenceChanged(t *testing.T) {
	f := newEvalBackupFixture(t)
	first := f.create(t)
	f.write(t, backupRunReportPath, `{"run":2}`)

	report, err := f.backup.CreateIfChanged(context.Background(), f.outRoot)

	require.NoError(t, err)
	assert.False(t, report.Unchanged)
	assert.NotEqual(t, first.SnapshotDir, report.SnapshotDir)
	assert.FileExists(t, filepath.Join(first.SnapshotDir, constants.EvaluationBackupManifestFilename), "the earlier snapshot must survive")
	assert.FileExists(t, filepath.Join(report.SnapshotDir, constants.EvaluationBackupManifestFilename))
}

func TestEvalBackup_CreateIfChangedCreatesFirstSnapshot(t *testing.T) {
	f := newEvalBackupFixture(t)

	report, err := f.backup.CreateIfChanged(context.Background(), f.outRoot)

	require.NoError(t, err)
	assert.False(t, report.Unchanged)
	assert.Equal(t, filepath.Join(f.outRoot, backupSnapshotPrefix), report.SnapshotDir)
}

func TestEvalBackup_CreateIfChangedRejectsDestinationInsideRuntime(t *testing.T) {
	f := newEvalBackupFixture(t)

	_, err := f.backup.CreateIfChanged(context.Background(), f.files.Resolve("backups"))

	assert.ErrorIs(t, err, constants.ErrEvaluationBackupDestinationInvalid)
}

func TestLatestEvalBackupSnapshot_PicksNewestCompleteSnapshot(t *testing.T) {
	f := newEvalBackupFixture(t)
	older := f.create(t)
	newer, err := f.laterBackup(time.Minute).Create(context.Background(), f.outRoot)
	require.NoError(t, err)
	incomplete := filepath.Join(f.outRoot, constants.EvaluationBackupDirPrefix+"29990101T000000Z")
	require.NoError(t, os.Mkdir(incomplete, constants.PermDirPrivate))

	latest, err := LatestEvalBackupSnapshot(f.outRoot)

	require.NoError(t, err)
	assert.Equal(t, newer.SnapshotDir, latest)
	assert.NotEqual(t, older.SnapshotDir, latest)
}

func TestLatestEvalBackupSnapshot_NoneFound(t *testing.T) {
	for name, dir := range map[string]string{
		"missing directory": filepath.Join(testutil.TempDir(t), "absent"),
		"empty directory":   testutil.TempDir(t),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LatestEvalBackupSnapshot(dir)
			assert.ErrorIs(t, err, constants.ErrEvaluationBackupNone)
		})
	}
}
