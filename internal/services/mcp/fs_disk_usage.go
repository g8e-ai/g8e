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
	var req FSDiskUsageRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("fs_disk_usage: unmarshal arguments: %w", err)
	}

	var result FSDiskUsageResult
	var err error

	if req.Path != "" {
		result, err = getDiskUsageForPath(req.Path)
	} else {
		result, err = getAllDiskUsage()
	}

	if err != nil {
		return CallToolResult{}, fmt.Errorf("fs_disk_usage: get disk usage: %w", err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("fs_disk_usage: marshal result: %w", err)
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

func getDiskUsageForPath(path string) (FSDiskUsageResult, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return FSDiskUsageResult{}, fmt.Errorf("fs_disk_usage: statfs: %w", err)
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	used := total - free
	usedPercent := float64(used) / float64(total) * 100

	return FSDiskUsageResult{
		Path: path,
		Filesystem: &FilesystemInfo{
			Path:          path,
			TotalBytes:    total,
			UsedBytes:     used,
			FreeBytes:     free,
			AvailableBytes: available,
			UsedPercent:   usedPercent,
		},
	}, nil
}

func getAllDiskUsage() (FSDiskUsageResult, error) {
	mounts, err := parseMounts()
	if err != nil {
		return FSDiskUsageResult{}, fmt.Errorf("fs_disk_usage: parse mounts: %w", err)
	}

	var filesystems []FilesystemInfo
	for _, mount := range mounts {
		result, err := getDiskUsageForPath(mount)
		if err != nil {
			continue
		}
		if result.Filesystem != nil {
			filesystems = append(filesystems, *result.Filesystem)
		}
	}

	return FSDiskUsageResult{
		Filesystems: filesystems,
		Count:       len(filesystems),
	}, nil
}

func parseMounts() ([]string, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("fs_disk_usage: read /proc/mounts: %w", err)
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
