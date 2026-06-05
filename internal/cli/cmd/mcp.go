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

package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/spf13/cobra"
)

// mcpCmd implements the MCP stdio transport mode
func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP protocol operations (stdio transport)",
		Long:  `Run g8e as an MCP server using stdio transport for local agent integration. Exposes all native tools without requiring gateway mode.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPStdio(cmd, args)
		},
	}

	return cmd
}

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ToolsListResult represents the result of tools/list
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// CallToolRequest is the params for the "tools/call" method
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// runMCPStdio implements the MCP stdio transport
func runMCPStdio(cmd *cobra.Command, args []string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	logger.Info("g8e MCP stdio server starting")

	// Initialize native tool handler
	nativeToolHandler := mcp.NewNativeToolHandler()

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			logger.Error("Failed to parse JSON-RPC request", "error", err)
			sendError(encoder, nil, -32700, "parse error")
			continue
		}

		logger.Info("Received MCP request", "method", req.Method, "id", req.ID)

		switch req.Method {
		case "tools/list":
			handleToolsList(encoder, req.ID, nativeToolHandler)
		case "tools/call":
			handleToolsCall(encoder, req.ID, req.Params, nativeToolHandler)
		case "initialize":
			handleInitialize(encoder, req.ID)
		case "ping":
			sendSuccess(encoder, req.ID, struct{}{})
		default:
			logger.Warn("Unknown MCP method", "method", req.Method)
			sendError(encoder, req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Error reading stdin", "error", err)
		return err
	}

	logger.Info("g8e MCP stdio server shutting down")
	return nil
}

func handleToolsList(encoder *json.Encoder, id interface{}, nativeToolHandler *mcp.NativeToolHandler) {
	nativeTools := nativeToolHandler.ListTools()
	tools := make([]Tool, 0, len(nativeTools))
	for _, nt := range nativeTools {
		tools = append(tools, Tool{
			Name:        nt.Name(),
			Description: nt.Description(),
			InputSchema: nt.InputSchema(),
		})
	}

	result := ToolsListResult{
		Tools: tools,
	}

	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode tools/list response", "error", err)
	}
}

func handleToolsCall(encoder *json.Encoder, id interface{}, params json.RawMessage, nativeToolHandler *mcp.NativeToolHandler) {
	var callParams CallToolRequest
	if err := json.Unmarshal(params, &callParams); err != nil {
		sendError(encoder, id, -32600, fmt.Sprintf("invalid tools/call params: %v", err))
		return
	}

	if callParams.Name == "" {
		sendError(encoder, id, -32600, "tool name required")
		return
	}

	// Execute native tool
	result, err := nativeToolHandler.HandleTool(context.Background(), callParams.Name, callParams.Arguments)
	if err != nil {
		sendError(encoder, id, -32603, fmt.Sprintf("tool execution failed: %v", err))
		return
	}

	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode tools/call response", "error", err)
	}
}

func handleInitialize(encoder *json.Encoder, id interface{}) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "g8e",
				"version": "1.0.0",
			},
		},
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode initialize response", "error", err)
	}
}

func sendError(encoder *json.Encoder, id interface{}, code int, message string) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode error response", "error", err)
	}
}

func sendSuccess(encoder *json.Encoder, id interface{}, result interface{}) {
	response := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	if err := encoder.Encode(response); err != nil {
		slog.Error("Failed to encode success response", "error", err)
	}
}
