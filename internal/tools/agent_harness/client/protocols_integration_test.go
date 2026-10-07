// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestDispatchCommand_UsesTypedAuthenticatedIngressAndRecordsExchange(t *testing.T) {
	var received DispatchCommandRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, constants.APIPaths.OperatorsCommands, r.URL.Path)
		assert.Equal(t, "cli-1", r.Header.Get(constants.HeaderCLISessionID))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"success":true,"transaction_id":"tx-1","action_type":"FILE_EDIT","result_payload":"cmVzdWx0"}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)
	exchanges := []Exchange{}
	client.Record(&exchanges)
	request := DispatchCommandRequest{
		TargetOperatorSessionID: "session-1",
		ActionType:              "FILE_EDIT",
		Payload:                 []byte("payload"),
		CaseID:                  "run-1",
		InvestigationID:         "scenario-1",
		TaskID:                  "attempt-1",
		CLISessionID:            "cli-1",
	}
	status, response, raw, err := client.DispatchCommand(context.Background(), Persona{ID: "evaluation", CLISessionID: "cli-1"}, request)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "session-1", received.TargetOperatorSessionID)
	assert.Equal(t, "FILE_EDIT", received.ActionType)
	assert.Equal(t, string(constants.EventOperatorFileEditRequested), received.EventType)
	assert.Equal(t, "tx-1", response.TransactionID)
	assert.Equal(t, []byte("result"), response.ResultPayload)
	assert.JSONEq(t, `{"success":true,"transaction_id":"tx-1","action_type":"FILE_EDIT","result_payload":"cmVzdWx0"}`, string(raw))
	require.Len(t, exchanges, 1)
	assert.Equal(t, constants.APIPaths.OperatorsCommands, exchanges[0].URL[len(server.URL):])
}

func TestDispatchCommand_ResolvesEventTypeForGovernedActions(t *testing.T) {
	tests := []struct {
		name          string
		actionType    string
		eventType     string
		wantEventType string
		wantAction    string
	}{
		{
			name:          "residency action derives event type",
			actionType:    string(constants.ActionTypeOllamaModelResidency),
			wantEventType: string(constants.EventOperatorOllamaModelResidencyRequested),
			wantAction:    string(constants.ActionTypeOllamaModelResidency),
		},
		{
			name:          "inventory action derives event type",
			actionType:    string(constants.ActionTypeOllamaModelInventory),
			wantEventType: string(constants.EventOperatorOllamaModelInventoryRequested),
			wantAction:    string(constants.ActionTypeOllamaModelInventory),
		},
		{
			name:          "execute bash action derives event type",
			actionType:    string(constants.ActionTypeExecuteBash),
			wantEventType: string(constants.EventOperatorCommandRequested),
			wantAction:    string(constants.ActionTypeExecuteBash),
		},
		{
			name:          "event type derives action type",
			eventType:     string(constants.EventOperatorOllamaModelResidencyRequested),
			wantEventType: string(constants.EventOperatorOllamaModelResidencyRequested),
			wantAction:    string(constants.ActionTypeOllamaModelResidency),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var received DispatchCommandRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(`{"success":true,"transaction_id":"tx-1"}`))
				require.NoError(t, err)
			}))
			t.Cleanup(server.Close)

			client, err := New(config.Config{MTLSBaseURL: server.URL})
			require.NoError(t, err)

			request := DispatchCommandRequest{
				TargetOperatorSessionID: "session-1",
				ActionType:              tt.actionType,
				EventType:               tt.eventType,
				Payload:                 []byte("payload"),
				CLISessionID:            "cli-1",
			}
			status, _, _, err := client.DispatchCommand(context.Background(), Persona{ID: "evaluation", CLISessionID: "cli-1"}, request)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, status)
			assert.Equal(t, tt.wantEventType, received.EventType)
			assert.Equal(t, tt.wantAction, received.ActionType)
		})
	}
}

func TestHealth_ReturnsTypedPostureAndRecordsExchange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, constants.APIPaths.Health, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"status":"ok","mode":"gateway","version":"2.1.8","pid":1,"governance_ready":true,"posture":"doctrine"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)
	exchanges := []Exchange{}
	client.Record(&exchanges)

	health, raw, err := client.Health(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "doctrine", health.Posture)
	assert.True(t, health.GovernanceReady)
	assert.JSONEq(t, `{"status":"ok","mode":"gateway","version":"2.1.8","pid":1,"governance_ready":true,"posture":"doctrine"}`, string(raw))
	require.Len(t, exchanges, 1)
	assert.Equal(t, constants.APIPaths.Health, exchanges[0].URL[len(server.URL):])
}

func TestAuditReceiptRecords_ReturnsCanonicalTypedRecords(t *testing.T) {
	expected := &models.ActionReceiptRecord{
		TransactionID: "tx-1", OperatorID: "operator-1", OperatorSessionID: "session-1",
		ActionReceipt: &operatorv1.ActionReceipt{TransactionId: "tx-1"},
	}
	body, err := json.Marshal(models.AuditReceiptsResponse{Success: true, Receipts: []*models.ActionReceiptRecord{expected}})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, constants.APIPaths.AuditReceipts, r.URL.Path)
		assert.Equal(t, "session-1", r.URL.Query().Get("operator_session_id"))
		assert.Equal(t, "cli-1", r.Header.Get(constants.HeaderCLISessionID))
		w.Header().Set("Content-Type", "application/json")
		_, writeErr := w.Write(body)
		require.NoError(t, writeErr)
	}))
	t.Cleanup(server.Close)
	client, err := New(config.Config{MTLSBaseURL: server.URL, CLISessionID: "cli-1"})
	require.NoError(t, err)

	records, raw, err := client.AuditReceiptRecords(context.Background(), "session-1")

	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, expected.TransactionID, records[0].TransactionID)
	assert.Equal(t, expected.ActionReceipt.TransactionId, records[0].ActionReceipt.TransactionId)
	assert.Equal(t, body, raw)
}

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
				Auth:        config.Auth{},
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

func TestMCPToolsCall_TypedArgs_SerializeToSameJSONRPCShape(t *testing.T) {
	var capturedParams map[string]any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Errorf("expected path /mcp, got %s", r.URL.Path)
		}
		var req JSONRPCRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if req.Method != "tools/call" {
			t.Errorf("expected method tools/call, got %s", req.Method)
		}
		capturedParams, _ = req.Params.(map[string]any)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"ok"}}`))
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	cfg := config.Config{MTLSBaseURL: srv.URL, Auth: config.Auth{}}
	c, err := New(cfg)
	require.NoError(t, err)

	args := ShellCommandArgs{Command: "dataop", Args: []string{"ingest", "TRK-001"}, Timeout: 10}
	_, err = c.MCPToolsCall(context.Background(), Persona{ID: "test"}, "run_shell_command", args)
	require.NoError(t, err)

	require.NotNil(t, capturedParams, "params should be a map")
	assert.Equal(t, "run_shell_command", capturedParams["name"])

	argsMap, ok := capturedParams["arguments"].(map[string]any)
	require.True(t, ok, "arguments should be a JSON object")
	assert.Equal(t, "dataop", argsMap["command"])
	assert.Equal(t, []any{"ingest", "TRK-001"}, argsMap["args"])
	assert.Equal(t, float64(10), argsMap["timeout"])
}

func TestMCPToolsCall(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		args    ToolArgs
		wantErr bool
	}{
		{
			name:    "successful call",
			tool:    "fs_list",
			args:    FSPathArgs{Path: "."},
			wantErr: false,
		},
		{
			name:    "call with no args",
			tool:    "fs_list",
			args:    FSPathArgs{},
			wantErr: false,
		},
		{
			name:    "call with nil args",
			tool:    "fs_list",
			args:    nil,
			wantErr: false,
		},
		{
			name:    "call with multi-field args",
			tool:    "fs_grep",
			args:    FSGrepArgs{Path: ".", Pattern: "TODO"},
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
				Auth:        config.Auth{},
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
		Auth:        config.Auth{},
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
		Auth:        config.Auth{},
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
		Auth:        config.Auth{},
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
		Auth:        config.Auth{},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	resp, err := client.MCPPromptsGet(ctx, p, "test-prompt", promptArgFixture{Arg: "value"})

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
		payload SkillPayload
		execID  string
		wantErr bool
	}{
		{
			name:    "successful call",
			skill:   "test-skill",
			payload: skillPayloadFixture{Input: "data"},
			execID:  "exec-123",
			wantErr: false,
		},
		{
			name:    "call with empty payload",
			skill:   "test-skill",
			payload: skillPayloadFixture{},
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
				Auth:        config.Auth{},
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
		Auth:        config.Auth{},
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
				Auth:        config.Auth{},
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
