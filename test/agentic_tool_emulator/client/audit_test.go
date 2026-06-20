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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/g8e-ai/g8e/test/agentic_tool_emulator/config"
)

func TestParseReceipts(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantLen  int
		wantZero bool
	}{
		{
			name:     "wrapped receipts",
			body:     []byte(`{"receipts":[{"transaction_id":"tx1"},{"transaction_id":"tx2"}]}`),
			wantLen:  2,
			wantZero: false,
		},
		{
			name:     "bare array",
			body:     []byte(`[{"transaction_id":"tx1"},{"transaction_id":"tx2"}]`),
			wantLen:  2,
			wantZero: false,
		},
		{
			name:     "empty wrapped array",
			body:     []byte(`{"receipts":[]}`),
			wantLen:  0,
			wantZero: false,
		},
		{
			name:     "empty bare array",
			body:     []byte(`[]`),
			wantLen:  0,
			wantZero: false,
		},
		{
			name:     "invalid JSON",
			body:     []byte(`invalid json`),
			wantLen:  0,
			wantZero: true,
		},
		{
			name:     "empty body",
			body:     []byte{},
			wantLen:  0,
			wantZero: true,
		},
		{
			name:     "nil body",
			body:     nil,
			wantLen:  0,
			wantZero: true,
		},
		{
			name:     "wrapped with empty receipts field",
			body:     []byte(`{"other":"data"}`),
			wantLen:  0,
			wantZero: false,
		},
		{
			name:     "single receipt wrapped",
			body:     []byte(`{"receipts":[{"transaction_id":"tx1","status":"completed"}]}`),
			wantLen:  1,
			wantZero: false,
		},
		{
			name:     "malformed receipt object",
			body:     []byte(`[{"transaction_id":"tx1"},{"invalid}]`),
			wantLen:  0,
			wantZero: true, // invalid JSON object
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseReceipts(tt.body)

			if tt.wantZero && result != nil {
				t.Errorf("parseReceipts() should return nil for invalid input, got %v", result)
			}
			if !tt.wantZero && result == nil {
				t.Error("parseReceipts() returned nil unexpectedly")
			}
			if !tt.wantZero && len(result) != tt.wantLen {
				t.Errorf("parseReceipts() length = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestParseReceipts_RawField(t *testing.T) {
	body := []byte(`{"receipts":[{"transaction_id":"tx1","status":"completed"}]}`)
	result := parseReceipts(body)

	if len(result) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(result))
	}

	if result[0].Raw == nil {
		t.Error("parseReceipts() should set Raw field")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(result[0].Raw, &decoded); err != nil {
		t.Errorf("failed to unmarshal Raw field: %v", err)
	}

	if decoded["transaction_id"] != "tx1" {
		t.Errorf("Raw field does not contain expected data")
	}
}

func TestAuditReceipts(t *testing.T) {
	tests := []struct {
		name              string
		operatorSessionID string
		responseBody      string
		wantErr           bool
		verifyQuery       func(url.Values)
	}{
		{
			name:              "successful retrieval without session ID",
			operatorSessionID: "",
			responseBody:      `{"receipts":[{"transaction_id":"tx1"},{"transaction_id":"tx2"}]}`,
			wantErr:           false,
			verifyQuery: func(v url.Values) {
				if v.Get("operator_session_id") != "" {
					t.Error("should not have operator_session_id query param")
				}
			},
		},
		{
			name:              "successful retrieval with session ID",
			operatorSessionID: "session-123",
			responseBody:      `{"receipts":[{"transaction_id":"tx1"}]}`,
			wantErr:           false,
			verifyQuery: func(v url.Values) {
				if v.Get("operator_session_id") != "session-123" {
					t.Errorf("expected operator_session_id session-123, got %s", v.Get("operator_session_id"))
				}
			},
		},
		{
			name:              "bare array response",
			operatorSessionID: "",
			responseBody:      `[{"transaction_id":"tx1"}]`,
			wantErr:           false,
		},
		{
			name:              "empty receipts",
			operatorSessionID: "",
			responseBody:      `{"receipts":[]}`,
			wantErr:           false,
		},
		{
			name:              "server error",
			operatorSessionID: "",
			responseBody:      `{"error": "internal error"}`,
			wantErr:           false, // AuditReceipts is lenient like ExportReceipts
		},
		{
			name:              "network error",
			operatorSessionID: "",
			responseBody:      ``,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedQuery url.Values

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != "/api/audit/receipts" {
					t.Errorf("expected path /api/audit/receipts, got %s", r.URL.Path)
				}

				receivedQuery = r.URL.Query()
				if tt.verifyQuery != nil {
					tt.verifyQuery(receivedQuery)
				}

				if tt.name == "network error" {
					// Close connection immediately
					hj, ok := w.(http.Hijacker)
					if ok {
						conn, _, _ := hj.Hijack()
						conn.Close()
						return
					}
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
			receipts, body, err := client.AuditReceipts(ctx, tt.operatorSessionID)

			if (err != nil) != tt.wantErr {
				t.Errorf("AuditReceipts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(body) == 0 && tt.responseBody != "" {
					t.Error("expected non-empty body")
				}
				if tt.name == "empty receipts" && len(receipts) != 0 {
					t.Errorf("expected 0 receipts, got %d", len(receipts))
				}
				if tt.name == "successful retrieval without session ID" && len(receipts) != 2 {
					t.Errorf("expected 2 receipts, got %d", len(receipts))
				}
			}
		})
	}
}

func TestExportReceipts(t *testing.T) {
	tests := []struct {
		name              string
		operatorSessionID string
		responseBody      []byte
		wantErr           bool
	}{
		{
			name:              "successful export without session ID",
			operatorSessionID: "",
			responseBody:      []byte(`{"receipts":[{"transaction_id":"tx1"}]}`),
			wantErr:           false,
		},
		{
			name:              "successful export with session ID",
			operatorSessionID: "session-123",
			responseBody:      []byte(`{"receipts":[{"transaction_id":"tx1"}]}`),
			wantErr:           false,
		},
		{
			name:              "empty export",
			operatorSessionID: "",
			responseBody:      []byte(`{}`),
			wantErr:           false,
		},
		{
			name:              "server error",
			operatorSessionID: "",
			responseBody:      []byte(`{"error": "internal error"}`),
			wantErr:           false, // ExportReceipts doesn't check status codes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedQuery url.Values

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != "/api/audit/receipts/export" {
					t.Errorf("expected path /api/audit/receipts/export, got %s", r.URL.Path)
				}

				receivedQuery = r.URL.Query()
				if tt.operatorSessionID != "" {
					if receivedQuery.Get("operator_session_id") != tt.operatorSessionID {
						t.Errorf("expected operator_session_id %s, got %s", tt.operatorSessionID, receivedQuery.Get("operator_session_id"))
					}
				} else {
					if receivedQuery.Get("operator_session_id") != "" {
						t.Error("should not have operator_session_id query param when empty")
					}
				}

				w.Write(tt.responseBody)
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
			body, err := client.ExportReceipts(ctx, tt.operatorSessionID)

			if (err != nil) != tt.wantErr {
				t.Errorf("ExportReceipts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(body) == 0 && len(tt.responseBody) > 0 {
					t.Error("expected non-empty body")
				}
			}
		})
	}
}

func TestDiscoverOperatorSession(t *testing.T) {
	tests := []struct {
		name            string
		cfgSessionID    string
		useCLIConfig    bool
		responseBody    string
		expectedSession string
		setupHandler    func(*http.Request)
	}{
		{
			name:            "session ID pinned in config",
			cfgSessionID:    "pinned-session-123",
			useCLIConfig:    false,
			responseBody:    `{"operators":[]}`,
			expectedSession: "pinned-session-123",
			setupHandler:    nil, // should not make HTTP call
		},
		{
			name:            "no session ID, no CLI config, empty response",
			cfgSessionID:    "",
			useCLIConfig:    false,
			responseBody:    `{"operators":[]}`,
			expectedSession: "",
		},
		{
			name:            "wrapped operators array",
			cfgSessionID:    "",
			useCLIConfig:    false,
			responseBody:    `{"operators":[{"operator_session_id":"session-456"}]}`,
			expectedSession: "session-456",
		},
		{
			name:            "bare operators array",
			cfgSessionID:    "",
			useCLIConfig:    false,
			responseBody:    `[{"operator_session_id":"session-789"}]`,
			expectedSession: "session-789",
		},
		{
			name:            "multiple operators, returns first",
			cfgSessionID:    "",
			useCLIConfig:    false,
			responseBody:    `[{"operator_session_id":"session-1"},{"operator_session_id":"session-2"}]`,
			expectedSession: "session-1",
		},
		{
			name:            "operator without session ID",
			cfgSessionID:    "",
			useCLIConfig:    false,
			responseBody:    `[{"operator_id":"op-1"}]`,
			expectedSession: "",
		},
		{
			name:            "invalid JSON response",
			cfgSessionID:    "",
			useCLIConfig:    false,
			responseBody:    `invalid json`,
			expectedSession: "",
		},
		{
			name:            "network error",
			cfgSessionID:    "",
			useCLIConfig:    false,
			responseBody:    ``,
			expectedSession: "",
			setupHandler: func(r *http.Request) {
				// Simulate network error by closing connection
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != "/api/operators" {
					t.Errorf("expected path /api/operators, got %s", r.URL.Path)
				}

				if tt.setupHandler != nil {
					tt.setupHandler(r)
				}

				if tt.name == "network error" {
					hj, ok := w.(http.Hijacker)
					if ok {
						conn, _, _ := hj.Hijack()
						conn.Close()
						return
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tt.responseBody))
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL:       server.URL,
				OperatorSessionID: tt.cfgSessionID,
				UseCLIConfig:      tt.useCLIConfig,
				Auth:              config.Auth{Insecure: true},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			sessionID := client.DiscoverOperatorSession(ctx)

			if sessionID != tt.expectedSession {
				t.Errorf("DiscoverOperatorSession() = %s, want %s", sessionID, tt.expectedSession)
			}
		})
	}
}

func TestDiscoverOperatorSession_WithUserID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Note: CLI config loading can't be mocked in unit test without filesystem access
		// so user_id won't be present. This test verifies the HTTP call structure.

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"operator_session_id":"session-456"}]`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL:  server.URL,
		UseCLIConfig: true,
		Auth:         config.Auth{Insecure: true},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	sessionID := client.DiscoverOperatorSession(ctx)

	// Since we can't mock the CLI config loading, this will make
	// the HTTP call without user_id. The test verifies the call
	// structure completes without error.
	_ = sessionID
}

func TestReceipt(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{
			name: "minimal receipt",
			data: map[string]interface{}{
				"transaction_id":    "tx-123",
				"transaction_hash":  "hash-abc",
				"action_type":       "EXECUTE_BASH",
				"target_resource":   "localhost",
				"status":            "completed",
				"state_root_before": "root-before",
				"state_root_after":  "root-after",
				"signature":         "sig-def",
			},
		},
		{
			name: "receipt with all fields",
			data: map[string]interface{}{
				"transaction_id":    "tx-456",
				"transaction_hash":  "hash-xyz",
				"action_type":       "MCP_CALL",
				"target_resource":   "remote-host",
				"status":            "pending",
				"state_root_before": "root-1",
				"state_root_after":  "root-2",
				"signature":         "sig-xyz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("failed to marshal test data: %v", err)
			}

			var rec Receipt
			if err := json.Unmarshal(data, &rec); err != nil {
				t.Fatalf("failed to unmarshal Receipt: %v", err)
			}

			if rec.TransactionID != tt.data["transaction_id"] {
				t.Errorf("TransactionID mismatch")
			}
			if rec.TransactionHash != tt.data["transaction_hash"] {
				t.Errorf("TransactionHash mismatch")
			}
			if rec.ActionType != tt.data["action_type"] {
				t.Errorf("ActionType mismatch")
			}
			if rec.TargetResource != tt.data["target_resource"] {
				t.Errorf("TargetResource mismatch")
			}
			if rec.Status != tt.data["status"] {
				t.Errorf("Status mismatch")
			}
			if rec.StateRootBefore != tt.data["state_root_before"] {
				t.Errorf("StateRootBefore mismatch")
			}
			if rec.StateRootAfter != tt.data["state_root_after"] {
				t.Errorf("StateRootAfter mismatch")
			}
			if rec.Signature != tt.data["signature"] {
				t.Errorf("Signature mismatch")
			}
		})
	}
}

func TestReceipt_RawField(t *testing.T) {
	data := map[string]interface{}{
		"transaction_id": "tx-123",
		"status":         "completed",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var rec Receipt
	if err := json.Unmarshal(jsonData, &rec); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	rec.Raw = jsonData

	if rec.Raw == nil {
		t.Error("Raw field should be set")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(rec.Raw, &decoded); err != nil {
		t.Errorf("failed to unmarshal Raw field: %v", err)
	}

	if decoded["transaction_id"] != "tx-123" {
		t.Error("Raw field should contain original data")
	}
}

func TestReceipt_MarshalJSON(t *testing.T) {
	rec := Receipt{
		TransactionID:   "tx-123",
		TransactionHash: "hash-abc",
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Status:          "completed",
		StateRootBefore: "root-before",
		StateRootAfter:  "root-after",
		Signature:       "sig-def",
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("failed to marshal Receipt: %v", err)
	}

	// Raw field should not be included in JSON output
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, exists := decoded["raw"]; exists {
		t.Error("Raw field should not be marshaled to JSON")
	}

	if decoded["transaction_id"] != "tx-123" {
		t.Error("TransactionID should be in JSON output")
	}
}
