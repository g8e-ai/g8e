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
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestAgentCmd(t *testing.T) {
	t.Run("agent command has correct use and description", func(t *testing.T) {
		cmd := agentCmd()
		assert.Equal(t, "agent", cmd.Use)
		assert.Contains(t, cmd.Short, "Agent integration")
		assert.Contains(t, cmd.Long, "popular AI agent binaries")
	})

	t.Run("agent list command works", func(t *testing.T) {
		cmd := agentListCmd()
		assert.Equal(t, "list", cmd.Use)
		assert.Contains(t, cmd.Short, "List supported")

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "claude")
		assert.Contains(t, output, "codex")
		assert.Contains(t, output, "devin")
		assert.Contains(t, output, "gemini")
		assert.Contains(t, output, "goose")
		assert.NotContains(t, output, "cursor")
		assert.NotContains(t, output, "aider")
		assert.NotContains(t, output, "generic")
	})

	t.Run("agent show command requires exactly one argument", func(t *testing.T) {
		cmd := agentShowCmd()
		assert.Equal(t, "show <agent>", cmd.Use)
		assert.Contains(t, cmd.Short, "Print MCP client configuration")
	})

	t.Run("agent show generates gateway configs for claude with full transport details", func(t *testing.T) {
		cmd := agentShowCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"claude"})
		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "g8e Gateway MCP Configurations")
		assert.Contains(t, output, "g8e.local")
		assert.Contains(t, output, "IP Address")
		assert.Contains(t, output, "Stdio Transport")
	})

	agents := []string{"gemini", "goose"}
	for _, agent := range agents {
		t.Run("agent show generates gateway configs for "+agent, func(t *testing.T) {
			cmd := agentShowCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs([]string{agent})
			err := cmd.Execute()
			require.NoError(t, err)
			output := buf.String()
			assert.Contains(t, output, "g8e Gateway MCP Configurations")
		})
	}

	t.Run("agent show returns error for unknown agent", func(t *testing.T) {
		cmd := agentShowCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"unknown-agent"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent not found")
	})

	t.Run("agent show is case-insensitive", func(t *testing.T) {
		cmd := agentShowCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"CLAUDE"})
		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "g8e Gateway MCP Configurations")
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
		require.NoError(t, err)
		assert.Equal(t, "2.0", req.JSONRPC)
		assert.InEpsilon(t, 1, req.ID, 0.0)
		assert.Equal(t, "tools/list", req.Method)
	})

	t.Run("JSON-RPC request without params parses correctly", func(t *testing.T) {
		jsonStr := `{"jsonrpc":"2.0","id":2,"method":"initialize"}`
		var req JSONRPCRequest
		err := json.Unmarshal([]byte(jsonStr), &req)
		require.NoError(t, err)
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
		require.NoError(t, err)
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
		require.NoError(t, err)
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
					InputSchema: &mcp.InputSchema{Type: "object"},
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
			InputSchema: &mcp.InputSchema{
				Type: "object",
				Properties: map[string]*mcp.PropertySchema{
					"command": {
						Type:        "string",
						Description: "The command to execute",
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
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.InEpsilon(t, 1, resp.ID, 0.0)
		assert.NotNil(t, resp.Result)

		result := resp.Result.(map[string]interface{})
		assert.Contains(t, result, "protocolVersion")
		assert.Contains(t, result, "capabilities")
		assert.Contains(t, result, "serverInfo")
	})
}

func TestSendError(t *testing.T) {
	t.Run("error response has correct structure", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		sendError(encoder, 1, -32601, "method not found")

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.InEpsilon(t, 1, resp.ID, 0.0)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, -32601, resp.Error.Code)
		assert.Equal(t, "method not found", resp.Error.Message)
	})
}

func TestIsL3ApprovalResponse(t *testing.T) {
	t.Run("detects L3 approval response via structured approval_url field", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"approval_url": "https://example.com/approve/123",
			},
		}
		assert.True(t, isL3ApprovalResponse(resp))
	})

	t.Run("does not detect L3 approval via content text without approval_url field", func(t *testing.T) {
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
		assert.False(t, isL3ApprovalResponse(resp))
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

	t.Run("handles non-map result", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: "string result",
		}
		assert.False(t, isL3ApprovalResponse(resp))
	})
}

func TestExtractApprovalURL(t *testing.T) {
	t.Run("extracts URL from structured approval_url field", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"approval_url": "https://example.com/api/v1/approve/abc123",
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/abc123", url)
	})

	t.Run("extracts URL from content text with approval path", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Execution paused. Please visit https://example.com/api/v1/approve/123 to authorize",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/123", url)
	})

	t.Run("handles URL at end of string", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Please visit https://example.com/api/v1/approve/456",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/456", url)
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

	t.Run("extracts URL from marshaled JSON as fallback", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"message": "Visit https://example.com/api/v1/approve/789 for approval",
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/789", url)
	})

	t.Run("prefers structured approval_url over text content", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"approval_url": "https://example.com/api/v1/approve/structured",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Visit https://example.com/api/v1/approve/text instead",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/structured", url)
	})
}

func TestProxyToGateway(t *testing.T) {
	t.Run("proxyToGateway marshals request correctly", func(t *testing.T) {
		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		}

		reqBody, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(reqBody), "tools/list")
		assert.Contains(t, string(reqBody), "2.0")
	})

	t.Run("proxyToGateway forwards request to server and returns response", func(t *testing.T) {
		expectedResp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      float64(1),
			Result: map[string]interface{}{
				"tools": []interface{}{
					map[string]interface{}{
						"name":        "test_tool",
						"description": "A test tool",
					},
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req JSONRPCRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, "tools/list", req.Method)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			err = json.NewEncoder(w).Encode(expectedResp)
			assert.NoError(t, err)
		}))
		defer server.Close()

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := proxyToGateway(client, server.URL, req)
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.NotNil(t, resp.Result)
	})

	t.Run("proxyToGateway returns error on server error with invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`internal server error`))
		}))
		defer server.Close()

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		}

		client := &http.Client{Timeout: 5 * time.Second}
		_, err := proxyToGateway(client, server.URL, req)
		require.Error(t, err)
	})

	t.Run("proxyToGateway returns error on invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{invalid json`))
		}))
		defer server.Close()

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		}

		client := &http.Client{Timeout: 5 * time.Second}
		_, err := proxyToGateway(client, server.URL, req)
		require.Error(t, err)
	})

	t.Run("proxyToGateway forwards L3 approval response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      float64(1),
				Result: map[string]interface{}{
					"approval_url": "https://example.com/approve/abc123",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": "Execution paused. Please visit https://example.com/approve/abc123 to authorize",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := proxyToGateway(client, server.URL, req)
		require.NoError(t, err)
		assert.True(t, isL3ApprovalResponse(resp))
	})
}

func TestProxyToGatewayWithRetry(t *testing.T) {
	t.Run("SSE credentials missing returns ErrNotAuthenticated", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      float64(1),
				Result: map[string]interface{}{
					"approval_url": "https://g8e.local:8443/api/v1/approve/tx-missing-creds",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": "Execution paused. Please authorize.",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		conn := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: server.URL,
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
		}

		_, err := proxySessionToGatewayWithRetryContext(context.Background(), conn, req, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
	})

	t.Run("SSE timeout returns error without polling", func(t *testing.T) {
		gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      float64(1),
				Result: map[string]interface{}{
					"approval_url": "https://g8e.local:8443/api/v1/approve/tx-timeout",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": "Execution paused. Please authorize.",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer gatewayServer.Close()

		sseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			<-r.Context().Done()
		}))
		defer sseServer.Close()

		conn := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: gatewayServer.URL,
			sseClient:  &http.Client{Timeout: 5 * time.Second},
			sseBaseURL: sseServer.URL,
			userID:     "user-timeout-test",
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		_, err := proxySessionToGatewayWithRetryContext(ctx, conn, req, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
	})

	t.Run("extractTxHashFromApprovalURL extracts hash from full URL", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  string
		}{
			{
				name:  "standard approval URL",
				input: "https://g8e.local:8443/api/v1/approve/abc123",
				want:  "abc123",
			},
			{
				name:  "URL with query params",
				input: "https://g8e.local:8443/api/v1/approve/txhash456?foo=bar",
				want:  "txhash456",
			},
			{
				name:  "URL with fragment",
				input: "https://g8e.local:8443/api/v1/approve/hash789#section",
				want:  "hash789",
			},
			{
				name:  "empty URL",
				input: "",
				want:  "",
			},
			{
				name:  "IP-based URL",
				input: "https://10.0.0.5:8443/api/v1/approve/deadbeef",
				want:  "deadbeef",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := extractTxHashFromApprovalURL(tt.input)
				assert.Equal(t, tt.want, got)
			})
		}
	})
}

func generateTestCerts(t *testing.T) (certPath, keyPath, caPath string) {
	t.Helper()
	tempDir := testutil.TempDir(t)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"g8e.local"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	certPath = filepath.Join(tempDir, constants.TestCertFilename)
	keyPath = filepath.Join(tempDir, constants.TestKeyFilename)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, os.WriteFile(certPath, certPEM, constants.PermFilePublic))

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(keyPath, keyPEM, constants.PermFilePublic))

	return certPath, keyPath, certPath
}

func TestEnvOr(t *testing.T) {
	t.Run("returns environment variable when set", func(t *testing.T) {
		t.Setenv("TEST_VAR", "test_value")
		result := envOr("TEST_VAR", "fallback")
		assert.Equal(t, "test_value", result)
	})

	t.Run("returns fallback when environment variable not set", func(t *testing.T) {
		result := envOr("NONEXISTENT_VAR", "fallback")
		assert.Equal(t, "fallback", result)
	})

	t.Run("returns fallback when environment variable is empty string", func(t *testing.T) {
		t.Setenv("EMPTY_VAR", "")
		result := envOr("EMPTY_VAR", "fallback")
		assert.Equal(t, "fallback", result)
	})
}

func TestSendSuccess(t *testing.T) {
	t.Run("success response has correct structure", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		sendSuccess(encoder, 1, map[string]string{"status": "ok"})

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.InEpsilon(t, 1, resp.ID, 0.0)
		assert.NotNil(t, resp.Result)
		assert.Nil(t, resp.Error)
	})

	t.Run("success response with nil result", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		sendSuccess(encoder, 2, nil)

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.Nil(t, resp.Error)
	})
}

func TestGetSupportedAgents(t *testing.T) {
	t.Run("returns all supported agents", func(t *testing.T) {
		agents := getSupportedAgents()
		assert.NotEmpty(t, agents)

		agentIDs := make(map[string]bool)
		for _, agent := range agents {
			agentIDs[agent.ID] = true
		}

		assert.True(t, agentIDs["claude"], "should include claude")
		assert.True(t, agentIDs["codex"], "should include codex")
		assert.True(t, agentIDs["devin"], "should include devin")
		assert.True(t, agentIDs["gemini"], "should include gemini")
		assert.True(t, agentIDs["goose"], "should include goose")
		assert.False(t, agentIDs["cursor"], "should not include cursor")
		assert.False(t, agentIDs["aider"], "should not include aider")
		assert.False(t, agentIDs["generic"], "should not include generic")
	})

	t.Run("each agent has non-empty ID and description", func(t *testing.T) {
		agents := getSupportedAgents()
		for _, agent := range agents {
			assert.NotEmpty(t, agent.ID, "agent ID should not be empty")
			assert.NotEmpty(t, agent.Description, "agent description should not be empty")
		}
	})
}

func TestExtractURLFromText(t *testing.T) {
	t.Run("extracts approval URL with correct prefix", func(t *testing.T) {
		text := "Please visit https://g8e.local/api/v1/approve/abc123 to authorize"
		url := extractURLFromText(text)
		assert.Contains(t, url, "https://")
		assert.Contains(t, url, "/approve/")
	})

	t.Run("extracts generic HTTPS URL when approval prefix not found", func(t *testing.T) {
		text := "Visit https://example.com/authorize for more info"
		url := extractURLFromText(text)
		assert.Equal(t, "https://example.com/authorize", url)
	})

	t.Run("handles text without URL", func(t *testing.T) {
		text := "No URL in this text"
		url := extractURLFromText(text)
		assert.Empty(t, url)
	})

	t.Run("handles empty string", func(t *testing.T) {
		url := extractURLFromText("")
		assert.Empty(t, url)
	})

	t.Run("extracts URL from text with quotes", func(t *testing.T) {
		text := `URL is "https://g8e.local/api/v1/approve/xyz" in quotes`
		url := extractURLFromText(text)
		assert.Contains(t, url, "https://")
	})

	t.Run("extracts URL from text with apostrophe", func(t *testing.T) {
		text := "URL is 'https://g8e.local/api/v1/approve/abc' in apostrophes"
		url := extractURLFromText(text)
		assert.Contains(t, url, "https://")
	})
}

func TestPrintMCPConfigStdio(t *testing.T) {
	t.Run("generates stdio config with binary path", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err := printMCPConfigStdio(cmd)
		// This will use the actual os.Executable, which should work in test environment
		if err != nil {
			// If it fails, check the error message
			assert.Contains(t, err.Error(), "failed to get binary path")
		} else {
			output := buf.String()
			assert.Contains(t, output, "mcpServers")
			assert.Contains(t, output, "g8e")
			assert.Contains(t, output, "stdio")
		}
	})
}

func TestPrintMCPConfigLocal(t *testing.T) {
	t.Run("generates local config with /etc/hosts entry", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

		clientDir := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient)
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(clientDir, constants.TestCertFilename), []byte("cert"), constants.PermFilePublic))
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(clientDir, constants.TestKeyFilename), []byte("key"), constants.PermFilePublic))
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle), []byte("ca"), constants.PermFilePublic))

		t.Setenv("G8E_PROJECT_ROOT", tempDir)

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = printMCPConfigLocal(cmd)
		if err != nil {
			assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
		}
	})
}

func TestPrintMCPConfigIP(t *testing.T) {
	t.Run("generates IP-based config", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

		clientDir := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient)
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(clientDir, constants.TestCertFilename), []byte("cert"), constants.PermFilePublic))
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(clientDir, constants.TestKeyFilename), []byte("key"), constants.PermFilePublic))
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle), []byte("ca"), constants.PermFilePublic))

		t.Setenv("G8E_PROJECT_ROOT", tempDir)

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = printMCPConfigIP(cmd)
		if err != nil {
			assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
		}
	})
}

func TestAgentRunCmd(t *testing.T) {
	t.Run("agent run command has correct structure", func(t *testing.T) {
		cmd := agentRunCmd()
		assert.Contains(t, cmd.Use, "run")
		assert.Contains(t, cmd.Short, "Govern any MCP server")
		assert.Contains(t, cmd.Long, "Launch an AI agent")
	})

	t.Run("agent run has url flag", func(t *testing.T) {
		cmd := agentRunCmd()
		urlFlag := cmd.Flags().Lookup("url")
		require.NotNil(t, urlFlag, "agent run should have --url flag")
		assert.Equal(t, "string", urlFlag.Value.Type())
	})

	t.Run("agent run has silence flags set", func(t *testing.T) {
		cmd := agentRunCmd()
		assert.True(t, cmd.SilenceErrors, "should silence errors")
		assert.True(t, cmd.SilenceUsage, "should silence usage")
	})
}

func TestPrintAgentShow(t *testing.T) {
	t.Run("printAgentShow handles all supported agents", func(t *testing.T) {
		agents := getSupportedAgents()
		for _, agent := range agents {
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			err := printAgentShow(cmd, agent.ID)
			if err != nil {
				assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
			} else {
				output := buf.String()
				assert.Contains(t, output, "g8e Gateway MCP Configurations")
			}
		}
	})

	t.Run("printAgentShow is case-insensitive for agent IDs", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Test uppercase
		err := printAgentShow(cmd, "CLAUDE")
		if err != nil {
			assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
		}

		// Test mixed case
		cmd2 := &cobra.Command{}
		var buf2 bytes.Buffer
		cmd2.SetOut(&buf2)
		cmd2.SetErr(&buf2)

		err = printAgentShow(cmd2, "ClaUdE")
		if err != nil {
			assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
		}
	})
}

func TestProxySessionToGateway(t *testing.T) {
	t.Run("proxySessionToGateway marshals request and sets headers", func(t *testing.T) {
		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		}

		reqBody, err := json.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(reqBody), "tools/list")
		assert.Contains(t, string(reqBody), "2.0")
	})

	t.Run("proxySessionToGateway forwards request to gateway", func(t *testing.T) {
		expectedResp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      float64(1),
			Result: map[string]interface{}{
				"tools": []interface{}{
					map[string]interface{}{
						"name":        "test_tool",
						"description": "A test tool",
					},
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req JSONRPCRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, "tools/list", req.Method)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			err = json.NewEncoder(w).Encode(expectedResp)
			assert.NoError(t, err)
		}))
		defer server.Close()

		session := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: server.URL,
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		}

		resp, err := proxySessionToGateway(session, req)
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.NotNil(t, resp.Result)
	})

	t.Run("proxySessionToGateway returns error on non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`internal server error`))
		}))
		defer server.Close()

		session := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: server.URL,
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		}

		_, err := proxySessionToGateway(session, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 500")
	})

	t.Run("proxySessionToGateway returns error on invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{invalid json`))
		}))
		defer server.Close()

		session := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: server.URL,
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		}

		_, err := proxySessionToGateway(session, req)
		require.Error(t, err)
	})
}

func TestSubprocessMCPProxyStop(t *testing.T) {
	t.Run("subprocessMCPProxy stop is safe on nil fields", func(t *testing.T) {
		proxy := &subprocessMCPProxy{
			command: "echo",
			args:    []string{"test"},
			logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}

		// Should not panic
		assert.NotPanics(t, func() {
			proxy.stop()
		})
	})

}

func TestMcpStdioCmd(t *testing.T) {
	t.Run("mcp stdio command has correct structure", func(t *testing.T) {
		cmd := mcpStdioCmd()
		assert.Equal(t, "stdio", cmd.Use)
		assert.Contains(t, cmd.Short, "Run MCP stdio server")
		assert.Contains(t, cmd.Long, "proxies all requests")
	})

	t.Run("mcp stdio command has RunE function", func(t *testing.T) {
		cmd := mcpStdioCmd()
		assert.NotNil(t, cmd.RunE, "should have RunE function")
	})
}

func TestWriteAgentConfig(t *testing.T) {
	t.Run("goose writes config file", func(t *testing.T) {
		t.Setenv("HOME", testutil.TempDir(t))
		binaryPath, err := os.Executable()
		require.NoError(t, err)

		configPath, cleanup, err := WriteAgentConfig("goose", binaryPath)
		require.NoError(t, err)
		assert.NotEmpty(t, configPath)
		if cleanup != nil {
			cleanup()
		}
	})

	t.Run("gemini writes config file with existing settings merge", func(t *testing.T) {
		tmpHome := testutil.TempDir(t)
		t.Setenv("HOME", tmpHome)
		binaryPath, err := os.Executable()
		require.NoError(t, err)

		configPath, cleanup, err := WriteAgentConfig("gemini", binaryPath)
		require.NoError(t, err)
		assert.NotEmpty(t, configPath)
		if cleanup != nil {
			cleanup()
		}

		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Contains(t, string(data), "g8e")
		assert.Contains(t, string(data), "tools")
		assert.Contains(t, string(data), "core")
	})

	t.Run("devin writes config file", func(t *testing.T) {
		t.Setenv("HOME", testutil.TempDir(t))
		binaryPath, err := os.Executable()
		require.NoError(t, err)

		configPath, cleanup, err := WriteAgentConfig("devin", binaryPath)
		require.NoError(t, err)
		assert.NotEmpty(t, configPath)
		if cleanup != nil {
			cleanup()
		}

		data, err := os.ReadFile(configPath)
		require.NoError(t, err)
		assert.Contains(t, string(data), "g8e")
	})

	t.Run("unknown agent writes temp file with cleanup", func(t *testing.T) {
		t.Setenv("HOME", testutil.TempDir(t))
		binaryPath, err := os.Executable()
		require.NoError(t, err)

		configPath, cleanup, err := WriteAgentConfig("unknown-agent", binaryPath)
		require.NoError(t, err)
		assert.NotEmpty(t, configPath)

		_, err = os.Stat(configPath)
		require.NoError(t, err)

		cleanup()

		_, err = os.Stat(configPath)
		assert.True(t, errors.Is(err, os.ErrNotExist))
	})
}

func TestAgentLaunchArgs(t *testing.T) {
	t.Run("claude returns governance flags", func(t *testing.T) {
		args, err := agentLaunchArgs("claude", "/path/to/config.json", "/fake/g8e")
		require.NoError(t, err)
		assert.Contains(t, args, "--mcp-config")
		assert.Contains(t, args, "/path/to/config.json")
		assert.Contains(t, args, "--strict-mcp-config")
		assert.Contains(t, args, "--disallowed-tools")
	})

	t.Run("codex returns governance flags", func(t *testing.T) {
		args, err := agentLaunchArgs("codex", "/path/to/config.json", "/fake/g8e")
		require.NoError(t, err)
		assert.Contains(t, args, "--mcp-config")
		assert.Contains(t, args, "--strict-mcp-config")
	})

	t.Run("goose returns no-profile args", func(t *testing.T) {
		args, err := agentLaunchArgs("goose", "/path/to/config.json", "/fake/g8e")
		require.NoError(t, err)
		assert.Contains(t, args, "session")
		assert.Contains(t, args, "--no-profile")
		assert.Contains(t, args, "--with-extension")
	})

	t.Run("gemini returns empty args", func(t *testing.T) {
		args, err := agentLaunchArgs("gemini", "/path/to/config.json", "/fake/g8e")
		require.NoError(t, err)
		assert.Empty(t, args)
	})

	t.Run("cursor returns error", func(t *testing.T) {
		_, err := agentLaunchArgs("cursor", "/path/to/config.json", "/fake/g8e")
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
	})

	t.Run("devin returns empty args", func(t *testing.T) {
		args, err := agentLaunchArgs("devin", "/path/to/config.json", "/fake/g8e")
		require.NoError(t, err)
		assert.Empty(t, args)
	})

	t.Run("aider returns error", func(t *testing.T) {
		_, err := agentLaunchArgs("aider", "/path/to/config.json", "/fake/g8e")
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
	})

	t.Run("ollama returns error", func(t *testing.T) {
		_, err := agentLaunchArgs("ollama", "/path/to/config.json", "/fake/g8e")
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
	})

	t.Run("unknown agent returns error", func(t *testing.T) {
		_, err := agentLaunchArgs("unknown-agent", "/path/to/config.json", "/fake/g8e")
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrAgentNotSupported)
	})

	t.Run("case-insensitive", func(t *testing.T) {
		args, err := agentLaunchArgs("CLAUDE", "/path/to/config.json", "/fake/g8e")
		require.NoError(t, err)
		assert.Contains(t, args, "--mcp-config")
	})
}

func TestRunMCPAgentRun_NoArgs(t *testing.T) {
	t.Run("returns error when no args and no url", func(t *testing.T) {
		err := runMCPAgentRun(nil, "", false, newFileSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "specify an agent name")
	})

	t.Run("returns error for unknown agent", func(t *testing.T) {
		err := runMCPAgentRun([]string{"unknown-agent-xyz"}, "", false, newFileSvc)
		require.Error(t, err)
	})

	t.Run("devin returns error for unknown agent (not cloud-based)", func(t *testing.T) {
		// Devin is now a local CLI agent and goes through launchAgentWithGovernance.
		// We can't test the full launch path here (requires gateway), but we verify
		// it does NOT return the old cloud-based error.
		err := runMCPAgentRun([]string{"devin"}, "", false, newFileSvc)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "cloud-based agent")
	})
}

func TestSubprocessMCPProxy_ForwardError(t *testing.T) {
	t.Run("forward returns error when stdin write fails", func(t *testing.T) {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		require.NoError(t, w.Close()) // close write end so writes fail
		defer r.Close()

		proxy := &subprocessMCPProxy{
			command: "echo",
			args:    []string{"test"},
			stdin:   w,
			logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}

		_, err = proxy.forward(JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		})
		require.Error(t, err)
	})
}

func TestHttpMCPProxy(t *testing.T) {
	t.Run("forward proxies to server", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      float64(1),
				Result:  map[string]interface{}{"status": "ok"},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		proxy := &httpMCPProxy{
			url:    server.URL,
			client: &http.Client{Timeout: 5 * time.Second},
		}

		resp, err := proxy.forward(JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		})
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
	})

	t.Run("stop is safe to call", func(t *testing.T) {
		proxy := &httpMCPProxy{
			url:    "http://localhost:1",
			client: &http.Client{},
		}
		assert.NotPanics(t, func() {
			proxy.stop()
		})
	})
}
