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

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
)

// JSONRPCRequest is an alias for responder.JSONRPCRequest
type JSONRPCRequest = responder.JSONRPCRequest

// JSONRPCResponse is an alias for responder.JSONRPCResponse
type JSONRPCResponse = responder.JSONRPCResponse

// JSONRPCError is an alias for responder.JSONRPCError
type JSONRPCError = responder.JSONRPCError

// Protocol-specific error codes for g8eo (reserved range -32000 to -32099)
const (
	// Verification Errors (-32000 range)
	ErrCodeInvalidEnvelope     = responder.ErrCodeInvalidEnvelope
	ErrCodeHashMismatch        = responder.ErrCodeHashMismatch
	ErrCodeExpired             = responder.ErrCodeExpired
	ErrCodeReplay              = responder.ErrCodeReplay
	ErrCodeStateMismatch       = responder.ErrCodeStateMismatch
	ErrCodeL1ValidationFailed  = responder.ErrCodeL1ValidationFailed
	ErrCodeL2SignatureInvalid  = responder.ErrCodeL2SignatureInvalid
	ErrCodeL3ProofInvalid      = responder.ErrCodeL3ProofInvalid
	ErrCodePayloadDecodeFailed = responder.ErrCodePayloadDecodeFailed

	// Resource/State Errors (-32100 range)
	ErrCodeResourceNotFound = responder.ErrCodeResourceNotFound
	ErrCodeGatewayNotReady  = responder.ErrCodeGatewayNotReady
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

// ToolsListResult is the result for the "tools/list" method.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// Tool represents an MCP tool.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ResourcesListResult is the result for the "resources/list" method.
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// PromptsListResult is the result for the "prompts/list" method.
type PromptsListResult struct {
	Prompts []Prompt `json:"prompts"`
}

// A2ASuspensionResponse is returned when an A2A call is suspended for L3 approval.
type A2ASuspensionResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	TxHash      string `json:"tx_hash"`
	ApprovalURL string `json:"approval_url"`
	Message     string `json:"message"`
}

// A2ASuccessResponse is returned when an A2A call succeeds.
type A2ASuccessResponse struct {
	ID     string      `json:"id"`
	Result interface{} `json:"result"`
}

// A2ADownstreamRequest is the request sent to a downstream A2A server.
type A2ADownstreamRequest struct {
	SkillName   string          `json:"skill_name"`
	PayloadJSON json.RawMessage `json:"payload"`
	ExecutionID string          `json:"execution_id,omitempty"`
}

// FieldReadRequest is the params for the "read_field" tool.
type FieldReadRequest struct {
	Collection        string `json:"collection"`
	DocumentID        string `json:"document_id"`
	FieldPath         string `json:"field_path"`
	OperatorSessionID string `json:"operator_session_id"`
}

// FieldReadResult is the result for the "read_field" tool.
type FieldReadResult struct {
	Value interface{} `json:"value"`
}
