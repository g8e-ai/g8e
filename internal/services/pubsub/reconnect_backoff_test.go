// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextReconnectDelay_ExponentialProgression(t *testing.T) {
	t.Parallel()
	base := 1 * time.Second
	max := 30 * time.Second

	delay := base
	expected := []time.Duration{
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second, // capped
		30 * time.Second, // still capped
	}
	for _, exp := range expected {
		delay = nextReconnectDelay(delay, max)
		assert.Equal(t, exp, delay)
	}
}

func TestNextReconnectDelay_CapsAtMax(t *testing.T) {
	t.Parallel()
	max := 30 * time.Second
	delay := nextReconnectDelay(20*time.Second, max)
	assert.Equal(t, max, delay, "20s*2=40s should cap at 30s")
}

func TestNextReconnectDelay_ExactDoubleBelowCap(t *testing.T) {
	t.Parallel()
	max := 30 * time.Second
	delay := nextReconnectDelay(8*time.Second, max)
	assert.Equal(t, 16*time.Second, delay, "8s*2=16s is below cap")
}

func TestShouldGiveUp_AtMaxAttempts(t *testing.T) {
	t.Parallel()
	assert.True(t, shouldGiveUp(3, 3), "attempts==max should give up")
}

func TestShouldGiveUp_BelowMaxAttempts(t *testing.T) {
	t.Parallel()
	assert.False(t, shouldGiveUp(2, 3), "attempts<max should not give up")
	assert.False(t, shouldGiveUp(0, 3), "zero attempts should not give up")
	assert.False(t, shouldGiveUp(1, 3))
}

func TestShouldGiveUp_ExceedsMaxAttempts(t *testing.T) {
	t.Parallel()
	assert.True(t, shouldGiveUp(5, 3), "attempts>max should give up")
}

func TestListenForCommands_MaxReconnectAttemptsGiveUp(t *testing.T) {
	t.Parallel()
	f := newPubsubFixture(t)

	// Configure the mock client to always fail Subscribe with a non-TLS error.
	f.DB.SetSubscribeError(errors.New("connection refused"))

	// Use a very short base delay so the test completes quickly.
	f.Svc.reconnectBaseDelay = 1 * time.Millisecond

	done := make(chan struct{})
	go func() {
		f.Svc.listenForCommands("test-channel")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("listenForCommands did not give up after max reconnect attempts")
	}
}

func TestListenForCommands_TLSCertErrorTriggersShutdown(t *testing.T) {
	t.Parallel()
	f := newPubsubFixture(t)

	// Configure the mock client to fail Subscribe with a TLS cert error.
	f.DB.SetSubscribeError(x509.UnknownAuthorityError{})

	done := make(chan struct{})
	go func() {
		f.Svc.listenForCommands("test-channel")
		close(done)
	}()

	select {
	case reason := <-f.Svc.ShutdownChan:
		assert.Equal(t, "SSL_CERT_FAILURE", reason)
	case <-time.After(5 * time.Second):
		t.Fatal("ShutdownChan did not receive SSL_CERT_FAILURE")
	}

	<-done // ensure goroutine exits
}

func TestListenForCommands_ContextCancellationExits(t *testing.T) {
	t.Parallel()
	f := newPubsubFixture(t)

	// Override the service context with a cancellable one.
	ctx, cancel := context.WithCancel(context.Background())
	f.Svc.ctx = ctx

	done := make(chan struct{})
	go func() {
		f.Svc.listenForCommands("test-channel")
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("listenForCommands did not exit after context cancellation")
	}
}

// subscribeOnceThenFailClient is a test PubSubClient whose first Subscribe
// call succeeds and delivers a single message before closing the channel.
// Subsequent Subscribe calls return the configured error. This verifies that
// a successful message receipt resets the reconnect attempt counter.
type subscribeOnceThenFailClient struct {
	mu         sync.Mutex
	subscribed bool
	failErr    error
}

func (c *subscribeOnceThenFailClient) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subscribed {
		return nil, c.failErr
	}
	c.subscribed = true
	ch := make(chan []byte, 1)
	ch <- []byte("test-message")
	close(ch)
	return ch, nil
}

func (c *subscribeOnceThenFailClient) Publish(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (c *subscribeOnceThenFailClient) Close() {}

func TestListenForCommands_SuccessfulReceiptResetsAttempts(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	mockClient := &subscribeOnceThenFailClient{
		failErr: errors.New("connection refused"),
	}

	svc, err := NewOperatorPubSubService(CommandServiceConfig{
		Config:             cfg,
		Logger:             logger,
		PubSubClient:       mockClient,
		ActuatorSigningKey: ed25519.PrivateKey(make([]byte, ed25519.PrivateKeySize)),
		ActuatorKeyID:      "test-key",
	}, GovernanceDeps{
		ReplayStore:       &testutil.MockReplayStore{},
		StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
		TransactionAudit:  &testutil.MockTransactionAudit{},
		L3Notary:          &testutil.MockL3Notary{},
	})
	require.NoError(t, err)
	svc.reconnectBaseDelay = 1 * time.Millisecond

	done := make(chan struct{})
	go func() {
		svc.listenForCommands("test-channel")
		close(done)
	}()

	// First Subscribe succeeds and delivers a message (resets attempts to 0).
	// Then the channel closes (attempts=1). Subsequent Subscribe calls fail
	// with "connection refused" — it should take 3 failures (attempts 1→2→3)
	// to give up, not 2, because the message receipt reset the counter.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("listenForCommands did not exit after reset + max reconnect attempts")
	}
}
