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
	"os/exec"
	"strings"
)

// SysServiceStatusTool checks systemd service status.
type SysServiceStatusTool struct{}

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
	var req struct {
		ServiceName string `json:"service_name"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.ServiceName == "" {
		return CallToolResult{}, fmt.Errorf("service_name required")
	}

	result, err := getServiceStatus(req.ServiceName)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to get service status: %w", err)
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

func getServiceStatus(serviceName string) (map[string]interface{}, error) {
	cmd := exec.Command("systemctl", "show", serviceName, "--no-pager")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]interface{}{
			"service_name": serviceName,
			"error":        string(output),
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

	result := map[string]interface{}{
		"service_name": serviceName,
		"load_state":   getProp(properties, "LoadState"),
		"active_state": getProp(properties, "ActiveState"),
		"sub_state":    getProp(properties, "SubState"),
		"enabled":      getProp(properties, "UnitFileState") == "enabled",
		"description":  getProp(properties, "Description"),
		"main_pid":     getProp(properties, "MainPID"),
		"exec_start":   getProp(properties, "ExecMainStartTimestamp"),
	}

	return result, nil
}

func getProp(props map[string]string, key string) string {
	if val, ok := props[key]; ok {
		return val
	}
	return "unknown"
}
