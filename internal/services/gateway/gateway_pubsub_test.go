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

//go:build integration

package gateway

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	pubsubv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/pubsub/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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
	broker := NewGatewayWebSocketHandler(logger)

	// Inject a subscriber with a tiny buffer and no ws so trySend's
	// nil-guard path is exercised.
	sub := &wsSubscriber{send: make(chan []byte, 1), done: make(chan struct{})}
	broker.subscribe("ch", sub)

	oldest := []byte(`"oldest"`)
	newest := []byte(`"newest"`)

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
	var event pubsubv1.PubSubEvent
	require.NoError(t, proto.Unmarshal(got, &event))
	assert.Equal(t, newest, event.Data, "newest message must survive drop-oldest")

	logs := buf.String()
	assert.Contains(t, logs, "back-pressure", "drop-oldest event must be logged")
	assert.Contains(t, logs, "dropped_total=1", "log must include running drop counter")
	assert.Contains(t, logs, "buffer_capacity=1", "log must include buffer capacity")
	assert.Contains(t, logs, "level=WARN", "pubsub: trySend: drop-oldest must be logged at WARN level")
}

// TestPubSubBackPressureKeepsSubscriptions verifies that under sustained
// back-pressure the subscriber remains routable via both exact and pattern
// channels. The prior kill-on-overflow policy synchronously evicted the
// subscriber from broker maps; drop-oldest must not.
func TestPubSubBackPressureKeepsSubscriptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{send: make(chan []byte, 1), done: make(chan struct{})}
	broker.subscribe("ch", sub)

	payload := []byte(`"x"`)
	// Drive several overflows in a row.
	for i := 0; i < 5; i++ {
		broker.Publish("ch", payload)
	}

	broker.mu.RLock()
	_, exactPresent := broker.subscribers["ch"]
	broker.mu.RUnlock()

	assert.True(t, exactPresent, "subscriber must remain in exact-channel map under back-pressure")
}

func TestPubSubSessionHandler_handleAction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{
		send:             make(chan []byte, 10),
		done:             make(chan struct{}),
		identitySPIFFEID: "spiffe://g8e.local/app/test-app",
		operatorID:       "test-operator",
	}
	handler := &pubSubSessionHandler{
		broker: broker,
		sub:    sub,
	}

	tests := []struct {
		name     string
		msg      *pubsubv1.PubSubMessage
		preSetup func()
		validate func(t *testing.T)
	}{
		{
			name: "subscribe action adds subscriber to channel",
			msg: &pubsubv1.PubSubMessage{
				Action:  "subscribe",
				Channel: "results:test-operator:cli-session-123",
			},
			preSetup: func() {},
			validate: func(t *testing.T) {
				broker.mu.RLock()
				_, exists := broker.subscribers["results:test-operator:cli-session-123"]
				broker.mu.RUnlock()
				assert.True(t, exists, "Subscriber should be added to channel")
			},
		},
		{
			name: "unsubscribe action removes subscriber from channel",
			msg: &pubsubv1.PubSubMessage{
				Action:  "unsubscribe",
				Channel: "results:test-operator:cli-session-456",
			},
			preSetup: func() {
				broker.subscribe("results:test-operator:cli-session-456", sub)
			},
			validate: func(t *testing.T) {
				broker.mu.RLock()
				_, exists := broker.subscribers["results:test-operator:cli-session-456"]
				broker.mu.RUnlock()
				assert.False(t, exists, "Subscriber should be removed from channel")
			},
		},
		{
			name: "publish action publishes data to channel",
			msg: &pubsubv1.PubSubMessage{
				Action:  "publish",
				Channel: "results:test-operator:cli-session-789",
				Data:    []byte(`"test-data"`),
			},
			preSetup: func() {},
			validate: func(t *testing.T) {
				// Publish should not panic
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.preSetup()
			handler.handleAction(tt.msg)
			tt.validate(t)
		})
	}
}

func TestPubSubSessionHandler_cleanup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{
		send:             make(chan []byte, 10),
		done:             make(chan struct{}),
		identitySPIFFEID: "spiffe://g8e.local/app/test-app",
		operatorID:       "test-operator",
	}
	handler := &pubSubSessionHandler{
		broker: broker,
		sub:    sub,
	}

	// Add subscriber to broker
	broker.subscribe("test-channel", sub)

	// Cleanup should remove subscriber and shutdown
	handler.cleanup()

	broker.mu.RLock()
	_, exists := broker.subscribers["test-channel"]
	broker.mu.RUnlock()
	assert.False(t, exists, "Subscriber should be removed from broker")
	assert.True(t, sub.isDone(), "Subscriber should be shut down")
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
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{send: make(chan []byte, 4), done: make(chan struct{})}
	broker.subscribe("ch", sub)

	// Happy path still works before shutdown.
	require.Equal(t, 1, broker.Publish("ch", []byte(`"pre"`)))

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
	assert.Equal(t, 0, broker.Publish("ch", []byte(`"post"`)),
		"trySend must return false once subscriber is done")

	// Extra shutdown calls remain no-ops.
	assert.NotPanics(t, func() { sub.shutdown() }, "repeat shutdown must be a no-op")
}

// TestPubSubHappyPathDoesNotLogDrop ensures the back-pressure warning
// is not emitted on normal delivery.
func TestPubSubHappyPathDoesNotLogDrop(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{send: make(chan []byte, 4), done: make(chan struct{})}
	broker.subscribe("ch", sub)

	require.Equal(t, 1, broker.Publish("ch", []byte(`"ok"`)))
	assert.NotContains(t, buf.String(), "back-pressure")
}

func TestNewGatewayWebSocketHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	assert.NotNil(t, broker)
	assert.NotNil(t, broker.logger)
	assert.NotNil(t, broker.subscribers)
	assert.NotNil(t, broker.handlers)
}

func TestRegisterHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	called := false
	var receivedChannel string
	var receivedData []byte

	unregister := broker.RegisterHandler("test-channel", func(channel string, data []byte) {
		called = true
		receivedChannel = channel
		receivedData = data
	})

	// Publish to the channel
	broker.Publish("test-channel", []byte("test-data"))

	// Handler is called synchronously in Publish, no additional synchronization needed
	assert.True(t, called, "handler should have been called")
	assert.Equal(t, "test-channel", receivedChannel)
	assert.Equal(t, []byte("test-data"), receivedData)

	// Unregister the handler
	unregister()

	// Reset and publish again - handler should not be called
	called = false
	broker.Publish("test-channel", []byte("test-data-2"))
	assert.False(t, called, "handler should not be called after unregister")
}

func TestSubscribe(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{send: make(chan []byte, 4), done: make(chan struct{})}

	// Subscribe to a channel
	broker.subscribe("test-channel", sub)

	// Verify subscriber is in the map
	broker.mu.RLock()
	_, exists := broker.subscribers["test-channel"]
	broker.mu.RUnlock()

	assert.True(t, exists, "subscriber should be in the channel map")
}

func TestIsDone(t *testing.T) {

	tests := []struct {
		name     string
		setup    func() *wsSubscriber
		expected bool
	}{
		{
			name:     "Not done initially",
			setup:    func() *wsSubscriber { return &wsSubscriber{done: make(chan struct{})} },
			expected: false,
		},
		{
			name: "Done after close",
			setup: func() *wsSubscriber {
				sub := &wsSubscriber{done: make(chan struct{})}
				close(sub.done)
				return sub
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			sub := tt.setup()
			assert.Equal(t, tt.expected, sub.isDone())
		})
	}
}

func TestExtractMTLSIdentity_NoTLS(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)

	spiffeID, operatorID := extractMTLSIdentity(req)

	assert.Empty(t, spiffeID, "SPIFFE ID should be empty when no TLS")
	assert.Empty(t, operatorID, "operator ID should be empty when no TLS")
}

func TestExtractMTLSIdentity_NoPeerCertificates(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.TLS = &tls.ConnectionState{}

	spiffeID, operatorID := extractMTLSIdentity(req)

	assert.Empty(t, spiffeID, "SPIFFE ID should be empty when no peer certificates")
	assert.Empty(t, operatorID, "operator ID should be empty when no peer certificates")
}

func TestExtractMTLSIdentity_NoURISANs(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{}},
	}

	spiffeID, operatorID := extractMTLSIdentity(req)

	assert.Empty(t, spiffeID, "SPIFFE ID should be empty when no URI SANs")
	assert.Empty(t, operatorID, "operator ID should be empty when no URI SANs")
}

func TestExtractMTLSIdentity_OperatorSPIFFEID(t *testing.T) {
	spiffeURL, err := url.Parse("spiffe://g8e.local/operator/org-123/op-456/session-789")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			URIs: []*url.URL{spiffeURL},
		}},
	}

	spiffeID, operatorID := extractMTLSIdentity(req)

	assert.Equal(t, "spiffe://g8e.local/operator/org-123/op-456/session-789", spiffeID)
	assert.Equal(t, "op-456", operatorID, "operator ID should be extracted from operator SPIFFE ID")
}

func TestExtractMTLSIdentity_AppSPIFFEID(t *testing.T) {
	spiffeURL, err := url.Parse("spiffe://g8e.local/app/op-123")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			URIs: []*url.URL{spiffeURL},
		}},
	}

	spiffeID, operatorID := extractMTLSIdentity(req)

	assert.Equal(t, "spiffe://g8e.local/app/op-123", spiffeID)
	assert.Equal(t, "op-123", operatorID, "operator ID should be extracted from app SPIFFE ID")
}

func TestExtractMTLSIdentity_UnknownSPIFFEID(t *testing.T) {
	spiffeURL, err := url.Parse("spiffe://g8e.local/unknown/type")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			URIs: []*url.URL{spiffeURL},
		}},
	}

	spiffeID, operatorID := extractMTLSIdentity(req)

	assert.Equal(t, "spiffe://g8e.local/unknown/type", spiffeID)
	assert.Empty(t, operatorID, "operator ID should be empty for unknown SPIFFE ID types")
}

func TestExtractMTLSIdentity_MalformedOperatorSPIFFEID(t *testing.T) {
	spiffeURL, err := url.Parse("spiffe://g8e.local/operator/too-short")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/test", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{
			URIs: []*url.URL{spiffeURL},
		}},
	}

	spiffeID, operatorID := extractMTLSIdentity(req)

	assert.Equal(t, "spiffe://g8e.local/operator/too-short", spiffeID)
	assert.Empty(t, operatorID, "operator ID should be empty for malformed operator SPIFFE ID")
}
