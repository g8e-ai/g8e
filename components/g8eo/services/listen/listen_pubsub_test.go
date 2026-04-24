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

package listen

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPubSubBackPressureDropsOldestAndLogs verifies the drop-oldest
// back-pressure policy: when a subscriber's send buffer saturates, trySend
// evicts the oldest queued frame, enqueues the newer one, logs a structured
// Warn identifying the event, and keeps the subscriber connected. This
// replaces the prior "kill the slow consumer" policy, which tore down WS
// connections on any transient burst (e.g., large stdout followed by rapid
// heartbeats).
func TestPubSubBackPressureDropsOldestAndLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	broker := NewPubSubBroker(logger)

	// Inject a subscriber with a tiny buffer and no ws so trySend's
	// nil-guard path is exercised.
	sub := &wsSubscriber{send: make(chan []byte, 1), done: make(chan struct{})}
	broker.subscribe("ch", sub)

	oldest := json.RawMessage(`"oldest"`)
	newest := json.RawMessage(`"newest"`)

	// First publish fills the 1-slot buffer.
	require.Equal(t, 1, broker.Publish("ch", oldest))
	// Second publish overflows: oldest is dropped, newest is enqueued,
	// subscriber stays alive. Publish returns 1 because the newer message
	// was delivered to the buffer.
	require.Equal(t, 1, broker.Publish("ch", newest))

	sub.mu.Lock()
	dropped := sub.dropped
	sub.mu.Unlock()
	assert.False(t, sub.isDone(), "subscriber must remain connected under drop-oldest policy")
	assert.Equal(t, uint64(1), dropped, "dropped counter must increment exactly once")

	// Buffer holds exactly the newest frame; the oldest was evicted.
	require.Len(t, sub.send, 1, "buffer must hold exactly one frame after drop-oldest")
	got := <-sub.send
	var event PubSubEvent
	require.NoError(t, json.Unmarshal(got, &event))
	assert.JSONEq(t, string(newest), string(event.Data), "newest message must survive drop-oldest")

	logs := buf.String()
	assert.Contains(t, logs, "back-pressure", "drop-oldest event must be logged")
	assert.Contains(t, logs, "dropped_total=1", "log must include running drop counter")
	assert.Contains(t, logs, "buffer_capacity=1", "log must include buffer capacity")
	assert.True(t, strings.Contains(logs, "level=WARN"), "drop-oldest must be logged at WARN level")
}

// TestPubSubBackPressureKeepsSubscriptions verifies that under sustained
// back-pressure the subscriber remains routable via both exact and pattern
// channels. The prior kill-on-overflow policy synchronously evicted the
// subscriber from broker maps; drop-oldest must not.
func TestPubSubBackPressureKeepsSubscriptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewPubSubBroker(logger)

	sub := &wsSubscriber{send: make(chan []byte, 1), done: make(chan struct{})}
	broker.subscribe("ch", sub)
	broker.psubscribe("ch.*", sub)

	payload := json.RawMessage(`"x"`)
	// Drive several overflows in a row.
	for i := 0; i < 5; i++ {
		broker.Publish("ch", payload)
	}

	broker.mu.RLock()
	_, exactPresent := broker.subscribers["ch"]
	_, patternPresent := broker.patterns["ch.*"]
	broker.mu.RUnlock()

	assert.True(t, exactPresent, "subscriber must remain in exact-channel map under back-pressure")
	assert.True(t, patternPresent, "subscriber must remain in pattern map under back-pressure")

	sub.mu.Lock()
	dropped := sub.dropped
	sub.mu.Unlock()
	assert.False(t, sub.isDone(), "subscriber must not be closed by back-pressure")
	assert.GreaterOrEqual(t, dropped, uint64(4), "each overflow must increment dropped counter")
}

// TestPubSubSubscriberShutdownIsIdempotentAndFailsFast verifies the
// single-shutdown invariant: calling shutdown() from any number of
// goroutines or lifecycle paths (writer error, read-loop exit, broker
// Close) collapses into one tear-down, and subsequent trySend calls fail
// fast via <-done without blocking, double-close panics, or send-on-closed
// channel panics. This is the regression contract replacing the prior
// triple-signal (closed bool + close(send) + ws.Close) bookkeeping that
// was the root cause of a subtle double-close race.
func TestPubSubSubscriberShutdownIsIdempotentAndFailsFast(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewPubSubBroker(logger)

	sub := &wsSubscriber{send: make(chan []byte, 4), done: make(chan struct{})}
	broker.subscribe("ch", sub)

	// Happy path still works before shutdown.
	require.Equal(t, 1, broker.Publish("ch", json.RawMessage(`"pre"`)))

	// Fire shutdown from multiple goroutines; sync.Once must coalesce.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub.shutdown()
		}()
	}
	wg.Wait()

	assert.True(t, sub.isDone(), "done must be signalled after shutdown")

	// Post-shutdown publishes must fail fast, not panic.
	assert.Equal(t, 0, broker.Publish("ch", json.RawMessage(`"post"`)),
		"trySend must return false once subscriber is done")

	// Extra shutdown calls remain no-ops.
	assert.NotPanics(t, func() { sub.shutdown() }, "repeat shutdown must be a no-op")
}

// TestPubSubHappyPathDoesNotLogDrop ensures the back-pressure warning
// is not emitted on normal delivery.
func TestPubSubHappyPathDoesNotLogDrop(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	broker := NewPubSubBroker(logger)

	sub := &wsSubscriber{send: make(chan []byte, 4), done: make(chan struct{})}
	broker.subscribe("ch", sub)

	require.Equal(t, 1, broker.Publish("ch", json.RawMessage(`"ok"`)))
	assert.NotContains(t, buf.String(), "back-pressure")
}
