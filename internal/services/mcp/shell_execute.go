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
	"os/exec"
	"strings"
	"time"
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

	// Set timeout limits
	timeout := 30 * time.Second
	if req.Timeout > 0 {
		if req.Timeout > 300 {
			return CallToolResult{}, fmt.Errorf("timeout cannot exceed 300 seconds")
		}
		timeout = time.Duration(req.Timeout) * time.Second
	}

	// Create command context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(cmdCtx, req.Command)
	if len(req.Args) > 0 {
		cmd.Args = append([]string{req.Command}, req.Args...)
	}
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	// Execute command
	output, err := cmd.CombinedOutput()
	timedOut := ctx.Err() == context.DeadlineExceeded

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
		"stdout":    string(output),
		"stderr":    "",
		"timed_out": timedOut,
	}

	if timedOut {
		result["error"] = "command timed out"
	} else if err != nil {
		result["error"] = err.Error()
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
