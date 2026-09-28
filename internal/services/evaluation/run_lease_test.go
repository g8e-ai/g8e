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
)

func newLeaseStore(t *testing.T) *Store {
	t.Helper()
	files, err := fs.NewRuntimeFileService(testutil.TempDir(t), nil)
	require.NoError(t, err)
	return NewStore(files)
}

func leaseFixture(pid int) RunLease {
	return RunLease{
		RunID:     "run-1",
		PID:       pid,
		Host:      "host-1",
		StartedAt: time.Unix(1_700_000_000, 0).UTC(),
		LogPath:   "eval/logs/run-1/execution.txt",
	}
}

func liveness(alive bool) LeaseLiveness { return func(RunLease) bool { return alive } }

func TestRunLeaseAcquireWritesDurableHandle(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()

	_, err := store.LoadRunLease(ctx, "run-1")
	require.ErrorIs(t, err, constants.ErrNotFound)

	stale, err := store.AcquireRunLease(ctx, leaseFixture(100), liveness(false))
	require.NoError(t, err)
	assert.Nil(t, stale)

	loaded, err := store.LoadRunLease(ctx, "run-1")
	require.NoError(t, err)
	assert.Equal(t, 100, loaded.PID)
	assert.Equal(t, "host-1", loaded.Host)
	assert.Equal(t, "eval/logs/run-1/execution.txt", loaded.LogPath)
	assert.False(t, loaded.CancelRequested())
}

func TestRunLeaseAcquireRejectsLiveHolderAndReplacesStaleOne(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	_, err := store.AcquireRunLease(ctx, leaseFixture(100), liveness(false))
	require.NoError(t, err)

	_, err = store.AcquireRunLease(ctx, leaseFixture(200), liveness(true))
	require.ErrorIs(t, err, constants.ErrEvaluationRunLeaseHeld)
	loaded, err := store.LoadRunLease(ctx, "run-1")
	require.NoError(t, err)
	assert.Equal(t, 100, loaded.PID, "a held lease is left untouched")

	stale, err := store.AcquireRunLease(ctx, leaseFixture(200), liveness(false))
	require.NoError(t, err)
	require.NotNil(t, stale, "the stale lease is reported")
	assert.Equal(t, 100, stale.PID)
	loaded, err = store.LoadRunLease(ctx, "run-1")
	require.NoError(t, err)
	assert.Equal(t, 200, loaded.PID)
}

func TestRunLeaseAcquireRequiresRunAndProcess(t *testing.T) {
	store := newLeaseStore(t)
	_, err := store.AcquireRunLease(context.Background(), RunLease{RunID: "run-1"}, liveness(false))
	require.ErrorIs(t, err, constants.ErrEvaluationReportPersistFailed)
	_, err = store.AcquireRunLease(context.Background(), RunLease{PID: 1}, liveness(false))
	require.ErrorIs(t, err, constants.ErrEvaluationReportPersistFailed)
}

func TestRunLeaseReleaseRemovesOnlyTheHoldersLease(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	_, err := store.AcquireRunLease(ctx, leaseFixture(100), liveness(false))
	require.NoError(t, err)

	require.NoError(t, store.ReleaseRunLease(ctx, "run-1", 999))
	_, err = store.LoadRunLease(ctx, "run-1")
	require.NoError(t, err, "another process cannot release the lease")

	require.NoError(t, store.ReleaseRunLease(ctx, "run-1", 100))
	_, err = store.LoadRunLease(ctx, "run-1")
	require.ErrorIs(t, err, constants.ErrNotFound)

	require.NoError(t, store.ReleaseRunLease(ctx, "run-1", 100), "releasing an absent lease is a no-op")
}

func TestRunLeaseCancelRequestIsRecordedOnTheLease(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()

	_, err := store.RequestRunCancel(ctx, "run-1", time.Now())
	require.ErrorIs(t, err, constants.ErrNotFound)

	_, err = store.AcquireRunLease(ctx, leaseFixture(100), liveness(false))
	require.NoError(t, err)
	requestedAt := time.Unix(1_700_000_500, 0).UTC()
	lease, err := store.RequestRunCancel(ctx, "run-1", requestedAt)
	require.NoError(t, err)
	assert.True(t, lease.CancelRequested())

	loaded, err := store.LoadRunLease(ctx, "run-1")
	require.NoError(t, err)
	require.NotNil(t, loaded.CancelRequestedAt)
	assert.Equal(t, requestedAt, *loaded.CancelRequestedAt)
	assert.Equal(t, 100, loaded.PID, "cancel keeps the holder identity")
}

func TestRunLeaseClearRemovesAnyHolder(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	_, err := store.AcquireRunLease(ctx, leaseFixture(100), liveness(false))
	require.NoError(t, err)

	require.NoError(t, store.ClearRunLease(ctx, "run-1"))
	_, err = store.LoadRunLease(ctx, "run-1")
	require.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRunLeaseRejectsInvalidRunID(t *testing.T) {
	store := newLeaseStore(t)
	_, err := store.LoadRunLease(context.Background(), "../escape")
	require.ErrorIs(t, err, constants.ErrEvidenceArtifactMalformed)
	require.ErrorIs(t, store.ClearRunLease(context.Background(), ""), constants.ErrEvidenceArtifactMalformed)
}
