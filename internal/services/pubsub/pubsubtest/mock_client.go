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
	mu           sync.Mutex
	subscribers  map[string][]chan []byte
	published    []PublishedMessage
	closed       bool
	publishError error
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
