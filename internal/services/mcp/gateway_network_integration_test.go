// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
)

func TestGatewayService_HandleToolsList(t *testing.T) {

	t.Run("native tools when no downstream", func(t *testing.T) {
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		var result ToolsListResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)
		require.NotEmpty(t, result.Tools)
	})

	t.Run("successful POST proxy to downstream", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"test-tool"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		g := newTestGatewayService(t,
			withDownstreamURL("http://localhost:9999"),
			withCircuitBreaker(3, 1*time.Minute),
		)

		// Open the circuit
		for i := 0; i < 5; i++ {
			g.recordFailure()
		}

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "circuit open")
	})

	t.Run("downstream HTTP error", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
	})

	t.Run("downstream connection error", func(t *testing.T) {
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"))

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "failed to query downstream")
	})

	t.Run("proxies to downstream with valid request", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), "tools/list")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("downstream tools returned", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"downstream_tool","description":"A downstream tool","inputSchema":{"type":"object"}}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		var result ToolsListResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)

		toolNames := make(map[string]bool)
		for _, tool := range result.Tools {
			toolNames[tool.Name] = true
		}

		require.Contains(t, toolNames, "downstream_tool", "should include downstream tool")
	})

	t.Run("method not allowed", func(t *testing.T) {
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestGatewayService_HandleResourcesList(t *testing.T) {

	t.Run("empty list when no downstream", func(t *testing.T) {
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		var result ResourcesListResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)
		require.Empty(t, result.Resources)
	})

	t.Run("successful POST proxy to downstream", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"resources":[{"uri":"file:///test.txt","name":"test"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		g := newTestGatewayService(t,
			withDownstreamURL("http://localhost:9999"),
			withCircuitBreaker(3, 1*time.Minute),
		)

		// Open the circuit
		for i := 0; i < 5; i++ {
			g.recordFailure()
		}

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "circuit open")
	})

	t.Run("downstream HTTP error", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
	})

	t.Run("downstream connection error", func(t *testing.T) {
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"))

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "failed to query downstream")
	})

	t.Run("method not allowed", func(t *testing.T) {
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"))

		req := httptest.NewRequest(http.MethodPut, "/mcp", strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestGatewayService_HandlePromptsList(t *testing.T) {

	t.Run("empty list when no downstream", func(t *testing.T) {
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		var result PromptsListResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)
		require.Empty(t, result.Prompts)
	})

	t.Run("proxies to downstream MCP server", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"prompts":[{"name":"test-prompt","description":"A test prompt"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		g := newTestGatewayService(t,
			withDownstreamURL("http://localhost:9999"),
			withCircuitBreaker(3, 1*time.Minute),
		)

		// Open the circuit
		for i := 0; i < 3; i++ {
			g.recordFailure()
		}

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
		require.Contains(t, resp.Error.Message, "circuit open")
	})

	t.Run("downstream HTTP error", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.NotNil(t, resp.Error)
	})

	t.Run("method not allowed", func(t *testing.T) {
		g := newTestGatewayService(t)

		req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestGatewayService_DispatchToDownstream(t *testing.T) {

	t.Run("successful dispatch", func(t *testing.T) {
		// Mock downstream MCP server
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"tool output"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		result, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{"arg":"val"}`), "test-session-id")
		require.NoError(t, err)
		require.Contains(t, result, "tool output")
	})

	t.Run("no downstream configured", func(t *testing.T) {
		g := newTestGatewayService(t, withDownstreamURL(""))

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Contains(t, err.Error(), "no downstream MCP server configured")
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"), withCircuitBreaker(3, 1*time.Minute))

		// Open the circuit
		for i := 0; i < 3; i++ {
			g.recordFailure()
		}

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Contains(t, err.Error(), "circuit open")
	})

	t.Run("HTTP connection failure", func(t *testing.T) {
		g := newTestGatewayService(t, withDownstreamURL("http://localhost:9999"), withCircuitBreaker(5, 1*time.Minute))

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("non-200 status code", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("MCP error response", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		_, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.Error(t, err)
		require.Contains(t, err.Error(), "MCP error")
	})

	t.Run("downstream request has proper MCP tools/call envelope", func(t *testing.T) {
		var capturedBody []byte
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL))

		_, err := g.DispatchToDownstream(context.Background(), "my-tool", json.RawMessage(`{"key":"value"}`), "test-session-id")
		require.NoError(t, err)

		var req response.JSONRPCRequest
		require.NoError(t, json.Unmarshal(capturedBody, &req), "downstream request should be valid JSON-RPC")
		assert.Equal(t, "2.0", req.JSONRPC)
		assert.Equal(t, "tools/call", req.Method)

		var params map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(req.Params, &params), "params should be a JSON object")
		assert.Contains(t, params, "name", "params must contain 'name' field")
		assert.Contains(t, params, "arguments", "params must contain 'arguments' field")

		var name string
		require.NoError(t, json.Unmarshal(params["name"], &name))
		assert.Equal(t, "my-tool", name, "params.name should match the requested tool name")

		var arguments json.RawMessage
		require.NoError(t, json.Unmarshal(params["arguments"], &arguments))
		assert.JSONEq(t, `{"key":"value"}`, string(arguments), "params.arguments should contain the original tool args")
	})

}

func TestGatewayService_DispatchToDownstream_Scrubbing(t *testing.T) {

	scrubSvc, err := scrubbing.NewScrubbingService(context.Background(), scrubbing.DefaultConfig(), slog.Default(), nil)
	require.NoError(t, err)

	t.Run("SSN in downstream response is scrubbed", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"Record found: SSN 123-45-6789 for John Doe"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL), withScrubbingService(scrubSvc))

		result, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.NoError(t, err)
		assert.NotContains(t, result, "123-45-6789")
		assert.Contains(t, result, "[PII]")
	})

	t.Run("API key in downstream response is scrubbed", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"Config loaded with key g8e_prod_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL), withScrubbingService(scrubSvc))

		result, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.NoError(t, err)
		assert.NotContains(t, result, "g8e_prod_")
		assert.Contains(t, result, "[REDACTED_API_KEY]")
	})

	t.Run("clean downstream response passes through", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"Operation completed successfully"}]}}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withDownstreamURL(downstream.URL), withScrubbingService(scrubSvc))

		result, err := g.DispatchToDownstream(context.Background(), "test-tool", json.RawMessage(`{}`), "test-session-id")
		require.NoError(t, err)
		assert.Contains(t, result, "Operation completed successfully")
	})
}

func TestGatewayService_DispatchToA2ADownstream(t *testing.T) {

	t.Run("successful dispatch with result", func(t *testing.T) {
		// Mock downstream A2A server
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result":"skill output"}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		result, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{"arg":"val"}`))
		require.NoError(t, err)
		require.Equal(t, "skill output", result)
	})

	t.Run("successful dispatch with summary", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"summary":"skill summary"}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		result, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.NoError(t, err)
		require.Equal(t, "skill summary", result)
	})

	t.Run("successful dispatch with empty response", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		result, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.NoError(t, err)
		require.Equal(t, "completed", result)
	})

	t.Run("no downstream configured", func(t *testing.T) {
		g := newTestGatewayService(t, withA2ADownstreamURL(""))

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "no downstream A2A server configured")
	})

	t.Run("circuit breaker open", func(t *testing.T) {
		g := newTestGatewayService(t, withA2ADownstreamURL("http://localhost:9999"), withCircuitBreaker(3, 1*time.Minute))

		// Open the circuit
		for i := 0; i < 3; i++ {
			g.recordFailure()
		}

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "circuit open")
	})

	t.Run("HTTP connection failure", func(t *testing.T) {
		g := newTestGatewayService(t, withA2ADownstreamURL("http://localhost:9999"), withCircuitBreaker(5, 1*time.Minute))

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("non-200 status code", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Error(t, err)
	})

	t.Run("A2A error response", func(t *testing.T) {
		downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"error":"skill execution failed"}`))
		}))
		defer downstream.Close()

		g := newTestGatewayService(t, withA2ADownstreamURL(downstream.URL), withCircuitBreaker(5, 1*time.Minute))

		_, err := g.DispatchToA2ADownstream(context.Background(), "test-skill", json.RawMessage(`{}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "A2A error")
	})

}
