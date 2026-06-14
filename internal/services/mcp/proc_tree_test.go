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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcTreeTool_Metadata(t *testing.T) {
	tool := &ProcTreeTool{}
	require.Equal(t, "proc_tree", tool.Name())
	require.Contains(t, tool.Description(), "process tree")
	require.NotNil(t, tool.InputSchema())
}

func TestProcTreeTool_Execute_Errors(t *testing.T) {
	tool := &ProcTreeTool{}

	t.Run("invalid json", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid arguments")
	})

	t.Run("proc directory not found", func(t *testing.T) {
		oldProcDir := procDirectory
		procDirectory = "/nonexistent-proc-dir-12345"
		defer func() { procDirectory = oldProcDir }()

		_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to build process tree")
	})
}

func TestBuildProcessTree(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup mock /proc
	// PID 1: init
	// PID 10: parent (child of 1)
	// PID 20: child1 (child of 10)
	// PID 30: child2 (child of 10)
	// PID 40: grandchild (child of 20)

	setupProc := func(pid int, name string, ppid int) {
		pidDir := filepath.Join(tmpDir, fmt.Sprintf("%d", pid))
		require.NoError(t, os.MkdirAll(pidDir, 0755))

		// stat format: pid (name) state ppid ...
		statContent := fmt.Sprintf("%d (%s) S %d 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0", pid, name, ppid)
		require.NoError(t, os.WriteFile(filepath.Join(pidDir, "stat"), []byte(statContent), 0644))
	}

	setupProc(1, "systemd", 0)
	setupProc(10, "parent", 1)
	setupProc(20, "child1", 10)
	setupProc(30, "child2", 10)
	setupProc(40, "grandchild", 20)

	oldProcDir := procDirectory
	procDirectory = tmpDir
	defer func() { procDirectory = oldProcDir }()

	t.Run("full tree from init", func(t *testing.T) {
		result, err := buildProcessTree(context.Background(), 1, 10)
		require.NoError(t, err)
		require.Equal(t, 1, result.RootPID)
		require.Equal(t, "systemd", result.Tree.Name)
		require.Equal(t, 1, result.Tree.PID)

		// systemd -> parent
		require.Len(t, result.Tree.Children, 1)
		parent := result.Tree.Children[0]
		require.Equal(t, "parent", parent.Name)
		require.Equal(t, 10, parent.PID)

		// parent -> child1, child2
		require.Len(t, parent.Children, 2)

		var c1, c2 *processNode
		for i := range parent.Children {
			switch parent.Children[i].PID {
			case 20:
				c1 = &parent.Children[i]
			case 30:
				c2 = &parent.Children[i]
			}
		}
		require.NotNil(t, c1)
		require.NotNil(t, c2)
		require.Equal(t, "child1", c1.Name)
		require.Equal(t, "child2", c2.Name)

		// child1 -> grandchild
		require.Len(t, c1.Children, 1)
		require.Equal(t, "grandchild", c1.Children[0].Name)
		require.Equal(t, 40, c1.Children[0].PID)

		// child2 has no children
		require.Empty(t, c2.Children)
	})

	t.Run("partial tree from parent", func(t *testing.T) {
		result, err := buildProcessTree(context.Background(), 10, 10)
		require.NoError(t, err)
		require.Equal(t, 10, result.RootPID)
		require.Equal(t, "parent", result.Tree.Name)
	})

	t.Run("max depth limit", func(t *testing.T) {
		// systemd (0) -> parent (1) -> child1 (2) -> grandchild (3)
		// max_depth 1 means only systemd and parent
		result, err := buildProcessTree(context.Background(), 1, 1)
		require.NoError(t, err)
		require.Equal(t, 1, result.RootPID)
		require.Len(t, result.Tree.Children, 1)            // parent
		require.Empty(t, result.Tree.Children[0].Children) // child1 excluded
	})

	t.Run("process not found", func(t *testing.T) {
		result, err := buildProcessTree(context.Background(), 999, 10)
		require.NoError(t, err)
		require.Equal(t, 999, result.RootPID)
		require.Contains(t, result.Error, "process 999 not found")
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := buildProcessTree(ctx, 1, 10)
		require.Error(t, err)
		require.Equal(t, context.Canceled, err)
	})
}

func TestBuildProcessTree_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	oldProcDir := procDirectory
	procDirectory = tmpDir
	defer func() { procDirectory = oldProcDir }()

	t.Run("invalid pid directory name", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "abc"), 0755))
		result, err := buildProcessTree(context.Background(), 1, 10)
		require.NoError(t, err)
		require.Contains(t, result.Error, "not found")
	})

	t.Run("missing stat file", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "100"), 0755))
		result, err := buildProcessTree(context.Background(), 1, 10)
		require.NoError(t, err)
		require.Contains(t, result.Error, "not found")
	})

	t.Run("malformed stat file", func(t *testing.T) {
		pidDir := filepath.Join(tmpDir, "100")
		require.NoError(t, os.MkdirAll(pidDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(pidDir, "stat"), []byte("100"), 0644)) // too few fields

		result, err := buildProcessTree(context.Background(), 1, 10)
		require.NoError(t, err)
		require.Contains(t, result.Error, "not found")
	})
}
