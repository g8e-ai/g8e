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
		g.inFlight.Add(1)
		return g.release, time.Since(start), nil
	default:
	}

	g.waiting.Add(1)
	defer g.waiting.Add(-1)
	timer := time.NewTimer(g.maxWait)
	defer timer.Stop()
	select {
	case g.slots <- struct{}{}:
		g.inFlight.Add(1)
		return g.release, time.Since(start), nil
	case <-ctx.Done():
		return nil, time.Since(start), fmt.Errorf("platform enrollment: wait for admission: %w", context.Cause(ctx))
	case <-timer.C:
		return nil, time.Since(start), constants.ErrPlatformEnrollmentRateLimited
	}
}

func (g *platformEnrollmentGate) release() {
	g.inFlight.Add(-1)
	<-g.slots
}
