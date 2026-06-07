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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
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
func (t *ProcMetricTopTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of processes to return (default 10)",
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
		limit = defaultProcessLimit
	}

	procDir := "/proc"
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("proc_metric_top: failed to read /proc: %w", err)
	}

	var processes []ProcessInfo

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

		statPath := filepath.Join(procDir, entry.Name(), "stat")
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
		memoryMB := float64(rss) * 4096 / (1024 * 1024)

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
