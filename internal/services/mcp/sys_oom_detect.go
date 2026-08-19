// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/security"
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
func (t *SysOOMDetectTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"log_path": {
				Type:        "string",
				Description: "Path to the log file (default /var/log/dmesg)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *SysOOMDetectTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req SysOOMDetectRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("sys_oom_detect: %w", constants.ErrMCPUnmarshalArguments)
	}

	logPath := req.LogPath
	if logPath == "" {
		logPath = constants.PathVarLogDmesg
	}

	// Validate path to prevent directory traversal attacks
	// Use current working directory as root for relative paths
	cwd, err := os.Getwd()
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_oom_detect: %w", constants.ErrMCPGetWorkingDirectory)
	}
	safePath, err := security.ValidatePath(logPath, cwd)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_oom_detect: %w", constants.ErrPathValidation)
	}

	file, err := os.Open(safePath)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_oom_detect: %w", constants.ErrMCPOpenLogFile)
	}
	defer file.Close()

	var events []OOMEvent
	oomRegex := regexp.MustCompile(`(?i)oom-killer|killed process`)
	pidRegex := regexp.MustCompile(`(?i)pid\s*[=:]\s*(\d+)`)
	processRegex := regexp.MustCompile(`(?i)(?:process\s+|killed process\s+\d+\s+\()([^)\s]+)\)?`)
	memoryRegex := regexp.MustCompile(`(?i)(\d+)\s*MB`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return CallToolResult{}, ctx.Err()
		}

		line := scanner.Text()
		if oomRegex.MatchString(line) {
			event := OOMEvent{
				Timestamp: time.Now().Format(time.RFC3339),
			}

			if matches := pidRegex.FindStringSubmatch(line); len(matches) > 1 {
				if pid, err := strconv.Atoi(matches[1]); err == nil {
					event.PID = pid
				}
			}
			if matches := processRegex.FindStringSubmatch(line); len(matches) > 1 {
				event.Process = matches[1]
			}
			if matches := memoryRegex.FindStringSubmatch(line); len(matches) > 1 {
				if memoryMB, err := strconv.Atoi(matches[1]); err == nil {
					event.MemoryMB = memoryMB
				}
			}

			events = append(events, event)
		}
	}

	if err := scanner.Err(); err != nil {
		return CallToolResult{}, fmt.Errorf("sys_oom_detect: %w", constants.ErrMCPReadLogFile)
	}

	result := SysOOMDetectResult{
		Events: events,
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_oom_detect: %w", constants.ErrMCPMarshalOOMResult)
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
