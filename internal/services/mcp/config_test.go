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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestNewGatewayConfig(t *testing.T) {
	gatewayURL := fmt.Sprintf("https://g8e.local:%d/mcp", constants.Ports.OperatorHttps)
	clientCertPath := "/path/to/client.crt"
	clientKeyPath := "/path/to/client.key"
	caCertPath := "/path/to/ca.crt"

	config, err := NewGatewayConfig(gatewayURL, clientCertPath, clientKeyPath, caCertPath)
	if err != nil {
		t.Fatalf("NewGatewayConfig failed: %v", err)
	}
	if config == nil {
		t.Fatal("NewGatewayConfig returned nil")
		return
	}

	if len(config.MCPServers) != 1 {
		t.Errorf("Expected 1 MCP server, got %d", len(config.MCPServers))
	}

	serverConfig, ok := config.MCPServers["g8e-gateway"]
	if !ok {
		t.Fatal("Expected 'g8e-gateway' server in MCPServers")
	}

	// Verify transport configuration
	if serverConfig.Transport.Type != "http" {
		t.Errorf("Expected transport type 'http', got '%s'", serverConfig.Transport.Type)
	}
	if serverConfig.Transport.URL != gatewayURL {
		t.Errorf("Expected URL '%s', got '%s'", gatewayURL, serverConfig.Transport.URL)
	}

	// Verify TLS configuration
	if serverConfig.TLS == nil {
		t.Fatal("Expected TLS configuration to be non-nil")
	}
	if serverConfig.TLS.ClientCertificate != clientCertPath {
		t.Errorf("Expected client certificate path '%s', got '%s'", clientCertPath, serverConfig.TLS.ClientCertificate)
	}
	if serverConfig.TLS.ClientKey != clientKeyPath {
		t.Errorf("Expected client key path '%s', got '%s'", clientKeyPath, serverConfig.TLS.ClientKey)
	}
	if serverConfig.TLS.CACertificate != caCertPath {
		t.Errorf("Expected CA certificate path '%s', got '%s'", caCertPath, serverConfig.TLS.CACertificate)
	}
	if !serverConfig.TLS.VerifyServer {
		t.Error("Expected VerifyServer to be true")
	}
	if serverConfig.TLS.VerifyHostname != "g8e.local" {
		t.Errorf("Expected VerifyHostname 'g8e.local', got '%s'", serverConfig.TLS.VerifyHostname)
	}

	// Verify capabilities
	if !serverConfig.Capabilities.Tools {
		t.Error("Expected Tools capability to be true")
	}
	if !serverConfig.Capabilities.Resources {
		t.Error("Expected Resources capability to be true")
	}
	if !serverConfig.Capabilities.Prompts {
		t.Error("Expected Prompts capability to be true")
	}

	// Verify description
	if serverConfig.Description != "g8e Gateway MCP endpoint" {
		t.Errorf("Expected description 'g8e Gateway MCP endpoint', got '%s'", serverConfig.Description)
	}

	// Verify notes
	if len(serverConfig.Notes) != 1 {
		t.Errorf("Expected 1 note, got %d", len(serverConfig.Notes))
	}
}

func TestConfigJSONSerialization(t *testing.T) {
	config, err := NewGatewayConfig(
		fmt.Sprintf("https://g8e.local:%d/mcp", constants.Ports.OperatorHttps),
		"/path/to/client.crt",
		"/path/to/client.key",
		"/path/to/ca.crt",
	)
	if err != nil {
		t.Fatalf("NewGatewayConfig failed: %v", err)
	}

	// Test JSON marshaling
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled Config
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal config from JSON: %v", err)
	}

	// Verify the unmarshaled config matches the original
	if len(unmarshaled.MCPServers) != len(config.MCPServers) {
		t.Errorf("Config mismatch after round-trip: expected %d servers, got %d",
			len(config.MCPServers), len(unmarshaled.MCPServers))
	}

	originalServer := config.MCPServers["g8e-gateway"]
	unmarshaledServer := unmarshaled.MCPServers["g8e-gateway"]

	if unmarshaledServer.Transport.Type != originalServer.Transport.Type {
		t.Errorf("Transport type mismatch: expected '%s', got '%s'",
			originalServer.Transport.Type, unmarshaledServer.Transport.Type)
	}

	if unmarshaledServer.TLS.VerifyHostname != originalServer.TLS.VerifyHostname {
		t.Errorf("VerifyHostname mismatch: expected '%s', got '%s'",
			originalServer.TLS.VerifyHostname, unmarshaledServer.TLS.VerifyHostname)
	}
}

func TestConfigEmptyServers(t *testing.T) {
	config := &Config{
		MCPServers: map[string]ServerConfig{},
	}

	if config.MCPServers == nil {
		t.Error("Expected MCPServers to be initialized")
	}

	if len(config.MCPServers) != 0 {
		t.Errorf("Expected empty MCPServers, got %d entries", len(config.MCPServers))
	}
}

func TestServerConfigDefaults(t *testing.T) {
	serverConfig := ServerConfig{
		Transport: TransportConfig{
			Type: "stdio",
		},
		Capabilities: Capabilities{
			Tools: true,
		},
		Description: "Test server",
	}

	// TLS should be optional (nil is valid)
	if serverConfig.TLS != nil {
		t.Error("Expected TLS to be nil when not provided")
	}

	// Headers should be optional (nil is valid)
	if serverConfig.Transport.Headers != nil {
		t.Error("Expected Headers to be nil when not provided")
	}

	// Notes should be optional (nil is valid)
	if serverConfig.Notes != nil {
		t.Error("Expected Notes to be nil when not provided")
	}
}

func TestNewGatewayConfigValidation(t *testing.T) {
	tests := []struct {
		name           string
		gatewayURL     string
		clientCert     string
		clientKey      string
		caCert         string
		verifyHostname string
		wantErr        bool
	}{
		{
			name:           "valid config",
			gatewayURL:     "https://g8e.local:8443/mcp",
			clientCert:     "/path/to/client.crt",
			clientKey:      "/path/to/client.key",
			caCert:         "/path/to/ca.crt",
			verifyHostname: "g8e.local",
			wantErr:        false,
		},
		{
			name:           "empty gateway URL",
			gatewayURL:     "",
			clientCert:     "/path/to/client.crt",
			clientKey:      "/path/to/client.key",
			caCert:         "/path/to/ca.crt",
			verifyHostname: "g8e.local",
			wantErr:        true,
		},
		{
			name:           "invalid URL scheme",
			gatewayURL:     "http://g8e.local:8443/mcp",
			clientCert:     "/path/to/client.crt",
			clientKey:      "/path/to/client.key",
			caCert:         "/path/to/ca.crt",
			verifyHostname: "g8e.local",
			wantErr:        true,
		},
		{
			name:           "empty client cert",
			gatewayURL:     "https://g8e.local:8443/mcp",
			clientCert:     "",
			clientKey:      "/path/to/client.key",
			caCert:         "/path/to/ca.crt",
			verifyHostname: "g8e.local",
			wantErr:        true,
		},
		{
			name:           "whitespace only client cert",
			gatewayURL:     "https://g8e.local:8443/mcp",
			clientCert:     "   ",
			clientKey:      "/path/to/client.key",
			caCert:         "/path/to/ca.crt",
			verifyHostname: "g8e.local",
			wantErr:        true,
		},
		{
			name:           "empty client key",
			gatewayURL:     "https://g8e.local:8443/mcp",
			clientCert:     "/path/to/client.crt",
			clientKey:      "",
			caCert:         "/path/to/ca.crt",
			verifyHostname: "g8e.local",
			wantErr:        true,
		},
		{
			name:           "empty CA cert",
			gatewayURL:     "https://g8e.local:8443/mcp",
			clientCert:     "/path/to/client.crt",
			clientKey:      "/path/to/client.key",
			caCert:         "",
			verifyHostname: "g8e.local",
			wantErr:        true,
		},
		{
			name:           "empty verify hostname",
			gatewayURL:     "https://g8e.local:8443/mcp",
			clientCert:     "/path/to/client.crt",
			clientKey:      "/path/to/client.key",
			caCert:         "/path/to/ca.crt",
			verifyHostname: "",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGatewayConfigWithHostname(tt.gatewayURL, tt.clientCert, tt.clientKey, tt.caCert, tt.verifyHostname)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGatewayConfigWithHostname() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
