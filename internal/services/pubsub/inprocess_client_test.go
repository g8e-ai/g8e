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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBroker struct {
	handlers map[string]func(string, []byte)
}

func (m *mockBroker) Publish(channel string, data []byte) int {
	if handler, ok := m.handlers[channel]; ok {
		handler(channel, data)
	}
	return 1
}

func (m *mockBroker) RegisterHandler(channel string, handler func(string, []byte)) func() {
	if m.handlers == nil {
		m.handlers = make(map[string]func(string, []byte))
	}
	m.handlers[channel] = handler
	return func() {
		delete(m.handlers, channel)
	}
}

func TestNewInProcessPubSubClient(t *testing.T) {
	t.Run("creates client successfully", func(t *testing.T) {
		t.Parallel()
		broker := &mockBroker{}
		client := NewInProcessPubSubClient(broker)
		require.NotNil(t, client)
		assert.Equal(t, broker, client.broker)
	})
}

func TestInProcessPubSubClient_Subscribe(t *testing.T) {
	t.Run("subscribes to channel successfully", func(t *testing.T) {
		t.Parallel()
		broker := &mockBroker{}
		client := NewInProcessPubSubClient(broker)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := client.Subscribe(ctx, "test-channel")
		require.NoError(t, err)
		require.NotNil(t, ch)
	})

	t.Run("rejects subscription when closed", func(t *testing.T) {
		t.Parallel()
		broker := &mockBroker{}
		client := NewInProcessPubSubClient(broker)
		client.Close()

		ctx := context.Background()
		_, err := client.Subscribe(ctx, "test-channel")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client is closed")
	})

	t.Run("rejects duplicate subscription", func(t *testing.T) {
		t.Parallel()
		broker := &mockBroker{}
		client := NewInProcessPubSubClient(broker)

		ctx1, cancel1 := context.WithCancel(context.Background())
		defer cancel1()

		_, err := client.Subscribe(ctx1, "test-channel")
		require.NoError(t, err)

		ctx2 := context.Background()
		_, err = client.Subscribe(ctx2, "test-channel")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already subscribed")
	})

	t.Run("receives published messages", func(t *testing.T) {
		t.Parallel()
		broker := &mockBroker{}
		client := NewInProcessPubSubClient(broker)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := client.Subscribe(ctx, "test-channel")
		require.NoError(t, err)

		// Publish a message
		go func() {
			time.Sleep(10 * time.Millisecond)
			broker.Publish("test-channel", []byte("test message"))
		}()

		select {
		case msg := <-ch:
			assert.Equal(t, []byte("test message"), msg)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("did not receive message")
		}
	})
}

func TestInProcessPubSubClient_Publish(t *testing.T) {
	t.Run("publishes successfully", func(t *testing.T) {
		t.Parallel()
		broker := &mockBroker{}
		client := NewInProcessPubSubClient(broker)

		ctx := context.Background()
		err := client.Publish(ctx, "test-channel", []byte("test message"))
		require.NoError(t, err)
	})

	t.Run("rejects publish when closed", func(t *testing.T) {
		t.Parallel()
		broker := &mockBroker{}
		client := NewInProcessPubSubClient(broker)
		client.Close()

		ctx := context.Background()
		err := client.Publish(ctx, "test-channel", []byte("test message"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client is closed")
	})
}

func TestInProcessPubSubClient_Close(t *testing.T) {
	t.Run("closes successfully", func(t *testing.T) {
		t.Parallel()
		broker := &mockBroker{}
		client := NewInProcessPubSubClient(broker)

		client.Close()
		// Should not panic
		client.Close()
	})

	t.Run("prevents operations after close", func(t *testing.T) {
		t.Parallel()
		broker := &mockBroker{}
		client := NewInProcessPubSubClient(broker)

		client.Close()

		ctx := context.Background()
		err := client.Publish(ctx, "test-channel", []byte("test"))
		require.Error(t, err)
	})
}
