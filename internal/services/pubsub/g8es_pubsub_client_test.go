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
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pubsubv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/pubsub/v1"
)

func TestNewOperatorPubSubClient(t *testing.T) {
	logger := slog.Default()

	t.Run("rejects empty baseURL", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("", "", logger)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "operator pub/sub URL is required")
	})

	t.Run("accepts ws:// URL", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("ws://localhost:8440", "", logger)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "ws://localhost:8440", client.baseURL)
		assert.Nil(t, client.tlsConfig)
		assert.Empty(t, client.serverName)
	})

	t.Run("accepts wss:// URL", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("wss://localhost:8440", "", logger)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "wss://localhost:8440", client.baseURL)
		assert.NotNil(t, client.tlsConfig)
	})

	t.Run("sets serverName for TLS SNI override", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("wss://192.168.1.1:8440", "gateway.local", logger)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "gateway.local", client.serverName)
		assert.Equal(t, "gateway.local", client.tlsConfig.ServerName)
	})
}

func TestPubSubWSURL(t *testing.T) {
	logger := slog.Default()

	t.Run("returns correct URL for ws://", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("ws://localhost:8440", "", logger)
		require.NoError(t, err)
		assert.Equal(t, "ws://localhost:8440/ws/pubsub", client.pubSubWSURL())
	})

	t.Run("returns correct URL for wss://", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("wss://localhost:8440", "", logger)
		require.NoError(t, err)
		assert.Equal(t, "wss://localhost:8440/ws/pubsub", client.pubSubWSURL())
	})
}

func TestConnectPubWs(t *testing.T) {
	logger := slog.Default()

	t.Run("fails on invalid endpoint", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("ws://invalid-host-that-does-not-exist:9999", "", logger)
		require.NoError(t, err)

		client.mu.Lock()
		err = client.connectPubWs()
		client.mu.Unlock()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect publish WebSocket")
	})

	t.Run("succeeds on valid endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()
		}))
		defer server.Close()

		wsURL := strings.Replace(server.URL, "http", "ws", 1)
		client, err := NewOperatorPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		client.mu.Lock()
		err = client.connectPubWs()
		client.mu.Unlock()

		assert.NoError(t, err)
		assert.NotNil(t, client.pubWs)

		client.Close()
	})
}

func TestPublish(t *testing.T) {
	logger := slog.Default()

	t.Run("fails when client is closed", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("ws://localhost:8440", "", logger)
		require.NoError(t, err)
		client.Close()

		err = client.Publish(context.Background(), "test-channel", []byte("test data"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operator pub/sub client is closed")
	})

	t.Run("fails on connection error", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("ws://invalid-host:9999", "", logger)
		require.NoError(t, err)

		err = client.Publish(context.Background(), "test-channel", []byte("test data"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect publish WebSocket")
	})

	t.Run("succeeds on valid connection", func(t *testing.T) {
		receivedData := make(chan []byte, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()

			_, data, err := conn.ReadMessage()
			assert.NoError(t, err)
			receivedData <- data
		}))
		defer server.Close()

		wsURL := strings.Replace(server.URL, "http", "ws", 1)
		client, err := NewOperatorPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		testData := []byte("test payload")
		err = client.Publish(context.Background(), "test-channel", testData)
		assert.NoError(t, err)

		select {
		case data := <-receivedData:
			var msg pubsubv1.PubSubMessage
			err := proto.Unmarshal(data, &msg)
			assert.NoError(t, err)
			assert.Equal(t, testData, msg.Data)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for published message")
		}

		client.Close()
	})
}

func TestClose(t *testing.T) {
	logger := slog.Default()

	t.Run("closes nil pubWs gracefully", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("ws://localhost:8440", "", logger)
		require.NoError(t, err)
		assert.NotPanics(t, func() {
			client.Close()
		})
	})

	t.Run("closes active pubWs", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()
			<-time.After(1 * time.Second)
		}))
		defer server.Close()

		wsURL := strings.Replace(server.URL, "http", "ws", 1)
		client, err := NewOperatorPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		err = client.Publish(context.Background(), "test-channel", []byte("test"))
		require.NoError(t, err)

		assert.NotNil(t, client.pubWs)
		client.Close()
		assert.Nil(t, client.pubWs)
		assert.True(t, client.closed)
	})
}

func TestSubscribe(t *testing.T) {
	logger := slog.Default()

	t.Run("fails on connection error", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("ws://invalid-host:9999", "", logger)
		require.NoError(t, err)

		_, err = client.Subscribe(context.Background(), "test-channel")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect to Operator pub/sub")
	})

	t.Run("receives subscribed ACK and messages", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()

			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					return
				}

				var msg pubsubv1.PubSubMessage
				if err := proto.Unmarshal(data, &msg); err != nil {
					continue
				}

				if msg.Action == "subscribe" {
					ack := pubsubv1.PubSubEvent{
						Type:    "subscribed",
						Channel: msg.Channel,
					}
					ackBytes, _ := proto.Marshal(&ack)
					conn.WriteMessage(websocket.BinaryMessage, ackBytes)

					testMsg := pubsubv1.PubSubEvent{
						Type: "message",
						Data: []byte("test payload"),
					}
					testMsgBytes, _ := proto.Marshal(&testMsg)
					conn.WriteMessage(websocket.BinaryMessage, testMsgBytes)
				}
			}
		}))
		defer server.Close()

		wsURL := strings.Replace(server.URL, "http", "ws", 1)
		client, err := NewOperatorPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		ch, err := client.Subscribe(context.Background(), "test-channel")
		assert.NoError(t, err)
		assert.NotNil(t, ch)

		select {
		case data := <-ch:
			assert.Equal(t, []byte("test payload"), data)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for message")
		}

		client.Close()
	})

	t.Run("buffers messages before ACK", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()

			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					return
				}

				var msg pubsubv1.PubSubMessage
				if err := proto.Unmarshal(data, &msg); err != nil {
					continue
				}

				if msg.Action == "subscribe" {
					preMsg := pubsubv1.PubSubEvent{
						Type: "message",
						Data: []byte("pre-ack message"),
					}
					preMsgBytes, _ := proto.Marshal(&preMsg)
					conn.WriteMessage(websocket.BinaryMessage, preMsgBytes)

					ack := pubsubv1.PubSubEvent{
						Type:    "subscribed",
						Channel: msg.Channel,
					}
					ackBytes, _ := proto.Marshal(&ack)
					conn.WriteMessage(websocket.BinaryMessage, ackBytes)
				}
			}
		}))
		defer server.Close()

		wsURL := strings.Replace(server.URL, "http", "ws", 1)
		client, err := NewOperatorPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		ch, err := client.Subscribe(context.Background(), "test-channel")
		assert.NoError(t, err)

		select {
		case data := <-ch:
			assert.Equal(t, []byte("pre-ack message"), data)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for buffered message")
		}

		client.Close()
	})

	t.Run("closes channel on context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()

			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					return
				}

				var msg pubsubv1.PubSubMessage
				if err := proto.Unmarshal(data, &msg); err != nil {
					continue
				}

				if msg.Action == "subscribe" {
					ack := pubsubv1.PubSubEvent{
						Type:    "subscribed",
						Channel: msg.Channel,
					}
					ackBytes, _ := proto.Marshal(&ack)
					conn.WriteMessage(websocket.BinaryMessage, ackBytes)
				}
			}
		}))
		defer server.Close()

		wsURL := strings.Replace(server.URL, "http", "ws", 1)
		client, err := NewOperatorPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		ch, err := client.Subscribe(ctx, "test-channel")
		assert.NoError(t, err)

		cancel()

		select {
		case _, ok := <-ch:
			assert.False(t, ok, "channel should be closed")
		case <-time.After(1 * time.Second):
			t.Fatal("channel did not close after context cancellation")
		}

		client.Close()
	})
}

func TestWaitForSubscribedACK(t *testing.T) {
	logger := slog.Default()

	t.Run("returns error on context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()
			<-time.After(10 * time.Second)
		}))
		defer server.Close()

		wsURL := strings.Replace(server.URL, "http", "ws", 1)
		dialer := websocket.Dialer{}
		ws, resp, err := dialer.Dial(wsURL, nil)
		require.NoError(t, err)
		if resp != nil {
			resp.Body.Close()
		}

		client, err := NewOperatorPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		var pending [][]byte
		err = client.waitForSubscribedACK(ctx, ws, "test-channel", &pending)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))

		ws.Close()
	})

	t.Run("returns error on connection close", func(t *testing.T) {
		ctx := context.Background()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			conn.Close()
		}))
		defer server.Close()

		wsURL := strings.Replace(server.URL, "http", "ws", 1)
		dialer := websocket.Dialer{}
		ws, resp, err := dialer.Dial(wsURL, nil)
		require.NoError(t, err)
		if resp != nil {
			resp.Body.Close()
		}

		client, err := NewOperatorPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		var pending [][]byte
		err = client.waitForSubscribedACK(ctx, ws, "test-channel", &pending)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection error")
	})
}
