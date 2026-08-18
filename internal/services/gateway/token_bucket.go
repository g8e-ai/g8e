// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"sync"
	"time"
)

// tokenBucket is a standard token-bucket rate limiter implementing the same
// semantics as golang.org/x/time/rate.Limiter for the subset of features used
// by this codebase (NewLimiter + Allow only).
type tokenBucket struct {
	mu     sync.Mutex
	rate   float64 // tokens per second
	burst  int     // maximum bucket capacity
	tokens float64
	last   time.Time
}

// newTokenBucket creates a token bucket with the given refill rate (tokens per
// second) and burst capacity.
func newTokenBucket(rps float64, burst int) *tokenBucket {
	return &tokenBucket{
		rate:   rps,
		burst:  burst,
		tokens: float64(burst),
		last:   time.Now(),
	}
}

// Allow returns true if one token is available, consuming it; false otherwise.
func (tb *tokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.last).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > float64(tb.burst) {
		tb.tokens = float64(tb.burst)
	}
	tb.last = now

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}
