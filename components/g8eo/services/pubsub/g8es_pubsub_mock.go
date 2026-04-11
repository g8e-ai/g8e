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
	"sync"
)

// MockG8esPubSubClient is a test double for G8esPubSubClient.
// It records published messages and allows tests to inject incoming messages.
type MockG8esPubSubClient struct {
	mu          sync.Mutex
	published   []MockPublishedMsg
	subscribers map[string][]chan []byte
	closed      bool
}

// MockPublishedMsg records a single Publish call
type MockPublishedMsg struct {
	Channel string
	Data    []byte
}

// NewMockG8esPubSubClient creates a new mock client
func NewMockG8esPubSubClient() *MockG8esPubSubClient {
	return &MockG8esPubSubClient{
		subscribers: make(map[string][]chan []byte),
	}
}

// Subscribe returns a channel that receives messages injected via InjectMessage
func (m *MockG8esPubSubClient) Subscribe(_ context.Context, channel string) (<-chan []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan []byte, 64)
	m.subscribers[channel] = append(m.subscribers[channel], ch)
	return ch, nil
}

// Publish records the message and fans it out to any subscribers on the same channel
func (m *MockG8esPubSubClient) Publish(_ context.Context, channel string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, MockPublishedMsg{Channel: channel, Data: data})
	for _, ch := range m.subscribers[channel] {
		select {
		case ch <- data:
		default:
		}
	}
	return nil
}

// Close is a no-op for the mock
func (m *MockG8esPubSubClient) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, chans := range m.subscribers {
		for _, ch := range chans {
			close(ch)
		}
	}
	m.subscribers = make(map[string][]chan []byte)
}

// InjectMessage simulates an incoming message on a subscribed channel
func (m *MockG8esPubSubClient) InjectMessage(channel string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.subscribers[channel] {
		select {
		case ch <- data:
		default:
		}
	}
}

// Published returns all messages published via Publish
func (m *MockG8esPubSubClient) Published() []MockPublishedMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockPublishedMsg, len(m.published))
	copy(result, m.published)
	return result
}

// PublishedCount returns the number of published messages
func (m *MockG8esPubSubClient) PublishedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.published)
}

// LastPublished returns the last published message, or nil if none
func (m *MockG8esPubSubClient) LastPublished() *MockPublishedMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.published) == 0 {
		return nil
	}
	msg := m.published[len(m.published)-1]
	return &msg
}

// Reset clears all recorded publishes
func (m *MockG8esPubSubClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = nil
}
