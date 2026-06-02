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

// SysContainerStatusTool checks Docker/podman container health.
type SysContainerStatusTool struct{}

// Name returns the tool identifier.
func (t *SysContainerStatusTool) Name() string {
	return "sys_container_status"
}

// Description returns a human-readable description.
func (t *SysContainerStatusTool) Description() string {
	return "Checks Docker/podman container health status."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *SysContainerStatusTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"container_name": map[string]interface{}{
				"type":        "string",
				"description": "Name or ID of the container",
			},
			"runtime": map[string]interface{}{
				"type":        "string",
				"description": "Container runtime (docker or podman, auto-detect if empty)",
				"enum":        []string{"docker", "podman"},
			},
		},
		"required": []string{"container_name"},
	}
}

// Execute implements the tool logic.
func (t *SysContainerStatusTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		ContainerName string `json:"container_name"`
		Runtime       string `json:"runtime,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.ContainerName == "" {
		return CallToolResult{}, fmt.Errorf("container_name required")
	}

	runtime := req.Runtime
	if runtime == "" {
		runtime = detectContainerRuntime()
	}

	result, err := getContainerStatus(req.ContainerName, runtime)
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

func detectContainerRuntime() string {
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	return "docker"
}

func getContainerStatus(containerName, runtime string) (map[string]interface{}, error) {
	var cmd *exec.Cmd
	if runtime == "docker" {
		cmd = exec.Command(runtime, "inspect", "--format", "{{json .}}", containerName)
	} else {
		cmd = exec.Command(runtime, "inspect", "--format", "json", containerName)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return map[string]interface{}{
			"container_name": containerName,
			"runtime":        runtime,
			"error":          string(output),
		}, nil
	}

	var inspectData []map[string]interface{}
	if err := json.Unmarshal(output, &inspectData); err != nil {
		return map[string]interface{}{
			"container_name": containerName,
			"runtime":        runtime,
			"error":          fmt.Sprintf("failed to parse inspect output: %v", err),
		}, nil
	}

	if len(inspectData) == 0 {
		return map[string]interface{}{
			"container_name": containerName,
			"runtime":        runtime,
			"error":          "container not found",
		}, nil
	}

	container := inspectData[0]
	state, _ := container["State"].(map[string]interface{})

	result := map[string]interface{}{
		"container_name": containerName,
		"runtime":        runtime,
		"status":         getNested(state, "Status"),
		"running":        getNested(state, "Running") == true,
		"paused":         getNested(state, "Paused") == true,
		"restarting":     getNested(state, "Restarting") == true,
		"health":         getNested(state, "Health", "Status"),
		"started_at":     getNested(state, "StartedAt"),
		"image":          getNested(container, "Config", "Image"),
	}

	return result, nil
}

func getNested(m map[string]interface{}, keys ...string) interface{} {
	current := m
	for _, key := range keys {
		if val, ok := current[key]; ok {
			if next, ok := val.(map[string]interface{}); ok {
				current = next
			} else {
				return val
			}
		} else {
			return nil
		}
	}
	return current
}
