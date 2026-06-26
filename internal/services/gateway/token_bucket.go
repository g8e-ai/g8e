// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
