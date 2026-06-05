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
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mapMethodToPath(method string) (string, error) {
	switch method {
	case "tools/list":
		return "/tools/list", nil
	case "tools/call":
		return "/tools/call", nil
	case "resources/list":
		return "/resources/list", nil
	case "resources/read":
		return "/resources/read", nil
	case "prompts/list":
		return "/prompts/list", nil
	case "prompts/get":
		return "/prompts/get", nil
	default:
		return "", fmt.Errorf("unsupported method: %s", method)
	}
}

func TestMapMethodToPath(t *testing.T) {
	t.Run("tools/list maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("tools/list")
		assert.NoError(t, err)
		assert.Equal(t, "/tools/list", path)
	})

	t.Run("tools/call maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("tools/call")
		assert.NoError(t, err)
		assert.Equal(t, "/tools/call", path)
	})

	t.Run("resources/list maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("resources/list")
		assert.NoError(t, err)
		assert.Equal(t, "/resources/list", path)
	})

	t.Run("resources/read maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("resources/read")
		assert.NoError(t, err)
		assert.Equal(t, "/resources/read", path)
	})

	t.Run("prompts/list maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("prompts/list")
		assert.NoError(t, err)
		assert.Equal(t, "/prompts/list", path)
	})

	t.Run("prompts/get maps correctly", func(t *testing.T) {
		path, err := mapMethodToPath("prompts/get")
		assert.NoError(t, err)
		assert.Equal(t, "/prompts/get", path)
	})

	t.Run("unsupported method returns error", func(t *testing.T) {
		path, err := mapMethodToPath("unknown/method")
		assert.Error(t, err)
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "unsupported method")
	})
}

func TestMcpCmd(t *testing.T) {
	t.Run("mcp command has correct use and description", func(t *testing.T) {
		cmd := mcpCmd()
		assert.Equal(t, "mcp", cmd.Use)
		assert.Contains(t, cmd.Short, "MCP protocol operations")
		assert.Contains(t, cmd.Short, "stdio")
		assert.Contains(t, cmd.Long, "stdio transport")
	})

	t.Run("mcp command has no required flags", func(t *testing.T) {
		cmd := mcpCmd()
		require.NotNil(t, cmd)

		// Should not have endpoint or pki-dir flags anymore
		endpointFlag := cmd.Flags().Lookup("endpoint")
		pkiDirFlag := cmd.Flags().Lookup("pki-dir")

		assert.Nil(t, endpointFlag, "mcp command should not have --endpoint flag")
		assert.Nil(t, pkiDirFlag, "mcp command should not have --pki-dir flag")
	})
}

func TestJSONRPCRequest(t *testing.T) {
	t.Run("valid JSON-RPC request parses correctly", func(t *testing.T) {
		jsonStr := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
		var req JSONRPCRequest
		err := json.Unmarshal([]byte(jsonStr), &req)
		assert.NoError(t, err)
		assert.Equal(t, "2.0", req.JSONRPC)
		assert.Equal(t, float64(1), req.ID)
		assert.Equal(t, "tools/list", req.Method)
	})

	t.Run("JSON-RPC request without params parses correctly", func(t *testing.T) {
		jsonStr := `{"jsonrpc":"2.0","id":2,"method":"initialize"}`
		var req JSONRPCRequest
		err := json.Unmarshal([]byte(jsonStr), &req)
		assert.NoError(t, err)
		assert.Equal(t, "initialize", req.Method)
		assert.Nil(t, req.Params)
	})
}

func TestJSONRPCResponse(t *testing.T) {
	t.Run("JSON-RPC response with result serializes correctly", func(t *testing.T) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  map[string]string{"status": "ok"},
		}
		data, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "2.0")
		assert.Contains(t, string(data), "result")
	})

	t.Run("JSON-RPC response with error serializes correctly", func(t *testing.T) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Error: &RPCError{
				Code:    -32601,
				Message: "method not found",
			},
		}
		data, err := json.Marshal(resp)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "error")
		assert.Contains(t, string(data), "-32601")
	})
}

func TestRPCError(t *testing.T) {
	t.Run("RPC error structure is correct", func(t *testing.T) {
		err := RPCError{
			Code:    -32700,
			Message: "parse error",
			Data:    "invalid JSON",
		}
		assert.Equal(t, -32700, err.Code)
		assert.Equal(t, "parse error", err.Message)
		assert.Equal(t, "invalid JSON", err.Data)
	})
}

func TestToolsListResult(t *testing.T) {
	t.Run("tools list result structure is correct", func(t *testing.T) {
		result := ToolsListResult{
			Tools: []Tool{
				{
					Name:        "test_tool",
					Description: "A test tool",
					InputSchema: map[string]interface{}{"type": "object"},
				},
			},
		}
		assert.Len(t, result.Tools, 1)
		assert.Equal(t, "test_tool", result.Tools[0].Name)
	})
}

func TestTool(t *testing.T) {
	t.Run("tool structure is correct", func(t *testing.T) {
		tool := Tool{
			Name:        "execute_bash",
			Description: "Execute a bash command",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The command to execute",
					},
				},
			},
		}
		assert.Equal(t, "execute_bash", tool.Name)
		assert.Equal(t, "Execute a bash command", tool.Description)
		assert.NotNil(t, tool.InputSchema)
	})
}

func TestHandleInitialize(t *testing.T) {
	t.Run("initialize response has correct structure", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		handleInitialize(encoder, 1)

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.Equal(t, float64(1), resp.ID)
		assert.NotNil(t, resp.Result)

		result := resp.Result.(map[string]interface{})
		assert.Contains(t, result, "protocolVersion")
		assert.Contains(t, result, "capabilities")
		assert.Contains(t, result, "serverInfo")
	})
}

func TestHandleToolsList(t *testing.T) {
	t.Run("tools/list response contains native tools", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		nativeToolHandler := mcp.NewNativeToolHandler()
		handleToolsList(encoder, 1, nativeToolHandler)

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		assert.NoError(t, err)
		assert.NotNil(t, resp.Result)

		result := resp.Result.(map[string]interface{})
		tools := result["tools"].([]interface{})
		assert.Greater(t, len(tools), 0)
	})
}

func TestSendError(t *testing.T) {
	t.Run("error response has correct structure", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		sendError(encoder, 1, -32601, "method not found")

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.Equal(t, float64(1), resp.ID)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, -32601, resp.Error.Code)
		assert.Equal(t, "method not found", resp.Error.Message)
	})
}

func TestHandleToolsCall(t *testing.T) {
	t.Run("tools/call executes native tool", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		nativeToolHandler := mcp.NewNativeToolHandler()
		handleToolsCall(encoder, 1, json.RawMessage(`{"name":"sys_info","arguments":{}}`), nativeToolHandler)

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		assert.NoError(t, err)
		assert.NotNil(t, resp.Result)
	})
}

func TestMcpStdioProxyCmd(t *testing.T) {
	t.Run("stdio-proxy command exists", func(t *testing.T) {
		cmd := mcpStdioProxyCmd()
		assert.NotNil(t, cmd)
		assert.Contains(t, cmd.Use, "stdio-proxy")
		assert.Contains(t, cmd.Short, "Proxy stdio MCP requests")
	})
}

func TestIsL3ApprovalResponse(t *testing.T) {
	t.Run("detects L3 approval response", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Execution paused. Please visit https://example.com/approve/123 to authorize",
					},
				},
			},
		}
		assert.True(t, isL3ApprovalResponse(resp))
	})

	t.Run("does not detect non-approval response", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Command executed successfully",
					},
				},
			},
		}
		assert.False(t, isL3ApprovalResponse(resp))
	})

	t.Run("handles nil result", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: nil,
		}
		assert.False(t, isL3ApprovalResponse(resp))
	})
}

func TestExtractApprovalURL(t *testing.T) {
	t.Run("extracts URL from approval response", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Execution paused. Please visit https://example.com/approve/123 to authorize",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/approve/123", url)
	})

	t.Run("handles URL at end of string", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Please visit https://example.com/approve/456",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/approve/456", url)
	})

	t.Run("handles nil result", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: nil,
		}
		url := extractApprovalURL(resp)
		assert.Empty(t, url)
	})

	t.Run("handles response without URL", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "No URL here",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Empty(t, url)
	})
}
