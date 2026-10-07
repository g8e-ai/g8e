// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testutil

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
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

// ---------------------------------------------------------------------------
// TestPubSubAvailable - unit coverage via in-process TLS server
// ---------------------------------------------------------------------------
