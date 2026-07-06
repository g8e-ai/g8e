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

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// Streamable HTTP transport constants for the unified MCP endpoint.
//
// This endpoint implements the single-URL JSON-RPC dispatch contract that
// standard MCP clients (e.g. Claude Code custom connectors) expect:
// one path that accepts POST messages and routes by the JSON-RPC "method"
// field, anchored by the "initialize" handshake.
const (
	// mcpDefaultProtocolVersion is advertised when a client does not request a
	// specific protocol version during initialize. Clients negotiate by
	// sending their own protocolVersion, which we echo when present.
	mcpDefaultProtocolVersion = "2025-06-18"
	mcpServerName             = "g8e-gateway"
	mcpServerVersion          = "1.0"
)

// InitializeResult is the response to the MCP "initialize" handshake.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      MCPServerInfo      `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities describes the server's supported MCP features.
type ServerCapabilities struct {
	Tools     ToolsCapability     `json:"tools"`
	Resources ResourcesCapability `json:"resources"`
	Prompts   PromptsCapability   `json:"prompts"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type ResourcesCapability struct{}

type PromptsCapability struct{}

// resourceTemplatesList is the result for "resources/templates/list".
type resourceTemplatesList struct {
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
}

// initializeParams captures the subset of initialize params we negotiate on.
type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

// HandleMCP is the unified MCP Streamable HTTP endpoint. It accepts a single
// JSON-RPC 2.0 request via POST and dispatches by method through the governed
// execution pipeline. GET is used for SSE streaming support.
//
//	@Summary		MCP endpoint
//	@Description	Unified MCP JSON-RPC endpoint for AI IDE integration
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/mcp [post]
func (g *GatewayService) HandleMCP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// handled below
	case http.MethodGet:
		// SSE endpoint for server-sent events
		g.handleMCPSSE(w, r)
		return
	default:
		w.Header().Set("Allow", "POST, GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, g.maxPayloadBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			maxMB := g.maxPayloadBytes / (1024 * 1024)
			g.responder.RPCError(w, nil, -32600, fmt.Sprintf("request payload too large (max %dMB)", maxMB))
			return
		}
		g.responder.RPCError(w, nil, -32603, "failed to read request body")
		return
	}

	// Reject JSON-RPC batches (an array). The current MCP spec removed batch
	// support; clients send a single message per request.
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		g.responder.RPCError(w, nil, -32600, "JSON-RPC batch requests are not supported")
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		g.responder.RPCError(w, nil, -32700, "parse error: invalid JSON")
		return
	}

	if req.JSONRPC != "2.0" {
		g.responder.RPCError(w, req.ID, -32600, "invalid request: jsonrpc version must be 2.0")
		return
	}

	if req.Method == "" {
		g.responder.RPCError(w, req.ID, -32600, "invalid request: method required")
		return
	}

	// Notifications (including notifications/initialized) expect no response body,
	// only acknowledgement (202 Accepted). They may carry an id or not.
	if strings.HasPrefix(req.Method, "notifications/") || req.Method == "initialized" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	result, rpcErr := g.dispatchMCP(r, &req)
	if rpcErr != nil {
		g.responder.RPCError(w, req.ID, rpcErr.code, rpcErr.message)
		return
	}
	if result == nil {
		// A handled notification-style method with an id but no payload.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	g.responder.RPCResponse(w, req.ID, result)
}

// mcpDispatchError carries a JSON-RPC error code and message.
type mcpDispatchError struct {
	code    int
	message string
}

// dispatchMCP routes a parsed JSON-RPC request to the appropriate governed
// handler and returns either a result or a structured JSON-RPC error.
func (g *GatewayService) dispatchMCP(r *http.Request, req *JSONRPCRequest) (interface{}, *mcpDispatchError) {
	ctx := r.Context()

	// Methods that do not require runtime dependencies
	switch req.Method {
	case "initialize":
		return g.mcpInitialize(req.Params), nil

	case "ping":
		return struct{}{}, nil

	case "resources/templates/list":
		return resourceTemplatesList{ResourceTemplates: []ResourceTemplate{}}, nil
	}

	// All remaining methods require runtime dependencies
	if !g.runtimeReady() {
		return nil, &mcpDispatchError{code: -32603, message: constants.ErrGatewayNotReady.Error()}
	}

	switch req.Method {
	case "tools/list":
		res, err := g.listToolsResult(ctx)
		return g.wrapDispatch(res, err)

	case "tools/call":
		res, err := g.callTool(ctx, r, req.Params)
		return g.wrapDispatch(res, err)

	case "resources/list":
		res, err := g.listResourcesResult(ctx)
		return g.wrapDispatch(res, err)

	case "resources/read":
		res, err := g.readResource(ctx, req.Params)
		return g.wrapDispatch(res, err)

	case "prompts/list":
		res, err := g.listPromptsResult(ctx)
		return g.wrapDispatch(res, err)

	case "prompts/get":
		res, err := g.getPrompt(ctx, req.Params)
		return g.wrapDispatch(res, err)

	case "a2a/call":
		res, err := g.a2aCall(ctx, r, req.Params)
		return g.wrapDispatch(res, err)

	default:
		return nil, &mcpDispatchError{code: -32601, message: fmt.Sprintf("method not found: %s", req.Method)}
	}
}

// wrapDispatch converts a (result, error) pair from a governed handler into the
// dispatcher's (result, *mcpDispatchError) contract, mapping governance errors
// to granular JSON-RPC codes.
func (g *GatewayService) wrapDispatch(result interface{}, err error) (interface{}, *mcpDispatchError) {
	if err != nil {
		code, msg := g.mapGatewayError(err)
		if code == 0 && msg == "" {
			code = -32603
			msg = err.Error()
		}
		return nil, &mcpDispatchError{code: code, message: msg}
	}
	return result, nil
}

// mcpInitialize builds the initialize handshake response, echoing the client's
// requested protocol version when supplied.
func (g *GatewayService) mcpInitialize(params json.RawMessage) InitializeResult {
	protocolVersion := mcpDefaultProtocolVersion
	if len(params) > 0 {
		var p initializeParams
		if err := json.Unmarshal(params, &p); err == nil && p.ProtocolVersion != "" {
			protocolVersion = p.ProtocolVersion
		}
	}

	return InitializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities: ServerCapabilities{
			Tools:     ToolsCapability{ListChanged: true},
			Resources: ResourcesCapability{},
			Prompts:   PromptsCapability{},
		},
		ServerInfo: MCPServerInfo{
			Name:    mcpServerName,
			Version: mcpServerVersion,
		},
		Instructions: "g8e g8e Gateway. Tool calls are executed through the platform's fail-closed governance pipeline; some calls may pause for out-of-band (WebAuthn) authorization.",
	}
}

// listToolsResult returns the tool catalog. With no downstream MCP server
// configured it returns the native tools compiled into the Operator; otherwise
// it proxies tools/list to the downstream server.
func (g *GatewayService) listToolsResult(ctx context.Context) (interface{}, error) {
	if g.getRuntimeDeps().DownstreamURL == "" {
		var nativeTools []NativeTool
		if g.nativeToolHandler != nil {
			nativeTools = g.nativeToolHandler.registry.List()
		}
		tools := make([]Tool, 0, len(nativeTools))
		for _, nt := range nativeTools {
			tools = append(tools, Tool{
				Name:        nt.Name(),
				Description: nt.Description(),
				InputSchema: nt.InputSchema(),
			})
		}
		return ToolsListResult{Tools: tools}, nil
	}
	raw, err := g.proxyListMethod(ctx, "tools/list")
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// listResourcesResult returns the resource catalog (empty with no downstream).
func (g *GatewayService) listResourcesResult(ctx context.Context) (interface{}, error) {
	if g.getRuntimeDeps().DownstreamURL == "" {
		return ResourcesListResult{Resources: []Resource{}}, nil
	}
	raw, err := g.proxyListMethod(ctx, "resources/list")
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// listPromptsResult returns the prompt catalog (empty with no downstream).
func (g *GatewayService) listPromptsResult(ctx context.Context) (interface{}, error) {
	if g.getRuntimeDeps().DownstreamURL == "" {
		return PromptsListResult{Prompts: []Prompt{}}, nil
	}
	raw, err := g.proxyListMethod(ctx, "prompts/list")
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// proxyListMethod forwards a discovery method to the downstream MCP server and
// returns the raw JSON-RPC result payload. It honours the circuit breaker.
func (g *GatewayService) proxyListMethod(ctx context.Context, method string) (json.RawMessage, error) {
	if g.isCircuitOpen() {
		return nil, fmt.Errorf("mcp_endpoint: downstream MCP server is temporarily unavailable (circuit open)")
	}

	reqBody := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q}`, method)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.getRuntimeDeps().DownstreamURL, strings.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("mcp_endpoint: failed to build downstream request: %w", err)
	}
	httpReq.Header.Set(constants.HeaderContentType, "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		g.recordFailure()
		return nil, fmt.Errorf("mcp_endpoint: failed to query downstream MCP server: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			g.logger.Error("mcp_endpoint: failed to close response body", "error", cerr)
		}
	}()

	if resp.StatusCode >= 500 {
		g.recordFailure()
		return nil, fmt.Errorf("mcp_endpoint: downstream MCP server returned status %d", resp.StatusCode)
	}
	g.recordSuccess()

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("mcp_endpoint: failed to decode downstream response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("mcp_endpoint: downstream MCP error: %s", rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// handleMCPSSE handles SSE GET requests on the unified /mcp endpoint.
// This is used by Claude Code and other MCP clients that support SSE transport.
func (g *GatewayService) handleMCPSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no") // For Nginx

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send initial SSE event to confirm connection
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	// Keep connection alive with periodic heartbeats
	// In a full implementation, this would subscribe to events and push them
	// For now, we maintain the connection for tool call responses
	ctx := r.Context()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
