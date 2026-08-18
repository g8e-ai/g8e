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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subscribeCloseImmediatelyClient is a test PubSubClient whose Subscribe calls
// fail for the first failCount attempts, then succeed once but close the
// channel immediately without delivering any messages, then fail on all
// subsequent calls. This verifies that a successful Subscribe resets the
// attempts counter even when no messages are received before the channel closes.
type subscribeCloseImmediatelyClient struct {
	mu         sync.Mutex
	callCount  int
	failCount  int
	failErr    error
	subscribed bool
}

func (c *subscribeCloseImmediatelyClient) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	c.mu.Lock()
	c.callCount++
	count := c.callCount
	c.mu.Unlock()

	if count <= c.failCount {
		return nil, c.failErr
	}
	if !c.subscribed {
		c.subscribed = true
		ch := make(chan []byte)
		close(ch)
		return ch, nil
	}
	return nil, c.failErr
}

func (c *subscribeCloseImmediatelyClient) Publish(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (c *subscribeCloseImmediatelyClient) Close() {}

func (c *subscribeCloseImmediatelyClient) getCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callCount
}

func TestListenForCommands_SubscribeSuccessResetsAttempts(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	mockClient := &subscribeCloseImmediatelyClient{
		failCount: 2,
		failErr:   errors.New("connection refused"),
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

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("listenForCommands did not exit after max reconnect attempts")
	}

	// Without the fix (attempts not reset on Subscribe success), the service
	// would give up after 3 Subscribe calls: 2 failures + 1 success where the
	// channel close increments attempts to 3.
	// With the fix, attempts resets to 0 on successful Subscribe, so the
	// channel close makes attempts=1, then 2 more failures are needed to
	// reach 3. Total: 5 Subscribe calls (2 fail + 1 succeed + 2 fail).
	assert.GreaterOrEqual(t, mockClient.getCallCount(), 4,
		"Subscribe should be called at least 4 times, indicating attempts was reset on successful Subscribe")
}

func TestListenForCommands_ContextCancellationExitsImmediately(t *testing.T) {
	t.Parallel()

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	mockClient := &subscribeCloseImmediatelyClient{
		failCount: 100,
		failErr:   errors.New("connection refused"),
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

	ctx, cancel := context.WithCancel(context.Background())
	svc.ctx = ctx

	done := make(chan struct{})
	go func() {
		svc.listenForCommands("test-channel")
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("listenForCommands did not exit after context cancellation")
	}
}
