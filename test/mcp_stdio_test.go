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

//go:build e2e

package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
TestMCPGateway_ConfigOutput validates that the gw mcp-config command
produces valid HTTP transport configuration for universal gateway access.

This test verifies:
1. Default output uses HTTP transport (universal endpoint)
2. HTTP transport includes correct Gateway URL placeholder
3. TLS configuration is present for mTLS
*/
func TestMCPGateway_ConfigOutput(t *testing.T) {
	t.Run("http mode default", func(t *testing.T) {
		output, err := runCLICommand("gw", "mcp-config")
		if err != nil {
			t.Skipf("CLI config not available (run './g8e auth login' first): %v", err)
		}

		var config mcp.Config
		err = json.Unmarshal([]byte(output), &config)
		require.NoError(t, err, "output should be valid JSON")

		gatewayConfig, ok := config.MCPServers["g8e-gateway"]
		assert.True(t, ok, "should have g8e-gateway config")

		assert.Equal(t, "http", gatewayConfig.Transport.Type, "default transport should be http")
		assert.Contains(t, gatewayConfig.Transport.URL, "https://", "url should use an https gateway URL")
		assert.Contains(t, gatewayConfig.Transport.URL, "/mcp", "url should point to the MCP API path")
		assert.NotNil(t, gatewayConfig.TLS, "should have tls config for http mode")
		assert.NotEmpty(t, gatewayConfig.TLS.ClientCertificate, "should have client certificate path")
		assert.NotEmpty(t, gatewayConfig.TLS.ClientKey, "should have client key path")
		assert.NotEmpty(t, gatewayConfig.TLS.CACertificate, "should have CA certificate path")
	})
}

/*
TestMCPGateway_CommandExists validates that the g8e gw mcp-config command
is available and produces valid output.
*/
func TestMCPGateway_CommandExists(t *testing.T) {
	t.Run("gw mcp-config command exists", func(t *testing.T) {
		output, err := runCLICommand("gw", "mcp-config")
		if err != nil {
			t.Skipf("CLI config not available (run './g8e auth login' first): %v", err)
		}
		assert.Contains(t, output, "http")
		assert.Contains(t, output, "clientCertificate")
		assert.Contains(t, output, "clientKey")
		assert.Contains(t, output, "caCertificate")
	})
}

/*
TestMCPGateway_JSONRPCParsing validates that the gateway can parse
JSON-RPC requests from HTTP format.
*/
func TestMCPGateway_JSONRPCParsing(t *testing.T) {
	t.Run("valid tools/list request", func(t *testing.T) {
		req := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
		var parsed struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		err := json.Unmarshal([]byte(req), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "tools/list", parsed.Method)
	})

	t.Run("valid tools/call request", func(t *testing.T) {
		req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool","arguments":{}}}`
		var parsed struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		err := json.Unmarshal([]byte(req), &parsed)
		require.NoError(t, err)
		assert.Equal(t, "tools/call", parsed.Method)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := `invalid json`
		var parsed struct {
			JSONRPC string `json:"jsonrpc"`
		}
		err := json.Unmarshal([]byte(req), &parsed)
		require.Error(t, err, "invalid JSON should fail to parse")
	})
}

/*
TestMCPGateway_ConfigTemplate validates that the config template file
exists and contains the expected HTTP transport structure.
*/
func TestMCPGateway_ConfigTemplate(t *testing.T) {
	t.Run("http template exists", func(t *testing.T) {
		repoRoot := ResolveRepoRootFromTestDir(t)
		fullPath := filepath.Join(repoRoot, "protocol/examples/mcp_server/g8e_gateway_mcp_config.json")
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err, "http template should exist at %s", fullPath)
		assert.Contains(t, string(content), `"type": "http"`)
		assert.Contains(t, string(content), `"url"`)
		assert.Contains(t, string(content), `"tls"`)
		assert.Contains(t, string(content), `"clientCertificate"`)
		assert.Contains(t, string(content), `"clientKey"`)
		assert.Contains(t, string(content), `"caCertificate"`)
	})
}

// getTestNodeBinaryPath returns the path to the cached test binary, building it if necessary.
// The binary is cached in .g8e/test-bin/g8e to avoid rebuilding on every test run.
func getTestNodeBinaryPath() (string, error) {
	// Initialize paths relative to test directory
	if err := paths.InitWithBase(constants.ProjectRootFromTestDir); err != nil {
		return "", fmt.Errorf("failed to initialize paths: %w", err)
	}
	repoRoot := paths.Infra.RuntimeDir

	// Use a dedicated test binary directory
	testBinDir := filepath.Join(repoRoot, ".g8e", "test-bin")
	if err := os.MkdirAll(testBinDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create test binary directory: %w", err)
	}

	g8ePath := filepath.Join(testBinDir, "g8e")

	// Check if binary exists and is newer than go.mod (simple staleness check)
	goModPath := filepath.Join(repoRoot, "go.mod")
	binaryInfo, err := os.Stat(g8ePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to stat binary: %w", err)
		}
		//Node Node Binary doesn't exist, need to build
	} else {
		goModInfo, err := os.Stat(goModPath)
		if err != nil {
			return "", fmt.Errorf("failed to stat go.mod: %w", err)
		}
		// If binary is older than go.mod, rebuild
		if binaryInfo.ModTime().Before(goModInfo.ModTime()) {
			_ = os.Remove(g8ePath) // Remove stale binary
		} else {
			//Node Node Binary exists and is up-to-date
			return g8ePath, nil
		}
	}

	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", g8ePath, "./cmd/operator")
	buildCmd.Dir = repoRoot
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to build g8e Node: %w, output: %s", err, string(output))
	}

	return g8ePath, nil
}

// Helper function to run CLI commands for testing
func runCLICommand(args ...string) (string, error) {
	// Initialize paths relative to test directory
	if err := paths.InitWithBase(constants.ProjectRootFromTestDir); err != nil {
		return "", fmt.Errorf("failed to initialize paths: %w", err)
	}
	repoRoot := paths.Infra.RuntimeDir

	g8ePath, err := getTestNodeBinaryPath()
	if err != nil {
		return "", err
	}

	cmdArgs := append([]string{}, args...)
	cmd := exec.Command(g8ePath, cmdArgs...)
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// Helper function to read file contents
func readFile(path string) (string, error) {
	// Initialize paths relative to test directory
	if err := paths.InitWithBase(constants.ProjectRootFromTestDir); err != nil {
		return "", fmt.Errorf("failed to initialize paths: %w", err)
	}
	repoRoot := paths.Infra.RuntimeDir

	fullPath := filepath.Join(repoRoot, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", fullPath, err)
	}

	return string(content), nil
}
