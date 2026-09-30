// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

var archiveTestNow = time.Unix(1_700_000_000, 0).UTC()

// archiveFixture is a real runtime tree holding one campaign with two
// scheduled runs, each one scenario across three roles.
type archiveFixture struct {
	files      fs.RuntimeFileService
	store      *Store
	controller *CampaignController
	req        CampaignInitRequest
	runIDs     []string
}

func newArchiveFixture(t *testing.T) *archiveFixture {
	t.Helper()
	files, err := fs.NewRuntimeFileService(testutil.TempDir(t), nil)
	require.NoError(t, err)
	store := NewStore(files)
	controller := NewCampaignController(store, &stubCampaignExecutor{}, func() time.Time { return archiveTestNow }, func(prefix string) string { return prefix + "-1" })

	req := testCampaignInitRequest(t)
	catalog := req.Catalog
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: catalog.GetSchemaVersion(),
		CatalogRef:    catalog.GetCatalogRef(),
		Scenarios:     catalog.GetScenarios()[:1],
	}
	digest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = digest
	req.Catalog = truncated

	fixture := &archiveFixture{files: files, store: store, controller: controller, req: req}
	for _, runID := range []string{"run-a", "run-b"} {
		req.RunID = runID
		_, err := controller.InitializeCampaign(context.Background(), req)
		require.NoError(t, err)
		count, err := controller.ScheduleHomogeneousRun(context.Background(), runID)
		require.NoError(t, err)
		require.Equal(t, 1, count)
		fixture.runIDs = append(fixture.runIDs, runID)
	}
	return fixture
}

func (f *archiveFixture) archiver(live bool) *Archiver {
	return NewArchiver(f.files, func() time.Time { return archiveTestNow }, func(RunLease) bool { return live })
}

func (f *archiveFixture) holdLease(t *testing.T, runID string) {
	t.Helper()
	_, err := f.store.AcquireRunLease(context.Background(), RunLease{RunID: runID, PID: 4242, Host: "host-1", StartedAt: archiveTestNow}, func(RunLease) bool { return false })
	require.NoError(t, err)
}

func TestArchiverArchiveRunMovesRunAndKeepsItReadable(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()

	manifest, err := f.archiver(false).ArchiveRun(ctx, "run-a", "operator-1")
	require.NoError(t, err)
	assert.Equal(t, ArchiveKindRun, manifest.Kind)
	assert.Equal(t, "run-a", manifest.ID)
	assert.Equal(t, f.req.CampaignID, manifest.CampaignID)
	assert.Equal(t, "operator-1", manifest.ArchivedBy)
	assert.Equal(t, archiveTestNow, manifest.ArchivedAt)

	active, err := f.store.RunExists(ctx, "run-a")
	require.NoError(t, err)
	assert.False(t, active, "archived run leaves the active tree")
	stillActive, err := f.store.RunExists(ctx, "run-b")
	require.NoError(t, err)
	assert.True(t, stillActive, "sibling run stays active")

	runIDs, err := f.store.ListRunIDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"run-b"}, runIDs)

	located, archived, err := LocateRun(ctx, f.files, "run-a")
	require.NoError(t, err)
	assert.True(t, archived)
	summary, err := NewCampaignController(located, nil, nil, nil).RunSummary(ctx, "run-a")
	require.NoError(t, err)
	assert.Equal(t, uint32(3), summary.QueuedCount, "archived run reads against its still-active campaign")

	saved, err := LoadArchiveManifest(ctx, f.files, ArchiveKindRun, "run-a")
	require.NoError(t, err)
	assert.Equal(t, "operator-1", saved.ArchivedBy)
}

func TestArchiverArchiveRunRejectsRunningAndArchivedRuns(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	f.holdLease(t, "run-a")

	_, err := f.archiver(true).ArchiveRun(ctx, "run-a", "operator-1")
	require.ErrorIs(t, err, constants.ErrEvaluationRunRunning)
	exists, err := f.store.RunExists(ctx, "run-a")
	require.NoError(t, err)
	assert.True(t, exists, "a refused archive moves nothing")

	_, err = f.archiver(false).ArchiveRun(ctx, "run-a", "operator-1")
	require.NoError(t, err, "a stale lease does not block archive")

	_, err = f.archiver(false).ArchiveRun(ctx, "run-a", "operator-1")
	require.ErrorIs(t, err, constants.ErrEvaluationArchived)

	_, err = f.archiver(false).ArchiveRun(ctx, "run-missing", "operator-1")
	require.ErrorIs(t, err, constants.ErrNotFound)
}

func TestArchiverUnarchiveRunRoundTrips(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()

	_, err := f.archiver(false).UnarchiveRun(ctx, "run-a")
	require.ErrorIs(t, err, constants.ErrEvaluationNotArchived)

	_, err = f.archiver(false).ArchiveRun(ctx, "run-a", "operator-1")
	require.NoError(t, err)
	manifest, err := f.archiver(false).UnarchiveRun(ctx, "run-a")
	require.NoError(t, err)
	assert.Equal(t, "operator-1", manifest.ArchivedBy)

	_, archived, err := LocateRun(ctx, f.files, "run-a")
	require.NoError(t, err)
	assert.False(t, archived)
	_, err = LoadArchiveManifest(ctx, f.files, ArchiveKindRun, "run-a")
	require.ErrorIs(t, err, constants.ErrNotFound, "the manifest is removed with the archive")
}

func TestArchiverArchiveCampaignMovesEveryRun(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()

	manifest, err := f.archiver(false).ArchiveCampaign(ctx, f.req.CampaignID, "operator-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"run-a", "run-b"}, manifest.RunIDs)

	for _, runID := range f.runIDs {
		located, archived, err := LocateRun(ctx, f.files, runID)
		require.NoError(t, err)
		assert.True(t, archived)
		summary, err := NewCampaignController(located, nil, nil, nil).RunSummary(ctx, runID)
		require.NoError(t, err, "an archived run reads against its archived campaign")
		assert.Equal(t, uint32(3), summary.QueuedCount)
	}
	_, archived, err := LocateCampaign(ctx, f.files, f.req.CampaignID)
	require.NoError(t, err)
	assert.True(t, archived)

	require.ErrorIs(t, RejectArchivedCampaign(ctx, f.files, f.req.CampaignID), constants.ErrEvaluationArchived)
	require.ErrorIs(t, RejectArchivedRun(ctx, f.files, "run-a"), constants.ErrEvaluationArchived)

	_, err = f.archiver(false).ArchiveCampaign(ctx, f.req.CampaignID, "operator-1")
	require.ErrorIs(t, err, constants.ErrEvaluationArchived)
}

func TestArchiverArchiveCampaignFailsBeforeMovingAnythingWhenARunIsRunning(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	f.holdLease(t, "run-b")

	_, err := f.archiver(true).ArchiveCampaign(ctx, f.req.CampaignID, "operator-1")
	require.ErrorIs(t, err, constants.ErrEvaluationRunRunning)

	for _, runID := range f.runIDs {
		exists, err := f.store.RunExists(ctx, runID)
		require.NoError(t, err)
		assert.True(t, exists, "run %s must not have moved", runID)
	}
	exists, err := f.store.CampaignExists(ctx, f.req.CampaignID)
	require.NoError(t, err)
	assert.True(t, exists)
	require.NoError(t, RejectArchivedCampaign(ctx, f.files, f.req.CampaignID))
}

func TestArchiverUnarchiveCampaignRestoresCampaignAndRuns(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()

	_, err := f.archiver(false).UnarchiveCampaign(ctx, f.req.CampaignID)
	require.ErrorIs(t, err, constants.ErrEvaluationNotArchived)

	_, err = f.archiver(false).ArchiveCampaign(ctx, f.req.CampaignID, "operator-1")
	require.NoError(t, err)

	_, err = f.archiver(false).UnarchiveRun(ctx, "run-a")
	require.ErrorIs(t, err, constants.ErrEvaluationArchived, "a run cannot leave the archive while its campaign is archived")

	_, err = f.archiver(false).UnarchiveCampaign(ctx, f.req.CampaignID)
	require.NoError(t, err)

	runIDs, err := f.store.ListRunIDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"run-a", "run-b"}, runIDs)
	require.NoError(t, RejectArchivedCampaign(ctx, f.files, f.req.CampaignID))
	summary, err := f.controller.RunSummary(ctx, "run-a")
	require.NoError(t, err)
	assert.Equal(t, uint32(3), summary.QueuedCount)
}

func TestArchiverUnarchiveCampaignLeavesIndividuallyArchivedRunsArchived(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()

	_, err := f.archiver(false).ArchiveRun(ctx, "run-a", "operator-1")
	require.NoError(t, err)
	manifest, err := f.archiver(false).ArchiveCampaign(ctx, f.req.CampaignID, "operator-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"run-b"}, manifest.RunIDs, "only active runs archive with the campaign")

	_, err = f.archiver(false).UnarchiveCampaign(ctx, f.req.CampaignID)
	require.NoError(t, err)

	_, archived, err := LocateRun(ctx, f.files, "run-a")
	require.NoError(t, err)
	assert.True(t, archived)
	_, archived, err = LocateRun(ctx, f.files, "run-b")
	require.NoError(t, err)
	assert.False(t, archived)
}
