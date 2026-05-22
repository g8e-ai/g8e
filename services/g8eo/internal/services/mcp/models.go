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
	ErrCodeResourceNotFound = -32100
	ErrCodeGatewayNotReady  = -32101
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

// ListResourcesRequest is the params for the "resources/list" method.
type ListResourcesRequest struct {
	// Optional cursor for pagination
	Cursor *string `json:"cursor,omitempty"`
}

// Resource represents an MCP resource.
type Resource struct {
	URI         string                 `json:"uri"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	MimeType    string                 `json:"mimeType,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ListResourcesResult is the result for the "resources/list" method.
type ListResourcesResult struct {
	Resources []Resource `json:"resources"`
}

// ReadResourceRequest is the params for the "resources/read" method.
type ReadResourceRequest struct {
	URI string `json:"uri"`
}

// ReadResourceResult is the result for the "resources/read" method.
type ReadResourceResult struct {
	Contents []TextContent `json:"contents"`
	MIMEType string        `json:"mimeType,omitempty"`
}

// ListPromptsRequest is the params for the "prompts/list" method.
type ListPromptsRequest struct {
	// Optional cursor for pagination
	Cursor *string `json:"cursor,omitempty"`
}

// Prompt represents an MCP prompt template.
type Prompt struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Arguments   []PromptArgument       `json:"arguments,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// PromptArgument represents an argument for a prompt template.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ListPromptsResult is the result for the "prompts/list" method.
type ListPromptsResult struct {
	Prompts []Prompt `json:"prompts"`
}

// GetPromptRequest is the params for the "prompts/get" method.
type GetPromptRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// GetPromptResult is the result for the "prompts/get" method.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages,omitempty"`
}

// PromptMessage represents a message in a prompt template.
type PromptMessage struct {
	Role    string      `json:"role"`
	Content TextContent `json:"content"`
}
