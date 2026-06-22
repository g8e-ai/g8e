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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/constants"
	pubsubv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/pubsub/v1"
)

func generateTestCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber:          serial,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

func newTestCertsTLSConfig(t *testing.T) *certs.TLSConfig {
	t.Helper()
	trustStore := certs.NewTrustStore(generateTestCAPEM(t))
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})
	return certs.NewTLSConfig(trustStore, clientIdentity)
}

func newTestCertsTLSConfigForServer(t *testing.T, server *httptest.Server) *certs.TLSConfig {
	t.Helper()
	serverCert := server.Certificate()
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw})
	trustStore := certs.NewTrustStore(caPEM)
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})
	return certs.NewTLSConfig(trustStore, clientIdentity)
}

func newTestRawTLSConfigForServer(t *testing.T, server *httptest.Server) *tls.Config {
	t.Helper()
	certsTLSConfig := newTestCertsTLSConfigForServer(t, server)
	tlsCfg, err := certsTLSConfig.GetTLSConfig()
	require.NoError(t, err)
	return tlsCfg
}

func httpsToWss(url string) string {
	return strings.Replace(url, "https://", "wss://", 1)
}

func TestNewOperatorPubSubClient(t *testing.T) {
	logger := slog.Default()
	tlsCfg := newTestCertsTLSConfig(t)

	t.Run("rejects empty baseURL", func(t *testing.T) {
		client, err := NewOperatorPubSubClient("", "", logger, tlsCfg)
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Error(t, err)
	})

	t.Run("rejects ws:// URL", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("ws://localhost:%d", constants.Ports.OperatorHttp), "", logger, tlsCfg)
		require.Error(t, err)
		assert.Nil(t, client)
	})

	t.Run("rejects nil certsTLSConfig", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), "", logger, nil)
		require.Error(t, err)
		assert.Nil(t, client)
	})

	t.Run("accepts wss:// URL", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), "", logger, tlsCfg)
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), client.baseURL)
		assert.NotNil(t, client.tlsConfig)
	})

	t.Run("sets serverName for TLS SNI override", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://192.168.1.1:%d", constants.Ports.OperatorHttp), "gateway.local", logger, tlsCfg)
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "gateway.local", client.serverName)
		assert.Equal(t, "gateway.local", client.tlsConfig.ServerName)
	})
}

func TestPubSubWSURL(t *testing.T) {
	logger := slog.Default()
	tlsCfg := newTestCertsTLSConfig(t)

	t.Run("returns correct URL for wss://", func(t *testing.T) {
		client, err := NewOperatorPubSubClient(fmt.Sprintf("wss://localhost:%d", constants.Ports.OperatorHttp), "", logger, tlsCfg)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("wss://localhost:%d/api/v1/pubsub/stream", constants.Ports.OperatorHttp), client.pubSubWSURL())
	})
}

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
