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
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// TestWriterGate_FIFOHandoff verifies that writers queueing for the writer gate
// acquire it in first-in, first-out (FIFO) order.
func TestWriterGate_FIFOHandoff(t *testing.T) {
	db, err := OpenDB(DefaultDBConfig(filepath.Join(testutil.TempDir(t), "fifo.db")), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// Hold the writer gate so subsequent attempts must queue.
	release, ok := db.TryAcquireWriter()
	require.True(t, ok, "initial writer gate acquisition must succeed")

	const numWaiters = 5
	entered := make([]chan struct{}, numWaiters)
	for i := range entered {
		entered[i] = make(chan struct{})
	}

	orderMu := sync.Mutex{}
	acquisitionOrder := make([]int, 0, numWaiters)

	var wg sync.WaitGroup
	wg.Add(numWaiters)

	for i := 0; i < numWaiters; i++ {
		idx := i
		go func() {
			defer wg.Done()
			close(entered[idx])
			if err := db.acquireWriter(context.Background()); err != nil {
				return
			}
			orderMu.Lock()
			acquisitionOrder = append(acquisitionOrder, idx)
			orderMu.Unlock()
			db.releaseWriter()
		}()
		// Wait until the goroutine is launched and ready, then give a small
		// slice for runtime scheduling to place it in the channel's wait queue.
		<-entered[idx]
		time.Sleep(10 * time.Millisecond)
	}

	// Release the initial gate; the queued goroutines should proceed in order.
	release()
	wg.Wait()

	require.Equal(t, []int{0, 1, 2, 3, 4}, acquisitionOrder, "writer gate handoff must preserve FIFO arrival order")
}

// TestWriterGate_ContextCancellationWhileQueued verifies that context cancellation
// unblocks a queued writer with ctx.Err() without leaking or poisoning the gate.
func TestWriterGate_ContextCancellationWhileQueued(t *testing.T) {
	db, err := OpenDB(DefaultDBConfig(filepath.Join(testutil.TempDir(t), "cancel.db")), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	release, ok := db.TryAcquireWriter()
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- db.acquireWriter(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for canceled writer to unblock")
	}

	// Releasing the holder should leave the gate uncorrupted and acquirable.
	release()

	release2, ok2 := db.TryAcquireWriter()
	require.True(t, ok2, "writer gate must remain acquirable after canceled waiter exits")
	release2()
}

// TestWriterGate_NoPooledConnectionHeldWhileQueued verifies that a writer waiting
// on the writer gate channel does NOT hold or check out any pooled database connection.
func TestWriterGate_NoPooledConnectionHeldWhileQueued(t *testing.T) {
	db, err := OpenDB(DefaultDBConfig(filepath.Join(testutil.TempDir(t), "no_conn.db")), testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// Configure pool to a single connection.
	db.SetMaxOpenConns(1)

	// Hold the writer gate before any query runs.
	release, ok := db.TryAcquireWriter()
	require.True(t, ok)

	// At this point, 0 connections should be checked out.
	require.Equal(t, 0, db.Stats().InUse, "no connection should be in use before write begins")

	var started atomic.Bool
	done := make(chan error, 1)

	go func() {
		started.Store(true)
		_, err := db.ExecWithRetry(context.Background(), "CREATE TABLE t (id INT)")
		done <- err
	}()

	// Wait for the goroutine to enter db.ExecWithRetry and block on db.acquireWriter.
	for i := 0; i < 50 && !started.Load(); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)

	// Crucial assertion: while queued on the gate channel, InUse must still be 0!
	assert.Equal(t, 0, db.Stats().InUse, "writer blocked in gate channel must NOT hold a database connection")

	// Release the gate, allowing the writer to acquire a connection and execute.
	release()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for queued ExecWithRetry to complete")
	}

	assert.Equal(t, 0, db.Stats().InUse, "connection must be returned to pool after execution")
}

// TestWriterGate_ZeroValueDBSafety verifies that a zero-value DB{} initializes
// the writer gate lazily and operates safely without nil channel panics.
func TestWriterGate_ZeroValueDBSafety(t *testing.T) {
	var db DB

	// TryAcquireWriter on zero-value DB.
	release, ok := db.TryAcquireWriter()
	require.True(t, ok, "TryAcquireWriter must succeed on zero-value DB")
	require.NotNil(t, release)
	release()

	// acquireWriter / releaseWriter on zero-value DB.
	ctx := context.Background()
	require.NoError(t, db.acquireWriter(ctx))
	db.releaseWriter()

	// Hold the gate so acquireWriter must wait on the channel:
	releaseHold, ok := db.TryAcquireWriter()
	require.True(t, ok)
	ctxCancel, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, db.acquireWriter(ctxCancel), context.Canceled)
	releaseHold()
}
