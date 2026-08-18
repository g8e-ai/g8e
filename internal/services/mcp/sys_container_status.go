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
	"os/exec"

	"github.com/g8e-ai/g8e/internal/constants"
)

type commandExecutor interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

type realCommandExecutor struct{}

func (r *realCommandExecutor) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

type containerStatusResult struct {
	ContainerName string `json:"container_name"`
	Status        string `json:"status"`
	Running       bool   `json:"running"`
	Paused        bool   `json:"paused"`
	Restarting    bool   `json:"restarting"`
	PID           int64  `json:"pid"`
	StartedAt     string `json:"started_at"`
	FinishedAt    string `json:"finished_at"`
	ExitCode      int64  `json:"exit_code"`
	Image         string `json:"image"`
	Created       string `json:"created"`
	Error         string `json:"error,omitempty"`
}

type containerInspectData struct {
	State   containerInspectState `json:"State"`
	Image   string                `json:"Image"`
	Created string                `json:"Created"`
}

type containerInspectState struct {
	Status     string `json:"Status"`
	Running    bool   `json:"Running"`
	Paused     bool   `json:"Paused"`
	Restarting bool   `json:"Restarting"`
	Pid        int64  `json:"Pid"`
	StartedAt  string `json:"StartedAt"`
	FinishedAt string `json:"FinishedAt"`
	ExitCode   int64  `json:"ExitCode"`
}

// SysContainerStatusTool checks container health status (podman).
type SysContainerStatusTool struct {
	executor commandExecutor
}

// Name returns the tool identifier.
func (t *SysContainerStatusTool) Name() string {
	return "sys_container_status"
}

// Description returns a human-readable description.
func (t *SysContainerStatusTool) Description() string {
	return "Checks container health status (podman)."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *SysContainerStatusTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"container_name": {
				Type:        "string",
				Description: "Name or ID of the container to check",
			},
		},
		Required: []string{"container_name"},
	}
}

// Execute implements the tool logic.
func (t *SysContainerStatusTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		ContainerName string `json:"container_name"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("sys_container_status: unmarshal arguments: %w: %w", constants.ErrMCPUnmarshalArguments, err)
	}

	if req.ContainerName == "" {
		return CallToolResult{}, constants.ErrMCPContainerNameRequired
	}

	executor := t.executor
	if executor == nil {
		executor = &realCommandExecutor{}
	}

	result, err := getContainerStatus(ctx, req.ContainerName, executor)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_container_status: get container status: %w", err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_container_status: marshal result: %w: %w", constants.ErrMCPMarshalResult, err)
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

func getContainerStatus(ctx context.Context, containerName string, executor commandExecutor) (containerStatusResult, error) {
	if ctx.Err() != nil {
		return containerStatusResult{}, ctx.Err()
	}

	output, err := executor.CombinedOutput(ctx, "podman", "inspect", containerName)
	if err != nil {
		return containerStatusResult{
			ContainerName: containerName,
			Error:         string(output),
		}, nil
	}

	var inspectData []containerInspectData
	if err := json.Unmarshal(output, &inspectData); err != nil {
		return containerStatusResult{
			ContainerName: containerName,
			Error:         fmt.Sprintf("failed to parse inspect output: %v", err),
		}, nil
	}

	if len(inspectData) == 0 {
		return containerStatusResult{
			ContainerName: containerName,
			Error:         "container not found",
		}, nil
	}

	container := inspectData[0]
	state := container.State

	return containerStatusResult{
		ContainerName: containerName,
		Status:        orUnknown(state.Status),
		Running:       state.Running,
		Paused:        state.Paused,
		Restarting:    state.Restarting,
		PID:           state.Pid,
		StartedAt:     orUnknown(state.StartedAt),
		FinishedAt:    orUnknown(state.FinishedAt),
		ExitCode:      state.ExitCode,
		Image:         orUnknown(container.Image),
		Created:       orUnknown(container.Created),
	}, nil
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
