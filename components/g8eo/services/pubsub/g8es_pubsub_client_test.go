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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestG8esPubSubClient_CheckTLSConnectivity verifies that checkTLSConnectivity
// correctly distinguishes between transport-layer failures and proxy rejections.
func TestG8esPubSubClient_CheckTLSConnectivity(t *testing.T) {
	t.Run("proxy rejection is treated as connectivity success", func(t *testing.T) {
		// Simulate the g8ed proxy rejecting a sessionless connection with HTTP 401.
		// The gorilla/websocket dialer returns a non-nil *http.Response on upgrade
		// failure, which is exactly the signal checkTLSConnectivity uses to confirm
		// that TCP and TLS are healthy.
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Operator session required", http.StatusUnauthorized)
		}))
		defer server.Close()

		wsURL := "ws://" + strings.TrimPrefix(server.Listener.Addr().String(), "http://")
		logger := testutil.NewTestLogger()
		client, err := NewG8esPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		err = client.checkTLSConnectivity(context.Background())
		assert.NoError(t, err, "proxy rejection should be treated as connectivity success")
	})

	t.Run("connection refused returns error", func(t *testing.T) {
		// Use a port that is not listening — this produces a transport-level error
		// with no HTTP response, which checkTLSConnectivity must propagate.
		logger := testutil.NewTestLogger()
		client, err := NewG8esPubSubClient("ws://127.0.0.1:19999", "", logger)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 2)
		defer cancel()

		err = client.checkTLSConnectivity(ctx)
		assert.Error(t, err, "connection refused should return an error")
	})

	t.Run("successful WebSocket upgrade is treated as connectivity success", func(t *testing.T) {
		// Simulate a server that accepts the WebSocket upgrade — the proxy is not
		// involved. checkTLSConnectivity must close the connection cleanly and succeed.
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			ws.Close()
		}))
		defer server.Close()

		wsURL := "ws://" + strings.TrimPrefix(server.Listener.Addr().String(), "http://")
		logger := testutil.NewTestLogger()
		client, err := NewG8esPubSubClient(wsURL, "", logger)
		require.NoError(t, err)

		err = client.checkTLSConnectivity(context.Background())
		assert.NoError(t, err, "successful upgrade should be treated as connectivity success")
	})
}

// TestG8esPubSubClient_Close verifies that Close marks the client closed and
// prevents subsequent Publish calls.
func TestG8esPubSubClient_Close(t *testing.T) {
	t.Run("publish after close returns error", func(t *testing.T) {
		logger := testutil.NewTestLogger()
		client, err := NewG8esPubSubClient("ws://"+constants.DefaultEndpoint+":443", "", logger)
		require.NoError(t, err)

		client.Close()

		err = client.Publish(context.Background(), "test-channel", []byte(`{}`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "closed")
	})
}

// TestG8esPubSubClient_ConnectPubWs verifies that connectPubWs (formerly
// ensurePubWs) is called correctly via the explicit nil guard in Publish, and
// that repeated Publish calls reuse the existing connection rather than
// reconnecting on every call.
func TestG8esPubSubClient_ConnectPubWs_ReusesConnection(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	var connectCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connectCount.Add(1)
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws://" + strings.TrimPrefix(server.Listener.Addr().String(), "http://")
	logger := testutil.NewTestLogger()
	client, err := NewG8esPubSubClient(wsURL, "", logger)
	require.NoError(t, err)
	defer client.Close()

	for i := 0; i < 3; i++ {
		require.NoError(t, client.Publish(context.Background(), "ch", []byte(`{}`)))
	}

	assert.Equal(t, int32(1), connectCount.Load(), "connectPubWs must be called once and the connection reused")
}

// TestG8esPubSubClient_WaitForSubscribedACK_ContextCancellation verifies that
// waitForSubscribedACK returns promptly when the context is cancelled, without
// waiting for the full ackTimeout.
func TestG8esPubSubClient_WaitForSubscribedACK_ContextCancellation(t *testing.T) {
	// Server that accepts the WebSocket upgrade but never sends any frames —
	// simulates a broker that is slow to ACK the subscription.
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		// Hold the connection open but never send the subscribed ACK.
		time.Sleep(10 * time.Second)
	}))
	defer server.Close()

	wsURL := "ws://" + strings.TrimPrefix(server.Listener.Addr().String(), "http://")
	logger := testutil.NewTestLogger()
	client, err := NewG8esPubSubClient(wsURL, "", logger)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())

	start := time.Now()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err = client.Subscribe(ctx, "test-channel")
	elapsed := time.Since(start)

	assert.Error(t, err, "Subscribe must return an error when context is cancelled")
	// Must return well before the 5-second ackTimeout — context deadline is
	// respected, not just detected at the next frame boundary.
	assert.Less(t, elapsed, 2*time.Second,
		"Subscribe took too long after context cancellation: %v", elapsed)
}
