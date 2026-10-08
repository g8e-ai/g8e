// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// platformEnrollmentGate bounds how many enrollment operations of one kind
// run governed work at once. It is independent of pending-request capacity
// (stored requests) and of CLI staging parallelism (client launches): those
// bound state and fan-in, this bounds the Gateway's concurrent L1-L5 work.
//
// A caller that cannot obtain a slot waits in a bounded queue. The wait ends
// when the caller's context is cancelled (the client went away; nothing has
// been written yet) or when maxWait elapses, in which case the caller gets the
// typed rate-limited error so the HTTP layer answers 429 with Retry-After
// before the client's own deadline fires.
type platformEnrollmentGate struct {
	slots   chan struct{}
	maxWait time.Duration

	waiting  atomic.Int64
	inFlight atomic.Int64

	// Cumulative counters for phase reports; see stats.
	admitted     atomic.Int64
	queued       atomic.Int64
	rateLimited  atomic.Int64
	cancelled    atomic.Int64
	totalWaitNs  atomic.Int64
	maxWaitNs    atomic.Int64
	peakInFlight atomic.Int64
}

// platformEnrollmentGateStats is a snapshot of a gate's cumulative counters.
// Queued counts admissions that had to wait; RateLimited and Cancelled count
// waiters that left without a slot. Waits cover every outcome.
type platformEnrollmentGateStats struct {
	Admitted     int64
	Queued       int64
	RateLimited  int64
	Cancelled    int64
	TotalWait    time.Duration
	MaxWait      time.Duration
	PeakInFlight int64
}

func newPlatformEnrollmentGate(limit int, maxWait time.Duration) *platformEnrollmentGate {
	return &platformEnrollmentGate{slots: make(chan struct{}, limit), maxWait: maxWait}
}

// acquire returns a release function and the time spent queued. The release
// function must be called exactly once.
func (g *platformEnrollmentGate) acquire(ctx context.Context) (func(), time.Duration, error) {
	start := time.Now()
	select {
	case g.slots <- struct{}{}:
		g.admit()
		return g.release, time.Since(start), nil
	default:
	}

	g.waiting.Add(1)
	defer g.waiting.Add(-1)
	timer := time.NewTimer(g.maxWait)
	defer timer.Stop()
	select {
	case g.slots <- struct{}{}:
		g.admit()
		g.queued.Add(1)
		return g.release, g.recordWait(start), nil
	case <-ctx.Done():
		g.cancelled.Add(1)
		return nil, g.recordWait(start), fmt.Errorf("platform enrollment: wait for admission: %w", context.Cause(ctx))
	case <-timer.C:
		g.rateLimited.Add(1)
		return nil, g.recordWait(start), constants.ErrPlatformEnrollmentRateLimited
	}
}

func (g *platformEnrollmentGate) admit() {
	g.admitted.Add(1)
	n := g.inFlight.Add(1)
	for peak := g.peakInFlight.Load(); n > peak && !g.peakInFlight.CompareAndSwap(peak, n); peak = g.peakInFlight.Load() {
	}
}

func (g *platformEnrollmentGate) recordWait(start time.Time) time.Duration {
	waited := time.Since(start)
	g.totalWaitNs.Add(int64(waited))
	for peak := g.maxWaitNs.Load(); int64(waited) > peak && !g.maxWaitNs.CompareAndSwap(peak, int64(waited)); peak = g.maxWaitNs.Load() {
	}
	return waited
}

func (g *platformEnrollmentGate) release() {
	g.inFlight.Add(-1)
	<-g.slots
}

func (g *platformEnrollmentGate) stats() platformEnrollmentGateStats {
	return platformEnrollmentGateStats{
		Admitted:     g.admitted.Load(),
		Queued:       g.queued.Load(),
		RateLimited:  g.rateLimited.Load(),
		Cancelled:    g.cancelled.Load(),
		TotalWait:    time.Duration(g.totalWaitNs.Load()),
		MaxWait:      time.Duration(g.maxWaitNs.Load()),
		PeakInFlight: g.peakInFlight.Load(),
	}
}
