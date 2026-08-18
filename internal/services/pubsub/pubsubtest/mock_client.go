// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package pubsubtest provides test-only PubSubClient implementations.
// This keeps mock infrastructure out of production code.
package pubsubtest

import (
	"context"
	"sync"

	"github.com/g8e-ai/g8e/internal/constants"
)

// PublishedMessage records a single Publish call for test assertions.
type PublishedMessage struct {
	Channel string
	Data    []byte
}

// MockOperatorPubSubClient is an in-memory PubSubClient for tests.
// It implements the pubsub.PubSubClient interface structurally.
type MockOperatorPubSubClient struct {
	mu             sync.Mutex
	subscribers    map[string][]chan []byte
	published      []PublishedMessage
	closed         bool
	publishError   error
	subscribeError error
}

// NewMockOperatorPubSubClient creates a new MockOperatorPubSubClient.
func NewMockOperatorPubSubClient() *MockOperatorPubSubClient {
	return &MockOperatorPubSubClient{
		subscribers: make(map[string][]chan []byte),
	}
}

// Subscribe creates a new channel for receiving messages on the given channel.
func (m *MockOperatorPubSubClient) Subscribe(_ context.Context, channel string) (<-chan []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscribeError != nil {
		return nil, m.subscribeError
	}

	if m.closed {
		return nil, constants.ErrClientClosed
	}

	ch := make(chan []byte, 64)
	m.subscribers[channel] = append(m.subscribers[channel], ch)
	return ch, nil
}

// Publish records the message and fans it out to all subscribers of the channel.
func (m *MockOperatorPubSubClient) Publish(_ context.Context, channel string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return constants.ErrClientClosed
	}

	if m.publishError != nil {
		return m.publishError
	}

	m.published = append(m.published, PublishedMessage{Channel: channel, Data: data})

	for _, ch := range m.subscribers[channel] {
		select {
		case ch <- data:
		default:
		}
	}
	return nil
}

// Close closes all subscriber channels and clears subscribers.
// The mock is reset to allow re-use in subsequent test assertions.
func (m *MockOperatorPubSubClient) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, subs := range m.subscribers {
		for _, ch := range subs {
			close(ch)
		}
	}
	m.subscribers = make(map[string][]chan []byte)
	m.closed = false
}

// InjectMessage sends a message to all subscribers of the channel without recording it.
func (m *MockOperatorPubSubClient) InjectMessage(channel string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ch := range m.subscribers[channel] {
		select {
		case ch <- data:
		default:
		}
	}
}

// Published returns a copy of all published messages.
func (m *MockOperatorPubSubClient) Published() []PublishedMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]PublishedMessage, len(m.published))
	copy(out, m.published)
	return out
}

// PublishedCount returns the number of published messages.
func (m *MockOperatorPubSubClient) PublishedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.published)
}

// LastPublished returns the last published message, or nil if none.
func (m *MockOperatorPubSubClient) LastPublished() *PublishedMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.published) == 0 {
		return nil
	}
	last := m.published[len(m.published)-1]
	return &last
}

// Reset clears all published messages.
func (m *MockOperatorPubSubClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = nil
}

// SetPublishError configures the mock to return an error on subsequent Publish calls.
func (m *MockOperatorPubSubClient) SetPublishError(shouldError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if shouldError {
		m.publishError = constants.ErrClientClosed
	} else {
		m.publishError = nil
	}
}

// SetSubscribeError configures the mock to return the given error on subsequent
// Subscribe calls. Pass nil to clear the error and resume normal behavior.
func (m *MockOperatorPubSubClient) SetSubscribeError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribeError = err
}
