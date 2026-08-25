// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/services/mcp"
)

// TestMCPConfigOutput_LocalConfigJSON validates that printMCPConfigLocal
// produces a valid MCP client configuration with HTTP transport, an HTTPS
// gateway URL containing /mcp, and TLS fields pointing at the CLI cert, key,
// and CA bundle paths from the resolved config. This is the hermetic
// replacement for the deleted TestMCPGateway_ConfigOutput E2E test, which
// shelled out to the g8e binary and skipped when local credentials were
// absent. This version overrides configLoad with a temp-rooted config from
// newCmdTestEnv so it runs as a Tier 1 unit test with no external
// dependencies.
func TestMCPConfigOutput_LocalConfigJSON(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	// Override configLoad so loadConfig("") returns the temp-rooted cfg
	// instead of reading from cwd. Restore the original after the test.
	originalConfigLoad := configLoad
	configLoad = func(string) (*config.Config, error) { return cfg, nil }
	t.Cleanup(func() { configLoad = originalConfigLoad })

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, printMCPConfigLocal(cmd), "printMCPConfigLocal must succeed with a temp-rooted config")

	output := buf.String()
	assert.Contains(t, output, "mcpServers", "local config output must contain mcpServers JSON")

	// Extract the JSON block from the output. The printer writes an
	// /etc/hosts comment line before the JSON, so find the first "{".
	jsonStart := strings.Index(output, "{")
	require.GreaterOrEqual(t, jsonStart, 0, "local config output must contain a JSON block")

	var mcpConfig mcp.Config
	require.NoError(t, json.NewDecoder(strings.NewReader(output[jsonStart:])).Decode(&mcpConfig),
		"local config output must decode as a valid mcp.Config")

	gatewayConfig, ok := mcpConfig.MCPServers["g8e-gateway"]
	require.True(t, ok, "local config must include a g8e-gateway server entry")

	assert.Equal(t, "http", gatewayConfig.Transport.Type,
		"local config transport type must be http")
	assert.Contains(t, gatewayConfig.Transport.URL, "https://",
		"local config transport URL must use an https gateway URL")
	assert.Contains(t, gatewayConfig.Transport.URL, "/mcp",
		"local config transport URL must point to the MCP API path")
	assert.Contains(t, gatewayConfig.Transport.URL, "g8e.local",
		"local config transport URL must use the g8e.local hostname")

	require.NotNil(t, gatewayConfig.TLS, "local config must include TLS configuration for http mode")
	assert.NotEmpty(t, gatewayConfig.TLS.ClientCertificate,
		"local config TLS must include a client certificate path")
	assert.NotEmpty(t, gatewayConfig.TLS.ClientKey,
		"local config TLS must include a client key path")
	assert.NotEmpty(t, gatewayConfig.TLS.CACertificate,
		"local config TLS must include a CA certificate path")
	assert.True(t, gatewayConfig.TLS.VerifyServer,
		"local config TLS must enable server verification")
	assert.Equal(t, "g8e.local", gatewayConfig.TLS.VerifyHostname,
		"local config TLS must verify the g8e.local hostname")

	assert.True(t, gatewayConfig.Capabilities.Tools,
		"local config must declare tools capability")
	assert.True(t, gatewayConfig.Capabilities.Resources,
		"local config must declare resources capability")
	assert.True(t, gatewayConfig.Capabilities.Prompts,
		"local config must declare prompts capability")
}

// TestMCPConfigOutput_IPConfigJSON validates that printMCPConfigIP produces
// a valid MCP client configuration with HTTP transport, an HTTPS gateway URL
// containing /mcp, and TLS fields. The IP config uses the external interface
// IP instead of g8e.local but still verifies against the g8e.local hostname
// because the gateway certificate carries g8e.local in its SAN. This is the
// hermetic replacement for the deleted TestMCPGateway_CommandExists E2E test.
func TestMCPConfigOutput_IPConfigJSON(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	originalConfigLoad := configLoad
	configLoad = func(string) (*config.Config, error) { return cfg, nil }
	t.Cleanup(func() { configLoad = originalConfigLoad })

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, printMCPConfigIP(cmd), "printMCPConfigIP must succeed with a temp-rooted config")

	output := buf.String()
	assert.Contains(t, output, "mcpServers", "IP config output must contain mcpServers JSON")

	jsonStart := strings.Index(output, "{")
	require.GreaterOrEqual(t, jsonStart, 0, "IP config output must contain a JSON block")

	var mcpConfig mcp.Config
	require.NoError(t, json.NewDecoder(strings.NewReader(output[jsonStart:])).Decode(&mcpConfig),
		"IP config output must decode as a valid mcp.Config")

	gatewayConfig, ok := mcpConfig.MCPServers["g8e-gateway"]
	require.True(t, ok, "IP config must include a g8e-gateway server entry")

	assert.Equal(t, "http", gatewayConfig.Transport.Type,
		"IP config transport type must be http")
	assert.Contains(t, gatewayConfig.Transport.URL, "https://",
		"IP config transport URL must use an https gateway URL")
	assert.Contains(t, gatewayConfig.Transport.URL, "/mcp",
		"IP config transport URL must point to the MCP API path")

	require.NotNil(t, gatewayConfig.TLS, "IP config must include TLS configuration for http mode")
	assert.NotEmpty(t, gatewayConfig.TLS.ClientCertificate,
		"IP config TLS must include a client certificate path")
	assert.NotEmpty(t, gatewayConfig.TLS.ClientKey,
		"IP config TLS must include a client key path")
	assert.NotEmpty(t, gatewayConfig.TLS.CACertificate,
		"IP config TLS must include a CA certificate path")
	assert.True(t, gatewayConfig.TLS.VerifyServer,
		"IP config TLS must enable server verification")
	assert.Equal(t, "g8e.local", gatewayConfig.TLS.VerifyHostname,
		"IP config TLS must verify the g8e.local hostname even when connecting via IP")
}

// TestMCPConfigOutput_AgentShowContainsAllSections validates that
// printAgentShow produces output containing all three config sections (g8e.local
// mTLS, IP Address mTLS, Stdio Transport) and the JSON field names that the
// deleted E2E test checked for (clientCertificate, clientKey, caCertificate).
// This is a hermetic Tier 1 test that overrides configLoad with a temp-rooted
// config.
func TestMCPConfigOutput_AgentShowContainsAllSections(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	originalConfigLoad := configLoad
	configLoad = func(string) (*config.Config, error) { return cfg, nil }
	t.Cleanup(func() { configLoad = originalConfigLoad })

	for _, agent := range getSupportedAgents() {
		t.Run(agent.ID, func(t *testing.T) {
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			require.NoError(t, printAgentShow(cmd, agent.ID),
				"printAgentShow must succeed for agent %s with a temp-rooted config", agent.ID)

			output := buf.String()
			assert.Contains(t, output, "g8e Gateway MCP Configurations",
				"agent show output must include the header")
			assert.Contains(t, output, "g8e.local",
				"agent show output must include the g8e.local section")
			assert.Contains(t, output, "IP Address",
				"agent show output must include the IP Address section")
			assert.Contains(t, output, "Stdio Transport",
				"agent show output must include the Stdio Transport section")

			// The JSON config blocks must contain the TLS field names
			// that agents consume. These are the same fields the deleted
			// E2E test checked for.
			assert.Contains(t, output, "clientCertificate",
				"agent show output must include clientCertificate TLS field")
			assert.Contains(t, output, "clientKey",
				"agent show output must include clientKey TLS field")
			assert.Contains(t, output, "caCertificate",
				"agent show output must include caCertificate TLS field")
			assert.Contains(t, output, "\"http\"",
				"agent show output must include the http transport type")
		})
	}
}

// TestMCPConfigOutput_StdioConfigJSON validates that printMCPConfigStdio
// produces a valid simplified stdio MCP configuration with the g8e binary
// path and the "mcp stdio" args. This covers the stdio transport section of
// the agent show output that the deleted E2E test did not explicitly verify.
func TestMCPConfigOutput_StdioConfigJSON(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	require.NoError(t, printMCPConfigStdio(cmd), "printMCPConfigStdio must succeed")

	output := buf.String()
	assert.Contains(t, output, "mcpServers", "stdio config output must contain mcpServers JSON")

	jsonStart := strings.Index(output, "{")
	require.GreaterOrEqual(t, jsonStart, 0, "stdio config output must contain a JSON block")

	var stdioConfig mcp.SimpleConfig
	require.NoError(t, json.NewDecoder(strings.NewReader(output[jsonStart:])).Decode(&stdioConfig),
		"stdio config output must decode as a valid mcp.SimpleConfig")

	nativeServer, ok := stdioConfig.MCPServers["g8e-native"]
	require.True(t, ok, "stdio config must include a g8e-native server entry")
	assert.NotEmpty(t, nativeServer.Command,
		"stdio config must include a non-empty command path (the g8e binary path from os.Executable)")
	assert.Equal(t, []string{"mcp", "stdio"}, nativeServer.Args,
		"stdio config args must be [mcp stdio]")
	assert.False(t, nativeServer.Disabled,
		"stdio config server must not be disabled")
}
