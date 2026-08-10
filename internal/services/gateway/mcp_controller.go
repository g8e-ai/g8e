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

import (
	"net/http"

	"github.com/g8e-ai/g8e/internal/services/mcp"
)

// MCPController handles MCP/A2A ingress endpoints. It is a thin wrapper around
// mcp.GatewayService that exposes its HTTP-facing methods through the controller
// pattern, so HTTPHandler routes through a controller slot rather than a direct
// field reference.
type MCPController struct {
	mcpGateway *mcp.GatewayService
}

// MCPControllerDeps groups all dependencies for MCPController.
type MCPControllerDeps struct {
	MCPGateway *mcp.GatewayService
}

func newMCPController(d MCPControllerDeps) *MCPController {
	return &MCPController{mcpGateway: d.MCPGateway}
}

// handleMCP delegates to mcp.GatewayService.HandleMCP.
func (c *MCPController) handleMCP(w http.ResponseWriter, r *http.Request) {
	c.mcpGateway.HandleMCP(w, r)
}

// handleA2aCall delegates to mcp.GatewayService.HandleA2aCall.
func (c *MCPController) handleA2aCall(w http.ResponseWriter, r *http.Request) {
	c.mcpGateway.HandleA2aCall(w, r)
}

// MCPGateway returns the underlying mcp.GatewayService for callers that need
// direct access (e.g., GatewayModeService.GetMCPGateway).
func (c *MCPController) MCPGateway() *mcp.GatewayService {
	return c.mcpGateway
}
