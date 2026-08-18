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
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// SysInfoTool provides system information including hostname, OS version, kernel, uptime, and load average.
type SysInfoTool struct{}

// Name returns the tool identifier.
func (t *SysInfoTool) Name() string {
	return "sys_info"
}

// Description returns a human-readable description.
func (t *SysInfoTool) Description() string {
	return "Provides system information including hostname, OS version, kernel, uptime, and load average."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *SysInfoTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type:       "object",
		Properties: make(map[string]*PropertySchema),
	}
}

// Execute implements the tool logic.
func (t *SysInfoTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req SysInfoRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("sys_info: unmarshal arguments: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_info: get hostname: %w", err)
	}

	if ctx.Err() != nil {
		return CallToolResult{}, ctx.Err()
	}

	osInfo := OSInfo{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Kernel:      getKernelVersion(ctx),
		OSVersion:   getOSVersion(ctx),
		Uptime:      getUptime(ctx),
		LoadAverage: getLoadAverage(ctx),
	}

	result := SysInfoResult{
		Hostname: hostname,
		OS:       osInfo,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_info: marshal result: %w", err)
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

// getKernelVersion returns the kernel version from /proc/version or uname.
func getKernelVersion(ctx context.Context) string {
	if ctx.Err() != nil {
		return "unknown"
	}

	data, err := os.ReadFile(constants.PathProcVersion)
	if err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			return strings.Join(fields[2:], " ")
		}
	}

	cmd := exec.CommandContext(ctx, "uname", "-r")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}

	return "unknown"
}

// getOSVersion returns the OS version from /etc/os-release.
func getOSVersion(ctx context.Context) string {
	if ctx.Err() != nil {
		return "unknown"
	}

	data, err := os.ReadFile(constants.PathEtcOSRelease)
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			value := strings.TrimPrefix(line, "PRETTY_NAME=")
			value = strings.Trim(value, "\"")
			return value
		}
	}
	return "unknown"
}

// getUptime returns system uptime in human-readable format.
func getUptime(ctx context.Context) string {
	if ctx.Err() != nil {
		return "unknown"
	}

	data, err := os.ReadFile(constants.PathProcUptime)
	if err != nil {
		return "unknown"
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return "unknown"
	}

	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "unknown"
	}

	duration := time.Duration(seconds) * time.Second
	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60
	return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
}

// getLoadAverage returns the 1, 5, and 15 minute load averages.
func getLoadAverage(ctx context.Context) string {
	if ctx.Err() != nil {
		return "unknown"
	}

	data, err := os.ReadFile(constants.PathProcLoadAvg)
	if err != nil {
		return "unknown"
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return "unknown"
	}

	return fmt.Sprintf("%s %s %s", fields[0], fields[1], fields[2])
}
