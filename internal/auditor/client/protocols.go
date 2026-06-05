// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// JSONRPCRequest / JSONRPCResponse mirror the Gateway's MCP/A2A ingress shape.
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// rpc posts a JSON-RPC request to an MCP/A2A route and decodes the response.
func (c *Client) rpc(ctx context.Context, p Persona, path, method string, params any) (*JSONRPCResponse, error) {
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params}
	body, _ := json.Marshal(req)
	_, raw, err := c.do(ctx, p, http.MethodPost, c.cfg.MTLSBaseURL+path, body)
	if err != nil {
		return nil, err
	}
	var resp JSONRPCResponse
	if len(raw) > 0 && json.Valid(raw) {
		_ = json.Unmarshal(raw, &resp)
	}
	return &resp, nil
}

// ---- MCP --------------------------------------------------------------------

func (c *Client) MCPToolsList(ctx context.Context, p Persona) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/api/mcp/v1/tools/list", "tools/list", map[string]any{})
}

// MCPToolsCall invokes a tool. The Gateway wraps this into a governed MCP_CALL
// envelope, runs the interlock sequence, and dispatches to the real Operator.
func (c *Client) MCPToolsCall(ctx context.Context, p Persona, tool string, args map[string]any) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/api/mcp/v1/tools/call", "tools/call", map[string]any{
		"name":      tool,
		"arguments": args,
	})
}

func (c *Client) MCPResourcesList(ctx context.Context, p Persona) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/api/mcp/v1/resources/list", "resources/list", map[string]any{})
}

func (c *Client) MCPResourcesRead(ctx context.Context, p Persona, uri string) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/api/mcp/v1/resources/read", "resources/read", map[string]any{"uri": uri})
}

func (c *Client) MCPPromptsList(ctx context.Context, p Persona) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/api/mcp/v1/prompts/list", "prompts/list", map[string]any{})
}

func (c *Client) MCPPromptsGet(ctx context.Context, p Persona, name string, args map[string]any) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/api/mcp/v1/prompts/get", "prompts/get", map[string]any{"name": name, "arguments": args})
}

// ---- A2A --------------------------------------------------------------------

// A2ACall invokes an A2A skill with a plain JSON payload.
func (c *Client) A2ACall(ctx context.Context, p Persona, skill string, payload map[string]any, execID string) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/api/a2a/v1/call", "a2a/call", map[string]any{
		"skill_name":   skill,
		"payload":      payload,
		"execution_id": execID,
	})
}

// A2ACallProto demonstrates A2A carrying a typed protobuf task payload rather
// than loose JSON: it marshals an A2ACallRequested, base64-encodes the bytes,
// and ships them under a content-type marker. The Gateway still wraps it in an
// A2A_CALL governance envelope; the difference is a deterministic, typed,
// schema-checked payload instead of free-form JSON.
func (c *Client) A2ACallProto(ctx context.Context, p Persona, skill string, payloadJSON, execID string) (*JSONRPCResponse, error) {
	msg := &operatorv1.A2ACallRequested{
		SkillName:   skill,
		PayloadJson: payloadJSON,
		ExecutionId: execID,
	}
	raw, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal A2ACallRequested: %w", err)
	}
	return c.rpc(ctx, p, "/api/a2a/v1/call", "a2a/call", map[string]any{
		"skill_name":   skill,
		"execution_id": execID,
		"encoding":     "protobuf",
		"content_type": "application/x-protobuf;type=g8e.operator.v1.A2ACallRequested",
		"payload_b64":  base64.StdEncoding.EncodeToString(raw),
	})
}

// ---- helpers ----------------------------------------------------------------

// Suspended reports whether a JSON-RPC response is an L3 suspension and returns
// the transaction hash to approve. The Gateway returns an approval URL of the
// form https://host:<https-port>/approve/{tx_hash} when a mutation needs a human notary.
func Suspended(resp *JSONRPCResponse) (txHash string, yes bool) {
	if resp == nil {
		return "", false
	}
	hay := ""
	if resp.Error != nil {
		hay = resp.Error.Message + string(resp.Error.Data)
	}
	hay += string(resp.Result)
	// Pull the last path segment of an /approve/{hash} URL if present.
	const marker = "/approve/"
	if i := indexOf(hay, marker); i >= 0 {
		rest := hay[i+len(marker):]
		end := 0
		for end < len(rest) && isHex(rest[end]) {
			end++
		}
		if end > 0 {
			return rest[:end], true
		}
	}
	return "", false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
