// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import "net/http"

// PubSubController handles the PubSub WebSocket stream endpoint. It is a thin
// wrapper around GatewayWebSocketHandler that exposes its HTTP-facing method
// through the controller pattern, so HTTPHandler routes through a controller
// slot rather than a direct field reference.
type PubSubController struct {
	handler *GatewayWebSocketHandler
}

// PubSubControllerDeps groups all dependencies for PubSubController.
type PubSubControllerDeps struct {
	Handler *GatewayWebSocketHandler
}

func newPubSubController(d PubSubControllerDeps) *PubSubController {
	return &PubSubController{handler: d.Handler}
}

// handleWebSocket delegates to GatewayWebSocketHandler.HandleWebSocket.
func (c *PubSubController) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	c.handler.HandleWebSocket(w, r)
}

// WebSocketHandler returns the underlying GatewayWebSocketHandler for callers
// that need direct access (e.g., GatewayModeService.GetGatewayWebSocketHandler).
func (c *PubSubController) WebSocketHandler() *GatewayWebSocketHandler {
	return c.handler
}
