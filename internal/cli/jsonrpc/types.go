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

package jsonrpc

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Standard JSON-RPC 2.0 error codes
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Gateway-specific error codes (reserved range -32000 to -32099)
const (
	CodeInvalidEnvelope     = -32000
	CodeHashMismatch        = -32001
	CodeExpired             = -32002
	CodeReplay              = -32003
	CodeStateMismatch       = -32004
	CodeL1ValidationFailed  = -32005
	CodeL2SignatureInvalid  = -32006
	CodeL3ProofInvalid      = -32007
	CodePayloadDecodeFailed = -32008
	CodeResourceNotFound    = -32098
	CodeGatewayNotReady     = -32099
)

// Request represents a JSON-RPC 2.0 request
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

// Validate checks if the request is valid according to JSON-RPC 2.0 spec
func (r *Request) Validate() error {
	if r.JSONRPC != "2.0" {
		return fmt.Errorf("invalid jsonrpc version: %s", r.JSONRPC)
	}
	if r.Method == "" {
		return errors.New("method is required")
	}
	if r.ID == nil {
		return errors.New("id is required")
	}
	return nil
}

// Response represents a JSON-RPC 2.0 response
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObj       `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
}

// ErrorObj represents a JSON-RPC 2.0 error object
type ErrorObj struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// NewSuccessResponse creates a successful JSON-RPC response
func NewSuccessResponse(id interface{}, result json.RawMessage) *Response {
	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

// NewErrorResponse creates an error JSON-RPC response
func NewErrorResponse(id interface{}, code int, message string, data json.RawMessage) *Response {
	return &Response{
		JSONRPC: "2.0",
		Error: &ErrorObj{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
}

// NewParseErrorResponse creates a parse error response
func NewParseErrorResponse(id interface{}, err error) *Response {
	return NewErrorResponse(id, CodeParseError, "Parse error", nil)
}

// NewInvalidRequestResponse creates an invalid request response
func NewInvalidRequestResponse(id interface{}, reason string) *Response {
	return NewErrorResponse(id, CodeInvalidRequest, "Invalid Request: "+reason, nil)
}

// NewMethodNotFoundResponse creates a method not found response
func NewMethodNotFoundResponse(id interface{}, method string) *Response {
	return NewErrorResponse(id, CodeMethodNotFound, fmt.Sprintf("Method not found: %s", method), nil)
}

// NewInvalidParamsResponse creates an invalid params response
func NewInvalidParamsResponse(id interface{}, reason string) *Response {
	return NewErrorResponse(id, CodeInvalidParams, "Invalid params: "+reason, nil)
}

// NewInternalErrorResponse creates an internal error response
func NewInternalErrorResponse(id interface{}, err error) *Response {
	msg := "Internal error"
	if err != nil {
		msg = err.Error()
	}
	return NewErrorResponse(id, CodeInternalError, msg, nil)
}
