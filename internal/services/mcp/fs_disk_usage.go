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

//go:build !windows

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// FSDiskUsageTool provides df-style free space reporting.
type FSDiskUsageTool struct{}

// Name returns the tool identifier.
func (t *FSDiskUsageTool) Name() string {
	return "fs_disk_usage"
}

// Description returns a human-readable description.
func (t *FSDiskUsageTool) Description() string {
	return "Provides df-style free space reporting for mounted filesystems."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *FSDiskUsageTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to check disk usage for (default all mounted filesystems)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *FSDiskUsageTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		Path string `json:"path,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	var result map[string]interface{}
	var err error

	if req.Path != "" {
		result, err = getDiskUsageForPath(req.Path)
	} else {
		result, err = getAllDiskUsage()
	}

	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to get disk usage: %w", err)
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

func getDiskUsageForPath(path string) (map[string]interface{}, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	used := total - free
	usedPercent := float64(used) / float64(total) * 100

	return map[string]interface{}{
		"path": path,
		"filesystem": map[string]interface{}{
			"total_bytes":     total,
			"used_bytes":      used,
			"free_bytes":      free,
			"available_bytes": available,
			"used_percent":    usedPercent,
		},
	}, nil
}

func getAllDiskUsage() (map[string]interface{}, error) {
	mounts, err := parseMounts()
	if err != nil {
		return nil, err
	}

	var filesystems []map[string]interface{}
	for _, mount := range mounts {
		usage, err := getDiskUsageForPath(mount)
		if err != nil {
			continue
		}
		filesystems = append(filesystems, usage)
	}

	return map[string]interface{}{
		"filesystems": filesystems,
		"count":       len(filesystems),
	}, nil
}

func parseMounts() ([]string, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var mounts []string

	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			mountPoint := fields[1]
			if strings.HasPrefix(mountPoint, "/") && !strings.HasPrefix(mountPoint, "/proc") && !strings.HasPrefix(mountPoint, "/sys") && !strings.HasPrefix(mountPoint, "/dev") {
				mounts = append(mounts, mountPoint)
			}
		}
	}

	return mounts, nil
}
