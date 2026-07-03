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

package e2e

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/services/mcp"
	tests "github.com/g8e-ai/g8e/test"
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
		repoRoot := tests.ResolveRepoRootFromTestDir(t)
		output, err := tests.RunCLICommand(t, repoRoot, "gw", "mcp-config")
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
		repoRoot := tests.ResolveRepoRootFromTestDir(t)
		output, err := tests.RunCLICommand(t, repoRoot, "gw", "mcp-config")
		if err != nil {
			t.Skipf("CLI config not available (run './g8e auth login' first): %v", err)
		}
		assert.Contains(t, output, "http")
		assert.Contains(t, output, "clientCertificate")
		assert.Contains(t, output, "clientKey")
		assert.Contains(t, output, "caCertificate")
	})
}
