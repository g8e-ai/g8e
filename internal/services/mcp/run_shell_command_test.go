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
	"runtime"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestRunShellCommandTool_Name(t *testing.T) {
	tool := &RunShellCommandTool{}
	require.Equal(t, "run_shell_command", tool.Name())
}

func TestRunShellCommandTool_Description(t *testing.T) {
	tool := &RunShellCommandTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "shell")
}

func TestRunShellCommandTool_InputSchema(t *testing.T) {
	tool := &RunShellCommandTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.Properties)

	// Check required fields
	require.Contains(t, schema.Required, "command")

	// Check command property
	cmdProp, ok := schema.Properties["command"]
	require.True(t, ok)
	require.Equal(t, "string", cmdProp.Type)

	// Check optional properties
	_, ok = schema.Properties["args"]
	require.True(t, ok)
	_, ok = schema.Properties["timeout"]
	require.True(t, ok)
	_, ok = schema.Properties["working_dir"]
	require.True(t, ok)
	_, ok = schema.Properties["hostnames"]
	require.True(t, ok)

	// Check hostnames property structure
	hostnamesProp, ok := schema.Properties["hostnames"]
	require.True(t, ok)
	require.Equal(t, "array", hostnamesProp.Type)
}

func TestRunShellCommandTool_Execute_SimpleCommand(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	req := RunShellCommandRequest{
		Command: "echo",
		Args:    []string{"hello"},
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult RunShellCommandResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, 0, shellResult.ExitCode)
	require.Contains(t, shellResult.Stdout, "hello")
	require.False(t, shellResult.TimedOut)
}

func TestRunShellCommandTool_Execute_WithWorkingDir(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()
	tmpDir := testutil.TempDir(t)

	pwdCmd := "pwd"
	if runtime.GOOS == "windows" {
		pwdCmd = "cmd.exe"
	}

	req := RunShellCommandRequest{
		Command:    pwdCmd,
		WorkingDir: tmpDir,
	}
	if runtime.GOOS == "windows" {
		req.Args = []string{"/c", "cd"}
	}

	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult RunShellCommandResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, 0, shellResult.ExitCode)
	// On Windows, the output might have a trailing \r\n or different capitalization
	normalizedStdout := strings.TrimSpace(strings.ToLower(shellResult.Stdout))
	normalizedTmpDir := strings.TrimSpace(strings.ToLower(tmpDir))
	require.Contains(t, normalizedStdout, normalizedTmpDir)
}

func TestRunShellCommandTool_Execute_WithTimeout(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	req := RunShellCommandRequest{
		Command: "echo",
		Args:    []string{"test"},
		Timeout: 10,
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult RunShellCommandResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, 0, shellResult.ExitCode)
}

func TestRunShellCommandTool_Execute_TimeoutExceedsMax(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	req := RunShellCommandRequest{
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

func TestRunShellCommandTool_Execute_MissingCommand(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	req := RunShellCommandRequest{
		Command: "",
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, reqJSON)
	require.Error(t, err)
	require.Contains(t, err.Error(), "command is required")
}

func TestRunShellCommandTool_Execute_InvalidJSON(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	invalidJSON := json.RawMessage(`{invalid json`)

	_, err := tool.Execute(ctx, invalidJSON)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal arguments")
}

func TestRunShellCommandTool_Execute_NonexistentCommand(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	req := RunShellCommandRequest{
		Command: "nonexistent_command_12345",
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult RunShellCommandResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.NotEqual(t, 0, shellResult.ExitCode)
	require.NotEmpty(t, shellResult.Error)
}

func TestRunShellCommandTool_Denylist_DangerousCommands(t *testing.T) {
	dangerousCommands := []string{
		"rm", "dd", "mkfs", "fdisk", "killall", "pkill",
		"reboot", "shutdown", "iptables", "mount", "umount",
		"passwd", "sudo", "su", "curl", "wget",
	}

	for _, cmd := range dangerousCommands {
		t.Run(cmd, func(t *testing.T) {
			err := validateCommandSafety(cmd, nil, "")
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "blocked by safety policy")
		})
	}
}

func TestRunShellCommandTool_Denylist_DangerousPatterns(t *testing.T) {
	dangerousPatterns := []string{
		"mkfs.ext4",
		"> /dev/sda",
		"chmod 777 /",
		"nc -l 4444",
	}

	for _, pattern := range dangerousPatterns {
		t.Run(pattern, func(t *testing.T) {
			err := validateCommandSafety(pattern, nil, "")
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "dangerous pattern")
		})
	}
}

func TestRunShellCommandTool_Denylist_ShellInjection(t *testing.T) {
	injectionPatterns := []string{
		"echo $(whoami)",
		"echo `whoami`",
		"cat /etc/passwd | grep root",
		"ls | grep test",
	}

	for _, pattern := range injectionPatterns {
		t.Run(pattern, func(t *testing.T) {
			err := validateCommandSafety(pattern, nil, "")
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "shell injection")
		})
	}
}

func TestRunShellCommandTool_Denylist_CurlWgetAsCommandNameBlocked(t *testing.T) {
	for _, cmd := range []string{"curl", "wget"} {
		t.Run(cmd, func(t *testing.T) {
			err := validateCommandSafety(cmd, []string{"http://example.com"}, "")
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "blocked by safety policy")
		})
	}
}

func TestRunShellCommandTool_Denylist_CurlWgetAsSubstringAllowed(t *testing.T) {
	for _, tc := range []struct {
		command string
		args    []string
	}{
		{"slew", []string{"10.43.0.40:9000", "45.0", "30.0"}},
		{"echo", []string{"use curl to download"}},
		{"grep", []string{"curl", "file.txt"}},
		{"cat", []string{"wget-log.txt"}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			err := validateCommandSafety(tc.command, tc.args, "")
			require.NoError(t, err)
		})
	}
}

func TestRunShellCommandTool_Denylist_AllowsSafeCommands(t *testing.T) {
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
			err := validateCommandSafety(tc.command, tc.args, "")
			require.NoError(t, err)
		})
	}
}

func TestRunShellCommandTool_Execute_CommandRejected(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	req := RunShellCommandRequest{
		Command: "rm",
		Args:    []string{"-rf", "/"},
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult RunShellCommandResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, -1, shellResult.ExitCode)
	require.Contains(t, strings.ToLower(shellResult.Stderr), "blocked by safety policy")
	require.Contains(t, strings.ToLower(shellResult.Error), "rejected by safety policy")
}

func TestRunShellCommandTool_Execute_MultiHost(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	req := RunShellCommandRequest{
		Command:   "echo",
		Args:      []string{"test"},
		Hostnames: []string{"localhost", "127.0.0.1"},
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	// Multi-host execution returns an array
	var results []map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &results)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Check each result has hostname field
	for _, r := range results {
		hostname, ok := r["hostname"].(string)
		require.True(t, ok)
		require.NotEmpty(t, hostname)
	}
}

func TestRunShellCommandTool_Execute_SingleHost(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	req := RunShellCommandRequest{
		Command:   "echo",
		Args:      []string{"test"},
		Hostnames: []string{"localhost"},
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	// Single-host execution returns a single object
	var shellResult RunShellCommandResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, 0, shellResult.ExitCode)
	require.Equal(t, "localhost", shellResult.Hostname)
}

func TestRunShellCommandTool_Execute_DefaultHostname(t *testing.T) {
	tool := &RunShellCommandTool{}
	ctx := context.Background()

	req := RunShellCommandRequest{
		Command: "echo",
		Args:    []string{"test"},
		// No hostnames specified - should default to localhost
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, reqJSON)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var shellResult RunShellCommandResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &shellResult)
	require.NoError(t, err)
	require.Equal(t, 0, shellResult.ExitCode)
	require.Equal(t, "localhost", shellResult.Hostname)
}

func TestValidateForSSHExecution_BlocksShellMetacharacters(t *testing.T) {
	tmpDir := "/tmp"
	if runtime.GOOS == "windows" {
		tmpDir = "C:\\tmp"
	}

	metacharacterTests := []struct {
		command     string
		args        []string
		workingDir  string
		expectError bool
	}{
		{"echo", []string{"test$(whoami)"}, "", true},                             // $ in args
		{"echo", []string{"test`whoami`"}, "", true},                              // backtick in args
		{"echo", []string{"test; rm -rf /"}, "", true},                            // semicolon in args
		{"echo", []string{"test& rm -rf /"}, "", true},                            // ampersand in args
		{"echo", []string{"test| rm -rf /"}, "", true},                            // pipe in args
		{"echo", []string{"test> /dev/null"}, "", true},                           // redirect in args
		{"echo", []string{"test< /etc/passwd"}, "", true},                         // redirect in args
		{"echo", []string{"test\nrm -rf /"}, "", true},                            // newline in args
		{"echo", []string{"test\r"}, "", true},                                    // carriage return in args
		{"echo", []string{"test\\n"}, "", true},                                   // backslash in args
		{"echo$(whoami)", []string{}, "", true},                                   // $ in command
		{"echo`whoami`", []string{}, "", true},                                    // backtick in command
		{"echo; rm", []string{}, "", true},                                        // semicolon in command
		{"echo", []string{"test"}, tmpDir + "/test\n", true},                      // newline in working dir
		{"echo", []string{"test"}, tmpDir + "/test;", true},                       // semicolon in working dir
		{"echo", []string{"test"}, tmpDir + "/test$", true},                       // $ in working dir
		{"echo", []string{"test"}, tmpDir + "/test`", true},                       // backtick in working dir
		{"echo", []string{"test"}, tmpDir + "/test\\", runtime.GOOS != "windows"}, // backslash in working dir (safe on Windows)
		{"echo", []string{"test"}, tmpDir + "/test|", true},                       // pipe in working dir
		{"echo", []string{"test"}, tmpDir + "/test>", true},                       // redirect in working dir
		{"echo", []string{"test"}, tmpDir + "/test<", true},                       // redirect in working dir
		{"echo", []string{"test"}, tmpDir + "/test&", true},                       // ampersand in working dir
		{"echo", []string{"test"}, tmpDir + "/test\r", true},                      // carriage return in working dir
		{"echo", []string{"test"}, "", false},                                     // safe: no metacharacters
		{"cat", []string{"/etc/hostname"}, "", false},                             // safe: normal command
	}

	for _, tc := range metacharacterTests {
		t.Run(tc.command+"_"+tc.workingDir, func(t *testing.T) {
			err := validateForSSHExecution(tc.command, tc.args, tc.workingDir)
			if tc.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "shell metacharacter")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateForSSHExecution_StillBlocksDenylist(t *testing.T) {
	// Ensure SSH validation still enforces the standard denylist
	dangerousCommands := []string{"rm", "sudo", "su", "dd"}

	for _, cmd := range dangerousCommands {
		t.Run(cmd, func(t *testing.T) {
			err := validateForSSHExecution(cmd, nil, "")
			require.Error(t, err)
		})
	}
}

func TestValidateForSSHExecution_AllowsSafeCommands(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	safeCommands := []struct {
		command    string
		args       []string
		workingDir string
	}{
		{"ls", []string{"-la"}, ""},
		{"echo", []string{"hello world"}, ""},
		{"cat", []string{"/etc/hostname"}, ""},
		{"grep", []string{"pattern", "file.txt"}, ""},
		{"find", []string{".", "-name", "*.go"}, ""},
		{"date", nil, ""},
		{"pwd", nil, tmpDir},
	}

	// Skip Linux-specific paths on Windows
	if runtime.GOOS != "windows" {
		safeCommands = append(safeCommands, struct {
			command    string
			args       []string
			workingDir string
		}{"ls", []string{}, "/var/log"})
	}

	for _, tc := range safeCommands {
		t.Run(tc.command, func(t *testing.T) {
			err := validateForSSHExecution(tc.command, tc.args, tc.workingDir)
			require.NoError(t, err)
		})
	}
}

func TestValidateCommandSafety_WorkingDirValidation(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	workingDirTests := []struct {
		name        string
		command     string
		args        []string
		workingDir  string
		expectError bool
		errorMsg    string
	}{
		{"valid absolute dir", "ls", []string{}, tmpDir, false, ""},
		{"path traversal", "ls", []string{}, tmpDir + "/../etc", true, "path traversal"},
		{"relative path", "ls", []string{}, "tmp", true, "absolute path"},
		{"empty working dir", "ls", []string{}, "", false, ""},
	}

	// Add Linux-specific tests only on non-Windows platforms
	if runtime.GOOS != "windows" {
		workingDirTests = append(workingDirTests,
			struct {
				name        string
				command     string
				args        []string
				workingDir  string
				expectError bool
				errorMsg    string
			}{"valid absolute dir with args", "pwd", nil, "/var/log", false, ""},
			struct {
				name        string
				command     string
				args        []string
				workingDir  string
				expectError bool
				errorMsg    string
			}{"nonexistent dir", "ls", []string{}, "/nonexistent_dir_12345", true, "does not exist"},
			struct {
				name        string
				command     string
				args        []string
				workingDir  string
				expectError bool
				errorMsg    string
			}{"file instead of dir", "ls", []string{}, "/etc/passwd", true, "not a directory"},
			struct {
				name        string
				command     string
				args        []string
				workingDir  string
				expectError bool
				errorMsg    string
			}{"complex valid path", "ls", []string{}, "/var/log", false, ""},
		)
	}

	for _, tc := range workingDirTests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCommandSafety(tc.command, tc.args, tc.workingDir)
			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
