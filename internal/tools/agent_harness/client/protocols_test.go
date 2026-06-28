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

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g8e-ai/g8e/internal/tools/agent_harness/config"
)

func TestMCPToolsList(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		wantErr      bool
	}{
		{
			name:         "successful list",
			responseBody: `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"tool1"},{"name":"tool2"}]}}`,
			wantErr:      false,
		},
		{
			name:         "empty tools list",
			responseBody: `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`,
			wantErr:      false,
		},
		{
			name:         "server error",
			responseBody: `{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid Request"}}`,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/mcp" {
					t.Errorf("expected path /mcp, got %s", r.URL.Path)
				}

				var req JSONRPCRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}

				if req.Method != "tools/list" {
					t.Errorf("expected method tools/list, got %s", req.Method)
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tt.responseBody))
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{Insecure: true},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			p := Persona{ID: "test-client"}

			resp, err := client.MCPToolsList(ctx, p)

			if (err != nil) != tt.wantErr {
				t.Errorf("MCPToolsList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && resp == nil {
				t.Error("MCPToolsList() returned nil response")
			}
		})
	}
}

func TestMCPToolsCall(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		args    map[string]any
		wantErr bool
	}{
		{
			name:    "successful call",
			tool:    "test-tool",
			args:    map[string]any{"arg1": "value1"},
			wantErr: false,
		},
		{
			name:    "call with no args",
			tool:    "test-tool",
			args:    map[string]any{},
			wantErr: false,
		},
		{
			name:    "call with nil args",
			tool:    "test-tool",
			args:    nil,
			wantErr: false,
		},
		{
			name:    "call with complex args",
			tool:    "complex-tool",
			args:    map[string]any{"nested": map[string]any{"key": "value"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/mcp" {
					t.Errorf("expected path /mcp, got %s", r.URL.Path)
				}

				var req JSONRPCRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}

				if req.Method != "tools/call" {
					t.Errorf("expected method tools/call, got %s", req.Method)
				}

				params, ok := req.Params.(map[string]any)
				if !ok {
					t.Error("params should be a map")
				}

				if params["name"] != tt.tool {
					t.Errorf("expected tool name %s, got %v", tt.tool, params["name"])
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"ok"}}`))
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{Insecure: true},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			p := Persona{ID: "test-client"}

			resp, err := client.MCPToolsCall(ctx, p, tt.tool, tt.args)

			if (err != nil) != tt.wantErr {
				t.Errorf("MCPToolsCall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && resp == nil {
				t.Error("MCPToolsCall() returned nil response")
			}
		})
	}
}

func TestMCPResourcesList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/mcp" {
			t.Errorf("expected path /mcp, got %s", r.URL.Path)
		}

		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.Method != "resources/list" {
			t.Errorf("expected method resources/list, got %s", req.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"resources":[{"uri":"res://1"}]}}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{Insecure: true},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	resp, err := client.MCPResourcesList(ctx, p)

	if err != nil {
		t.Errorf("MCPResourcesList() error = %v", err)
	}

	if resp == nil {
		t.Error("MCPResourcesList() returned nil response")
	}
}

func TestMCPResourcesRead(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/mcp" {
			t.Errorf("expected path /mcp, got %s", r.URL.Path)
		}

		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.Method != "resources/read" {
			t.Errorf("expected method resources/read, got %s", req.Method)
		}

		params, ok := req.Params.(map[string]any)
		if !ok {
			t.Error("params should be a map")
		}

		if params["uri"] != "res://test" {
			t.Errorf("expected uri res://test, got %v", params["uri"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"contents":"test content"}}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{Insecure: true},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	resp, err := client.MCPResourcesRead(ctx, p, "res://test")

	if err != nil {
		t.Errorf("MCPResourcesRead() error = %v", err)
	}

	if resp == nil {
		t.Error("MCPResourcesRead() returned nil response")
	}
}

func TestMCPPromptsList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/mcp" {
			t.Errorf("expected path /mcp, got %s", r.URL.Path)
		}

		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.Method != "prompts/list" {
			t.Errorf("expected method prompts/list, got %s", req.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"prompts":[{"name":"prompt1"}]}}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{Insecure: true},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	resp, err := client.MCPPromptsList(ctx, p)

	if err != nil {
		t.Errorf("MCPPromptsList() error = %v", err)
	}

	if resp == nil {
		t.Error("MCPPromptsList() returned nil response")
	}
}

func TestMCPPromptsGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/mcp" {
			t.Errorf("expected path /mcp, got %s", r.URL.Path)
		}

		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.Method != "prompts/get" {
			t.Errorf("expected method prompts/get, got %s", req.Method)
		}

		params, ok := req.Params.(map[string]any)
		if !ok {
			t.Error("params should be a map")
		}

		if params["name"] != "test-prompt" {
			t.Errorf("expected name test-prompt, got %v", params["name"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"messages":[{"role":"user","content":"test"}]}}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{Insecure: true},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	resp, err := client.MCPPromptsGet(ctx, p, "test-prompt", map[string]any{"arg": "value"})

	if err != nil {
		t.Errorf("MCPPromptsGet() error = %v", err)
	}

	if resp == nil {
		t.Error("MCPPromptsGet() returned nil response")
	}
}

func TestA2ACall(t *testing.T) {
	tests := []struct {
		name    string
		skill   string
		payload map[string]any
		execID  string
		wantErr bool
	}{
		{
			name:    "successful call",
			skill:   "test-skill",
			payload: map[string]any{"input": "data"},
			execID:  "exec-123",
			wantErr: false,
		},
		{
			name:    "call with empty payload",
			skill:   "test-skill",
			payload: map[string]any{},
			execID:  "exec-456",
			wantErr: false,
		},
		{
			name:    "call with nil payload",
			skill:   "test-skill",
			payload: nil,
			execID:  "exec-789",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/api/a2a/v1/call" {
					t.Errorf("expected path /api/a2a/v1/call, got %s", r.URL.Path)
				}

				var req JSONRPCRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}

				if req.Method != "a2a/call" {
					t.Errorf("expected method a2a/call, got %s", req.Method)
				}

				params, ok := req.Params.(map[string]any)
				if !ok {
					t.Error("params should be a map")
				}

				if params["skill_name"] != tt.skill {
					t.Errorf("expected skill_name %s, got %v", tt.skill, params["skill_name"])
				}

				if params["execution_id"] != tt.execID {
					t.Errorf("expected execution_id %s, got %v", tt.execID, params["execution_id"])
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"completed"}}`))
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{Insecure: true},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			p := Persona{ID: "test-client"}

			resp, err := client.A2ACall(ctx, p, tt.skill, tt.payload, tt.execID)

			if (err != nil) != tt.wantErr {
				t.Errorf("A2ACall() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && resp == nil {
				t.Error("A2ACall() returned nil response")
			}
		})
	}
}

func TestA2ACallProto(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/a2a/v1/call" {
			t.Errorf("expected path /api/a2a/v1/call, got %s", r.URL.Path)
		}

		var req JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if req.Method != "a2a/call" {
			t.Errorf("expected method a2a/call, got %s", req.Method)
		}

		params, ok := req.Params.(map[string]any)
		if !ok {
			t.Error("params should be a map")
		}

		if params["skill_name"] != "test-skill" {
			t.Errorf("expected skill_name test-skill, got %v", params["skill_name"])
		}

		if params["encoding"] != "protobuf" {
			t.Errorf("expected encoding protobuf, got %v", params["encoding"])
		}

		if params["content_type"] != "application/x-protobuf;type=g8e.operator.v1.A2ACallRequested" {
			t.Errorf("unexpected content_type: %v", params["content_type"])
		}

		payloadB64, ok := params["payload_b64"].(string)
		if !ok {
			t.Error("payload_b64 should be a string")
		}

		_, err := base64.StdEncoding.DecodeString(payloadB64)
		if err != nil {
			t.Errorf("payload_b64 is not valid base64: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"completed"}}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{Insecure: true},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	resp, err := client.A2ACallProto(ctx, p, "test-skill", `{"input":"data"}`, "exec-123")

	if err != nil {
		t.Errorf("A2ACallProto() error = %v", err)
	}

	if resp == nil {
		t.Error("A2ACallProto() returned nil response")
	}
}

func TestSuspended(t *testing.T) {
	tests := []struct {
		name          string
		resp          *JSONRPCResponse
		wantHash      string
		wantSuspended bool
	}{
		{
			name:          "nil response",
			resp:          nil,
			wantHash:      "",
			wantSuspended: false,
		},
		{
			name:          "no error, no result",
			resp:          &JSONRPCResponse{},
			wantHash:      "",
			wantSuspended: false,
		},
		{
			name: "suspended in error message",
			resp: &JSONRPCResponse{
				Error: &JSONRPCError{
					Message: "Transaction suspended. Approve at https://example.com/approve/abc123",
				},
			},
			wantHash:      "abc123",
			wantSuspended: true,
		},
		{
			name: "suspended in error data",
			resp: &JSONRPCResponse{
				Error: &JSONRPCError{
					Message: "Error",
					Data:    json.RawMessage(`{"approval_url":"https://example.com/approve/def456"}`),
				},
			},
			wantHash:      "def456",
			wantSuspended: true,
		},
		{
			name: "not suspended - no approval URL",
			resp: &JSONRPCResponse{
				Error: &JSONRPCError{
					Message: "Some other error",
				},
			},
			wantHash:      "",
			wantSuspended: false,
		},
		{
			name: "suspended with mixed case hash",
			resp: &JSONRPCResponse{
				Error: &JSONRPCError{
					Message: "Approve at https://example.com/approve/AbC123F",
				},
			},
			wantHash:      "AbC123F",
			wantSuspended: true,
		},
		{
			name: "suspended with numeric hash",
			resp: &JSONRPCResponse{
				Error: &JSONRPCError{
					Message: "Approve at https://example.com/approve/1234567890",
				},
			},
			wantHash:      "1234567890",
			wantSuspended: true,
		},
		{
			name: "suspended with truncated hash (non-hex char stops extraction)",
			resp: &JSONRPCResponse{
				Error: &JSONRPCError{
					Message: "Approve at https://example.com/approve/abc123xyz",
				},
			},
			wantHash:      "abc123",
			wantSuspended: true,
		},
		{
			name: "empty hash after marker",
			resp: &JSONRPCResponse{
				Error: &JSONRPCError{
					Message: "Approve at https://example.com/approve/",
				},
			},
			wantHash:      "",
			wantSuspended: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, suspended := Suspended(tt.resp)

			if hash != tt.wantHash {
				t.Errorf("Suspended() hash = %s, want %s", hash, tt.wantHash)
			}
			if suspended != tt.wantSuspended {
				t.Errorf("Suspended() suspended = %v, want %v", suspended, tt.wantSuspended)
			}
		})
	}
}

func TestExtractHashFromURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "valid approval URL",
			url:  "https://example.com/approve/abc123",
			want: "abc123",
		},
		{
			name: "approval URL with port",
			url:  "https://example.com:8443/approve/def456",
			want: "def456",
		},
		{
			name: "approval URL with query params",
			url:  "https://example.com/approve/abc123def?param=value",
			want: "abc123def",
		},
		{
			name: "approval URL with fragment",
			url:  "https://example.com/approve/abc123#section",
			want: "abc123",
		},
		{
			name: "no approval marker",
			url:  "https://example.com/some/other/path",
			want: "",
		},
		{
			name: "empty hash after marker",
			url:  "https://example.com/approve/",
			want: "",
		},
		{
			name: "mixed case hash",
			url:  "https://example.com/approve/AbC123F",
			want: "AbC123F",
		},
		{
			name: "numeric hash",
			url:  "https://example.com/approve/1234567890",
			want: "1234567890",
		},
		{
			name: "truncated hash at non-hex char",
			url:  "https://example.com/approve/abc123xyz",
			want: "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHashFromURL(tt.url)
			if got != tt.want {
				t.Errorf("extractHashFromURL() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		name string
		s    string
		sub  string
		want int
	}{
		{
			name: "substring found at start",
			s:    "hello world",
			sub:  "hello",
			want: 0,
		},
		{
			name: "substring found in middle",
			s:    "hello world",
			sub:  "lo wo",
			want: 3,
		},
		{
			name: "substring found at end",
			s:    "hello world",
			sub:  "world",
			want: 6,
		},
		{
			name: "substring not found",
			s:    "hello world",
			sub:  "xyz",
			want: -1,
		},
		{
			name: "empty substring",
			s:    "hello",
			sub:  "",
			want: 0,
		},
		{
			name: "empty string",
			s:    "",
			sub:  "test",
			want: -1,
		},
		{
			name: "both empty",
			s:    "",
			sub:  "",
			want: 0,
		},
		{
			name: "substring longer than string",
			s:    "hi",
			sub:  "hello",
			want: -1,
		},
		{
			name: "case sensitive",
			s:    "Hello World",
			sub:  "hello",
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indexOf(tt.s, tt.sub)
			if got != tt.want {
				t.Errorf("indexOf() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsHex(t *testing.T) {
	tests := []struct {
		name string
		b    byte
		want bool
	}{
		{
			name: "lowercase a-f",
			b:    'a',
			want: true,
		},
		{
			name: "lowercase f",
			b:    'f',
			want: true,
		},
		{
			name: "uppercase A-F",
			b:    'A',
			want: true,
		},
		{
			name: "uppercase F",
			b:    'F',
			want: true,
		},
		{
			name: "digit 0-9",
			b:    '5',
			want: true,
		},
		{
			name: "digit 0",
			b:    '0',
			want: true,
		},
		{
			name: "digit 9",
			b:    '9',
			want: true,
		},
		{
			name: "non-hex lowercase",
			b:    'g',
			want: false,
		},
		{
			name: "non-hex uppercase",
			b:    'G',
			want: false,
		},
		{
			name: "special character",
			b:    '#',
			want: false,
		},
		{
			name: "space",
			b:    ' ',
			want: false,
		},
		{
			name: "null byte",
			b:    0,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHex(tt.b)
			if got != tt.want {
				t.Errorf("isHex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJSONRPCRequest(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test/method",
		Params:  map[string]any{"key": "value"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded JSONRPCRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.JSONRPC != "2.0" {
		t.Error("JSONRPC mismatch")
	}
	if decoded.ID != 1 {
		t.Error("ID mismatch")
	}
	if decoded.Method != "test/method" {
		t.Error("Method mismatch")
	}
}

func TestJSONRPCResponse(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"status":"ok"}`),
		Error:   nil,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded JSONRPCResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.JSONRPC != "2.0" {
		t.Error("JSONRPC mismatch")
	}
	if decoded.ID != 1 {
		t.Error("ID mismatch")
	}
	if decoded.Error != nil {
		t.Error("Error should be nil")
	}
}

func TestJSONRPCError(t *testing.T) {
	err := JSONRPCError{
		Code:    -32600,
		Message: "Invalid Request",
		Data:    json.RawMessage(`{"details":"more info"}`),
	}

	data, errData := json.Marshal(err)
	if errData != nil {
		t.Fatalf("failed to marshal: %v", errData)
	}

	var decoded JSONRPCError
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Code != -32600 {
		t.Error("Code mismatch")
	}
	if decoded.Message != "Invalid Request" {
		t.Error("Message mismatch")
	}
}

func TestRPC(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
		wantErr      bool
	}{
		{
			name:         "successful RPC",
			responseBody: `{"jsonrpc":"2.0","id":1,"result":{"status":"ok"}}`,
			wantErr:      false,
		},
		{
			name:         "RPC error",
			responseBody: `{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid Request"}}`,
			wantErr:      false,
		},
		{
			name:         "invalid JSON response",
			responseBody: `invalid json`,
			wantErr:      false, // rpc() is lenient
		},
		{
			name:         "empty response",
			responseBody: ``,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tt.responseBody))
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{Insecure: true},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			p := Persona{ID: "test-client"}

			resp, err := client.rpc(ctx, p, "/test", "test.method", map[string]any{})

			if (err != nil) != tt.wantErr {
				t.Errorf("rpc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && resp == nil {
				t.Error("rpc() returned nil response")
			}
		})
	}
}
