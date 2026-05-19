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

package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// Broker is the interface for the PubSubBroker to avoid import cycles.
type Broker interface {
	Publish(channel string, data []byte) int
	RegisterHandler(channel string, handler func(string, []byte)) func()
}

// InProcessPubSubClient implements PubSubClient for in-process communication
// between the ListenService (broker) and PubSubCommandService (executor).
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
		return nil, fmt.Errorf("client is closed")
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
		return fmt.Errorf("client is closed")
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
