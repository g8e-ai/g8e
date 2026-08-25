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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const (
	procPageSizeBytes = 4096
)

// ProcMetricTopTool parses /proc to extract top resource-consuming processes.
type ProcMetricTopTool struct{}

// Name returns the tool identifier.
func (t *ProcMetricTopTool) Name() string {
	return "proc_metric_top"
}

// Description returns a human-readable description.
func (t *ProcMetricTopTool) Description() string {
	return "Parses /proc to extract top resource-consuming processes by CPU and memory."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *ProcMetricTopTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"limit": {
				Type:        "integer",
				Description: "Maximum number of processes to return (default 10)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *ProcMetricTopTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req ProcMetricTopRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("proc_metric_top: invalid arguments: %w", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = constants.DefaultProcessLimit
	}

	entries, err := os.ReadDir(constants.PathProc)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("proc_metric_top: failed to read %s: %w", constants.PathProc, err)
	}

	processes := make([]ProcessInfo, 0, limit)

	for _, entry := range entries {
		if ctx.Err() != nil {
			return CallToolResult{}, ctx.Err()
		}

		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		statPath := filepath.Join(constants.PathProc, entry.Name(), "stat")
		statBytes, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		statFields := strings.Fields(string(statBytes))
		if len(statFields) < 24 {
			continue
		}

		name := statFields[1]
		if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") {
			name = name[1 : len(name)-1]
		}

		utime, err := strconv.ParseFloat(statFields[13], 64)
		if err != nil {
			continue
		}
		stime, err := strconv.ParseFloat(statFields[14], 64)
		if err != nil {
			continue
		}
		totalTime := utime + stime

		rss, err := strconv.ParseInt(statFields[23], 10, 64)
		if err != nil {
			continue
		}
		memoryMB := float64(rss) * float64(procPageSizeBytes) / (1024 * 1024)

		processes = append(processes, ProcessInfo{
			PID:        pid,
			Name:       name,
			CPUPercent: totalTime,
			MemoryMB:   memoryMB,
			User:       string(constants.SystemHealthUnknown),
			Command:    name,
		})
	}

	if len(processes) > limit {
		processes = processes[:limit]
	}

	result := ProcMetricTopResult{
		Processes: processes,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("proc_metric_top: failed to marshal result: %w", err)
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
