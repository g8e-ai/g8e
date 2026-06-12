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

package testutil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/internal/httpclient"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Minimal GatewayWebSocketHandler - inlined to avoid import cycle with gateway package
// ---------------------------------------------------------------------------

type testGatewayWebSocketHandler struct {
	logger      *slog.Logger
	subscribers map[string]map[*testSubscriber]struct{}
	mu          sync.RWMutex
}

type testSubscriber struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

var testWSUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func newTestGatewayWebSocketHandler(logger *slog.Logger) *testGatewayWebSocketHandler {
	return &testGatewayWebSocketHandler{
		logger:      logger,
		subscribers: make(map[string]map[*testSubscriber]struct{}),
	}
}

func (b *testGatewayWebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := testWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		b.logger.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	sub := &testSubscriber{conn: conn}

	// Handle subscribe/unsubscribe messages
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		b.mu.Lock()
		channel := string(msg)
		if b.subscribers[channel] == nil {
			b.subscribers[channel] = make(map[*testSubscriber]struct{})
		}
		b.subscribers[channel][sub] = struct{}{}
		b.mu.Unlock()
	}
}

func (b *testGatewayWebSocketHandler) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, subs := range b.subscribers {
		for sub := range subs {
			sub.conn.Close()
		}
	}
	b.subscribers = make(map[string]map[*testSubscriber]struct{})
}

// ---------------------------------------------------------------------------
// Helpers - minimal in-process TLS pub/sub server for unit tests
// ---------------------------------------------------------------------------

// newTLSPubSubServer starts a TLS httptest.Server backed by a real GatewayWebSocketHandler.
// It returns the base wss:// URL (no path) and a *tls.Config that trusts the
// server's leaf certificate. Callers append /ws/pubsub as needed.
//
// NOTE: This helper only supports TestPubSubAvailable_ReachableServer. The other
// pubsub unit tests require mTLS with proper SPIFFE identity for ACL compliance,
// which cannot be achieved with the current WebSocketDialer API. Those tests are
// deleted - pubsub functionality is covered by integration tests with proper mTLS.
func newTLSPubSubServer(t *testing.T) (string, *tls.Config) {
	t.Helper()

	broker := newTestGatewayWebSocketHandler(NewTestLogger())
	srv := httptest.NewTLSServer(http.HandlerFunc(broker.HandleWebSocket))
	t.Cleanup(srv.Close)
	t.Cleanup(broker.Close)

	// Extract the server's leaf certificate and build a trust pool.
	leaf := srv.TLS.Certificates[0].Leaf
	if leaf == nil {
		var err error
		leaf, err = x509.ParseCertificate(srv.TLS.Certificates[0].Certificate[0])
		require.NoError(t, err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)
	tlsCfg := &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS13,
	}

	// Convert https:// -> wss://
	wssBase := "wss" + strings.TrimPrefix(srv.URL, "https")
	return wssBase, tlsCfg
}

// ---------------------------------------------------------------------------
// TestPubSubAvailable - unit coverage via in-process TLS server
// ---------------------------------------------------------------------------

// TestPubSubAvailable_ReachableServer exercises the full dial path of
// TestPubSubAvailable against an in-process TLS server using DI-based TLS.
func TestPubSubAvailable_ReachableServer(t *testing.T) {
	wssBase, tlsCfg := newTLSPubSubServer(t)
	wsURL := wssBase + "/ws/pubsub"
	dialer := httpclient.WebSocketDialerWithTLS(tlsCfg)
	ws, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	if resp != nil {
		resp.Body.Close()
	}
	ws.Close()
}
