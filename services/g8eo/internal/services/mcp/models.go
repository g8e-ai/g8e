package mcp

import (
	"encoding/json"

	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
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
	ErrCodeResourceNotFound  = -32100
	ErrCodeSubstrateNotReady = -32101
)

// CallToolRequest is the params for the "tools/call" method.
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// CallToolResult is the result for the "tools/call" method.
type CallToolResult struct {
	Content []TextContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// TextContent represents a text part of a tool response.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SuspendedTransaction is an alias for the shared models type.
type SuspendedTransaction = models.SuspendedTransaction
