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
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	sshlib "golang.org/x/crypto/ssh"

	"github.com/g8e-ai/g8e/internal/pkg/ssh"
)

// OperatorDeployTool deploys the g8e operator to remote hosts.
type OperatorDeployTool struct{}

// Name returns the tool identifier.
func (t *OperatorDeployTool) Name() string {
	return "operator_deploy"
}

// Description returns a human-readable description.
func (t *OperatorDeployTool) Description() string {
	return "Deploys the g8e operator to a list of remote hosts via SSH."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *OperatorDeployTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"hostnames": {
				Type:        "array",
				Description: "List of hostnames to deploy the operator to",
			},
			"operator_binary": {
				Type:        "string",
				Description: "Path to the operator binary (optional, defaults to current g8e binary)",
			},
			"operator_args": {
				Type:        "array",
				Description: "Additional arguments to pass to the operator (optional)",
			},
			"timeout": {
				Type:        "integer",
				Description: "Timeout in seconds for deployment (default: 300, max: 600)",
			},
		},
		Required: []string{"hostnames"},
	}
}

// Execute implements the tool logic.
func (t *OperatorDeployTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req OperatorDeployRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("operator_deploy: unmarshal arguments: %w", err)
	}

	// Validate inputs
	if err := validateHostnames(req.Hostnames); err != nil {
		return CallToolResult{}, fmt.Errorf("operator_deploy: %w", err)
	}
	if err := validateOperatorBinaryPath(req.OperatorBinary); err != nil {
		return CallToolResult{}, fmt.Errorf("operator_deploy: %w", err)
	}
	if err := validateOperatorArgs(req.OperatorArgs); err != nil {
		return CallToolResult{}, fmt.Errorf("operator_deploy: %w", err)
	}

	// Determine operator binary path
	operatorBinary := req.OperatorBinary
	if operatorBinary == "" {
		// Default to the current executable
		execPath, err := os.Executable()
		if err != nil {
			return CallToolResult{}, fmt.Errorf("operator_deploy: get current executable: %w", err)
		}
		operatorBinary = execPath
	}

	// Validate operator binary exists.
	// operatorBinary is validated by validateOperatorBinaryPath to satisfy CodeQL uncontrolled-data-in-path-expression rule.
	if _, err := os.Stat(operatorBinary); err != nil {
		return CallToolResult{}, fmt.Errorf("operator_deploy: operator binary not found: %w", err)
	}

	// Set timeout
	timeout := 300 * time.Second
	if req.Timeout > 0 {
		if req.Timeout > 600 {
			return CallToolResult{}, fmt.Errorf("operator_deploy: timeout cannot exceed 600 seconds")
		}
		timeout = time.Duration(req.Timeout) * time.Second
	}

	// Build operator arguments
	operatorArgs := req.OperatorArgs
	if operatorArgs == nil {
		operatorArgs = []string{}
	}

	// Deploy to each host
	var deployments []OperatorDeploymentResult
	for _, hostname := range req.Hostnames {
		result := t.deployToHost(ctx, hostname, operatorBinary, operatorArgs, timeout)
		deployments = append(deployments, result)
	}

	result := OperatorDeployResult{
		Deployments: deployments,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("operator_deploy: marshal result: %w", err)
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

// deployToHost deploys the operator to a specific host.
func (t *OperatorDeployTool) deployToHost(ctx context.Context, hostname, operatorBinary string, operatorArgs []string, timeout time.Duration) OperatorDeploymentResult {
	// Local deployment
	if hostname == "localhost" || hostname == "127.0.0.1" {
		return t.deployLocally(ctx, hostname, operatorBinary, operatorArgs, timeout)
	}

	// Remote deployment via SSH
	return t.deployViaSSH(ctx, hostname, operatorBinary, operatorArgs, timeout)
}

// deployLocally deploys the operator on the local machine.
func (t *OperatorDeployTool) deployLocally(ctx context.Context, hostname, operatorBinary string, operatorArgs []string, timeout time.Duration) OperatorDeploymentResult {
	result := OperatorDeploymentResult{
		Hostname: hostname,
		Success:  false,
	}

	// Create command context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command with separate args to prevent shell injection.
	// operatorBinary is validated by validateOperatorBinaryPath, operatorArgs by validateOperatorArgs.
	cmd := exec.CommandContext(cmdCtx, operatorBinary, operatorArgs...)

	// Execute command
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()

	result.Output = stdoutBuf.String()
	result.Message = "Local deployment completed"

	if err != nil {
		result.Error = err.Error()
		result.Message = fmt.Sprintf("Local deployment failed: %s", stderrBuf.String())
		return result
	}

	result.Success = true
	result.Message = "Operator deployed successfully"
	return result
}

// deployViaSSH deploys the operator to a remote host via SSH.
func (t *OperatorDeployTool) deployViaSSH(ctx context.Context, hostname, operatorBinary string, operatorArgs []string, timeout time.Duration) OperatorDeploymentResult {
	result := OperatorDeploymentResult{
		Hostname: hostname,
		Success:  false,
	}

	// Resolve SSH connection parameters
	r, err := ssh.ResolveHost(hostname, "", "", "", "")
	if err != nil {
		result.Error = fmt.Sprintf("resolve host: %v", err)
		result.Message = "Failed to resolve SSH host"
		return result
	}
	if r.Hostname == "" {
		result.Error = "failed to resolve hostname"
		result.Message = "Failed to resolve SSH host"
		return result
	}

	// Build auth methods
	authMethods, err := ssh.BuildAuthMethods(r, "", "")
	if err != nil {
		result.Error = fmt.Sprintf("build auth methods: %v", err)
		result.Message = "Failed to build SSH authentication"
		return result
	}
	if len(authMethods) == 0 {
		result.Error = "no SSH auth methods available"
		result.Message = "No SSH authentication methods available"
		return result
	}

	// Build host key callback
	hostKeyCallback, err := ssh.BuildHostKeyCallback("")
	if err != nil {
		result.Error = fmt.Sprintf("host key verification: %v", err)
		result.Message = "Failed to verify host key"
		return result
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
		result.Error = fmt.Sprintf("SSH dial failed: %v", err)
		result.Message = "Failed to connect via SSH"
		return result
	}
	defer client.Close()

	// Create session
	session, err := client.NewSession()
	if err != nil {
		result.Error = fmt.Sprintf("SSH session creation failed: %v", err)
		result.Message = "Failed to create SSH session"
		return result
	}
	defer session.Close()

	// Determine if we need to transfer the binary
	// For now, we'll assume the operator is already present on the remote host
	// or we'll try to execute it directly if it's a local path that can be transferred

	// Check if operator binary is a local file that needs to be transferred
	if filepath.IsAbs(operatorBinary) || !strings.Contains(operatorBinary, "/") {
		// Transfer the binary to the remote host
		remotePath := "/tmp/g8e-operator"
		if err := t.transferBinaryViaSCP(client, operatorBinary, remotePath); err != nil {
			result.Error = fmt.Sprintf("transfer binary: %v", err)
			result.Message = fmt.Sprintf("Failed to transfer operator binary: %v", err)
			return result
		}
		operatorBinary = remotePath
	}

	// Build command with proper shell quoting
	fullCmd := shellQuoteCommand(operatorBinary, operatorArgs)

	// Execute command with timeout and separate stdout/stderr.
	// fullCmd is built by shellQuoteCommand which properly quotes arguments to prevent shell injection.
	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf
	err = session.Run(fullCmd)

	result.Output = stdoutBuf.String()

	if err != nil {
		result.Error = err.Error()
		result.Message = fmt.Sprintf("Remote deployment failed: %s", stderrBuf.String())
		return result
	}

	result.Success = true
	result.Message = "Operator deployed successfully via SSH"
	return result
}

// transferBinaryViaSCP transfers a binary file to the remote host via SFTP.
func (t *OperatorDeployTool) transferBinaryViaSCP(client *sshlib.Client, localPath, remotePath string) error {
	// Create SFTP client
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Open local file
	srcFile, err := os.Open(filepath.Clean(localPath))
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer srcFile.Close()

	// Create remote file
	dstFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote file: %w", err)
	}
	defer dstFile.Close()

	// Copy file contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy file contents: %w", err)
	}

	// Make the file executable
	if err := sftpClient.Chmod(remotePath, 0755); err != nil {
		return fmt.Errorf("chmod file: %w", err)
	}

	return nil
}
