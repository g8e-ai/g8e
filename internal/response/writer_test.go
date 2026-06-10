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

package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestNewWriter(t *testing.T) {
	var logger *slog.Logger
	w := NewWriter(logger)
	if w == nil {
		t.Fatal("NewWriter() returned nil")
		return
	}
	if w.logger != logger {
		t.Error("NewWriter() did not set logger")
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
			w := NewWriter((*slog.Logger)(nil))
			rw := httptest.NewRecorder()

			w.JSON(rw, tt.status, tt.data)

			if rw.Code != tt.status {
				t.Errorf("JSON() status = %v, want %v", rw.Code, tt.status)
			}

			ct := rw.Header().Get(constants.HeaderContentType)
			if ct != "application/json" {
				t.Errorf("JSON() Content-Type = %v, want application/json", ct)
			}

			if rw.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Error("JSON() missing X-Content-Type-Options header")
			}

			if rw.Header().Get("X-Frame-Options") != "DENY" {
				t.Error("JSON() missing X-Frame-Options header")
			}

			if rw.Header().Get("Content-Security-Policy") != "default-src 'none'; frame-ancestors 'none'" {
				t.Error("JSON() missing or incorrect Content-Security-Policy header")
			}

			var got map[string]interface{}
			if err := json.Unmarshal(rw.Body.Bytes(), &got); err != nil {
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
			w := NewWriter((*slog.Logger)(nil))
			rw := httptest.NewRecorder()

			w.Error(rw, tt.status, tt.msg)

			if rw.Code != tt.wantCode {
				t.Errorf("Error() status = %v, want %v", rw.Code, tt.wantCode)
			}

			ct := rw.Header().Get(constants.HeaderContentType)
			if ct != "application/json" {
				t.Errorf("Error() Content-Type = %v, want application/json", ct)
			}

			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rw.Body.Bytes(), &resp); err != nil {
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
			w := NewWriter((*slog.Logger)(nil))
			rw := httptest.NewRecorder()

			w.RPCResponse(rw, tt.id, tt.result)

			if rw.Code != http.StatusOK {
				t.Errorf("RPCResponse() status = %v, want 200", rw.Code)
			}

			ct := rw.Header().Get(constants.HeaderContentType)
			if ct != "application/json" {
				t.Errorf("RPCResponse() Content-Type = %v, want application/json", ct)
			}

			var resp JSONRPCResponse
			if err := json.Unmarshal(rw.Body.Bytes(), &resp); err != nil {
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
	w := NewWriter(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	rw := httptest.NewRecorder()

	type unmarshalable struct {
		Func func()
	}

	result := unmarshalable{
		Func: func() {},
	}

	w.RPCResponse(rw, 1, result)

	if rw.Code != http.StatusOK {
		t.Errorf("RPCResponse() status = %v, want 200", rw.Code)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(rw.Body.Bytes(), &resp); err != nil {
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
			code:     constants.ErrCodeInvalidEnvelope,
			msg:      "Invalid envelope",
			wantID:   float64(2),
			wantCode: constants.ErrCodeInvalidEnvelope,
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
			w := NewWriter((*slog.Logger)(nil))
			rw := httptest.NewRecorder()

			w.RPCError(rw, tt.id, tt.code, tt.msg)

			if rw.Code != http.StatusOK {
				t.Errorf("RPCError() status = %v, want 200", rw.Code)
			}

			ct := rw.Header().Get(constants.HeaderContentType)
			if ct != "application/json" {
				t.Errorf("RPCError() Content-Type = %v, want application/json", ct)
			}

			var resp JSONRPCResponse
			if err := json.Unmarshal(rw.Body.Bytes(), &resp); err != nil {
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
		{"ErrCodeInvalidEnvelope", constants.ErrCodeInvalidEnvelope, -32000},
		{"ErrCodeHashMismatch", constants.ErrCodeHashMismatch, -32001},
		{"ErrCodeExpired", constants.ErrCodeExpired, -32002},
		{"ErrCodeReplay", constants.ErrCodeReplay, -32003},
		{"ErrCodeStateMismatch", constants.ErrCodeStateMismatch, -32004},
		{"ErrCodeL1ValidationFailed", constants.ErrCodeL1ValidationFailed, -32005},
		{"ErrCodeL2SignatureInvalid", constants.ErrCodeL2SignatureInvalid, -32006},
		{"ErrCodeL3ProofInvalid", constants.ErrCodeL3ProofInvalid, -32007},
		{"ErrCodePayloadDecodeFailed", constants.ErrCodePayloadDecodeFailed, -32008},
		{"ErrCodeResourceNotFound", constants.ErrCodeResourceNotFound, -32100},
		{"ErrCodeGatewayNotReady", constants.ErrCodeGatewayNotReady, -32101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.value {
				t.Errorf("%s = %v, want %v", tt.name, tt.code, tt.value)
			}
		})
	}
}
