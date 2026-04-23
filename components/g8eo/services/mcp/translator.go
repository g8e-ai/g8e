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
	"fmt"

	"reflect"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
)

// MCPToolNameToEventType maps MCP tool names to g8e event types.
func MCPToolNameToEventType(toolName string) string {
	switch toolName {
	case "run_commands_with_operator":
		return constants.Event.Operator.Command.Requested
	case "file_create_on_operator", "file_write_on_operator", "file_read_on_operator", "file_update_on_operator":
		return constants.Event.Operator.FileEdit.Requested
	case "check_port_status":
		return constants.Event.Operator.PortCheck.Requested
	case "list_files_and_directories_with_detailed_metadata":
		return constants.Event.Operator.FsList.Requested
	case "fetch_file_history":
		return constants.Event.Operator.FetchFileHistory.Requested
	case "fetch_file_diff":
		return constants.Event.Operator.FetchFileDiff.Requested
	case "grant_intent_permission":
		return constants.Event.Operator.Intent.ApprovalRequested
	default:
		return ""
	}
}

// validateToolArguments validates MCP tool arguments against the expected g8e payload structure.
// This prevents malformed payloads from bypassing typed deserialization.
func validateToolArguments(toolName string, arguments json.RawMessage) error {
	switch toolName {
	case "run_commands_with_operator":
		var payload models.CommandRequestPayload
		return json.Unmarshal(arguments, &payload)
	case "file_create_on_operator", "file_write_on_operator", "file_read_on_operator", "file_update_on_operator":
		var payload models.FileEditRequestPayload
		return json.Unmarshal(arguments, &payload)
	case "check_port_status":
		var payload models.PortCheckRequestPayload
		return json.Unmarshal(arguments, &payload)
	case "list_files_and_directories_with_detailed_metadata":
		var payload models.FsListRequestPayload
		return json.Unmarshal(arguments, &payload)
	case "fetch_file_history":
		var payload models.FetchFileHistoryRequestPayload
		return json.Unmarshal(arguments, &payload)
	case "fetch_file_diff":
		var payload models.FetchFileDiffRequestPayload
		return json.Unmarshal(arguments, &payload)
	case "grant_intent_permission":
		var payload map[string]interface{}
		return json.Unmarshal(arguments, &payload)
	default:
		return fmt.Errorf("unsupported tool for validation: %s", toolName)
	}
}

// TranslateToolCallToCommand converts an MCP JSON-RPC tool call into an internal g8e message.
func TranslateToolCallToCommand(req *JSONRPCRequest) (*models.G8eMessage, error) {
	if req.Method != "tools/call" {
		return nil, fmt.Errorf("method not found: %s", req.Method)
	}

	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, fmt.Errorf("failed to unmarshal call tool params: %w", err)
	}

	eventType := MCPToolNameToEventType(params.Name)
	if eventType == "" {
		return nil, fmt.Errorf("unsupported tool: %s", params.Name)
	}

	if err := validateToolArguments(params.Name, params.Arguments); err != nil {
		return nil, fmt.Errorf("invalid tool arguments for %s: %w", params.Name, err)
	}

	// For the first pass, we map the MCP request ID as the g8e message ID.
	// This ensures the ID travels with the request through the execution pipeline.
	g8eMsg := &models.G8eMessage{
		ID:        req.ID,
		EventType: eventType,
		Payload:   params.Arguments,
	}

	return g8eMsg, nil
}

// TranslateResultToMCP wraps a g8e result payload into an MCP JSON-RPC response.
func TranslateResultToMCP(requestID string, executionID string, eventType string, payload interface{}) (*JSONRPCResponse, error) {
	if payload == nil {
		return nil, fmt.Errorf("nil payload")
	}

	// Normalize pointers to values for simpler switching
	val := reflect.ValueOf(payload)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil, fmt.Errorf("nil payload")
		}
		payload = val.Elem().Interface()
	}

	content := []Content{}
	isError := false
	var metadata *MCPResultMetadata

	switch p := payload.(type) {
	case models.ExecutionResultsPayload:
		if p.Stdout != "" {
			content = append(content, Content{Type: "text", Text: p.Stdout})
		}
		if p.Stderr != "" {
			content = append(content, Content{Type: "text", Text: p.Stderr})
		}
		if p.ErrorMessage != nil {
			content = append(content, Content{Type: "text", Text: *p.ErrorMessage})
			isError = true
		}
	case models.FileEditResultPayload:
		metadata = &MCPResultMetadata{
			OriginalPayload: p,
			EventType:       eventType,
			ExecutionID:     executionID,
		}
		if p.ErrorMessage != nil {
			content = append(content, Content{Type: "text", Text: *p.ErrorMessage})
			isError = true
		} else if p.Content != nil {
			content = append(content, Content{Type: "text", Text: *p.Content})
		}
	case models.FsListResultPayload:
		metadata = &MCPResultMetadata{
			OriginalPayload: p,
			EventType:       eventType,
			ExecutionID:     executionID,
		}
		if p.ErrorMessage != nil {
			content = append(content, Content{Type: "text", Text: *p.ErrorMessage})
			isError = true
		} else {
			raw, _ := json.MarshalIndent(p.Entries, "", "  ")
			content = append(content, Content{Type: "text", Text: string(raw)})
		}
	case models.PortCheckResultPayload:
		metadata = &MCPResultMetadata{
			OriginalPayload: p,
			EventType:       eventType,
			ExecutionID:     executionID,
		}
		if p.ErrorMessage != nil {
			content = append(content, Content{Type: "text", Text: *p.ErrorMessage})
			isError = true
		} else {
			for _, entry := range p.Results {
				status := "CLOSED"
				if entry.Open {
					status = "OPEN"
				}
				content = append(content, Content{Type: "text", Text: fmt.Sprintf("Host %s Port %d is %s", entry.Host, entry.Port, status)})
			}
		}
	case models.FsReadResultPayload:
		metadata = &MCPResultMetadata{
			OriginalPayload: p,
			EventType:       eventType,
			ExecutionID:     executionID,
		}
		if p.ErrorMessage != nil {
			content = append(content, Content{Type: "text", Text: *p.ErrorMessage})
			isError = true
		} else {
			content = append(content, Content{Type: "text", Text: p.Content})
		}
	case models.FetchFileHistoryResultPayload:
		metadata = &MCPResultMetadata{
			OriginalPayload: p,
			EventType:       eventType,
			ExecutionID:     executionID,
		}
		if p.Error != "" {
			content = append(content, Content{Type: "text", Text: p.Error})
			isError = true
		} else {
			raw, _ := json.MarshalIndent(p.History, "", "  ")
			content = append(content, Content{Type: "text", Text: string(raw)})
		}
	case models.FetchFileDiffResultPayload:
		metadata = &MCPResultMetadata{
			OriginalPayload: p,
			EventType:       eventType,
			ExecutionID:     executionID,
		}
		if p.Error != nil {
			content = append(content, Content{Type: "text", Text: *p.Error})
			isError = true
		} else if p.Diff != nil {
			content = append(content, Content{Type: "text", Text: p.Diff.DiffContent})
		} else {
			raw, _ := json.MarshalIndent(p.Diffs, "", "  ")
			content = append(content, Content{Type: "text", Text: string(raw)})
		}
	case models.RestoreFileResultPayload:
		metadata = &MCPResultMetadata{
			OriginalPayload: p,
			EventType:       eventType,
			ExecutionID:     executionID,
		}
		if p.Error != "" {
			content = append(content, Content{Type: "text", Text: p.Error})
			isError = true
		} else {
			content = append(content, Content{Type: "text", Text: fmt.Sprintf("Successfully restored %s to commit %s", p.FilePath, p.CommitHash)})
		}
	case []byte:
		content = append(content, Content{Type: "text", Text: string(p)})
	case json.RawMessage:
		content = append(content, Content{Type: "text", Text: string(p)})
	default:
		// Fallback for other payloads
		raw, _ := json.Marshal(payload)
		content = append(content, Content{Type: "text", Text: string(raw)})
	}

	result := CallToolResult{
		Content:  content,
		IsError:  isError,
		Metadata: metadata,
	}

	resultRaw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      requestID,
		Result:  resultRaw,
	}, nil
}

// WrapResult converts a g8e result payload into an MCP-formatted G8eMessage payload.
func WrapResult(requestID string, executionID string, eventType string, payload interface{}) ([]byte, error) {
	mcpResp, err := TranslateResultToMCP(requestID, executionID, eventType, payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(mcpResp)
}
