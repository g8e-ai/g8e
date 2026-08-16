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

// rpcWithCLI is like rpc but uses the CLI-cert TLS client so handleCLIAuth
// stamps the host user's identity onto the suspended transaction.
func (c *Client) rpcWithCLI(ctx context.Context, p Persona, path, method string, params any) (*JSONRPCResponse, error) {
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params}
	body, _ := json.Marshal(req)
	_, raw, err := c.doWithCLI(ctx, p, http.MethodPost, c.cfg.MTLSBaseURL+path, body)
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

// ToolArgs is the typed argument payload for an MCP tools/call invocation.
// Each implementation is a JSON-serializable struct matching a tool's
// argument schema. The harness client marshals the value under the
// "arguments" key of the JSON-RPC params, mirroring the wire shape that
// loose map[string]any values produced previously.
type ToolArgs interface {
	isToolArgs()
}

// ShellCommandArgs are the arguments for the run_shell_command tool.
type ShellCommandArgs struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"`
}

func (ShellCommandArgs) isToolArgs() {}

// FSPathArgs are the arguments for path-only filesystem tools (fs_list, fs_read).
type FSPathArgs struct {
	Path string `json:"path"`
}

func (FSPathArgs) isToolArgs() {}

// FSGrepArgs are the arguments for the fs_grep tool.
type FSGrepArgs struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
}

func (FSGrepArgs) isToolArgs() {}

// FSWriteArgs are the arguments for the fs_write tool.
type FSWriteArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (FSWriteArgs) isToolArgs() {}

// ExecuteBashArgs are the arguments for the execute_bash tool.
type ExecuteBashArgs struct {
	Command string `json:"command"`
}

func (ExecuteBashArgs) isToolArgs() {}

// ---- MCP params envelopes ---------------------------------------------------

// toolsCallParams is the JSON-RPC params envelope for tools/call.
type toolsCallParams struct {
	Name      string   `json:"name"`
	Arguments ToolArgs `json:"arguments"`
}

// resourcesReadParams is the JSON-RPC params envelope for resources/read.
type resourcesReadParams struct {
	URI string `json:"uri"`
}

// promptsGetParams is the JSON-RPC params envelope for prompts/get. Arguments
// holds the schema-less prompt arguments whose shape varies per prompt name.
type promptsGetParams struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments"`
}

func (c *Client) MCPToolsList(ctx context.Context, p Persona) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/mcp", "tools/list", struct{}{})
}

// MCPToolsCall invokes a tool. The Gateway wraps this into a governed MCP_CALL
// envelope, runs the interlock sequence, and dispatches to the real Operator.
func (c *Client) MCPToolsCall(ctx context.Context, p Persona, tool string, args ToolArgs) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/mcp", "tools/call", toolsCallParams{
		Name:      tool,
		Arguments: args,
	})
}

// MCPToolsCallWithCLI is like MCPToolsCall but routes through the CLI-cert
// TLS client so the gateway's handleCLIAuth stamps the host user's identity
// onto the suspended transaction. Use this for notary-scenario submits.
func (c *Client) MCPToolsCallWithCLI(ctx context.Context, p Persona, tool string, args ToolArgs) (*JSONRPCResponse, error) {
	return c.rpcWithCLI(ctx, p, "/mcp", "tools/call", toolsCallParams{
		Name:      tool,
		Arguments: args,
	})
}

func (c *Client) MCPResourcesList(ctx context.Context, p Persona) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/mcp", "resources/list", struct{}{})
}

func (c *Client) MCPResourcesRead(ctx context.Context, p Persona, uri string) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/mcp", "resources/read", resourcesReadParams{URI: uri})
}

func (c *Client) MCPPromptsList(ctx context.Context, p Persona) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/mcp", "prompts/list", struct{}{})
}

// MCPPromptsGet invokes a named prompt. args is a schema-less map because
// prompt arguments vary by prompt definition — the harness client forwards
// the caller-supplied map verbatim without interpreting its shape.
func (c *Client) MCPPromptsGet(ctx context.Context, p Persona, name string, args map[string]any) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/mcp", "prompts/get", promptsGetParams{
		Name:      name,
		Arguments: args,
	})
}

// ---- A2A --------------------------------------------------------------------

// a2aCallParams is the JSON-RPC params envelope for a2a/call with a JSON payload.
type a2aCallParams struct {
	SkillName   string `json:"skill_name"`
	Payload     any    `json:"payload"`
	ExecutionID string `json:"execution_id"`
}

// a2aCallProtoParams is the JSON-RPC params envelope for a2a/call with a
// base64-encoded protobuf payload.
type a2aCallProtoParams struct {
	SkillName   string `json:"skill_name"`
	ExecutionID string `json:"execution_id"`
	Encoding    string `json:"encoding"`
	ContentType string `json:"content_type"`
	PayloadB64  string `json:"payload_b64"`
}

// A2ACall invokes an A2A skill with a plain JSON payload. payload is a
// schema-less map because skill payloads vary by skill definition — the
// harness client forwards the caller-supplied map verbatim without
// interpreting its shape.
func (c *Client) A2ACall(ctx context.Context, p Persona, skill string, payload map[string]any, execID string) (*JSONRPCResponse, error) {
	return c.rpc(ctx, p, "/api/a2a/v1/call", "a2a/call", a2aCallParams{
		SkillName:   skill,
		Payload:     payload,
		ExecutionID: execID,
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
	return c.rpc(ctx, p, "/api/a2a/v1/call", "a2a/call", a2aCallProtoParams{
		SkillName:   skill,
		ExecutionID: execID,
		Encoding:    "protobuf",
		ContentType: "application/x-protobuf;type=g8e.operator.v1.A2ACallRequested",
		PayloadB64:  base64.StdEncoding.EncodeToString(raw),
	})
}

// ---- helpers ----------------------------------------------------------------

// suspensionErrorData is the structured error data payload returned by the
// Gateway when a mutation needs a human notary. The Gateway may include
// additional fields; only ApprovalURL is consumed here.
type suspensionErrorData struct {
	ApprovalURL string `json:"approval_url"`
}

// Suspended reports whether a JSON-RPC response is an L3 suspension and returns
// the transaction hash to approve. The Gateway returns an approval URL of the
// form https://host:<https-port>/approve/{tx_hash} when a mutation needs a human notary.
func Suspended(resp *JSONRPCResponse) (txHash string, yes bool) {
	if resp == nil {
		return "", false
	}

	// First, try to parse structured error data for approval_url field
	if resp.Error != nil && len(resp.Error.Data) > 0 {
		var errData suspensionErrorData
		if json.Unmarshal(resp.Error.Data, &errData) == nil {
			if hash := extractHashFromURL(errData.ApprovalURL); hash != "" {
				return hash, true
			}
		}
	}

	// Fall back to string search in error message and result
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

// extractHashFromURL extracts the transaction hash from an approval URL
func extractHashFromURL(url string) string {
	const marker = "/approve/"
	if i := indexOf(url, marker); i >= 0 {
		rest := url[i+len(marker):]
		end := 0
		for end < len(rest) && isHex(rest[end]) && rest[end] != '?' && rest[end] != '#' && rest[end] != '/' {
			end++
		}
		if end > 0 {
			return rest[:end]
		}
	}
	return ""
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
