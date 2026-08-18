// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/services/mcp"
	tests "github.com/g8e-ai/g8e/test"
)

/*
TestMCPGateway_ConfigOutput validates that the "mcp agent show" command
produces valid HTTP transport configuration for universal gateway access.

This test verifies:
1. Default output uses HTTP transport (universal endpoint)
2. HTTP transport includes correct Gateway URL placeholder
3. TLS configuration is present for mTLS

Note: the old "gw mcp-config" command was replaced by "mcp agent show <agent>"
(see internal/cli/cmd/mcp.go), which prints multiple named config sections
(g8e.local mTLS, IP mTLS, stdio) rather than a single top-level JSON document.
*/
func TestMCPGateway_ConfigOutput(t *testing.T) {
	t.Run("http mode default", func(t *testing.T) {
		repoRoot := tests.ResolveRepoRootFromTestDir(t)
		output, err := tests.RunCLICommand(t, repoRoot, "mcp", "agent", "show", "cursor")
		if err != nil {
			t.Skipf("CLI config not available (run './g8e auth login' first): %v", err)
		}

		jsonStart := strings.Index(output, "{")
		require.GreaterOrEqual(t, jsonStart, 0, "output should contain a JSON config block")

		var config mcp.Config
		err = json.NewDecoder(strings.NewReader(output[jsonStart:])).Decode(&config)
		require.NoError(t, err, "output should contain valid JSON")

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
TestMCPGateway_CommandExists validates that the "mcp agent show" command
is available and produces valid output.
*/
func TestMCPGateway_CommandExists(t *testing.T) {
	t.Run("mcp agent show command exists", func(t *testing.T) {
		repoRoot := tests.ResolveRepoRootFromTestDir(t)
		output, err := tests.RunCLICommand(t, repoRoot, "mcp", "agent", "show", "cursor")
		if err != nil {
			t.Skipf("CLI config not available (run './g8e auth login' first): %v", err)
		}
		assert.Contains(t, output, "http")
		assert.Contains(t, output, "clientCertificate")
		assert.Contains(t, output, "clientKey")
		assert.Contains(t, output, "caCertificate")
	})
}
