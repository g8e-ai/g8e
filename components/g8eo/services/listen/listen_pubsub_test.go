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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPubSubBackPressureDropIsLogged verifies that when a subscriber's
// send buffer is saturated, trySend drops the subscriber *and* emits a
// structured warning identifying the back-pressure event. This was the
// silent failure mode where a slow subscriber vanished with no diagnostic
// under burst load.
func TestPubSubBackPressureDropIsLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	broker := NewPubSubBroker(logger)

	// Inject a subscriber with a tiny buffer and no ws so trySend's
	// nil-guard path is exercised.
	sub := &wsSubscriber{send: make(chan []byte, 1)}
	broker.subscribe("ch", sub)

	payload := json.RawMessage(`"hello"`)

	// First publish fills the 1-slot buffer.
	require.Equal(t, 1, broker.Publish("ch", payload))
	// Second publish cannot enqueue: subscriber must be dropped.
	require.Equal(t, 0, broker.Publish("ch", payload))

	sub.closeMu.Lock()
	closed := sub.closed
	sub.closeMu.Unlock()
	assert.True(t, closed, "subscriber should be marked closed after back-pressure drop")

	logs := buf.String()
	assert.Contains(t, logs, "back-pressure", "back-pressure drop must be logged")
	assert.Contains(t, logs, "buffer_capacity=1", "log must include buffer capacity")
	assert.True(t, strings.Contains(logs, "level=WARN"), "drop must be logged at WARN level")
}

// TestPubSubBackPressureDropEvictsSynchronously verifies that when trySend
// drops a subscriber due to a saturated send buffer, it is removed from the
// broker's routing tables (both exact and pattern) immediately—before the
// write-loop goroutine or websocket close side effects can run. This closes
// the window where a zombie subscriber remained reachable by Publish between
// drop and eventual cleanup via the read-loop erroring out.
func TestPubSubBackPressureDropEvictsSynchronously(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewPubSubBroker(logger)

	sub := &wsSubscriber{send: make(chan []byte, 1)}
	broker.subscribe("ch", sub)
	broker.psubscribe("ch.*", sub)

	payload := json.RawMessage(`"x"`)
	// Fill the 1-slot buffer via exact-channel delivery.
	require.Equal(t, 1, broker.Publish("ch", payload))
	// Next publish to either exact or pattern channel must drop the sub.
	broker.Publish("ch", payload)

	broker.mu.RLock()
	_, exactPresent := broker.subscribers["ch"]
	_, patternPresent := broker.patterns["ch.*"]
	broker.mu.RUnlock()

	assert.False(t, exactPresent, "subscriber must be evicted from exact-channel map synchronously on drop")
	assert.False(t, patternPresent, "subscriber must be evicted from pattern map synchronously on drop")
}

// TestPubSubHappyPathDoesNotLogDrop ensures the back-pressure warning
// is not emitted on normal delivery.
func TestPubSubHappyPathDoesNotLogDrop(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	broker := NewPubSubBroker(logger)

	sub := &wsSubscriber{send: make(chan []byte, 4)}
	broker.subscribe("ch", sub)

	require.Equal(t, 1, broker.Publish("ch", json.RawMessage(`"ok"`)))
	assert.NotContains(t, buf.String(), "back-pressure")
}
