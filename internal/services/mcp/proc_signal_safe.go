// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/g8e-ai/g8e/internal/constants"
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
		return CallToolResult{}, fmt.Errorf("%w: %v", constants.ErrMCPUnmarshalArguments, err)
	}

	if req.PID <= 0 || req.Signal == "" {
		return CallToolResult{}, fmt.Errorf("%w", constants.ErrMCPProcSignalRequired)
	}

	denylist := []int{1, 2}
	for _, deniedPID := range denylist {
		if req.PID == deniedPID {
			result := ProcSignalSafeResult{
				Sent:   false,
				PID:    req.PID,
				Signal: req.Signal,
				Error:  "PID is protected by denylist",
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
		result := ProcSignalSafeResult{
			Sent:   false,
			PID:    req.PID,
			Signal: req.Signal,
			Error:  "unsupported signal",
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
		result := ProcSignalSafeResult{
			Sent:   false,
			PID:    req.PID,
			Signal: req.Signal,
			Error:  err.Error(),
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
		result := ProcSignalSafeResult{
			Sent:   false,
			PID:    req.PID,
			Signal: req.Signal,
			Error:  err.Error(),
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

	result := ProcSignalSafeResult{
		Sent:   true,
		PID:    req.PID,
		Signal: req.Signal,
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
