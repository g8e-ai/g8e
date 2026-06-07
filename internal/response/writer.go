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

	"github.com/g8e-ai/g8e/internal/constants"
)

// JSONRPCRequest represents a standard JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

// JSONRPCResponse represents a standard JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
}

// JSONRPCError represents a standard JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Writer provides a unified way to write JSON and JSON-RPC responses.
type Writer struct {
	logger *slog.Logger
}

// NewWriter creates a new Writer.
func NewWriter(logger *slog.Logger) *Writer {
	return &Writer{
		logger: logger,
	}
}

// JSON writes a standard JSON response.
func (w *Writer) JSON(rw http.ResponseWriter, status int, data interface{}) {
	rw.Header().Set(constants.HeaderContentType, "application/json")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("X-Frame-Options", "DENY")
	rw.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	rw.WriteHeader(status)
	if err := json.NewEncoder(rw).Encode(data); err != nil {
		w.logger.Error("Failed to encode JSON response", "error", err)
	}
}

// Error writes a standard JSON error response.
func (w *Writer) Error(rw http.ResponseWriter, status int, msg string) {
	w.JSON(rw, status, struct {
		Error string `json:"error"`
	}{Error: msg})
}

// RPCResponse writes a JSON-RPC 2.0 success response.
func (w *Writer) RPCResponse(rw http.ResponseWriter, id interface{}, result interface{}) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
	}

	// If result is already json.RawMessage, don't double-marshal
	if raw, ok := result.(json.RawMessage); ok {
		res.Result = raw
	} else {
		b, err := json.Marshal(result)
		if err != nil {
			w.logger.Error("Failed to marshal JSON-RPC result", "error", err)
			w.RPCError(rw, id, -32603, "failed to marshal result")
			return
		}
		res.Result = json.RawMessage(b)
	}

	rw.Header().Set(constants.HeaderContentType, "application/json")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("X-Frame-Options", "DENY")
	rw.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(rw).Encode(res); err != nil {
		w.logger.Error("Failed to encode JSON-RPC response", "error", err)
	}
}

// RPCError writes a JSON-RPC 2.0 error response.
func (w *Writer) RPCError(rw http.ResponseWriter, id interface{}, code int, msg string) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: msg,
		},
	}
	rw.Header().Set(constants.HeaderContentType, "application/json")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("X-Frame-Options", "DENY")
	rw.WriteHeader(http.StatusOK) // JSON-RPC usually returns 200 even for errors
	if err := json.NewEncoder(rw).Encode(res); err != nil {
		w.logger.Error("Failed to encode JSON-RPC error response", "error", err)
	}
}
