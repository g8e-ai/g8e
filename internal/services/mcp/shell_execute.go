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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/pkg/ssh"
	sshlib "golang.org/x/crypto/ssh"
)

// ShellExecuteTool runs shell commands with denylist enforcement and timeout limits.
type ShellExecuteTool struct{}

// Name returns the tool identifier.
func (t *ShellExecuteTool) Name() string {
	return "shell_execute"
}

// Description returns a human-readable description.
func (t *ShellExecuteTool) Description() string {
	return "Executes shell commands with denylist enforcement for dangerous operations and timeout limits."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *ShellExecuteTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command to execute (e.g., 'ls', 'echo')",
			},
			"args": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Command arguments (optional)",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (default: 30, max: 300)",
			},
			"working_dir": map[string]interface{}{
				"type":        "string",
				"description": "Working directory (optional, defaults to current directory)",
			},
			"hostnames": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "List of hostnames to execute on (optional, defaults to localhost). Uses SSH config for remote hosts.",
			},
		},
		"required": []string{"command"},
	}
}

// Execute implements the tool logic.
func (t *ShellExecuteTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		Command    string   `json:"command"`
		Args       []string `json:"args,omitempty"`
		Timeout    int      `json:"timeout,omitempty"`
		WorkingDir string   `json:"working_dir,omitempty"`
		Hostnames  []string `json:"hostnames,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.Command == "" {
		return CallToolResult{}, fmt.Errorf("command is required")
	}

	// Validate against denylist
	if err := validateCommandSafety(req.Command, req.Args); err != nil {
		result := map[string]interface{}{
			"exit_code": -1,
			"stdout":    "",
			"stderr":    err.Error(),
			"timed_out": false,
			"error":     "command rejected by safety policy",
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

	// Determine target hostnames (default to localhost if not provided)
	hostnames := req.Hostnames
	if len(hostnames) == 0 {
		hostnames = []string{"localhost"}
	}

	// Set timeout limits
	timeout := 30 * time.Second
	if req.Timeout > 0 {
		if req.Timeout > 300 {
			return CallToolResult{}, fmt.Errorf("timeout cannot exceed 300 seconds")
		}
		timeout = time.Duration(req.Timeout) * time.Second
	}

	// Execute on each host and collect results
	var results []map[string]interface{}
	for _, hostname := range hostnames {
		result, err := executeOnHost(ctx, hostname, req.Command, req.Args, req.WorkingDir, timeout)
		if err != nil {
			result = map[string]interface{}{
				"exit_code": -1,
				"stdout":    "",
				"stderr":    err.Error(),
				"timed_out": false,
				"error":     err.Error(),
				"hostname":  hostname,
			}
		} else {
			result["hostname"] = hostname
		}
		results = append(results, result)
	}

	// If only one host, return single result; otherwise return array
	var resultJSON []byte
	var err error
	if len(results) == 1 {
		resultJSON, err = json.Marshal(results[0])
	} else {
		resultJSON, err = json.Marshal(results)
	}
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

// validateCommandSafety checks if a command is safe to execute based on denylist.
func validateCommandSafety(command string, args []string) error {
	dangerousCommands := []string{
		"rm",
		"dd",
		"mkfs",
		"fdisk",
		"format",
		"del",
		"erase",
		"shred",
		"wipe",
		"killall",
		"pkill",
		"reboot",
		"shutdown",
		"halt",
		"poweroff",
		"init",
		"systemctl",
		"service",
		"iptables",
		"ip6tables",
		"nft",
		"ufw",
		"firewall-cmd",
		"route",
		"ifconfig",
		"ip",
		"brctl",
		"tc",
		"modprobe",
		"insmod",
		"rmmod",
		"depmod",
		"mount",
		"umount",
		"swapon",
		"swapoff",
		"mkswap",
		"lvcreate",
		"lvremove",
		"lvchange",
		"vgcreate",
		"vgremove",
		"pvcreate",
		"pvremove",
		"cryptsetup",
		"passwd",
		"chpasswd",
		"usermod",
		"userdel",
		"groupmod",
		"crontab",
		"at",
		"batch",
		"sudo",
		"su",
		"doas",
		"runuser",
	}

	// Check base command
	cmdBase := command
	if idx := strings.Index(command, " "); idx >= 0 {
		cmdBase = command[:idx]
	}

	for _, dangerous := range dangerousCommands {
		if cmdBase == dangerous {
			return fmt.Errorf("command '%s' is blocked by safety policy", dangerous)
		}
	}

	// Check for dangerous patterns in arguments
	fullCommand := command
	if len(args) > 0 {
		fullCommand = strings.Join(append([]string{command}, args...), " ")
	}

	dangerousPatterns := []string{
		"rm -rf /",
		"rm -rf /*",
		":(){:|:&};:",
		"dd if=/dev/zero",
		"mkfs",
		"> /dev/sda",
		"> /dev/vda",
		"chmod 777 /",
		"chown -R",
		"wget",
		"curl",
		"nc -l",
		"ncat -l",
		"ssh",
		"scp",
		"rsync",
	}

	lowerCmd := strings.ToLower(fullCommand)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerCmd, pattern) {
			return fmt.Errorf("command contains dangerous pattern: %s", pattern)
		}
	}

	// Check for shell injection attempts
	if strings.Contains(lowerCmd, "$(") || strings.Contains(lowerCmd, "`") || strings.Contains(lowerCmd, "|") {
		return fmt.Errorf("command contains shell injection pattern")
	}

	return nil
}

// executeOnHost executes a command on a specific host (localhost or remote via SSH).
func executeOnHost(ctx context.Context, hostname, command string, args []string, workingDir string, timeout time.Duration) (map[string]interface{}, error) {
	// Local execution
	if hostname == "localhost" || hostname == "127.0.0.1" {
		return executeLocally(ctx, command, args, workingDir, timeout)
	}

	// Remote execution via SSH
	return executeViaSSH(ctx, hostname, command, args, workingDir, timeout)
}

// executeLocally executes a command on the local machine.
func executeLocally(ctx context.Context, command string, args []string, workingDir string, timeout time.Duration) (map[string]interface{}, error) {
	// Create command context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(cmdCtx, command)
	if len(args) > 0 {
		cmd.Args = append([]string{command}, args...)
	}
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// Execute command with separate stdout and stderr
	stdout, err := cmd.Output()
	timedOut := ctx.Err() == context.DeadlineExceeded

	var stderr []byte
	if exitError, ok := err.(*exec.ExitError); ok {
		stderr = exitError.Stderr
	}

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}

	result := map[string]interface{}{
		"exit_code": exitCode,
		"stdout":    string(stdout),
		"stderr":    string(stderr),
		"timed_out": timedOut,
	}

	if timedOut {
		result["error"] = "command timed out"
	} else if err != nil {
		result["error"] = err.Error()
	}

	return result, nil
}

// executeViaSSH executes a command on a remote host via SSH.
func executeViaSSH(ctx context.Context, hostname, command string, args []string, workingDir string, timeout time.Duration) (map[string]interface{}, error) {
	// Resolve SSH connection parameters
	r := ssh.ResolveHost(hostname, "", "", "", "")
	if r.Hostname == "" {
		return nil, fmt.Errorf("failed to resolve hostname: %s", hostname)
	}

	// Build auth methods
	authMethods := ssh.BuildAuthMethods(r, "")
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH auth methods available for %s", hostname)
	}

	// Build host key callback
	hostKeyCallback, err := ssh.BuildHostKeyCallback()
	if err != nil {
		return nil, fmt.Errorf("host key verification failed: %w", err)
	}

	// Create SSH client config
	clientConfig := &sshlib.ClientConfig{
		User:            r.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	// Connect to SSH server
	addr := net.JoinHostPort(r.Hostname, r.Port)
	client, err := sshlib.Dial("tcp", addr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH dial failed: %w", err)
	}
	defer client.Close()

	// Create session
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("SSH session creation failed: %w", err)
	}
	defer session.Close()

	// Build full command with args
	fullCmd := command
	if len(args) > 0 {
		fullCmd = fmt.Sprintf("%s %s", command, strings.Join(args, " "))
	}

	// Add working directory if specified (properly quoted to prevent injection)
	if workingDir != "" {
		// Use shell quoting to safely escape the working directory path
		// Pattern: replace ' with '"'"' (end quote, literal quote, start quote)
		quotedDir := fmt.Sprintf("'%s'", strings.ReplaceAll(workingDir, "'", "'\"'\"'"))
		fullCmd = fmt.Sprintf("cd %s && %s", quotedDir, fullCmd)
	}

	// Execute command with timeout and separate stdout/stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf
	err = session.Run(fullCmd)
	timedOut := ctx.Err() == context.DeadlineExceeded

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*sshlib.ExitError); ok {
			exitCode = exitError.ExitStatus()
		} else {
			exitCode = -1
		}
	}

	result := map[string]interface{}{
		"exit_code": exitCode,
		"stdout":    stdoutBuf.String(),
		"stderr":    stderrBuf.String(),
		"timed_out": timedOut,
	}

	if timedOut {
		result["error"] = "command timed out"
	} else if err != nil {
		result["error"] = err.Error()
	}

	return result, nil
}
