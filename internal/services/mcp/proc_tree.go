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
)

// ProcTreeTool provides parent-child process relationships.
type ProcTreeTool struct{}

// Name returns the tool identifier.
func (t *ProcTreeTool) Name() string {
	return "proc_tree"
}

// Description returns a human-readable description.
func (t *ProcTreeTool) Description() string {
	return "Provides parent-child process relationships and process tree."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *ProcTreeTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pid": map[string]interface{}{
				"type":        "integer",
				"description": "Process ID to start tree from (default 1 for init)",
			},
			"max_depth": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum depth to traverse (default 10)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *ProcTreeTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		PID      int `json:"pid,omitempty"`
		MaxDepth int `json:"max_depth,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	pid := req.PID
	if pid <= 0 {
		pid = 1
	}

	maxDepth := req.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 10
	}

	tree, err := buildProcessTree(pid, maxDepth)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to build process tree: %w", err)
	}

	resultJSON, err := json.Marshal(tree)
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

type processNode struct {
	PID      int           `json:"pid"`
	Name     string        `json:"name"`
	PPID     int           `json:"ppid"`
	Children []processNode `json:"children,omitempty"`
}

func buildProcessTree(rootPID, maxDepth int) (map[string]interface{}, error) {
	procDir := "/proc"
	processes := make(map[int]*processNode)

	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
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
		if len(statFields) < 4 {
			continue
		}

		name := statFields[1]
		if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") {
			name = name[1 : len(name)-1]
		}

		ppid, _ := strconv.Atoi(statFields[3])

		processes[pid] = &processNode{
			PID:  pid,
			Name: name,
			PPID: ppid,
		}
	}

	rootNode := processes[rootPID]
	if rootNode == nil {
		return map[string]interface{}{
			"error": fmt.Sprintf("process %d not found", rootPID),
		}, nil
	}

	tree := buildTreeRecursive(rootNode, processes, 0, maxDepth)

	return map[string]interface{}{
		"root_pid": rootPID,
		"tree":     tree,
	}, nil
}

func buildTreeRecursive(node *processNode, processes map[int]*processNode, currentDepth, maxDepth int) processNode {
	if currentDepth >= maxDepth {
		return *node
	}

	for _, proc := range processes {
		if proc.PPID == node.PID {
			child := buildTreeRecursive(proc, processes, currentDepth+1, maxDepth)
			node.Children = append(node.Children, child)
		}
	}

	return *node
}
