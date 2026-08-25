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

	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// newTestMCPController constructs an MCPController backed by a minimally-wired
// mcp.GatewayService (no envProc, no suspended store, no DB). This is sufficient
// for verifying constructor wiring and that controller methods delegate to the
// underlying service — the service's own error paths (405, "not ready") prove
// the call reached it.
func newTestMCPController(t *testing.T) *MCPController {
	t.Helper()
	logger := testutil.NewTestLogger()
	resp := response.NewWriter(logger)
	g, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:     logger,
		Responder:  resp,
		AuditStore: mcp.NoopAuditEventRecorder{},
	})
	require.NoError(t, err)
	return newMCPController(MCPControllerDeps{MCPGateway: g})
}

func TestNewMCPController_Wiring(t *testing.T) {
	c := newTestMCPController(t)

	assert.NotNil(t, c.MCPGateway(), "MCPGateway accessor should return the wrapped service")
}

func TestMCPController_NewMCPController_NilDeps(t *testing.T) {
	// A zero-value MCPControllerDeps must produce a non-nil controller whose
	// accessor returns nil — this is the posture HTTPHandler would be in if
	// MCP were not configured.
	c := newMCPController(MCPControllerDeps{})
	require.NotNil(t, c)
	assert.Nil(t, c.MCPGateway(), "MCPGateway accessor should be nil when no gateway is wired")
}

func TestMCPController_HandleMCP_Delegates(t *testing.T) {
	c := newTestMCPController(t)

	// HandleMCP rejects non-GET/POST methods with 405 + Allow header. Reaching
	// that branch proves the controller delegated to mcp.GatewayService.HandleMCP.
	req := httptest.NewRequest(http.MethodPut, "/mcp", nil)
	rr := httptest.NewRecorder()
	c.handleMCP(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	assert.Equal(t, "POST, GET", rr.Header().Get("Allow"))
}

func TestMCPController_HandleA2aCall_Delegates(t *testing.T) {
	c := newTestMCPController(t)

	// With nil envProc, HandleA2aCall emits a JSON-RPC -32603 "not ready"
	// error. Reaching that branch proves the controller delegated.
	req := httptest.NewRequest(http.MethodPost, "/a2a/call", nil)
	rr := httptest.NewRecorder()
	c.handleA2aCall(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "JSON-RPC errors are returned with HTTP 200")
	assert.Contains(t, rr.Body.String(), "-32603")
	assert.Contains(t, rr.Body.String(), "not ready")
}
