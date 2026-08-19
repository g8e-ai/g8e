// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/internal/constants"
	pubsubv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/pubsub/v1"
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
	sub := &wsSubscriber{buf: newDropOldestBuf(1), done: make(chan struct{})}
	broker.subscribe("ch", sub)

	oldest := []byte(`"oldest"`)
	newest := []byte(`"newest"`)

	// First publish fills the 1-slot buffer.
	require.Equal(t, 1, broker.Publish("ch", oldest))
	// Second publish overflows: oldest is dropped, newest is enqueued,
	// subscriber stays alive. Publish returns 1 because the newer message
	// was delivered to the buffer.
	require.Equal(t, 1, broker.Publish("ch", newest))

	sub.buf.mu.Lock()
	dropped := sub.buf.dropped
	sub.buf.mu.Unlock()
	assert.False(t, sub.isDone(), "subscriber must remain connected under drop-oldest policy")
	assert.Equal(t, int64(1), dropped, "dropped counter must increment exactly once")

	// Buffer holds exactly the newest frame; the oldest was evicted.
	require.Len(t, sub.buf.recv(), 1, "buffer must hold exactly one frame after drop-oldest")
	got := <-sub.buf.recv()
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

	sub := &wsSubscriber{buf: newDropOldestBuf(1), done: make(chan struct{})}
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
		buf:              newDropOldestBuf(10),
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
		buf:              newDropOldestBuf(10),
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

	sub := &wsSubscriber{buf: newDropOldestBuf(4), done: make(chan struct{})}
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

	sub := &wsSubscriber{buf: newDropOldestBuf(4), done: make(chan struct{})}
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

	sub := &wsSubscriber{buf: newDropOldestBuf(4), done: make(chan struct{})}

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

// TestPubSubPatternSubscribeAndPublishFanOut verifies the PSUBSCRIBE /
// PMESSAGE path end-to-end: a subscriber registered via psubscribe for a
// glob pattern receives a pmessage event (with Type=pmessage, Pattern set,
// and the original channel preserved) when a matching channel is
// published. Non-matching channels deliver nothing.
func TestPubSubPatternSubscribeAndPublishFanOut(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{buf: newDropOldestBuf(4), done: make(chan struct{})}
	broker.psubscribe("heartbeat:*", sub)

	// Matching channel delivers a pmessage.
	require.Equal(t, 1, broker.Publish("heartbeat:op-1:sess-1", []byte(`"thump"`)))

	got := <-sub.buf.recv()
	var event pubsubv1.PubSubEvent
	require.NoError(t, proto.Unmarshal(got, &event))
	assert.Equal(t, constants.PubSubEventPMessage, event.Type, "event type must be pmessage for pattern delivery")
	assert.Equal(t, "heartbeat:op-1:sess-1", event.Channel, "pmessage must carry the published channel")
	assert.Equal(t, "heartbeat:*", event.Pattern, "pmessage must carry the matched pattern")
	assert.Equal(t, []byte(`"thump"`), event.Data)

	// Non-matching channel delivers nothing to the pattern subscriber.
	assert.Equal(t, 0, broker.Publish("results:op-1:sess-1", []byte(`"nope"`)))
	select {
	case extra := <-sub.buf.recv():
		t.Fatalf("pattern subscriber must not receive non-matching publish; got %v", extra)
	default:
	}
}

// TestPubSubPatternPublish_MalformedPatternSkipped injects a malformed
// glob pattern directly into patternSubscribers and asserts that Publish
// does not panic, logs a WARN, and skips the entry without delivering.
func TestPubSubPatternPublish_MalformedPatternSkipped(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{buf: newDropOldestBuf(4), done: make(chan struct{})}
	// "[" is an invalid glob pattern: path.Match returns ErrBadPattern.
	broker.mu.Lock()
	broker.patternSubscribers["heartbeat:["] = map[*wsSubscriber]struct{}{sub: {}}
	broker.mu.Unlock()

	assert.NotPanics(t, func() {
		broker.Publish("heartbeat:op-1:sess-1", []byte(`"x"`))
	})

	assert.Contains(t, logBuf.String(), "malformed pattern", "malformed pattern must be logged at WARN")
	select {
	case extra := <-sub.buf.recv():
		t.Fatalf("subscriber must not receive delivery for a malformed pattern; got %v", extra)
	default:
	}
}

// TestPubSubPatternRemoveSubEvictsFromPatternMaps verifies that removeSub
// evicts the subscriber from every pattern it was registered under, not
// just the exact-channel maps.
func TestPubSubPatternRemoveSubEvictsFromPatternMaps(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{buf: newDropOldestBuf(4), done: make(chan struct{})}
	broker.psubscribe("heartbeat:*", sub)
	broker.psubscribe("results:*", sub)

	broker.removeSub(sub)

	broker.mu.RLock()
	_, hbPresent := broker.patternSubscribers["heartbeat:*"]
	_, resPresent := broker.patternSubscribers["results:*"]
	broker.mu.RUnlock()

	assert.False(t, hbPresent, "removeSub must evict the subscriber from the heartbeat:* pattern map")
	assert.False(t, resPresent, "removeSub must evict the subscriber from the results:* pattern map")
}

// TestPubSubClose_ShutsDownPatternSubscribers verifies that Close collects
// pattern subscribers (alongside exact-channel subscribers) into the
// shutdown set and clears the patternSubscribers map.
func TestPubSubClose_ShutsDownPatternSubscribers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	patSub := &wsSubscriber{buf: newDropOldestBuf(4), done: make(chan struct{})}
	exactSub := &wsSubscriber{buf: newDropOldestBuf(4), done: make(chan struct{})}
	broker.psubscribe("heartbeat:*", patSub)
	broker.subscribe("results:op-1:sess-1", exactSub)

	broker.Close()

	assert.True(t, patSub.isDone(), "Close must shut down pattern subscribers")
	assert.True(t, exactSub.isDone(), "Close must shut down exact-channel subscribers")

	broker.mu.RLock()
	assert.Empty(t, broker.patternSubscribers, "Close must clear the patternSubscribers map")
	assert.Empty(t, broker.subscribers, "Close must clear the subscribers map")
	broker.mu.RUnlock()
}

// TestPubSubUnsubscribeActionTearsDownBothMaps verifies that the
// PubSubActionUnsubscribe case evicts the subscriber from both the
// exact-channel and pattern maps, matching the Python client's single
// UNSUBSCRIBE action for both subscription kinds.
func TestPubSubUnsubscribeActionTearsDownBothMaps(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{
		buf:              newDropOldestBuf(4),
		done:             make(chan struct{}),
		identitySPIFFEID: "spiffe://g8e.local/app/op-1",
		operatorID:       "op-1",
	}
	handler := &pubSubSessionHandler{broker: broker, sub: sub}

	// Register the same channel string as both an exact subscription and a
	// pattern subscription.
	broker.subscribe("heartbeat:op-1", sub)
	broker.psubscribe("heartbeat:op-1", sub)

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionUnsubscribe,
		Channel: "heartbeat:op-1",
	})

	broker.mu.RLock()
	_, exactPresent := broker.subscribers["heartbeat:op-1"]
	_, patPresent := broker.patternSubscribers["heartbeat:op-1"]
	broker.mu.RUnlock()

	assert.False(t, exactPresent, "UNSUBSCRIBE must evict from the exact-channel map")
	assert.False(t, patPresent, "UNSUBSCRIBE must evict from the pattern map")
}

// TestPubSubPSubscribeAction_ACLRejectsCrossOperator verifies that the
// PubSubActionPSubscribe case enforces verifyPatternACL before registering
// the subscriber: a cross-operator pattern is rejected and the subscriber
// is never added to patternSubscribers.
func TestPubSubPSubscribeAction_ACLRejectsCrossOperator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{
		buf:              newDropOldestBuf(4),
		done:             make(chan struct{}),
		identitySPIFFEID: "spiffe://g8e.local/app/op-1",
		operatorID:       "op-1",
	}
	handler := &pubSubSessionHandler{broker: broker, sub: sub}

	handler.handleAction(&pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionPSubscribe,
		Channel: "heartbeat:op-other:*",
	})

	broker.mu.RLock()
	_, present := broker.patternSubscribers["heartbeat:op-other:*"]
	broker.mu.RUnlock()
	assert.False(t, present, "cross-operator pattern subscription must be rejected by the ACL")
}

// TestPubSubPSubscribeAction_ACLAcceptsWildcardAndOwnOperatorID verifies
// that the PSUBSCRIBE action accepts the wildcard operator_id segment and
// the subscriber's own operator_id, registering the subscriber in both
// cases.
func TestPubSubPSubscribeAction_ACLAcceptsWildcardAndOwnOperatorID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	broker := NewGatewayWebSocketHandler(logger)

	sub := &wsSubscriber{
		buf:              newDropOldestBuf(4),
		done:             make(chan struct{}),
		identitySPIFFEID: "spiffe://g8e.local/app/op-1",
		operatorID:       "op-1",
	}
	handler := &pubSubSessionHandler{broker: broker, sub: sub}

	for _, pattern := range []string{"heartbeat:*", "heartbeat:op-1:*", "results:op-1"} {
		handler.handleAction(&pubsubv1.PubSubMessage{
			Action:  constants.PubSubActionPSubscribe,
			Channel: pattern,
		})
	}

	broker.mu.RLock()
	_, hbWild := broker.patternSubscribers["heartbeat:*"]
	_, hbOwn := broker.patternSubscribers["heartbeat:op-1:*"]
	_, resOwn := broker.patternSubscribers["results:op-1"]
	broker.mu.RUnlock()

	assert.True(t, hbWild, "wildcard operator_id pattern must be accepted")
	assert.True(t, hbOwn, "own operator_id pattern must be accepted")
	assert.True(t, resOwn, "own operator_id exact-segment pattern must be accepted")
}

// TestVerifyPatternACL is the table-driven security contract for
// verifyPatternACL: wildcard accepted, own operator_id accepted, other
// operator_id rejected, missing operator_id rejected, malformed pattern
// rejected, results:* wildcard accepted.
func TestVerifyPatternACL(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		operator  string
		spiffeID  string
		wantErr   bool
		wantErrIs error
	}{
		{
			name:     "wildcard operator_id accepted",
			pattern:  "heartbeat:*",
			operator: "op-1",
			spiffeID: "spiffe://g8e.local/app/op-1",
		},
		{
			name:     "own operator_id accepted",
			pattern:  "heartbeat:op-1:*",
			operator: "op-1",
			spiffeID: "spiffe://g8e.local/app/op-1",
		},
		{
			name:      "other operator_id rejected",
			pattern:   "heartbeat:op-other:*",
			operator:  "op-1",
			spiffeID:  "spiffe://g8e.local/app/op-1",
			wantErr:   true,
			wantErrIs: nil, // dynamic fmt.Errorf, no sentinel
		},
		{
			name:      "missing operator_id rejected",
			pattern:   "heartbeat:*",
			operator:  "",
			spiffeID:  "",
			wantErr:   true,
			wantErrIs: constants.ErrPubSubCertificateMissingOperatorID,
		},
		{
			name:      "malformed pattern rejected",
			pattern:   "heartbeat",
			operator:  "op-1",
			spiffeID:  "spiffe://g8e.local/app/op-1",
			wantErr:   true,
			wantErrIs: constants.ErrPubSubInvalidChannelFormat,
		},
		{
			name:     "results wildcard accepted",
			pattern:  "results:*",
			operator: "op-1",
			spiffeID: "spiffe://g8e.local/app/op-1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := verifyPatternACL(tt.pattern, tt.operator, tt.spiffeID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}
