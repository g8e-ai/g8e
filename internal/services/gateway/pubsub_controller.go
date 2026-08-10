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
