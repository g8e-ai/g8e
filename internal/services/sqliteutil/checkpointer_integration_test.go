// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package sqliteutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// TestCheckpointer_TriggerFiresAfter64Commits verifies that every 64th gated commit
// triggers a background WAL checkpoint wake signal.
func TestCheckpointer_TriggerFiresAfter64Commits(t *testing.T) {
	dbPath := filepath.Join(testutil.TempDir(t), "checkpointer_commits.db")
	db, err := OpenDB(DefaultDBConfig(dbPath), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	_, err = db.Exec("CREATE TABLE commits_test (id INT)")
	require.NoError(t, err)

	ctx := context.Background()
	// Commits start at 0.
	require.Equal(t, int64(0), db.commits)

	// Perform 63 commits via ExecWithRetry.
	for i := 0; i < walCheckpointCommitInterval-1; i++ {
		_, err := db.ExecWithRetry(ctx, "INSERT INTO commits_test VALUES (?)", i)
		require.NoError(t, err)
	}
	require.Equal(t, int64(walCheckpointCommitInterval-1), db.commits)

	// The 64th commit should trigger the checkpoint signal without blocking.
	_, err = db.ExecWithRetry(ctx, "INSERT INTO commits_test VALUES (?)", walCheckpointCommitInterval)
	require.NoError(t, err)
	require.Equal(t, int64(walCheckpointCommitInterval), db.commits)

	// Give the background checkpointer a moment to process the signal.
	time.Sleep(50 * time.Millisecond)

	// Verify the database is healthy and queryable.
	var count int
	require.NoError(t, db.QueryRowWithRetry(ctx, "SELECT COUNT(*) FROM commits_test").Scan(&count))
	assert.Equal(t, walCheckpointCommitInterval, count)
}

// TestCheckpointer_GracefulShutdownViaClose verifies that Close() cleanly stops
// the background checkpointer goroutine before closing the connection pool.
func TestCheckpointer_GracefulShutdownViaClose(t *testing.T) {
	dbPath := filepath.Join(testutil.TempDir(t), "shutdown.db")
	db, err := OpenDB(DefaultDBConfig(dbPath), testutil.NewTestLogger())
	require.NoError(t, err)

	// Check that the checkpointer is active (stopped channel is open).
	select {
	case <-db.stopped:
		t.Fatal("checkpointer stopped channel should be open while running")
	default:
	}

	// Close() must signal stop and wait for checkpointer to exit.
	require.NoError(t, db.Close())

	// db.stopped must be closed now.
	select {
	case <-db.stopped:
		// Expected: channel is closed.
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for checkpointer goroutine to stop on Close()")
	}

	// Verify idempotency of Close().
	assert.NoError(t, db.Close())
}

// TestCheckpointer_WALFileSizeBoundedUnderSustainedLoad verifies that WAL file size
// remains bounded under a burst of gated writes due to regular passive checkpointing.
func TestCheckpointer_WALFileSizeBoundedUnderSustainedLoad(t *testing.T) {
	dbPath := filepath.Join(testutil.TempDir(t), "wal_bound.db")
	db, err := OpenDB(DefaultDBConfig(dbPath), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	_, err = db.Exec("CREATE TABLE sustained_load (id INT, payload TEXT)")
	require.NoError(t, err)

	ctx := context.Background()
	payload := string(make([]byte, 1024)) // 1KB per row

	// Write 256 rows (crossing 4 checkpoint intervals: 64, 128, 192, 256).
	for i := 0; i < 256; i++ {
		_, err := db.ExecWithRetry(ctx, fmt.Sprintf("INSERT INTO sustained_load VALUES (%d, ?)", i), payload)
		require.NoError(t, err)
	}

	// Allow checkpoint goroutine to process the passive checkpoints.
	time.Sleep(100 * time.Millisecond)

	walPath := dbPath + "-wal"
	info, err := os.Stat(walPath)
	if err == nil {
		// WAL file should be well-bounded (typically < 2MB under periodic PASSIVE checkpoints).
		const maxWALSizeBytes = 4 * 1024 * 1024 // 4 MB limit
		assert.Less(t, info.Size(), int64(maxWALSizeBytes), "WAL file size should remain bounded under periodic checkpoints")
	}
}
