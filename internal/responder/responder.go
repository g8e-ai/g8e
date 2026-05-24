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

// Protocol-specific error codes for g8eo (reserved range -32000 to -32099)
const (
	// Verification Errors (-32000 range)
	ErrCodeInvalidEnvelope     = -32000
	ErrCodeHashMismatch        = -32001
	ErrCodeExpired             = -32002
	ErrCodeReplay              = -32003
	ErrCodeStateMismatch       = -32004
	ErrCodeL1ValidationFailed  = -32005
	ErrCodeL2SignatureInvalid  = -32006
	ErrCodeL3ProofInvalid      = -32007
	ErrCodePayloadDecodeFailed = -32008

	// Resource/State Errors (-32100 range)
	ErrCodeResourceNotFound = -32100
	ErrCodeGatewayNotReady  = -32101
)

// Responder provides a unified way to write JSON and JSON-RPC responses.
type Responder struct {
	logger *slog.Logger
}

// New creates a new Responder.
func New(logger *slog.Logger) *Responder {
	return &Responder{
		logger: logger,
	}
}

// JSON writes a standard JSON response.
func (r *Responder) JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set(constants.HeaderContentType, "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		r.logger.Error("Failed to encode JSON response", "error", err)
	}
}

// Error writes a standard JSON error response.
func (r *Responder) Error(w http.ResponseWriter, status int, msg string) {
	r.JSON(w, status, struct {
		Error string `json:"error"`
	}{Error: msg})
}

// RPCResponse writes a JSON-RPC 2.0 success response.
func (r *Responder) RPCResponse(w http.ResponseWriter, id interface{}, result interface{}) {
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
			r.logger.Error("Failed to marshal JSON-RPC result", "error", err)
			r.RPCError(w, id, -32603, "failed to marshal result")
			return
		}
		res.Result = json.RawMessage(b)
	}

	w.Header().Set(constants.HeaderContentType, "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		r.logger.Error("Failed to encode JSON-RPC response", "error", err)
	}
}

// RPCError writes a JSON-RPC 2.0 error response.
func (r *Responder) RPCError(w http.ResponseWriter, id interface{}, code int, msg string) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: msg,
		},
	}
	w.Header().Set(constants.HeaderContentType, "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(http.StatusOK) // JSON-RPC usually returns 200 even for errors
	if err := json.NewEncoder(w).Encode(res); err != nil {
		r.logger.Error("Failed to encode JSON-RPC error response", "error", err)
	}
}
