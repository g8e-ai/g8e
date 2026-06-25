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
	"fmt"
	"net/url"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
)

// Config represents the top-level MCP client configuration structure.
// This is the single source of truth for the MCP config schema used by:
// - CLI commands (gw mcp-config)
// - Tests (test/mcp_stdio_test.go)
// - Example templates (protocol/examples/mcp_server/g8e_gateway_mcp_config.json)
type Config struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ServerConfig represents a single MCP server configuration.
type ServerConfig struct {
	Transport    TransportConfig `json:"transport"`
	TLS          *TLSConfig      `json:"tls,omitempty"`
	Capabilities Capabilities    `json:"capabilities"`
	Description  string          `json:"description"`
	Notes        []string        `json:"notes"`
}

// TransportConfig represents the transport configuration.
type TransportConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// TLSConfig represents TLS configuration for HTTP transport.
type TLSConfig struct {
	ClientCertificateEnv string `json:"clientCertificateEnv,omitempty"`
	ClientKeyEnv         string `json:"clientKeyEnv,omitempty"`
	CACertificateEnv     string `json:"caCertificateEnv,omitempty"`
	ClientCertificate    string `json:"clientCertificate,omitempty"`
	ClientKey            string `json:"clientKey,omitempty"`
	CACertificate        string `json:"caCertificate,omitempty"`
	VerifyServer         bool   `json:"verifyServer"`
	VerifyHostname       string `json:"verifyHostname"`
}

// Capabilities represents the MCP server capabilities.
type Capabilities struct {
	Tools     bool `json:"tools"`
	Resources bool `json:"resources"`
	Prompts   bool `json:"prompts"`
}

// NewGatewayConfig creates a standard gateway MCP configuration with the given gateway URL and cert paths.
func NewGatewayConfig(gatewayURL, clientCertPath, clientKeyPath, caCertPath string) (*Config, error) {
	return NewGatewayConfigWithHostname(gatewayURL, clientCertPath, clientKeyPath, caCertPath, "g8e.local")
}

// NewGatewayConfigWithHostname creates a gateway MCP configuration with a custom hostname for verification.
func NewGatewayConfigWithHostname(gatewayURL, clientCertPath, clientKeyPath, caCertPath, verifyHostname string) (*Config, error) {
	if err := validateGatewayURL(gatewayURL); err != nil {
		return nil, fmt.Errorf("validate gateway URL: %w", err)
	}
	if err := validateCertPath(clientCertPath, "client certificate"); err != nil {
		return nil, fmt.Errorf("validate client certificate path: %w", err)
	}
	if err := validateCertPath(clientKeyPath, "client key"); err != nil {
		return nil, fmt.Errorf("validate client key path: %w", err)
	}
	if err := validateCertPath(caCertPath, "CA certificate"); err != nil {
		return nil, fmt.Errorf("validate CA certificate path: %w", err)
	}
	if verifyHostname == "" {
		return nil, constants.ErrMCPConfigVerifyHostnameEmpty
	}

	return &Config{
		MCPServers: map[string]ServerConfig{
			"g8e-gateway": {
				Transport: TransportConfig{
					Type: "http",
					URL:  gatewayURL,
				},
				TLS: &TLSConfig{
					ClientCertificate: clientCertPath,
					ClientKey:         clientKeyPath,
					CACertificate:     caCertPath,
					VerifyServer:      true,
					VerifyHostname:    verifyHostname,
				},
				Capabilities: Capabilities{
					Tools:     true,
					Resources: true,
					Prompts:   true,
				},
				Description: "g8e Gateway MCP endpoint",
				Notes: []string{
					"mTLS is required for production access.",
				},
			},
		},
	}, nil
}

// validateGatewayURL validates that the gateway URL is a valid HTTPS URL.
func validateGatewayURL(gatewayURL string) error {
	if gatewayURL == "" {
		return constants.ErrGatewayURLRequired
	}

	parsedURL, err := url.Parse(gatewayURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}

	if parsedURL.Scheme != "https" {
		return constants.ErrMCPConfigGatewayURLInvalidScheme
	}

	if parsedURL.Host == "" {
		return constants.ErrMCPConfigGatewayURLHostEmpty
	}

	return nil
}

// SimpleStdioServerConfig represents a simplified MCP server configuration for stdio transport.
// This format is compatible with Cursor/Devin MCP clients.
type SimpleStdioServerConfig struct {
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	Disabled bool     `json:"disabled"`
}

// SimpleConfig represents a simplified MCP client configuration structure.
type SimpleConfig struct {
	MCPServers map[string]SimpleStdioServerConfig `json:"mcpServers"`
}

// NewStdioConfigSimple creates a simplified stdio transport MCP configuration for local native tools.
// This format is compatible with Cursor/Devin MCP clients.
func NewStdioConfigSimple(g8eBinaryPath string) (*SimpleConfig, error) {
	if g8eBinaryPath == "" {
		return nil, constants.ErrMCPConfigBinaryPathEmpty
	}
	if strings.TrimSpace(g8eBinaryPath) == "" {
		return nil, constants.ErrMCPConfigBinaryPathWhitespace
	}

	return &SimpleConfig{
		MCPServers: map[string]SimpleStdioServerConfig{
			"g8e-native": {
				Command:  g8eBinaryPath,
				Args:     []string{"mcp", "stdio"},
				Disabled: false,
			},
		},
	}, nil
}

// validateCertPath validates that a certificate path is non-empty.
func validateCertPath(path, certType string) error {
	if path == "" {
		return constants.ErrMCPConfigCertPathEmpty
	}
	if strings.TrimSpace(path) == "" {
		return constants.ErrMCPConfigCertPathWhitespace
	}
	return nil
}
