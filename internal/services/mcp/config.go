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
func NewGatewayConfig(gatewayURL, clientCertPath, clientKeyPath, caCertPath string) *Config {
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
					VerifyHostname:    "g8e.local",
				},
				Capabilities: Capabilities{
					Tools:     true,
					Resources: true,
					Prompts:   true,
				},
				Description: "g8e Gateway MCP endpoint",
				Notes: []string{
					"Use the canonical g8e.local internal hostname through gateway-managed translation.",
					"mTLS is required for production access.",
				},
			},
		},
	}
}
