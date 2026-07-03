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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewTokenBucket_InitialCapacity(t *testing.T) {
	tb := newTokenBucket(10, 5)
	assert.Equal(t, 5, tb.burst)
	assert.Equal(t, 5.0, tb.tokens)
	assert.Equal(t, 10.0, tb.rate)
}

func TestTokenBucket_AllowWithinBurst(t *testing.T) {
	tb := newTokenBucket(100, 3)
	for i := 0; i < 3; i++ {
		assert.True(t, tb.Allow(), "call %d should be allowed within burst", i)
	}
	assert.False(t, tb.Allow(), "4th call should be denied (bucket empty)")
}

func TestTokenBucket_RefillOverTime(t *testing.T) {
	tb := newTokenBucket(100, 1)
	assert.True(t, tb.Allow(), "first call consumes the only token")
	assert.False(t, tb.Allow(), "second call denied immediately")

	tb.last = tb.last.Add(-20 * time.Millisecond)
	assert.True(t, tb.Allow(), "after 20ms at 100 rps, ~2 tokens refilled")
}

func TestTokenBucket_RefillCappedAtBurst(t *testing.T) {
	tb := newTokenBucket(100, 2)
	tb.tokens = 0
	tb.last = tb.last.Add(-1 * time.Hour)
	assert.True(t, tb.Allow(), "first call after long wait")
	assert.True(t, tb.Allow(), "second call (refill capped at burst=2)")
	assert.False(t, tb.Allow(), "third call denied (no more than burst)")
}

func TestTokenBucket_ConcurrentAllow(t *testing.T) {
	tb := newTokenBucket(0, 100)
	done := make(chan bool, 200)
	allowed := 0
	var mu sync.Mutex

	for i := 0; i < 200; i++ {
		go func() {
			ok := tb.Allow()
			mu.Lock()
			if ok {
				allowed++
			}
			mu.Unlock()
			done <- true
		}()
	}

	for i := 0; i < 200; i++ {
		<-done
	}

	assert.LessOrEqual(t, allowed, 100, "should not allow more than burst")
	assert.Greater(t, allowed, 0, "should allow some calls")
}

func TestTokenBucket_ZeroRate(t *testing.T) {
	tb := newTokenBucket(0, 1)
	assert.True(t, tb.Allow(), "first call uses initial token")
	assert.False(t, tb.Allow(), "second call denied (no refill at 0 rate)")

	tb.last = tb.last.Add(-1 * time.Second)
	assert.False(t, tb.Allow(), "still denied after time (0 rate)")
}
