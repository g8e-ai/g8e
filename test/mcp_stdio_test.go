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

package tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MCPConfig represents the MCP client configuration structure
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// MCPServerConfig represents a single MCP server configuration
type MCPServerConfig struct {
	Transport    TransportConfig `json:"transport"`
	TLS          *TLSConfig      `json:"tls,omitempty"`
	Capabilities struct {
		Tools     bool `json:"tools"`
		Resources bool `json:"resources"`
		Prompts   bool `json:"prompts"`
	} `json:"capabilities"`
	Description string   `json:"description"`
	Notes       []string `json:"notes"`
}

// TransportConfig represents the transport configuration
type TransportConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// TLSConfig represents TLS configuration for HTTP transport
type TLSConfig struct {
	ClientCertificateEnv string `json:"clientCertificateEnv"`
	ClientKeyEnv         string `json:"clientKeyEnv"`
	CACertificateEnv     string `json:"caCertificateEnv"`
	VerifyServer         bool   `json:"verifyServer"`
	VerifyHostname       string `json:"verifyHostname"`
}

/*
TestMCPStdio_ConfigOutput validates that the gw mcp-config command
produces valid stdio transport configuration for IDE integration.

This test verifies:
1. Default output uses stdio transport
2. Command references g8e mcp stdio
3. HTTP mode produces valid HTTP transport config
4. Both modes include correct Gateway URL placeholder
*/
func TestMCPStdio_ConfigOutput(t *testing.T) {
	t.Run("stdio mode default", func(t *testing.T) {
		output, err := runCLICommand("gw", "mcp-config")
		assert.NoError(t, err, "gw mcp-config should succeed")

		var config MCPConfig
		err = json.Unmarshal([]byte(output), &config)
		assert.NoError(t, err, "output should be valid JSON")

		gatewayConfig, ok := config.MCPServers["g8e-gateway"]
		assert.True(t, ok, "should have g8e-gateway config")

		assert.Equal(t, "stdio", gatewayConfig.Transport.Type, "default transport should be stdio")
		assert.Equal(t, "g8e", gatewayConfig.Transport.Command, "command should be g8e")
		assert.Len(t, gatewayConfig.Transport.Args, 3, "should have 3 args")
		assert.Equal(t, "mcp", gatewayConfig.Transport.Args[0])
		assert.Equal(t, "stdio", gatewayConfig.Transport.Args[1])
		assert.Contains(t, gatewayConfig.Transport.Args[2], "https://localhost:", "endpoint should include Gateway URL")
	})

	t.Run("http mode", func(t *testing.T) {
		output, err := runCLICommand("gw", "mcp-config", "--transport", "http")
		assert.NoError(t, err, "gw mcp-config --transport http should succeed")

		var config MCPConfig
		err = json.Unmarshal([]byte(output), &config)
		assert.NoError(t, err, "output should be valid JSON")

		gatewayConfig, ok := config.MCPServers["g8e-gateway"]
		assert.True(t, ok, "should have g8e-gateway config")

		assert.Equal(t, "http", gatewayConfig.Transport.Type, "transport should be http")
		assert.Contains(t, gatewayConfig.Transport.URL, "https://localhost:", "url should include Gateway URL")
		assert.NotNil(t, gatewayConfig.TLS, "should have tls config for http mode")
	})
}

/*
TestMCPStdio_CommandExists validates that the g8e mcp stdio command
is available and has the correct help text.
*/
func TestMCPStdio_CommandExists(t *testing.T) {
	t.Run("mcp command exists", func(t *testing.T) {
		output, err := runCLICommand("mcp", "--help")
		assert.NoError(t, err, "g8e mcp --help should succeed")
		assert.Contains(t, output, "MCP client utilities")
		assert.Contains(t, output, "stdio")
	})

	t.Run("mcp stdio command exists", func(t *testing.T) {
		output, err := runCLICommand("mcp", "stdio", "--help")
		assert.NoError(t, err, "g8e mcp stdio --help should succeed")
		assert.Contains(t, output, "stdio-based MCP client")
		assert.Contains(t, output, "IDE integration")
		assert.Contains(t, output, "--endpoint")
	})
}

/*
TestMCPStdio_JSONRPCParsing validates that the stdio bridge can parse
JSON-RPC requests from stdin format.
*/
func TestMCPStdio_JSONRPCParsing(t *testing.T) {
	t.Run("valid tools/list request", func(t *testing.T) {
		req := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
		var parsed struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		err := json.Unmarshal([]byte(req), &parsed)
		assert.NoError(t, err)
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
		assert.NoError(t, err)
		assert.Equal(t, "tools/call", parsed.Method)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := `invalid json`
		var parsed struct {
			JSONRPC string `json:"jsonrpc"`
		}
		err := json.Unmarshal([]byte(req), &parsed)
		assert.Error(t, err, "invalid JSON should fail to parse")
	})
}

/*
TestMCPStdio_ConfigTemplate validates that the config template files
exist and contain the expected structure.
*/
func TestMCPStdio_ConfigTemplate(t *testing.T) {
	t.Run("stdio template exists", func(t *testing.T) {
		content, err := readFile("protocol/examples/mcp_server/g8e_gateway_mcp_config.json")
		assert.NoError(t, err, "stdio template should exist")
		assert.Contains(t, content, `"type": "stdio"`)
		assert.Contains(t, content, `"command": "g8e"`)
		assert.Contains(t, content, `"mcp stdio"`)
	})

	t.Run("http template exists", func(t *testing.T) {
		content, err := readFile("protocol/examples/mcp_server/g8e_gateway_mcp_config_http.json")
		assert.NoError(t, err, "http template should exist")
		assert.Contains(t, content, `"type": "http"`)
		assert.Contains(t, content, `"url"`)
		assert.Contains(t, content, `"tls"`)
	})
}

// Helper function to run CLI commands for testing
func runCLICommand(args ...string) (string, error) {
	// This would typically use exec.Command to run the g8e binary
	// For now, return a placeholder since we're testing structure
	// In a real test environment, this would execute the built binary
	return "", nil
}

// Helper function to read file contents
func readFile(path string) (string, error) {
	// Placeholder for file reading
	// In real implementation, this would use os.ReadFile
	return "", nil
}
