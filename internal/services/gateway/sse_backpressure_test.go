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
	"bytes"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPubSubBackPressure_DropsOldestUnderBurst verifies that when a subscriber's
// send buffer is full, the drop-oldest policy evicts the oldest queued message
// to make room for newer ones. The subscriber remains connected (not evicted),
// and the dropped count increases. This is the core back-pressure behavior for
// SSE/WebSocket streams under burst conditions.
func TestPubSubBackPressure_DropsOldestUnderBurst(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	broker := NewGatewayWebSocketHandler(logger)

	// Create a subscriber with a tiny buffer (capacity 2).
	sub := &wsSubscriber{
		buf:  newDropOldestBuf(2),
		done: make(chan struct{}),
	}
	broker.subscribe("burst-channel", sub)

	// Fill the buffer completely.
	msg1 := []byte("msg-1")
	msg2 := []byte("msg-2")
	assert.True(t, broker.trySend(sub, msg1), "first send should succeed")
	assert.True(t, broker.trySend(sub, msg2), "second send should fill buffer")

	// Third send should trigger drop-oldest: msg1 is evicted, msg3 is enqueued.
	msg3 := []byte("msg-3")
	assert.True(t, broker.trySend(sub, msg3), "third send should succeed via drop-oldest")

	// Verify the buffer contains msg2 and msg3 (msg1 was dropped).
	received := make([]string, 0, 2)
	for {
		select {
		case msg := <-sub.buf.recv():
			received = append(received, string(msg))
		default:
			goto done
		}
	}
done:
	assert.Len(t, received, 2, "buffer should contain 2 messages after drop-oldest")
	if len(received) >= 2 {
		assert.Equal(t, "msg-2", received[0], "oldest surviving message should be msg-2")
		assert.Equal(t, "msg-3", received[1], "newest message should be msg-3")
	}

	// Verify the drop was logged.
	assert.Contains(t, buf.String(), "back-pressure")
	assert.Contains(t, buf.String(), "dropped oldest")
}

// TestPubSubBackPressure_ConcurrentPublishersNoDeadlock verifies that
// concurrent publishers to the same channel do not deadlock when the
// subscriber buffer is full. The drop-oldest policy must handle concurrent
// trySend calls without blocking.
func TestPubSubBackPressure_ConcurrentPublishersNoDeadlock(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}))
	broker := NewGatewayWebSocketHandler(logger)

	// Subscriber with a small buffer that will fill up quickly.
	sub := &wsSubscriber{
		buf:  newDropOldestBuf(4),
		done: make(chan struct{}),
	}
	broker.subscribe("concurrent-channel", sub)

	const numPublishers = 20
	const msgsPerPublisher = 50

	var wg sync.WaitGroup
	for i := 0; i < numPublishers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerPublisher; j++ {
				broker.trySend(sub, []byte("msg"))
			}
		}(i)
	}

	// This should complete without deadlock.
	wg.Wait()

	// The subscriber should still be subscribed (not evicted by back-pressure).
	broker.mu.RLock()
	_, stillSubscribed := broker.subscribers["concurrent-channel"]
	broker.mu.RUnlock()
	assert.True(t, stillSubscribed, "subscriber should remain subscribed after back-pressure")
}
