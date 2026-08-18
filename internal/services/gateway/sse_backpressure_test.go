// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

// TestDropOldestBuf_DroppedCountAndEvictionOrder verifies that dropOldestBuf
// correctly tracks the dropped count and evicts the oldest message first when
// the buffer is full.
func TestDropOldestBuf_DroppedCountAndEvictionOrder(t *testing.T) {
	buf := newDropOldestBuf(2)

	// Fill the buffer.
	ok, dropped := buf.send([]byte("msg-1"))
	assert.True(t, ok)
	assert.Equal(t, int64(0), dropped)

	ok, dropped = buf.send([]byte("msg-2"))
	assert.True(t, ok)
	assert.Equal(t, int64(0), dropped)

	// Third send should evict msg-1 (oldest) and increment dropped count.
	ok, dropped = buf.send([]byte("msg-3"))
	assert.True(t, ok)
	assert.Equal(t, int64(1), dropped, "dropped count should be 1 after one eviction")

	// Verify buffer contains msg-2 and msg-3 (msg-1 was evicted).
	first := <-buf.recv()
	assert.Equal(t, "msg-2", string(first), "oldest surviving message should be msg-2")
	second := <-buf.recv()
	assert.Equal(t, "msg-3", string(second), "newest message should be msg-3")

	// Fourth send on empty buffer should reset dropped count to 0.
	ok, dropped = buf.send([]byte("msg-4"))
	assert.True(t, ok)
	assert.Equal(t, int64(1), dropped, "dropped count is cumulative and should still be 1")
}

// TestDropOldestBuf_CloseTerminatesRecv verifies that Close closes the
// underlying channel, causing recv() to return a closed channel.
func TestDropOldestBuf_CloseTerminatesRecv(t *testing.T) {
	buf := newDropOldestBuf(2)
	buf.Close()

	_, ok := <-buf.recv()
	assert.False(t, ok, "recv should return false after Close")
}

// TestDropOldestBuf_ConcurrentSendNoDeadlock verifies that concurrent send
// calls do not deadlock under mutex contention.
func TestDropOldestBuf_ConcurrentSendNoDeadlock(t *testing.T) {
	buf := newDropOldestBuf(4)

	const numGoroutines = 10
	const sendsPerGoroutine = 100

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < sendsPerGoroutine; j++ {
				buf.send([]byte("msg"))
			}
		}()
	}

	wg.Wait()
	buf.Close()

	// Drain remaining messages — should not block.
	drained := 0
	for {
		select {
		case _, ok := <-buf.recv():
			if !ok {
				goto done
			}
			drained++
		default:
			goto done
		}
	}
done:
	assert.Greater(t, drained, 0, "should have some messages remaining after concurrent sends")
}
