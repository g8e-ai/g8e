// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package pubsub

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	pubsubv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/pubsub/v1"
)

func TestConnectPubWs(t *testing.T) {
	logger := slog.Default()

	t.Run("fails on invalid endpoint", func(t *testing.T) {
		tlsCfg := newTestCertsTLSConfig(t)
		client, err := NewOperatorPubSubClient("wss://invalid-host-that-does-not-exist:9999", "", logger, tlsCfg)
		require.NoError(t, err)

		client.mu.Lock()
		err = client.connectPubWs()
		client.mu.Unlock()

		require.Error(t, err)
		assert.Error(t, err)
	})

	t.Run("succeeds on valid endpoint", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()
		}))
		defer server.Close()

		wsURL := httpsToWss(server.URL)
		tlsCfg := newTestCertsTLSConfigForServer(t, server)
		client, err := NewOperatorPubSubClient(wsURL, "", logger, tlsCfg)
		require.NoError(t, err)

		client.mu.Lock()
		err = client.connectPubWs()
		client.mu.Unlock()

		require.NoError(t, err)
		assert.NotNil(t, client.pubWs)

		client.Close()
	})
}

func TestPublish(t *testing.T) {
	logger := slog.Default()
	tlsCfg := newTestCertsTLSConfig(t)

	t.Run("fails when client is closed", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), "", logger, tlsCfg)
		require.NoError(t, err)
		client.Close()

		err = client.Publish(context.Background(), "test-channel", []byte("test data"))
		require.Error(t, err)
		assert.Error(t, err)
	})

	t.Run("fails on connection error", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("wss://invalid-host:9999", "", logger, tlsCfg)
		require.NoError(t, err)

		err = client.Publish(context.Background(), "test-channel", []byte("test data"))
		require.Error(t, err)
		assert.Error(t, err)
	})

	t.Run("succeeds on valid connection", func(t *testing.T) {
		receivedData := make(chan []byte, 1)
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()

			_, data, err := conn.ReadMessage()
			assert.NoError(t, err)
			receivedData <- data
		}))
		defer server.Close()

		wsURL := httpsToWss(server.URL)
		serverTLSCfg := newTestCertsTLSConfigForServer(t, server)
		client, err := NewOperatorPubSubClient(wsURL, "", logger, serverTLSCfg)
		require.NoError(t, err)

		testData := []byte("test payload")
		err = client.Publish(context.Background(), "test-channel", testData)
		require.NoError(t, err)

		select {
		case data := <-receivedData:
			var msg pubsubv1.PubSubMessage
			err := proto.Unmarshal(data, &msg)
			require.NoError(t, err)
			assert.Equal(t, testData, msg.Data)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for published message")
		}

		client.Close()
	})
}

func TestClose(t *testing.T) {
	logger := slog.Default()
	tlsCfg := newTestCertsTLSConfig(t)

	t.Run("closes nil pubWs gracefully", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), "", logger, tlsCfg)
		require.NoError(t, err)
		assert.NotPanics(t, func() {
			client.Close()
		})
	})

	t.Run("closes active pubWs", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()
			<-time.After(1 * time.Second)
		}))
		defer server.Close()

		wsURL := httpsToWss(server.URL)
		serverTLSCfg := newTestCertsTLSConfigForServer(t, server)
		client, err := NewOperatorPubSubClient(wsURL, "", logger, serverTLSCfg)
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
	tlsCfg := newTestCertsTLSConfig(t)

	t.Run("fails on connection error", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("wss://invalid-host:9999", "", logger, tlsCfg)
		require.NoError(t, err)

		_, err = client.Subscribe(context.Background(), "test-channel")
		require.Error(t, err)
		assert.Error(t, err)
	})

	t.Run("receives subscribed ACK and messages", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		wsURL := httpsToWss(server.URL)
		serverTLSCfg := newTestCertsTLSConfigForServer(t, server)
		client, err := NewOperatorPubSubClient(wsURL, "", logger, serverTLSCfg)
		require.NoError(t, err)

		ch, err := client.Subscribe(context.Background(), "test-channel")
		require.NoError(t, err)
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
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		wsURL := httpsToWss(server.URL)
		serverTLSCfg := newTestCertsTLSConfigForServer(t, server)
		client, err := NewOperatorPubSubClient(wsURL, "", logger, serverTLSCfg)
		require.NoError(t, err)

		ch, err := client.Subscribe(context.Background(), "test-channel")
		require.NoError(t, err)

		select {
		case data := <-ch:
			assert.Equal(t, []byte("pre-ack message"), data)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for buffered message")
		}

		client.Close()
	})

	t.Run("closes channel on context cancellation", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		wsURL := httpsToWss(server.URL)
		serverTLSCfg := newTestCertsTLSConfigForServer(t, server)
		client, err := NewOperatorPubSubClient(wsURL, "", logger, serverTLSCfg)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		ch, err := client.Subscribe(ctx, "test-channel")
		require.NoError(t, err)

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

// TestSubscribe_PresentsTheDeploymentLaunchID verifies that a worker launched
// by `operator deploy` identifies its launch on the subscription dial, which is
// what lets the Gateway announce its established command subscription to the
// deploying CLI, and that any other worker presents no launch header.
func TestSubscribe_PresentsTheDeploymentLaunchID(t *testing.T) {
	for name, launchID := range map[string]string{
		"deploy-launched": "35fe96f6-cb3c-4e7e-a392-ed72e84ac9ad",
		"started by hand": "",
	} {
		t.Run(name, func(t *testing.T) {
			got := make(chan []string, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				conn, err := upgrader.Upgrade(w, r, nil)
				assert.NoError(t, err)
				defer conn.Close()
				got <- r.Header.Values(constants.HeaderDeploymentID)

				_, data, err := conn.ReadMessage()
				if err != nil {
					return
				}
				var msg pubsubv1.PubSubMessage
				assert.NoError(t, proto.Unmarshal(data, &msg))
				ack, _ := proto.Marshal(&pubsubv1.PubSubEvent{Type: constants.PubSubEventSubscribed, Channel: msg.Channel})
				_ = conn.WriteMessage(websocket.BinaryMessage, ack)
				_, _, _ = conn.ReadMessage()
			}))
			defer server.Close()

			client, err := NewOperatorPubSubClient(httpsToWss(server.URL), "", slog.Default(), newTestCertsTLSConfigForServer(t, server))
			require.NoError(t, err)
			client.SetDeploymentID(launchID)

			_, err = client.Subscribe(context.Background(), "cmd:op-1:sess-1")
			require.NoError(t, err)
			client.Close()

			select {
			case values := <-got:
				if launchID == "" {
					assert.Empty(t, values)
				} else {
					assert.Equal(t, []string{launchID}, values)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("dial never reached the server")
			}
		})
	}
}

func TestWaitForSubscribedACK(t *testing.T) {
	logger := slog.Default()

	t.Run("returns error on context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			defer conn.Close()
			<-time.After(10 * time.Second)
		}))
		defer server.Close()

		wsURL := httpsToWss(server.URL)
		rawTLSCfg := newTestRawTLSConfigForServer(t, server)
		dialer := websocket.Dialer{TLSClientConfig: rawTLSCfg}
		ws, resp, err := dialer.Dial(wsURL, nil)
		assert.NoError(t, err) //nolint:testifylint,require-error // in http handler
		if resp != nil {
			resp.Body.Close()
		}

		serverTLSCfg := newTestCertsTLSConfigForServer(t, server)
		client, err := NewOperatorPubSubClient(wsURL, "", logger, serverTLSCfg)
		assert.NoError(t, err) //nolint:testifylint,require-error // in http handler

		var pending [][]byte
		err = client.waitForSubscribedACK(ctx, ws, "test-channel", &pending)
		assert.Error(t, err)                     //nolint:testifylint,require-error // in http handler
		assert.ErrorIs(t, err, context.Canceled) //nolint:testifylint,require-error // in http handler

		ws.Close()
	})

	t.Run("returns error on connection close", func(t *testing.T) {
		ctx := context.Background()

		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			conn, err := upgrader.Upgrade(w, r, nil)
			assert.NoError(t, err)
			conn.Close()
		}))
		defer server.Close()

		wsURL := httpsToWss(server.URL)
		rawTLSCfg := newTestRawTLSConfigForServer(t, server)
		dialer := websocket.Dialer{TLSClientConfig: rawTLSCfg}
		ws, resp, err := dialer.Dial(wsURL, nil)
		assert.NoError(t, err) //nolint:testifylint,require-error // in http handler
		if resp != nil {
			resp.Body.Close()
		}

		serverTLSCfg := newTestCertsTLSConfigForServer(t, server)
		client, err := NewOperatorPubSubClient(wsURL, "", logger, serverTLSCfg)
		assert.NoError(t, err) //nolint:testifylint,require-error // in http handler

		var pending [][]byte
		err = client.waitForSubscribedACK(ctx, ws, "test-channel", &pending)
		assert.Error(t, err) //nolint:testifylint,require-error // in http handler
		assert.Contains(t, err.Error(), "connection error")
	})
}

// TestSubscribe_EmitsConnectedLogLine asserts that Subscribe emits the
// "operator pub/sub WebSocket connected" log marker after a successful dial
// and before the subscription message is sent. This is the stable connectivity
// marker the Tier 3 E2E suite greps for (R4); it must be distinct from the
// pre-dial "Dialing Operator pub/sub WebSocket" line and the per-subscription
// "operator pub/sub subscription confirmed" line.
func TestSubscribe_EmitsConnectedLogLine(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				ack := pubsubv1.PubSubEvent{Type: "subscribed", Channel: msg.Channel}
				ackBytes, _ := proto.Marshal(&ack)
				conn.WriteMessage(websocket.BinaryMessage, ackBytes)
			}
		}
	}))
	defer server.Close()

	wsURL := httpsToWss(server.URL)
	serverTLSCfg := newTestCertsTLSConfigForServer(t, server)
	client, err := NewOperatorPubSubClient(wsURL, "", logger, serverTLSCfg)
	require.NoError(t, err)

	ch, err := client.Subscribe(context.Background(), "test-channel")
	require.NoError(t, err)
	assert.NotNil(t, ch)
	client.Close()

	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "operator pub/sub WebSocket connected")
	assert.Contains(t, logOutput, "channel=test-channel")
	assert.Contains(t, logOutput, "tls_enabled=true")
	// The connected marker must be distinct from the pre-dial and
	// subscription-confirmed markers.
	assert.Contains(t, logOutput, "Dialing Operator pub/sub WebSocket")
	assert.Contains(t, logOutput, "operator pub/sub subscription confirmed")
}

// TestConnectPubWs_EmitsConnectedLogLine asserts that connectPubWs emits the
// "operator pub/sub WebSocket connected" log marker with direction=publish
// after a successful dial. This is the publish-path counterpart to the
// subscribe-path marker and is the second stable connectivity marker the
// Tier 3 E2E suite greps for (R4).
func TestConnectPubWs_EmitsConnectedLogLine(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		assert.NoError(t, err)
		defer conn.Close()
		<-time.After(1 * time.Second)
	}))
	defer server.Close()

	wsURL := httpsToWss(server.URL)
	serverTLSCfg := newTestCertsTLSConfigForServer(t, server)
	client, err := NewOperatorPubSubClient(wsURL, "", logger, serverTLSCfg)
	require.NoError(t, err)

	client.mu.Lock()
	err = client.connectPubWs()
	client.mu.Unlock()
	require.NoError(t, err)
	client.Close()

	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "operator pub/sub WebSocket connected")
	assert.Contains(t, logOutput, "direction=publish")
	assert.Contains(t, logOutput, "tls_enabled=true")
}
