// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/pkg/ssh"
)

// NetSSHKnownHostsTool lists known hosts from SSH config and known_hosts files.
type NetSSHKnownHostsTool struct{}

// Name returns the tool identifier.
func (t *NetSSHKnownHostsTool) Name() string {
	return "net_ssh_known_hosts"
}

// Description returns a human-readable description.
func (t *NetSSHKnownHostsTool) Description() string {
	return "Lists known hosts from SSH config and known_hosts files based on OS type."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *NetSSHKnownHostsTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"ssh_config_path": {
				Type:        "string",
				Description: "Path to SSH config file (optional, defaults to ~/.ssh/config)",
			},
			"known_hosts_path": {
				Type:        "string",
				Description: "Path to known_hosts file (optional, defaults to ~/.ssh/known_hosts)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *NetSSHKnownHostsTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req NetSSHKnownHostsRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("net_ssh_known_hosts: %w", constants.ErrMCPUnmarshalArguments)
	}

	// Validate input paths
	if err := validateSSHConfigPath(req.SSHConfigPath); err != nil {
		return CallToolResult{}, fmt.Errorf("net_ssh_known_hosts: %w", err)
	}
	if err := validateKnownHostsPath(req.KnownHostsPath); err != nil {
		return CallToolResult{}, fmt.Errorf("net_ssh_known_hosts: %w", err)
	}

	// Determine default paths based on OS
	home, err := os.UserHomeDir()
	if err != nil {
		return CallToolResult{}, fmt.Errorf("net_ssh_known_hosts: %w", constants.ErrMCPGetHomeDirectory)
	}

	configPath := req.SSHConfigPath
	if configPath == "" {
		configPath = filepath.Join(home, constants.SshDirname, constants.SshConfigBasename)
	}

	knownHostsPath := req.KnownHostsPath
	if knownHostsPath == "" {
		knownHostsPath = filepath.Join(home, constants.SshDirname, constants.SshKnownHostsBasename)
	}

	// Parse SSH config
	configHosts := []SSHConfigHost{}
	blocks, err := ssh.ParseConfig(configPath)
	if err == nil {
		for pattern, block := range blocks {
			host := SSHConfigHost{
				Pattern:       pattern,
				Hostname:      block.Hostname,
				User:          block.User,
				Port:          block.Port,
				IdentityFiles: block.IdentityFiles,
				ProxyCommand:  block.ProxyCommand,
			}
			configHosts = append(configHosts, host)
		}
	}

	// Parse known_hosts file.
	// knownHostsPath is validated by validateKnownHostsPath to satisfy CodeQL uncontrolled-data-in-path-expression rule.
	knownHosts := []SSHKnownHost{}
	khFile, err := os.Open(knownHostsPath)
	if err == nil {
		defer khFile.Close()
		scanner := bufio.NewScanner(khFile)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			// Parse known_hosts line format:
			// host-pattern key-type public-key
			// or @marker host-pattern key-type public-key
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}

			// Skip markers (e.g., @cert-authority, @revoked)
			hostIdx := 0
			if strings.HasPrefix(parts[0], "@") {
				hostIdx = 1
			}

			if len(parts) <= hostIdx+1 {
				continue
			}

			hostPattern := parts[hostIdx]
			keyType := parts[hostIdx+1]

			// Create a hash of the key for security (don't expose full keys)
			keyHash := ""
			if len(parts) > hostIdx+2 {
				keyHash = fmt.Sprintf("%x...", len(parts[hostIdx+2]))
			}

			knownHost := SSHKnownHost{
				HostPattern: hostPattern,
				KeyType:     keyType,
				KeyHash:     keyHash,
			}
			knownHosts = append(knownHosts, knownHost)
		}
		if err := scanner.Err(); err != nil {
			return CallToolResult{}, fmt.Errorf("net_ssh_known_hosts: scan known_hosts file: %w", err)
		}
	}

	result := NetSSHKnownHostsResult{
		ConfigHosts:    configHosts,
		KnownHosts:     knownHosts,
		OS:             runtime.GOOS,
		ConfigPath:     configPath,
		KnownHostsPath: knownHostsPath,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("net_ssh_known_hosts: %w", constants.ErrMCPMarshalResult)
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
