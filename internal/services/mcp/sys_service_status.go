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
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// SysServiceStatusTool checks systemd service status.
type SysServiceStatusTool struct {
	executor commandExecutor
}

// Name returns the tool identifier.
func (t *SysServiceStatusTool) Name() string {
	return "sys_service_status"
}

// Description returns a human-readable description.
func (t *SysServiceStatusTool) Description() string {
	return "Checks systemd service status (operator, gateway, etc.)."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *SysServiceStatusTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"service_name": {
				Type:        "string",
				Description: "Name of the systemd service (e.g., g8e-operator, g8e-gateway)",
			},
		},
		Required: []string{"service_name"},
	}
}

// Execute implements the tool logic.
func (t *SysServiceStatusTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req SysServiceStatusRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("sys_service_status: unmarshal arguments: %w: %w", constants.ErrMCPUnmarshalArguments, err)
	}

	if req.ServiceName == "" {
		return CallToolResult{}, constants.ErrMCPServiceNameRequired
	}

	if ctx.Err() != nil {
		return CallToolResult{}, ctx.Err()
	}

	executor := t.executor
	if executor == nil {
		executor = &realCommandExecutor{}
	}

	result, err := getServiceStatus(ctx, req.ServiceName, executor)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_service_status: get service status: %w", err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_service_status: marshal result: %w: %w", constants.ErrMCPMarshalResult, err)
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

func getServiceStatus(ctx context.Context, serviceName string, executor commandExecutor) (SysServiceStatusResult, error) {
	if ctx.Err() != nil {
		return SysServiceStatusResult{}, ctx.Err()
	}

	// serviceName is passed as a separate argument to executor.CombinedOutput to satisfy CodeQL command-injection rule.
	// This prevents shell injection by avoiding shell interpretation.
	output, err := executor.CombinedOutput(ctx, "systemctl", "show", serviceName, "--no-pager")
	if err != nil {
		return SysServiceStatusResult{
			ServiceName: serviceName,
			Error:       string(output),
		}, nil
	}

	properties := make(map[string]string)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			properties[parts[0]] = parts[1]
		}
	}

	result := SysServiceStatusResult{
		ServiceName: serviceName,
		LoadState:   getProp(properties, "LoadState"),
		ActiveState: getProp(properties, "ActiveState"),
		SubState:    getProp(properties, "SubState"),
		Enabled:     getProp(properties, "UnitFileState") == "enabled",
		Description: getProp(properties, "Description"),
		MainPID:     getProp(properties, "MainPID"),
		ExecStart:   getProp(properties, "ExecMainStartTimestamp"),
	}

	return result, nil
}

func getProp(props map[string]string, key string) string {
	if val, ok := props[key]; ok {
		return val
	}
	return "unknown"
}
