// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/g8e-ai/g8e/internal/constants"
)

// Broker is the interface for the GatewayWebSocketHandler to avoid import cycles.
type Broker interface {
	Publish(channel string, data []byte) int
	RegisterHandler(channel string, handler func(string, []byte)) func()
}

// InProcessPubSubClient implements PubSubClient for in-process communication
// between the GatewayService (broker) and OperatorPubSubService (executor).
type InProcessPubSubClient struct {
	broker Broker
	mu     sync.Mutex
	subs   map[string]chan []byte
	closed bool
	logger *slog.Logger
}

// NewInProcessPubSubClient creates a new in-process pub/sub client.
func NewInProcessPubSubClient(broker Broker) *InProcessPubSubClient {
	return &InProcessPubSubClient{
		broker: broker,
		subs:   make(map[string]chan []byte),
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

func (c *InProcessPubSubClient) Subscribe(ctx context.Context, channel string) (<-chan []byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, constants.ErrClientClosed
	}

	if _, exists := c.subs[channel]; exists {
		return nil, fmt.Errorf("already subscribed to channel %s", channel)
	}

	ch := make(chan []byte, 1024)
	c.subs[channel] = ch

	// Register in-process handler with the broker
	unregister := c.broker.RegisterHandler(channel, func(chName string, data []byte) {
		c.mu.Lock()
		targetCh, ok := c.subs[chName]
		closed := c.closed
		c.mu.Unlock()

		if ok && !closed {
			select {
			case targetCh <- data:
			default:
				// Drop message on back-pressure for in-process loopback
				c.logger.Warn("InProcessPubSubClient: dropped message on back-pressure",
					"channel", chName, "payload_size", len(data))
			}
		}
	})

	// Unregister on context cancellation
	go func() {
		<-ctx.Done()
		c.mu.Lock()
		if !c.closed {
			unregister()
			if ch, ok := c.subs[channel]; ok {
				close(ch)
				delete(c.subs, channel)
			}
		}
		c.mu.Unlock()
	}()

	return ch, nil
}

func (c *InProcessPubSubClient) Publish(ctx context.Context, channel string, data []byte) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()

	if closed {
		return constants.ErrClientClosed
	}

	c.broker.Publish(channel, data)
	return nil
}

func (c *InProcessPubSubClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	// NOTE: Individual subscriptions are unregistered via their own context cancellation.
	// We clear the map to stop receiving.
	c.subs = make(map[string]chan []byte)
	c.closed = true
}
