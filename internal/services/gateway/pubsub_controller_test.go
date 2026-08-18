// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/testutil"
)

// newTestPubSubController constructs a PubSubController backed by a real
// GatewayWebSocketHandler. No subscribers or DB are needed — the handler is
// fully functional on its own.
func newTestPubSubController(t *testing.T) *PubSubController {
	t.Helper()
	logger := testutil.NewTestLogger()
	h := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { h.Close() })
	return newPubSubController(PubSubControllerDeps{Handler: h})
}

func TestNewPubSubController_Wiring(t *testing.T) {
	c := newTestPubSubController(t)

	assert.NotNil(t, c.WebSocketHandler(), "WebSocketHandler accessor should return the wrapped handler")
}

func TestNewPubSubController_NilDeps(t *testing.T) {
	c := newPubSubController(PubSubControllerDeps{})
	require.NotNil(t, c)
	assert.Nil(t, c.WebSocketHandler(), "WebSocketHandler accessor should be nil when no handler is wired")
}

func TestPubSubController_HandleWebSocket_Delegates(t *testing.T) {
	c := newTestPubSubController(t)

	// A plain HTTP GET (no WebSocket upgrade headers) causes gorilla/websocket's
	// Upgrader to reject the handshake with 400 Bad Request. Reaching that
	// branch proves the controller delegated to GatewayWebSocketHandler.HandleWebSocket.
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rr := httptest.NewRecorder()
	c.handleWebSocket(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code, "non-WebSocket request should be rejected by the upgrade handshake")
}

func TestPubSubController_PublishDelegatesToUnderlyingHandler(t *testing.T) {
	c := newTestPubSubController(t)

	// Publish through the accessor-returned handler. With no subscribers,
	// Publish returns 0 deliveries — proving the controller exposes the same
	// handler instance the broker operates on.
	delivered := c.WebSocketHandler().Publish("test-channel", []byte("hello"))
	assert.Equal(t, 0, delivered, "publish with no subscribers should report 0 deliveries")
}
