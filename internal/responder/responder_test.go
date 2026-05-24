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

package responder

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestNew(t *testing.T) {
	var logger *slog.Logger
	r := New(logger)
	if r == nil {
		t.Fatal("New() returned nil")
	}
	if r.logger != logger {
		t.Error("New() did not set logger")
	}
}

func TestJSON(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		data     interface{}
		wantBody string
	}{
		{
			name:     "success response",
			status:   http.StatusOK,
			data:     map[string]string{"message": "hello"},
			wantBody: `{"message":"hello"}`,
		},
		{
			name:     "created response",
			status:   http.StatusCreated,
			data:     map[string]int{"id": 123},
			wantBody: `{"id":123}`,
		},
		{
			name:     "null data",
			status:   http.StatusOK,
			data:     nil,
			wantBody: "null",
		},
		{
			name:     "empty object",
			status:   http.StatusOK,
			data:     struct{}{},
			wantBody: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New((*slog.Logger)(nil))
			w := httptest.NewRecorder()

			r.JSON(w, tt.status, tt.data)

			if w.Code != tt.status {
				t.Errorf("JSON() status = %v, want %v", w.Code, tt.status)
			}

			ct := w.Header().Get(constants.HeaderContentType)
			if ct != "application/json" {
				t.Errorf("JSON() Content-Type = %v, want application/json", ct)
			}

			if w.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Error("JSON() missing X-Content-Type-Options header")
			}

			if w.Header().Get("X-Frame-Options") != "DENY" {
				t.Error("JSON() missing X-Frame-Options header")
			}

			if w.Header().Get("Content-Security-Policy") != "default-src 'none'; frame-ancestors 'none'" {
				t.Error("JSON() missing or incorrect Content-Security-Policy header")
			}

			var got map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("JSON() failed to unmarshal response: %v", err)
			}
		})
	}
}

func TestError(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		msg      string
		wantCode int
	}{
		{
			name:     "bad request",
			status:   http.StatusBadRequest,
			msg:      "invalid input",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "not found",
			status:   http.StatusNotFound,
			msg:      "resource not found",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "internal server error",
			status:   http.StatusInternalServerError,
			msg:      "something went wrong",
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New((*slog.Logger)(nil))
			w := httptest.NewRecorder()

			r.Error(w, tt.status, tt.msg)

			if w.Code != tt.wantCode {
				t.Errorf("Error() status = %v, want %v", w.Code, tt.wantCode)
			}

			ct := w.Header().Get(constants.HeaderContentType)
			if ct != "application/json" {
				t.Errorf("Error() Content-Type = %v, want application/json", ct)
			}

			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("Error() failed to unmarshal response: %v", err)
			}

			if resp.Error != tt.msg {
				t.Errorf("Error() message = %v, want %v", resp.Error, tt.msg)
			}
		})
	}
}

func TestRPCResponse(t *testing.T) {
	tests := []struct {
		name     string
		id       interface{}
		result   interface{}
		wantID   interface{}
		wantJSON string
	}{
		{
			name:     "string result",
			id:       1,
			result:   "success",
			wantID:   float64(1),
			wantJSON: `"success"`,
		},
		{
			name:     "object result",
			id:       2,
			result:   map[string]string{"key": "value"},
			wantID:   float64(2),
			wantJSON: `{"key":"value"}`,
		},
		{
			name:     "null result",
			id:       3,
			result:   nil,
			wantID:   float64(3),
			wantJSON: "null",
		},
		{
			name:     "json.RawMessage result",
			id:       4,
			result:   json.RawMessage(`{"raw":"message"}`),
			wantID:   float64(4),
			wantJSON: `{"raw":"message"}`,
		},
		{
			name:     "string ID",
			id:       "test-id",
			result:   "data",
			wantID:   "test-id",
			wantJSON: `"data"`,
		},
		{
			name:     "null ID",
			id:       nil,
			result:   "data",
			wantID:   nil,
			wantJSON: `"data"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New((*slog.Logger)(nil))
			w := httptest.NewRecorder()

			r.RPCResponse(w, tt.id, tt.result)

			if w.Code != http.StatusOK {
				t.Errorf("RPCResponse() status = %v, want 200", w.Code)
			}

			ct := w.Header().Get(constants.HeaderContentType)
			if ct != "application/json" {
				t.Errorf("RPCResponse() Content-Type = %v, want application/json", ct)
			}

			var resp JSONRPCResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("RPCResponse() failed to unmarshal response: %v", err)
			}

			if resp.JSONRPC != "2.0" {
				t.Errorf("RPCResponse() jsonrpc = %v, want 2.0", resp.JSONRPC)
			}

			if resp.ID != tt.wantID {
				t.Errorf("RPCResponse() id = %v, want %v", resp.ID, tt.wantID)
			}

			if resp.Error != nil {
				t.Errorf("RPCResponse() error = %v, want nil", resp.Error)
			}

			if string(resp.Result) != tt.wantJSON {
				t.Errorf("RPCResponse() result = %v, want %v", string(resp.Result), tt.wantJSON)
			}
		})
	}
}

func TestRPCResponse_UnmarshalableResult(t *testing.T) {
	r := New(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	w := httptest.NewRecorder()

	type unmarshalable struct {
		Func func()
	}

	result := unmarshalable{
		Func: func() {},
	}

	r.RPCResponse(w, 1, result)

	if w.Code != http.StatusOK {
		t.Errorf("RPCResponse() status = %v, want 200", w.Code)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("RPCResponse() failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Error("RPCResponse() expected error for unmarshalable result, got nil")
	}

	if resp.Error.Code != -32603 {
		t.Errorf("RPCResponse() error code = %v, want -32603", resp.Error.Code)
	}
}

func TestRPCError(t *testing.T) {
	tests := []struct {
		name     string
		id       interface{}
		code     int
		msg      string
		wantID   interface{}
		wantCode int
		wantMsg  string
	}{
		{
			name:     "standard error",
			id:       1,
			code:     -32600,
			msg:      "Invalid request",
			wantID:   float64(1),
			wantCode: -32600,
			wantMsg:  "Invalid request",
		},
		{
			name:     "verification error",
			id:       2,
			code:     ErrCodeInvalidEnvelope,
			msg:      "Invalid envelope",
			wantID:   float64(2),
			wantCode: ErrCodeInvalidEnvelope,
			wantMsg:  "Invalid envelope",
		},
		{
			name:     "string ID",
			id:       "test-id",
			code:     -32700,
			msg:      "Parse error",
			wantID:   "test-id",
			wantCode: -32700,
			wantMsg:  "Parse error",
		},
		{
			name:     "null ID",
			id:       nil,
			code:     -32602,
			msg:      "Invalid params",
			wantID:   nil,
			wantCode: -32602,
			wantMsg:  "Invalid params",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New((*slog.Logger)(nil))
			w := httptest.NewRecorder()

			r.RPCError(w, tt.id, tt.code, tt.msg)

			if w.Code != http.StatusOK {
				t.Errorf("RPCError() status = %v, want 200", w.Code)
			}

			ct := w.Header().Get(constants.HeaderContentType)
			if ct != "application/json" {
				t.Errorf("RPCError() Content-Type = %v, want application/json", ct)
			}

			var resp JSONRPCResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("RPCError() failed to unmarshal response: %v", err)
			}

			if resp.JSONRPC != "2.0" {
				t.Errorf("RPCError() jsonrpc = %v, want 2.0", resp.JSONRPC)
			}

			if resp.ID != tt.wantID {
				t.Errorf("RPCError() id = %v, want %v", resp.ID, tt.wantID)
			}

			if resp.Error == nil {
				t.Fatal("RPCError() error = nil, want error object")
			}

			if resp.Error.Code != tt.wantCode {
				t.Errorf("RPCError() error code = %v, want %v", resp.Error.Code, tt.wantCode)
			}

			if resp.Error.Message != tt.wantMsg {
				t.Errorf("RPCError() error message = %v, want %v", resp.Error.Message, tt.wantMsg)
			}

			if resp.Result != nil {
				t.Error("RPCError() result = non-nil, want nil")
			}
		})
	}
}

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		name  string
		code  int
		value int
	}{
		{"ErrCodeInvalidEnvelope", ErrCodeInvalidEnvelope, -32000},
		{"ErrCodeHashMismatch", ErrCodeHashMismatch, -32001},
		{"ErrCodeExpired", ErrCodeExpired, -32002},
		{"ErrCodeReplay", ErrCodeReplay, -32003},
		{"ErrCodeStateMismatch", ErrCodeStateMismatch, -32004},
		{"ErrCodeL1ValidationFailed", ErrCodeL1ValidationFailed, -32005},
		{"ErrCodeL2SignatureInvalid", ErrCodeL2SignatureInvalid, -32006},
		{"ErrCodeL3ProofInvalid", ErrCodeL3ProofInvalid, -32007},
		{"ErrCodePayloadDecodeFailed", ErrCodePayloadDecodeFailed, -32008},
		{"ErrCodeResourceNotFound", ErrCodeResourceNotFound, -32100},
		{"ErrCodeGatewayNotReady", ErrCodeGatewayNotReady, -32101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.value {
				t.Errorf("%s = %v, want %v", tt.name, tt.code, tt.value)
			}
		})
	}
}
