// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"net/http"

	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
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
