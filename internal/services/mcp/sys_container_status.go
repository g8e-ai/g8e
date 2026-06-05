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
)

// SysContainerStatusTool checks container health status (podman).
type SysContainerStatusTool struct{}

// Name returns the tool identifier.
func (t *SysContainerStatusTool) Name() string {
	return "sys_container_status"
}

// Description returns a human-readable description.
func (t *SysContainerStatusTool) Description() string {
	return "Checks container health status (podman)."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *SysContainerStatusTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"container_name": map[string]interface{}{
				"type":        "string",
				"description": "Name or ID of the container to check",
			},
		},
		"required": []string{"container_name"},
	}
}

// Execute implements the tool logic.
func (t *SysContainerStatusTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		ContainerName string `json:"container_name"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.ContainerName == "" {
		return CallToolResult{}, fmt.Errorf("container_name required")
	}

	result, err := getContainerStatus(req.ContainerName)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to get container status: %w", err)
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

func getContainerStatus(containerName string) (map[string]interface{}, error) {
	cmd := exec.Command("podman", "inspect", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]interface{}{
			"container_name": containerName,
			"error":          string(output),
		}, nil
	}

	var inspectData []map[string]interface{}
	if err := json.Unmarshal(output, &inspectData); err != nil {
		return map[string]interface{}{
			"container_name": containerName,
			"error":          fmt.Sprintf("failed to parse inspect output: %v", err),
		}, nil
	}

	if len(inspectData) == 0 {
		return map[string]interface{}{
			"container_name": containerName,
			"error":          "container not found",
		}, nil
	}

	container := inspectData[0]
	state := getNestedMap(container, "State")

	result := map[string]interface{}{
		"container_name": containerName,
		"status":         getString(state, "Status"),
		"running":        getBool(state, "Running"),
		"paused":         getBool(state, "Paused"),
		"restarting":     getBool(state, "Restarting"),
		"pid":            getInt(state, "Pid"),
		"started_at":     getString(state, "StartedAt"),
		"finished_at":    getString(state, "FinishedAt"),
		"exit_code":      getInt(state, "ExitCode"),
		"image":          getString(container, "Image"),
		"created":        getString(container, "Created"),
	}

	return result, nil
}

func getNestedMap(m map[string]interface{}, key string) map[string]interface{} {
	if val, ok := m[key]; ok {
		if nested, ok := val.(map[string]interface{}); ok {
			return nested
		}
	}
	return make(map[string]interface{})
}

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return "unknown"
}

func getBool(m map[string]interface{}, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

func getInt(m map[string]interface{}, key string) int64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		}
	}
	return 0
}
