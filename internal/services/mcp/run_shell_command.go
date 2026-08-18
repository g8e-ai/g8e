// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	sshlib "golang.org/x/crypto/ssh"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pkg/ssh"
)

// RunShellCommandTool runs shell commands with denylist enforcement and timeout limits.
type RunShellCommandTool struct{}

// Name returns the tool identifier.
func (t *RunShellCommandTool) Name() string {
	return "run_shell_command"
}

// Description returns a human-readable description.
func (t *RunShellCommandTool) Description() string {
	return "Executes shell commands with denylist enforcement for dangerous operations and timeout limits."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *RunShellCommandTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"command": {
				Type:        "string",
				Description: "Command to execute (e.g., 'ls', 'echo')",
			},
			"args": {
				Type:        "array",
				Description: "Command arguments (optional)",
			},
			"timeout": {
				Type:        "integer",
				Description: "Timeout in seconds (default: 30, max: 300)",
			},
			"working_dir": {
				Type:        "string",
				Description: "Working directory (optional, defaults to current directory)",
			},
			"hostnames": {
				Type:        "array",
				Description: "List of hostnames to execute on (optional, defaults to localhost). Uses SSH config for remote hosts.",
			},
		},
		Required: []string{"command"},
	}
}

// Execute implements the tool logic.
func (t *RunShellCommandTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req RunShellCommandRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("run_shell_command: unmarshal arguments: %w", err)
	}

	if req.Command == "" {
		return CallToolResult{}, fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandRequired)
	}

	// Validate against denylist
	if err := validateCommandSafety(req.Command, req.Args, req.WorkingDir); err != nil {
		result := RunShellCommandResult{
			ExitCode: -1,
			Stdout:   "",
			Stderr:   err.Error(),
			TimedOut: false,
			Error:    constants.ErrMCPRunShellCommandSafetyRejected.Error(),
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return CallToolResult{}, fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandMarshalResult)
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
		hostnames = []string{constants.LocalhostHostname}
	}

	// Set timeout limits
	timeout := time.Duration(constants.DefaultShellCommandTimeout) * time.Second
	if req.Timeout > 0 {
		if req.Timeout > constants.MaxShellCommandTimeout {
			return CallToolResult{}, fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandTimeoutExceeded)
		}
		timeout = time.Duration(req.Timeout) * time.Second
	}

	// Execute on each host and collect results
	var results []RunShellCommandResult
	for _, hostname := range hostnames {
		result, err := runShellCommandOnHost(ctx, hostname, req.Command, req.Args, req.WorkingDir, timeout)
		if err != nil {
			result = RunShellCommandResult{
				ExitCode: -1,
				Stdout:   "",
				Stderr:   err.Error(),
				TimedOut: false,
				Error:    err.Error(),
				Hostname: hostname,
			}
		} else {
			result.Hostname = hostname
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
		return CallToolResult{}, fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandMarshalResult)
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
func validateCommandSafety(command string, args []string, workingDir string) error {

	// Check base command
	cmdBase := command
	if idx := strings.Index(command, " "); idx >= 0 {
		cmdBase = command[:idx]
	}

	for _, dangerous := range constants.DangerousCommands {
		if cmdBase == dangerous {
			return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandBlocked, dangerous)
		}
	}

	// Check for dangerous patterns in arguments
	fullCommand := command
	if len(args) > 0 {
		fullCommand = strings.Join(append([]string{command}, args...), " ")
	}

	lowerCmd := strings.ToLower(fullCommand)
	for _, pattern := range constants.DangerousPatterns {
		if strings.Contains(lowerCmd, pattern) {
			return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandDangerousPattern, pattern)
		}
	}

	// Check for shell injection attempts
	for _, pattern := range constants.ShellInjectionPatterns {
		if strings.Contains(lowerCmd, pattern) {
			return fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandShellInjection)
		}
	}

	// Validate working directory if specified
	if workingDir != "" {
		// Check for path traversal attempts BEFORE cleaning
		if strings.Contains(workingDir, "..") {
			return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandPathTraversal, workingDir)
		}

		// Ensure the path is absolute
		if !filepath.IsAbs(workingDir) {
			return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandAbsolutePathRequired, workingDir)
		}

		// Clean the path to resolve any redundant components
		cleanDir := filepath.Clean(workingDir)

		// Verify the directory exists and is accessible
		info, err := os.Stat(cleanDir)
		if err != nil {
			return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandDirNotAccessible, workingDir)
		}

		// Ensure it's actually a directory
		if !info.IsDir() {
			return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandNotDirectory, workingDir)
		}
	}

	return nil
}

// shellQuoteArg quotes a single argument for safe shell execution.
// Uses POSIX shell quoting: replace ' with '"'"' and wrap in single quotes.
func shellQuoteArg(arg string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(arg, "'", "'\"'\"'"))
}

// shellQuoteCommand builds a safely quoted shell command from command and args.
func shellQuoteCommand(command string, args []string) string {
	quotedCmd := shellQuoteArg(command)
	if len(args) == 0 {
		return quotedCmd
	}
	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		quotedArgs[i] = shellQuoteArg(arg)
	}
	return fmt.Sprintf("%s %s", quotedCmd, strings.Join(quotedArgs, " "))
}

// validateForSSHExecution performs stricter validation for SSH execution
// since SSH session.Run() executes through a remote shell.
func validateForSSHExecution(command string, args []string, workingDir string) error {
	// Additional SSH-specific validation: ensure no shell metacharacters
	// can slip through even with quoting - check this FIRST before other validation

	// Check command for shell metacharacters
	for _, meta := range constants.ShellMetacharacters {
		if strings.Contains(command, meta) {
			return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandShellMetacharacter, meta)
		}
	}

	// Check args for shell metacharacters
	for _, arg := range args {
		for _, meta := range constants.ShellMetacharacters {
			if strings.Contains(arg, meta) {
				return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandArgMetacharacter, meta)
			}
		}
	}

	// Check working directory for shell metacharacters
	if workingDir != "" {
		for _, meta := range constants.ShellMetacharacters {
			// On Windows, backslash is a legitimate path separator
			if meta == "\\" && runtime.GOOS == "windows" {
				continue
			}
			if strings.Contains(workingDir, meta) {
				return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandDirMetacharacter, meta)
			}
		}
	}

	// Run the standard safety validation but without workingDir
	// since SSH executes on remote hosts where we can't verify local directory existence
	if err := validateCommandSafety(command, args, ""); err != nil {
		return err
	}

	// For SSH, only validate working directory path structure (not existence)
	if workingDir != "" {
		// Clean the path to resolve any relative components
		cleanDir := filepath.Clean(workingDir)

		// Check for path traversal attempts
		if strings.Contains(cleanDir, "..") {
			return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandPathTraversal, workingDir)
		}

		// Ensure the path is absolute
		if !filepath.IsAbs(cleanDir) {
			return fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandAbsolutePathRequired, workingDir)
		}
	}

	return nil
}

// runShellCommandOnHost executes a command on a specific host (localhost or remote via SSH).
func runShellCommandOnHost(ctx context.Context, hostname, command string, args []string, workingDir string, timeout time.Duration) (RunShellCommandResult, error) {
	// Local execution
	if hostname == constants.LocalhostHostname || hostname == constants.LocalhostIP {
		return runShellCommandLocally(ctx, command, args, workingDir, timeout)
	}

	// Remote execution via SSH
	return runShellCommandViaSSH(ctx, hostname, command, args, workingDir, timeout)
}

// runShellCommandLocally executes a command on the local machine.
func runShellCommandLocally(ctx context.Context, command string, args []string, workingDir string, timeout time.Duration) (RunShellCommandResult, error) {
	// Create command context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command with safe argument passing
	// Use command as the executable name and args as separate arguments
	// This prevents shell injection by avoiding shell interpretation
	var cmd *exec.Cmd
	useShell := false
	resolvedPath, errLook := exec.LookPath(command)
	if errLook != nil {
		useShell = true
	} else {
		// Read first 2 bytes to check for shebang (shell script)
		if file, err := os.Open(resolvedPath); err == nil {
			buf := make([]byte, 2)
			if n, err := file.Read(buf); err == nil && n == 2 {
				if string(buf) == "#!" {
					useShell = true
				}
			}
			file.Close()
		}
	}

	if useShell {
		// Invoke via sh to ensure proper script execution
		shArgs := append([]string{"-c", `exec "$0" "$@"`, command}, args...)
		cmd = exec.CommandContext(cmdCtx, "sh", shArgs...)
	} else {
		cmd = exec.CommandContext(cmdCtx, command, args...)
	}
	if workingDir != "" {
		// workingDir is validated by validateFilePath to satisfy CodeQL command-injection rule.
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

	result := RunShellCommandResult{
		ExitCode: exitCode,
		Stdout:   string(stdout),
		Stderr:   string(stderr),
		TimedOut: timedOut,
	}

	if timedOut {
		result.Error = constants.ErrMCPRunShellCommandTimedOut.Error()
	} else if err != nil {
		result.Error = err.Error()
	}

	return result, nil
}

// runShellCommandViaSSH executes a command on a remote host via SSH.
func runShellCommandViaSSH(ctx context.Context, hostname, command string, args []string, workingDir string, timeout time.Duration) (RunShellCommandResult, error) {
	// Validate command and args for SSH execution to prevent shell injection
	// SSH session.Run() executes through a remote shell, so we must be stricter
	if err := validateForSSHExecution(command, args, workingDir); err != nil {
		return RunShellCommandResult{}, err
	}

	// Resolve SSH connection parameters
	r, err := ssh.ResolveHost(hostname, "", "", "", "")
	if err != nil {
		return RunShellCommandResult{}, fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandResolveHost)
	}
	if r.Hostname == "" {
		return RunShellCommandResult{}, fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandHostnameResolveFailed, hostname)
	}

	// Build auth methods
	authMethods, err := ssh.BuildAuthMethods(r, "", "")
	if err != nil {
		return RunShellCommandResult{}, fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandBuildAuth)
	}
	if len(authMethods) == 0 {
		return RunShellCommandResult{}, fmt.Errorf("run_shell_command: %w: %s", constants.ErrMCPRunShellCommandNoAuth, hostname)
	}

	// Build host key callback
	hostKeyCallback, err := ssh.BuildHostKeyCallback("")
	if err != nil {
		return RunShellCommandResult{}, fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandHostKeyVerification)
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
		return RunShellCommandResult{}, fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandSSHDial)
	}
	defer client.Close()

	// Create session
	session, err := client.NewSession()
	if err != nil {
		return RunShellCommandResult{}, fmt.Errorf("run_shell_command: %w", constants.ErrMCPRunShellCommandSSHSession)
	}
	defer session.Close()

	// Build full command with args using proper shell quoting
	// This prevents shell injection by properly escaping each argument
	fullCmd := shellQuoteCommand(command, args)

	// Add working directory if specified (properly quoted to prevent injection)
	if workingDir != "" {
		quotedDir := shellQuoteArg(workingDir)
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

	result := RunShellCommandResult{
		ExitCode: exitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		TimedOut: timedOut,
	}

	if timedOut {
		result.Error = constants.ErrMCPRunShellCommandTimedOut.Error()
	} else if err != nil {
		result.Error = err.Error()
	}

	return result, nil
}
