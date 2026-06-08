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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// ProcSignalSafeTool sends signals to processes with denylist enforcement.
type ProcSignalSafeTool struct{}

// Name returns the tool identifier.
func (t *ProcSignalSafeTool) Name() string {
	return "proc_signal_safe"
}

// Description returns a human-readable description.
func (t *ProcSignalSafeTool) Description() string {
	return "Sends signals to processes with denylist enforcement for protected PIDs."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *ProcSignalSafeTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"pid": {
				Type:        "integer",
				Description: "Process ID to signal",
			},
			"signal": {
				Type:        "string",
				Description: "Signal name (SIGTERM, SIGKILL, SIGINT)",
			},
		},
		Required: []string{"pid", "signal"},
	}
}

// Execute implements the tool logic.
func (t *ProcSignalSafeTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		PID    int    `json:"pid"`
		Signal string `json:"signal"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.PID <= 0 || req.Signal == "" {
		return CallToolResult{}, fmt.Errorf("pid and signal required")
	}

	denylist := []int{1, 2}
	for _, deniedPID := range denylist {
		if req.PID == deniedPID {
			result := map[string]interface{}{
				"sent":   false,
				"pid":    req.PID,
				"signal": req.Signal,
				"error":  "PID is protected by denylist",
			}
			resultJSON, err := json.Marshal(result)
			if err != nil {
				return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
			}
			return CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: string(resultJSON),
					},
				},
			}, nil
		}
	}

	var sig syscall.Signal
	switch strings.ToUpper(req.Signal) {
	case "SIGTERM":
		sig = syscall.SIGTERM
	case "SIGKILL":
		sig = syscall.SIGKILL
	case "SIGINT":
		sig = syscall.SIGINT
	default:
		result := map[string]interface{}{
			"sent":   false,
			"pid":    req.PID,
			"signal": req.Signal,
			"error":  "unsupported signal",
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
		}
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	process, err := os.FindProcess(req.PID)
	if err != nil {
		result := map[string]interface{}{
			"sent":   false,
			"pid":    req.PID,
			"signal": req.Signal,
			"error":  err.Error(),
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
		}
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	if err := process.Signal(sig); err != nil {
		result := map[string]interface{}{
			"sent":   false,
			"pid":    req.PID,
			"signal": req.Signal,
			"error":  err.Error(),
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
		}
		return CallToolResult{
			Content: []TextContent{
				{
					Type: "text",
					Text: string(resultJSON),
				},
			},
		}, nil
	}

	result := map[string]interface{}{
		"sent":   true,
		"pid":    req.PID,
		"signal": req.Signal,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
	}

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}
