// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestPlatformEnrollmentGate_GrantsUpToLimitWithoutQueueing(t *testing.T) {
	gate := newPlatformEnrollmentGate(2, time.Second)

	releaseA, waitedA, err := gate.acquire(t.Context())
	require.NoError(t, err)
	releaseB, _, err := gate.acquire(t.Context())
	require.NoError(t, err)

	assert.Less(t, waitedA, 100*time.Millisecond)
	assert.EqualValues(t, 2, gate.inFlight.Load())
	assert.EqualValues(t, 0, gate.waiting.Load())

	releaseA()
	releaseB()
	assert.EqualValues(t, 0, gate.inFlight.Load())
	assert.Equal(t, platformEnrollmentGateStats{Admitted: 2, PeakInFlight: 2}, gate.stats(),
		"immediate admissions are not queued and record no wait")
}

func TestPlatformEnrollmentGate_WaiterTakesSlotWhenReleased(t *testing.T) {
	gate := newPlatformEnrollmentGate(1, 5*time.Second)
	release, _, err := gate.acquire(t.Context())
	require.NoError(t, err)

	type result struct {
		release func()
		err     error
	}
	got := make(chan result, 1)
	go func() {
		r, _, err := gate.acquire(t.Context())
		got <- result{r, err}
	}()
	require.Eventually(t, func() bool { return gate.waiting.Load() == 1 }, time.Second, time.Millisecond)

	release()

	select {
	case r := <-got:
		require.NoError(t, r.err)
		assert.EqualValues(t, 1, gate.inFlight.Load())
		r.release()
	case <-time.After(5 * time.Second):
		t.Fatal("waiter was not admitted after the slot was released")
	}
	assert.EqualValues(t, 0, gate.waiting.Load())
	stats := gate.stats()
	assert.EqualValues(t, 2, stats.Admitted)
	assert.EqualValues(t, 1, stats.Queued)
	assert.EqualValues(t, 1, stats.PeakInFlight)
	assert.Positive(t, stats.MaxWait)
}

func TestPlatformEnrollmentGate_CancelledWaiterLeavesNoSlotHeld(t *testing.T) {
	gate := newPlatformEnrollmentGate(1, 5*time.Second)
	release, _, err := gate.acquire(t.Context())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		_, _, err := gate.acquire(ctx)
		errCh <- err
	}()
	require.Eventually(t, func() bool { return gate.waiting.Load() == 1 }, time.Second, time.Millisecond)

	cancel()

	require.ErrorIs(t, <-errCh, context.Canceled)
	assert.EqualValues(t, 1, gate.inFlight.Load(), "only the original holder keeps a slot")
	assert.EqualValues(t, 0, gate.waiting.Load())

	release()
	releaseAgain, _, err := gate.acquire(t.Context())
	require.NoError(t, err, "the cancelled waiter must not have consumed the slot")
	releaseAgain()
	stats := gate.stats()
	assert.EqualValues(t, 1, stats.Cancelled)
	assert.EqualValues(t, 2, stats.Admitted)
	assert.Zero(t, stats.Queued)
}

func TestPlatformEnrollmentGate_QueueWaitIsBoundedAndTyped(t *testing.T) {
	gate := newPlatformEnrollmentGate(1, 50*time.Millisecond)
	release, _, err := gate.acquire(t.Context())
	require.NoError(t, err)
	defer release()

	start := time.Now()
	_, waited, err := gate.acquire(t.Context())

	require.ErrorIs(t, err, constants.ErrPlatformEnrollmentRateLimited)
	assert.GreaterOrEqual(t, waited, 50*time.Millisecond)
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.EqualValues(t, 1, gate.inFlight.Load())
	stats := gate.stats()
	assert.EqualValues(t, 1, stats.RateLimited)
	assert.GreaterOrEqual(t, stats.MaxWait, 50*time.Millisecond)
}
