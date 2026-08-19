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
)

const (
	defaultRootPID  = 1
	defaultMaxDepth = 10
	minStatFields   = 4
)

var procDirectory = "/proc"

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
func (t *ProcTreeTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"pid": {
				Type:        "integer",
				Description: "Process ID to start tree from (default 1 for init)",
			},
			"max_depth": {
				Type:        "integer",
				Description: "Maximum depth to traverse (default 10)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *ProcTreeTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req ProcTreeRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	pid := req.PID
	if pid <= 0 {
		pid = defaultRootPID
	}

	maxDepth := req.MaxDepth
	if maxDepth <= 0 {
		maxDepth = defaultMaxDepth
	}

	tree, err := buildProcessTree(ctx, pid, maxDepth)
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

type processTreeResult struct {
	RootPID int         `json:"root_pid"`
	Tree    processNode `json:"tree"`
	Error   string      `json:"error,omitempty"`
}

func buildProcessTree(ctx context.Context, rootPID, maxDepth int) (*processTreeResult, error) {
	entries, err := os.ReadDir(procDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", procDirectory, err)
	}

	processes := make(map[int]*processNode)

	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		statPath := filepath.Join(procDirectory, entry.Name(), "stat")
		statBytes, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		statFields := strings.Fields(string(statBytes))
		if len(statFields) < minStatFields {
			continue
		}

		name := statFields[1]
		if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") {
			name = name[1 : len(name)-1]
		}

		ppid, err := strconv.Atoi(statFields[3])
		if err != nil {
			continue
		}

		processes[pid] = &processNode{
			PID:  pid,
			Name: name,
			PPID: ppid,
		}
	}

	rootNode := processes[rootPID]
	if rootNode == nil {
		return &processTreeResult{
			RootPID: rootPID,
			Error:   fmt.Sprintf("process %d not found", rootPID),
		}, nil
	}

	tree := buildTreeRecursive(rootNode, processes, 0, maxDepth)

	return &processTreeResult{
		RootPID: rootPID,
		Tree:    tree,
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
