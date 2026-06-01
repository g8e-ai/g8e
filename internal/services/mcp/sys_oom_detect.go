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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"
)

// SysOOMDetectTool scans system logs for OOM killer events.
type SysOOMDetectTool struct{}

// Name returns the tool identifier.
func (t *SysOOMDetectTool) Name() string {
	return "sys_oom_detect"
}

// Description returns a human-readable description.
func (t *SysOOMDetectTool) Description() string {
	return "Scans system logs for OOM (Out of Memory) killer events."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *SysOOMDetectTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"log_path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the log file (default /var/log/dmesg)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *SysOOMDetectTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		LogPath string `json:"log_path,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	logPath := req.LogPath
	if logPath == "" {
		logPath = "/var/log/dmesg"
	}

	file, err := os.Open(logPath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	var events []map[string]interface{}
	oomRegex := regexp.MustCompile(`(?i)oom-killer|killed process`)
	pidRegex := regexp.MustCompile(`pid\s*=\s*(\d+)`)
	processRegex := regexp.MustCompile(`process\s+(\S+)`)
	memoryRegex := regexp.MustCompile(`(\d+)\s*MB`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return CallToolResult{}, ctx.Err()
		}

		line := scanner.Text()
		if oomRegex.MatchString(line) {
			event := map[string]interface{}{
				"timestamp": time.Now().Format(time.RFC3339),
			}

			if matches := pidRegex.FindStringSubmatch(line); len(matches) > 1 {
				if pid, err := strconv.Atoi(matches[1]); err == nil {
					event["pid"] = pid
				}
			}
			if matches := processRegex.FindStringSubmatch(line); len(matches) > 1 {
				event["process"] = matches[1]
			}
			if matches := memoryRegex.FindStringSubmatch(line); len(matches) > 1 {
				if memoryMB, err := strconv.Atoi(matches[1]); err == nil {
					event["memory_mb"] = memoryMB
				}
			}

			events = append(events, event)
		}
	}

	if err := scanner.Err(); err != nil {
		return CallToolResult{}, fmt.Errorf("error reading log file: %w", err)
	}

	result := map[string]interface{}{
		"events": events,
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
