// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"crypto/x509"
	"fmt"
	"sync"
	"testing"
	"time"

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

// outageClient is a PubSubClient whose Gateway is unreachable for the first
// failures Subscribe calls and reachable afterwards. A reachable subscription
// stays open until the test ends.
type outageClient struct {
	mu        sync.Mutex
	failures  int
	calls     int
	err       error
	recovered chan struct{}
}

func newOutageClient(failures int, err error) *outageClient {
	return &outageClient{failures: failures, err: err, recovered: make(chan struct{})}
}

func (c *outageClient) Subscribe(ctx context.Context, _ string) (<-chan []byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls <= c.failures {
		return nil, c.err
	}
	if c.calls == c.failures+1 {
		close(c.recovered)
	}
	ch := make(chan []byte)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (c *outageClient) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (c *outageClient) Close() {}

func (c *outageClient) subscribeCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// The Gateway can be down for hours. The command listener must outlive any
// number of failed subscribes and re-establish the cmd channel as soon as the
// Gateway returns: an Operator that heartbeats over its publish socket but has
// no cmd subscription is unreachable and still reported active.
func TestListenForCommands_SurvivesLongGatewayOutage(t *testing.T) {
	t.Parallel()
	f := newPubsubFixture(t)

	// 200 failed subscribes at the 30x backoff cap is hours of outage at the
	// production 1s base delay.
	client := newOutageClient(200, fmt.Errorf("connection refused"))
	f.Svc.client = client
	f.Svc.reconnectBaseDelay = time.Microsecond

	ctx, cancel := context.WithCancel(context.Background())
	f.Svc.ctx = ctx

	done := make(chan struct{})
	go func() {
		f.Svc.listenForCommands("test-channel")
		close(done)
	}()

	select {
	case <-client.recovered:
	case <-done:
		t.Fatal("listenForCommands exited during the Gateway outage")
	case <-time.After(10 * time.Second):
		t.Fatal("listenForCommands did not re-subscribe after the Gateway returned")
	}
	require.Greater(t, client.subscribeCalls(), 200)

	select {
	case <-done:
		t.Fatal("listenForCommands exited while subscribed")
	default:
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("listenForCommands did not exit after context cancellation")
	}
}

// A subscription that the Gateway drops (restart) is re-established, repeatedly.
func TestListenForCommands_ResubscribesAfterEveryDroppedChannel(t *testing.T) {
	t.Parallel()
	f := newPubsubFixture(t)

	client := &droppingClient{}
	f.Svc.client = client
	f.Svc.reconnectBaseDelay = time.Microsecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Svc.ctx = ctx

	done := make(chan struct{})
	go func() {
		f.Svc.listenForCommands("test-channel")
		close(done)
	}()

	require.Eventually(t, func() bool { return client.subscribeCalls() > 20 }, 10*time.Second, time.Millisecond,
		"listener must keep re-subscribing after each dropped channel")

	select {
	case <-done:
		t.Fatal("listenForCommands exited after a dropped channel")
	default:
	}
}

// droppingClient hands out a subscription that the "Gateway" closes at once.
type droppingClient struct {
	mu    sync.Mutex
	calls int
}

func (c *droppingClient) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	ch := make(chan []byte)
	close(ch)
	return ch, nil
}

func (c *droppingClient) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (c *droppingClient) Close() {}

func (c *droppingClient) subscribeCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
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

func TestWaitForReconnect_ContextCancellationInterruptsDelay(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.False(t, waitForReconnect(ctx, time.Hour))
}
