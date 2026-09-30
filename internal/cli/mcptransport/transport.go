// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcptransport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// JSONRPCRequest represents a standard JSON-RPC 2.0 request or notification.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a standard JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError represents a standard JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Handler handles an incoming non-notification JSON-RPC request.
type Handler interface {
	Handle(ctx context.Context, req JSONRPCRequest) (JSONRPCResponse, error)
}

// HandlerFunc is an adapter allowing a function with the appropriate signature
// to be used as a Handler.
type HandlerFunc func(ctx context.Context, req JSONRPCRequest) (JSONRPCResponse, error)

// Handle calls f(ctx, req).
func (f HandlerFunc) Handle(ctx context.Context, req JSONRPCRequest) (JSONRPCResponse, error) {
	return f(ctx, req)
}

// SendError writes a JSON-RPC error response to out.
func SendError(out io.Writer, id interface{}, code int, message string) error {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	return json.NewEncoder(out).Encode(resp)
}

// SendSuccess writes a JSON-RPC success response to out.
func SendSuccess(out io.Writer, id interface{}, result interface{}) error {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	return json.NewEncoder(out).Encode(resp)
}

// NewInitializeResponse returns a standard MCP initialize response payload.
func NewInitializeResponse(id interface{}, serverName, serverVersion string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]interface{}{
				"name":    serverName,
				"version": serverVersion,
			},
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{},
				"prompts":   map[string]interface{}{},
			},
		},
	}
}

// ServeStdio reads newline-delimited JSON-RPC requests from in, dispatches them
// to handler, and writes JSON-RPC responses to out.
//
// Rules enforced:
//   - Blank lines are skipped.
//   - Unparseable lines produce a standard JSON-RPC parse error (-32700) response.
//   - Fire-and-forget notifications (requests with ID == nil and non-empty Method)
//     are dropped silently without a response, per MCP and JSON-RPC 2.0 specifications.
//   - Handler errors produce an internal error (-32603) response.
func ServeStdio(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	logger *slog.Logger,
	handler Handler,
) error {
	scanner := bufio.NewScanner(in)
	const maxScanBuffer = 1024 * 1024
	scanner.Buffer(make([]byte, maxScanBuffer), maxScanBuffer)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			if logger != nil {
				logger.Error("Failed to parse JSON-RPC request", "error", err)
			}
			_ = SendError(out, nil, constants.JSONRPCErrorCodeParseError, constants.JSONRPCErrorMessageParseError)
			continue
		}

		// MCP notifications are fire-and-forget. They must not receive a
		// response — drop them silently.
		if req.ID == nil && req.Method != "" {
			if logger != nil {
				logger.Debug("Dropping MCP notification", "method", req.Method)
			}
			continue
		}

		resp, err := handler.Handle(ctx, req)
		if err != nil {
			if logger != nil {
				logger.Error("Failed to handle MCP request", "method", req.Method, "id", req.ID, "error", err)
			}
			_ = SendError(out, req.ID, -32603, fmt.Sprintf("gateway proxy error: %v", err))
			continue
		}

		if err := encoder.Encode(resp); err != nil {
			if logger != nil {
				logger.Error("Failed to encode response", "error", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if logger != nil {
			logger.Error("Error reading stdin", "error", err)
		}
		return fmt.Errorf("mcptransport: read stdin: %w", err)
	}

	return nil
}
